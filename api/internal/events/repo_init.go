// Package events implements the three backend pipeline events:
// RepoInit, OpenPR, and MergePR.
package events

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/isoprism/api/internal/ai"
	"github.com/isoprism/api/internal/github"
	"github.com/isoprism/api/internal/parser"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RepoInit indexes a repository from the current HEAD of its default branch.
// It runs asynchronously; callers should launch it in a goroutine.
func RepoInit(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, enricher *ai.Enricher, repoID string) {
	log.Printf("RepoInit: starting for repo %s", repoID)

	// Mark running
	if _, err := db.Exec(ctx, `update repositories set index_status='running' where id=$1`, repoID); err != nil {
		log.Printf("RepoInit: failed to mark running: %v", err)
		return
	}

	fail := func(msg string, err error) {
		log.Printf("RepoInit: %s: %v", msg, err)
		db.Exec(ctx, `update repositories set index_status='failed' where id=$1`, repoID)
	}

	// Load repo details
	var installationID int64
	var fullName, defaultBranch string
	err := db.QueryRow(ctx, `
		select gi.installation_id, r.full_name, r.default_branch
		from repositories r
		join github_installations gi on gi.id = r.installation_id
		where r.id = $1
	`, repoID).Scan(&installationID, &fullName, &defaultBranch)
	if err != nil {
		fail("loading repo", err)
		return
	}

	ghClient, err := appClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		fail("getting GitHub client", err)
		return
	}

	parts := splitRepo(fullName)
	if parts == nil {
		fail("invalid repo name", nil)
		return
	}
	owner, repo := parts[0], parts[1]

	// Get HEAD SHA
	headSHA, err := ghClient.GetBranchSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		fail("getting branch SHA", err)
		return
	}
	log.Printf("RepoInit: HEAD=%s", headSHA)

	// Get file tree
	tree, err := ghClient.GetTree(ctx, owner, repo, headSHA)
	if err != nil {
		fail("getting file tree", err)
		return
	}

	// Filter to supported source files
	var sourceFiles []github.GHTreeEntry
	for _, entry := range tree {
		if entry.Type == "blob" && parser.IsSupportedFile(entry.Path) && entry.Size < 500_000 {
			sourceFiles = append(sourceFiles, entry)
		}
	}
	log.Printf("RepoInit: %d source files to parse", len(sourceFiles))

	// Parse files concurrently (bounded pool of 10)
	type fileResult struct {
		nodes   []parser.Node
		path    string
		content []byte
	}
	results := make(chan fileResult, len(sourceFiles))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, entry := range sourceFiles {
		entry := entry
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			content, err := ghClient.GetFileContent(ctx, owner, repo, entry.Path, headSHA)
			if err != nil {
				log.Printf("RepoInit: failed to fetch %s: %v", entry.Path, err)
				return
			}
			nodes := parser.Parse(content, entry.Path)
			if len(nodes) > 0 {
				results <- fileResult{nodes: nodes, path: entry.Path, content: content}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all parsed nodes and file contents
	var allNodes []parser.Node
	fileContents := map[string][]byte{} // filePath → full source
	for fr := range results {
		allNodes = append(allNodes, fr.nodes...)
		fileContents[fr.path] = fr.content
	}
	log.Printf("RepoInit: parsed %d nodes", len(allNodes))

	// Insert code_nodes
	nodeIDs := make(map[string]string) // full_name → db UUID
	for _, n := range allNodes {
		var id string
		err := db.QueryRow(ctx, `
			insert into code_nodes (repo_id, commit_sha, name, full_name, file_path,
				line_start, line_end, signature, language, kind, body_hash)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			on conflict (repo_id, commit_sha, full_name, file_path) do update
				set body_hash = excluded.body_hash
			returning id
		`, repoID, headSHA, n.Name, n.FullName, n.FilePath,
			n.LineStart, n.LineEnd, n.Signature, n.Language, n.Kind, n.BodyHash,
		).Scan(&id)
		if err != nil {
			log.Printf("RepoInit: insert node %s: %v", n.FullName, err)
			continue
		}
		nodeIDs[n.FullName] = id
	}

	// Extract and insert call edges — pass full file content, not per-node snippets.
	// parser.ParseFile requires a complete Go file; a bare function body won't parse.
	nodeByName := make(map[string]bool, len(allNodes))
	for _, n := range allNodes {
		nodeByName[n.FullName] = true
	}

	for filePath, content := range fileContents {
		edges := parser.ExtractCallEdges(content, filePath, nodeByName)
		for _, edge := range edges {
			callerID, ok := nodeIDs[edge.CallerFullName]
			if !ok {
				continue
			}
			calleeID, ok := nodeIDs[edge.CalleeFullName]
			if !ok {
				continue
			}
			db.Exec(ctx, `
				insert into code_edges (repo_id, commit_sha, caller_id, callee_id)
				values ($1,$2,$3,$4)
				on conflict do nothing
			`, repoID, headSHA, callerID, calleeID)
		}
	}
	log.Printf("RepoInit: extracted call edges for %d files", len(fileContents))

	// AI enrichment: generate summaries for all nodes
	if enricher != nil && len(allNodes) > 0 {
		// Batch in groups of 30 to avoid token limits
		const batchSize = 30
		for i := 0; i < len(allNodes); i += batchSize {
			end := i + batchSize
			if end > len(allNodes) {
				end = len(allNodes)
			}
			batch := allNodes[i:end]

			inputs := make([]ai.NodeInput, 0, len(batch))
			for _, n := range batch {
				// Only enrich nodes without an existing summary
				var hasSummary bool
				db.QueryRow(ctx, `select summary is not null from code_nodes where id=$1`, nodeIDs[n.FullName]).Scan(&hasSummary)
				if !hasSummary {
					inputs = append(inputs, ai.NodeInput{
						FullName:  n.FullName,
						Signature: n.Signature,
						Body:      n.Body,
					})
				}
			}
			if len(inputs) == 0 {
				continue
			}

			summaries, err := enricher.EnrichNodes(ctx, inputs)
			if err != nil {
				log.Printf("RepoInit: AI enrichment error: %v", err)
				continue
			}
			for fullName, summary := range summaries {
				id, ok := nodeIDs[fullName]
				if !ok {
					continue
				}
				s := summary
				db.Exec(ctx, `update code_nodes set summary=$1 where id=$2`, s, id)
			}
		}
	}

	// Mark ready
	_, err = db.Exec(ctx, `
		update repositories set index_status='ready', main_commit_sha=$1 where id=$2
	`, headSHA, repoID)
	if err != nil {
		fail("marking ready", err)
		return
	}

	log.Printf("RepoInit: completed for repo %s (%d nodes)", repoID, len(allNodes))
}

func splitRepo(fullName string) []string {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	return parts
}
