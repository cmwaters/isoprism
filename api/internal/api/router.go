package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aperture/api/internal/api/handlers"
	"github.com/aperture/api/internal/config"
	"github.com/aperture/api/internal/github"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/golang-jwt/jwt/v5"
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

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Get("/api/v1/debug/stats", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		stats := map[string]interface{}{}

		var orgs, repos, activerepos, prs, openprs, installs int
		db.QueryRow(ctx, `select count(*) from organizations`).Scan(&orgs)
		db.QueryRow(ctx, `select count(*) from repositories`).Scan(&repos)
		db.QueryRow(ctx, `select count(*) from repositories where is_active`).Scan(&activerepos)
		db.QueryRow(ctx, `select count(*) from pull_requests`).Scan(&prs)
		db.QueryRow(ctx, `select count(*) from pull_requests where state = 'open'`).Scan(&openprs)
		db.QueryRow(ctx, `select count(*) from github_installations`).Scan(&installs)

		stats["organizations"] = orgs
		stats["installations"] = installs
		stats["repositories"] = map[string]int{"total": repos, "active": activerepos}
		stats["pull_requests"] = map[string]int{"total": prs, "open": openprs}

		// Most recent PRs
		rows, _ := db.Query(ctx, `
			select r.full_name, pr.number, pr.title, pr.state, pr.created_at
			from pull_requests pr join repositories r on r.id = pr.repo_id
			order by pr.created_at desc limit 5
		`)
		if rows != nil {
			defer rows.Close()
			type recentPR struct {
				Repo   string `json:"repo"`
				Number int    `json:"number"`
				Title  string `json:"title"`
				State  string `json:"state"`
				At     string `json:"created_at"`
			}
			var recent []recentPR
			for rows.Next() {
				var p recentPR
				var at interface{}
				rows.Scan(&p.Repo, &p.Number, &p.Title, &p.State, &at)
				p.At = fmt.Sprintf("%v", at)
				recent = append(recent, p)
			}
			stats["recent_prs"] = recent
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	ghHandler := &handlers.GitHubHandler{
		DB:            db,
		AppClient:     appClient,
		WebhookSecret: cfg.GitHubWebhookSecret,
		FrontendURL:   cfg.FrontendURL,
	}
	orgHandler := &handlers.OrgHandler{DB: db, AppClient: appClient}
	queueHandler := &handlers.QueueHandler{DB: db}

	// Public routes (no auth)
	r.Post("/webhooks/github", ghHandler.HandleWebhook)
	r.Get("/api/v1/github/callback", ghHandler.HandleInstallationCallback)
	r.Get("/api/v1/auth/status", orgHandler.GetAuthStatus)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(supabaseAuthMiddleware(cfg.SupabaseURL))

		r.Get("/api/v1/me/orgs", orgHandler.ListMyOrgs)
		r.Delete("/api/v1/me", orgHandler.DeleteMe)

		r.Route("/api/v1/orgs/{orgSlug}", func(r chi.Router) {
			r.Get("/", orgHandler.GetOrg)
			r.Get("/queue", queueHandler.GetQueue)
			r.Get("/repos", orgHandler.ListRepos)
			r.Patch("/repos/{repoID}", orgHandler.UpdateRepo)
			r.Delete("/repos/{repoID}", orgHandler.DeleteRepo)
			r.Post("/repos/{repoID}/sync", ghHandler.SyncRepo)
			r.Get("/prs/{prID}", orgHandler.GetPR)
			r.Get("/teams", orgHandler.ListTeams)
			r.Post("/join-requests", orgHandler.CreateJoinRequest)
			r.Get("/join-requests", orgHandler.ListJoinRequests)
			r.Patch("/join-requests/{requestID}", orgHandler.UpdateJoinRequest)
		})
	})

	return r
}

func supabaseAuthMiddleware(supabaseURL string) func(http.Handler) http.Handler {
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
