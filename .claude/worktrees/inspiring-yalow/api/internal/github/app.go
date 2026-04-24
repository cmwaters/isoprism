package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)


// AppClient handles GitHub App authentication: generating JWTs and
// fetching/caching per-installation access tokens.
type AppClient struct {
	clientID   string // GitHub App Client ID (e.g. "Iv23liXXXXXX") — used as JWT iss
	privateKey *rsa.PrivateKey

	mu     sync.Mutex
	tokens map[int64]*cachedToken // installation_id → token
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewAppClient creates a GitHub App client.
// clientID is the App's Client ID (shown on the App settings page, starts with "Iv").
// privateKeyPEM is the RSA private key in PEM format (literal \n sequences are normalised).
func NewAppClient(clientID, privateKeyPEM string) (*AppClient, error) {
	clientID = strings.TrimSpace(clientID)
	// Railway (and many env var stores) preserve literal \n rather than real
	// newlines. Normalise before attempting PEM decode.
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, `\n`, "\n")
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from GitHub App private key")
	}
	log.Printf("GitHub App client: clientID=%q, pem block type=%q, key bytes=%d", clientID, block.Type, len(block.Bytes))
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub App private key: %w", err)
	}
	return &AppClient{
		clientID:   clientID,
		privateKey: key,
		tokens:     make(map[int64]*cachedToken),
	}, nil
}

// generateJWT produces a short-lived (9min) JWT for GitHub App authentication.
// GitHub requires the iss claim to be the App's Client ID (not the numeric App ID)
// as of their 2025 JWT authentication update.
func (a *AppClient) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-30 * time.Second).Unix(), // issued slightly in the past to avoid clock skew
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.clientID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	return signed, nil
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d fetching installation: %s", resp.StatusCode, string(body))
	}
	var inst GHInstallation
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return nil, err
	}
	return &inst, nil
}
