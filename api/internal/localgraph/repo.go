package localgraph

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/isoprism/api/internal/models"
)

type CommitGraph struct {
	Repo      models.Repository
	Programs  []models.GraphProgram
	Graph     models.RepoGraphResponse
	Sources   map[string]models.NodeCodeResponse
	treeGraph treeGraph
}

func LoadRepoMetadata(ctx context.Context, opts Options) (models.Repository, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return models.Repository{}, err
	}
	g := gitClient{root: root}
	defaultBranch, err := g.resolveDefaultBranch(ctx)
	if err != nil {
		return models.Repository{}, err
	}
	sha, err := g.resolveCommit(ctx, "HEAD")
	if err != nil {
		return models.Repository{}, err
	}

	return models.Repository{
		ID:            "local",
		FullName:      "local/" + filepath.Base(root),
		DefaultBranch: defaultBranch,
		MainCommitSHA: &sha,
		IndexStatus:   "ready",
		IsActive:      true,
	}, nil
}

func LoadCommitGraph(ctx context.Context, opts Options) (CommitGraph, error) {
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return CommitGraph{}, err
	}
	g := gitClient{root: root}
	repo, err := LoadRepoMetadata(ctx, opts)
	if err != nil {
		return CommitGraph{}, err
	}
	sha := derefString(repo.MainCommitSHA)
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(root, ".isoprism")
	}
	graph, err := loadTreeGraph(ctx, g, cacheDir, "HEAD", sha)
	if err != nil {
		return CommitGraph{}, err
	}
	nodesByID, sources := graphNodesAndSources(graph, sha)
	programs := localPrograms(nodesByID)
	repoGraph := models.RepoGraphResponse{
		Repo:     repo,
		Programs: programs,
		Nodes:    []models.GraphNode{},
		Edges:    []models.GraphEdge{},
	}
	return CommitGraph{Repo: repo, Programs: programs, Graph: repoGraph, Sources: sources, treeGraph: graph}, nil
}

func ProgramGraph(ctx context.Context, opts Options, programID string) (models.RepoGraphResponse, map[string]models.NodeCodeResponse, error) {
	data, err := LoadCommitGraph(ctx, opts)
	if err != nil {
		return models.RepoGraphResponse{}, nil, err
	}
	nodesByID, sources := graphNodesAndSources(data.treeGraph, derefString(data.Repo.MainCommitSHA))
	edges := graphEdges(data.treeGraph)
	selected := boundedNodeIDs(programID, edges, 2)
	if len(selected) == 0 {
		selected[programID] = true
	}
	nodes := make([]models.GraphNode, 0, len(selected))
	for id := range selected {
		node, ok := nodesByID[id]
		if !ok || node.IsTest {
			continue
		}
		node.Degree = degree(id, edges)
		nodes = append(nodes, node)
	}
	filteredEdges := filterGraphEdges(edges, selected)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].FullName < nodes[j].FullName })
	return models.RepoGraphResponse{Repo: data.Repo, Programs: data.Programs, Nodes: nodes, Edges: filteredEdges}, sources, nil
}

func ExpandGraph(ctx context.Context, opts Options, nodeID string, visibleIDs []string) (models.GraphExpansionResponse, map[string]models.NodeCodeResponse, error) {
	data, err := LoadCommitGraph(ctx, opts)
	if err != nil {
		return models.GraphExpansionResponse{}, nil, err
	}
	nodesByID, sources := graphNodesAndSources(data.treeGraph, derefString(data.Repo.MainCommitSHA))
	edges := graphEdges(data.treeGraph)
	selected := map[string]bool{}
	for _, id := range visibleIDs {
		selected[id] = true
	}
	selected[nodeID] = true
	for _, edge := range edges {
		if edge.SourceID == nodeID {
			selected[edge.DestinationID] = true
		}
		if edge.DestinationID == nodeID {
			selected[edge.SourceID] = true
		}
	}
	nodes := make([]models.GraphNode, 0, len(selected))
	for id := range selected {
		node, ok := nodesByID[id]
		if !ok || node.IsTest {
			continue
		}
		node.Degree = degree(id, edges)
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].FullName < nodes[j].FullName })
	return models.GraphExpansionResponse{
		Nodes:               nodes,
		Edges:               filterGraphEdges(edges, selected),
		ExpandedNodeID:      nodeID,
		HasMore:             false,
		HiddenNeighborCount: 0,
	}, sources, nil
}

func graphNodesAndSources(graph treeGraph, commitSHA string) (map[string]models.GraphNode, map[string]models.NodeCodeResponse) {
	nodes := map[string]models.GraphNode{}
	sources := map[string]models.NodeCodeResponse{}
	for id, obj := range graph.nodes {
		node := graphNodeFromObject(id, obj, "context")
		nodes[id] = node
		sources[id] = models.NodeCodeResponse{
			NodeID:   id,
			FilePath: obj.FilePath,
			Language: obj.Language,
			Head: &models.NodeCodeSegment{
				CommitSHA: commitSHA,
				StartLine: obj.LineStart,
				EndLine:   obj.LineEnd,
				Source:    obj.Body,
			},
		}
	}
	resolveLocalTypeRefs(nodes)
	return nodes, sources
}

func graphEdges(graph treeGraph) []models.GraphEdge {
	var edges []models.GraphEdge
	seen := map[string]bool{}
	for _, edge := range graph.edges {
		source := resolveEdgeNodeID(edge.SourceRef, graph, graph)
		target := resolveEdgeNodeID(edge.TargetRef, graph, graph)
		if source == "" || target == "" || source == target {
			continue
		}
		key := source + "|" + target + "|" + edge.Kind
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, models.GraphEdge{SourceID: source, DestinationID: target, EdgeKind: edge.Kind, Weight: 1, UnderlyingEdgeCount: 1})
	}
	return edges
}

func localPrograms(nodes map[string]models.GraphNode) []models.GraphProgram {
	var programs []models.GraphProgram
	for _, node := range nodes {
		if node.IsTest {
			continue
		}
		name := lastFullNameSegment(node.FullName)
		if name != "main" && !node.IsEntrypoint {
			continue
		}
		programs = append(programs, models.GraphProgram{
			ID:           node.ID,
			FullName:     node.FullName,
			FilePath:     node.FilePath,
			PackagePath:  node.PackagePath,
			LineStart:    node.LineStart,
			LineEnd:      node.LineEnd,
			Language:     node.Language,
			Kind:         node.Kind,
			IsEntrypoint: true,
		})
	}
	if len(programs) == 0 {
		for _, node := range nodes {
			if node.IsTest {
				continue
			}
			programs = append(programs, models.GraphProgram{
				ID: node.ID, FullName: node.FullName, FilePath: node.FilePath, PackagePath: node.PackagePath,
				LineStart: node.LineStart, LineEnd: node.LineEnd, Language: node.Language, Kind: node.Kind,
			})
			break
		}
	}
	sort.Slice(programs, func(i, j int) bool {
		if programs[i].FilePath != programs[j].FilePath {
			return programs[i].FilePath < programs[j].FilePath
		}
		return programs[i].FullName < programs[j].FullName
	})
	return programs
}

func boundedNodeIDs(seed string, edges []models.GraphEdge, depth int) map[string]bool {
	selected := map[string]bool{}
	if seed == "" {
		return selected
	}
	selected[seed] = true
	queue := []string{seed}
	distance := map[string]int{seed: 0}
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		if distance[id] >= depth {
			continue
		}
		for _, edge := range edges {
			next := ""
			if edge.SourceID == id {
				next = edge.DestinationID
			} else if edge.DestinationID == id {
				next = edge.SourceID
			}
			if next == "" || selected[next] {
				continue
			}
			selected[next] = true
			distance[next] = distance[id] + 1
			queue = append(queue, next)
			if len(selected) >= 150 {
				return selected
			}
		}
	}
	return selected
}

func filterGraphEdges(edges []models.GraphEdge, selected map[string]bool) []models.GraphEdge {
	out := []models.GraphEdge{}
	for _, edge := range edges {
		if selected[edge.SourceID] && selected[edge.DestinationID] {
			out = append(out, edge)
		}
	}
	return out
}

func resolveLocalTypeRefs(nodes map[string]models.GraphNode) {
	typeIDByName := map[string]string{}
	for id, node := range nodes {
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		typeIDByName[node.FullName] = id
		typeIDByName[lastFullNameSegment(node.FullName)] = id
	}
	resolve := func(refs []models.TypeRef) []models.TypeRef {
		out := make([]models.TypeRef, len(refs))
		copy(out, refs)
		for i := range out {
			if id := typeIDByName[baseTypeName(out[i].Type)]; id != "" {
				nodeID := id
				out[i].NodeID = &nodeID
			}
		}
		return out
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

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func lastFullNameSegment(fullName string) string {
	if dot := strings.LastIndex(fullName, "."); dot >= 0 {
		return fullName[dot+1:]
	}
	return fullName
}
