package config

import (
	"fmt"
	"os"
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

	FrontendURL string
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
		FrontendURL:            getEnv("FRONTEND_URL", "http://localhost:3000"),
	}
	return cfg, nil
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
