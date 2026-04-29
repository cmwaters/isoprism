package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BetaHandler struct {
	FeedbackToken string
	FeedbackRepo  string
	AdminPassword string
	FrontendURL   string
	DB            *pgxpool.Pool
}

type FeedbackRequest struct {
	Type            string `json:"type"`
	Title           string `json:"title"`
	Details         string `json:"details"`
	BetaID          string `json:"beta_id"`
	UserID          string `json:"user_id"`
	RepoFullName    string `json:"repo_full_name"`
	RepoID          string `json:"repo_id"`
	PRNumber        *int   `json:"pr_number"`
	PRTitle         string `json:"pr_title"`
	NodeFullName    string `json:"node_full_name"`
	NodeFilePath    string `json:"node_file_path"`
	BrowserPath     string `json:"browser_path"`
	AppCommitSHA    string `json:"app_commit_sha"`
	SourceCommitSHA string `json:"source_commit_sha"`
}

type createIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

type CreateBetaTesterRequest struct {
	Name string `json:"name"`
}

type CreateBetaTesterResponse struct {
	BetaTester
	Token string `json:"token"`
	Link  string `json:"link"`
}

type BetaTester struct {
	ID                       string                  `json:"id"`
	BetaID                   string                  `json:"beta_id"`
	Name                     string                  `json:"name"`
	Email                    *string                 `json:"email"`
	Status                   string                  `json:"status"`
	InvitedAt                time.Time               `json:"invited_at"`
	AcceptedAt               *time.Time              `json:"accepted_at"`
	CompletedAt              *time.Time              `json:"completed_at"`
	UserID                   *string                 `json:"user_id"`
	SelectedRepoID           *string                 `json:"selected_repo_id"`
	SelectedRepoFullName     *string                 `json:"selected_repo_full_name"`
	TrialStartsAt            *time.Time              `json:"trial_starts_at"`
	TrialEndsAt              *time.Time              `json:"trial_ends_at"`
	QuestionnaireSubmittedAt *time.Time              `json:"questionnaire_submitted_at"`
	Questionnaire            *BetaQuestionnaireAdmin `json:"questionnaire"`
	Token                    *string                 `json:"token,omitempty"`
	Link                     string                  `json:"link"`
}

type BetaQuestionnaireAdmin struct {
	FasterRating       *int    `json:"faster_rating"`
	RiskClarityRating  *int    `json:"risk_clarity_rating"`
	ConfusingOrMissing *string `json:"confusing_or_missing"`
	BugsHit            *string `json:"bugs_hit"`
	BuildNext          *string `json:"build_next"`
	WouldKeepUsing     *string `json:"would_keep_using"`
}

// GET /api/v1/admin/beta/testers
func (h *BetaHandler) ListBetaTesters(w http.ResponseWriter, r *http.Request) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		select
			b.id, b.beta_id, b.name, b.email, b.status, b.invited_at, b.token,
			b.accepted_at, b.completed_at, b.user_id, b.selected_repo_id,
			r.full_name, b.trial_starts_at, b.trial_ends_at,
			q.submitted_at, q.faster_rating, q.risk_clarity_rating,
			q.confusing_or_missing, q.bugs_hit, q.build_next, q.would_keep_using
		from beta_invites b
		left join repositories r on r.id = b.selected_repo_id
		left join beta_questionnaires q on q.invite_id = b.id
		order by b.created_at desc
	`)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	testers := make([]BetaTester, 0)
	for rows.Next() {
		tester, err := scanBetaTester(rows, h.FrontendURL)
		if err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		testers = append(testers, tester)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"testers": testers})
}

// POST /api/v1/admin/beta/testers
func (h *BetaHandler) CreateBetaTester(w http.ResponseWriter, r *http.Request) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateBetaTesterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid tester payload", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	token, err := newInviteToken()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	betaID, err := newBetaID(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "failed to generate beta id", http.StatusInternalServerError)
		return
	}

	var tester BetaTester
	err = h.DB.QueryRow(r.Context(), `
		insert into beta_invites (beta_id, name, token)
		values ($1, $2, $3)
		returning id, beta_id, name, email, status, invited_at, token,
			accepted_at, completed_at, user_id, selected_repo_id,
			trial_starts_at, trial_ends_at
	`, betaID, req.Name, token).Scan(
		&tester.ID, &tester.BetaID, &tester.Name, &tester.Email, &tester.Status,
		&tester.InvitedAt, &tester.Token, &tester.AcceptedAt, &tester.CompletedAt, &tester.UserID,
		&tester.SelectedRepoID, &tester.TrialStartsAt, &tester.TrialEndsAt,
	)
	if err != nil {
		http.Error(w, "failed to create tester", http.StatusInternalServerError)
		return
	}
	tester.Link = inviteLink(h.FrontendURL, token)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateBetaTesterResponse{
		BetaTester: tester,
		Token:      token,
		Link:       tester.Link,
	})
}

// DELETE /api/v1/admin/beta/testers/{testerID}
func (h *BetaHandler) DeleteBetaTester(w http.ResponseWriter, r *http.Request) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	testerID := strings.TrimSpace(chi.URLParam(r, "testerID"))
	if testerID == "" {
		http.Error(w, "tester id is required", http.StatusBadRequest)
		return
	}

	tag, err := h.DB.Exec(r.Context(), `delete from beta_invites where id = $1`, testerID)
	if err != nil {
		http.Error(w, "failed to delete tester", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "tester not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/beta/feedback
func (h *BetaHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if h.FeedbackToken == "" || h.FeedbackRepo == "" {
		http.Error(w, "feedback GitHub target is not configured", http.StatusNotImplemented)
		return
	}

	var req FeedbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid feedback payload", http.StatusBadRequest)
		return
	}

	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.Type != "bug" && req.Type != "feature" {
		http.Error(w, "feedback type must be bug or feature", http.StatusBadRequest)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Details = strings.TrimSpace(req.Details)
	if req.Title == "" || req.Details == "" {
		http.Error(w, "title and details are required", http.StatusBadRequest)
		return
	}

	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}

	issue := createIssueRequest{
		Title:  fmt.Sprintf("[%s] %s", req.Type, req.Title),
		Body:   feedbackIssueBody(req, userID),
		Labels: []string{req.Type},
	}

	payload, err := json.Marshal(issue)
	if err != nil {
		http.Error(w, "failed to encode issue", http.StatusInternalServerError)
		return
	}

	ghReq, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodPost,
		fmt.Sprintf("https://api.github.com/repos/%s/issues", h.FeedbackRepo),
		bytes.NewReader(payload),
	)
	if err != nil {
		http.Error(w, "failed to create GitHub request", http.StatusInternalServerError)
		return
	}
	ghReq.Header.Set("Authorization", "Bearer "+h.FeedbackToken)
	ghReq.Header.Set("Accept", "application/vnd.github+json")
	ghReq.Header.Set("Content-Type", "application/json")
	ghReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(ghReq)
	if err != nil {
		http.Error(w, "failed to submit GitHub issue", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("GitHub issue creation failed: %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		http.Error(w, "failed to decode GitHub issue", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "submitted",
		"issue":    result.Number,
		"html_url": result.HTMLURL,
	})
}

func feedbackIssueBody(req FeedbackRequest, userID string) string {
	pr := "none"
	if req.PRNumber != nil {
		pr = fmt.Sprintf("#%d", *req.PRNumber)
		if req.PRTitle != "" {
			pr += " " + req.PRTitle
		}
	}

	return fmt.Sprintf(`## Feedback

%s

## Context

- Beta ID: %s
- User ID: %s
- Repository: %s
- Repository ID: %s
- PR: %s
- Node: %s
- Node file: %s
- Browser path: %s
- App commit: %s
- Source commit: %s
`,
		req.Details,
		valueOrUnknown(req.BetaID),
		valueOrUnknown(userID),
		valueOrUnknown(req.RepoFullName),
		valueOrUnknown(req.RepoID),
		pr,
		valueOrUnknown(req.NodeFullName),
		valueOrUnknown(req.NodeFilePath),
		valueOrUnknown(req.BrowserPath),
		valueOrUnknown(req.AppCommitSHA),
		valueOrUnknown(req.SourceCommitSHA),
	)
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func (h *BetaHandler) authorizedAdmin(r *http.Request) bool {
	if h.AdminPassword == "" {
		return false
	}
	got := r.Header.Get("X-Admin-Password")
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.AdminPassword)) == 1
}

type betaTesterScanner interface {
	Scan(dest ...interface{}) error
}

func scanBetaTester(row betaTesterScanner, frontendURL string) (BetaTester, error) {
	var tester BetaTester
	var submittedAt *time.Time
	var questionnaire BetaQuestionnaireAdmin
	err := row.Scan(
		&tester.ID, &tester.BetaID, &tester.Name, &tester.Email, &tester.Status,
		&tester.InvitedAt, &tester.Token, &tester.AcceptedAt, &tester.CompletedAt, &tester.UserID,
		&tester.SelectedRepoID, &tester.SelectedRepoFullName, &tester.TrialStartsAt,
		&tester.TrialEndsAt, &submittedAt, &questionnaire.FasterRating,
		&questionnaire.RiskClarityRating, &questionnaire.ConfusingOrMissing,
		&questionnaire.BugsHit, &questionnaire.BuildNext, &questionnaire.WouldKeepUsing,
	)
	if err != nil {
		return tester, err
	}
	tester.QuestionnaireSubmittedAt = submittedAt
	if submittedAt != nil {
		tester.Questionnaire = &questionnaire
	}
	if tester.Token != nil && strings.TrimSpace(*tester.Token) != "" {
		tester.Link = inviteLink(frontendURL, *tester.Token)
	}
	return tester, nil
}

func newInviteToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func newBetaID(ctx context.Context, db *pgxpool.Pool) (string, error) {
	for i := 0; i < 8; i++ {
		raw := make([]byte, 5)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		id := "BETA-" + strings.ToUpper(base64.RawURLEncoding.EncodeToString(raw))
		var exists bool
		if err := db.QueryRow(ctx, `select exists(select 1 from beta_invites where beta_id=$1)`, id).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not generate unique beta id")
}

func inviteLink(frontendURL, token string) string {
	base := strings.TrimRight(frontendURL, "/")
	if token == "" {
		return base + "/beta/{token}"
	}
	return base + "/beta/" + token
}
