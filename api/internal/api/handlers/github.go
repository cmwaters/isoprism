package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/isoprism/api/internal/ai"
	"github.com/isoprism/api/internal/events"
	"github.com/isoprism/api/internal/github"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GitHubHandler struct {
	DB            *pgxpool.Pool
	AppClient     *github.AppClient
	WebhookSecret string
	FrontendURL   string
	FrontendURLs  []string
	Enricher      *ai.Enricher
}

// POST /webhooks/github
func (h *GitHubHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := github.ReadAndVerify(r, h.WebhookSecret)
	if err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	log.Printf("webhook: event=%q", event)
	switch event {
	case "pull_request":
		h.handlePREvent(r.Context(), body)
	case "installation":
		h.handleInstallationEvent(r.Context(), body)
	case "installation_repositories":
		h.handleInstallationReposEvent(r.Context(), body)
	case "ping":
		// acknowledge
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── pull_request ──────────────────────────────────────────────────────────────

func (h *GitHubHandler) handlePREvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRPayload(body)
	if err != nil {
		log.Printf("webhook: PR parse error: %v", err)
		return
	}
	log.Printf("webhook: PR action=%q repo=%q pr=#%d", payload.Action, payload.Repository.FullName, payload.PullRequest.Number)

	// Resolve repo in DB
	var repoID string
	err = h.DB.QueryRow(ctx, `
		select r.id from repositories r
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1 and r.github_repo_id = $2 and r.is_active = true
	`, payload.Installation.ID, payload.Repository.ID).Scan(&repoID)
	if err != nil {
		log.Printf("webhook: no repo found for installation=%d repo_id=%d", payload.Installation.ID, payload.Repository.ID)
		return
	}

	pr := payload.PullRequest
	state := pr.State
	if pr.MergedAt != nil {
		state = "merged"
	}

	switch payload.Action {
	case "opened", "synchronize", "reopened", "ready_for_review":
		// Upsert PR
		var prDBID string
		err = h.DB.QueryRow(ctx, `
			insert into pull_requests (
				repo_id, github_pr_id, number, title, body,
				author_login, author_avatar_url, base_branch, head_branch,
				base_commit_sha, head_commit_sha, state, draft, html_url, opened_at
			) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			on conflict (repo_id, github_pr_id) do update set
				title           = excluded.title,
				body            = excluded.body,
				author_login    = excluded.author_login,
				author_avatar_url = excluded.author_avatar_url,
				base_branch     = excluded.base_branch,
				head_branch     = excluded.head_branch,
				base_commit_sha = excluded.base_commit_sha,
				state           = excluded.state,
				draft           = excluded.draft,
				head_commit_sha = excluded.head_commit_sha,
				html_url        = excluded.html_url,
				graph_status    = 'pending'
			returning id
		`,
			repoID, pr.ID, pr.Number, pr.Title, pr.Body,
			pr.User.Login, pr.User.AvatarURL, pr.Base.Ref, pr.Head.Ref,
			pr.Base.SHA, pr.Head.SHA, state, pr.Draft, pr.HTMLURL, pr.CreatedAt,
		).Scan(&prDBID)
		if err != nil {
			log.Printf("webhook: upsert PR error: %v", err)
			return
		}
		// Trigger OpenPR in background
		go events.OpenPR(context.Background(), h.DB, h.AppClient, h.Enricher, prDBID)

	case "closed":
		// Update state; if merged, advance main_commit_sha
		_, _ = h.DB.Exec(ctx, `
			update pull_requests set state=$1 where repo_id=$2 and github_pr_id=$3
		`, state, repoID, pr.ID)

		if pr.MergedAt != nil && pr.MergeCommitSHA != nil {
			go events.MergePR(context.Background(), h.DB, h.AppClient, repoID, *pr.MergeCommitSHA)
		}
	}
}

// ── installation ──────────────────────────────────────────────────────────────

func (h *GitHubHandler) handleInstallationEvent(ctx context.Context, body []byte) {
	payload, err := github.ParseInstallationPayload(body)
	if err != nil {
		return
	}
	if payload.Action == "deleted" {
		_, _ = h.DB.Exec(ctx, `delete from github_installations where installation_id=$1`, payload.Installation.ID)
	}
}

// ── installation_repositories ─────────────────────────────────────────────────

func (h *GitHubHandler) handleInstallationReposEvent(ctx context.Context, body []byte) {
	payload, err := github.ParseInstallationReposPayload(body)
	if err != nil {
		return
	}

	var dbInstallationID string
	err = h.DB.QueryRow(ctx, `select id from github_installations where installation_id=$1`, payload.Installation.ID).Scan(&dbInstallationID)
	if err != nil {
		log.Printf("webhook: installation_repositories: installation not found id=%d", payload.Installation.ID)
		return
	}

	// Find user_id linked to this installation via existing repos
	var userID string
	h.DB.QueryRow(ctx, `select user_id from repositories where installation_id=$1 limit 1`, dbInstallationID).Scan(&userID)

	for _, repo := range payload.RepositoriesAdded {
		if userID == "" {
			continue
		}
		_, _ = h.DB.Exec(ctx, `
			insert into repositories (user_id, installation_id, github_repo_id, full_name, is_active)
			values ($1,$2,$3,$4,true)
			on conflict (user_id, github_repo_id) do update set full_name=excluded.full_name, is_active=true
		`, userID, dbInstallationID, repo.ID, repo.FullName)
	}
	for _, repo := range payload.RepositoriesRemoved {
		_, _ = h.DB.Exec(ctx, `update repositories set is_active=false where installation_id=$1 and github_repo_id=$2`, dbInstallationID, repo.ID)
	}
}

// ── Installation callback (GitHub App OAuth redirect) ─────────────────────────

// GET /api/v1/github/callback
func (h *GitHubHandler) HandleInstallationCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	installationIDStr := r.URL.Query().Get("installation_id")
	userID, redirectBaseURL := h.parseInstallState(r.URL.Query().Get("state"))
	setupAction := r.URL.Query().Get("setup_action")

	if setupAction == "request" {
		http.Redirect(w, r, redirectBaseURL+"/request-sent", http.StatusFound)
		return
	}

	if installationIDStr == "" {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}
	var installationID int64
	fmt.Sscanf(installationIDStr, "%d", &installationID)

	// Fetch installation details from GitHub
	installation, err := h.AppClient.GetInstallation(ctx, installationID)
	if err != nil {
		log.Printf("callback: failed to fetch installation: %v", err)
		http.Error(w, "failed to fetch installation", http.StatusInternalServerError)
		return
	}

	// Upsert installation record (no org_id in new schema)
	var dbInstallationID string
	err = h.DB.QueryRow(ctx, `
		insert into github_installations (installation_id, account_login, account_type, account_avatar_url)
		values ($1,$2,$3,$4)
		on conflict (installation_id) do update set
			account_login     = excluded.account_login,
			account_avatar_url = excluded.account_avatar_url
		returning id
	`, installationID, installation.Account.Login, installation.Account.Type, installation.Account.AvatarURL,
	).Scan(&dbInstallationID)
	if err != nil {
		log.Printf("callback: upsert installation: %v", err)
		http.Error(w, "failed to save installation", http.StatusInternalServerError)
		return
	}

	// Ensure user row exists
	if userID != "" {
		ensureUserExists(ctx, h.DB, userID)
	}

	// Fetch repos for this installation and upsert them
	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("callback: get GitHub client: %v", err)
	} else {
		repos, err := ghClient.ListInstallationRepos(ctx)
		if err != nil {
			log.Printf("callback: list repos: %v", err)
		} else {
			for _, repo := range repos {
				if userID != "" {
					_, _ = h.DB.Exec(ctx, `
						insert into repositories (user_id, installation_id, github_repo_id, full_name, default_branch, is_active)
						values ($1,$2,$3,$4,$5,true)
						on conflict (user_id, github_repo_id) do update set
							full_name      = excluded.full_name,
							default_branch = excluded.default_branch,
							is_active      = true
					`, userID, dbInstallationID, repo.ID, repo.FullName, repo.DefaultBranch)
				}
			}
		}
	}

	if setupAction == "update" {
		http.Redirect(w, r, redirectBaseURL+"/onboarding/repos", http.StatusFound)
	} else {
		http.Redirect(w, r, redirectBaseURL+"/onboarding/repos", http.StatusFound)
	}
}

type installState struct {
	UserID      string `json:"user_id"`
	FrontendURL string `json:"frontend_url"`
}

func (h *GitHubHandler) parseInstallState(raw string) (string, string) {
	redirectBaseURL := h.FrontendURL
	if raw == "" {
		return "", redirectBaseURL
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return raw, redirectBaseURL
	}

	var state installState
	if err := json.Unmarshal(decoded, &state); err != nil {
		return raw, redirectBaseURL
	}
	if h.isAllowedFrontendURL(state.FrontendURL) {
		redirectBaseURL = state.FrontendURL
	}
	return state.UserID, redirectBaseURL
}

func (h *GitHubHandler) isAllowedFrontendURL(frontendURL string) bool {
	for _, allowed := range h.FrontendURLs {
		if frontendURL == allowed {
			return true
		}
	}
	return false
}

func splitRepoName(fullName string) []string {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	return parts
}
