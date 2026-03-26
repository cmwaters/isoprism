package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aperture/api/internal/github"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GitHubHandler struct {
	DB            *pgxpool.Pool
	AppClient     *github.AppClient
	WebhookSecret string
	FrontendURL   string
}

// POST /webhooks/github
func (h *GitHubHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := github.ReadAndVerify(r, h.WebhookSecret)
	if err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	log.Printf("webhook received: event=%q", event)
	switch event {
	case "pull_request":
		h.handlePREvent(r.Context(), body)
	case "installation":
		h.handleInstallationEvent(r.Context(), body)
	case "ping":
		// acknowledge
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *GitHubHandler) handlePREvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRPayload(body)
	if err != nil {
		log.Printf("error parsing PR payload: %v", err)
		return
	}

	log.Printf("PR event: action=%q repo=%q pr=#%d", payload.Action, payload.Repository.FullName, payload.PullRequest.Number)

	switch payload.Action {
	case "opened", "synchronize", "reopened", "ready_for_review", "closed":
	default:
		return
	}

	// Look up org + repo for this installation
	var orgID, repoID string
	err = h.DB.QueryRow(ctx, `
		select r.org_id, r.id
		from repositories r
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1
		  and r.github_repo_id = $2
		  and r.is_active = true
	`, payload.Installation.ID, payload.Repository.ID).Scan(&orgID, &repoID)
	if err != nil {
		log.Printf("PR event: no matching active repo found for installation=%d repo_id=%d: %v", payload.Installation.ID, payload.Repository.ID, err)
		return
	}

	pr := payload.PullRequest
	state := pr.State
	if pr.MergedAt != nil {
		state = "merged"
	}

	now := time.Now()
	_, err = h.DB.Exec(ctx, `
		insert into pull_requests (
			org_id, repo_id, github_pr_id, number, title, body,
			author_github_login, author_avatar_url, base_branch, head_branch,
			state, draft, additions, deletions, changed_files, html_url,
			opened_at, closed_at, merged_at, last_activity_at, last_synced_at,
			updated_at
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		on conflict (repo_id, github_pr_id) do update set
			title = excluded.title,
			body = excluded.body,
			state = excluded.state,
			draft = excluded.draft,
			additions = excluded.additions,
			deletions = excluded.deletions,
			changed_files = excluded.changed_files,
			closed_at = excluded.closed_at,
			merged_at = excluded.merged_at,
			last_activity_at = excluded.last_activity_at,
			last_synced_at = excluded.last_synced_at,
			updated_at = excluded.updated_at
	`,
		orgID, repoID, pr.ID, pr.Number, pr.Title, pr.Body,
		pr.User.Login, pr.User.AvatarURL, pr.Base.Ref, pr.Head.Ref,
		state, pr.Draft, pr.Additions, pr.Deletions, pr.ChangedFiles, pr.HTMLURL,
		pr.CreatedAt, pr.ClosedAt, pr.MergedAt, pr.UpdatedAt, now, now,
	)
	if err != nil {
		log.Printf("error upserting PR %d: %v", pr.Number, err)
	}
}

func (h *GitHubHandler) handleInstallationEvent(ctx context.Context, body []byte) {
	payload, err := github.ParseInstallationPayload(body)
	if err != nil {
		log.Printf("error parsing installation payload: %v", err)
		return
	}
	if payload.Action == "deleted" {
		_, _ = h.DB.Exec(ctx,
			`delete from github_installations where installation_id = $1`,
			payload.Installation.ID,
		)
	}
}

// GET /api/v1/github/callback
// Called after GitHub App installation. Creates or updates the org and redirects to repo selection.
func (h *GitHubHandler) HandleInstallationCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	installationIDStr := r.URL.Query().Get("installation_id")
	userID := r.URL.Query().Get("state")
	setupAction := r.URL.Query().Get("setup_action") // "install" or "update"

	if installationIDStr == "" {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}

	var installationID int64
	fmt.Sscanf(installationIDStr, "%d", &installationID)

	// Fetch installation details (account login, type, etc.) using App JWT
	installation, err := h.AppClient.GetInstallation(ctx, installationID)
	if err != nil {
		log.Printf("failed to fetch installation details: %v", err)
		http.Error(w, fmt.Sprintf("failed to fetch installation details: %v", err), http.StatusInternalServerError)
		return
	}

	accountLogin := installation.Account.Login
	accountType := installation.Account.Type
	accountID := installation.Account.ID
	avatarURL := installation.Account.AvatarURL

	// Get installation token for API calls
	token, err := h.AppClient.InstallationToken(ctx, installationID)
	if err != nil {
		http.Error(w, "failed to get installation token", http.StatusInternalServerError)
		return
	}
	ghClient := github.NewClient(token)

	var orgID string
	var orgSlug string

	if setupAction == "update" {
		// On update: look up the existing org by installation_id and re-sync repos.
		// If not found (e.g. DB was reset but app is still installed on GitHub), fall through
		// to the install path below to recreate everything from scratch.
		err = h.DB.QueryRow(ctx, `
			select o.id, o.slug
			from organizations o
			join github_installations gi on gi.org_id = o.id
			where gi.installation_id = $1
		`, installationID).Scan(&orgID, &orgSlug)
		if err == nil {
			// Found — ensure the user is a member and redirect to queue
			if userID != "" {
				ensureUserExists(ctx, h.DB, userID)
				_, _ = h.DB.Exec(ctx, `
					insert into org_members (org_id, user_id, role)
					values ($1, $2, 'org_admin')
					on conflict (org_id, user_id) do nothing
				`, orgID, userID)
			}
			h.syncRepos(ctx, ghClient, orgID, installationIDStr)
			http.Redirect(w, r, h.FrontendURL+"/orgs/"+orgSlug, http.StatusFound)
			return
		}
		// Not found — fall through to create org/installation from scratch
	}

	// New installation: create org
	orgSlug = accountLogin // GitHub logins are already URL-safe slugs
	err = h.DB.QueryRow(ctx, `
		insert into organizations (name, slug, github_account_login, github_account_type, github_account_id, avatar_url)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (slug) do update set
			avatar_url = excluded.avatar_url,
			github_account_id = excluded.github_account_id
		returning id
	`, accountLogin, orgSlug, accountLogin, accountType, accountID, avatarURL).Scan(&orgID)
	if err != nil {
		log.Printf("failed to upsert org: %v", err)
		http.Error(w, "failed to create org", http.StatusInternalServerError)
		return
	}

	// Add installing user as org admin (if userID provided)
	if userID != "" {
		ensureUserExists(ctx, h.DB, userID)
		_, _ = h.DB.Exec(ctx, `
			insert into org_members (org_id, user_id, role)
			values ($1, $2, 'org_admin')
			on conflict (org_id, user_id) do update set role = 'org_admin'
		`, orgID, userID)
	}

	// Create default org preferences
	_, _ = h.DB.Exec(ctx,
		`insert into org_preferences (org_id) values ($1) on conflict do nothing`,
		orgID,
	)

	// Store installation
	var dbInstallationID string
	err = h.DB.QueryRow(ctx, `
		insert into github_installations (org_id, installation_id, account_login, account_type, account_avatar_url)
		values ($1, $2, $3, $4, $5)
		on conflict (installation_id) do update set
			account_login = excluded.account_login,
			account_avatar_url = excluded.account_avatar_url
		returning id
	`, orgID, installationID, accountLogin, accountType, avatarURL).Scan(&dbInstallationID)
	if err != nil {
		log.Printf("failed to upsert installation: %v", err)
		http.Error(w, "failed to save installation", http.StatusInternalServerError)
		return
	}

	// Sync repos
	h.syncReposWithInstallationID(ctx, ghClient, orgID, dbInstallationID)

	// If GitHub org: sync teams and members
	if accountType == "Organization" {
		h.syncOrgTeams(ctx, ghClient, orgID, accountLogin)
	}

	http.Redirect(w, r, h.FrontendURL+"/onboarding/repos?org="+orgSlug, http.StatusFound)
}

func (h *GitHubHandler) syncRepos(ctx context.Context, ghClient *github.Client, orgID, installationIDStr string) {
	var dbInstallationID string
	err := h.DB.QueryRow(ctx, `
		select id from github_installations where org_id = $1
	`, orgID).Scan(&dbInstallationID)
	if err != nil {
		return
	}
	h.syncReposWithInstallationID(ctx, ghClient, orgID, dbInstallationID)
}

func (h *GitHubHandler) syncReposWithInstallationID(ctx context.Context, ghClient *github.Client, orgID, dbInstallationID string) {
	repos, err := ghClient.ListInstallationRepos(ctx)
	if err != nil {
		log.Printf("failed to list repos: %v", err)
		return
	}
	for _, repo := range repos {
		_, err := h.DB.Exec(ctx, `
			insert into repositories (org_id, installation_id, github_repo_id, full_name, default_branch)
			values ($1, $2, $3, $4, $5)
			on conflict (org_id, github_repo_id) do update set
				full_name = excluded.full_name,
				default_branch = excluded.default_branch
		`, orgID, dbInstallationID, repo.ID, repo.FullName, repo.DefaultBranch)
		if err != nil {
			log.Printf("error upserting repo %s: %v", repo.FullName, err)
		}
	}
}

func (h *GitHubHandler) syncOrgTeams(ctx context.Context, ghClient *github.Client, orgID, orgLogin string) {
	ghTeams, err := ghClient.ListOrgTeams(ctx, orgLogin)
	if err != nil {
		log.Printf("failed to list org teams for %s: %v", orgLogin, err)
		return
	}

	for _, ghTeam := range ghTeams {
		// Create team
		var teamID string
		err := h.DB.QueryRow(ctx, `
			insert into teams (org_id, name, slug, github_team_id)
			values ($1, $2, $3, $4)
			on conflict (org_id, slug) do update set
				name = excluded.name,
				github_team_id = excluded.github_team_id
			returning id
		`, orgID, ghTeam.Name, ghTeam.Slug, ghTeam.ID).Scan(&teamID)
		if err != nil {
			log.Printf("error upserting team %s: %v", ghTeam.Slug, err)
			continue
		}

		// Sync team members
		members, err := ghClient.ListOrgTeamMembers(ctx, orgLogin, ghTeam.Slug)
		if err != nil {
			log.Printf("error fetching members for team %s: %v", ghTeam.Slug, err)
			continue
		}

		for _, member := range members {
			// Upsert user by github_user_id (they may not have signed in yet)
			var memberUserID string
			err := h.DB.QueryRow(ctx, `
				insert into users (id, email, github_user_id, github_username)
				values (gen_random_uuid(), $1, $2, $3)
				on conflict (github_user_id) do update set
					github_username = excluded.github_username
				returning id
			`, member.Login+"@github", member.ID, member.Login).Scan(&memberUserID)
			if err != nil {
				log.Printf("error upserting user %s: %v", member.Login, err)
				continue
			}

			// Add to org as member
			_, _ = h.DB.Exec(ctx, `
				insert into org_members (org_id, user_id, role)
				values ($1, $2, 'member')
				on conflict (org_id, user_id) do nothing
			`, orgID, memberUserID)

			// Add to team
			_, _ = h.DB.Exec(ctx, `
				insert into team_members (team_id, user_id, role)
				values ($1, $2, 'member')
				on conflict (team_id, user_id) do nothing
			`, teamID, memberUserID)
		}
	}
}

// POST /api/v1/orgs/{orgSlug}/repos/{repoID}/sync
func (h *GitHubHandler) SyncRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repoID := r.PathValue("repoID")

	var installationID int64
	var fullName string
	var orgID string
	err := h.DB.QueryRow(ctx, `
		select gi.installation_id, r.full_name, r.org_id
		from repositories r
		join github_installations gi on gi.id = r.installation_id
		where r.id = $1 and r.is_active = true
	`, repoID).Scan(&installationID, &fullName, &orgID)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		http.Error(w, "failed to authenticate with GitHub", http.StatusInternalServerError)
		return
	}

	parts := splitRepoName(fullName)
	if parts == nil {
		http.Error(w, "invalid repo name", http.StatusInternalServerError)
		return
	}

	prs, err := ghClient.ListOpenPullRequests(ctx, parts[0], parts[1])
	if err != nil {
		http.Error(w, "failed to fetch PRs from GitHub", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	for _, pr := range prs {
		state := pr.State
		if pr.MergedAt != nil {
			state = "merged"
		}
		_, err := h.DB.Exec(ctx, `
			insert into pull_requests (
				org_id, repo_id, github_pr_id, number, title, body,
				author_github_login, author_avatar_url, base_branch, head_branch,
				state, draft, additions, deletions, changed_files, html_url,
				opened_at, closed_at, merged_at, last_activity_at, last_synced_at,
				updated_at
			) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
			on conflict (repo_id, github_pr_id) do update set
				title = excluded.title,
				state = excluded.state,
				draft = excluded.draft,
				additions = excluded.additions,
				deletions = excluded.deletions,
				changed_files = excluded.changed_files,
				last_synced_at = excluded.last_synced_at,
				updated_at = excluded.updated_at
		`,
			orgID, repoID, pr.ID, pr.Number, pr.Title, pr.Body,
			pr.User.Login, pr.User.AvatarURL, pr.Base.Ref, pr.Head.Ref,
			state, pr.Draft, pr.Additions, pr.Deletions, pr.ChangedFiles, pr.HTMLURL,
			pr.CreatedAt, pr.ClosedAt, pr.MergedAt, pr.UpdatedAt, now, now,
		)
		if err != nil {
			log.Printf("error upserting PR #%d: %v", pr.Number, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"synced": len(prs)})
}

// ensureUserExists syncs the user from auth.users into public.users if they don't exist yet.
// The auth trigger normally handles this, but it only fires on INSERT/UPDATE to auth.users —
// not when a user operates with an existing session — so the public.users row may be missing
// (e.g. after a DB reset) when the GitHub App callback runs.
func ensureUserExists(ctx context.Context, db *pgxpool.Pool, userID string) {
	_, _ = db.Exec(ctx, `
		insert into public.users (id, email, display_name, avatar_url, github_user_id, github_username)
		select
			au.id,
			au.email,
			coalesce(au.raw_user_meta_data->>'full_name', au.raw_user_meta_data->>'name'),
			au.raw_user_meta_data->>'avatar_url',
			(au.raw_user_meta_data->>'provider_id')::bigint,
			au.raw_user_meta_data->>'user_name'
		from auth.users au
		where au.id = $1
		on conflict (id) do nothing
	`, userID)
}

func splitRepoName(fullName string) []string {
	for i, c := range fullName {
		if c == '/' {
			return []string{fullName[:i], fullName[i+1:]}
		}
	}
	return nil
}
