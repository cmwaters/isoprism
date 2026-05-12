package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BetaHandler struct {
	FeedbackToken  string
	FeedbackRepo   string
	AdminPassword  string
	MailtrapAPIKey string
	EmailFrom      string
	FrontendURL    string
	DB             *pgxpool.Pool
}

type FeedbackRequest struct {
	Type            string `json:"type"`
	Title           string `json:"title"`
	Details         string `json:"details"`
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
	Name          string `json:"name"`
	Email         string `json:"email"`
	Languages     string `json:"languages"`
	PublicRepoURL string `json:"public_repo_url"`
}

type CreateBetaTesterResponse struct {
	BetaTester
	Token string `json:"token"`
	Link  string `json:"link"`
}

type BetaTester struct {
	ID                       string                  `json:"id"`
	Name                     string                  `json:"name"`
	Email                    *string                 `json:"email"`
	Status                   string                  `json:"status"`
	InvitedAt                *time.Time              `json:"invited_at"`
	AcceptedAt               *time.Time              `json:"accepted_at"`
	CompletedAt              *time.Time              `json:"completed_at"`
	UserID                   *string                 `json:"user_id"`
	SelectedRepoID           *string                 `json:"selected_repo_id"`
	SelectedRepoFullName     *string                 `json:"selected_repo_full_name"`
	TrialStartsAt            *time.Time              `json:"trial_starts_at"`
	TrialEndsAt              *time.Time              `json:"trial_ends_at"`
	ReviewSentAt             *time.Time              `json:"review_sent_at"`
	PilotLanguages           *string                 `json:"pilot_languages"`
	PublicRepoURL            *string                 `json:"public_repo_url"`
	IssueCount               int                     `json:"issue_count"`
	FeatureCount             int                     `json:"feature_count"`
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

type PilotRegistrationRequest struct {
	AIWritesMostSoftware string `json:"ai_writes_most_software"`
	CurrentReviewTools   string `json:"current_review_tools"`
	ReviewWorkPercent    int    `json:"review_work_percent"`
	RoleChange           string `json:"role_change"`
	ReviewPainPoints     string `json:"review_pain_points"`
	AIReviewDifference   string `json:"ai_review_difference"`
	OtherComments        string `json:"other_comments"`
	InterestedInPilot    bool   `json:"interested_in_pilot"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	PilotLanguages       string `json:"pilot_languages"`
	PublicRepoURL        string `json:"public_repo_url"`
}

type PilotReviewRequest struct {
	FasterRating       *int   `json:"faster_rating"`
	RiskClarityRating  *int   `json:"risk_clarity_rating"`
	ConfusingOrMissing string `json:"confusing_or_missing"`
	BugsHit            string `json:"bugs_hit"`
	BuildNext          string `json:"build_next"`
	WouldKeepUsing     string `json:"would_keep_using"`
}

type AcceptPilotInviteRequest struct {
	UserID string `json:"user_id"`
}

type PilotForm struct {
	ID          string          `json:"id"`
	PilotUserID *string         `json:"pilot_user_id"`
	FormType    string          `json:"form_type"`
	Name        *string         `json:"name"`
	Email       *string         `json:"email"`
	Answers     json.RawMessage `json:"answers"`
	SubmittedAt time.Time       `json:"submitted_at"`
}

// GET /api/v1/admin/beta/testers
func (h *BetaHandler) ListBetaTesters(w http.ResponseWriter, r *http.Request) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		select
			b.id, b.name, b.email, b.status, b.invited_at, b.token,
			b.accepted_at, b.completed_at, b.user_id, b.selected_repo_id,
			r.full_name, b.trial_starts_at, b.trial_ends_at, b.review_sent_at,
			b.pilot_languages, b.public_repo_url,
			b.issue_count,
			b.feature_count,
			q.submitted_at, q.faster_rating, q.risk_clarity_rating,
			q.confusing_or_missing, q.bugs_hit, q.build_next, q.would_keep_using
		from pilot_users b
		left join repositories r on r.id = b.selected_repo_id
		left join pilot_questionaire q on q.invite_id = b.id
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
	req.Email = strings.TrimSpace(req.Email)
	req.Languages = strings.TrimSpace(req.Languages)
	req.PublicRepoURL = strings.TrimSpace(req.PublicRepoURL)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var tester BetaTester
	err := h.DB.QueryRow(r.Context(), `
		insert into pilot_users (name, email, pilot_languages, public_repo_url, status)
		values ($1, nullif($2, ''), nullif($3, ''), nullif($4, ''), 'registered')
		returning id, name, email, status, invited_at, token,
			accepted_at, completed_at, user_id, selected_repo_id,
			trial_starts_at, trial_ends_at
	`, req.Name, req.Email, req.Languages, req.PublicRepoURL).Scan(
		&tester.ID, &tester.Name, &tester.Email, &tester.Status,
		&tester.InvitedAt, &tester.Token, &tester.AcceptedAt, &tester.CompletedAt, &tester.UserID,
		&tester.SelectedRepoID, &tester.TrialStartsAt, &tester.TrialEndsAt,
	)
	if err != nil {
		http.Error(w, "failed to create tester", http.StatusInternalServerError)
		return
	}
	if tester.Token != nil {
		tester.Link = inviteLink(h.FrontendURL, *tester.Token)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateBetaTesterResponse{
		BetaTester: tester,
		Token:      valueFromPtr(tester.Token),
		Link:       tester.Link,
	})
}

// POST /api/v1/pilot/register
func (h *BetaHandler) RegisterPilotInterest(w http.ResponseWriter, r *http.Request) {
	var req PilotRegistrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid registration payload", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.PilotLanguages = strings.TrimSpace(req.PilotLanguages)
	req.PublicRepoURL = strings.TrimSpace(req.PublicRepoURL)
	if req.InterestedInPilot && (req.Name == "" || req.Email == "") {
		http.Error(w, "name and email are required for pilot interest", http.StatusBadRequest)
		return
	}
	if req.Email != "" {
		var existingID string
		err := h.DB.QueryRow(r.Context(), `
			select id
			from pilot_forms
			where form_type = 'registration'
			  and lower(email) = lower($1)
			limit 1
		`, req.Email).Scan(&existingID)
		if err == nil {
			http.Error(w, "this email has already been registered", http.StatusConflict)
			return
		}
		if err != pgx.ErrNoRows {
			http.Error(w, "failed to check registration", http.StatusInternalServerError)
			return
		}
	}

	answers, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "failed to encode registration", http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start registration", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var pilotUserID *string
	if req.InterestedInPilot {
		var id string
		err = tx.QueryRow(r.Context(), `
			insert into pilot_users (name, email, status, pilot_languages, public_repo_url)
			values ($1, $2, 'registered', nullif($3, ''), nullif($4, ''))
			returning id
		`, req.Name, req.Email, req.PilotLanguages, req.PublicRepoURL).Scan(&id)
		if err != nil {
			http.Error(w, "failed to create pilot user", http.StatusInternalServerError)
			return
		}
		pilotUserID = &id
	}

	var formID string
	err = tx.QueryRow(r.Context(), `
		insert into pilot_forms (pilot_user_id, form_type, name, email, answers)
		values ($1, 'registration', nullif($2, ''), nullif($3, ''), $4::jsonb)
		returning id
	`, pilotUserID, req.Name, req.Email, string(answers)).Scan(&formID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "this email has already been registered", http.StatusConflict)
			return
		}
		http.Error(w, "failed to save registration", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save registration", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "submitted",
		"form_id":       formID,
		"pilot_user_id": pilotUserID,
	})
}

// POST /api/v1/pilot/invites/{token}/accept
func (h *BetaHandler) AcceptPilotInvite(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		http.Error(w, "invite token is required", http.StatusBadRequest)
		return
	}
	var req AcceptPilotInviteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid invite payload", http.StatusBadRequest)
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		http.Error(w, "user id is required", http.StatusBadRequest)
		return
	}

	tag, err := h.DB.Exec(r.Context(), `
		update pilot_users
		set user_id = $1,
			accepted_at = coalesce(accepted_at, now()),
			status = case when status = 'registered' then 'invited' else status end
		where token = $2
	`, req.UserID, token)
	if err != nil {
		http.Error(w, "failed to accept invite", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "invite not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
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

	tag, err := h.DB.Exec(r.Context(), `delete from pilot_users where id = $1`, testerID)
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

// GET /api/v1/admin/pilot/forms
func (h *BetaHandler) ListPilotForms(w http.ResponseWriter, r *http.Request) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		select id, pilot_user_id, form_type, name, email, answers, submitted_at
		from pilot_forms
		order by submitted_at desc
	`)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	forms := make([]PilotForm, 0)
	for rows.Next() {
		var form PilotForm
		if err := rows.Scan(&form.ID, &form.PilotUserID, &form.FormType, &form.Name, &form.Email, &form.Answers, &form.SubmittedAt); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		forms = append(forms, form)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"forms": forms})
}

// POST /api/v1/admin/pilot/users/{testerID}/invite
func (h *BetaHandler) InvitePilotUser(w http.ResponseWriter, r *http.Request) {
	h.sendPilotEmail(w, r, "invite")
}

// POST /api/v1/admin/pilot/users/{testerID}/review-email
func (h *BetaHandler) SendReviewEmail(w http.ResponseWriter, r *http.Request) {
	h.sendPilotEmail(w, r, "review")
}

func (h *BetaHandler) sendPilotEmail(w http.ResponseWriter, r *http.Request, kind string) {
	if !h.authorizedAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.MailtrapAPIKey == "" {
		http.Error(w, "MAILTRAP_API_KEY is not configured", http.StatusNotImplemented)
		return
	}

	testerID := strings.TrimSpace(chi.URLParam(r, "testerID"))
	if testerID == "" {
		http.Error(w, "tester id is required", http.StatusBadRequest)
		return
	}

	token, err := newInviteToken()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	var name, email string
	var link string
	if kind == "review" {
		err = h.DB.QueryRow(r.Context(), `
			update pilot_users
			set review_token = $1, review_sent_at = now()
			where id = $2
				and email is not null and email <> ''
				and token is not null
				and user_id is not null
			returning name, email
		`, token, testerID).Scan(&name, &email)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "pilot user must be invited and registered with GitHub before review email", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "failed to create review email", http.StatusInternalServerError)
			return
		}
		link = reviewLink(h.FrontendURL, token)
	} else {
		err = h.DB.QueryRow(r.Context(), `
			update pilot_users
			set token = $1, status = 'invited', invited_at = now()
			where id = $2 and email is not null and email <> ''
			returning name, email
		`, token, testerID).Scan(&name, &email)
		link = inviteLink(h.FrontendURL, token)
	}
	if err != nil {
		http.Error(w, "pilot user with email not found", http.StatusNotFound)
		return
	}

	subject := "Your Isoprism pilot invite"
	html := fmt.Sprintf(`<p>Hey %s</p><p>Thanks for your interest in the Isoprism pilot. We'd love to have you help us out.</p><p><a href="%s">Click this link to get started.</a> You will be asked to connect your Github and select a repository to work from.</p><p>At the end of the week, you should receive another email to fill out a quick survey to tell us how it went. Feel free to submit any feature requests or bugs along the way.</p><p>Cheers,</p><p>Callum</p>`, htmlEscape(name), link)
	if kind == "review" {
		subject = "Share your Isoprism pilot review"
		html = fmt.Sprintf(`<p>Hi %s,</p><p>Thanks for trying the Isoprism pilot. Could you complete the short review questionnaire?</p><p><a href="%s">Complete the pilot review</a></p>`, htmlEscape(name), link)
	}
	if err := h.sendMailtrapEmail(r.Context(), email, subject, html); err != nil {
		log.Printf("pilot %s email failed for pilot_user_id=%s: %v", kind, testerID, err)
		http.Error(w, "failed to send email: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "sent",
		"link":   link,
	})
}

// POST /api/v1/pilot/review/{token}
func (h *BetaHandler) SubmitPilotReview(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		http.Error(w, "review token is required", http.StatusBadRequest)
		return
	}
	var req PilotReviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid review payload", http.StatusBadRequest)
		return
	}

	answers, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "failed to encode review", http.StatusInternalServerError)
		return
	}

	var userID, name, email string
	err = h.DB.QueryRow(r.Context(), `
		select id, name, coalesce(email, '')
		from pilot_users
		where review_token = $1
	`, token).Scan(&userID, &name, &email)
	if err != nil {
		http.Error(w, "review link not found", http.StatusNotFound)
		return
	}

	_, err = h.DB.Exec(r.Context(), `
		insert into pilot_forms (pilot_user_id, form_type, name, email, answers)
		values ($1, 'review', nullif($2, ''), nullif($3, ''), $4::jsonb)
	`, userID, name, email, string(answers))
	if err != nil {
		http.Error(w, "failed to save review", http.StatusInternalServerError)
		return
	}

	_, _ = h.DB.Exec(r.Context(), `
		insert into pilot_questionaire (
			invite_id, faster_rating, risk_clarity_rating, confusing_or_missing,
			bugs_hit, build_next, would_keep_using
		) values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (invite_id) do update set
			faster_rating = excluded.faster_rating,
			risk_clarity_rating = excluded.risk_clarity_rating,
			confusing_or_missing = excluded.confusing_or_missing,
			bugs_hit = excluded.bugs_hit,
			build_next = excluded.build_next,
			would_keep_using = excluded.would_keep_using,
			submitted_at = now()
	`, userID, req.FasterRating, req.RiskClarityRating, req.ConfusingOrMissing, req.BugsHit, req.BuildNext, req.WouldKeepUsing)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "submitted"})
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
	if userID != "" {
		column := "issue_count"
		if req.Type == "feature" {
			column = "feature_count"
		}
		_, _ = h.DB.Exec(r.Context(), fmt.Sprintf(`
			update pilot_users set %s = %s + 1 where user_id = $1
		`, column, column), userID)
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
		&tester.ID, &tester.Name, &tester.Email, &tester.Status,
		&tester.InvitedAt, &tester.Token, &tester.AcceptedAt, &tester.CompletedAt, &tester.UserID,
		&tester.SelectedRepoID, &tester.SelectedRepoFullName, &tester.TrialStartsAt,
		&tester.TrialEndsAt, &tester.ReviewSentAt, &tester.PilotLanguages, &tester.PublicRepoURL,
		&tester.IssueCount, &tester.FeatureCount, &submittedAt, &questionnaire.FasterRating,
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

func (h *BetaHandler) sendMailtrapEmail(ctx context.Context, to, subject, htmlBody string) error {
	from, err := mail.ParseAddress(h.EmailFrom)
	if err != nil {
		return err
	}
	fromPayload := map[string]string{"email": from.Address}
	if from.Name != "" {
		fromPayload["name"] = from.Name
	}

	payload, err := json.Marshal(map[string]interface{}{
		"from":    fromPayload,
		"to":      []map[string]string{{"email": to}},
		"subject": subject,
		"html":    htmlBody,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://send.api.mailtrap.io/api/send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.MailtrapAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mailtrap returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func inviteLink(frontendURL, token string) string {
	base := strings.TrimRight(frontendURL, "/")
	if token == "" {
		return base + "/pilot/{token}"
	}
	return base + "/pilot/" + token
}

func reviewLink(frontendURL, token string) string {
	base := strings.TrimRight(frontendURL, "/")
	if token == "" {
		return base + "/pilot/review/{token}"
	}
	return base + "/pilot/review/" + token
}

func valueFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func htmlEscape(value string) string {
	return html.EscapeString(value)
}
