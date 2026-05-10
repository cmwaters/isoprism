package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/isoprism/api/internal/ai"
	"github.com/isoprism/api/internal/github"
	"github.com/isoprism/api/internal/parser"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxPRChangedFiles = 300
	maxPRAdditions    = 20000
	maxPRDeletions    = 20000
	maxPRChangedLines = 30000
)

// OpenPR processes a newly opened or synchronised pull request.
// It computes which functions changed between base and head, generates change
// summaries, and updates pr_node_changes + pr_analyses.
func OpenPR(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, enricher *ai.Enricher, prID string) {
	log.Printf("OpenPR: starting for pr %s", prID)
	stats := newPRProcessingStats()
	markPRProcessing(ctx, db, prID, "running", stats, "")

	// Load PR details
	var repoID, fullName, baseBranch, defaultBranch, mainCommitSHA string
	var installationID int64
	var headSHA, baseSHA, baseCommit string
	var prNumber int
	err := db.QueryRow(ctx, `
		select pr.repo_id, pr.head_commit_sha, pr.base_commit_sha,
		       pr.number, r.full_name, pr.base_branch, r.default_branch, coalesce(r.main_commit_sha, ''),
		       gi.installation_id
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where pr.id = $1
	`, prID).Scan(&repoID, &headSHA, &baseSHA, &prNumber, &fullName, &baseBranch, &defaultBranch, &mainCommitSHA, &installationID)
	if err != nil {
		log.Printf("OpenPR: failed to load PR: %v", err)
		markPRProcessing(ctx, db, prID, "failed", stats, "failed to load PR: "+err.Error())
		return
	}

	if baseBranch != defaultBranch {
		reason := fmt.Sprintf("base branch %q is not indexed default branch %q", baseBranch, defaultBranch)
		log.Printf("OpenPR: skipping pr %s: %s", prID, reason)
		stats.SkipReason = reason
		markPRProcessing(ctx, db, prID, "skipped", stats, reason)
		return
	}

	if headSHA == "" || baseSHA == "" || mainCommitSHA == "" {
		reason := "missing commit SHAs"
		log.Printf("OpenPR: %s for pr %s", reason, prID)
		markPRProcessing(ctx, db, prID, "failed", stats, reason)
		return
	}

	if baseSHA != mainCommitSHA {
		reason := fmt.Sprintf("base sha %s does not match indexed default branch sha %s", baseSHA, mainCommitSHA)
		log.Printf("OpenPR: skipping pr %s: %s", prID, reason)
		stats.SkipReason = reason
		markPRProcessing(ctx, db, prID, "skipped", stats, reason)
		return
	}
	baseCommit = baseSHA

	ghClient, err := appClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("OpenPR: GitHub client error: %v", err)
		markPRProcessing(ctx, db, prID, "failed", stats, "github client error: "+err.Error())
		return
	}

	parts := splitRepo(fullName)
	if parts == nil {
		markPRProcessing(ctx, db, prID, "failed", stats, "invalid repository full name")
		return
	}
	owner, repo := parts[0], parts[1]

	ghPR, err := ghClient.GetPullRequest(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("OpenPR: fetch PR metadata error: %v", err)
		markPRProcessing(ctx, db, prID, "failed", stats, "fetch PR metadata error: "+err.Error())
		return
	}
	stats.GitHubChangedFiles = ghPR.ChangedFiles
	stats.GitHubAdditions = ghPR.Additions
	stats.GitHubDeletions = ghPR.Deletions
	if shouldSkipPRForSize(ghPR) {
		reason := prSizeSkipReason(ghPR)
		log.Printf("OpenPR: skipping pr %s because %s", prID, reason)
		stats.SkipReason = reason
		markPRSkipped(ctx, db, prID, reason, stats)
		return
	}

	// Mark running
	if _, err := db.Exec(ctx, `delete from pr_node_changes where pull_request_id=$1`, prID); err != nil {
		log.Printf("OpenPR: failed to clear stale node changes for pr %s: %v", prID, err)
		markPRProcessing(ctx, db, prID, "failed", stats, "failed to clear stale node changes: "+err.Error())
		return
	}

	// Fetch changed files
	changedFiles, err := ghClient.ListPullRequestFiles(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("OpenPR: PR files error, falling back to compare: %v", err)
		compareFiles, err := ghClient.CompareCommits(ctx, owner, repo, baseCommit, headSHA)
		if err != nil {
			log.Printf("OpenPR: compare error: %v", err)
			markPRProcessing(ctx, db, prID, "failed", stats, "compare error: "+err.Error())
			return
		}
		changedFiles = make([]github.GHPullRequestFile, 0, len(compareFiles))
		for _, file := range compareFiles {
			var previousFilename *string
			if file.PreviousFilename != "" {
				value := file.PreviousFilename
				previousFilename = &value
			}
			var patch *string
			if file.Patch != "" {
				value := file.Patch
				patch = &value
			}
			changedFiles = append(changedFiles, github.GHPullRequestFile{
				Filename:         file.Filename,
				PreviousFilename: previousFilename,
				Status:           file.Status,
				Additions:        file.Additions,
				Deletions:        file.Deletions,
				Changes:          file.Changes,
				Patch:            patch,
			})
		}
	}
	stats.ChangedFiles = len(changedFiles)

	type changedNode struct {
		node        parser.Node
		changeType  string
		diffHunk    string
		oldFullName *string
		oldFilePath *string
		isTest      bool
	}

	var changed []changedNode
	changedFileContents := map[string][]byte{}

	for _, file := range changedFiles {
		headPath := file.Filename
		basePath := file.Filename
		if file.PreviousFilename != nil && *file.PreviousFilename != "" {
			basePath = *file.PreviousFilename
		}
		patch := ""
		if file.Patch != nil {
			patch = *file.Patch
		}
		if !parser.IsSupportedFile(headPath) && !parser.IsSupportedFile(basePath) {
			stats.UnsupportedChangedFiles++
			continue
		}
		stats.SupportedChangedFiles++

		var headNodes []parser.Node
		if file.Status != "removed" && parser.IsSupportedFile(headPath) {
			content, err := ghClient.GetFileContent(ctx, owner, repo, headPath, headSHA)
			if err != nil {
				log.Printf("OpenPR: fetch head file %s: %v", headPath, err)
				stats.HeadFileFetchErrors++
				continue
			}
			stats.HeadFilesFetched++
			changedFileContents[headPath] = content
			headNodes = parser.Parse(content, headPath)
			stats.HeadNodesParsed += len(headNodes)
		}

		// Fetch base version for comparison (if file was modified, not added)
		baseNodesByName := map[string]parser.Node{}
		baseNodesByHash := map[string][]parser.Node{}
		matchedBase := map[string]bool{}
		if file.Status == "modified" || file.Status == "renamed" || file.Status == "removed" {
			baseContent, err := ghClient.GetFileContent(ctx, owner, repo, basePath, baseCommit)
			if err == nil {
				stats.BaseFilesFetched++
				baseNodes := parser.Parse(baseContent, basePath)
				stats.BaseNodesParsed += len(baseNodes)
				for _, n := range baseNodes {
					baseNodesByName[n.FullName] = n
					baseNodesByHash[n.BodyHash] = append(baseNodesByHash[n.BodyHash], n)
				}
			} else {
				log.Printf("OpenPR: fetch base file %s: %v", basePath, err)
				stats.BaseFileFetchErrors++
			}
		}

		for _, n := range headNodes {
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
					    outputs = excluded.outputs,
					    line_start = excluded.line_start,
					    line_end = excluded.line_end
				returning id
			`, repoID, headSHA, n.FullName, n.FilePath,
				n.LineStart, n.LineEnd, inputs, outputs, n.Language, n.Kind, n.BodyHash,
			).Scan(&nodeID)
			if err != nil {
				stats.HeadNodeUpsertErrors++
				continue
			}
			stats.HeadNodesUpserted++

			changeType := "added"
			var baseNode *parser.Node
			if candidate, exists := baseNodesByName[n.FullName]; exists {
				matchedBase[semanticNodeKey(candidate)] = true
				if candidate.BodyHash == n.BodyHash {
					if file.Status != "renamed" || candidate.FilePath == n.FilePath {
						continue // unchanged
					}
					changeType = "renamed"
					baseNode = &candidate
				} else {
					changeType = "modified"
					baseNode = &candidate
				}
			} else if candidate, ok := firstUnmatchedBaseNodeWithHash(baseNodesByHash[n.BodyHash], matchedBase); ok {
				matchedBase[semanticNodeKey(candidate)] = true
				changeType = "renamed"
				baseNode = &candidate
			} else if candidate, ok := firstUnmatchedOverlappingBaseNode(baseNodesByName, matchedBase, n); ok {
				matchedBase[semanticNodeKey(candidate)] = true
				changeType = "renamed"
				baseNode = &candidate
			}

			var oldFullName, oldFilePath *string
			if baseNode != nil && (baseNode.FullName != n.FullName || baseNode.FilePath != n.FilePath) {
				oldFullName = stringPtr(baseNode.FullName)
				oldFilePath = stringPtr(baseNode.FilePath)
				if changeType == "modified" {
					changeType = "renamed"
				}
			}

			oldStart, oldEnd := 0, 0
			if baseNode != nil {
				oldStart, oldEnd = baseNode.LineStart, baseNode.LineEnd
			}
			changed = append(changed, changedNode{
				node:        n,
				changeType:  changeType,
				diffHunk:    componentDiffHunk(changeType, patch, n.Body, oldStart, oldEnd, n.LineStart, n.LineEnd, oldFullName, oldFilePath),
				oldFullName: oldFullName,
				oldFilePath: oldFilePath,
				isTest:      n.IsTestCode,
			})
		}

		// Detect deleted nodes (in base but not in head)
		if file.Status == "modified" || file.Status == "renamed" || file.Status == "removed" {
			for _, baseNode := range baseNodesByName {
				if matchedBase[semanticNodeKey(baseNode)] {
					continue
				}
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
					if nodeID == "" && baseNode.IsTestCode {
						inputs, _ := json.Marshal(baseNode.Inputs)
						outputs, _ := json.Marshal(baseNode.Outputs)
						db.QueryRow(ctx, `
							insert into code_nodes (repo_id, commit_sha, full_name, file_path,
								line_start, line_end, inputs, outputs, language, kind, body_hash)
							values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
							on conflict (repo_id, commit_sha, full_name, file_path) do update
								set body_hash = excluded.body_hash,
								    inputs = excluded.inputs,
								    outputs = excluded.outputs,
								    line_start = excluded.line_start,
								    line_end = excluded.line_end
							returning id
						`, repoID, baseCommit, baseNode.FullName, baseNode.FilePath,
							baseNode.LineStart, baseNode.LineEnd, inputs, outputs, baseNode.Language, baseNode.Kind, baseNode.BodyHash,
						).Scan(&nodeID)
					}
					if nodeID != "" {
						changed = append(changed, changedNode{
							node:       baseNode,
							changeType: "deleted",
							diffHunk:   componentDiffHunk("deleted", patch, baseNode.Body, baseNode.LineStart, baseNode.LineEnd, 0, 0, nil, nil),
							isTest:     baseNode.IsTestCode,
						})
					}
				}
			}
		}
	}

	log.Printf("OpenPR: found %d changed nodes for pr %s", len(changed), prID)
	stats.ChangedNodesDetected = len(changed)
	for _, c := range changed {
		if c.isTest {
			stats.TestNodesDetected++
		}
	}

	// Generate AI change summaries
	var aiInputs []ai.NodeInput
	for _, c := range changed {
		if !c.isTest && c.changeType != "deleted" {
			aiInputs = append(aiInputs, ai.NodeInput{
				FullName: c.node.FullName,
				Body:     c.node.Body,
				DiffHunk: c.diffHunk,
			})
		}
	}

	var changeSummaries map[string]string
	var prOut ai.PROutput

	const maxAIChangedNodes = 80
	if len(aiInputs) > maxAIChangedNodes {
		prOut = ai.PROutput{
			Summary:   "Large PR changing " + strconv.Itoa(len(aiInputs)) + " functions; detailed AI analysis skipped.",
			RiskScore: 5,
			RiskLabel: "medium",
		}
	} else if enricher != nil && len(aiInputs) > 0 {
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
			stats.NodeChangesSkippedMissingNode++
			continue
		}

		var summary *string
		if s, ok := changeSummaries[c.node.FullName]; ok {
			summary = &s
		}
		diffHunk := c.diffHunk

		tag, err := db.Exec(ctx, `
			insert into pr_node_changes (pull_request_id, node_id, change_type, change_summary, diff_hunk, old_full_name, old_file_path)
			values ($1,$2,$3,$4,$5,$6,$7)
			on conflict (pull_request_id, node_id) do update set
				change_type    = excluded.change_type,
				change_summary = excluded.change_summary,
				diff_hunk      = excluded.diff_hunk,
				old_full_name  = excluded.old_full_name,
				old_file_path  = excluded.old_file_path
		`, prID, nodeID, c.changeType, summary, nullIfEmpty(diffHunk), c.oldFullName, c.oldFilePath)
		if err != nil {
			log.Printf("OpenPR: failed to persist node change %s for pr %s: %v", c.node.FullName, prID, err)
			stats.NodeChangePersistErrors++
			continue
		}
		if tag.RowsAffected() > 0 {
			stats.NodeChangesPersisted++
		}
	}

	// Persist pr_analyses
	nodesChanged := 0
	for _, c := range changed {
		if !c.isTest {
			nodesChanged++
		}
	}

	riskScore := prOut.RiskScore
	riskLabel := prOut.RiskLabel
	summary := prOut.Summary
	now := time.Now()
	modelName := "claude-sonnet-4-6"

	if _, err := db.Exec(ctx, `
		insert into pr_analyses (pull_request_id, summary, nodes_changed, risk_score, risk_label, ai_model, generated_at)
		values ($1,$2,$3,$4,$5,$6,$7)
		on conflict (pull_request_id) do update set
			summary       = excluded.summary,
			nodes_changed = excluded.nodes_changed,
			risk_score    = excluded.risk_score,
			risk_label    = excluded.risk_label,
			ai_model      = excluded.ai_model,
			generated_at  = excluded.generated_at
	`, prID, nullIfEmpty(summary), nodesChanged, nullIfZero(riskScore), nullIfEmpty(riskLabel), modelName, now); err != nil {
		log.Printf("OpenPR: failed to persist PR analysis for pr %s: %v", prID, err)
		stats.AnalysisPersistErrors++
	}

	// Build call edges for the PR's changed files (pass full file content per file).
	nodeByName := make(map[string]bool)
	for _, c := range changed {
		if !c.isTest && c.changeType != "deleted" {
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

	resolverFileContents := make(map[string][]byte, len(changedFileContents)+len(fileToNodes))
	for path, content := range changedFileContents {
		resolverFileContents[path] = content
	}
	importDirSuffixes := map[string]bool{}
	for filePath := range fileToNodes {
		content, ok := resolverFileContents[filePath]
		if !ok {
			var err error
			content, err = ghClient.GetFileContent(ctx, owner, repo, filePath, headSHA)
			if err != nil {
				log.Printf("OpenPR: edge extraction fetch %s: %v", filePath, err)
				continue
			}
			resolverFileContents[filePath] = content
		}
		for suffix := range parser.GoImportDirSuffixes(content, filePath) {
			importDirSuffixes[suffix] = true
		}
	}
	if len(importDirSuffixes) > 0 {
		typeRows, _ := db.Query(ctx, `
			select distinct file_path
			from code_nodes
			where repo_id=$1
			  and commit_sha in ($2, $3)
			  and language='go'
			  and kind in ('struct', 'interface', 'type')
		`, repoID, headSHA, baseCommit)
		if typeRows != nil {
			for typeRows.Next() {
				var typeFilePath string
				typeRows.Scan(&typeFilePath)
				if _, exists := resolverFileContents[typeFilePath]; exists || !matchesImportDirSuffix(typeFilePath, importDirSuffixes) {
					continue
				}
				content, err := ghClient.GetFileContent(ctx, owner, repo, typeFilePath, headSHA)
				if err != nil {
					continue
				}
				resolverFileContents[typeFilePath] = content
			}
			typeRows.Close()
		}
	}
	resolverIndex := parser.BuildResolverIndex(resolverFileContents, nodeByName)

	for filePath := range fileToNodes {
		content, ok := resolverFileContents[filePath]
		if !ok {
			var err error
			content, err = ghClient.GetFileContent(ctx, owner, repo, filePath, headSHA)
			if err != nil {
				log.Printf("OpenPR: edge extraction fetch %s: %v", filePath, err)
				continue
			}
		}
		edges := parser.ExtractCallEdgesWithResolver(content, filePath, resolverIndex)
		stats.CallEdgesExtracted += len(edges)
		for _, edge := range edges {
			var callerID, calleeID string
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CallerFullName).Scan(&callerID)
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CalleeFullName).Scan(&calleeID)
			if callerID != "" && calleeID != "" && callerID != calleeID {
				tag, err := db.Exec(ctx, `
					insert into code_edges (repo_id, commit_sha, caller_id, callee_id)
					values ($1,$2,$3,$4)
					on conflict do nothing
				`, repoID, headSHA, callerID, calleeID)
				if err != nil {
					stats.CallEdgePersistErrors++
				} else if tag.RowsAffected() > 0 {
					stats.CallEdgesPersisted++
				}
			}
		}
	}

	insertTestReferences(ctx, db, repoID, headSHA, changedFileContents, nodeByName, refNodeIDs)

	// Mark ready
	var finalErr string
	if stats.ChangedNodesDetected > 0 && stats.NodeChangesPersisted == 0 {
		finalErr = "detected changed nodes but persisted zero pr_node_changes"
		log.Printf("OpenPR: warning for pr %s: %s", prID, finalErr)
	}
	markPRProcessing(ctx, db, prID, "ready", stats, finalErr)
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

func matchesImportDirSuffix(filePath string, suffixes map[string]bool) bool {
	dir := strings.Trim(filepathToSlashDir(filePath), "/")
	for suffix := range suffixes {
		suffix = strings.Trim(suffix, "/")
		if suffix == "" {
			continue
		}
		if dir == suffix || strings.HasSuffix(dir, "/"+suffix) {
			return true
		}
	}
	return false
}

func filepathToSlashDir(filePath string) string {
	if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
		return filePath[:idx]
	}
	return ""
}

func shouldSkipPRForSize(pr *github.GHPullRequest) bool {
	return pr.ChangedFiles > maxPRChangedFiles ||
		pr.Additions > maxPRAdditions ||
		pr.Deletions > maxPRDeletions ||
		pr.Additions+pr.Deletions > maxPRChangedLines
}

func prSizeSkipReason(pr *github.GHPullRequest) string {
	return fmt.Sprintf(
		"PR size exceeds beta processing limits (%d files changed, %d additions, %d deletions; limits: %d files, %d additions, %d deletions, or %d changed lines).",
		pr.ChangedFiles,
		pr.Additions,
		pr.Deletions,
		maxPRChangedFiles,
		maxPRAdditions,
		maxPRDeletions,
		maxPRChangedLines,
	)
}

func markPRSkipped(ctx context.Context, db *pgxpool.Pool, prID, reason string, stats prProcessingStats) {
	now := time.Now()
	_, _ = db.Exec(ctx, `delete from pr_node_changes where pull_request_id=$1`, prID)
	_, _ = db.Exec(ctx, `
		insert into pr_analyses (pull_request_id, summary, nodes_changed, risk_score, risk_label, ai_model, generated_at)
		values ($1,$2,$3,$4,$5,$6,$7)
		on conflict (pull_request_id) do update set
			summary       = excluded.summary,
			nodes_changed = excluded.nodes_changed,
			risk_score    = excluded.risk_score,
			risk_label    = excluded.risk_label,
			ai_model      = excluded.ai_model,
			generated_at  = excluded.generated_at
	`, prID, reason, 0, nil, nil, "pr-size-limit", now)
	markPRProcessing(ctx, db, prID, "skipped", stats, reason)
}

type prProcessingStats struct {
	GitHubChangedFiles            int    `json:"github_changed_files,omitempty"`
	GitHubAdditions               int    `json:"github_additions,omitempty"`
	GitHubDeletions               int    `json:"github_deletions,omitempty"`
	ChangedFiles                  int    `json:"changed_files,omitempty"`
	SupportedChangedFiles         int    `json:"supported_changed_files,omitempty"`
	UnsupportedChangedFiles       int    `json:"unsupported_changed_files,omitempty"`
	HeadFilesFetched              int    `json:"head_files_fetched,omitempty"`
	HeadFileFetchErrors           int    `json:"head_file_fetch_errors,omitempty"`
	BaseFilesFetched              int    `json:"base_files_fetched,omitempty"`
	BaseFileFetchErrors           int    `json:"base_file_fetch_errors,omitempty"`
	HeadNodesParsed               int    `json:"head_nodes_parsed,omitempty"`
	BaseNodesParsed               int    `json:"base_nodes_parsed,omitempty"`
	HeadNodesUpserted             int    `json:"head_nodes_upserted,omitempty"`
	HeadNodeUpsertErrors          int    `json:"head_node_upsert_errors,omitempty"`
	ChangedNodesDetected          int    `json:"changed_nodes_detected,omitempty"`
	TestNodesDetected             int    `json:"test_nodes_detected,omitempty"`
	NodeChangesPersisted          int    `json:"node_changes_persisted,omitempty"`
	NodeChangesSkippedMissingNode int    `json:"node_changes_skipped_missing_node,omitempty"`
	NodeChangePersistErrors       int    `json:"node_change_persist_errors,omitempty"`
	AnalysisPersistErrors         int    `json:"analysis_persist_errors,omitempty"`
	CallEdgesExtracted            int    `json:"call_edges_extracted,omitempty"`
	CallEdgesPersisted            int    `json:"call_edges_persisted,omitempty"`
	CallEdgePersistErrors         int    `json:"call_edge_persist_errors,omitempty"`
	SkipReason                    string `json:"skip_reason,omitempty"`
}

func newPRProcessingStats() prProcessingStats {
	return prProcessingStats{}
}

func markPRProcessing(ctx context.Context, db *pgxpool.Pool, prID, graphStatus string, stats prProcessingStats, processingError string) {
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		log.Printf("OpenPR: failed to marshal processing stats for pr %s: %v", prID, err)
		statsJSON = []byte(`{}`)
	}
	_, err = db.Exec(ctx, `
		update pull_requests set
			graph_status = $1,
			processor_commit_sha = $2,
			processed_at = now(),
			processing_error = $3,
			processing_stats = $4
		where id = $5
	`, graphStatus, currentProcessorCommitSHA(), nullIfEmpty(processingError), statsJSON, prID)
	if err != nil {
		log.Printf("OpenPR: failed to update processing metadata for pr %s: %v", prID, err)
	}
}

func currentProcessorCommitSHA() string {
	for _, key := range []string{"ISOPRISM_COMMIT_SHA", "RAILWAY_GIT_COMMIT_SHA", "VERCEL_GIT_COMMIT_SHA", "GIT_COMMIT_SHA"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "unknown"
}

// componentDiffHunk returns the diff shown for a parsed component. Modified
// components use the GitHub patch filtered to the component range. Added and
// deleted components synthesize a full component hunk so semantic node stats
// count the whole new/removed component, even when Git's file diff treats moved
// body lines as unchanged context.
func componentDiffHunk(changeType, patch, body string, oldStart, oldEnd, newStart, newEnd int, oldFullName, oldFilePath *string) string {
	switch changeType {
	case "added":
		return prefixSourceLines(body, '+')
	case "deleted":
		return prefixSourceLines(body, '-')
	case "renamed":
		if hunk := extractComponentHunk(patch, oldStart, oldEnd, newStart, newEnd); hunk != "" {
			return hunk
		}
		return renameMetadataHunk(oldFullName, oldFilePath)
	default:
		return extractComponentHunk(patch, oldStart, oldEnd, newStart, newEnd)
	}
}

func renameMetadataHunk(oldFullName, oldFilePath *string) string {
	var lines []string
	if oldFilePath != nil && *oldFilePath != "" {
		lines = append(lines, "rename from "+*oldFilePath)
	}
	if oldFullName != nil && *oldFullName != "" {
		lines = append(lines, "rename symbol from "+*oldFullName)
	}
	return strings.Join(lines, "\n")
}

func stringPtr(s string) *string {
	return &s
}

func semanticNodeKey(n parser.Node) string {
	return n.FullName + "|" + n.FilePath
}

func firstUnmatchedBaseNodeWithHash(nodes []parser.Node, matched map[string]bool) (parser.Node, bool) {
	for _, n := range nodes {
		if !matched[semanticNodeKey(n)] {
			return n, true
		}
	}
	return parser.Node{}, false
}

func firstUnmatchedOverlappingBaseNode(nodes map[string]parser.Node, matched map[string]bool, head parser.Node) (parser.Node, bool) {
	for _, n := range nodes {
		if matched[semanticNodeKey(n)] || n.Kind != head.Kind || n.FilePath != head.FilePath {
			continue
		}
		if lineRangesOverlap(n.LineStart, n.LineEnd, head.LineStart, head.LineEnd) {
			return n, true
		}
	}
	return parser.Node{}, false
}

func lineRangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart <= bEnd && bStart <= aEnd
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
