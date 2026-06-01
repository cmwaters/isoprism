package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/isoprism/api/internal/api"
	"github.com/isoprism/api/internal/config"
	"github.com/isoprism/api/internal/db"
	"github.com/isoprism/api/internal/github"
)

// main starts the process and reports fatal startup errors.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Println("connected to database")

	if err := db.VerifyMigrationVersion(ctx, pool); err != nil {
		log.Fatalf("database migration check failed: %v", err)
	}
	log.Printf("database migration version %s verified", db.RequiredMigrationVersion)

	appClient, err := github.NewAppClient(cfg.GitHubClientID, cfg.GitHubAppPrivateKey)
	if err != nil {
		log.Fatalf("failed to initialise GitHub App client: %v", err)
	}

	router := api.NewRouter(cfg, pool, appClient)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("API server listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
