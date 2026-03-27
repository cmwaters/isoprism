package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VerifyWebhookSignature validates the X-Hub-Signature-256 header against the payload.
func VerifyWebhookSignature(secret string, body []byte, signatureHeader string) error {
	if len(signatureHeader) < 7 || signatureHeader[:7] != "sha256=" {
		return fmt.Errorf("invalid signature format")
	}
	sig, err := hex.DecodeString(signatureHeader[7:])
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// ReadAndVerify reads the request body and verifies the webhook signature.
// Returns the raw body for further parsing.
func ReadAndVerify(r *http.Request, secret string) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if err := VerifyWebhookSignature(secret, body, sig); err != nil {
		return nil, err
	}
	return body, nil
}

// ---- Webhook payloads ----

type WebhookPRPayload struct {
	Action      string       `json:"action"`
	Number      int          `json:"number"`
	PullRequest GHPullRequest `json:"pull_request"`
	Repository  struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

type WebhookInstallationPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login     string `json:"login"`
			Type      string `json:"type"`
			AvatarURL string `json:"avatar_url"`
		} `json:"account"`
	} `json:"installation"`
	Repositories []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repositories"`
}

type WebhookInstallationReposPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	RepositoriesAdded []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
		Name     string `json:"name"`
	} `json:"repositories_added"`
	RepositoriesRemoved []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repositories_removed"`
}

func ParsePRPayload(body []byte) (*WebhookPRPayload, error) {
	var p WebhookPRPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func ParseInstallationPayload(body []byte) (*WebhookInstallationPayload, error) {
	var p WebhookInstallationPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func ParseInstallationReposPayload(body []byte) (*WebhookInstallationReposPayload, error) {
	var p WebhookInstallationReposPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- pull_request_review ----

type WebhookPRReviewPayload struct {
	Action string `json:"action"` // "submitted" | "dismissed" | "edited"
	Review struct {
		ID       int64  `json:"id"`
		State    string `json:"state"`    // "approved" | "changes_requested" | "commented" | "dismissed"
		CommitID string `json:"commit_id"` // SHA at which the review was submitted
		User     struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
		SubmittedAt *time.Time `json:"submitted_at"`
	} `json:"review"`
	PullRequest struct {
		ID     int64 `json:"id"`
		Number int   `json:"number"`
	} `json:"pull_request"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func ParsePRReviewPayload(body []byte) (*WebhookPRReviewPayload, error) {
	var p WebhookPRReviewPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- pull_request_review_comment ----

// WebhookPRReviewCommentPayload is fired when an inline review comment is created/edited/deleted.
// Threads are reconstructed from InReplyToID: null = root (thread ID), non-null = reply.
type WebhookPRReviewCommentPayload struct {
	Action  string `json:"action"` // "created" | "edited" | "deleted"
	Comment struct {
		ID          int64  `json:"id"`
		InReplyToID *int64 `json:"in_reply_to_id"` // nil = root comment = thread ID
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	PullRequest struct {
		ID     int64 `json:"id"`
		Number int   `json:"number"`
	} `json:"pull_request"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func ParsePRReviewCommentPayload(body []byte) (*WebhookPRReviewCommentPayload, error) {
	var p WebhookPRReviewCommentPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- pull_request_review_thread ----

// WebhookPRReviewThreadPayload fires when a review thread is resolved or unresolved.
// The thread's root comment ID (thread.comments[0].id) is used as the stable thread key.
type WebhookPRReviewThreadPayload struct {
	Action string `json:"action"` // "resolved" | "unresolved"
	Thread struct {
		Comments []struct {
			ID int64 `json:"id"`
		} `json:"comments"`
	} `json:"thread"`
	PullRequest struct {
		ID     int64 `json:"id"`
		Number int   `json:"number"`
	} `json:"pull_request"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func ParsePRReviewThreadPayload(body []byte) (*WebhookPRReviewThreadPayload, error) {
	var p WebhookPRReviewThreadPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- check_suite ----

// WebhookCheckSuitePayload fires when a CI check suite completes or is requested.
// We use head_sha to match suites back to open pull_requests.
type WebhookCheckSuitePayload struct {
	Action     string `json:"action"` // "completed" | "requested" | "rerequested"
	CheckSuite struct {
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`     // "queued" | "in_progress" | "completed"
		Conclusion string `json:"conclusion"` // "success"|"failure"|"neutral"|"cancelled"|"skipped"|"timed_out"|"action_required"
		App        struct {
			Slug string `json:"slug"` // e.g. "github-actions"
		} `json:"app"`
	} `json:"check_suite"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func ParseCheckSuitePayload(body []byte) (*WebhookCheckSuitePayload, error) {
	var p WebhookCheckSuitePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
