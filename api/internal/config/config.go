package config

import (
	"fmt"
	"os"
	"strings"
)

// Config stores runtime settings for runtime configuration.
type Config struct {
	Port string

	SupabaseURL            string
	SupabaseServiceRoleKey string

	GitHubAppPrivateKey string // RSA private key PEM
	GitHubWebhookSecret string
	GitHubClientID      string // App Client ID (starts with "Iv") — used as JWT iss
	GitHubClientSecret  string

	GeminiAPIKey string

	GitHubFeedbackToken string
	GitHubFeedbackRepo  string
	AdminPassword       string
	MailtrapAPIKey      string
	PilotEmailFrom      string

	FrontendURL  string
	FrontendURLs []string
	CommitSHA    string
}

// Load loads runtime configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                   getEnv("PORT", "8080"),
		SupabaseURL:            mustGetEnv("SUPABASE_URL"),
		SupabaseServiceRoleKey: mustGetEnv("SUPABASE_SERVICE_ROLE_KEY"),
		GitHubAppPrivateKey:    mustGetEnv("GITHUB_APP_PRIVATE_KEY"),
		GitHubWebhookSecret:    mustGetEnv("GITHUB_WEBHOOK_SECRET"),
		GitHubClientID:         mustGetEnv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:     mustGetEnv("GITHUB_CLIENT_SECRET"),
		GeminiAPIKey:           getEnv("GEMINI_API_KEY", ""),
		GitHubFeedbackToken:    getEnv("GITHUB_FEEDBACK_TOKEN", ""),
		GitHubFeedbackRepo:     getEnv("GITHUB_FEEDBACK_REPO", ""),
		AdminPassword:          getEnv("ADMIN_PASSWORD", ""),
		MailtrapAPIKey:         getEnv("MAILTRAP_API_KEY", ""),
		PilotEmailFrom:         getEnv("PILOT_EMAIL_FROM", "Isoprism <pilot@isoprism.com>"),
		FrontendURL:            getEnv("FRONTEND_URL", "https://isoprism.com"),
		CommitSHA:              firstEnv("ISOPRISM_COMMIT_SHA", "RAILWAY_GIT_COMMIT_SHA", "VERCEL_GIT_COMMIT_SHA", "GIT_COMMIT_SHA"),
	}
	if cfg.CommitSHA == "" {
		cfg.CommitSHA = "unknown"
	}
	cfg.FrontendURLs = frontendURLs(cfg.FrontendURL)
	return cfg, nil
}

// frontendURLs builds the allowed frontend URL list from configuration.
func frontendURLs(defaultURL string) []string {
	raw := getEnv("FRONTEND_URLS", defaultURL+",http://localhost:3000")
	seen := make(map[string]bool)
	var urls []string
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		urls = append(urls, value)
	}
	return urls
}

// getEnv loads env for runtime configuration.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// firstEnv returns the first matching env.
func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// mustGetEnv returns a required environment variable or panics.
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}
