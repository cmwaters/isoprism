package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/isoprism/api/internal/ai"
	"github.com/isoprism/api/internal/api/handlers"
	"github.com/isoprism/api/internal/config"
	"github.com/isoprism/api/internal/events"
	"github.com/isoprism/api/internal/github"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(cfg *config.Config, db *pgxpool.Pool, appClient *github.AppClient) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.FrontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	enricher := ai.NewEnricher(cfg.AnthropicAPIKey)

	ghHandler := &handlers.GitHubHandler{
		DB:            db,
		AppClient:     appClient,
		WebhookSecret: cfg.GitHubWebhookSecret,
		FrontendURL:   cfg.FrontendURL,
		Enricher:      enricher,
	}
	repoHandler := &handlers.RepoHandler{DB: db}
	queueHandler := &handlers.QueueHandler{DB: db}
	graphHandler := &handlers.GraphHandler{DB: db, AppClient: appClient}

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Debug endpoints — no auth, for development use only
	r.Post("/debug/repos/{repoID}/reindex", func(w http.ResponseWriter, r *http.Request) {
		repoID := chi.URLParam(r, "repoID")
		db.Exec(r.Context(), `update repositories set index_status='pending' where id=$1`, repoID)
		go func() {
			bgCtx := context.Background()
			events.RepoInit(bgCtx, db, appClient, enricher, repoID)
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "reindex_started", "repo_id": repoID})
	})

	r.Post("/debug/prs/{prID}/reprocess", func(w http.ResponseWriter, r *http.Request) {
		prID := chi.URLParam(r, "prID")
		go func() {
			bgCtx := context.Background()
			events.OpenPR(bgCtx, db, appClient, enricher, prID)
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "reprocess_started", "pr_id": prID})
	})

	// Sync PR metadata (title, body, author) from GitHub — useful when webhook was missed.
	r.Post("/debug/prs/{prID}/sync", func(w http.ResponseWriter, r *http.Request) {
		prID := chi.URLParam(r, "prID")
		ctx := r.Context()

		var fullName string
		var installationID int64
		var prNumber int
		err := db.QueryRow(ctx, `
			select r.full_name, gi.installation_id, pr.number
			from pull_requests pr
			join repositories r on r.id = pr.repo_id
			join github_installations gi on gi.id = r.installation_id
			where pr.id = $1
		`, prID).Scan(&fullName, &installationID, &prNumber)
		if err != nil {
			http.Error(w, "pr not found", http.StatusNotFound)
			return
		}

		ghClient, err := appClient.ClientForInstallation(ctx, installationID)
		if err != nil {
			http.Error(w, "github client error", http.StatusInternalServerError)
			return
		}

		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			http.Error(w, "invalid repo name", http.StatusInternalServerError)
			return
		}

		pr, err := ghClient.GetPullRequest(ctx, parts[0], parts[1], prNumber)
		if err != nil {
			http.Error(w, "github fetch error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		db.Exec(ctx, `
			update pull_requests set
				title = $1, body = $2, author_login = $3, author_avatar_url = $4,
				state = $5, head_commit_sha = $6, base_commit_sha = $7
			where id = $8
		`, pr.Title, pr.Body, pr.User.Login, pr.User.AvatarURL,
			pr.State, pr.Head.SHA, pr.Base.SHA, prID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "synced",
			"pr_id":       prID,
			"author":      pr.User.Login,
			"body_length": len(pr.Body),
			"additions":   pr.Additions,
			"deletions":   pr.Deletions,
		})
	})

	// Public routes
	r.Post("/webhooks/github", ghHandler.HandleWebhook)
	r.Get("/api/v1/github/callback", ghHandler.HandleInstallationCallback)
	r.Get("/api/v1/auth/status", repoHandler.GetAuthStatus)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(supabaseAuthMiddleware(cfg.SupabaseURL))

		r.Get("/api/v1/me/repos", repoHandler.ListMyRepos)
		r.Delete("/api/v1/me", repoHandler.DeleteMe)

		r.Route("/api/v1/repos/{repoID}", func(r chi.Router) {
			r.Get("/", repoHandler.GetRepo)
			r.Post("/index", indexRepoHandler(db, appClient, enricher))
			r.Get("/status", repoHandler.GetRepoStatus)
			r.Get("/queue", queueHandler.GetQueue)
			r.Get("/graph", graphHandler.GetRepoGraph)
			r.Get("/nodes/{nodeID}/code", graphHandler.GetRepoNodeCode)
			r.Get("/prs/{prID}/graph", graphHandler.GetGraph)
			r.Get("/prs/number/{number}/graph", graphHandler.GetGraphByNumber)
			r.Get("/prs/{prID}/nodes/{nodeID}/code", graphHandler.GetNodeCode)
		})
	})

	return r
}

// indexRepoHandler returns a handler that triggers RepoInit for a repo.
func indexRepoHandler(db *pgxpool.Pool, appClient *github.AppClient, enricher *ai.Enricher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := chi.URLParam(r, "repoID")
		userID := r.Header.Get("X-User-ID")
		ctx := r.Context()

		var exists bool
		db.QueryRow(ctx, `select exists(select 1 from repositories where id=$1 and user_id=$2)`, repoID, userID).Scan(&exists)
		if !exists {
			http.Error(w, "repo not found", http.StatusNotFound)
			return
		}

		db.Exec(ctx, `update repositories set index_status='pending' where id=$1`, repoID)

		go func() {
			bgCtx := context.Background()
			events.RepoInit(bgCtx, db, appClient, enricher, repoID)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "indexing_started"})
	}
}

func supabaseAuthMiddleware(_ string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}
			sub, ok := claims["sub"].(string)
			if !ok || sub == "" {
				http.Error(w, "missing sub claim", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), "userID", sub)
			r = r.WithContext(ctx)
			r.Header.Set("X-User-ID", sub)
			next.ServeHTTP(w, r)
		})
	}
}
