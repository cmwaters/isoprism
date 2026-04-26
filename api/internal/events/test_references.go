package events

import (
	"context"
	"log"

	"github.com/isoprism/api/internal/parser"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertTestReferences(
	ctx context.Context,
	db *pgxpool.Pool,
	repoID string,
	commitSHA string,
	fileContents map[string][]byte,
	nodeByName map[string]bool,
	nodeIDs map[string]string,
) {
	if len(fileContents) == 0 || len(nodeByName) == 0 {
		return
	}

	if _, err := db.Exec(ctx, `delete from code_test_references where repo_id=$1 and commit_sha=$2`, repoID, commitSHA); err != nil {
		log.Printf("test references: failed to clear old rows: %v", err)
	}

	count := 0
	for filePath, content := range fileContents {
		refs := parser.ExtractTestReferences(content, filePath, nodeByName)
		for _, ref := range refs {
			targetID, ok := nodeIDs[ref.TargetFullName]
			if !ok {
				continue
			}
			_, err := db.Exec(ctx, `
				insert into code_test_references (
					repo_id, commit_sha, test_name, test_full_name, test_file_path,
					test_line_start, test_line_end, target_node_id
				)
				values ($1,$2,$3,$4,$5,$6,$7,$8)
				on conflict do nothing
			`, repoID, commitSHA, ref.TestName, ref.TestFullName, ref.TestFilePath,
				ref.LineStart, ref.LineEnd, targetID)
			if err != nil {
				log.Printf("test references: insert %s -> %s: %v", ref.TestFullName, ref.TargetFullName, err)
				continue
			}
			count++
		}
	}
	log.Printf("test references: indexed %d references for %s", count, commitSHA)
}
