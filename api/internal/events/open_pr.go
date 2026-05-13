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
	if err := ReprocessPRGraph(ctx, db, appClient, prID); err != nil {
		log.Printf("OpenPR: graph reprocess stopped for pr %s: %v", prID, err)
		return
	}
	if err := ReprocessPRAI(ctx, db, appClient, enricher, prID); err != nil {
		log.Printf("OpenPR: AI reprocess stopped for pr %s: %v", prID, err)
	}
}

// ReprocessPRGraph rebuilds the structural PR overlay without running AI.
func ReprocessPRGraph(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, prID string) error {
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
		return err
	}

	if baseBranch != defaultBranch {
		reason := fmt.Sprintf("base branch %q is not indexed default branch %q", baseBranch, defaultBranch)
		log.Printf("OpenPR: skipping pr %s: %s", prID, reason)
		stats.SkipReason = reason
		markPRProcessing(ctx, db, prID, "skipped", stats, reason)
		return fmt.Errorf("%s", reason)
	}

	if headSHA == "" || baseSHA == "" || mainCommitSHA == "" {
		reason := "missing commit SHAs"
		log.Printf("OpenPR: %s for pr %s", reason, prID)
		markPRProcessing(ctx, db, prID, "failed", stats, reason)
		return fmt.Errorf("%s", reason)
	}

	if baseSHA != mainCommitSHA {
		reason := fmt.Sprintf("base sha %s does not match indexed default branch sha %s", baseSHA, mainCommitSHA)
		log.Printf("OpenPR: skipping pr %s: %s", prID, reason)
		stats.SkipReason = reason
		markPRProcessing(ctx, db, prID, "skipped", stats, reason)
		return fmt.Errorf("%s", reason)
	}
	baseCommit = baseSHA

	ghClient, err := appClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("OpenPR: GitHub client error: %v", err)
		markPRProcessing(ctx, db, prID, "failed", stats, "github client error: "+err.Error())
		return err
	}

	parts := splitRepo(fullName)
	if parts == nil {
		markPRProcessing(ctx, db, prID, "failed", stats, "invalid repository full name")
		return fmt.Errorf("invalid repository full name")
	}
	owner, repo := parts[0], parts[1]

	ghPR, err := ghClient.GetPullRequest(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("OpenPR: fetch PR metadata error: %v", err)
		markPRProcessing(ctx, db, prID, "failed", stats, "fetch PR metadata error: "+err.Error())
		return err
	}
	stats.GitHubChangedFiles = ghPR.ChangedFiles
	stats.GitHubAdditions = ghPR.Additions
	stats.GitHubDeletions = ghPR.Deletions
	if shouldSkipPRForSize(ghPR) {
		reason := prSizeSkipReason(ghPR)
		log.Printf("OpenPR: skipping pr %s because %s", prID, reason)
		stats.SkipReason = reason
		markPRSkipped(ctx, db, prID, reason, stats)
		return fmt.Errorf("%s", reason)
	}

	existingChangeSummaries := loadExistingPRChangeSummaries(ctx, db, prID)

	// Mark running
	if _, err := db.Exec(ctx, `delete from pr_node_changes where pull_request_id=$1`, prID); err != nil {
		log.Printf("OpenPR: failed to clear stale node changes for pr %s: %v", prID, err)
		markPRProcessing(ctx, db, prID, "failed", stats, "failed to clear stale node changes: "+err.Error())
		return err
	}

	// Fetch changed files
	changedFiles, err := ghClient.ListPullRequestFiles(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("OpenPR: PR files error, falling back to compare: %v", err)
		compareFiles, err := ghClient.CompareCommits(ctx, owner, repo, baseCommit, headSHA)
		if err != nil {
			log.Printf("OpenPR: compare error: %v", err)
			markPRProcessing(ctx, db, prID, "failed", stats, "compare error: "+err.Error())
			return err
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
	var allHeadNodes []parser.Node
	var allBaseNodes []parser.Node

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
			allHeadNodes = append(allHeadNodes, headNodes...)
			stats.HeadNodesParsed += len(headNodes)
		}
		if file.Status != "removed" && parser.IsSupportedFile(headPath) {
			if err := pruneStaleCodeNodesForFile(ctx, db, repoID, headSHA, headPath, parserNodeFullNames(headNodes)); err != nil {
				log.Printf("OpenPR: prune stale head nodes %s@%s: %v", headPath, headSHA, err)
			}
		} else if file.Status == "removed" && parser.IsSupportedFile(headPath) {
			if err := pruneStaleCodeNodesForFile(ctx, db, repoID, headSHA, headPath, nil); err != nil {
				log.Printf("OpenPR: prune removed head nodes %s@%s: %v", headPath, headSHA, err)
			}
		}

		// Load base version metadata from the indexed graph. The base graph was
		// already parsed by RepoInit, so PR processing only fetches head content.
		baseNodesByName := map[string]parser.Node{}
		baseNodesByHash := map[string][]parser.Node{}
		matchedBase := map[string]bool{}
		if file.Status == "modified" || file.Status == "renamed" || file.Status == "removed" {
			baseNodes, err := loadBaseNodesForPath(ctx, db, repoID, baseCommit, basePath)
			if err == nil {
				stats.BaseNodesLoaded += len(baseNodes)
				allBaseNodes = append(allBaseNodes, baseNodes...)
				for _, n := range baseNodes {
					baseNodesByName[n.FullName] = n
					baseNodesByHash[n.BodyHash] = append(baseNodesByHash[n.BodyHash], n)
				}
			} else {
				log.Printf("OpenPR: load base nodes %s@%s: %v", basePath, baseCommit, err)
				stats.BaseNodeLoadErrors++
			}
		}

		for _, n := range headNodes {
			// Insert head node (or update if body changed)
			var nodeID string
			inputs, _ := json.Marshal(n.Inputs)
			outputs, _ := json.Marshal(n.Outputs)
			err := db.QueryRow(ctx, `
				insert into code_nodes (repo_id, commit_sha, full_name, file_path,
					line_start, line_end, inputs, outputs, language, kind, body_hash,
					doc_comment, is_test, is_entrypoint)
				values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
				on conflict (repo_id, commit_sha, full_name, file_path) do update
					set body_hash = excluded.body_hash,
					    inputs = excluded.inputs,
					    outputs = excluded.outputs,
					    line_start = excluded.line_start,
					    line_end = excluded.line_end,
					    doc_comment = excluded.doc_comment,
					    is_test = excluded.is_test,
					    is_entrypoint = excluded.is_entrypoint
				returning id
			`, repoID, headSHA, n.FullName, n.FilePath,
				n.LineStart, n.LineEnd, string(inputs), string(outputs), n.Language, n.Kind, n.BodyHash,
				nullIfEmpty(n.DocComment), n.IsTest, n.IsEntrypoint,
			).Scan(&nodeID)
			if err != nil {
				log.Printf("OpenPR: failed to upsert head node %s for pr %s: %v", n.FullName, prID, err)
				stats.HeadNodeUpsertErrors++
				continue
			}
			stats.HeadNodesUpserted++

			changeType, baseNode, unchanged := classifyHeadNodeChange(n, file.Status, baseNodesByName, baseNodesByHash, matchedBase)
			if unchanged {
				continue
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
				isTest:      n.IsTest,
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
					if nodeID == "" && baseNode.IsTest {
						inputs, _ := json.Marshal(baseNode.Inputs)
						outputs, _ := json.Marshal(baseNode.Outputs)
						db.QueryRow(ctx, `
							insert into code_nodes (repo_id, commit_sha, full_name, file_path,
								line_start, line_end, inputs, outputs, language, kind, body_hash,
								doc_comment, is_test, is_entrypoint)
							values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
							on conflict (repo_id, commit_sha, full_name, file_path) do update
								set body_hash = excluded.body_hash,
								    inputs = excluded.inputs,
								    outputs = excluded.outputs,
								    line_start = excluded.line_start,
								    line_end = excluded.line_end,
								    doc_comment = excluded.doc_comment,
								    is_test = excluded.is_test,
								    is_entrypoint = excluded.is_entrypoint
							returning id
						`, repoID, baseCommit, baseNode.FullName, baseNode.FilePath,
							baseNode.LineStart, baseNode.LineEnd, string(inputs), string(outputs), baseNode.Language, baseNode.Kind, baseNode.BodyHash,
							nullIfEmpty(baseNode.DocComment), baseNode.IsTest, baseNode.IsEntrypoint,
						).Scan(&nodeID)
					}
					if nodeID != "" {
						changed = append(changed, changedNode{
							node:       baseNode,
							changeType: "deleted",
							diffHunk:   componentDiffHunk("deleted", patch, "", baseNode.LineStart, baseNode.LineEnd, 0, 0, nil, nil),
							isTest:     baseNode.IsTest,
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
		if s, ok := existingChangeSummaries[nodeID]; ok {
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

	if _, err := db.Exec(ctx, `
		insert into pr_analyses (pull_request_id, nodes_changed)
		values ($1,$2)
		on conflict (pull_request_id) do update set
			nodes_changed = excluded.nodes_changed
	`, prID, nodesChanged); err != nil {
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
	knownTypeNodes := []parser.Node{}
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
		select full_name, id, kind from code_nodes
		where repo_id=$1 and commit_sha in ($2, $3)
		order by case when commit_sha=$2 then 0 else 1 end
	`, repoID, headSHA, baseCommit)
	if refRows != nil {
		for refRows.Next() {
			var fn, id, kind string
			refRows.Scan(&fn, &id, &kind)
			if _, exists := refNodeIDs[fn]; !exists {
				refNodeIDs[fn] = id
			}
			nodeByName[fn] = true
			if kind == "struct" || kind == "type" || kind == "interface" {
				knownTypeNodes = append(knownTypeNodes, parser.Node{FullName: fn, Kind: kind})
			}
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
			var sourceID, destinationID string
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CallerFullName).Scan(&sourceID)
			db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
				repoID, headSHA, edge.CalleeFullName).Scan(&destinationID)
			if destinationID == "" {
				destinationID = refNodeIDs[edge.CalleeFullName]
			}
			if sourceID != "" && destinationID != "" && sourceID != destinationID {
				tag, err := db.Exec(ctx, `
					insert into code_edges (repo_id, commit_sha, source_id, destination_id, edge_kind)
					values ($1,$2,$3,$4,$5)
					on conflict do nothing
				`, repoID, headSHA, sourceID, destinationID, edgeKindCalls)
				if err != nil {
					stats.CallEdgePersistErrors++
				} else if tag.RowsAffected() > 0 {
					stats.CallEdgesPersisted++
				}
			}
		}
	}
	for _, edge := range semanticTypeEdgesWithKnownTypes(allHeadNodes, append(allHeadNodes, knownTypeNodes...)) {
		var sourceID, destinationID string
		db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
			repoID, headSHA, edge.SourceFullName).Scan(&sourceID)
		db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
			repoID, headSHA, edge.DestinationFullName).Scan(&destinationID)
		if sourceID == "" || destinationID == "" || sourceID == destinationID {
			continue
		}
		if _, err := db.Exec(ctx, `
			insert into code_edges (repo_id, commit_sha, source_id, destination_id, edge_kind)
			values ($1,$2,$3,$4,$5)
			on conflict do nothing
		`, repoID, headSHA, sourceID, destinationID, edge.Kind); err != nil {
			stats.CallEdgePersistErrors++
		}
	}
	for _, edge := range semanticTypeEdgesWithKnownTypes(allBaseNodes, append(allBaseNodes, knownTypeNodes...)) {
		var sourceID, destinationID string
		db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
			repoID, baseCommit, edge.SourceFullName).Scan(&sourceID)
		db.QueryRow(ctx, `select id from code_nodes where repo_id=$1 and commit_sha=$2 and full_name=$3`,
			repoID, baseCommit, edge.DestinationFullName).Scan(&destinationID)
		if sourceID == "" || destinationID == "" || sourceID == destinationID {
			continue
		}
		if _, err := db.Exec(ctx, `
			insert into code_edges (repo_id, commit_sha, source_id, destination_id, edge_kind)
			values ($1,$2,$3,$4,$5)
			on conflict do nothing
		`, repoID, baseCommit, sourceID, destinationID, edge.Kind); err != nil {
			stats.CallEdgePersistErrors++
		}
	}

	// Mark ready
	var finalErr string
	if stats.ChangedNodesDetected > 0 && stats.NodeChangesPersisted == 0 {
		finalErr = "detected changed nodes but persisted zero pr_node_changes"
		log.Printf("OpenPR: warning for pr %s: %s", prID, finalErr)
	} else if stats.HeadNodesParsed > 0 && stats.HeadNodesUpserted == 0 && stats.HeadNodeUpsertErrors > 0 {
		finalErr = "parsed head nodes but failed to upsert all code_nodes"
		log.Printf("OpenPR: warning for pr %s: %s", prID, finalErr)
	}
	markPRProcessing(ctx, db, prID, "ready", stats, finalErr)
	log.Printf("OpenPR: completed for pr %s (%d changed nodes)", prID, nodesChanged)
	return nil
}

// ReprocessPRAI regenerates only AI summaries for an already-built PR graph.
func ReprocessPRAI(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, enricher *ai.Enricher, prID string) error {
	log.Printf("ReprocessPRAI: starting for pr %s", prID)
	if enricher == nil || !enricher.HasAPIKey() {
		log.Printf("ReprocessPRAI: skipping pr %s because GEMINI_API_KEY is not configured", prID)
		return nil
	}

	input, nodeIDsByFullName, testNodeIDsByName, err := buildPRAnalysisInput(ctx, db, appClient, prID)
	if err != nil {
		return err
	}
	totalInputs := len(input.Changes) + len(input.TestChanges)
	const maxAIChangedItems = 80
	if totalInputs > maxAIChangedItems {
		summary := "Large PR changing " + strconv.Itoa(totalInputs) + " components; detailed AI analysis skipped."
		payload, _ := json.Marshal(ai.PRAnalysisOutput{PRSummary: summary, RiskScore: 5})
		if _, err := db.Exec(ctx, `update pr_node_changes set change_summary = null where pull_request_id = $1`, prID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
			insert into pr_analyses (
				pull_request_id, summary, nodes_changed, risk_score, risk_label,
				ai_model, generated_at, analysis_payload, prompt_version
			)
			values ($1,$2,coalesce((select nodes_changed from pr_analyses where pull_request_id=$1),0),$3,null,$4,now(),$5,$6)
			on conflict (pull_request_id) do update set
				summary = excluded.summary,
				risk_score = excluded.risk_score,
				risk_label = null,
				ai_model = excluded.ai_model,
				generated_at = excluded.generated_at,
				analysis_payload = excluded.analysis_payload,
				prompt_version = excluded.prompt_version
		`, prID, summary, 5, "ai-size-limit", string(payload), ai.PromptVersion); err != nil {
			return err
		}
		return nil
	}

	out, err := enricher.EnrichPRChanges(ctx, input)
	if err != nil {
		log.Printf("ReprocessPRAI: AI enrichment error for pr %s: %v", prID, err)
		return persistUnavailablePRAnalysis(ctx, db, prID)
	}
	if strings.TrimSpace(out.PRSummary) == "" && out.RiskScore == 0 {
		return nil
	}

	if _, err := db.Exec(ctx, `update pr_node_changes set change_summary = null where pull_request_id=$1`, prID); err != nil {
		return err
	}
	for fullName, summary := range out.ChangeSummariesByFullName() {
		for _, nodeID := range nodeIDsByFullName[fullName] {
			if _, err := db.Exec(ctx, `
				update pr_node_changes set change_summary=$1
				where pull_request_id=$2 and node_id=$3
			`, summary, prID, nodeID); err != nil {
				return err
			}
		}
	}
	for name, summary := range out.TestAssertionsByName() {
		for _, nodeID := range testNodeIDsByName[name] {
			if _, err := db.Exec(ctx, `
				update pr_node_changes set change_summary=$1
				where pull_request_id=$2 and node_id=$3
			`, summary, prID, nodeID); err != nil {
				return err
			}
		}
	}

	payload, _ := json.Marshal(out)
	if _, err := db.Exec(ctx, `
		insert into pr_analyses (
			pull_request_id, summary, nodes_changed, risk_score, risk_label,
			ai_model, generated_at, analysis_payload, prompt_version
		)
		values ($1,$2,coalesce((select nodes_changed from pr_analyses where pull_request_id=$1),0),$3,null,$4,now(),$5,$6)
		on conflict (pull_request_id) do update set
			summary = excluded.summary,
			risk_score = excluded.risk_score,
			risk_label = null,
			ai_model = excluded.ai_model,
			generated_at = excluded.generated_at,
			analysis_payload = excluded.analysis_payload,
			prompt_version = excluded.prompt_version
	`, prID, out.PRSummary, out.RiskScore, enricher.Model, string(payload), ai.PromptVersion); err != nil {
		return err
	}

	log.Printf("ReprocessPRAI: completed for pr %s", prID)
	return nil
}

func loadExistingPRChangeSummaries(ctx context.Context, db *pgxpool.Pool, prID string) map[string]string {
	summaries := map[string]string{}
	rows, err := db.Query(ctx, `
		select node_id, change_summary
		from pr_node_changes
		where pull_request_id = $1
		  and change_summary is not null
		  and change_summary <> ''
	`, prID)
	if err != nil {
		log.Printf("OpenPR: failed to load existing change summaries for pr %s: %v", prID, err)
		return summaries
	}
	defer rows.Close()

	for rows.Next() {
		var nodeID, summary string
		if err := rows.Scan(&nodeID, &summary); err != nil {
			continue
		}
		summaries[nodeID] = summary
	}
	if err := rows.Err(); err != nil {
		log.Printf("OpenPR: failed reading existing change summaries for pr %s: %v", prID, err)
	}
	return summaries
}

func buildPRAnalysisInput(ctx context.Context, db *pgxpool.Pool, appClient *github.AppClient, prID string) (ai.PRAnalysisInput, map[string][]string, map[string][]string, error) {
	var repoID, fullName string
	var installationID int64
	var prNumber int
	var title, body string
	err := db.QueryRow(ctx, `
		select pr.repo_id, r.full_name, gi.installation_id, pr.number, pr.title, coalesce(pr.body, '')
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where pr.id = $1
	`, prID).Scan(&repoID, &fullName, &installationID, &prNumber, &title, &body)
	if err != nil {
		return ai.PRAnalysisInput{}, nil, nil, fmt.Errorf("load PR for AI: %w", err)
	}

	rows, err := db.Query(ctx, `
		select cn.id, cn.full_name, cn.file_path, cn.is_test, pnc.change_type, coalesce(pnc.diff_hunk, '')
		from pr_node_changes pnc
		join code_nodes cn on cn.id = pnc.node_id
		where pnc.pull_request_id = $1
		order by cn.file_path, cn.line_start, cn.full_name
	`, prID)
	if err != nil {
		return ai.PRAnalysisInput{}, nil, nil, err
	}
	defer rows.Close()

	input := ai.PRAnalysisInput{Title: title, Description: body}
	nodeIDsByFullName := map[string][]string{}
	testNodeIDsByName := map[string][]string{}
	nodeFilePaths := map[string]bool{}
	rowCount := 0
	for rows.Next() {
		var nodeID, fullName, filePath, changeType, diffHunk string
		var isTest bool
		if err := rows.Scan(&nodeID, &fullName, &filePath, &isTest, &changeType, &diffHunk); err != nil {
			return ai.PRAnalysisInput{}, nil, nil, err
		}
		rowCount++
		nodeFilePaths[filePath] = true
		if changeType == "deleted" {
			continue
		}
		if isTest || parser.IsTestFile(filePath) {
			input.TestChanges = append(input.TestChanges, ai.PRTestInput{
				Name:     fullName,
				FilePath: filePath,
				DiffHunk: diffHunk,
			})
			testNodeIDsByName[fullName] = append(testNodeIDsByName[fullName], nodeID)
			continue
		}
		input.Changes = append(input.Changes, ai.PRChangeInput{
			FullName:   fullName,
			FilePath:   filePath,
			ChangeType: changeType,
			DiffHunk:   diffHunk,
		})
		nodeIDsByFullName[fullName] = append(nodeIDsByFullName[fullName], nodeID)
	}
	if err := rows.Err(); err != nil {
		return ai.PRAnalysisInput{}, nil, nil, err
	}
	if rowCount == 0 {
		return ai.PRAnalysisInput{}, nil, nil, fmt.Errorf("PR graph has no persisted node changes; run /debug/prs/%s/reprocess/graph first", prID)
	}

	parts := splitRepo(fullName)
	if parts == nil {
		return ai.PRAnalysisInput{}, nil, nil, fmt.Errorf("invalid repository full name")
	}
	ghClient, err := appClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		return ai.PRAnalysisInput{}, nil, nil, err
	}
	files, err := ghClient.ListPullRequestFiles(ctx, parts[0], parts[1], prNumber)
	if err != nil {
		return ai.PRAnalysisInput{}, nil, nil, err
	}
	for _, file := range files {
		if nodeFilePaths[file.Filename] {
			continue
		}
		if file.Patch == nil || *file.Patch == "" {
			continue
		}
		input.OtherFiles = append(input.OtherFiles, ai.PROtherFileInput{
			Path:     file.Filename,
			Status:   file.Status,
			DiffHunk: *file.Patch,
		})
	}

	return input, nodeIDsByFullName, testNodeIDsByName, nil
}

func persistUnavailablePRAnalysis(ctx context.Context, db *pgxpool.Pool, prID string) error {
	_, err := db.Exec(ctx, `
		insert into pr_analyses (pull_request_id, summary, nodes_changed, risk_score, risk_label, ai_model, generated_at, analysis_payload, prompt_version)
		values ($1,'PR analysis unavailable.',coalesce((select nodes_changed from pr_analyses where pull_request_id=$1),0),null,null,null,null,null,null)
		on conflict (pull_request_id) do update set
			summary = excluded.summary,
			risk_score = null,
			risk_label = null,
			ai_model = null,
			generated_at = null,
			analysis_payload = null,
			prompt_version = null
	`, prID)
	return err
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

func loadBaseNodesForPath(ctx context.Context, db *pgxpool.Pool, repoID, commitSHA, filePath string) ([]parser.Node, error) {
	rows, err := db.Query(ctx, `
		select full_name, file_path, line_start, line_end, inputs, outputs, language, kind, body_hash, coalesce(doc_comment, ''),
		       is_test, is_entrypoint
		from code_nodes
		where repo_id=$1 and commit_sha=$2 and file_path=$3
		order by line_start, full_name
	`, repoID, commitSHA, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []parser.Node{}
	for rows.Next() {
		var n parser.Node
		var inputsRaw, outputsRaw []byte
		var docComment string
		if err := rows.Scan(
			&n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.BodyHash,
			&docComment, &n.IsTest, &n.IsEntrypoint,
		); err != nil {
			return nil, err
		}
		n.Name = leafName(n.FullName)
		n.DocComment = docComment
		n.Inputs = decodeParserParams(inputsRaw)
		n.Outputs = decodeParserParams(outputsRaw)
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func decodeParserParams(raw []byte) []parser.Param {
	if len(raw) == 0 {
		return nil
	}
	var params []parser.Param
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil
	}
	return params
}

func leafName(fullName string) string {
	if idx := strings.LastIndex(fullName, "."); idx >= 0 && idx < len(fullName)-1 {
		return fullName[idx+1:]
	}
	return fullName
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
		insert into pr_analyses (
			pull_request_id, summary, nodes_changed, risk_score, risk_label,
			ai_model, generated_at, analysis_payload, prompt_version
		)
		values ($1,$2,$3,$4,$5,$6,$7,null,null)
		on conflict (pull_request_id) do update set
			summary       = excluded.summary,
			nodes_changed = excluded.nodes_changed,
			risk_score    = excluded.risk_score,
			risk_label    = excluded.risk_label,
			ai_model      = excluded.ai_model,
			generated_at  = excluded.generated_at,
			analysis_payload = null,
			prompt_version = null
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
	BaseNodesLoaded               int    `json:"base_nodes_loaded,omitempty"`
	BaseNodeLoadErrors            int    `json:"base_node_load_errors,omitempty"`
	HeadNodesParsed               int    `json:"head_nodes_parsed,omitempty"`
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
	`, graphStatus, currentProcessorCommitSHA(), nullIfEmpty(processingError), string(statsJSON), prID)
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

// componentDiffHunk returns the diff shown for a parsed component. Modified,
// renamed, and deleted components use the GitHub patch filtered to the component
// range. Added components synthesize a full component hunk from the fetched head
// body so semantic node stats count the whole new component even when Git's file
// diff treats moved/copied body lines as unchanged context.
func componentDiffHunk(changeType, patch, body string, oldStart, oldEnd, newStart, newEnd int, oldFullName, oldFilePath *string) string {
	switch changeType {
	case "added":
		return prefixSourceLines(body, '+')
	case "deleted":
		return extractComponentHunk(patch, oldStart, oldEnd, 0, 0)
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

func parserNodeFullNames(nodes []parser.Node) []string {
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		names = append(names, n.FullName)
	}
	return names
}

func pruneStaleCodeNodesForFile(ctx context.Context, db *pgxpool.Pool, repoID, commitSHA, filePath string, currentFullNames []string) error {
	if len(currentFullNames) == 0 {
		_, err := db.Exec(ctx, `
			delete from code_nodes
			where repo_id=$1 and commit_sha=$2 and file_path=$3
		`, repoID, commitSHA, filePath)
		return err
	}
	_, err := db.Exec(ctx, `
		delete from code_nodes
		where repo_id=$1 and commit_sha=$2 and file_path=$3
		  and not (full_name = any($4))
	`, repoID, commitSHA, filePath, currentFullNames)
	return err
}

func classifyHeadNodeChange(
	head parser.Node,
	fileStatus string,
	baseNodesByName map[string]parser.Node,
	baseNodesByHash map[string][]parser.Node,
	matched map[string]bool,
) (string, *parser.Node, bool) {
	if candidate, exists := baseNodesByName[head.FullName]; exists {
		matched[semanticNodeKey(candidate)] = true
		if candidate.BodyHash == head.BodyHash {
			if fileStatus != "renamed" || candidate.FilePath == head.FilePath {
				return "", nil, true
			}
			return "renamed", &candidate, false
		}
		return "modified", &candidate, false
	}

	if candidate, ok := firstUnmatchedBaseNodeWithHash(baseNodesByHash[head.BodyHash], matched); ok {
		matched[semanticNodeKey(candidate)] = true
		return "renamed", &candidate, false
	}

	return "added", nil, false
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
