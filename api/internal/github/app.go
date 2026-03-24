package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)


// AppClient handles GitHub App authentication: generating JWTs and
// fetching/caching per-installation access tokens.
type AppClient struct {
	appID      string
	privateKey *rsa.PrivateKey

	mu     sync.Mutex
	tokens map[int64]*cachedToken // installation_id → token
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

func NewAppClient(appID, privateKeyPEM string) (*AppClient, error) {
	// Railway (and many env var stores) preserve literal \n rather than real
	// newlines. Normalise before attempting PEM decode.
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, `\n`, "\n")
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from GitHub App private key")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub App private key: %w", err)
	}
	return &AppClient{
		appID:      appID,
		privateKey: key,
		tokens:     make(map[int64]*cachedToken),
	}, nil
}

// generateJWT produces a short-lived (9min) JWT for GitHub App authentication.
func (a *AppClient) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-30 * time.Second).Unix(), // issued slightly in the past to avoid clock skew
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(a.privateKey)
}

// InstallationToken returns a valid access token for the given installation,
// fetching a new one if the cached token is expired or absent.
func (a *AppClient) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if cached, ok := a.tokens[installationID]; ok {
		if time.Until(cached.expiresAt) > 2*time.Minute {
			return cached.token, nil
		}
	}

	appJWT, err := a.generateJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d fetching installation token", resp.StatusCode)
	}

	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}

	a.tokens[installationID] = &cachedToken{
		token:     body.Token,
		expiresAt: body.ExpiresAt,
	}
	return body.Token, nil
}

// ClientForInstallation returns a GitHub API client authenticated for a given installation.
func (a *AppClient) ClientForInstallation(ctx context.Context, installationID int64) (*Client, error) {
	token, err := a.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return NewClient(token), nil
}

// GetInstallation fetches installation details using the App JWT (not an installation token).
func (a *AppClient) GetInstallation(ctx context.Context, installationID int64) (*GHInstallation, error) {
	appJWT, err := a.generateJWT()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching installation", resp.StatusCode)
	}
	var inst GHInstallation
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return nil, err
	}
	return &inst, nil
}
