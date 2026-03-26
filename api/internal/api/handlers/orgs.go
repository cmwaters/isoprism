package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/aperture/api/internal/github"
	"github.com/aperture/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrgHandler struct {
	DB        *pgxpool.Pool
	AppClient *github.AppClient
}

// GET /api/v1/me/orgs
func (h *OrgHandler) ListMyOrgs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Header.Get("X-User-ID")

	rows, err := h.DB.Query(ctx, `
		select o.id, o.name, o.slug, o.github_account_login, o.github_account_type,
		       o.github_account_id, o.avatar_url, o.created_at
		from organizations o
		join org_members om on om.org_id = o.id
		where om.user_id = $1
		order by o.created_at asc
	`, userID)
	if err != nil {
		http.Error(w, "failed to fetch orgs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orgs []models.Organization
	for rows.Next() {
		var o models.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.GitHubAccountLogin, &o.GitHubAccountType,
			&o.GitHubAccountID, &o.AvatarURL, &o.CreatedAt); err == nil {
			orgs = append(orgs, o)
		}
	}
	if orgs == nil {
		orgs = []models.Organization{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"orgs": orgs})
}

// GET /api/v1/orgs/{orgSlug}
func (h *OrgHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := chi.URLParam(r, "orgSlug")

	var org models.Organization
	err := h.DB.QueryRow(ctx, `
		select id, name, slug, github_account_login, github_account_type,
		       github_account_id, avatar_url, created_at
		from organizations where slug = $1
	`, orgSlug).Scan(&org.ID, &org.Name, &org.Slug, &org.GitHubAccountLogin, &org.GitHubAccountType,
		&org.GitHubAccountID, &org.AvatarURL, &org.CreatedAt)
	if err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// GET /api/v1/orgs/{orgSlug}/repos
func (h *OrgHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := chi.URLParam(r, "orgSlug")

	var orgID string
	if err := h.DB.QueryRow(ctx, `select id from organizations where slug = $1`, orgSlug).Scan(&orgID); err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	rows, err := h.DB.Query(ctx, `
		select id, org_id, installation_id, github_repo_id, full_name, default_branch, is_active, created_at
		from repositories
		where org_id = $1
		order by full_name asc
	`, orgID)
	if err != nil {
		http.Error(w, "failed to fetch repos", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repos []models.Repository
	for rows.Next() {
		var repo models.Repository
		if err := rows.Scan(&repo.ID, &repo.OrgID, &repo.InstallationID, &repo.GitHubRepoID,
			&repo.FullName, &repo.DefaultBranch, &repo.IsActive, &repo.CreatedAt); err == nil {
			repos = append(repos, repo)
		}
	}
	if repos == nil {
		repos = []models.Repository{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"repos": repos})
}

// PATCH /api/v1/orgs/{orgSlug}/repos/{repoID}
func (h *OrgHandler) UpdateRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repoID := chi.URLParam(r, "repoID")

	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	_, err := h.DB.Exec(ctx, `update repositories set is_active = $1 where id = $2`, body.IsActive, repoID)
	if err != nil {
		http.Error(w, "failed to update repo", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/orgs/{orgSlug}/repos/{repoID}
func (h *OrgHandler) DeleteRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repoID := chi.URLParam(r, "repoID")

	_, err := h.DB.Exec(ctx, `delete from repositories where id = $1`, repoID)
	if err != nil {
		http.Error(w, "failed to delete repo", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/orgs/{orgSlug}/prs/{prID}
func (h *OrgHandler) GetPR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prID := chi.URLParam(r, "prID")

	var pr models.PullRequest
	var analysis models.PRAnalysis
	var repoFullName string

	err := h.DB.QueryRow(ctx, `
		select
			pr.id, pr.org_id, pr.repo_id, pr.github_pr_id, pr.number, pr.title,
			pr.body, pr.author_github_login, pr.author_avatar_url,
			pr.base_branch, pr.head_branch, pr.state, pr.draft,
			pr.additions, pr.deletions, pr.changed_files, pr.html_url,
			pr.opened_at, pr.closed_at, pr.merged_at, pr.last_activity_at,
			pr.last_synced_at, pr.created_at, pr.updated_at,
			r.full_name as repo_full_name,
			pa.id, pa.pull_request_id, pa.commit_sha,
			pa.summary, pa.why, pa.impacted_areas, pa.key_files,
			pa.size_label, pa.risk_score, pa.risk_label, pa.risk_reasons
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		left join pr_analyses pa on pa.pull_request_id = pr.id
		where pr.id = $1
	`, prID).Scan(
		&pr.ID, &pr.OrgID, &pr.RepoID, &pr.GitHubPRID, &pr.Number, &pr.Title,
		&pr.Body, &pr.AuthorGitHubLogin, &pr.AuthorAvatarURL,
		&pr.BaseBranch, &pr.HeadBranch, &pr.State, &pr.Draft,
		&pr.Additions, &pr.Deletions, &pr.ChangedFiles, &pr.HTMLURL,
		&pr.OpenedAt, &pr.ClosedAt, &pr.MergedAt, &pr.LastActivityAt,
		&pr.LastSyncedAt, &pr.CreatedAt, &pr.UpdatedAt,
		&repoFullName,
		&analysis.ID, &analysis.PullRequestID, &analysis.CommitSHA,
		&analysis.Summary, &analysis.Why, &analysis.ImpactedAreas, &analysis.KeyFiles,
		&analysis.SizeLabel, &analysis.RiskScore, &analysis.RiskLabel, &analysis.RiskReasons,
	)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}
	pr.RepoFullName = repoFullName
	if analysis.Summary != nil {
		pr.Analysis = &analysis
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pr)
}

// DELETE /api/v1/me
func (h *OrgHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Header.Get("X-User-ID")

	// Delete orgs where this user is the sole admin (personal orgs)
	_, _ = h.DB.Exec(ctx, `
		delete from organizations
		where id in (
			select org_id from org_members where user_id = $1 and role = 'org_admin'
		)
		and (select count(*) from org_members om2 where om2.org_id = organizations.id and om2.role = 'org_admin') = 1
	`, userID)

	// Delete the user (cascades to org_members, team_members)
	_, err := h.DB.Exec(ctx, `delete from users where id = $1`, userID)
	if err != nil {
		http.Error(w, "failed to delete account", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/orgs/{orgSlug}/teams
func (h *OrgHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := chi.URLParam(r, "orgSlug")

	var orgID string
	if err := h.DB.QueryRow(ctx, `select id from organizations where slug = $1`, orgSlug).Scan(&orgID); err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	rows, err := h.DB.Query(ctx, `
		select id, org_id, name, slug, github_team_id, created_at
		from teams where org_id = $1 order by name asc
	`, orgID)
	if err != nil {
		http.Error(w, "failed to fetch teams", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.GitHubTeamID, &t.CreatedAt); err == nil {
			teams = append(teams, t)
		}
	}
	if teams == nil {
		teams = []models.Team{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"teams": teams})
}

// GET /api/v1/auth/status?github_token=...
// Checks if the user's GitHub account matches any connected org.
// Returns a redirect destination.
func (h *OrgHandler) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	githubToken := r.URL.Query().Get("github_token")
	userID := r.URL.Query().Get("user_id")

	if githubToken == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding"})
		return
	}

	ghClient := github.NewClient(githubToken)

	// Get the user's own login
	ghUser, err := ghClient.GetAuthenticatedUser(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding"})
		return
	}

	// Get user's org memberships
	userOrgs, _ := ghClient.ListUserOrgs(ctx)

	// Build list of logins to check: personal login + all org logins
	logins := []string{ghUser.Login}
	for _, org := range userOrgs {
		logins = append(logins, org.Login)
	}

	// Check against connected orgs in our DB
	for _, login := range logins {
		var orgSlug string
		err := h.DB.QueryRow(ctx, `
			select slug from organizations where github_account_login = $1
		`, login).Scan(&orgSlug)
		if err != nil {
			continue
		}

		// Org exists — check if user is already a member
		if userID != "" {
			var isMember bool
			h.DB.QueryRow(ctx, `
				select exists(select 1 from org_members where org_id = (select id from organizations where slug = $1) and user_id = $2)
			`, orgSlug, userID).Scan(&isMember)

			if isMember {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"redirect": "/orgs/" + orgSlug})
				return
			}

			// Also check by github_user_id (user may have been pre-seeded from org sync)
			var isSeededMember bool
			h.DB.QueryRow(ctx, `
				select exists(
					select 1 from org_members om
					join users u on u.id = om.user_id
					where om.org_id = (select id from organizations where slug = $1)
					  and u.github_user_id = $2
				)
			`, orgSlug, ghUser.ID).Scan(&isSeededMember)

			if isSeededMember {
				// Link the seeded user to the actual auth user and redirect
				h.DB.Exec(ctx, `
					update org_members set user_id = $1
					where user_id = (
						select id from users where github_user_id = $2 and id != $1
					)
					and org_id = (select id from organizations where slug = $3)
				`, userID, ghUser.ID, orgSlug)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"redirect": "/orgs/" + orgSlug})
				return
			}
		}

		// Org exists but user is not a member — send to join request
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding/join?org=" + orgSlug})
		return
	}

	// No matching org found
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"redirect": "/onboarding"})
}

// POST /api/v1/orgs/{orgSlug}/join-requests
func (h *OrgHandler) CreateJoinRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := chi.URLParam(r, "orgSlug")
	userID := r.Header.Get("X-User-ID")

	var orgID string
	if err := h.DB.QueryRow(ctx, `select id from organizations where slug = $1`, orgSlug).Scan(&orgID); err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	var req models.OrgJoinRequest
	err := h.DB.QueryRow(ctx, `
		insert into org_join_requests (org_id, user_id)
		values ($1, $2)
		on conflict (org_id, user_id) do update set status = 'pending'
		returning id, org_id, user_id, status, created_at
	`, orgID, userID).Scan(&req.ID, &req.OrgID, &req.UserID, &req.Status, &req.CreatedAt)
	if err != nil {
		http.Error(w, "failed to create join request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

// GET /api/v1/orgs/{orgSlug}/join-requests (org_admin only)
func (h *OrgHandler) ListJoinRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := chi.URLParam(r, "orgSlug")

	rows, err := h.DB.Query(ctx, `
		select jr.id, jr.org_id, jr.user_id, jr.status, jr.created_at, jr.resolved_at, jr.resolved_by,
		       u.github_username, u.avatar_url
		from org_join_requests jr
		join users u on u.id = jr.user_id
		join organizations o on o.id = jr.org_id
		where o.slug = $1 and jr.status = 'pending'
		order by jr.created_at asc
	`, orgSlug)
	if err != nil {
		http.Error(w, "failed to fetch join requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type JoinRequestWithUser struct {
		models.OrgJoinRequest
		GitHubUsername *string `json:"github_username"`
		AvatarURL      *string `json:"avatar_url"`
	}
	var requests []JoinRequestWithUser
	for rows.Next() {
		var req JoinRequestWithUser
		if err := rows.Scan(&req.ID, &req.OrgID, &req.UserID, &req.Status, &req.CreatedAt,
			&req.ResolvedAt, &req.ResolvedBy, &req.GitHubUsername, &req.AvatarURL); err == nil {
			requests = append(requests, req)
		}
	}
	if requests == nil {
		requests = []JoinRequestWithUser{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"requests": requests})
}

// PATCH /api/v1/orgs/{orgSlug}/join-requests/{requestID} (org_admin only)
func (h *OrgHandler) UpdateJoinRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi.URLParam(r, "requestID")
	adminUserID := r.Header.Get("X-User-ID")

	var body struct {
		Status string `json:"status"` // "approved" or "rejected"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Status != "approved" && body.Status != "rejected") {
		http.Error(w, "status must be 'approved' or 'rejected'", http.StatusBadRequest)
		return
	}

	// Fetch the join request
	var orgID, userID string
	err := h.DB.QueryRow(ctx, `
		select org_id, user_id from org_join_requests where id = $1
	`, requestID).Scan(&orgID, &userID)
	if err != nil {
		http.Error(w, "join request not found", http.StatusNotFound)
		return
	}

	// Update status
	_, err = h.DB.Exec(ctx, `
		update org_join_requests set status = $1, resolved_at = now(), resolved_by = $2
		where id = $3
	`, body.Status, adminUserID, requestID)
	if err != nil {
		http.Error(w, "failed to update join request", http.StatusInternalServerError)
		return
	}

	// If approved: add to org_members and match to any pre-seeded team memberships
	if body.Status == "approved" {
		_, _ = h.DB.Exec(ctx, `
			insert into org_members (org_id, user_id, role)
			values ($1, $2, 'member')
			on conflict (org_id, user_id) do nothing
		`, orgID, userID)

		// Transfer any pre-seeded team memberships (seeded by GitHub username before user signed up)
		_, _ = h.DB.Exec(ctx, `
			update team_members set user_id = $1
			where user_id = (
				select id from users where github_user_id = (
					select github_user_id from users where id = $1
				) and id != $1
			)
			and team_id in (
				select id from teams where org_id = $2
			)
		`, userID, orgID)
	}

	w.WriteHeader(http.StatusNoContent)
}
