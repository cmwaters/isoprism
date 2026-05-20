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

	if !hasPilotUser(ctx, h.DB, userID) {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/login?error=pilot_required"})
		return
	}

	// Ensure user row exists (created by Supabase trigger, but may be missing
	// if the trigger hasn't fired yet).
	ensureUserExists(ctx, h.DB, userID)
	cleanupExpiredRepositories(ctx, h.DB)

	// Prefer the user's explicitly selected repository.
	var fullName string
	err := h.DB.QueryRow(ctx, `
		select r.full_name
		from users u
		join repositories r on r.id = u.selected_repo_id
		where u.id = $1
		  and r.index_status = 'ready'
		  and r.is_active = true
	`, userID).Scan(&fullName)
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/" + fullName})
		return
	}

	// Fallback for older accounts that have an indexed repo but no explicit
	// selection yet.
	err = h.DB.QueryRow(ctx, `
		select full_name from repositories
		where user_id = $1 and index_status = 'ready' and is_active = true
		order by coalesce(selected_at, indexed_at, created_at) desc limit 1
	`, userID).Scan(&fullName)
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/" + fullName})
		return
	}

	// Check for any authorized repos (installation done but not indexed yet)
	var hasRepos bool
	h.DB.QueryRow(ctx, `
		select exists(select 1 from repositories where user_id = $1 and is_active = true)
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
	cleanupExpiredRepositories(ctx, h.DB)

	rows, err := h.DB.Query(ctx, `
		select r.id, r.user_id, r.installation_id, r.github_repo_id, r.full_name,
		       r.default_branch, r.main_commit_sha, r.index_status, r.is_active,
		       r.github_access_status, r.authorized_at, r.revoked_at, r.indexed_at,
		       r.selected_at, r.unused_at, r.purge_after,
		       (u.selected_repo_id = r.id) as is_selected,
		       case when p.user_id is not null then 'pilot' else coalesce(u.account_class, 'regular') end as user_class,
		       r.created_at
		from repositories r
		join users u on u.id = r.user_id
		left join pilot_users p on p.user_id = r.user_id
		where r.user_id = $1 and r.is_active = true
		order by (u.selected_repo_id = r.id) desc, r.full_name
	`, userID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	repos := make([]models.Repository, 0)
	for rows.Next() {
		var repo models.Repository
		var authorizedAt, revokedAt, indexedAt, selectedAt, unusedAt, purgeAfter sql.NullTime
		if err := rows.Scan(
			&repo.ID, &repo.UserID, &repo.InstallationID, &repo.GitHubRepoID,
			&repo.FullName, &repo.DefaultBranch, &repo.MainCommitSHA,
			&repo.IndexStatus, &repo.IsActive, &repo.GitHubAccessStatus,
			&authorizedAt, &revokedAt, &indexedAt, &selectedAt, &unusedAt, &purgeAfter,
			&repo.IsSelected, &repo.UserClass, &repo.CreatedAt,
		); err != nil {
			log.Printf("ListMyRepos: scan error: %v", err)
			continue
		}
		repo.AuthorizedAt = nullTimePtr(authorizedAt)
		repo.RevokedAt = nullTimePtr(revokedAt)
		repo.IndexedAt = nullTimePtr(indexedAt)
		repo.SelectedAt = nullTimePtr(selectedAt)
		repo.UnusedAt = nullTimePtr(unusedAt)
		repo.PurgeAfter = nullTimePtr(purgeAfter)
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
	var authorizedAt, revokedAt, indexedAt, selectedAt, unusedAt, purgeAfter sql.NullTime
	err := h.DB.QueryRow(ctx, `
		select id, user_id, installation_id, github_repo_id, full_name,
		       default_branch, main_commit_sha, index_status, is_active,
		       github_access_status, authorized_at, revoked_at, indexed_at,
		       selected_at, unused_at, purge_after, created_at
		from repositories where id = $1 and user_id = $2 and is_active = true
	`, repoID, userID).Scan(
		&repo.ID, &repo.UserID, &repo.InstallationID, &repo.GitHubRepoID,
		&repo.FullName, &repo.DefaultBranch, &repo.MainCommitSHA,
		&repo.IndexStatus, &repo.IsActive, &repo.GitHubAccessStatus,
		&authorizedAt, &revokedAt, &indexedAt, &selectedAt, &unusedAt, &purgeAfter,
		&repo.CreatedAt,
	)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}
	repo.AuthorizedAt = nullTimePtr(authorizedAt)
	repo.RevokedAt = nullTimePtr(revokedAt)
	repo.IndexedAt = nullTimePtr(indexedAt)
	repo.SelectedAt = nullTimePtr(selectedAt)
	repo.UnusedAt = nullTimePtr(unusedAt)
	repo.PurgeAfter = nullTimePtr(purgeAfter)
	repo.IsSelected = isRepoSelected(ctx, h.DB, userID, repoID)
	repo.UserClass = userClass(ctx, h.DB, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repo)
}

// POST /api/v1/repos/{repoID}/select
func (h *RepoHandler) SelectRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()
	ensureUserExists(ctx, h.DB, userID)
	cleanupExpiredRepositories(ctx, h.DB)

	var status string
	err := h.DB.QueryRow(ctx, `
		select index_status from repositories
		where id = $1 and user_id = $2 and is_active = true
	`, repoID, userID).Scan(&status)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}
	if status != "ready" {
		http.Error(w, "repo must be indexed before it can be selected", http.StatusConflict)
		return
	}

	if err := selectRepository(ctx, h.DB, userID, repoID); err != nil {
		http.Error(w, "failed to select repo", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "selected", "repo_id": repoID})
}

// DELETE /api/v1/repos/{repoID}/index
func (h *RepoHandler) UnindexRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var exists bool
	h.DB.QueryRow(ctx, `
		select exists(select 1 from repositories where id = $1 and user_id = $2 and is_active = true)
	`, repoID, userID).Scan(&exists)
	if !exists {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	if err := markRepoUnused(ctx, h.DB, userID, repoID); err != nil {
		http.Error(w, "failed to uninstall repo index", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "uninstall_scheduled", "repo_id": repoID})
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
				  and pr.base_branch = r.default_branch
				  and pr.base_commit_sha = r.main_commit_sha
			),
			count(*) filter (
				where pr.state = 'open'
				  and pr.draft = false
				  and pr.graph_status = 'ready'
				  and pr.base_branch = r.default_branch
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

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func isRepoSelected(ctx context.Context, db *pgxpool.Pool, userID, repoID string) bool {
	var selected bool
	_ = db.QueryRow(ctx, `
		select exists(select 1 from users where id = $1 and selected_repo_id = $2)
	`, userID, repoID).Scan(&selected)
	return selected
}

func userClass(ctx context.Context, db *pgxpool.Pool, userID string) string {
	if hasPilotUser(ctx, db, userID) {
		return "pilot"
	}
	var class string
	_ = db.QueryRow(ctx, `select coalesce(account_class, 'regular') from users where id = $1`, userID).Scan(&class)
	if class == "" {
		return "regular"
	}
	return class
}

func hasPilotUser(ctx context.Context, db *pgxpool.Pool, userID string) bool {
	var isPilot bool
	_ = db.QueryRow(ctx, `select exists(select 1 from pilot_users where user_id = $1)`, userID).Scan(&isPilot)
	return isPilot
}

func HasPilotUser(ctx context.Context, db *pgxpool.Pool, userID string) bool {
	return hasPilotUser(ctx, db, userID)
}

func selectRepository(ctx context.Context, db *pgxpool.Pool, userID, repoID string) error {
	var previousRepoID sql.NullString
	_ = db.QueryRow(ctx, `select selected_repo_id from users where id = $1`, userID).Scan(&previousRepoID)

	_, err := db.Exec(ctx, `
		update users set selected_repo_id = $1 where id = $2
	`, repoID, userID)
	if err != nil {
		return err
	}

	_, _ = db.Exec(ctx, `
		update repositories
		set selected_at = case when id = $1 then now() else selected_at end,
		    unused_at = case when id = $1 then null else unused_at end,
		    purge_after = case when id = $1 then null else purge_after end
		where user_id = $2
	`, repoID, userID)

	_, _ = db.Exec(ctx, `
		update pilot_users
		set selected_repo_id = $1,
			trial_starts_at = coalesce(trial_starts_at, now()),
			trial_ends_at = coalesce(trial_ends_at, now() + interval '7 days'),
			status = case when status in ('registered', 'invited') then 'active' else status end
		where user_id = $2
	`, repoID, userID)

	if previousRepoID.Valid && previousRepoID.String != repoID && userClass(ctx, db, userID) == "pilot" {
		_ = scheduleUnusedRepoCleanup(ctx, db, userID, previousRepoID.String)
	}

	return nil
}

func markRepoUnused(ctx context.Context, db *pgxpool.Pool, userID, repoID string) error {
	if err := scheduleUnusedRepoCleanup(ctx, db, userID, repoID); err != nil {
		return err
	}
	_, _ = db.Exec(ctx, `update users set selected_repo_id = null where id = $1 and selected_repo_id = $2`, userID, repoID)
	_, _ = db.Exec(ctx, `update pilot_users set selected_repo_id = null where user_id = $1 and selected_repo_id = $2`, userID, repoID)
	return nil
}

func scheduleUnusedRepoCleanup(ctx context.Context, db *pgxpool.Pool, userID, repoID string) error {
	_, err := db.Exec(ctx, `
		update repositories
		set unused_at = coalesce(unused_at, now()),
		    purge_after = coalesce(purge_after, now() + interval '1 day'),
		    selected_at = null
		where id = $1 and user_id = $2 and is_active = true
	`, repoID, userID)
	return err
}

func cleanupExpiredRepositories(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, `
		delete from repositories
		where is_active = false
		  and purge_after is not null
		  and purge_after <= now()
	`)
	rows, err := db.Query(ctx, `
		select id from repositories
		where is_active = true
		  and unused_at is not null
		  and purge_after is not null
		  and purge_after <= now()
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var repoID string
		if err := rows.Scan(&repoID); err != nil {
			continue
		}
		_, _ = db.Exec(ctx, `delete from pull_requests where repo_id = $1`, repoID)
		_, _ = db.Exec(ctx, `delete from code_edges where repo_id = $1`, repoID)
		_, _ = db.Exec(ctx, `delete from code_nodes where repo_id = $1`, repoID)
		_, _ = db.Exec(ctx, `delete from indexing_jobs where repo_id = $1`, repoID)
		_, _ = db.Exec(ctx, `
			update repositories
			set main_commit_sha = null,
			    index_status = 'pending',
			    indexed_at = null,
			    unused_at = null,
			    purge_after = null
			where id = $1
		`, repoID)
	}
}

func CleanupExpiredRepositories(ctx context.Context, db *pgxpool.Pool) {
	cleanupExpiredRepositories(ctx, db)
}

func SelectRepository(ctx context.Context, db *pgxpool.Pool, userID, repoID string) error {
	return selectRepository(ctx, db, userID, repoID)
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
		insert into users (id, email, account_class) values ($1, '', 'regular') on conflict (id) do nothing
	`, userID)
}

func EnsureUserExists(ctx context.Context, db *pgxpool.Pool, userID string) {
	ensureUserExists(ctx, db, userID)
}
