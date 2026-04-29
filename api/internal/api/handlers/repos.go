package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

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

	response := map[string]interface{}{
		"index_status": indexStatus,
		"pr_count":     prCount,
		"ready_count":  readyCount,
	}

	var job struct {
		CommitSHA  string
		Status     string
		Phase      string
		Message    sql.NullString
		FilesTotal int
		FilesDone  int
		NodesTotal int
		NodesDone  int
		EdgesTotal int
		EdgesDone  int
		StartedAt  sql.NullTime
		UpdatedAt  time.Time
		Error      sql.NullString
	}
	err = h.DB.QueryRow(ctx, `
		select commit_sha, status, phase, message,
		       files_total, files_done, nodes_total, nodes_done, edges_total, edges_done,
		       started_at, updated_at, error
		from indexing_jobs
		where repo_id=$1
		order by created_at desc
		limit 1
	`, repoID).Scan(
		&job.CommitSHA, &job.Status, &job.Phase, &job.Message,
		&job.FilesTotal, &job.FilesDone, &job.NodesTotal, &job.NodesDone, &job.EdgesTotal, &job.EdgesDone,
		&job.StartedAt, &job.UpdatedAt, &job.Error,
	)
	if err == nil {
		percent := indexPercent(job.Status, job.Phase, job.FilesTotal, job.FilesDone, job.NodesTotal, job.NodesDone, job.EdgesTotal, job.EdgesDone)
		jobInfo := map[string]interface{}{
			"commit_sha":  job.CommitSHA,
			"status":      job.Status,
			"phase":       job.Phase,
			"message":     job.Message.String,
			"percent":     percent,
			"files_total": job.FilesTotal,
			"files_done":  job.FilesDone,
			"nodes_total": job.NodesTotal,
			"nodes_done":  job.NodesDone,
			"edges_total": job.EdgesTotal,
			"edges_done":  job.EdgesDone,
			"updated_at":  job.UpdatedAt,
		}
		if job.Error.Valid {
			jobInfo["error"] = job.Error.String
		}
		if eta := indexETASeconds(job.Status, percent, job.StartedAt); eta != nil {
			jobInfo["eta_seconds"] = *eta
		}
		response["index_job"] = jobInfo
		response["index_phase"] = job.Phase
		response["index_message"] = job.Message.String
		response["index_percent"] = percent
		if eta := indexETASeconds(job.Status, percent, job.StartedAt); eta != nil {
			response["eta_seconds"] = *eta
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func indexPercent(status, phase string, filesTotal, filesDone, nodesTotal, nodesDone, edgesTotal, edgesDone int) int {
	if status == "ready" {
		return 100
	}
	if status == "failed" {
		return 0
	}
	switch phase {
	case "queued", "pending":
		return 2
	case "fetching_tree":
		return 5
	case "fetching_files":
		return 5 + scaledPercent(filesDone, filesTotal, 40)
	case "writing_nodes":
		return 45 + scaledPercent(nodesDone, nodesTotal, 20)
	case "building_edges":
		return 65 + scaledPercent(edgesDone, edgesTotal, 25)
	case "extracting_tests":
		return 92
	default:
		return 1
	}
}

func scaledPercent(done, total, width int) int {
	if total <= 0 {
		return 0
	}
	if done > total {
		done = total
	}
	return done * width / total
}

func indexETASeconds(status string, percent int, startedAt sql.NullTime) *int {
	if status != "running" || !startedAt.Valid || percent < 5 || percent >= 100 {
		return nil
	}
	elapsed := int(time.Since(startedAt.Time).Seconds())
	if elapsed <= 0 {
		return nil
	}
	remaining := elapsed * (100 - percent) / percent
	return &remaining
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
