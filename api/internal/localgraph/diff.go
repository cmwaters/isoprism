package localgraph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isoprism/api/internal/models"
	"github.com/isoprism/api/internal/parser"
)

const indexTreeRef = ":index:"

func GenerateDiff(ctx context.Context, opts Options) (ReviewGraphPayload, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	g := gitClient{root: root}
	baseRef, headRef, err := resolveDiffRefs(ctx, g, opts.Args)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	baseSHA, err := g.resolveCommit(ctx, baseRef)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	headSHA, err := g.resolveCommit(ctx, headRef)
	if err != nil {
		if headRef == indexTreeRef {
			headSHA = "index"
		} else if headSHA, err = g.resolveObject(ctx, headRef); err != nil {
			return ReviewGraphPayload{}, err
		}
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(root, ".isoprism")
	}
	if opts.RebuildCache {
		if err := os.RemoveAll(filepath.Join(cacheDir, "objects")); err != nil {
			return ReviewGraphPayload{}, err
		}
	}

	baseGraph, err := loadTreeGraph(ctx, g, cacheDir, baseRef, baseSHA)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	headGraph, err := loadTreeGraph(ctx, g, cacheDir, headRef, headSHA)
	if err != nil {
		return ReviewGraphPayload{}, err
	}
	changes, err := loadFileChanges(ctx, g, baseRef, headRef)
	if err != nil {
		return ReviewGraphPayload{}, err
	}

	payload := buildReviewPayload(root, baseRef, headRef, baseSHA, headSHA, baseGraph, headGraph, changes)
	sortGraphPayload(&payload)
	return payload, nil
}

func resolveDiffRefs(ctx context.Context, g gitClient, args []string) (string, string, error) {
	if len(args) == 0 {
		branch, err := g.resolveDefaultBranch(ctx)
		if err != nil {
			return "", "", err
		}
		return branch, "HEAD", nil
	}
	for _, arg := range args {
		if arg == "unstaged" {
			return "", "", fmt.Errorf("unstaged mode needs working-tree overlay support and is not implemented yet; use staged or pass committed refs")
		}
	}
	switch len(args) {
	case 1:
		if args[0] == "staged" {
			return "HEAD", indexTreeRef, nil
		}
		return args[0], "HEAD", nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", fmt.Errorf("diff accepts zero, one, or two refs")
	}
}

func loadTreeGraph(ctx context.Context, g gitClient, cacheDir, ref, sha string) (treeGraph, error) {
	tree, err := g.listTree(ctx, ref)
	if err != nil {
		return treeGraph{}, err
	}
	graph := treeGraph{
		ref: ref, sha: sha, tree: tree,
		nodes: map[string]graphNodeObject{}, nodesByRef: map[string]graphNodeObject{}, edgesByRef: map[string][]semanticEdge{},
	}
	fileContents := map[string][]byte{}
	nodeByName := map[string]bool{}
	var parsedNodes []graphNodeObject

	for path, blobSHA := range tree {
		if !parser.IsSupportedFile(path) {
			continue
		}
		objects, content, err := loadBlobNodes(ctx, g, cacheDir, ref, path, blobSHA)
		if err != nil {
			return treeGraph{}, err
		}
		if content != nil {
			fileContents[path] = content
		} else if len(objects) > 0 {
			content, _ = g.showFile(ctx, ref, path)
			fileContents[path] = content
		}
		for id, obj := range objects {
			graph.nodes[id] = obj
			graph.nodesByRef[semanticRef(obj.FilePath, obj.FullName)] = obj
			nodeByName[obj.FullName] = true
			parsedNodes = append(parsedNodes, obj)
		}
	}

	parserContent := map[string][]byte{}
	for path, content := range fileContents {
		parserContent[path] = content
	}
	resolver := parser.BuildResolverIndex(parserContent, nodeByName)
	for path, content := range parserContent {
		for _, edge := range parser.ExtractCallEdgesWithResolver(content, path, resolver) {
			source, target := graphNodeByFullName(parsedNodes, edge.CallerFullName), graphNodeByFullName(parsedNodes, edge.CalleeFullName)
			if source.FullName == "" || target.FullName == "" {
				continue
			}
			graph.edges = append(graph.edges, semanticEdge{SourceRef: semanticRef(source.FilePath, source.FullName), TargetRef: semanticRef(target.FilePath, target.FullName), Kind: "calls"})
		}
	}
	for _, edge := range semanticTypeEdges(parsedNodes) {
		graph.edges = append(graph.edges, edge)
	}
	seen := map[string]bool{}
	for _, edge := range graph.edges {
		key := edge.SourceRef + "\x00" + edge.TargetRef + "\x00" + edge.Kind
		if seen[key] {
			continue
		}
		seen[key] = true
		graph.edgesByRef[edge.SourceRef] = append(graph.edgesByRef[edge.SourceRef], edge)
		graph.edgesByRef[edge.TargetRef] = append(graph.edgesByRef[edge.TargetRef], edge)
	}
	return graph, nil
}

func loadBlobNodes(ctx context.Context, g gitClient, cacheDir, ref, path, blobSHA string) (map[string]graphNodeObject, []byte, error) {
	indexPath := filepath.Join(cacheDir, "objects", "index", "blob_to_nodes", blobSHA+".json")
	var index struct {
		SchemaVersion string            `json:"schema_version"`
		Nodes         map[string]string `json:"nodes"`
	}
	if err := readJSON(indexPath, &index); err == nil && index.SchemaVersion == nodeSchemaVersion {
		out := map[string]graphNodeObject{}
		for _, id := range index.Nodes {
			var obj graphNodeObject
			if err := readJSON(filepath.Join(cacheDir, "objects", "nodes", id+".json"), &obj); err != nil {
				return nil, nil, fmt.Errorf("malformed cache object %s: %w; run isoprism diff --rebuild-cache", id, err)
			}
			out[id] = obj
		}
		return out, nil, nil
	}

	content, err := g.showFile(ctx, ref, path)
	if err != nil {
		return nil, nil, err
	}
	nodes := parser.Parse(content, path)
	out := map[string]graphNodeObject{}
	index.SchemaVersion = nodeSchemaVersion
	index.Nodes = map[string]string{}
	for _, n := range nodes {
		blob := blobSHA
		id := nodeID(nodeKind(n), n.FullName, n.FilePath, blobSHA)
		obj := graphNodeObject{
			SchemaVersion: nodeSchemaVersion,
			Type:          nodeKind(n), FullName: n.FullName, FilePath: n.FilePath, GitBlobSHA: &blob,
			LineStart: n.LineStart, LineEnd: n.LineEnd, Inputs: n.Inputs, Outputs: n.Outputs,
			Language: n.Language, Kind: n.Kind, BodyHash: n.BodyHash, Body: n.Body, Fields: n.Fields,
			DocComment: n.DocComment, IsTest: n.IsTest, IsEntrypoint: n.IsEntrypoint,
			OutgoingLinks: []linkObject{},
		}
		out[id] = obj
		index.Nodes[n.FullName] = id
		if err := writeJSONAtomic(filepath.Join(cacheDir, "objects", "nodes", id+".json"), obj); err != nil {
			return nil, nil, err
		}
	}
	if err := writeJSONAtomic(indexPath, index); err != nil {
		return nil, nil, err
	}
	return out, content, nil
}

func loadFileChanges(ctx context.Context, g gitClient, baseRef, headRef string) ([]fileChange, error) {
	changes, err := g.diffNameStatus(ctx, baseRef, headRef)
	if err != nil {
		return nil, err
	}
	stats, _ := g.diffNumstat(ctx, baseRef, headRef)
	for i := range changes {
		key := changes[i].Filename
		if changes[i].Status == "removed" && changes[i].PreviousFilename != "" {
			key = changes[i].PreviousFilename
		}
		if s, ok := stats[key]; ok {
			changes[i].Additions, changes[i].Deletions = s[0], s[1]
		}
		paths := []string{changes[i].Filename}
		if changes[i].PreviousFilename != "" {
			paths = append(paths, changes[i].PreviousFilename)
		}
		patch, _ := g.diffPatch(ctx, baseRef, headRef, paths...)
		changes[i].Patch = patch
	}
	return changes, nil
}

func buildReviewPayload(root, baseRef, headRef, baseSHA, headSHA string, baseGraph, headGraph treeGraph, changes []fileChange) ReviewGraphPayload {
	filePatches := map[string]string{}
	files := make([]models.PRFileDiff, 0, len(changes))
	for _, c := range changes {
		patch := c.Patch
		filePatches[c.Filename] = patch
		if c.PreviousFilename != "" {
			filePatches[c.PreviousFilename] = patch
		}
		var previous *string
		if c.PreviousFilename != "" {
			previous = &c.PreviousFilename
		}
		files = append(files, models.PRFileDiff{
			Filename: c.Filename, PreviousFilename: previous, Status: c.Status,
			Additions: c.Additions, Deletions: c.Deletions, Changes: c.Additions + c.Deletions, Patch: &patch,
		})
	}

	changedRefs := map[string]bool{}
	nodesByID := map[string]models.GraphNode{}
	testChanges := map[string]models.GraphNode{}
	lineChanges := map[string]int{}
	for ref, head := range headGraph.nodesByRef {
		base, exists := baseGraph.nodesByRef[ref]
		if exists && base.BodyHash == head.BodyHash {
			continue
		}
		changeType := "added"
		if exists {
			changeType = "modified"
		}
		id := nodeID(nodeKindFromObject(head), head.FullName, head.FilePath, derefBlob(head.GitBlobSHA))
		node := graphNodeFromObject(id, head, "changed")
		node.ChangeType = &changeType
		node.LinesAdded = max(1, head.LineEnd-head.LineStart+1)
		node.LinesRemoved = 0
		if patch := filePatches[head.FilePath]; patch != "" {
			node.DiffHunk = &patch
		}
		changedRefs[ref] = true
		lineChanges[id] = node.LinesAdded + node.LinesRemoved
		if node.IsTest {
			testChanges[id] = node
		} else {
			nodesByID[id] = node
		}
	}
	for ref, base := range baseGraph.nodesByRef {
		if _, exists := headGraph.nodesByRef[ref]; exists {
			continue
		}
		id := nodeID(nodeKindFromObject(base), base.FullName, base.FilePath, derefBlob(base.GitBlobSHA))
		changeType := "deleted"
		node := graphNodeFromObject(id, base, "changed")
		node.ChangeType = &changeType
		node.LinesRemoved = max(1, base.LineEnd-base.LineStart+1)
		if patch := filePatches[base.FilePath]; patch != "" {
			node.DiffHunk = &patch
		}
		changedRefs[ref] = true
		lineChanges[id] = node.LinesAdded + node.LinesRemoved
		if node.IsTest {
			testChanges[id] = node
		} else {
			nodesByID[id] = node
		}
	}

	edges := visibleEdges(headGraph, baseGraph, changedRefs, nodesByID)
	for _, edge := range edges {
		if source, ok := headGraph.nodesByRef[edge.SourceRef]; ok {
			id := nodeID(nodeKindFromObject(source), source.FullName, source.FilePath, derefBlob(source.GitBlobSHA))
			if !nodesByID[id].IsTest {
				ensureContextNode(nodesByID, id, source, lineChanges)
			}
		}
		if target, ok := headGraph.nodesByRef[edge.TargetRef]; ok {
			id := nodeID(nodeKindFromObject(target), target.FullName, target.FilePath, derefBlob(target.GitBlobSHA))
			if !nodesByID[id].IsTest {
				ensureContextNode(nodesByID, id, target, lineChanges)
			}
		}
	}

	graphEdges := make([]models.GraphEdge, 0, len(edges))
	seenEdges := map[string]bool{}
	for _, edge := range edges {
		source := resolveEdgeNodeID(edge.SourceRef, headGraph, baseGraph)
		target := resolveEdgeNodeID(edge.TargetRef, headGraph, baseGraph)
		if source == "" || target == "" || source == target {
			continue
		}
		if _, ok := nodesByID[source]; !ok {
			continue
		}
		if _, ok := nodesByID[target]; !ok {
			continue
		}
		key := source + "|" + target + "|" + edge.Kind
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		graphEdges = append(graphEdges, models.GraphEdge{SourceID: source, DestinationID: target, EdgeKind: edge.Kind, Weight: 1, UnderlyingEdgeCount: 1})
	}

	nodes := make([]models.GraphNode, 0, len(nodesByID))
	for id, node := range nodesByID {
		node.Degree = degree(id, graphEdges)
		if !changedNode(node) {
			node.NodeType = "context"
		}
		nodes = append(nodes, node)
	}
	tests := make([]models.GraphNode, 0, len(testChanges))
	for _, node := range testChanges {
		tests = append(tests, node)
	}
	sources := map[string]string{}
	for id, node := range nodesByID {
		if obj, ok := headGraph.nodesByRef[semanticRef(node.FilePath, node.FullName)]; ok {
			sources[id] = obj.Body
			continue
		}
		if obj, ok := baseGraph.nodesByRef[semanticRef(node.FilePath, node.FullName)]; ok {
			sources[id] = obj.Body
		}
	}
	for id, node := range testChanges {
		if obj, ok := headGraph.nodesByRef[semanticRef(node.FilePath, node.FullName)]; ok {
			sources[id] = obj.Body
			continue
		}
		if obj, ok := baseGraph.nodesByRef[semanticRef(node.FilePath, node.FullName)]; ok {
			sources[id] = obj.Body
		}
	}
	body := fmt.Sprintf("Local semantic diff from %s to %s", baseRef, headRef)
	payload := ReviewGraphPayload{
		SchemaVersion: "review-graph-v1",
		Mode:          "diff",
		Repository:    LocalRepository{Root: root, Name: filepath.Base(root), DefaultBranch: baseRef},
		Diff:          DiffMetadata{BaseRef: baseRef, HeadRef: headRef, BaseSHA: baseSHA, HeadSHA: headSHA},
		Graph: models.GraphResponse{
			PR:    models.GraphPR{ID: headSHA, Number: 0, Title: "Local diff", BaseCommitSHA: baseSHA, HeadCommitSHA: headSHA, Body: body, AuthorLogin: "local"},
			Nodes: nodes, Edges: graphEdges, Files: files, TestChanges: tests, TestContext: []models.GraphNode{},
		},
		Annotations: loadAnnotations(filepath.Join(root, ".isoprism"), baseSHA, headSHA),
		Metadata:    map[string]interface{}{"generator": "isoprism local CLI", "parity": "parser-and-payload-v1", "sources": sources},
	}
	return payload
}

func ensureContextNode(nodes map[string]models.GraphNode, id string, obj graphNodeObject, lineChanges map[string]int) {
	if _, ok := nodes[id]; ok {
		return
	}
	node := graphNodeFromObject(id, obj, "context")
	nodes[id] = node
	lineChanges[id] = 0
}

func changedNode(node models.GraphNode) bool {
	return node.ChangeType != nil
}

func visibleEdges(headGraph, baseGraph treeGraph, changedRefs map[string]bool, nodes map[string]models.GraphNode) []semanticEdge {
	seen := map[string]bool{}
	var out []semanticEdge
	for ref := range changedRefs {
		for _, graph := range []treeGraph{headGraph, baseGraph} {
			for _, edge := range graph.edgesByRef[ref] {
				key := edge.SourceRef + "\x00" + edge.TargetRef + "\x00" + edge.Kind
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, edge)
			}
		}
	}
	return out
}

func resolveEdgeNodeID(ref string, headGraph, baseGraph treeGraph) string {
	if node, ok := headGraph.nodesByRef[ref]; ok {
		return nodeID(nodeKindFromObject(node), node.FullName, node.FilePath, derefBlob(node.GitBlobSHA))
	}
	if node, ok := baseGraph.nodesByRef[ref]; ok {
		return nodeID(nodeKindFromObject(node), node.FullName, node.FilePath, derefBlob(node.GitBlobSHA))
	}
	return ""
}

func graphNodeByFullName(nodes []graphNodeObject, fullName string) graphNodeObject {
	for _, node := range nodes {
		if node.FullName == fullName {
			return node
		}
	}
	return graphNodeObject{}
}

func nodeKindFromObject(obj graphNodeObject) string {
	if obj.IsTest {
		return "test"
	}
	return obj.Kind
}

func derefBlob(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func degree(id string, edges []models.GraphEdge) int {
	count := 0
	for _, e := range edges {
		if e.SourceID == id || e.DestinationID == id {
			count++
		}
	}
	return count
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
