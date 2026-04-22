package events

import (
	"context"
	"log"

	"github.com/isoprism/api/internal/github"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MergePR advances the base graph to the merge commit SHA.
func MergePR(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, repoID, mergeCommitSHA string) {
	log.Printf("MergePR: advancing repo %s to %s", repoID, mergeCommitSHA)
	_, err := db.Exec(ctx, `
		update repositories set main_commit_sha = $1 where id = $2
	`, mergeCommitSHA, repoID)
	if err != nil {
		log.Printf("MergePR: failed to update main_commit_sha: %v", err)
	}
}
