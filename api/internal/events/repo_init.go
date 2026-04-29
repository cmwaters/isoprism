// Package events implements the three backend pipeline events:
// RepoInit, OpenPR, and MergePR.
package events

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"sync/atomic"

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
		updateIndexJobFailed(ctx, db, repoID, msg, err)
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
	startIndexJob(ctx, db, repoID, headSHA, "fetching_tree", "Reading repository tree")

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
	updateIndexJobProgress(ctx, db, repoID, headSHA, "fetching_files", "Fetching and parsing source files", len(sourceFiles), 0, 0, 0, 0, 0)

	// Parse files concurrently (bounded pool of 10)
	type fileResult struct {
		nodes   []parser.Node
		path    string
		content []byte
	}
	results := make(chan fileResult, len(sourceFiles))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var filesDone int64

	for _, entry := range sourceFiles {
		entry := entry
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				done := atomic.AddInt64(&filesDone, 1)
				if done == int64(len(sourceFiles)) || done%25 == 0 {
					updateIndexJobProgress(ctx, db, repoID, headSHA, "fetching_files", "Fetching and parsing source files", len(sourceFiles), int(done), 0, 0, 0, 0)
				}
			}()

			content, err := ghClient.GetFileContent(ctx, owner, repo, entry.Path, headSHA)
			if err != nil {
				log.Printf("RepoInit: failed to fetch %s: %v", entry.Path, err)
				return
			}
			nodes := parser.Parse(content, entry.Path)
			if len(nodes) > 0 || parser.IsTestFile(entry.Path) {
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
		for _, n := range fr.nodes {
			if !n.IsTestCode {
				allNodes = append(allNodes, n)
			}
		}
		fileContents[fr.path] = fr.content
	}
	log.Printf("RepoInit: parsed %d nodes", len(allNodes))
	updateIndexJobProgress(ctx, db, repoID, headSHA, "writing_nodes", "Writing code graph nodes", len(sourceFiles), len(sourceFiles), len(allNodes), 0, 0, 0)

	// Insert code_nodes
	nodeIDs := make(map[string]string) // full_name → db UUID
	for i, n := range allNodes {
		var id string
		inputs, _ := json.Marshal(n.Inputs)
		outputs, _ := json.Marshal(n.Outputs)
		err := db.QueryRow(ctx, `
			insert into code_nodes (repo_id, commit_sha, full_name, file_path,
				line_start, line_end, inputs, outputs, language, kind, body_hash)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			on conflict (repo_id, commit_sha, full_name, file_path) do update
				set body_hash = excluded.body_hash,
				    inputs = excluded.inputs,
				    outputs = excluded.outputs
			returning id
		`, repoID, headSHA, n.FullName, n.FilePath,
			n.LineStart, n.LineEnd, inputs, outputs, n.Language, n.Kind, n.BodyHash,
		).Scan(&id)
		if err != nil {
			log.Printf("RepoInit: insert node %s: %v", n.FullName, err)
			continue
		}
		nodeIDs[n.FullName] = id
		if i+1 == len(allNodes) || (i+1)%250 == 0 {
			updateIndexJobProgress(ctx, db, repoID, headSHA, "writing_nodes", "Writing code graph nodes", len(sourceFiles), len(sourceFiles), len(allNodes), i+1, 0, 0)
		}
	}

	// Extract and insert call edges — pass full file content, not per-node snippets.
	// parser.ParseFile requires a complete Go file; a bare function body won't parse.
	nodeByName := make(map[string]bool, len(allNodes))
	for _, n := range allNodes {
		nodeByName[n.FullName] = true
	}

	edgeFilesTotal := len(fileContents)
	edgeFilesDone := 0
	updateIndexJobProgress(ctx, db, repoID, headSHA, "building_edges", "Building call graph edges", len(sourceFiles), len(sourceFiles), len(allNodes), len(allNodes), edgeFilesTotal, 0)
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
		edgeFilesDone++
		if edgeFilesDone == edgeFilesTotal || edgeFilesDone%50 == 0 {
			updateIndexJobProgress(ctx, db, repoID, headSHA, "building_edges", "Building call graph edges", len(sourceFiles), len(sourceFiles), len(allNodes), len(allNodes), edgeFilesTotal, edgeFilesDone)
		}
	}
	log.Printf("RepoInit: extracted call edges for %d files", len(fileContents))

	updateIndexJobProgress(ctx, db, repoID, headSHA, "extracting_tests", "Linking tests to graph nodes", len(sourceFiles), len(sourceFiles), len(allNodes), len(allNodes), edgeFilesTotal, edgeFilesTotal)
	insertTestReferences(ctx, db, repoID, headSHA, fileContents, nodeByName, nodeIDs)

	// Mark the structural graph ready before optional AI enrichment. Large repos can
	// spend a long time in enrichment, but the graph is already usable here.
	_, err = db.Exec(ctx, `
		update repositories set index_status='ready', main_commit_sha=$1 where id=$2
	`, headSHA, repoID)
	if err != nil {
		fail("marking ready", err)
		return
	}
	finishIndexJobReady(ctx, db, repoID, headSHA)
	log.Printf("RepoInit: structural graph ready for repo %s (%d nodes)", repoID, len(allNodes))

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
						FullName: n.FullName,
						Body:     n.Body,
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

	log.Printf("RepoInit: completed for repo %s (%d nodes)", repoID, len(allNodes))
}

func splitRepo(fullName string) []string {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	return parts
}

func startIndexJob(ctx context.Context, db *pgxpool.Pool, repoID, commitSHA, phase, message string) {
	_, _ = db.Exec(ctx, `
		insert into indexing_jobs (repo_id, commit_sha, status, phase, message, started_at, updated_at, finished_at, error)
		values ($1, $2, 'running', $3, $4, now(), now(), null, null)
		on conflict (repo_id, commit_sha) do update set
			status = 'running',
			phase = excluded.phase,
			message = excluded.message,
			started_at = now(),
			updated_at = now(),
			finished_at = null,
			error = null
	`, repoID, commitSHA, phase, message)
}

func updateIndexJobProgress(ctx context.Context, db *pgxpool.Pool, repoID, commitSHA, phase, message string, filesTotal, filesDone, nodesTotal, nodesDone, edgesTotal, edgesDone int) {
	_, _ = db.Exec(ctx, `
		update indexing_jobs
		set phase=$3, message=$4,
		    files_total=$5, files_done=$6,
		    nodes_total=$7, nodes_done=$8,
		    edges_total=$9, edges_done=$10,
		    updated_at=now()
		where repo_id=$1 and commit_sha=$2
	`, repoID, commitSHA, phase, message, filesTotal, filesDone, nodesTotal, nodesDone, edgesTotal, edgesDone)
}

func finishIndexJobReady(ctx context.Context, db *pgxpool.Pool, repoID, commitSHA string) {
	_, _ = db.Exec(ctx, `
		update indexing_jobs
		set status='ready', phase='ready', message='Graph ready',
		    files_done=greatest(files_done, files_total),
		    nodes_done=greatest(nodes_done, nodes_total),
		    edges_done=greatest(edges_done, edges_total),
		    updated_at=now(), finished_at=now(), error=null
		where repo_id=$1 and commit_sha=$2
	`, repoID, commitSHA)
}

func updateIndexJobFailed(ctx context.Context, db *pgxpool.Pool, repoID, msg string, err error) {
	detail := msg
	if err != nil {
		detail = msg + ": " + err.Error()
	}
	_, _ = db.Exec(ctx, `
		update indexing_jobs
		set status='failed', phase='failed', message='Indexing failed',
		    error=$2, updated_at=now(), finished_at=now()
		where id = (
			select id from indexing_jobs
			where repo_id=$1 and status in ('pending', 'running')
			order by created_at desc
			limit 1
		)
	`, repoID, detail)
}
