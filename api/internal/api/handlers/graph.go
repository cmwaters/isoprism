package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/isoprism/api/internal/github"
	"github.com/isoprism/api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphHandler struct {
	DB        *pgxpool.Pool
	AppClient *github.AppClient
}

// GET /api/v1/repos/{repoID}/prs/{prID}/graph
func (h *GraphHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	prID := chi.URLParam(r, "prID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	// Verify repo ownership
	var exists bool
	h.DB.QueryRow(ctx, `select exists(select 1 from repositories where id=$1 and user_id=$2)`, repoID, userID).Scan(&exists)
	if !exists {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	// Load PR + main commit SHA
	var pr models.GraphPR
	var baseCommit, headCommit, mainCommitSHA string
	err := h.DB.QueryRow(ctx, `
		select pr.id, pr.number, pr.title, pr.html_url,
		       coalesce(pr.base_commit_sha,''), coalesce(pr.head_commit_sha,''),
		       coalesce(r.main_commit_sha,''),
		       coalesce(pr.body,''), coalesce(pr.author_login,'')
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		where pr.id=$1 and pr.repo_id=$2
	`, prID, repoID).Scan(&pr.ID, &pr.Number, &pr.Title, &pr.HTMLURL, &baseCommit, &headCommit, &mainCommitSHA,
		&pr.Body, &pr.AuthorLogin)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}
	pr.BaseCommitSHA = baseCommit
	pr.HeadCommitSHA = headCommit

	// Load changed nodes from pr_node_changes
	type rawChange struct {
		nodeID        string
		changeType    string
		changeSummary *string
		diffHunk      *string
	}
	changedRows, err := h.DB.Query(ctx, `
		select pnc.node_id, pnc.change_type, pnc.change_summary, pnc.diff_hunk
		from pr_node_changes pnc
		where pnc.pull_request_id = $1
	`, prID)
	if err != nil {
		log.Printf("GetGraph: pr_node_changes query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer changedRows.Close()

	changedSet := map[string]rawChange{}
	var changedIDs []string
	for changedRows.Next() {
		var c rawChange
		if err := changedRows.Scan(&c.nodeID, &c.changeType, &c.changeSummary, &c.diffHunk); err != nil {
			continue
		}
		changedSet[c.nodeID] = c
		changedIDs = append(changedIDs, c.nodeID)
	}
	changedRows.Close()

	if len(changedIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.GraphResponse{
			PR:    pr,
			Nodes: []models.GraphNode{},
			Edges: []models.GraphEdge{},
		})
		return
	}

	// Get full_names of the changed nodes (stored at PR head SHA)
	changedIDToFullName := map[string]string{}
	changedFullNames := make([]string, 0, len(changedIDs))
	fnRows, _ := h.DB.Query(ctx, `select id, full_name from code_nodes where id = any($1::uuid[])`, changedIDs)
	if fnRows != nil {
		defer fnRows.Close()
		for fnRows.Next() {
			var id, fn string
			fnRows.Scan(&id, &fn)
			changedIDToFullName[id] = fn
			changedFullNames = append(changedFullNames, fn)
		}
		fnRows.Close()
	}

	// Find equivalent node IDs at the main branch commit (for edge lookups).
	// code_edges are stored with main-branch node IDs during repo_init.
	mainIDByFullName := map[string]string{}
	fullNameByMainID := map[string]string{}
	var mainChangedIDs []string
	if mainCommitSHA != "" && len(changedFullNames) > 0 {
		mRows, _ := h.DB.Query(ctx, `
			select id, full_name from code_nodes
			where repo_id=$1 and commit_sha=$2 and full_name = any($3)
		`, repoID, mainCommitSHA, changedFullNames)
		if mRows != nil {
			defer mRows.Close()
			for mRows.Next() {
				var id, fn string
				mRows.Scan(&id, &fn)
				mainIDByFullName[fn] = id
				fullNameByMainID[id] = fn
				mainChangedIDs = append(mainChangedIDs, id)
			}
			mRows.Close()
		}
	}

	// Build lookup IDs: prefer main-branch IDs (so edges resolve), fall back to PR head IDs
	lookupIDs := make([]string, 0, len(mainChangedIDs)+len(changedIDs))
	lookupIDs = append(lookupIDs, mainChangedIDs...)
	// Also include PR-head IDs to catch edges built during open_pr
	for _, id := range changedIDs {
		if fn, ok := changedIDToFullName[id]; ok {
			if _, hasmain := mainIDByFullName[fn]; !hasmain {
				lookupIDs = append(lookupIDs, id)
			}
		}
	}

	// Query call edges touching any of the lookup IDs (no commit_sha filter — edges
	// may exist at either main SHA or PR head SHA depending on when they were built)
	type edgeRow struct{ callerID, calleeID string }
	var allEdges []edgeRow
	callerSet := map[string]bool{}
	calleeSet := map[string]bool{}

	if len(lookupIDs) > 0 {
		eRows, _ := h.DB.Query(ctx, `
			select caller_id, callee_id
			from code_edges
			where repo_id = $1
			  and (caller_id = any($2::uuid[]) or callee_id = any($2::uuid[]))
		`, repoID, lookupIDs)
		if eRows != nil {
			defer eRows.Close()
			for eRows.Next() {
				var e edgeRow
				eRows.Scan(&e.callerID, &e.calleeID)
				allEdges = append(allEdges, e)

				// Determine caller/callee relative to changed nodes.
				// An ID is "changed" if it's a main-branch equivalent of a changed node
				// or directly a PR-head changed node.
				isChanged := func(id string) bool {
					if _, ok := changedSet[id]; ok {
						return true
					}
					if fn, ok := fullNameByMainID[id]; ok {
						for _, cid := range changedIDs {
							if changedIDToFullName[cid] == fn {
								return true
							}
						}
					}
					return false
				}

				if isChanged(e.calleeID) {
					callerSet[e.callerID] = true
				}
				if isChanged(e.callerID) {
					calleeSet[e.calleeID] = true
				}
			}
			eRows.Close()
		}
	}

	// Build the full node ID set (cap at 20).
	// Changed nodes use their PR-head IDs; context nodes use whatever ID the edge returned.
	includedIDs := map[string]bool{}
	for _, id := range changedIDs {
		includedIDs[id] = true
	}
	// Also include main-branch changed IDs so we can resolve context edges
	for _, id := range mainChangedIDs {
		includedIDs[id] = true
	}
	for id := range callerSet {
		if !includedIDs[id] && len(includedIDs) < 20 {
			includedIDs[id] = true
		}
	}
	for id := range calleeSet {
		if !includedIDs[id] && len(includedIDs) < 20 {
			includedIDs[id] = true
		}
	}

	idList := make([]string, 0, len(includedIDs))
	for id := range includedIDs {
		idList = append(idList, id)
	}

	nodeRows, err := h.DB.Query(ctx, `
		select id, name, full_name, file_path, line_start, line_end,
		       signature, language, kind, coalesce(summary,'')
		from code_nodes where id = any($1::uuid[])
	`, idList)
	if err != nil {
		log.Printf("GetGraph: node query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer nodeRows.Close()

	nodeMap := map[string]models.GraphNode{}
	for nodeRows.Next() {
		var n models.GraphNode
		var summary string
		if err := nodeRows.Scan(
			&n.ID, &n.Name, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&n.Signature, &n.Language, &n.Kind, &summary,
		); err != nil {
			continue
		}
		if summary != "" {
			n.Summary = &summary
		}
		nodeMap[n.ID] = n
	}

	// Tag node types. For nodes that appear in both PR-head and main-branch forms,
	// prefer the PR-head version (it has change info).
	finalNodeMap := map[string]models.GraphNode{} // keyed by the ID we'll use in the response

	// First pass: add changed nodes (PR-head IDs) with change info
	for _, id := range changedIDs {
		n, ok := nodeMap[id]
		if !ok {
			continue
		}
		n.NodeType = "changed"
		c := changedSet[id]
		n.ChangeSummary = c.changeSummary
		n.DiffHunk = c.diffHunk
		ct := c.changeType
		n.ChangeType = &ct
		if c.diffHunk != nil {
			added, removed := countDiffLines(*c.diffHunk)
			n.LinesAdded = added
			n.LinesRemoved = removed
		}
		finalNodeMap[id] = n
	}

	// Build a lookup: main-branch ID → PR-head changed node ID (for edge remapping)
	mainIDToPRID := map[string]string{}
	for prID2, fn := range changedIDToFullName {
		if mainID, ok := mainIDByFullName[fn]; ok {
			mainIDToPRID[mainID] = prID2
		}
	}

	// Second pass: add context nodes (caller/callee) that aren't already included
	for id := range includedIDs {
		// Skip if it's a main-branch equivalent of a changed node
		if prEquiv, ok := mainIDToPRID[id]; ok {
			_ = prEquiv
			continue
		}
		if _, already := finalNodeMap[id]; already {
			continue
		}
		n, ok := nodeMap[id]
		if !ok {
			continue
		}
		if callerSet[id] {
			n.NodeType = "caller"
		} else {
			n.NodeType = "callee"
		}
		finalNodeMap[id] = n
	}

	// Remap edges: replace main-branch changed node IDs with their PR-head IDs
	remapID := func(id string) string {
		if prEquiv, ok := mainIDToPRID[id]; ok {
			return prEquiv
		}
		return id
	}

	if len(idList) > 0 {
		testRows, err := h.DB.Query(ctx, `
			select target_node_id, test_name, test_full_name, test_file_path, test_line_start, test_line_end
			from code_test_references
			where repo_id=$1
			  and target_node_id = any($2::uuid[])
			  and (($3 <> '' and commit_sha = $3) or ($4 <> '' and commit_sha = $4))
			order by test_file_path, test_line_start, test_name
		`, repoID, idList, mainCommitSHA, headCommit)
		if err == nil {
			defer testRows.Close()
			seenTests := map[string]bool{}
			for testRows.Next() {
				var targetID string
				var t models.GraphNodeTest
				if err := testRows.Scan(&targetID, &t.Name, &t.FullName, &t.FilePath, &t.LineStart, &t.LineEnd); err != nil {
					continue
				}
				responseNodeID := remapID(targetID)
				n, ok := finalNodeMap[responseNodeID]
				if !ok {
					continue
				}
				key := responseNodeID + "|" + t.FullName + "|" + t.FilePath
				if seenTests[key] {
					continue
				}
				seenTests[key] = true
				n.Tests = append(n.Tests, t)
				finalNodeMap[responseNodeID] = n
			}
			testRows.Close()
		} else {
			log.Printf("GetGraph: test references query: %v", err)
		}
	}

	nodes := make([]models.GraphNode, 0, len(finalNodeMap))
	for _, n := range finalNodeMap {
		if n.Tests == nil {
			n.Tests = []models.GraphNodeTest{}
		}
		nodes = append(nodes, n)
	}

	edges := make([]models.GraphEdge, 0)
	seenEdges := map[string]bool{}
	for _, e := range allEdges {
		callerID := remapID(e.callerID)
		calleeID := remapID(e.calleeID)
		if _, ok := finalNodeMap[callerID]; !ok {
			continue
		}
		if _, ok := finalNodeMap[calleeID]; !ok {
			continue
		}
		key := callerID + "|" + calleeID
		if !seenEdges[key] {
			seenEdges[key] = true
			edges = append(edges, models.GraphEdge{CallerID: callerID, CalleeID: calleeID})
		}
	}

	// Add implicit struct → method edges (methods whose full_name = StructName.MethodName)
	for structID, structNode := range finalNodeMap {
		if structNode.Kind != "struct" && structNode.Kind != "type" {
			continue
		}
		prefix := structNode.Name + "."
		for methodID, methodNode := range finalNodeMap {
			if methodID == structID || methodNode.Kind != "method" {
				continue
			}
			if strings.HasPrefix(methodNode.FullName, prefix) {
				key := structID + "|" + methodID
				if !seenEdges[key] {
					seenEdges[key] = true
					edges = append(edges, models.GraphEdge{CallerID: structID, CalleeID: methodID})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.GraphResponse{
		PR:    pr,
		Nodes: nodes,
		Edges: edges,
	})
}

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

// GET /api/v1/repos/{repoID}/prs/{prID}/nodes/{nodeID}/code
func (h *GraphHandler) GetNodeCode(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	prID := chi.URLParam(r, "prID")
	nodeID := chi.URLParam(r, "nodeID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	type nodeMeta struct {
		id        string
		fullName  string
		filePath  string
		language  string
		lineStart int
		lineEnd   int
	}

	var fullName string
	var installationID int64
	var baseCommit, headCommit string
	err := h.DB.QueryRow(ctx, `
		select r.full_name, gi.installation_id,
		       coalesce(pr.base_commit_sha,''), coalesce(pr.head_commit_sha,'')
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where pr.id=$1 and pr.repo_id=$2 and r.user_id=$3
	`, prID, repoID, userID).Scan(&fullName, &installationID, &baseCommit, &headCommit)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}

	var selected nodeMeta
	err = h.DB.QueryRow(ctx, `
		select id, full_name, file_path, language, line_start, line_end
		from code_nodes
		where id=$1 and repo_id=$2
	`, nodeID, repoID).Scan(&selected.id, &selected.fullName, &selected.filePath, &selected.language, &selected.lineStart, &selected.lineEnd)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	var changeType, diffHunk *string
	_ = h.DB.QueryRow(ctx, `
		select change_type, diff_hunk
		from pr_node_changes
		where pull_request_id=$1 and node_id=$2
	`, prID, nodeID).Scan(&changeType, &diffHunk)

	findNode := func(commitSHA string) *nodeMeta {
		if commitSHA == "" {
			return nil
		}
		var n nodeMeta
		err := h.DB.QueryRow(ctx, `
			select id, full_name, file_path, language, line_start, line_end
			from code_nodes
			where repo_id=$1 and commit_sha=$2 and full_name=$3 and file_path=$4
		`, repoID, commitSHA, selected.fullName, selected.filePath).Scan(&n.id, &n.fullName, &n.filePath, &n.language, &n.lineStart, &n.lineEnd)
		if err != nil {
			return nil
		}
		return &n
	}

	baseNode := findNode(baseCommit)
	headNode := findNode(headCommit)

	if changeType == nil {
		// Context nodes are not in pr_node_changes. Show the head version when present,
		// otherwise fall back to the selected node metadata.
		if headNode == nil {
			headNode = &selected
		}
	}

	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid repo name", http.StatusInternalServerError)
		return
	}

	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("GetNodeCode: github client: %v", err)
		http.Error(w, "github client error", http.StatusInternalServerError)
		return
	}

	fetchSegment := func(n *nodeMeta, commitSHA string) *models.NodeCodeSegment {
		if n == nil || commitSHA == "" {
			return nil
		}
		content, err := ghClient.GetFileContent(ctx, parts[0], parts[1], n.filePath, commitSHA)
		if err != nil {
			log.Printf("GetNodeCode: fetch %s@%s: %v", n.filePath, commitSHA, err)
			return nil
		}
		return &models.NodeCodeSegment{
			CommitSHA: commitSHA,
			StartLine: n.lineStart,
			EndLine:   n.lineEnd,
			Source:    sliceSourceLines(string(content), n.lineStart, n.lineEnd),
		}
	}

	response := models.NodeCodeResponse{
		NodeID:     nodeID,
		FilePath:   selected.filePath,
		Language:   selected.language,
		Base:       fetchSegment(baseNode, baseCommit),
		Head:       fetchSegment(headNode, headCommit),
		DiffHunk:   diffHunk,
		ChangeType: changeType,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sliceSourceLines(source string, startLine, endLine int) string {
	if startLine <= 0 || endLine < startLine {
		return ""
	}
	lines := strings.Split(source, "\n")
	start := startLine - 1
	if start >= len(lines) {
		return ""
	}
	end := endLine
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}
