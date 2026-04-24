package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
