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
	graphExpansionMaxNodes = 40
)

type graphEdgeRow struct {
	sourceID          string
	destinationID     string
	sourceName        string
	destinationName   string
	commitSHA         string
	edgeKind          string
	sourceIsTest      bool
	destinationIsTest bool
	changeType        string
}

type graphCandidate struct {
	id               string
	seed             bool
	lines            int
	sourceCount      int
	destinationCount int
	degree           int
	depth            int
	boundary         bool
	weight           int
}

type graphNodeRecord struct {
	node      models.GraphNode
	commitSHA string
}

type rawPRNodeChange struct {
	nodeID        string
	changeType    string
	changeSummary *string
	diffHunk      *string
	oldFullName   *string
	oldFilePath   *string
}

func edgeChangeTypePtr(changeType string) *string {
	if strings.TrimSpace(changeType) == "" || changeType == "unchanged" {
		return nil
	}
	return &changeType
}

func markEdgeChangeType(edge graphEdgeRow, state map[string]bool, changedNames map[string]bool) string {
	if edge.edgeKind != "calls" {
		return "unchanged"
	}
	if !changedNames[edge.sourceName] {
		return "unchanged"
	}
	switch {
	case state["base"] && !state["head"]:
		return "deleted"
	case !state["base"] && state["head"]:
		return "added"
	default:
		return "unchanged"
	}
}

func relevantProductionEdge(edge graphEdgeRow, changedNames map[string]bool) bool {
	if edge.sourceIsTest || edge.destinationIsTest {
		return false
	}
	if edge.edgeKind == "owns_method" {
		return changedNames[edge.sourceName] || changedNames[edge.destinationName]
	}
	if edge.changeType == "added" || edge.changeType == "deleted" {
		return true
	}
	return changedNames[edge.sourceName] || changedNames[edge.destinationName]
}

func isTestGraphNode(node models.GraphNode) bool {
	if node.IsTest {
		return true
	}
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

func applyNodeSummary(node *models.GraphNode, docComment, summary string) {
	docComment = strings.TrimSpace(docComment)
	summary = strings.TrimSpace(summary)
	if docComment != "" {
		node.DocComment = &docComment
	}

	switch {
	case docComment != "" && summary != "":
		combined := docComment + "\n\n" + summary
		node.Summary = &combined
	case docComment != "":
		node.Summary = &docComment
	case summary != "":
		node.Summary = &summary
	}
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

func (h *GraphHandler) attachTestsFromEdges(ctx context.Context, repoID string, targetIDs []string, commitSHAs []string, canonicalizeID func(string) string, nodes []models.GraphNode) {
	if len(targetIDs) == 0 || len(commitSHAs) == 0 {
		return
	}
	nodeIndex := map[string]int{}
	for i := range nodes {
		nodeIndex[nodes[i].ID] = i
	}
	rows, err := h.DB.Query(ctx, `
		with recursive reachable(target_id, current_id, depth) as (
			select e.destination_id, e.source_id, 1
			from code_edges e
			where e.repo_id = $1
			  and e.commit_sha = any($2)
			  and e.destination_id = any($3::uuid[])
			  and e.edge_kind = 'calls'
			union
			select r.target_id, e.source_id, r.depth + 1
			from reachable r
			join code_edges e on e.destination_id = r.current_id
			where e.repo_id = $1
			  and e.commit_sha = any($2)
			  and e.edge_kind = 'calls'
			  and r.depth < 8
		)
		select r.target_id, t.full_name, t.file_path, t.line_start, t.line_end
		from reachable r
		join code_nodes t on t.id = r.current_id
		where t.is_test = true
		  and t.is_entrypoint = true
		order by t.file_path, t.line_start, t.full_name
	`, repoID, commitSHAs, targetIDs)
	if err != nil {
		log.Printf("test edges query: %v", err)
		return
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var targetID string
		var t models.GraphNodeTest
		if err := rows.Scan(&targetID, &t.FullName, &t.FilePath, &t.LineStart, &t.LineEnd); err != nil {
			continue
		}
		t.Name = lastFullNameSegment(t.FullName)
		responseNodeID := canonicalizeID(targetID)
		i, ok := nodeIndex[responseNodeID]
		if !ok {
			continue
		}
		key := responseNodeID + "|" + t.FullName + "|" + t.FilePath
		if seen[key] {
			continue
		}
		seen[key] = true
		nodes[i].Tests = append(nodes[i].Tests, t)
	}
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func mergeGraphCandidate(current graphCandidate, next graphCandidate, id string) graphCandidate {
	if current.id == "" {
		next.id = id
		return next
	}
	current.seed = current.seed || next.seed
	current.lines += next.lines
	if next.sourceCount > current.sourceCount {
		current.sourceCount = next.sourceCount
	}
	if next.destinationCount > current.destinationCount {
		current.destinationCount = next.destinationCount
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
		source := canonicalizeID(edge.SourceID)
		target := canonicalizeID(edge.DestinationID)
		if source == "" || target == "" || source == target {
			continue
		}
		if !visible[source] || !visible[target] {
			continue
		}
		key := source + "|" + target + "|" + edge.EdgeKind
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, models.GraphEdge{SourceID: source, DestinationID: target, EdgeKind: edge.EdgeKind, ChangeType: edge.ChangeType, Weight: edge.Weight, UnderlyingEdgeCount: edge.UnderlyingEdgeCount})
	}
	return result
}

func appendTestFocusEdges(
	edges []models.GraphEdge,
	allEdges []graphEdgeRow,
	testChanges []models.GraphNode,
	finalNodeMap map[string]models.GraphNode,
	testContext map[string]models.GraphNode,
	canonicalizeID func(string) string,
) []models.GraphEdge {
	testIDs := map[string]bool{}
	for _, n := range testChanges {
		testIDs[n.ID] = true
	}
	if len(testIDs) == 0 {
		return edges
	}

	visibleProductionIDs := map[string]bool{}
	for id := range finalNodeMap {
		visibleProductionIDs[id] = true
	}
	testContextIDs := map[string]bool{}
	for id := range testContext {
		testContextIDs[id] = true
	}

	seen := map[string]bool{}
	for _, e := range edges {
		seen[e.SourceID+"|"+e.DestinationID+"|"+e.EdgeKind] = true
	}
	for _, e := range allEdges {
		if e.edgeKind != "calls" {
			continue
		}
		source := canonicalizeID(e.sourceID)
		target := canonicalizeID(e.destinationID)
		if source == "" || target == "" || source == target || !testIDs[source] {
			continue
		}
		if !testIDs[target] && !visibleProductionIDs[target] && !testContextIDs[target] {
			continue
		}
		key := source + "|" + target + "|" + e.edgeKind
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, models.GraphEdge{SourceID: source, DestinationID: target, EdgeKind: e.edgeKind, ChangeType: edgeChangeTypePtr(e.changeType), Weight: 1, UnderlyingEdgeCount: 1})
	}
	return edges
}

func selectRankedVisibleGraph(seedIDs []string, allEdges []graphEdgeRow, lineChanges map[string]int) (map[string]graphCandidate, []models.GraphEdge) {
	adj := map[string]map[string]bool{}
	sourceCount := map[string]int{}
	destinationCount := map[string]int{}
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
		if e.sourceID == "" || e.destinationID == "" || e.sourceID == e.destinationID {
			continue
		}
		ensure(e.sourceID)
		ensure(e.destinationID)
		if !adj[e.sourceID][e.destinationID] {
			adj[e.sourceID][e.destinationID] = true
			adj[e.destinationID][e.sourceID] = true
			destinationCount[e.sourceID]++
			sourceCount[e.destinationID]++
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
			id:               id,
			seed:             seedSet[id],
			lines:            lineChanges[id],
			sourceCount:      sourceCount[id],
			destinationCount: destinationCount[id],
			degree:           len(adj[id]),
			depth:            depth,
		}
		c.weight = c.lines + c.sourceCount + c.destinationCount
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
		if _, ok := selected[e.sourceID]; !ok {
			continue
		}
		if _, ok := selected[e.destinationID]; !ok {
			continue
		}
		key := e.sourceID + "|" + e.destinationID + "|" + e.edgeKind
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		visibleEdges = append(visibleEdges, models.GraphEdge{SourceID: e.sourceID, DestinationID: e.destinationID, EdgeKind: e.edgeKind, ChangeType: edgeChangeTypePtr(e.changeType), Weight: 1, UnderlyingEdgeCount: 1})
	}

	return selected, visibleEdges
}

func selectVisibleGraph(seedIDs []string, allEdges []graphEdgeRow, lineChanges map[string]int) (map[string]graphCandidate, []models.GraphEdge) {
	adj := map[string]map[string]bool{}
	sourceCount := map[string]int{}
	destinationCount := map[string]int{}
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
	seedSet := map[string]bool{}
	for _, id := range seedIDs {
		ensure(id)
		seedSet[id] = true
	}
	for _, e := range allEdges {
		if e.sourceID == "" || e.destinationID == "" || e.sourceID == e.destinationID {
			continue
		}
		ensure(e.sourceID)
		ensure(e.destinationID)
		if !adj[e.sourceID][e.destinationID] {
			adj[e.sourceID][e.destinationID] = true
			adj[e.destinationID][e.sourceID] = true
			destinationCount[e.sourceID]++
			sourceCount[e.destinationID]++
		}
	}

	selected := map[string]graphCandidate{}
	for _, id := range seedIDs {
		if !knownIDs[id] {
			continue
		}
		selected[id] = graphCandidate{
			id:               id,
			seed:             true,
			lines:            lineChanges[id],
			sourceCount:      sourceCount[id],
			destinationCount: destinationCount[id],
			degree:           len(adj[id]),
			depth:            0,
			weight:           lineChanges[id] + sourceCount[id] + destinationCount[id],
		}
	}
	for _, e := range allEdges {
		sourceSeed := seedSet[e.sourceID]
		targetSeed := seedSet[e.destinationID]
		if !sourceSeed && !targetSeed {
			continue
		}
		for _, id := range []string{e.sourceID, e.destinationID} {
			if selected[id].id != "" {
				continue
			}
			depth := 1
			if seedSet[id] {
				depth = 0
			}
			selected[id] = graphCandidate{
				id:               id,
				seed:             seedSet[id],
				lines:            lineChanges[id],
				sourceCount:      sourceCount[id],
				destinationCount: destinationCount[id],
				degree:           len(adj[id]),
				depth:            depth,
				weight:           lineChanges[id] + sourceCount[id] + destinationCount[id],
			}
		}
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
		if _, ok := selected[e.sourceID]; !ok {
			continue
		}
		if _, ok := selected[e.destinationID]; !ok {
			continue
		}
		key := e.sourceID + "|" + e.destinationID + "|" + e.edgeKind
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		visibleEdges = append(visibleEdges, models.GraphEdge{SourceID: e.sourceID, DestinationID: e.destinationID, EdgeKind: e.edgeKind, ChangeType: edgeChangeTypePtr(e.changeType), Weight: 1, UnderlyingEdgeCount: 1})
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

func graphVisibleSet(ids []string, expandedID string) map[string]bool {
	visible := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			visible[id] = true
		}
	}
	if strings.TrimSpace(expandedID) != "" {
		visible[expandedID] = true
	}
	return visible
}

func graphDegreeByNode(edges []graphEdgeRow) map[string]int {
	neighbors := map[string]map[string]bool{}
	ensure := func(id string) {
		if neighbors[id] == nil {
			neighbors[id] = map[string]bool{}
		}
	}
	for _, edge := range edges {
		if edge.sourceID == "" || edge.destinationID == "" || edge.sourceID == edge.destinationID {
			continue
		}
		ensure(edge.sourceID)
		ensure(edge.destinationID)
		neighbors[edge.sourceID][edge.destinationID] = true
		neighbors[edge.destinationID][edge.sourceID] = true
	}
	degrees := map[string]int{}
	for id, adjacent := range neighbors {
		degrees[id] = len(adjacent)
	}
	return degrees
}

func graphHasHiddenNeighbor(id string, visible map[string]bool, edges []graphEdgeRow) bool {
	for _, edge := range edges {
		if edge.sourceID == id && !visible[edge.destinationID] {
			return true
		}
		if edge.destinationID == id && !visible[edge.sourceID] {
			return true
		}
	}
	return false
}

func expansionNodeType(id, expandedID string, edges []graphEdgeRow, node models.GraphNode) string {
	if node.ChangeType != nil {
		return "changed"
	}
	callsExpanded := false
	calledByExpanded := false
	for _, edge := range edges {
		if edge.sourceID == id && edge.destinationID == expandedID {
			callsExpanded = true
		}
		if edge.sourceID == expandedID && edge.destinationID == id {
			calledByExpanded = true
		}
	}
	if callsExpanded && !calledByExpanded {
		return "caller"
	}
	if calledByExpanded && !callsExpanded {
		return "callee"
	}
	return "context"
}

func selectExpansionNeighbors(expandedID string, visible map[string]bool, nodeMap map[string]models.GraphNode, edges []graphEdgeRow) ([]string, int, bool) {
	expanded, ok := nodeMap[expandedID]
	if !ok {
		return nil, 0, false
	}
	neighborSet := map[string]bool{}
	for _, edge := range edges {
		if edge.sourceID == expandedID && !visible[edge.destinationID] {
			neighborSet[edge.destinationID] = true
		}
		if edge.destinationID == expandedID && !visible[edge.sourceID] {
			neighborSet[edge.sourceID] = true
		}
	}
	neighbors := make([]string, 0, len(neighborSet))
	for id := range neighborSet {
		if _, ok := nodeMap[id]; ok {
			neighbors = append(neighbors, id)
		}
	}
	degrees := graphDegreeByNode(edges)
	sameScope := func(id string) bool {
		n := nodeMap[id]
		return n.FilePath == expanded.FilePath || packagePathForNode(n) == packagePathForNode(expanded)
	}
	sort.SliceStable(neighbors, func(i, j int) bool {
		a, b := nodeMap[neighbors[i]], nodeMap[neighbors[j]]
		aChanged := a.ChangeType != nil
		bChanged := b.ChangeType != nil
		if aChanged != bChanged {
			return aChanged
		}
		if a.IsEntrypoint != b.IsEntrypoint {
			return a.IsEntrypoint
		}
		if degrees[a.ID] != degrees[b.ID] {
			return degrees[a.ID] > degrees[b.ID]
		}
		aSame, bSame := sameScope(a.ID), sameScope(b.ID)
		if aSame != bSame {
			return aSame
		}
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.LineStart != b.LineStart {
			return a.LineStart < b.LineStart
		}
		return a.ID < b.ID
	})
	hiddenCount := len(neighbors)
	hasMore := hiddenCount > graphExpansionMaxNodes
	if hasMore {
		neighbors = neighbors[:graphExpansionMaxNodes]
	}
	return neighbors, hiddenCount, hasMore
}

func graphEdgesForVisibleSet(edges []graphEdgeRow, visible map[string]bool) []models.GraphEdge {
	result := make([]models.GraphEdge, 0)
	seen := map[string]bool{}
	for _, edge := range edges {
		if !visible[edge.sourceID] || !visible[edge.destinationID] || edge.sourceID == edge.destinationID {
			continue
		}
		key := edge.sourceID + "|" + edge.destinationID + "|" + edge.edgeKind
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, models.GraphEdge{
			SourceID:            edge.sourceID,
			DestinationID:       edge.destinationID,
			EdgeKind:            edge.edgeKind,
			ChangeType:          edgeChangeTypePtr(edge.changeType),
			Weight:              1,
			UnderlyingEdgeCount: 1,
		})
	}
	return result
}

func (h *GraphHandler) loadRepoExpansionData(ctx context.Context, repoID, commitSHA string) (map[string]models.GraphNode, []graphEdgeRow, error) {
	nodeRows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
		from code_nodes
		where repo_id=$1 and commit_sha=$2
	`, repoID, commitSHA)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()

	nodeMap := map[string]models.GraphNode{}
	for nodeRows.Next() {
		var n models.GraphNode
		var inputsRaw, outputsRaw []byte
		var docComment, summary string
		if err := nodeRows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if isTestGraphNode(n) {
			continue
		}
		applyNodeSummary(&n, docComment, summary)
		n.NodeType = "context"
		if n.IsEntrypoint || n.FullName == "main" || strings.HasSuffix(n.FullName, ".main") {
			n.NodeType = "entrypoint"
		}
		n.PackagePath = packagePathForNode(n)
		n.Tests = []models.GraphNodeTest{}
		nodeMap[n.ID] = n
	}

	edgeRows, err := h.DB.Query(ctx, `
		select source_id, destination_id, edge_kind
		from code_edges
		where repo_id=$1 and commit_sha=$2
	`, repoID, commitSHA)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()

	edges := make([]graphEdgeRow, 0)
	for edgeRows.Next() {
		var edge graphEdgeRow
		if err := edgeRows.Scan(&edge.sourceID, &edge.destinationID, &edge.edgeKind); err != nil {
			continue
		}
		if _, ok := nodeMap[edge.sourceID]; !ok {
			continue
		}
		if _, ok := nodeMap[edge.destinationID]; !ok {
			continue
		}
		edges = append(edges, edge)
	}
	return nodeMap, edges, nil
}

func (h *GraphHandler) loadPRExpansionData(ctx context.Context, repoID, prID, mainCommitSHA, headCommit string) (map[string]models.GraphNode, []graphEdgeRow, error) {
	nodeMap, _, err := h.loadRepoExpansionData(ctx, repoID, mainCommitSHA)
	if err != nil {
		return nil, nil, err
	}
	mainIDByFullName := map[string]string{}
	for id, node := range nodeMap {
		mainIDByFullName[node.FullName] = id
	}

	changedRows, err := h.DB.Query(ctx, `
		select pnc.node_id, pnc.change_type, pnc.change_summary, pnc.diff_hunk,
		       pnc.old_full_name, pnc.old_file_path
		from pr_node_changes pnc
		where pnc.pull_request_id = $1
	`, prID)
	if err != nil {
		return nil, nil, err
	}
	defer changedRows.Close()

	changedSet := map[string]rawPRNodeChange{}
	changedIDs := make([]string, 0)
	for changedRows.Next() {
		var c rawPRNodeChange
		if err := changedRows.Scan(&c.nodeID, &c.changeType, &c.changeSummary, &c.diffHunk, &c.oldFullName, &c.oldFilePath); err != nil {
			continue
		}
		changedSet[c.nodeID] = c
		changedIDs = append(changedIDs, c.nodeID)
	}
	if len(changedIDs) == 0 {
		return nodeMap, []graphEdgeRow{}, nil
	}

	changedByFullName := map[string]string{}
	changedNodeRows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
		from code_nodes
		where id = any($1::uuid[])
	`, changedIDs)
	if err != nil {
		return nil, nil, err
	}
	defer changedNodeRows.Close()

	for changedNodeRows.Next() {
		var n models.GraphNode
		var inputsRaw, outputsRaw []byte
		var docComment, summary string
		if err := changedNodeRows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if isTestGraphNode(n) {
			continue
		}
		applyNodeSummary(&n, docComment, summary)
		c := changedSet[n.ID]
		n.NodeType = "changed"
		n.ChangeSummary = c.changeSummary
		n.DiffHunk = c.diffHunk
		ct := c.changeType
		n.ChangeType = &ct
		n.OldFullName = c.oldFullName
		n.OldFilePath = c.oldFilePath
		if c.diffHunk != nil {
			added, removed := countDiffLines(*c.diffHunk)
			n.LinesAdded = added
			n.LinesRemoved = removed
			n.Weight = added + removed
		}
		n.PackagePath = packagePathForNode(n)
		n.Tests = []models.GraphNodeTest{}
		nodeMap[n.ID] = n
		changedByFullName[n.FullName] = n.ID
		if c.oldFullName != nil && strings.TrimSpace(*c.oldFullName) != "" {
			changedByFullName[*c.oldFullName] = n.ID
		}
	}

	canonicalize := func(id, fullName, commitSHA string) string {
		if changedID, ok := changedByFullName[fullName]; ok {
			return changedID
		}
		if commitSHA == headCommit {
			if mainID, ok := mainIDByFullName[fullName]; ok {
				return mainID
			}
		}
		return id
	}

	edgeRows, err := h.DB.Query(ctx, `
		select e.commit_sha, e.source_id, e.destination_id, e.edge_kind,
		       caller.full_name, callee.full_name,
		       caller.is_test, callee.is_test
		from code_edges e
		join code_nodes caller on caller.id = e.source_id
		join code_nodes callee on callee.id = e.destination_id
		where e.repo_id = $1
		  and (($2 <> '' and e.commit_sha = $2) or ($3 <> '' and e.commit_sha = $3))
	`, repoID, mainCommitSHA, headCommit)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()

	edgeCommitState := map[string]map[string]bool{}
	edges := make([]graphEdgeRow, 0)
	seen := map[string]bool{}
	for edgeRows.Next() {
		var edge graphEdgeRow
		if err := edgeRows.Scan(&edge.commitSHA, &edge.sourceID, &edge.destinationID, &edge.edgeKind, &edge.sourceName, &edge.destinationName, &edge.sourceIsTest, &edge.destinationIsTest); err != nil {
			continue
		}
		if edge.sourceIsTest || edge.destinationIsTest {
			continue
		}
		stateKey := edge.sourceName + "|" + edge.destinationName + "|" + edge.edgeKind
		if edgeCommitState[stateKey] == nil {
			edgeCommitState[stateKey] = map[string]bool{}
		}
		if edge.commitSHA == mainCommitSHA {
			edgeCommitState[stateKey]["base"] = true
		}
		if edge.commitSHA == headCommit {
			edgeCommitState[stateKey]["head"] = true
		}
		edge.sourceID = canonicalize(edge.sourceID, edge.sourceName, edge.commitSHA)
		edge.destinationID = canonicalize(edge.destinationID, edge.destinationName, edge.commitSHA)
		if edge.sourceID == edge.destinationID {
			continue
		}
		if _, ok := nodeMap[edge.sourceID]; !ok {
			continue
		}
		if _, ok := nodeMap[edge.destinationID]; !ok {
			continue
		}
		key := edge.sourceID + "|" + edge.destinationID + "|" + edge.edgeKind + "|" + edge.sourceName + "|" + edge.destinationName
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, edge)
	}
	changedNames := map[string]bool{}
	for name := range changedByFullName {
		changedNames[name] = true
	}
	for i := range edges {
		state := edgeCommitState[edges[i].sourceName+"|"+edges[i].destinationName+"|"+edges[i].edgeKind]
		edges[i].changeType = markEdgeChangeType(edges[i], state, changedNames)
	}
	return nodeMap, edges, nil
}

// POST /api/v1/repos/{repoID}/graph/expand
func (h *GraphHandler) ExpandGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	var req models.GraphExpansionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	if req.NodeID == "" {
		http.Error(w, "node_id is required", http.StatusBadRequest)
		return
	}

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
		http.Error(w, "repo graph is not indexed", http.StatusConflict)
		return
	}

	mode := strings.TrimSpace(req.GraphContext.Mode)
	if mode == "" {
		mode = "repo"
	}

	var nodeMap map[string]models.GraphNode
	var allEdges []graphEdgeRow
	switch mode {
	case "repo":
		nodeMap, allEdges, err = h.loadRepoExpansionData(ctx, repoID, *repo.MainCommitSHA)
	case "pr":
		prID := strings.TrimSpace(req.GraphContext.PRID)
		if prID == "" {
			http.Error(w, "graph_context.pr_id is required for pr expansion", http.StatusBadRequest)
			return
		}
		var baseCommit, headCommit, baseBranch string
		err = h.DB.QueryRow(ctx, `
			select coalesce(base_commit_sha,''), coalesce(head_commit_sha,''), base_branch
			from pull_requests
			where id=$1 and repo_id=$2
		`, prID, repoID).Scan(&baseCommit, &headCommit, &baseBranch)
		if err != nil {
			http.Error(w, "pr not found", http.StatusNotFound)
			return
		}
		if baseBranch != repo.DefaultBranch {
			http.Error(w, "PR graph only supports pull requests targeting the indexed default branch", http.StatusConflict)
			return
		}
		if baseCommit == "" || baseCommit != *repo.MainCommitSHA {
			http.Error(w, "PR base SHA does not match the indexed default branch SHA", http.StatusConflict)
			return
		}
		nodeMap, allEdges, err = h.loadPRExpansionData(ctx, repoID, prID, *repo.MainCommitSHA, headCommit)
	default:
		http.Error(w, "graph_context.mode must be repo or pr", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("ExpandGraph: graph data query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if _, ok := nodeMap[req.NodeID]; !ok {
		http.Error(w, "node not found in graph context", http.StatusNotFound)
		return
	}

	visible := graphVisibleSet(req.VisibleNodeIDs, req.NodeID)
	newIDs, hiddenCount, hasMore := selectExpansionNeighbors(req.NodeID, visible, nodeMap, allEdges)
	responseVisible := map[string]bool{}
	for id := range visible {
		responseVisible[id] = true
	}
	for _, id := range newIDs {
		responseVisible[id] = true
	}

	degrees := graphDegreeByNode(allEdges)
	nodes := make([]models.GraphNode, 0, len(newIDs))
	for _, id := range newIDs {
		n := nodeMap[id]
		n.NodeType = expansionNodeType(id, req.NodeID, allEdges, n)
		n.Degree = degrees[id]
		n.GraphDepth = 1
		n.Boundary = graphHasHiddenNeighbor(id, responseVisible, allEdges)
		if n.PackagePath == "" {
			n.PackagePath = packagePathForNode(n)
		}
		if n.Tests == nil {
			n.Tests = []models.GraphNodeTest{}
		}
		nodes = append(nodes, n)
	}
	resolveGraphTypeRefs(nodeMap)
	for i := range nodes {
		if resolved, ok := nodeMap[nodes[i].ID]; ok {
			nodes[i].Inputs = resolved.Inputs
			nodes[i].Outputs = resolved.Outputs
		}
	}
	sortGraphNodes(nodes)
	if len(newIDs) > 0 {
		h.attachTestsFromEdges(ctx, repoID, newIDs, []string{*repo.MainCommitSHA}, func(id string) string { return id }, nodes)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.GraphExpansionResponse{
		Nodes:               nodes,
		Edges:               graphEdgesForVisibleSet(allEdges, responseVisible),
		ExpandedNodeID:      req.NodeID,
		HasMore:             hasMore,
		HiddenNeighborCount: hiddenCount,
	})
}

// GET /api/v1/repos/{repoID}/graph
func (h *GraphHandler) GetRepoGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

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
			Repo:  repo,
			Nodes: []models.GraphNode{},
			Edges: []models.GraphEdge{},
		})
		return
	}

	nodeRows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
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
		var docComment, summary string
		if err := nodeRows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		applyNodeSummary(&n, docComment, summary)
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
			select source_id, destination_id, edge_kind
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
			if err := edgeRows.Scan(&e.sourceID, &e.destinationID, &e.edgeKind); err != nil {
				continue
			}
			if _, ok := nodeMap[e.sourceID]; !ok {
				continue
			}
			if _, ok := nodeMap[e.destinationID]; !ok {
				continue
			}
			allEdges = append(allEdges, e)
		}
		edgeRows.Close()
	}

	selected, edges := selectRankedVisibleGraph(seedIDs, allEdges, map[string]int{})
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
		n.PackagePath = packagePathForNode(n)
		nodeMap[id] = n
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
		h.attachTestsFromEdges(ctx, repoID, nodeIDs, []string{*repo.MainCommitSHA}, func(id string) string { return id }, nodes)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.RepoGraphResponse{
		Repo:  repo,
		Nodes: nodes,
		Edges: edges,
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
	changedRows, err := h.DB.Query(ctx, `
		select pnc.node_id, pnc.change_type, pnc.change_summary, pnc.diff_hunk,
		       pnc.old_full_name, pnc.old_file_path
		from pr_node_changes pnc
		where pnc.pull_request_id = $1
	`, prID)
	if err != nil {
		log.Printf("GetGraph: pr_node_changes query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer changedRows.Close()

	changedSet := map[string]rawPRNodeChange{}
	var changedIDs []string
	for changedRows.Next() {
		var c rawPRNodeChange
		if err := changedRows.Scan(&c.nodeID, &c.changeType, &c.changeSummary, &c.diffHunk, &c.oldFullName, &c.oldFilePath); err != nil {
			continue
		}
		changedSet[c.nodeID] = c
		changedIDs = append(changedIDs, c.nodeID)
	}
	changedRows.Close()
	testChanges := h.loadPRTestChanges(ctx, changedIDs, changedSet)

	if len(changedIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.GraphResponse{
			PR:          pr,
			Nodes:       []models.GraphNode{},
			Edges:       []models.GraphEdge{},
			Files:       files,
			TestChanges: testChanges,
			TestContext: []models.GraphNode{},
		})
		return
	}

	// Get full_names of the changed nodes (stored at PR head SHA)
	changedIDToFullName := map[string]string{}
	changedIDIsTest := map[string]bool{}
	productionChangedIDs := make([]string, 0, len(changedIDs))
	productionChangedFullNames := make([]string, 0, len(changedIDs))
	fnRows, _ := h.DB.Query(ctx, `select id, full_name, is_test from code_nodes where id = any($1::uuid[])`, changedIDs)
	if fnRows != nil {
		defer fnRows.Close()
		for fnRows.Next() {
			var id, fn string
			var isTest bool
			fnRows.Scan(&id, &fn, &isTest)
			changedIDToFullName[id] = fn
			changedIDIsTest[id] = isTest
			if !isTest {
				productionChangedIDs = append(productionChangedIDs, id)
				productionChangedFullNames = append(productionChangedFullNames, fn)
			}
		}
		fnRows.Close()
	}

	defaultBranchLookupNames := append([]string{}, productionChangedFullNames...)
	productionChangedNameSet := map[string]bool{}
	for _, fn := range productionChangedFullNames {
		productionChangedNameSet[fn] = true
	}
	changedIDToOldFullName := map[string]string{}
	for _, id := range changedIDs {
		c := changedSet[id]
		if changedIDIsTest[id] || c.oldFullName == nil || strings.TrimSpace(*c.oldFullName) == "" {
			continue
		}
		changedIDToOldFullName[id] = *c.oldFullName
		productionChangedNameSet[*c.oldFullName] = true
		defaultBranchLookupNames = append(defaultBranchLookupNames, *c.oldFullName)
	}

	// Find equivalent node IDs at the indexed default-branch commit (for edge lookups).
	// code_edges are stored with main-branch node IDs during repo_init.
	mainIDByFullName := map[string]string{}
	var mainChangedIDs []string
	if mainCommitSHA != "" && len(defaultBranchLookupNames) > 0 {
		mRows, _ := h.DB.Query(ctx, `
			select id, full_name from code_nodes
			where repo_id=$1 and commit_sha=$2 and full_name = any($3)
		`, repoID, mainCommitSHA, defaultBranchLookupNames)
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

	mainIDToPRID := map[string]string{}
	for prID2, fn := range changedIDToFullName {
		if changedIDIsTest[prID2] {
			continue
		}
		if mainID, ok := mainIDByFullName[fn]; ok {
			mainIDToPRID[mainID] = prID2
		}
		if oldFn, ok := changedIDToOldFullName[prID2]; ok {
			if mainID, ok := mainIDByFullName[oldFn]; ok {
				mainIDToPRID[mainID] = prID2
			}
		}
	}
	remapID := func(id string) string {
		if prEquiv, ok := mainIDToPRID[id]; ok {
			return prEquiv
		}
		return id
	}

	var allEdges []graphEdgeRow
	if len(changedIDs) > 0 {
		eRows, _ := h.DB.Query(ctx, `
			select e.commit_sha, e.source_id, e.destination_id, e.edge_kind,
			       caller.full_name, callee.full_name,
			       caller.is_test, callee.is_test
			from code_edges e
			join code_nodes caller on caller.id = e.source_id
			join code_nodes callee on callee.id = e.destination_id
			where e.repo_id = $1
			  and (($2 <> '' and e.commit_sha = $2) or ($3 <> '' and e.commit_sha = $3))
		`, repoID, mainCommitSHA, headCommit)
		if eRows != nil {
			defer eRows.Close()
			edgeCommitState := map[string]map[string]bool{}
			for eRows.Next() {
				var e graphEdgeRow
				eRows.Scan(&e.commitSHA, &e.sourceID, &e.destinationID, &e.edgeKind, &e.sourceName, &e.destinationName, &e.sourceIsTest, &e.destinationIsTest)
				key := e.sourceName + "|" + e.destinationName + "|" + e.edgeKind
				if edgeCommitState[key] == nil {
					edgeCommitState[key] = map[string]bool{}
				}
				if e.commitSHA == mainCommitSHA {
					edgeCommitState[key]["base"] = true
				}
				if e.commitSHA == headCommit {
					edgeCommitState[key]["head"] = true
				}
				e.sourceID = remapID(e.sourceID)
				e.destinationID = remapID(e.destinationID)
				if e.sourceID == e.destinationID {
					continue
				}
				allEdges = append(allEdges, e)
			}
			eRows.Close()
			for i := range allEdges {
				state := edgeCommitState[allEdges[i].sourceName+"|"+allEdges[i].destinationName+"|"+allEdges[i].edgeKind]
				allEdges[i].changeType = markEdgeChangeType(allEdges[i], state, productionChangedNameSet)
			}
		}
	}

	lineChanges := map[string]int{}
	for _, id := range productionChangedIDs {
		c := changedSet[id]
		if c.diffHunk != nil {
			added, removed := countDiffLines(*c.diffHunk)
			lineChanges[id] = added + removed
		}
	}
	productionEdges := make([]graphEdgeRow, 0, len(allEdges))
	for _, edge := range allEdges {
		if !relevantProductionEdge(edge, productionChangedNameSet) {
			continue
		}
		productionEdges = append(productionEdges, edge)
	}
	selected, edges := selectVisibleGraph(productionChangedIDs, productionEdges, lineChanges)

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
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
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
		var docComment, summary string
		if err := nodeRows.Scan(
			&n.ID, &commitSHA, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if isTestGraphNode(n) {
			continue
		}
		applyNodeSummary(&n, docComment, summary)
		nodeMap[n.ID] = graphNodeRecord{node: n, commitSHA: commitSHA}
	}

	changedIDSet := map[string]bool{}
	for _, id := range productionChangedIDs {
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
		if e.EdgeKind == "calls" && changedIDSet[e.DestinationID] {
			callerSet[e.SourceID] = true
		}
		if e.EdgeKind == "calls" && changedIDSet[e.SourceID] {
			calleeSet[e.DestinationID] = true
		}
	}

	// First pass: add changed nodes (PR-head IDs) with change info
	for _, id := range productionChangedIDs {
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
		n.OldFullName = c.oldFullName
		n.OldFilePath = c.oldFilePath
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
		} else if calleeSet[id] {
			n.NodeType = "callee"
		} else {
			n.NodeType = "context"
		}
		n.Weight = meta.weight
		n.Degree = meta.degree
		n.GraphDepth = meta.depth
		n.Boundary = meta.boundary
		finalNodeMap[id] = n
	}

	finalNodeIDs := make([]string, 0, len(finalNodeMap))
	for id := range finalNodeMap {
		finalNodeIDs = append(finalNodeIDs, id)
	}
	if len(finalNodeIDs) > 0 {
		nodesForTests := make([]models.GraphNode, 0, len(finalNodeMap))
		for _, n := range finalNodeMap {
			nodesForTests = append(nodesForTests, n)
		}
		h.attachTestsFromEdges(ctx, repoID, finalNodeIDs, nonEmptyStrings(mainCommitSHA, headCommit), canonicalizeID, nodesForTests)
		for _, n := range nodesForTests {
			finalNodeMap[n.ID] = n
		}
	}

	resolveGraphTypeRefs(finalNodeMap)

	nodes := make([]models.GraphNode, 0, len(finalNodeMap))
	for _, n := range finalNodeMap {
		if n.Tests == nil {
			n.Tests = []models.GraphNodeTest{}
		}
		n.PackagePath = packagePathForNode(n)
		nodes = append(nodes, n)
	}
	sortGraphNodes(nodes)

	seenEdges := map[string]bool{}
	for _, e := range edges {
		seenEdges[e.SourceID+"|"+e.DestinationID+"|"+e.EdgeKind] = true
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
				key := structID + "|" + methodID + "|owns_method"
				if !seenEdges[key] {
					seenEdges[key] = true
					edges = append(edges, models.GraphEdge{SourceID: structID, DestinationID: methodID, EdgeKind: "owns_method", Weight: 1, UnderlyingEdgeCount: 1})
				}
			}
		}
	}
	testContextMap := h.loadPRTestContext(ctx, allEdges, testChanges, canonicalizeID)
	edges = appendTestFocusEdges(edges, allEdges, testChanges, finalNodeMap, testContextMap, canonicalizeID)
	testContext := make([]models.GraphNode, 0, len(testContextMap))
	for _, n := range testContextMap {
		testContext = append(testContext, n)
	}
	sortGraphNodes(testContext)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.GraphResponse{
		PR:          pr,
		Nodes:       nodes,
		Edges:       edges,
		Files:       files,
		TestChanges: testChanges,
		TestContext: testContext,
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

func (h *GraphHandler) loadPRTestChanges(ctx context.Context, changedIDs []string, changedSet map[string]rawPRNodeChange) []models.GraphNode {
	if len(changedIDs) == 0 {
		return []models.GraphNode{}
	}

	rows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
		from code_nodes
		where id = any($1::uuid[])
	`, changedIDs)
	if err != nil {
		log.Printf("GetGraph: test change node query: %v", err)
		return []models.GraphNode{}
	}
	defer rows.Close()

	testChanges := []models.GraphNode{}
	for rows.Next() {
		var n models.GraphNode
		var inputsRaw, outputsRaw []byte
		var docComment, summary string
		if err := rows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		if !isTestGraphNode(n) {
			continue
		}
		applyNodeSummary(&n, docComment, summary)
		n.NodeType = "changed"
		n.PackagePath = packagePathForNode(n)
		if c, ok := changedSet[n.ID]; ok {
			ct := c.changeType
			n.ChangeType = &ct
			n.ChangeSummary = c.changeSummary
			n.DiffHunk = c.diffHunk
			n.OldFullName = c.oldFullName
			n.OldFilePath = c.oldFilePath
			if c.diffHunk != nil {
				n.LinesAdded, n.LinesRemoved = countDiffLines(*c.diffHunk)
			}
		}
		n.Tests = []models.GraphNodeTest{}
		testChanges = append(testChanges, n)
	}
	sortGraphNodes(testChanges)
	return testChanges
}

func (h *GraphHandler) loadPRTestContext(ctx context.Context, allEdges []graphEdgeRow, testChanges []models.GraphNode, canonicalizeID func(string) string) map[string]models.GraphNode {
	testIDs := map[string]bool{}
	for _, n := range testChanges {
		testIDs[n.ID] = true
	}
	if len(testIDs) == 0 {
		return map[string]models.GraphNode{}
	}

	contextIDs := map[string]bool{}
	for _, edge := range allEdges {
		if edge.edgeKind != "calls" {
			continue
		}
		source := canonicalizeID(edge.sourceID)
		target := canonicalizeID(edge.destinationID)
		if source == "" || target == "" || source == target || !testIDs[source] || testIDs[target] || edge.destinationIsTest {
			continue
		}
		contextIDs[target] = true
	}
	if len(contextIDs) == 0 {
		return map[string]models.GraphNode{}
	}

	ids := make([]string, 0, len(contextIDs))
	for id := range contextIDs {
		ids = append(ids, id)
	}
	rows, err := h.DB.Query(ctx, `
		select id, full_name, file_path, line_start, line_end,
		       inputs, outputs, language, kind, is_test, is_entrypoint, coalesce(doc_comment,''), coalesce(summary,'')
		from code_nodes
		where id = any($1::uuid[])
	`, ids)
	if err != nil {
		log.Printf("GetGraph: test context node query: %v", err)
		return map[string]models.GraphNode{}
	}
	defer rows.Close()

	contextNodes := map[string]models.GraphNode{}
	for rows.Next() {
		var n models.GraphNode
		var inputsRaw, outputsRaw []byte
		var docComment, summary string
		if err := rows.Scan(
			&n.ID, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&inputsRaw, &outputsRaw, &n.Language, &n.Kind, &n.IsTest, &n.IsEntrypoint, &docComment, &summary,
		); err != nil {
			continue
		}
		if isTestGraphNode(n) {
			continue
		}
		n.Inputs = decodeTypeRefs(inputsRaw)
		n.Outputs = decodeTypeRefs(outputsRaw)
		applyNodeSummary(&n, docComment, summary)
		n.NodeType = "context"
		n.GraphDepth = 2
		n.Tests = []models.GraphNodeTest{}
		n.PackagePath = packagePathForNode(n)
		contextNodes[n.ID] = n
	}
	return contextNodes
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

	var changeType, diffHunk, oldFullName, oldFilePath *string
	_ = h.DB.QueryRow(ctx, `
		select change_type, diff_hunk, old_full_name, old_file_path
		from pr_node_changes
		where pull_request_id=$1 and node_id=$2
	`, prID, nodeID).Scan(&changeType, &diffHunk, &oldFullName, &oldFilePath)

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

	baseLookupFullName, baseLookupFilePath := baseLookupIdentity(selected.fullName, selected.filePath, changeType, oldFullName, oldFilePath)
	selectedForBaseLookup := selected
	selectedForBaseLookup.fullName = baseLookupFullName
	selectedForBaseLookup.filePath = baseLookupFilePath
	baseNode := func() *nodeMeta {
		if baseCommit == "" {
			return nil
		}
		var n nodeMeta
		err := h.DB.QueryRow(ctx, `
			select id, full_name, file_path, language, line_start, line_end
			from code_nodes
			where repo_id=$1 and commit_sha=$2 and full_name=$3 and file_path=$4
		`, repoID, baseCommit, selectedForBaseLookup.fullName, selectedForBaseLookup.filePath).Scan(&n.id, &n.fullName, &n.filePath, &n.language, &n.lineStart, &n.lineEnd)
		if err != nil {
			return nil
		}
		return &n
	}()
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

func baseLookupIdentity(selectedFullName, selectedFilePath string, changeType, oldFullName, oldFilePath *string) (string, string) {
	if changeType != nil && *changeType == "renamed" {
		if oldFullName != nil && *oldFullName != "" {
			selectedFullName = *oldFullName
		}
		if oldFilePath != nil && *oldFilePath != "" {
			selectedFilePath = *oldFilePath
		}
	}
	return selectedFullName, selectedFilePath
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
