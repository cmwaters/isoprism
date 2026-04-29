package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/isoprism/api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RepoHandler struct {
	DB *pgxpool.Pool
}

// GET /api/v1/auth/status?user_id={uuid}
// Shared redirect helper for the landing page and auth callback.
// Returns a redirect URL based on whether the user has a ready repo.
func (h *RepoHandler) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if userID == "" {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/login"})
		return
	}

	// Ensure user row exists (created by Supabase trigger, but may be missing
	// if the trigger hasn't fired yet).
	ensureUserExists(ctx, h.DB, userID)

	// Check for a ready repo
	var fullName string
	err := h.DB.QueryRow(ctx, `
		select full_name from repositories
		where user_id = $1 and index_status = 'ready' and is_active = true
		order by created_at desc limit 1
	`, userID).Scan(&fullName)
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/" + fullName})
		return
	}

	// Check for any repos (installation done but not indexed yet)
	var hasRepos bool
	h.DB.QueryRow(ctx, `
		select exists(select 1 from repositories where user_id = $1)
	`, userID).Scan(&hasRepos)

	if hasRepos {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding/repos"})
	} else {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding"})
	}
}

// GET /api/v1/me/repos
func (h *RepoHandler) ListMyRepos(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	rows, err := h.DB.Query(ctx, `
		select r.id, r.user_id, r.installation_id, r.github_repo_id, r.full_name,
		       r.default_branch, r.main_commit_sha, r.index_status, r.is_active, r.created_at
		from repositories r
		where r.user_id = $1 and r.is_active = true
		order by r.full_name
	`, userID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	repos := make([]models.Repository, 0)
	for rows.Next() {
		var repo models.Repository
		if err := rows.Scan(
			&repo.ID, &repo.UserID, &repo.InstallationID, &repo.GitHubRepoID,
			&repo.FullName, &repo.DefaultBranch, &repo.MainCommitSHA,
			&repo.IndexStatus, &repo.IsActive, &repo.CreatedAt,
		); err != nil {
			log.Printf("ListMyRepos: scan error: %v", err)
			continue
		}
		repos = append(repos, repo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"repos": repos})
}

// GET /api/v1/repos/{repoID}
func (h *RepoHandler) GetRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var repo models.Repository
	err := h.DB.QueryRow(ctx, `
		select id, user_id, installation_id, github_repo_id, full_name,
		       default_branch, main_commit_sha, index_status, is_active, created_at
		from repositories where id = $1 and user_id = $2
	`, repoID, userID).Scan(
		&repo.ID, &repo.UserID, &repo.InstallationID, &repo.GitHubRepoID,
		&repo.FullName, &repo.DefaultBranch, &repo.MainCommitSHA,
		&repo.IndexStatus, &repo.IsActive, &repo.CreatedAt,
	)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repo)
}

// GET /api/v1/repos/{repoID}/status
func (h *RepoHandler) GetRepoStatus(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var indexStatus string
	err := h.DB.QueryRow(ctx, `
		select index_status from repositories where id = $1 and user_id = $2
	`, repoID, userID).Scan(&indexStatus)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	var prCount, readyCount int
	h.DB.QueryRow(ctx, `
		select
			count(*) filter (
				where pr.state = 'open'
				  and pr.draft = false
				  and pr.base_branch = 'main'
				  and pr.base_commit_sha = r.main_commit_sha
			),
			count(*) filter (
				where pr.state = 'open'
				  and pr.draft = false
				  and pr.graph_status = 'ready'
				  and pr.base_branch = 'main'
				  and pr.base_commit_sha = r.main_commit_sha
			)
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		where pr.repo_id = $1
	`, repoID).Scan(&prCount, &readyCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"index_status": indexStatus,
		"pr_count":     prCount,
		"ready_count":  readyCount,
	})
}

// DELETE /api/v1/me — delete the current user's account and all their data.
func (h *RepoHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	if _, err := h.DB.Exec(ctx, `delete from users where id = $1`, userID); err != nil {
		http.Error(w, "failed to delete account", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureUserExists creates a users row for the given Supabase user ID if it
// doesn't already exist. This is a no-op when the auth trigger has already
// created the row.
func ensureUserExists(ctx context.Context, db *pgxpool.Pool, userID string) {
	_, _ = db.Exec(ctx, `
		insert into users (id, email) values ($1, '') on conflict (id) do nothing
	`, userID)
}
