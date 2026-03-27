package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aperture/api/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type QueueHandler struct {
	DB *pgxpool.Pool
}

// Tier constants — higher value = more urgent.
const (
	tierACritical = 8 // author: CI failing or merge conflict
	tierAHigh     = 7 // author: changes_requested (unaddressed) or unresolved threads
	tierBHigh     = 6 // reviewer: re-review you requested, or explicitly requested
	tierAMedium   = 5 // author: approved, needs merging
	tierBMedium   = 4 // reviewer: general visibility (assumed codeowner)
)

func tierLabel(t int) string {
	switch {
	case t >= tierACritical:
		return "critical"
	case t >= tierAHigh:
		return "high"
	default:
		return "medium"
	}
}

// prSignals holds the queue-specific signals fetched alongside each PR.
type prSignals struct {
	HasApproval             bool
	HasChangesRequested     bool
	LastChangesRequester    *string
	LastChangesRequestedSHA *string
	LastReviewAt            *time.Time
	CIFailing               bool
	ThreadsAwaitingAuthor   int
	RequestedYou            bool
	HeadSHA                 *string
	Mergeable               *bool
}

// GET /api/v1/orgs/{orgSlug}/queue
func (h *QueueHandler) GetQueue(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var orgID string
	if err := h.DB.QueryRow(ctx, `select id from organizations where slug = $1`, orgSlug).Scan(&orgID); err != nil {
		http.Error(w, "org not found", http.StatusNotFound)
		return
	}

	// Resolve current user's GitHub username — personalises the ranking.
	var githubUsername string
	if userID != "" {
		_ = h.DB.QueryRow(ctx,
			`select coalesce(github_username, '') from users where id = $1`, userID,
		).Scan(&githubUsername)
	}
	log.Printf("GetQueue: org=%s user=%s github=%s", orgSlug, userID, githubUsername)

	rows, err := h.DB.Query(ctx, `
		select
			pr.id, pr.org_id, pr.repo_id, pr.github_pr_id, pr.number, pr.title,
			pr.body, pr.author_github_login, pr.author_avatar_url,
			pr.base_branch, pr.head_branch, pr.state, pr.draft,
			pr.additions, pr.deletions, pr.changed_files, pr.html_url,
			pr.opened_at, pr.closed_at, pr.merged_at, pr.last_activity_at,
			pr.last_synced_at, pr.created_at, pr.updated_at,
			pr.head_sha,
			pr.mergeable,
			r.full_name as repo_full_name,

			-- Any approval from any reviewer?
			exists (
				select 1 from pr_reviews rev
				where rev.pull_request_id = pr.id and rev.state = 'approved'
			) as has_approval,

			-- Any reviewer requested changes (and review not dismissed)?
			exists (
				select 1 from pr_reviews rev
				where rev.pull_request_id = pr.id and rev.state = 'changes_requested'
			) as has_changes_requested,

			-- Who most recently requested changes?
			(
				select rev.reviewer_login from pr_reviews rev
				where rev.pull_request_id = pr.id and rev.state = 'changes_requested'
				order by rev.submitted_at desc limit 1
			) as last_changes_requester,

			-- At which SHA did they request changes?
			(
				select rev.commit_sha from pr_reviews rev
				where rev.pull_request_id = pr.id and rev.state = 'changes_requested'
				order by rev.submitted_at desc limit 1
			) as last_changes_requested_sha,

			-- When was the most recent review of any kind?
			(
				select max(rev.submitted_at) from pr_reviews rev
				where rev.pull_request_id = pr.id
			) as last_review_at,

			-- Is CI failing for the current head commit?
			exists (
				select 1 from pr_check_runs cr
				where cr.pull_request_id = pr.id
				  and cr.head_sha        = pr.head_sha
				  and cr.status          = 'completed'
				  and cr.conclusion      in ('failure', 'timed_out', 'action_required')
			) as ci_failing,

			-- Unresolved threads where the last comment is from someone other than the author
			-- (i.e. the author owes a response).
			(
				select count(*) from pr_review_threads prt
				where prt.pull_request_id      = pr.id
				  and prt.is_resolved          = false
				  and prt.last_commenter_login is distinct from pr.author_github_login
			) as threads_awaiting_author,

			-- Was the current user explicitly requested as a reviewer?
			exists (
				select 1 from pr_review_requests prr
				where prr.pull_request_id = pr.id
				  and prr.reviewer_login  = $2
			) as requested_you

		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		where pr.org_id   = $1
		  and pr.state    = 'open'
		  and pr.draft    = false
		  and r.is_active = true
	`, orgID, githubUsername)
	if err != nil {
		log.Printf("GetQueue: DB error: %v", err)
		http.Error(w, "failed to fetch queue: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []models.QueueItem
	for rows.Next() {
		var pr models.PullRequest
		var s prSignals

		if err := rows.Scan(
			&pr.ID, &pr.OrgID, &pr.RepoID, &pr.GitHubPRID, &pr.Number, &pr.Title,
			&pr.Body, &pr.AuthorGitHubLogin, &pr.AuthorAvatarURL,
			&pr.BaseBranch, &pr.HeadBranch, &pr.State, &pr.Draft,
			&pr.Additions, &pr.Deletions, &pr.ChangedFiles, &pr.HTMLURL,
			&pr.OpenedAt, &pr.ClosedAt, &pr.MergedAt, &pr.LastActivityAt,
			&pr.LastSyncedAt, &pr.CreatedAt, &pr.UpdatedAt,
			&s.HeadSHA,
			&s.Mergeable,
			&pr.RepoFullName,
			&s.HasApproval,
			&s.HasChangesRequested,
			&s.LastChangesRequester,
			&s.LastChangesRequestedSHA,
			&s.LastReviewAt,
			&s.CIFailing,
			&s.ThreadsAwaitingAuthor,
			&s.RequestedYou,
		); err != nil {
			log.Printf("GetQueue: scan error (skipping row): %v", err)
			continue
		}

		// Store resolved signal fields back onto the PR model for JSON output.
		pr.HeadSHA = s.HeadSHA
		pr.Mergeable = s.Mergeable

		item := buildQueueItem(pr, s, githubUsername)
		if item != nil {
			items = append(items, *item)
		}
	}

	// Sort: highest tier first; within a tier, highest staleness first.
	sort.Slice(items, func(i, j int) bool {
		if items[i].TierScore != items[j].TierScore {
			return items[i].TierScore > items[j].TierScore
		}
		return items[i].WaitingHours > items[j].WaitingHours
	})

	// Top 5.
	if len(items) > 5 {
		items = items[:5]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

// buildQueueItem classifies a PR into a bucket/tier or returns nil if it should
// be hidden from the current user's queue.
func buildQueueItem(pr models.PullRequest, s prSignals, githubUsername string) *models.QueueItem {
	isAuthor := githubUsername != "" && pr.AuthorGitHubLogin == githubUsername

	// Have new commits been pushed since the last changes_requested review?
	// If both SHAs are known and different, the author has responded.
	commitsAfterReview := s.HasChangesRequested &&
		s.LastChangesRequestedSHA != nil &&
		s.HeadSHA != nil &&
		*s.LastChangesRequestedSHA != *s.HeadSHA

	// Staleness: how long since the last review (or PR open).
	stalenessRef := pr.OpenedAt
	if s.LastReviewAt != nil {
		stalenessRef = *s.LastReviewAt
	}
	waitingHours := time.Since(stalenessRef).Hours()

	var tier int
	var stateReason, actionBucket string

	if isAuthor {
		// ── Bucket A: Author Required ──────────────────────────────────────
		actionBucket = "author"
		switch {
		case s.CIFailing:
			tier = tierACritical
			stateReason = "ci_failing"

		case s.Mergeable != nil && !*s.Mergeable:
			tier = tierACritical
			stateReason = "merge_conflict"

		case s.HasChangesRequested && !commitsAfterReview:
			// Reviewer is waiting on the author to address feedback.
			tier = tierAHigh
			stateReason = "changes_requested"

		case s.ThreadsAwaitingAuthor > 0:
			// Reviewer asked question(s); author hasn't replied.
			tier = tierAHigh
			stateReason = "unresolved_threads"

		case s.HasApproval:
			// Approved — just needs merging.
			tier = tierAMedium
			stateReason = "ready_to_merge"

		default:
			// Ball is in the reviewer's court; nothing to do for the author.
			return nil
		}

	} else {
		// ── Bucket B: Reviewer Required ────────────────────────────────────
		actionBucket = "reviewer"
		switch {
		case s.CIFailing:
			// Author must fix the build first; nothing for reviewer to do.
			return nil

		case s.Mergeable != nil && !*s.Mergeable:
			// Author must resolve conflicts first.
			return nil

		case s.HasChangesRequested && !commitsAfterReview:
			// Author hasn't addressed the feedback yet.
			return nil

		case s.HasChangesRequested && commitsAfterReview:
			// Author has pushed new commits since changes were requested.
			tier = tierBHigh
			if s.LastChangesRequester != nil &&
				strings.EqualFold(*s.LastChangesRequester, githubUsername) {
				// You specifically asked for changes — higher obligation to re-review.
				stateReason = "re_review"
			} else {
				stateReason = "re_review"
				tier = tierBMedium // someone else's changes_requested, lower priority
			}

		case s.RequestedYou:
			tier = tierBHigh
			stateReason = "review_requested"

		default:
			// General visibility — assume codeowner.
			tier = tierBMedium
			stateReason = "needs_review"
		}
	}

	return &models.QueueItem{
		PullRequest:  pr,
		TierScore:    tier,
		WaitingHours: waitingHours,
		StateReason:  stateReason,
		ActionBucket: actionBucket,
		PriorityTier: tierLabel(tier),
	}
}
