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
	case "pull_request_review":
		h.handlePRReviewEvent(r.Context(), body)
	case "pull_request_review_comment":
		h.handlePRReviewCommentEvent(r.Context(), body)
	case "pull_request_review_thread":
		h.handlePRReviewThreadEvent(r.Context(), body)
	case "check_suite":
		h.handleCheckSuiteEvent(r.Context(), body)
	case "installation":
		h.handleInstallationEvent(r.Context(), body)
	case "installation_repositories":
		h.handleInstallationReposEvent(r.Context(), body)
	case "ping":
		// acknowledge
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- pull_request ----

func (h *GitHubHandler) handlePREvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRPayload(body)
	if err != nil {
		log.Printf("error parsing PR payload: %v", err)
		return
	}

	log.Printf("PR event: action=%q repo=%q pr=#%d", payload.Action, payload.Repository.FullName, payload.PullRequest.Number)

	switch payload.Action {
	case "opened", "synchronize", "reopened", "ready_for_review", "closed",
		"review_requested", "review_request_removed":
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
		log.Printf("PR event: no matching active repo found for installation=%d repo_id=%d: %v",
			payload.Installation.ID, payload.Repository.ID, err)
		return
	}

	pr := payload.PullRequest
	state := pr.State
	if pr.MergedAt != nil {
		state = "merged"
	}

	now := time.Now()
	headSHA := pr.Head.SHA

	// Upsert the PR, returning its DB id for subsequent signal updates.
	var prDBID string
	err = h.DB.QueryRow(ctx, `
		insert into pull_requests (
			org_id, repo_id, github_pr_id, number, title, body,
			author_github_login, author_avatar_url, base_branch, head_branch,
			state, draft, additions, deletions, changed_files, html_url,
			opened_at, closed_at, merged_at, last_activity_at, last_synced_at,
			updated_at, head_sha, mergeable
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		on conflict (repo_id, github_pr_id) do update set
			title            = excluded.title,
			body             = excluded.body,
			state            = excluded.state,
			draft            = excluded.draft,
			additions        = excluded.additions,
			deletions        = excluded.deletions,
			changed_files    = excluded.changed_files,
			closed_at        = excluded.closed_at,
			merged_at        = excluded.merged_at,
			last_activity_at = excluded.last_activity_at,
			last_synced_at   = excluded.last_synced_at,
			updated_at       = excluded.updated_at,
			head_sha         = excluded.head_sha,
			mergeable        = excluded.mergeable
		returning id
	`,
		orgID, repoID, pr.ID, pr.Number, pr.Title, pr.Body,
		pr.User.Login, pr.User.AvatarURL, pr.Base.Ref, pr.Head.Ref,
		state, pr.Draft, pr.Additions, pr.Deletions, pr.ChangedFiles, pr.HTMLURL,
		pr.CreatedAt, pr.ClosedAt, pr.MergedAt, pr.UpdatedAt, now, now,
		headSHA, pr.Mergeable,
	).Scan(&prDBID)
	if err != nil {
		log.Printf("error upserting PR #%d: %v", pr.Number, err)
		return
	}

	// Sync explicit review requests.
	h.syncReviewRequests(ctx, prDBID, pr.RequestedReviewers)

	// On synchronize/reopen: GitHub recomputes mergeability async — fetch it after a short delay.
	if payload.Action == "synchronize" || payload.Action == "reopened" || payload.Action == "opened" {
		go h.fetchMergeability(payload.Installation.ID, payload.Repository.FullName, pr.Number, repoID, pr.ID)
	}
}

// syncReviewRequests replaces the review-request list for a PR.
func (h *GitHubHandler) syncReviewRequests(ctx context.Context, prDBID string, requestedReviewers []struct{ Login string `json:"login"` }) {
	_, _ = h.DB.Exec(ctx, `delete from pr_review_requests where pull_request_id = $1`, prDBID)
	for _, rv := range requestedReviewers {
		_, _ = h.DB.Exec(ctx, `
			insert into pr_review_requests (pull_request_id, reviewer_login)
			values ($1, $2)
			on conflict do nothing
		`, prDBID, rv.Login)
	}
}

// fetchMergeability waits 30 s then polls GitHub for the computed mergeable state.
func (h *GitHubHandler) fetchMergeability(installationID int64, fullName string, prNumber int, repoID string, githubPRID int64) {
	time.Sleep(30 * time.Second)
	ctx := context.Background()

	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		return
	}
	parts := splitRepoName(fullName)
	if parts == nil {
		return
	}
	pr, err := ghClient.GetPullRequest(ctx, parts[0], parts[1], prNumber)
	if err != nil || pr.Mergeable == nil {
		return
	}
	_, _ = h.DB.Exec(ctx,
		`update pull_requests set mergeable = $1 where repo_id = $2 and github_pr_id = $3`,
		pr.Mergeable, repoID, githubPRID,
	)
}

// ---- pull_request_review ----

func (h *GitHubHandler) handlePRReviewEvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRReviewPayload(body)
	if err != nil {
		log.Printf("error parsing PR review payload: %v", err)
		return
	}
	if payload.Action != "submitted" {
		return
	}

	review := payload.Review
	log.Printf("PR review event: reviewer=%q state=%q repo_id=%d pr=#%d",
		review.User.Login, review.State, payload.Repository.ID, payload.PullRequest.Number)

	// Find the DB pull_request id.
	var prDBID string
	err = h.DB.QueryRow(ctx, `
		select pr.id from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1
		  and r.github_repo_id   = $2
		  and pr.number          = $3
	`, payload.Installation.ID, payload.Repository.ID, payload.PullRequest.Number).Scan(&prDBID)
	if err != nil {
		log.Printf("PR review event: PR not found: %v", err)
		return
	}

	submittedAt := time.Now()
	if review.SubmittedAt != nil {
		submittedAt = *review.SubmittedAt
	}
	commitSHA := review.CommitID

	_, err = h.DB.Exec(ctx, `
		insert into pr_reviews (pull_request_id, github_review_id, reviewer_login, reviewer_avatar_url, state, submitted_at, commit_sha)
		values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (github_review_id) do update set
			state        = excluded.state,
			submitted_at = excluded.submitted_at,
			commit_sha   = excluded.commit_sha
	`, prDBID, review.ID, review.User.Login, review.User.AvatarURL, review.State, submittedAt, commitSHA)
	if err != nil {
		log.Printf("PR review event: error upserting review: %v", err)
	}

	// Reviewer has submitted — remove them from pending review requests.
	_, _ = h.DB.Exec(ctx,
		`delete from pr_review_requests where pull_request_id = $1 and reviewer_login = $2`,
		prDBID, review.User.Login,
	)
}

// ---- pull_request_review_comment (inline thread comments) ----

func (h *GitHubHandler) handlePRReviewCommentEvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRReviewCommentPayload(body)
	if err != nil {
		log.Printf("error parsing PR review comment payload: %v", err)
		return
	}
	if payload.Action != "created" {
		return
	}

	// Thread ID = root comment id (InReplyToID is nil for root, otherwise points to root).
	threadID := payload.Comment.ID
	if payload.Comment.InReplyToID != nil {
		threadID = *payload.Comment.InReplyToID
	}

	var prDBID string
	err = h.DB.QueryRow(ctx, `
		select pr.id from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1
		  and r.github_repo_id   = $2
		  and pr.number          = $3
	`, payload.Installation.ID, payload.Repository.ID, payload.PullRequest.Number).Scan(&prDBID)
	if err != nil {
		log.Printf("PR review comment: PR not found: %v", err)
		return
	}

	_, err = h.DB.Exec(ctx, `
		insert into pr_review_threads (pull_request_id, github_thread_id, last_commenter_login, updated_at)
		values ($1, $2, $3, now())
		on conflict (pull_request_id, github_thread_id) do update set
			last_commenter_login = excluded.last_commenter_login,
			updated_at           = excluded.updated_at
	`, prDBID, threadID, payload.Comment.User.Login)
	if err != nil {
		log.Printf("PR review comment: error upserting thread: %v", err)
	}
}

// ---- pull_request_review_thread (resolved / unresolved) ----

func (h *GitHubHandler) handlePRReviewThreadEvent(ctx context.Context, body []byte) {
	payload, err := github.ParsePRReviewThreadPayload(body)
	if err != nil {
		log.Printf("error parsing PR review thread payload: %v", err)
		return
	}

	if len(payload.Thread.Comments) == 0 {
		return
	}
	// Root comment id is the stable thread key.
	threadID := payload.Thread.Comments[0].ID
	isResolved := payload.Action == "resolved"

	var prDBID string
	err = h.DB.QueryRow(ctx, `
		select pr.id from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1
		  and r.github_repo_id   = $2
		  and pr.number          = $3
	`, payload.Installation.ID, payload.Repository.ID, payload.PullRequest.Number).Scan(&prDBID)
	if err != nil {
		log.Printf("PR review thread: PR not found: %v", err)
		return
	}

	_, err = h.DB.Exec(ctx, `
		insert into pr_review_threads (pull_request_id, github_thread_id, is_resolved, updated_at)
		values ($1, $2, $3, now())
		on conflict (pull_request_id, github_thread_id) do update set
			is_resolved = excluded.is_resolved,
			updated_at  = excluded.updated_at
	`, prDBID, threadID, isResolved)
	if err != nil {
		log.Printf("PR review thread: error upserting thread: %v", err)
	}
}

// ---- check_suite ----

func (h *GitHubHandler) handleCheckSuiteEvent(ctx context.Context, body []byte) {
	payload, err := github.ParseCheckSuitePayload(body)
	if err != nil {
		log.Printf("error parsing check_suite payload: %v", err)
		return
	}

	suite := payload.CheckSuite
	log.Printf("check_suite event: action=%q app=%q sha=%s status=%q conclusion=%q",
		payload.Action, suite.App.Slug, suite.HeadSHA, suite.Status, suite.Conclusion)

	// Find open PRs with this head SHA under this installation's org.
	rows, err := h.DB.Query(ctx, `
		select pr.id from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where gi.installation_id = $1
		  and r.github_repo_id   = $2
		  and pr.head_sha        = $3
		  and pr.state           = 'open'
	`, payload.Installation.ID, payload.Repository.ID, suite.HeadSHA)
	if err != nil {
		log.Printf("check_suite: DB query error: %v", err)
		return
	}
	defer rows.Close()

	var conclusion *string
	if suite.Conclusion != "" {
		conclusion = &suite.Conclusion
	}

	for rows.Next() {
		var prDBID string
		if err := rows.Scan(&prDBID); err != nil {
			continue
		}
		_, _ = h.DB.Exec(ctx, `
			insert into pr_check_runs (pull_request_id, head_sha, app_slug, status, conclusion, updated_at)
			values ($1, $2, $3, $4, $5, now())
			on conflict (pull_request_id, app_slug, head_sha) do update set
				status     = excluded.status,
				conclusion = excluded.conclusion,
				updated_at = excluded.updated_at
		`, prDBID, suite.HeadSHA, suite.App.Slug, suite.Status, conclusion)
	}
}

// ---- installation ----

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

// ---- installation_repositories ----

func (h *GitHubHandler) handleInstallationReposEvent(ctx context.Context, body []byte) {
	payload, err := github.ParseInstallationReposPayload(body)
	if err != nil {
		log.Printf("error parsing installation_repositories payload: %v", err)
		return
	}

	var orgID, dbInstallationID string
	err = h.DB.QueryRow(ctx, `
		select org_id, id from github_installations where installation_id = $1
	`, payload.Installation.ID).Scan(&orgID, &dbInstallationID)
	if err != nil {
		log.Printf("installation_repositories: no installation found for id=%d: %v", payload.Installation.ID, err)
		return
	}

	type addedRepo struct {
		dbID     string
		fullName string
	}
	var added []addedRepo

	for _, repo := range payload.RepositoriesAdded {
		var dbRepoID string
		err := h.DB.QueryRow(ctx, `
			insert into repositories (org_id, installation_id, github_repo_id, full_name, is_active)
			values ($1, $2, $3, $4, true)
			on conflict (org_id, github_repo_id) do update set
				full_name = excluded.full_name,
				is_active = true
			returning id
		`, orgID, dbInstallationID, repo.ID, repo.FullName).Scan(&dbRepoID)
		if err != nil {
			log.Printf("installation_repositories: error upserting repo %s: %v", repo.FullName, err)
			continue
		}
		added = append(added, addedRepo{dbID: dbRepoID, fullName: repo.FullName})
	}

	for _, repo := range payload.RepositoriesRemoved {
		_, _ = h.DB.Exec(ctx, `
			update repositories set is_active = false
			where org_id = $1 and github_repo_id = $2
		`, orgID, repo.ID)
	}

	log.Printf("installation_repositories: added=%d removed=%d for installation_id=%d",
		len(payload.RepositoriesAdded), len(payload.RepositoriesRemoved), payload.Installation.ID)

	if len(added) > 0 {
		go func() {
			bgCtx := context.Background()
			ghClient, err := h.AppClient.ClientForInstallation(bgCtx, payload.Installation.ID)
			if err != nil {
				log.Printf("installation_repositories: failed to get GitHub client: %v", err)
				return
			}
			for _, r := range added {
				n := h.syncOpenPRs(bgCtx, ghClient, orgID, r.dbID, r.fullName)
				log.Printf("installation_repositories: synced %d open PRs for %s", n, r.fullName)
			}
		}()
	}
}

// ---- GitHub App installation callback ----

// GET /api/v1/github/callback
func (h *GitHubHandler) HandleInstallationCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	installationIDStr := r.URL.Query().Get("installation_id")
	userID := r.URL.Query().Get("state")
	setupAction := r.URL.Query().Get("setup_action") // "install" | "update" | "request"

	if setupAction == "request" {
		http.Redirect(w, r, h.FrontendURL+"/request-sent", http.StatusFound)
		return
	}

	if installationIDStr == "" {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}

	var installationID int64
	fmt.Sscanf(installationIDStr, "%d", &installationID)

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

	token, err := h.AppClient.InstallationToken(ctx, installationID)
	if err != nil {
		http.Error(w, "failed to get installation token", http.StatusInternalServerError)
		return
	}
	ghClient := github.NewClient(token)

	var orgID, orgSlug string

	if setupAction == "update" {
		err = h.DB.QueryRow(ctx, `
			select o.id, o.slug
			from organizations o
			join github_installations gi on gi.org_id = o.id
			where gi.installation_id = $1
		`, installationID).Scan(&orgID, &orgSlug)
		if err == nil {
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
	}

	orgSlug = accountLogin
	err = h.DB.QueryRow(ctx, `
		insert into organizations (name, slug, github_account_login, github_account_type, github_account_id, avatar_url)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (slug) do update set
			avatar_url        = excluded.avatar_url,
			github_account_id = excluded.github_account_id
		returning id
	`, accountLogin, orgSlug, accountLogin, accountType, accountID, avatarURL).Scan(&orgID)
	if err != nil {
		log.Printf("failed to upsert org: %v", err)
		http.Error(w, "failed to create org", http.StatusInternalServerError)
		return
	}

	if userID != "" {
		ensureUserExists(ctx, h.DB, userID)
		_, _ = h.DB.Exec(ctx, `
			insert into org_members (org_id, user_id, role)
			values ($1, $2, 'org_admin')
			on conflict (org_id, user_id) do update set role = 'org_admin'
		`, orgID, userID)
	}

	_, _ = h.DB.Exec(ctx,
		`insert into org_preferences (org_id) values ($1) on conflict do nothing`,
		orgID,
	)

	var dbInstallationID string
	err = h.DB.QueryRow(ctx, `
		insert into github_installations (org_id, installation_id, account_login, account_type, account_avatar_url)
		values ($1, $2, $3, $4, $5)
		on conflict (installation_id) do update set
			account_login      = excluded.account_login,
			account_avatar_url = excluded.account_avatar_url
		returning id
	`, orgID, installationID, accountLogin, accountType, avatarURL).Scan(&dbInstallationID)
	if err != nil {
		log.Printf("failed to upsert installation: %v", err)
		http.Error(w, "failed to save installation", http.StatusInternalServerError)
		return
	}

	h.syncReposWithInstallationID(ctx, ghClient, orgID, dbInstallationID)

	if accountType == "Organization" {
		h.syncOrgTeams(ctx, ghClient, orgID, accountLogin)
	}

	http.Redirect(w, r, h.FrontendURL+"/onboarding/repos?org="+orgSlug, http.StatusFound)
}

// ---- repo sync helpers ----

func (h *GitHubHandler) syncRepos(ctx context.Context, ghClient *github.Client, orgID, installationIDStr string) {
	var dbInstallationID string
	err := h.DB.QueryRow(ctx, `select id from github_installations where org_id = $1`, orgID).Scan(&dbInstallationID)
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
				full_name      = excluded.full_name,
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
		var teamID string
		err := h.DB.QueryRow(ctx, `
			insert into teams (org_id, name, slug, github_team_id)
			values ($1, $2, $3, $4)
			on conflict (org_id, slug) do update set
				name           = excluded.name,
				github_team_id = excluded.github_team_id
			returning id
		`, orgID, ghTeam.Name, ghTeam.Slug, ghTeam.ID).Scan(&teamID)
		if err != nil {
			log.Printf("error upserting team %s: %v", ghTeam.Slug, err)
			continue
		}

		members, err := ghClient.ListOrgTeamMembers(ctx, orgLogin, ghTeam.Slug)
		if err != nil {
			log.Printf("error fetching members for team %s: %v", ghTeam.Slug, err)
			continue
		}

		for _, member := range members {
			var memberUserID string
			err := h.DB.QueryRow(ctx, `
				insert into users (id, email, github_user_id, github_username)
				values (gen_random_uuid(), $1, $2, $3)
				on conflict (github_user_id) do update set github_username = excluded.github_username
				returning id
			`, member.Login+"@github", member.ID, member.Login).Scan(&memberUserID)
			if err != nil {
				log.Printf("error upserting user %s: %v", member.Login, err)
				continue
			}

			_, _ = h.DB.Exec(ctx, `
				insert into org_members (org_id, user_id, role) values ($1, $2, 'member')
				on conflict (org_id, user_id) do nothing
			`, orgID, memberUserID)

			_, _ = h.DB.Exec(ctx, `
				insert into team_members (team_id, user_id, role) values ($1, $2, 'member')
				on conflict (team_id, user_id) do nothing
			`, teamID, memberUserID)
		}
	}
}

// syncOpenPRs fetches open PRs for a repo from GitHub, upserts them, and also
// syncs reviews and review requests so queue signals are immediately available.
func (h *GitHubHandler) syncOpenPRs(ctx context.Context, ghClient *github.Client, orgID, repoID, fullName string) int {
	parts := splitRepoName(fullName)
	if parts == nil {
		log.Printf("syncOpenPRs: invalid repo name %q", fullName)
		return 0
	}

	prs, err := ghClient.ListOpenPullRequests(ctx, parts[0], parts[1])
	if err != nil {
		log.Printf("syncOpenPRs: failed to fetch PRs for %s: %v", fullName, err)
		return 0
	}

	now := time.Now()
	for _, pr := range prs {
		state := pr.State
		if pr.MergedAt != nil {
			state = "merged"
		}
		headSHA := pr.Head.SHA

		var prDBID string
		err := h.DB.QueryRow(ctx, `
			insert into pull_requests (
				org_id, repo_id, github_pr_id, number, title, body,
				author_github_login, author_avatar_url, base_branch, head_branch,
				state, draft, additions, deletions, changed_files, html_url,
				opened_at, closed_at, merged_at, last_activity_at, last_synced_at,
				updated_at, head_sha, mergeable
			) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
			on conflict (repo_id, github_pr_id) do update set
				title          = excluded.title,
				state          = excluded.state,
				draft          = excluded.draft,
				additions      = excluded.additions,
				deletions      = excluded.deletions,
				changed_files  = excluded.changed_files,
				last_synced_at = excluded.last_synced_at,
				updated_at     = excluded.updated_at,
				head_sha       = excluded.head_sha,
				mergeable      = coalesce(excluded.mergeable, pull_requests.mergeable)
			returning id
		`,
			orgID, repoID, pr.ID, pr.Number, pr.Title, pr.Body,
			pr.User.Login, pr.User.AvatarURL, pr.Base.Ref, pr.Head.Ref,
			state, pr.Draft, pr.Additions, pr.Deletions, pr.ChangedFiles, pr.HTMLURL,
			pr.CreatedAt, pr.ClosedAt, pr.MergedAt, pr.UpdatedAt, now, now,
			headSHA, pr.Mergeable,
		).Scan(&prDBID)
		if err != nil {
			log.Printf("syncOpenPRs: error upserting PR #%d: %v", pr.Number, err)
			continue
		}

		// Sync reviews (provides commit_sha for stale-review detection).
		reviews, err := ghClient.ListPRReviews(ctx, parts[0], parts[1], pr.Number)
		if err == nil {
			for _, rv := range reviews {
				commitSHA := rv.CommitID
				_, _ = h.DB.Exec(ctx, `
					insert into pr_reviews (pull_request_id, github_review_id, reviewer_login, reviewer_avatar_url, state, submitted_at, commit_sha)
					values ($1, $2, $3, $4, $5, $6, $7)
					on conflict (github_review_id) do update set
						state        = excluded.state,
						submitted_at = excluded.submitted_at,
						commit_sha   = excluded.commit_sha
				`, prDBID, rv.ID, rv.User.Login, rv.User.AvatarURL, rv.State, rv.SubmittedAt, commitSHA)
			}
		}

		// Sync review requests from the PR's requested_reviewers field.
		h.syncReviewRequests(ctx, prDBID, pr.RequestedReviewers)
	}
	return len(prs)
}

// ---- HTTP handlers for manual sync ----

// POST /api/v1/orgs/{orgSlug}/sync
func (h *GitHubHandler) SyncOrg(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgSlug := r.PathValue("orgSlug")

	var orgID string
	var installationID int64
	err := h.DB.QueryRow(ctx, `
		select o.id, gi.installation_id
		from organizations o
		join github_installations gi on gi.org_id = o.id
		where o.slug = $1
	`, orgSlug).Scan(&orgID, &installationID)
	if err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	rows, err := h.DB.Query(ctx, `
		select id, full_name from repositories where org_id = $1 and is_active = true
	`, orgID)
	if err != nil {
		http.Error(w, "failed to fetch repos", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type repoInfo struct {
		id       string
		fullName string
	}
	var repos []repoInfo
	for rows.Next() {
		var ri repoInfo
		if err := rows.Scan(&ri.id, &ri.fullName); err == nil {
			repos = append(repos, ri)
		}
	}

	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		http.Error(w, "failed to authenticate with GitHub", http.StatusInternalServerError)
		return
	}

	total := 0
	for _, repo := range repos {
		total += h.syncOpenPRs(ctx, ghClient, orgID, repo.id, repo.fullName)
	}

	log.Printf("SyncOrg: synced %d open PRs across %d repos for org=%s", total, len(repos), orgSlug)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"synced": total, "repos": len(repos)})
}

// POST /api/v1/orgs/{orgSlug}/repos/{repoID}/sync
func (h *GitHubHandler) SyncRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repoID := r.PathValue("repoID")

	var installationID int64
	var fullName, orgID string
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

	synced := h.syncOpenPRs(ctx, ghClient, orgID, repoID, fullName)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"synced": synced})
}

// ---- helpers ----

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
