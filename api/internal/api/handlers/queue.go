package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/isoprism/api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type QueueHandler struct {
	DB *pgxpool.Pool
}

// GET /api/v1/repos/{repoID}/queue
func (h *QueueHandler) GetQueue(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	// Verify repo ownership
	var exists bool
	h.DB.QueryRow(ctx, `select exists(select 1 from repositories where id=$1 and user_id=$2)`, repoID, userID).Scan(&exists)
	if !exists {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	rows, err := h.DB.Query(ctx, `
		select
			pr.id, pr.repo_id, pr.github_pr_id, pr.number, pr.title, pr.body,
			pr.author_login, pr.author_avatar_url,
			pr.base_branch, pr.head_branch, pr.base_commit_sha, pr.head_commit_sha,
			pr.state, pr.draft, pr.html_url, pr.opened_at, pr.merged_at,
			pr.last_activity_at, pr.graph_status, pr.created_at,
			coalesce(pa.summary, '') as summary,
			coalesce(pa.nodes_changed, 0) as nodes_changed,
			pa.risk_score,
			pa.risk_label
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		left join pr_analyses pa on pa.pull_request_id = pr.id
		where pr.repo_id = $1
		  and pr.state   = 'open'
		  and pr.draft   = false
		  and pr.base_branch = 'main'
		  and pr.base_commit_sha = r.main_commit_sha
		  and pr.graph_status = 'ready'
		order by pr.opened_at asc
	`, repoID)
	if err != nil {
		log.Printf("GetQueue: db error: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type queueRow struct {
		pr           models.PullRequest
		summary      string
		nodesChanged int
		riskScore    *int
		riskLabel    *string
	}

	var queueItems []queueRow
	for rows.Next() {
		var row queueRow
		var pr models.PullRequest
		if err := rows.Scan(
			&pr.ID, &pr.RepoID, &pr.GitHubPRID, &pr.Number, &pr.Title, &pr.Body,
			&pr.AuthorLogin, &pr.AuthorAvatarURL,
			&pr.BaseBranch, &pr.HeadBranch, &pr.BaseCommitSHA, &pr.HeadCommitSHA,
			&pr.State, &pr.Draft, &pr.HTMLURL, &pr.OpenedAt, &pr.MergedAt,
			&pr.LastActivityAt, &pr.GraphStatus, &pr.CreatedAt,
			&row.summary, &row.nodesChanged, &row.riskScore, &row.riskLabel,
		); err != nil {
			log.Printf("GetQueue: scan error: %v", err)
			continue
		}
		row.pr = pr
		queueItems = append(queueItems, row)
	}

	// Score urgency for each PR
	type scoredPR struct {
		models.QueuePR
	}

	prs := make([]models.QueuePR, 0, len(queueItems))
	for _, row := range queueItems {
		age := time.Since(row.pr.OpenedAt)

		// wait_time_score: hours open, normalised to [0,1] with 1 week as max
		waitHours := age.Hours()
		waitScore := math.Min(waitHours/168.0, 1.0)

		// risk_score: 1–10, normalised
		riskVal := 5
		if row.riskScore != nil {
			riskVal = *row.riskScore
		}
		riskScore := float64(riskVal) / 10.0

		// nodes_changed_score: normalised to [0,1] with 15 as max
		nodesScore := math.Min(float64(row.nodesChanged)/15.0, 1.0)

		urgency := waitScore*0.4 + riskScore*0.35 + nodesScore*0.25

		q := models.QueuePR{
			PullRequest:  row.pr,
			Summary:      nilStrPtr(row.summary),
			NodesChanged: row.nodesChanged,
			RiskScore:    row.riskScore,
			RiskLabel:    row.riskLabel,
			UrgencyScore: urgency,
		}
		prs = append(prs, q)
	}

	// Sort descending by urgency
	for i := 0; i < len(prs); i++ {
		for j := i + 1; j < len(prs); j++ {
			if prs[j].UrgencyScore > prs[i].UrgencyScore {
				prs[i], prs[j] = prs[j], prs[i]
			}
		}
	}

	// Cap at 5
	if len(prs) > 5 {
		prs = prs[:5]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"prs": prs})
}

func nilStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
