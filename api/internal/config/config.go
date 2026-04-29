package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port string

	SupabaseURL            string
	SupabaseServiceRoleKey string

	GitHubAppPrivateKey string // RSA private key PEM
	GitHubWebhookSecret string
	GitHubClientID      string // App Client ID (starts with "Iv") — used as JWT iss
	GitHubClientSecret  string

	AnthropicAPIKey string
	OpenAIAPIKey    string

	GitHubFeedbackToken string
	GitHubFeedbackRepo  string
	AdminPassword       string

	FrontendURL  string
	FrontendURLs []string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                   getEnv("PORT", "8080"),
		SupabaseURL:            mustGetEnv("SUPABASE_URL"),
		SupabaseServiceRoleKey: mustGetEnv("SUPABASE_SERVICE_ROLE_KEY"),
		GitHubAppPrivateKey:    mustGetEnv("GITHUB_APP_PRIVATE_KEY"),
		GitHubWebhookSecret:    mustGetEnv("GITHUB_WEBHOOK_SECRET"),
		GitHubClientID:         mustGetEnv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:     mustGetEnv("GITHUB_CLIENT_SECRET"),
		AnthropicAPIKey:        getEnv("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:           getEnv("OPENAI_API_KEY", ""),
		GitHubFeedbackToken:    getEnv("GITHUB_FEEDBACK_TOKEN", ""),
		GitHubFeedbackRepo:     getEnv("GITHUB_FEEDBACK_REPO", ""),
		AdminPassword:          getEnv("ADMIN_PASSWORD", ""),
		FrontendURL:            getEnv("FRONTEND_URL", "https://isoprism.com"),
	}
	cfg.FrontendURLs = frontendURLs(cfg.FrontendURL)
	return cfg, nil
}

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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}
