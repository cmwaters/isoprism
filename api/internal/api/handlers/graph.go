package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
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

const (
	graphNeighborhoodDepth = 2
	graphMaxVisibleNodes   = 150
)

type graphEdgeRow struct {
	callerID string
	calleeID string
}

type graphCandidate struct {
	id          string
	seed        bool
	lines       int
	callerCount int
	calleeCount int
	degree      int
	depth       int
	boundary    bool
	weight      int
}

type graphNodeRecord struct {
	node      models.GraphNode
	commitSHA string
}

type projectedGraph struct {
	nodes       map[string]models.GraphNode
	edgeRows    []graphEdgeRow
	edges       map[string]models.GraphEdge
	seedIDs     []string
	lineChanges map[string]int
}

func isTestGraphNode(node models.GraphNode) bool {
	normalized := strings.ReplaceAll(strings.ToLower(node.FilePath), "\\", "/")
	base := normalized
	if slash := strings.LastIndex(base, "/"); slash >= 0 {
		base = base[slash+1:]
	}
	ext := ""
	stem := base
	if dot := strings.LastIndex(base, "."); dot >= 0 {
		ext = base[dot:]
		stem = base[:dot]
	}

	if ext == ".go" {
		return strings.HasSuffix(base, "_test.go") || strings.HasPrefix(lastFullNameSegment(node.FullName), "Test")
	}
	if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
		return strings.Contains(normalized, "/__tests__/") ||
			strings.HasSuffix(stem, ".test") ||
			strings.HasSuffix(stem, ".spec")
	}
	return strings.HasPrefix(lastFullNameSegment(node.FullName), "Test")
}

func semanticGraphKey(node models.GraphNode) string {
	return node.FilePath + "|" + node.FullName
}

func lastFullNameSegment(fullName string) string {
	if dot := strings.LastIndex(fullName, "."); dot >= 0 {
		return fullName[dot+1:]
	}
	return fullName
}

func decodeTypeRefs(raw []byte) []models.TypeRef {
	if len(raw) == 0 {
		return []models.TypeRef{}
	}
	var refs []models.TypeRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return []models.TypeRef{}
	}
	return refs
}

func resolveGraphTypeRefs(nodes map[string]models.GraphNode) {
	typeIDByName := map[string]string{}
	for id, node := range nodes {
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		typeIDByName[node.FullName] = id
		typeIDByName[lastFullNameSegment(node.FullName)] = id
	}
	resolve := func(refs []models.TypeRef) []models.TypeRef {
		resolved := make([]models.TypeRef, len(refs))
		copy(resolved, refs)
		for i := range resolved {
			if id, ok := typeIDByName[baseTypeName(resolved[i].Type)]; ok {
				nodeID := id
				resolved[i].NodeID = &nodeID
			}
		}
		return resolved
	}
	for id, node := range nodes {
		node.Inputs = resolve(node.Inputs)
		node.Outputs = resolve(node.Outputs)
		nodes[id] = node
	}
}

func baseTypeName(typeName string) string {
	t := strings.TrimSpace(typeName)
	t = strings.TrimPrefix(t, "*")
	for strings.HasPrefix(t, "[]") {
		t = strings.TrimPrefix(t, "[]")
		t = strings.TrimPrefix(t, "*")
	}
	if strings.HasPrefix(t, "map[") {
		if idx := strings.LastIndex(t, "]"); idx >= 0 && idx+1 < len(t) {
			t = strings.TrimPrefix(t[idx+1:], "*")
		}
	}
	if dot := strings.LastIndex(t, "."); dot >= 0 {
		return t[dot+1:]
	}
	return t
}

func requestedGranularity(r *http.Request, fallback string) string {
	switch strings.ToLower(r.URL.Query().Get("granularity")) {
	case "function", "object", "package":
		return strings.ToLower(r.URL.Query().Get("granularity"))
	default:
		return fallback
	}
}

func packagePathForNode(node models.GraphNode) string {
	if node.PackagePath != "" {
		return node.PackagePath
	}
	path := strings.ReplaceAll(node.FilePath, "\\", "/")
	if path == "" {
		return "."
	}
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		return path[:slash]
	}
	return "."
}

func objectNameForNode(node models.GraphNode) string {
	switch node.Kind {
	case "method":
		if dot := strings.LastIndex(node.FullName, "."); dot > 0 {
			return node.FullName[:dot]
		}
	case "struct", "type", "interface":
		return node.FullName
	}
	return ""
}

func graphNodeRoleRank(role string) int {
	switch role {
	case "changed":
		return 0
	case "entrypoint":
		return 1
	case "caller":
		return 2
	case "callee":
		return 3
	default:
		return 4
	}
}

func mergeGraphNodeRole(current, next string) string {
	if current == "" {
		return next
	}
	if graphNodeRoleRank(next) < graphNodeRoleRank(current) {
		return next
	}
	return current
}

func groupIDForNode(node models.GraphNode, granularity string) string {
	switch granularity {
	case "package":
		return "group:package:" + packagePathForNode(node)
	case "object":
		if objectName := objectNameForNode(node); objectName != "" {
			return "group:object:" + packagePathForNode(node) + ":" + objectName
		}
		return node.ID
	default:
		return node.ID
	}
}

func newAggregateNode(id string, node models.GraphNode, granularity string) models.GraphNode {
	pkg := packagePathForNode(node)
	switch granularity {
	case "package":
		name := pkg
		if name == "." {
			name = "root"
		} else if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}
		summary := "Package containing code nodes from " + pkg + "."
		return models.GraphNode{
			ID:          id,
			FullName:    name,
			FilePath:    pkg,
			PackagePath: pkg,
			LineStart:   node.LineStart,
			LineEnd:     node.LineEnd,
			Inputs:      []models.TypeRef{},
			Outputs:     []models.TypeRef{},
			Language:    node.Language,
			Kind:        "package",
			Granularity: "package",
			NodeType:    node.NodeType,
			Summary:     &summary,
			Tests:       []models.GraphNodeTest{},
			Expandable:  true,
		}
	case "object":
		objectName := objectNameForNode(node)
		summary := "Object containing a type and its associated methods."
		return models.GraphNode{
			ID:          id,
			FullName:    objectName,
			FilePath:    node.FilePath,
			PackagePath: pkg,
			LineStart:   node.LineStart,
			LineEnd:     node.LineEnd,
			Inputs:      []models.TypeRef{},
			Outputs:     []models.TypeRef{},
			Language:    node.Language,
			Kind:        "object",
			Granularity: "object",
			NodeType:    node.NodeType,
			Summary:     &summary,
			Tests:       []models.GraphNodeTest{},
			Expandable:  true,
		}
	default:
		node.Granularity = "function"
		node.PackagePath = pkg
		return node
	}
}

func projectGraph(nodes []models.GraphNode, rawEdges []graphEdgeRow, granularity string) projectedGraph {
	projected := projectedGraph{
		nodes:       map[string]models.GraphNode{},
		edges:       map[string]models.GraphEdge{},
		lineChanges: map[string]int{},
	}
	if granularity == "function" {
		for _, node := range nodes {
			node.Granularity = "function"
			node.PackagePath = packagePathForNode(node)
			node.MemberCount = 1
			node.CollapsedNodeIDs = []string{node.ID}
			projected.nodes[node.ID] = node
			if node.NodeType == "changed" || node.NodeType == "entrypoint" {
				projected.seedIDs = append(projected.seedIDs, node.ID)
			}
			projected.lineChanges[node.ID] = node.LinesAdded + node.LinesRemoved
		}
		for _, edge := range rawEdges {
			if edge.callerID == "" || edge.calleeID == "" || edge.callerID == edge.calleeID {
				continue
			}
			key := edge.callerID + "|" + edge.calleeID
			if _, exists := projected.edges[key]; exists {
				continue
			}
			projected.edgeRows = append(projected.edgeRows, edge)
			projected.edges[key] = models.GraphEdge{CallerID: edge.callerID, CalleeID: edge.calleeID, Weight: 1, UnderlyingEdgeCount: 1}
		}
		return projected
	}

	nodeByID := map[string]models.GraphNode{}
	groupByNodeID := map[string]string{}
	seedSet := map[string]bool{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
		groupID := groupIDForNode(node, granularity)
		groupByNodeID[node.ID] = groupID

		group, exists := projected.nodes[groupID]
		if !exists {
			if groupID == node.ID {
				node.Granularity = "function"
				node.PackagePath = packagePathForNode(node)
				node.MemberCount = 0
				node.CollapsedNodeIDs = []string{node.ID}
				group = node
			} else {
				group = newAggregateNode(groupID, node, granularity)
			}
		}

		group.NodeType = mergeGraphNodeRole(group.NodeType, node.NodeType)
		group.Weight += node.Weight
		group.Degree += node.Degree
		group.LinesAdded += node.LinesAdded
		group.LinesRemoved += node.LinesRemoved
		if node.LineStart > 0 && (group.LineStart == 0 || node.LineStart < group.LineStart) {
			group.LineStart = node.LineStart
		}
		if node.LineEnd > group.LineEnd {
			group.LineEnd = node.LineEnd
		}
		if group.Language == "" {
			group.Language = node.Language
		}
		if group.Summary == nil && node.Summary != nil {
			group.Summary = node.Summary
		}
		group.MemberCount++
		group.CollapsedNodeIDs = append(group.CollapsedNodeIDs, node.ID)
		if node.ChangeType != nil || node.NodeType == "changed" {
			group.ChangedMemberCount++
		}
		group.Tests = append(group.Tests, node.Tests...)
		if node.NodeType == "changed" || node.NodeType == "entrypoint" {
			seedSet[groupID] = true
		}
		projected.nodes[groupID] = group
		projected.lineChanges[groupID] = group.LinesAdded + group.LinesRemoved
	}

	for _, edge := range rawEdges {
		callerGroup := groupByNodeID[edge.callerID]
		calleeGroup := groupByNodeID[edge.calleeID]
		if callerGroup == "" || calleeGroup == "" || callerGroup == calleeGroup {
			continue
		}
		key := callerGroup + "|" + calleeGroup
		agg := projected.edges[key]
		if agg.CallerID == "" {
			agg.CallerID = callerGroup
			agg.CalleeID = calleeGroup
			projected.edgeRows = append(projected.edgeRows, graphEdgeRow{callerID: callerGroup, calleeID: calleeGroup})
		}
		agg.Weight++
		agg.UnderlyingEdgeCount++
		callerNode := nodeByID[edge.callerID]
		calleeNode := nodeByID[edge.calleeID]
		if callerNode.NodeType == "changed" || calleeNode.NodeType == "changed" || callerNode.ChangeType != nil || calleeNode.ChangeType != nil {
			agg.ChangedWeight++
		}
		if len(agg.SampleEdges) < 3 {
			agg.SampleEdges = append(agg.SampleEdges, models.GraphEdgeSample{
				CallerID:   edge.callerID,
				CalleeID:   edge.calleeID,
				CallerName: callerNode.FullName,
				CalleeName: calleeNode.FullName,
			})
		}
		projected.edges[key] = agg
	}

	for id := range seedSet {
		projected.seedIDs = append(projected.seedIDs, id)
	}
	sort.Strings(projected.seedIDs)
	if len(projected.seedIDs) == 0 {
		for id := range projected.nodes {
			projected.seedIDs = append(projected.seedIDs, id)
			break
		}
	}
	return projected
}

func projectedVisibleEdges(edges map[string]models.GraphEdge, selected map[string]graphCandidate) []models.GraphEdge {
	out := make([]models.GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := selected[edge.CallerID]; !ok {
			continue
		}
		if _, ok := selected[edge.CalleeID]; !ok {
			continue
		}
		out = append(out, edge)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CallerID != out[j].CallerID {
			return out[i].CallerID < out[j].CallerID
		}
		return out[i].CalleeID < out[j].CalleeID
	})
	return out
}

func mergeGraphCandidate(current graphCandidate, next graphCandidate, id string) graphCandidate {
	if current.id == "" {
		next.id = id
		return next
	}
	current.seed = current.seed || next.seed
	current.lines += next.lines
	if next.callerCount > current.callerCount {
		current.callerCount = next.callerCount
	}
	if next.calleeCount > current.calleeCount {
		current.calleeCount = next.calleeCount
	}
	if next.degree > current.degree {
		current.degree = next.degree
	}
	if next.depth < current.depth {
		current.depth = next.depth
	}
	current.boundary = current.boundary || next.boundary
	current.weight += next.weight
	current.id = id
	return current
}

func canonicalizeGraphEdges(edges []models.GraphEdge, canonicalizeID func(string) string, selected map[string]graphCandidate) []models.GraphEdge {
	visible := map[string]bool{}
	for id := range selected {
		visible[id] = true
	}

	result := make([]models.GraphEdge, 0, len(edges))
	seen := map[string]bool{}
	for _, edge := range edges {
		source := canonicalizeID(edge.CallerID)
		target := canonicalizeID(edge.CalleeID)
		if source == "" || target == "" || source == target {
			continue
		}
		if !visible[source] || !visible[target] {
			continue
		}
		key := source + "|" + target
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, models.GraphEdge{CallerID: source, CalleeID: target, Weight: 1, UnderlyingEdgeCount: 1})
	}
	return result
}

func selectVisibleGraph(seedIDs []string, allEdges []graphEdgeRow, lineChanges map[string]int) (map[string]graphCandidate, []models.GraphEdge) {
	adj := map[string]map[string]bool{}
	callerCount := map[string]int{}
	calleeCount := map[string]int{}
	knownIDs := map[string]bool{}

	ensure := func(id string) {
		if id == "" {
			return
		}
		knownIDs[id] = true
		if adj[id] == nil {
			adj[id] = map[string]bool{}
		}
	}
	for _, id := range seedIDs {
		ensure(id)
	}
	for _, e := range allEdges {
		if e.callerID == "" || e.calleeID == "" || e.callerID == e.calleeID {
			continue
		}
		ensure(e.callerID)
		ensure(e.calleeID)
		if !adj[e.callerID][e.calleeID] {
			adj[e.callerID][e.calleeID] = true
			adj[e.calleeID][e.callerID] = true
			calleeCount[e.callerID]++
			callerCount[e.calleeID]++
		}
	}

	seedSet := map[string]bool{}
	for _, id := range seedIDs {
		if knownIDs[id] {
			seedSet[id] = true
		}
	}

	depths := map[string]int{}
	queue := make([]string, 0, len(seedSet))
	for id := range seedSet {
		depths[id] = 0
		queue = append(queue, id)
	}
	if len(queue) == 0 {
		for id := range knownIDs {
			depths[id] = 0
			queue = append(queue, id)
			break
		}
	}
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		depth := depths[id]
		if depth >= graphNeighborhoodDepth {
			continue
		}
		for nb := range adj[id] {
			if _, seen := depths[nb]; seen {
				continue
			}
			depths[nb] = depth + 1
			queue = append(queue, nb)
		}
	}

	candidates := make([]graphCandidate, 0, len(depths))
	for id, depth := range depths {
		if depth > graphNeighborhoodDepth {
			continue
		}
		c := graphCandidate{
			id:          id,
			seed:        seedSet[id],
			lines:       lineChanges[id],
			callerCount: callerCount[id],
			calleeCount: calleeCount[id],
			degree:      len(adj[id]),
			depth:       depth,
		}
		c.weight = c.lines + c.callerCount + c.calleeCount
		candidates = append(candidates, c)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.seed != b.seed {
			return a.seed
		}
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		if a.degree != b.degree {
			return a.degree > b.degree
		}
		return a.id < b.id
	})

	selected := map[string]graphCandidate{}
	for _, c := range candidates {
		if len(selected) >= graphMaxVisibleNodes {
			break
		}
		selected[c.id] = c
	}
	for id, c := range selected {
		for nb := range adj[id] {
			if _, ok := selected[nb]; !ok {
				c.boundary = true
				selected[id] = c
				break
			}
		}
	}

	visibleEdges := make([]models.GraphEdge, 0)
	seenEdges := map[string]bool{}
	for _, e := range allEdges {
		if _, ok := selected[e.callerID]; !ok {
			continue
		}
		if _, ok := selected[e.calleeID]; !ok {
			continue
		}
		key := e.callerID + "|" + e.calleeID
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		visibleEdges = append(visibleEdges, models.GraphEdge{CallerID: e.callerID, CalleeID: e.calleeID, Weight: 1, UnderlyingEdgeCount: 1})
	}

	return selected, visibleEdges
}

func sortGraphNodes(nodes []models.GraphNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.GraphDepth != b.GraphDepth {
			return a.GraphDepth < b.GraphDepth
		}
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.Degree != b.Degree {
			return a.Degree > b.Degree
		}
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.LineStart != b.LineStart {
			return a.LineStart < b.LineStart
		}
		return a.ID < b.ID
	})
}

// GET /api/v1/repos/{repoID}/graph
func (h *GraphHandler) GetRepoGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()
	granularity := requestedGranularity(r, "package")

	var repo models.Repository
	err := h.DB.QueryRow(ctx, `
		select id, user_id, installation_id, github_repo_id, full_name,
		       default_branch, main_commit_sha, index_status, is_active, created_at
		from repositories
		where id=$1 and user_id=$2
	`, repoID, userID).Scan(
		&repo.ID, &repo.UserID, &repo.InstallationID, &repo.GitHubRepoID,
		&repo.FullName, &repo.DefaultBranch, &repo.MainCommitSHA,
		&repo.IndexStatus, &repo.IsActive, &repo.CreatedAt,
	)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}
	if repo.MainCommitSHA == nil || *repo.MainCommitSHA == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.RepoGraphResponse{
			Repo:        repo,
			Granularity: granularity,
			Nodes:       []models.GraphNode{},
			Edges:       []models.GraphEdge{},
		})
		return
	}

	nodeRows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, coalesce(summary,'')
		from code_nodes
		where repo_id=$1 and commit_sha=$2
		order by file_path, line_start
	`, repoID, *repo.MainCommitSHA)
	if err != nil {
		log.Printf("GetRepoGraph: node query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer nodeRows.Close()

	nodeMap := map[string]models.GraphNode{}
	seedIDs := make([]string, 0)
	fallbackSeed := ""
	for nodeRows.Next() {
		var n models.GraphNode
		var inputsRaw, outputsRaw []byte
		var summary string
		if err := nodeRows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if summary != "" {
			n.Summary = &summary
		}
		if isTestGraphNode(n) {
			continue
		}
		n.Tests = []models.GraphNodeTest{}
		nodeMap[n.ID] = n
		if fallbackSeed == "" {
			fallbackSeed = n.ID
		}
		if n.FullName == "main" || strings.HasSuffix(n.FullName, ".main") {
			seedIDs = append(seedIDs, n.ID)
		}
	}
	nodeRows.Close()
	if len(seedIDs) == 0 && fallbackSeed != "" {
		seedIDs = append(seedIDs, fallbackSeed)
	}

	allEdges := make([]graphEdgeRow, 0)
	if len(nodeMap) > 0 {
		edgeRows, err := h.DB.Query(ctx, `
			select caller_id, callee_id
			from code_edges
			where repo_id=$1 and commit_sha=$2
		`, repoID, *repo.MainCommitSHA)
		if err != nil {
			log.Printf("GetRepoGraph: edge query: %v", err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer edgeRows.Close()
		for edgeRows.Next() {
			var e graphEdgeRow
			if err := edgeRows.Scan(&e.callerID, &e.calleeID); err != nil {
				continue
			}
			if _, ok := nodeMap[e.callerID]; !ok {
				continue
			}
			if _, ok := nodeMap[e.calleeID]; !ok {
				continue
			}
			allEdges = append(allEdges, e)
		}
		edgeRows.Close()
	}

	selected, edges := selectVisibleGraph(seedIDs, allEdges, map[string]int{})
	allNodes := make([]models.GraphNode, 0, len(nodeMap))
	seedSet := map[string]bool{}
	for _, id := range seedIDs {
		seedSet[id] = true
	}
	for id, n := range nodeMap {
		if seedSet[id] {
			n.NodeType = "entrypoint"
		} else {
			n.NodeType = "context"
		}
		n.Granularity = "function"
		n.PackagePath = packagePathForNode(n)
		nodeMap[id] = n
		allNodes = append(allNodes, n)
	}
	if granularity != "function" {
		projected := projectGraph(allNodes, allEdges, granularity)
		selectedGroups, _ := selectVisibleGraph(projected.seedIDs, projected.edgeRows, projected.lineChanges)
		nodes := make([]models.GraphNode, 0, len(selectedGroups))
		for id, meta := range selectedGroups {
			n, ok := projected.nodes[id]
			if !ok {
				continue
			}
			n.Weight = meta.weight
			n.Degree = meta.degree
			n.GraphDepth = meta.depth
			n.Boundary = meta.boundary
			nodes = append(nodes, n)
		}
		sortGraphNodes(nodes)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.RepoGraphResponse{
			Repo:        repo,
			Granularity: granularity,
			Nodes:       nodes,
			Edges:       projectedVisibleEdges(projected.edges, selectedGroups),
		})
		return
	}

	nodes := make([]models.GraphNode, 0, len(selected))
	nodeIDs := make([]string, 0, len(selected))
	for id, meta := range selected {
		n, ok := nodeMap[id]
		if !ok {
			continue
		}
		if seedSet[id] {
			n.NodeType = "entrypoint"
		} else {
			n.NodeType = "context"
		}
		n.Weight = meta.weight
		n.Degree = meta.degree
		n.GraphDepth = meta.depth
		n.Boundary = meta.boundary
		nodes = append(nodes, n)
		nodeIDs = append(nodeIDs, id)
	}
	resolveGraphTypeRefs(nodeMap)
	for i := range nodes {
		if resolved, ok := nodeMap[nodes[i].ID]; ok {
			nodes[i].Inputs = resolved.Inputs
			nodes[i].Outputs = resolved.Outputs
		}
	}
	sortGraphNodes(nodes)

	if len(nodeIDs) > 0 {
		testRows, err := h.DB.Query(ctx, `
			select target_node_id, test_name, test_full_name, test_file_path, test_line_start, test_line_end
			from code_test_references
			where repo_id=$1
			  and commit_sha=$2
			  and target_node_id = any($3::uuid[])
			order by test_file_path, test_line_start, test_name
		`, repoID, *repo.MainCommitSHA, nodeIDs)
		if err == nil {
			defer testRows.Close()
			nodeIndex := map[string]int{}
			for i := range nodes {
				nodeIndex[nodes[i].ID] = i
			}
			seenTests := map[string]bool{}
			for testRows.Next() {
				var targetID string
				var t models.GraphNodeTest
				if err := testRows.Scan(&targetID, &t.Name, &t.FullName, &t.FilePath, &t.LineStart, &t.LineEnd); err != nil {
					continue
				}
				i, ok := nodeIndex[targetID]
				if !ok {
					continue
				}
				key := targetID + "|" + t.FullName + "|" + t.FilePath
				if seenTests[key] {
					continue
				}
				seenTests[key] = true
				nodes[i].Tests = append(nodes[i].Tests, t)
			}
			testRows.Close()
		} else {
			log.Printf("GetRepoGraph: test references query: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.RepoGraphResponse{
		Repo:        repo,
		Granularity: granularity,
		Nodes:       nodes,
		Edges:       edges,
	})
}

// GET /api/v1/repos/{repoID}/prs/number/{number}/graph
func (h *GraphHandler) GetGraphByNumber(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil {
		http.Error(w, "invalid pr number", http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var prID string
	err = h.DB.QueryRow(ctx, `
		select pr.id
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		where pr.repo_id=$1 and pr.number=$2 and r.user_id=$3
	`, repoID, number, userID).Scan(&prID)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}

	chi.RouteContext(r.Context()).URLParams.Add("prID", prID)
	h.GetGraph(w, r)
}

// GET /api/v1/repos/{repoID}/prs/{prID}/graph
func (h *GraphHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	prID := chi.URLParam(r, "prID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()
	granularity := requestedGranularity(r, "package")

	// Verify repo ownership
	var exists bool
	h.DB.QueryRow(ctx, `select exists(select 1 from repositories where id=$1 and user_id=$2)`, repoID, userID).Scan(&exists)
	if !exists {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	// Load PR + main commit SHA
	var pr models.GraphPR
	var baseCommit, headCommit, mainCommitSHA, baseBranch, defaultBranch, fullName string
	var installationID int64
	err := h.DB.QueryRow(ctx, `
		select pr.id, pr.number, pr.title, pr.html_url,
		       coalesce(pr.base_commit_sha,''), coalesce(pr.head_commit_sha,''),
		       coalesce(r.main_commit_sha,''),
		       coalesce(pr.body,''), coalesce(pr.author_login,''), pr.base_branch, r.default_branch,
		       r.full_name, gi.installation_id
		from pull_requests pr
		join repositories r on r.id = pr.repo_id
		join github_installations gi on gi.id = r.installation_id
		where pr.id=$1 and pr.repo_id=$2
	`, prID, repoID).Scan(&pr.ID, &pr.Number, &pr.Title, &pr.HTMLURL, &baseCommit, &headCommit, &mainCommitSHA,
		&pr.Body, &pr.AuthorLogin, &baseBranch, &defaultBranch, &fullName, &installationID)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}
	pr.BaseCommitSHA = baseCommit
	pr.HeadCommitSHA = headCommit
	files, filesErr := h.loadPRFileDiffs(ctx, fullName, installationID, pr.Number)
	if filesErr != nil {
		log.Printf("GetGraph: PR file diffs unavailable for %s#%d: %v", fullName, pr.Number, filesErr)
		files = []models.PRFileDiff{}
	}
	if baseBranch != defaultBranch {
		http.Error(w, "PR graph only supports pull requests targeting the indexed default branch", http.StatusConflict)
		return
	}
	if baseCommit == "" || mainCommitSHA == "" || baseCommit != mainCommitSHA {
		http.Error(w, "PR base SHA does not match the indexed default branch SHA", http.StatusConflict)
		return
	}

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
			PR:          pr,
			Granularity: granularity,
			Nodes:       []models.GraphNode{},
			Edges:       []models.GraphEdge{},
			Files:       files,
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

	// Find equivalent node IDs at the indexed default-branch commit (for edge lookups).
	// code_edges are stored with main-branch node IDs during repo_init.
	mainIDByFullName := map[string]string{}
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
				mainChangedIDs = append(mainChangedIDs, id)
			}
			mRows.Close()
		}
	}

	// Build lookup IDs: prefer main-branch IDs (so edges resolve), fall back to PR head IDs.
	lookupIDs := make([]string, 0, len(mainChangedIDs)+len(changedIDs))
	lookupIDs = append(lookupIDs, mainChangedIDs...)
	for _, id := range changedIDs {
		if fn, ok := changedIDToFullName[id]; ok {
			if _, hasMain := mainIDByFullName[fn]; !hasMain {
				lookupIDs = append(lookupIDs, id)
			}
		}
	}

	mainIDToPRID := map[string]string{}
	for prID2, fn := range changedIDToFullName {
		if mainID, ok := mainIDByFullName[fn]; ok {
			mainIDToPRID[mainID] = prID2
		}
	}
	remapID := func(id string) string {
		if prEquiv, ok := mainIDToPRID[id]; ok {
			return prEquiv
		}
		return id
	}

	var allEdges []graphEdgeRow
	if len(lookupIDs) > 0 {
		eRows, _ := h.DB.Query(ctx, `
			select caller_id, callee_id
			from code_edges
			where repo_id = $1
			  and (($2 <> '' and commit_sha = $2) or ($3 <> '' and commit_sha = $3))
		`, repoID, mainCommitSHA, headCommit)
		if eRows != nil {
			defer eRows.Close()
			for eRows.Next() {
				var e graphEdgeRow
				eRows.Scan(&e.callerID, &e.calleeID)
				e.callerID = remapID(e.callerID)
				e.calleeID = remapID(e.calleeID)
				if e.callerID == e.calleeID {
					continue
				}
				allEdges = append(allEdges, e)
			}
			eRows.Close()
		}
	}

	lineChanges := map[string]int{}
	for _, id := range changedIDs {
		c := changedSet[id]
		if c.diffHunk != nil {
			added, removed := countDiffLines(*c.diffHunk)
			lineChanges[id] = added + removed
		}
	}
	selected, edges := selectVisibleGraph(changedIDs, allEdges, lineChanges)

	idSet := map[string]bool{}
	for id := range selected {
		idSet[id] = true
	}
	for _, id := range mainChangedIDs {
		idSet[id] = true
	}
	idList := make([]string, 0, len(idSet))
	for id := range idSet {
		idList = append(idList, id)
	}

	nodeRows, err := h.DB.Query(ctx, `
		select id, commit_sha, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, coalesce(summary,'')
		from code_nodes where id = any($1::uuid[])
	`, idList)
	if err != nil {
		log.Printf("GetGraph: node query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer nodeRows.Close()

	nodeMap := map[string]graphNodeRecord{}
	for nodeRows.Next() {
		var n models.GraphNode
		var commitSHA string
		var inputsRaw, outputsRaw []byte
		var summary string
		if err := nodeRows.Scan(
			&n.ID, &commitSHA, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if isTestGraphNode(n) {
			continue
		}
		if summary != "" {
			n.Summary = &summary
		}
		nodeMap[n.ID] = graphNodeRecord{node: n, commitSHA: commitSHA}
	}

	changedIDSet := map[string]bool{}
	for _, id := range changedIDs {
		changedIDSet[id] = true
	}
	canonicalByKey := map[string]string{}
	preferCanonical := func(candidateID, currentID string) bool {
		if currentID == "" {
			return true
		}
		if changedIDSet[candidateID] != changedIDSet[currentID] {
			return changedIDSet[candidateID]
		}
		candidate := nodeMap[candidateID]
		current := nodeMap[currentID]
		if candidate.commitSHA == headCommit && current.commitSHA != headCommit {
			return true
		}
		if current.commitSHA == headCommit && candidate.commitSHA != headCommit {
			return false
		}
		if candidate.commitSHA == mainCommitSHA && current.commitSHA != mainCommitSHA {
			return true
		}
		if current.commitSHA == mainCommitSHA && candidate.commitSHA != mainCommitSHA {
			return false
		}
		return candidateID < currentID
	}
	for id, record := range nodeMap {
		key := semanticGraphKey(record.node)
		if preferCanonical(id, canonicalByKey[key]) {
			canonicalByKey[key] = id
		}
	}
	canonicalByID := map[string]string{}
	for id, record := range nodeMap {
		if canonicalID := canonicalByKey[semanticGraphKey(record.node)]; canonicalID != "" {
			canonicalByID[id] = canonicalID
		}
	}
	canonicalizeID := func(id string) string {
		remapped := remapID(id)
		if canonicalID, ok := canonicalByID[remapped]; ok {
			return canonicalID
		}
		return remapped
	}
	selectedCanonical := map[string]graphCandidate{}
	for id, meta := range selected {
		canonicalID := canonicalizeID(id)
		if _, ok := nodeMap[canonicalID]; !ok {
			continue
		}
		selectedCanonical[canonicalID] = mergeGraphCandidate(selectedCanonical[canonicalID], meta, canonicalID)
	}
	edges = canonicalizeGraphEdges(edges, canonicalizeID, selectedCanonical)

	// Tag node types. For nodes that appear in both PR-head and main-branch forms,
	// prefer the PR-head version (it has change info).
	finalNodeMap := map[string]models.GraphNode{} // keyed by the ID we'll use in the response
	callerSet := map[string]bool{}
	calleeSet := map[string]bool{}
	for _, e := range edges {
		if changedIDSet[e.CalleeID] {
			callerSet[e.CallerID] = true
		}
		if changedIDSet[e.CallerID] {
			calleeSet[e.CalleeID] = true
		}
	}

	// First pass: add changed nodes (PR-head IDs) with change info
	for _, id := range changedIDs {
		canonicalID := canonicalizeID(id)
		meta, isSelected := selectedCanonical[canonicalID]
		if !isSelected {
			continue
		}
		record, ok := nodeMap[canonicalID]
		if !ok {
			continue
		}
		n := record.node
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
		n.Weight = meta.weight
		n.Degree = meta.degree
		n.GraphDepth = meta.depth
		n.Boundary = meta.boundary
		finalNodeMap[canonicalID] = n
	}

	// Second pass: add context nodes (caller/callee) that aren't already included
	for id, meta := range selectedCanonical {
		// Skip if it's a main-branch equivalent of a changed node
		if _, already := finalNodeMap[id]; already {
			continue
		}
		record, ok := nodeMap[id]
		if !ok {
			continue
		}
		n := record.node
		if callerSet[id] {
			n.NodeType = "caller"
		} else {
			n.NodeType = "callee"
		}
		n.Weight = meta.weight
		n.Degree = meta.degree
		n.GraphDepth = meta.depth
		n.Boundary = meta.boundary
		finalNodeMap[id] = n
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
				responseNodeID := canonicalizeID(targetID)
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

	resolveGraphTypeRefs(finalNodeMap)

	nodes := make([]models.GraphNode, 0, len(finalNodeMap))
	for _, n := range finalNodeMap {
		if n.Tests == nil {
			n.Tests = []models.GraphNodeTest{}
		}
		n.Granularity = "function"
		n.PackagePath = packagePathForNode(n)
		nodes = append(nodes, n)
	}
	sortGraphNodes(nodes)

	seenEdges := map[string]bool{}
	for _, e := range edges {
		seenEdges[e.CallerID+"|"+e.CalleeID] = true
	}

	// Add implicit struct → method edges (methods whose full_name = StructName.MethodName)
	for structID, structNode := range finalNodeMap {
		if structNode.Kind != "struct" && structNode.Kind != "type" {
			continue
		}
		prefix := structNode.FullName + "."
		for methodID, methodNode := range finalNodeMap {
			if methodID == structID || methodNode.Kind != "method" {
				continue
			}
			if strings.HasPrefix(methodNode.FullName, prefix) {
				key := structID + "|" + methodID
				if !seenEdges[key] {
					seenEdges[key] = true
					edges = append(edges, models.GraphEdge{CallerID: structID, CalleeID: methodID, Weight: 1, UnderlyingEdgeCount: 1})
				}
			}
		}
	}

	if granularity != "function" {
		rawEdges := make([]graphEdgeRow, 0, len(edges))
		for _, edge := range edges {
			rawEdges = append(rawEdges, graphEdgeRow{callerID: edge.CallerID, calleeID: edge.CalleeID})
		}
		projected := projectGraph(nodes, rawEdges, granularity)
		selectedGroups, _ := selectVisibleGraph(projected.seedIDs, projected.edgeRows, projected.lineChanges)
		nodes = make([]models.GraphNode, 0, len(selectedGroups))
		for id, meta := range selectedGroups {
			n, ok := projected.nodes[id]
			if !ok {
				continue
			}
			n.Weight = meta.weight
			n.Degree = meta.degree
			n.GraphDepth = meta.depth
			n.Boundary = meta.boundary
			nodes = append(nodes, n)
		}
		sortGraphNodes(nodes)
		edges = projectedVisibleEdges(projected.edges, selectedGroups)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.GraphResponse{
		PR:          pr,
		Granularity: granularity,
		Nodes:       nodes,
		Edges:       edges,
		Files:       files,
	})
}

func (h *GraphHandler) loadPRFileDiffs(ctx context.Context, fullName string, installationID int64, prNumber int) ([]models.PRFileDiff, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo name %q", fullName)
	}
	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		return nil, err
	}
	files, err := ghClient.ListPullRequestFiles(ctx, parts[0], parts[1], prNumber)
	if err != nil {
		return nil, err
	}
	out := make([]models.PRFileDiff, 0, len(files))
	for _, file := range files {
		changes := file.Changes
		if changes == 0 {
			changes = file.Additions + file.Deletions
		}
		out = append(out, models.PRFileDiff{
			Filename:         file.Filename,
			PreviousFilename: file.PreviousFilename,
			Status:           file.Status,
			Additions:        file.Additions,
			Deletions:        file.Deletions,
			Changes:          changes,
			Patch:            file.Patch,
		})
	}
	return out, nil
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

// GET /api/v1/repos/{repoID}/nodes/{nodeID}/code
func (h *GraphHandler) GetRepoNodeCode(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	nodeID := chi.URLParam(r, "nodeID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	type nodeMeta struct {
		id        string
		filePath  string
		language  string
		lineStart int
		lineEnd   int
	}

	var fullName string
	var installationID int64
	var mainCommit string
	err := h.DB.QueryRow(ctx, `
		select r.full_name, gi.installation_id, coalesce(r.main_commit_sha, '')
		from repositories r
		join github_installations gi on gi.id = r.installation_id
		where r.id=$1 and r.user_id=$2
	`, repoID, userID).Scan(&fullName, &installationID, &mainCommit)
	if err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	var selected nodeMeta
	err = h.DB.QueryRow(ctx, `
		select id, file_path, language, line_start, line_end
		from code_nodes
		where id=$1 and repo_id=$2
	`, nodeID, repoID).Scan(&selected.id, &selected.filePath, &selected.language, &selected.lineStart, &selected.lineEnd)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid repo name", http.StatusInternalServerError)
		return
	}

	ghClient, err := h.AppClient.ClientForInstallation(ctx, installationID)
	if err != nil {
		log.Printf("GetRepoNodeCode: github client: %v", err)
		http.Error(w, "github client error", http.StatusInternalServerError)
		return
	}

	var head *models.NodeCodeSegment
	if mainCommit != "" {
		content, err := ghClient.GetFileContent(ctx, parts[0], parts[1], selected.filePath, mainCommit)
		if err != nil {
			log.Printf("GetRepoNodeCode: fetch %s@%s: %v", selected.filePath, mainCommit, err)
		} else {
			head = &models.NodeCodeSegment{
				CommitSHA: mainCommit,
				StartLine: selected.lineStart,
				EndLine:   selected.lineEnd,
				Source:    sliceSourceLines(string(content), selected.lineStart, selected.lineEnd),
			}
		}
	}

	response := models.NodeCodeResponse{
		NodeID:   nodeID,
		FilePath: selected.filePath,
		Language: selected.language,
		Head:     head,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
