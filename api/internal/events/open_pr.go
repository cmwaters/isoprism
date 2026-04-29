package events

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/isoprism/api/internal/ai"
	"github.com/isoprism/api/internal/github"
	"github.com/isoprism/api/internal/parser"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPR processes a newly opened or synchronised pull request.
// It computes which functions changed between base and head, generates change
// summaries, and updates pr_node_changes + pr_analyses.
func OpenPR(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, enricher *ai.Enricher, prID string) {
	log.Printf("OpenPR: starting for pr %s", prID)

	// Load PR details
	var repoID, fullName, baseBranch, mainCommitSHA string
	var installationID int64
	var headSHA, baseSHA, baseCommit string
	var prNumber int
	err := db.QueryRow(ctx, `
		select pr.repo_id, pr.head_commit_sha, pr.base_commit_sha,
		       pr.number, r.full_name, pr.base_branch, coalesce(r.main_commit_sha, ''),
		       gi.installation_id
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where pr.id = $1
	`, prID).Scan(&repoID, &headSHA, &baseSHA, &prNumber, &fullName, &baseBranch, &mainCommitSHA, &installationID)
	if err != nil {
		log.Printf("OpenPR: failed to load PR: %v", err)
		return
	}

	if baseBranch != "main" {
		log.Printf("OpenPR: skipping pr %s: base branch %q is not main", prID, baseBranch)
		db.Exec(ctx, `update pull_requests set graph_status='skipped' where id=$1`, prID)
		return
	}

	if headSHA == "" || baseSHA == "" || mainCommitSHA == "" {
		log.Printf("OpenPR: missing commit SHAs for pr %s", prID)
		db.Exec(ctx, `update pull_requests set graph_status='failed' where id=$1`, prID)
		return
	}

	if baseSHA != mainCommitSHA {
		log.Printf("OpenPR: skipping pr %s: base sha %s does not match indexed main sha %s", prID, baseSHA, mainCommitSHA)
		db.Exec(ctx, `update pull_requests set graph_status='skipped' where id=$1`, prID)
		return
	}
	baseCommit = baseSHA

	ghClient, err := appClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("OpenPR: GitHub client error: %v", err)
		db.Exec(ctx, `update pull_requests set graph_status='failed' where id=$1`, prID)
		return
	}

	parts := splitRepo(fullName)
	if parts == nil {
		return
	}
	owner, repo := parts[0], parts[1]

	// Mark running
	db.Exec(ctx, `update pull_requests set graph_status='running' where id=$1`, prID)
	if _, err := db.Exec(ctx, `delete from pr_node_changes where pull_request_id=$1`, prID); err != nil {
		log.Printf("OpenPR: failed to clear stale node changes for pr %s: %v", prID, err)
		db.Exec(ctx, `update pull_requests set graph_status='failed' where id=$1`, prID)
		return
	}

	// Fetch changed files
	changedFiles, err := ghClient.CompareCommits(ctx, owner, repo, baseCommit, headSHA)
	if err != nil {
		log.Printf("OpenPR: compare error: %v", err)
		db.Exec(ctx, `update pull_requests set graph_status='failed' where id=$1`, prID)
		return
	}

	type changedNode struct {
		node       parser.Node
		changeType string
		diffHunk   string
	}

	var changed []changedNode
	changedFileContents := map[string][]byte{}

	for _, file := range changedFiles {
		if !parser.IsSupportedFile(file.Filename) {
			continue
		}

		// Fetch head version of the file
		content, err := ghClient.GetFileContent(ctx, owner, repo, file.Filename, headSHA)
		if err != nil {
			log.Printf("OpenPR: fetch file %s: %v", file.Filename, err)
			continue
		}
		changedFileContents[file.Filename] = content

		headNodes := parser.Parse(content, file.Filename)

		// Fetch base version for comparison (if file was modified, not added)
		baseNodesByName := map[string]parser.Node{}
		if file.Status == "modified" || file.Status == "renamed" {
			baseRef := baseCommit
			basePath := file.Filename
			baseContent, err := ghClient.GetFileContent(ctx, owner, repo, basePath, baseRef)
			if err == nil {
				for _, n := range parser.Parse(baseContent, basePath) {
					if n.IsTestCode {
						continue
					}
					baseNodesByName[n.FullName] = n
				}
			}
		}

		for _, n := range headNodes {
			if n.IsTestCode {
				continue
			}
			// Insert head node (or update if body changed)
			var nodeID string
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
			).Scan(&nodeID)
			if err != nil {
				continue
			}

			changeType := "added"
			var baseNode *parser.Node
			if candidate, exists := baseNodesByName[n.FullName]; exists {
				if candidate.BodyHash == n.BodyHash {
					continue // unchanged
				}
				changeType = "modified"
				baseNode = &candidate
			}

			oldStart, oldEnd := 0, 0
			if baseNode != nil {
				oldStart, oldEnd = baseNode.LineStart, baseNode.LineEnd
			}
			changed = append(changed, changedNode{
				node:       n,
				changeType: changeType,
				diffHunk:   componentDiffHunk(changeType, file.Patch, n.Body, oldStart, oldEnd, n.LineStart, n.LineEnd),
			})
		}

		// Detect deleted nodes (in base but not in head)
		if file.Status == "modified" {
			for _, baseNode := range baseNodesByName {
				found := false
				for _, n := range headNodes {
					if n.FullName == baseNode.FullName {
						found = true
						break
					}
				}
				if !found {
					// Look up existing node ID at base commit
					var nodeID string
					db.QueryRow(ctx, `
						select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3 and file_path=$4
					`, repoID, baseCommit, baseNode.FullName, baseNode.FilePath).Scan(&nodeID)
					if nodeID != "" {
						changed = append(changed, changedNode{
							node:       baseNode,
							changeType: "deleted",
							diffHunk:   componentDiffHunk("deleted", file.Patch, baseNode.Body, baseNode.LineStart, baseNode.LineEnd, 0, 0),
						})
					}
				}
			}
		}
	}

	log.Printf("OpenPR: found %d changed nodes for pr %s", len(changed), prID)

	// Generate AI change summaries
	var aiInputs []ai.NodeInput
	for _, c := range changed {
		if c.changeType != "deleted" {
			aiInputs = append(aiInputs, ai.NodeInput{
				FullName: c.node.FullName,
				Body:     c.node.Body,
				DiffHunk: c.diffHunk,
			})
		}
	}

	var changeSummaries map[string]string
	var prOut ai.PROutput

	if enricher != nil && len(aiInputs) > 0 {
		cs, po, err := enricher.EnrichPRChanges(ctx, aiInputs)
		if err != nil {
			log.Printf("OpenPR: AI enrichment error: %v", err)
		} else {
			changeSummaries = cs
			prOut = po
		}
	}

	// Persist pr_node_changes
	for _, c := range changed {
		var nodeID string
		db.QueryRow(ctx, `
			select id from code_nodes
			where repo_id=$1 and commit_sha=$2 and full_name=$3 and file_path=$4
		`, repoID, headSHA, c.node.FullName, c.node.FilePath).Scan(&nodeID)

		if nodeID == "" && c.changeType == "deleted" {
			db.QueryRow(ctx, `
				select id from code_nodes
				where repo_id=$1 and commit_sha=$2 and full_name=$3 and file_path=$4
			`, repoID, baseCommit, c.node.FullName, c.node.FilePath).Scan(&nodeID)
		}

		if nodeID == "" {
			continue
		}

		var summary *string
		if s, ok := changeSummaries[c.node.FullName]; ok {
			summary = &s
		}
		diffHunk := c.diffHunk

		db.Exec(ctx, `
			insert into pr_node_changes (pull_request_id, node_id, change_type, change_summary, diff_hunk)
			values ($1,$2,$3,$4,$5)
			on conflict (pull_request_id, node_id) do update set
				change_type    = excluded.change_type,
				change_summary = excluded.change_summary,
				diff_hunk      = excluded.diff_hunk
		`, prID, nodeID, c.changeType, summary, nullIfEmpty(diffHunk))
	}

	// Persist pr_analyses
	nodesChanged := 0
	for _, c := range changed {
		if c.changeType != "deleted" {
			nodesChanged++
		}
	}

	riskScore := prOut.RiskScore
	riskLabel := prOut.RiskLabel
	summary := prOut.Summary
	now := time.Now()
	modelName := "claude-sonnet-4-6"

	db.Exec(ctx, `
		insert into pr_analyses (pull_request_id, summary, nodes_changed, risk_score, risk_label, ai_model, generated_at)
		values ($1,$2,$3,$4,$5,$6,$7)
		on conflict (pull_request_id) do update set
			summary       = excluded.summary,
			nodes_changed = excluded.nodes_changed,
			risk_score    = excluded.risk_score,
			risk_label    = excluded.risk_label,
			ai_model      = excluded.ai_model,
			generated_at  = excluded.generated_at
	`, prID, nullIfEmpty(summary), nodesChanged, nullIfZero(riskScore), nullIfEmpty(riskLabel), modelName, now)

	// Build call edges for the PR's changed files (pass full file content per file).
	nodeByName := make(map[string]bool)
	for _, c := range changed {
		if c.changeType != "deleted" {
			nodeByName[c.node.FullName] = true
		}
	}
	// Include all known nodes at this SHA for cross-file resolution
	refNodeIDs := make(map[string]string)
	knownRows, _ := db.Query(ctx, `select full_name from code_nodes where repo_id=$1 and commit_sha=$2`, repoID, headSHA)
	if knownRows != nil {
		for knownRows.Next() {
			var fn string
			knownRows.Scan(&fn)
			nodeByName[fn] = true
		}
		knownRows.Close()
	}
	refRows, _ := db.Query(ctx, `
		select full_name, id from code_nodes
		where repo_id=$1 and commit_sha in ($2, $3)
		order by case when commit_sha=$2 then 0 else 1 end
	`, repoID, headSHA, baseCommit)
	if refRows != nil {
		for refRows.Next() {
			var fn, id string
			refRows.Scan(&fn, &id)
			if _, exists := refNodeIDs[fn]; !exists {
				refNodeIDs[fn] = id
			}
			nodeByName[fn] = true
		}
		refRows.Close()
	}

	// Group changed nodes by file so we fetch each file's content once
	fileToNodes := map[string]bool{}
	for _, c := range changed {
		if c.changeType != "deleted" {
			fileToNodes[c.node.FilePath] = true
		}
	}
	for filePath := range fileToNodes {
		content, err := ghClient.GetFileContent(ctx, owner, repo, filePath, headSHA)
		if err != nil {
			log.Printf("OpenPR: edge extraction fetch %s: %v", filePath, err)
			continue
		}
		edges := parser.ExtractCallEdges(content, filePath, nodeByName)
		for _, edge := range edges {
			var callerID, calleeID string
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CallerFullName).Scan(&callerID)
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CalleeFullName).Scan(&calleeID)
			if callerID != "" && calleeID != "" && callerID != calleeID {
				db.Exec(ctx, `
					insert into code_edges (repo_id, commit_sha, caller_id, callee_id)
					values ($1,$2,$3,$4)
					on conflict do nothing
				`, repoID, headSHA, callerID, calleeID)
			}
		}
	}

	insertTestReferences(ctx, db, repoID, headSHA, changedFileContents, nodeByName, refNodeIDs)

	// Mark ready
	db.Exec(ctx, `update pull_requests set graph_status='ready' where id=$1`, prID)
	log.Printf("OpenPR: completed for pr %s (%d changed nodes)", prID, nodesChanged)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// componentDiffHunk returns the diff shown for a parsed component. Modified
// components use the GitHub patch filtered to the component range. Added and
// deleted components synthesize a full component hunk so semantic node stats
// count the whole new/removed component, even when Git's file diff treats moved
// body lines as unchanged context.
func componentDiffHunk(changeType, patch, body string, oldStart, oldEnd, newStart, newEnd int) string {
	switch changeType {
	case "added":
		return prefixSourceLines(body, '+')
	case "deleted":
		return prefixSourceLines(body, '-')
	default:
		return extractComponentHunk(patch, oldStart, oldEnd, newStart, newEnd)
	}
}

func prefixSourceLines(source string, prefix byte) string {
	if source == "" {
		return ""
	}
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		lines[i] = string(prefix) + line
	}
	return strings.Join(lines, "\n")
}

// extractComponentHunk returns only the diff lines that belong to one parsed
// component. oldStart/oldEnd address the base file; newStart/newEnd address the
// head file. Added components pass only the new range, deleted components pass
// only the old range.
func extractComponentHunk(patch string, oldStart, oldEnd, newStart, newEnd int) string {
	if patch == "" {
		return ""
	}
	lines := strings.Split(patch, "\n")

	oldRange := lineRange{start: oldStart, end: oldEnd}
	newRange := lineRange{start: newStart, end: newEnd}
	var out []string
	oldLine, newLine := 0, 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			oldLine, _, newLine, _ = parseHunkHeader(line)
			continue
		}
		if oldLine == 0 && newLine == 0 {
			continue
		}

		oldCurrent, newCurrent := 0, 0
		kind := byte(' ')
		if line != "" {
			kind = line[0]
		}

		switch kind {
		case '+':
			newCurrent = newLine
			newLine++
		case '-':
			oldCurrent = oldLine
			oldLine++
		default:
			oldCurrent = oldLine
			newCurrent = newLine
			oldLine++
			newLine++
		}

		if !oldRange.contains(oldCurrent) && !newRange.contains(newCurrent) {
			continue
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

type lineRange struct {
	start int
	end   int
}

func (r lineRange) contains(line int) bool {
	return line > 0 && r.start > 0 && r.end >= r.start && line >= r.start && line <= r.end
}

func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int) {
	// Format: @@ -a,b +c,d @@ optional context
	minusIdx := strings.Index(header, "-")
	plusIdx := strings.Index(header, "+")
	if minusIdx < 0 || plusIdx < 0 {
		return 0, 1, 0, 1
	}
	oldStart, oldCount = parseHunkRange(header[minusIdx+1 : plusIdx])
	rest := header[plusIdx+1:]
	end := strings.IndexAny(rest, " @")
	if end > 0 {
		rest = rest[:end]
	}
	newStart, newCount = parseHunkRange(rest)
	return
}

func parseHunkRange(rest string) (start, count int) {
	rest = strings.TrimSpace(rest)
	if ci := strings.Index(rest, ","); ci >= 0 {
		start, _ = strconv.Atoi(rest[:ci])
		count, _ = strconv.Atoi(rest[ci+1:])
	} else {
		start, _ = strconv.Atoi(rest)
		count = 1
	}
	return
}

// countDiffLines counts +/- lines in a unified diff patch.
func countDiffLines(patch string) (added, removed int) {
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}
