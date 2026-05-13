package handlers

import (
	"testing"

	"github.com/isoprism/api/internal/models"
)

func TestBaseLookupIdentityUsesOldIdentityForRenames(t *testing.T) {
	changeType := "renamed"
	oldFullName := "BlockAPI.closeAllListeners"
	oldFilePath := "rpc/grpc/api.go"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		&oldFullName,
		&oldFilePath,
	)

	if gotName != oldFullName {
		t.Fatalf("base lookup full name = %q, want %q", gotName, oldFullName)
	}
	if gotPath != oldFilePath {
		t.Fatalf("base lookup file path = %q, want %q", gotPath, oldFilePath)
	}
}

func TestBaseLookupIdentityFallsBackWhenRenameMetadataMissing(t *testing.T) {
	changeType := "renamed"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		nil,
		nil,
	)

	if gotName != "BlockAPI.closeAllListenersLocked" {
		t.Fatalf("base lookup full name = %q", gotName)
	}
	if gotPath != "rpc/grpc/api.go" {
		t.Fatalf("base lookup file path = %q", gotPath)
	}
}

func TestBaseLookupIdentityIgnoresOldIdentityForNonRenames(t *testing.T) {
	changeType := "modified"
	oldFullName := "BlockAPI.closeAllListeners"
	oldFilePath := "rpc/grpc/old_api.go"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		&oldFullName,
		&oldFilePath,
	)

	if gotName != "BlockAPI.closeAllListenersLocked" {
		t.Fatalf("base lookup full name = %q", gotName)
	}
	if gotPath != "rpc/grpc/api.go" {
		t.Fatalf("base lookup file path = %q", gotPath)
	}
}

func TestAppendTestFocusEdgesKeepsChangedTestHelpersReachable(t *testing.T) {
	edges := appendTestFocusEdges(
		[]models.GraphEdge{{SourceID: "prod-a", DestinationID: "prod-b", EdgeKind: "calls"}},
		[]graphEdgeRow{
			{sourceID: "test-entry", destinationID: "test-helper", edgeKind: "calls"},
			{sourceID: "test-helper", destinationID: "prod-a", edgeKind: "calls"},
			{sourceID: "prod-a", destinationID: "test-helper", edgeKind: "calls"},
			{sourceID: "test-entry", destinationID: "other-prod", edgeKind: "calls"},
		},
		[]models.GraphNode{
			{ID: "test-entry", IsTest: true, IsEntrypoint: true},
			{ID: "test-helper", IsTest: true, IsEntrypoint: false},
		},
		map[string]models.GraphNode{"prod-a": {ID: "prod-a"}},
		map[string]models.GraphNode{},
		func(id string) string { return id },
	)

	if !hasGraphEdge(edges, "test-entry", "test-helper") {
		t.Fatalf("missing test entrypoint -> helper edge: %#v", edges)
	}
	if !hasGraphEdge(edges, "test-helper", "prod-a") {
		t.Fatalf("missing helper -> visible production edge: %#v", edges)
	}
	if hasGraphEdge(edges, "prod-a", "test-helper") {
		t.Fatalf("production -> test helper edge should not be added: %#v", edges)
	}
	if hasGraphEdge(edges, "test-entry", "other-prod") {
		t.Fatalf("edge to non-visible production node should not be added: %#v", edges)
	}
}

func TestEdgeChangesOnlyApplyToChangedProductionEndpoints(t *testing.T) {
	changedNames := map[string]bool{
		"rpc/grpc:coregrpc.BlockAPI.Stop": true,
	}
	baseOnly := map[string]bool{"base": true}

	gotChanged := markEdgeChangeType(graphEdgeRow{
		sourceName:      "rpc/grpc:coregrpc.BlockAPI.Stop",
		destinationName: "types:types.EventBus.Unsubscribe",
		edgeKind:        "calls",
	}, baseOnly, changedNames)
	if gotChanged != "deleted" {
		t.Fatalf("edge touching changed node = %q, want deleted", gotChanged)
	}

	gotUnchangedCaller := markEdgeChangeType(graphEdgeRow{
		sourceName:      "rpc/grpc:coregrpc.StartGRPCServer",
		destinationName: "rpc/grpc:coregrpc.BlockAPI.Stop",
		edgeKind:        "calls",
	}, baseOnly, changedNames)
	if gotUnchangedCaller != "unchanged" {
		t.Fatalf("edge from unchanged caller to changed callee = %q, want unchanged", gotUnchangedCaller)
	}

	gotContext := markEdgeChangeType(graphEdgeRow{
		sourceName:      "rpc/client/local:local.Local.Unsubscribe",
		destinationName: "types:types.EventBus.Unsubscribe",
		edgeKind:        "calls",
	}, baseOnly, changedNames)
	if gotContext != "unchanged" {
		t.Fatalf("context edge = %q, want unchanged", gotContext)
	}

	if relevantProductionEdge(graphEdgeRow{
		sourceName:      "rpc/client/local:local.Local.Unsubscribe",
		destinationName: "types:types.EventBus.Unsubscribe",
		edgeKind:        "calls",
		changeType:      "unchanged",
	}, changedNames) {
		t.Fatalf("unchanged context-to-context edge should not expand the PR graph")
	}
}

func TestSelectVisibleGraphIncludesReceiverOwnerEdges(t *testing.T) {
	selected, edges := selectVisibleGraph(
		[]string{"stop"},
		[]graphEdgeRow{
			{sourceID: "block-api", destinationID: "stop", edgeKind: "owns_method"},
		},
		map[string]int{"stop": 4},
	)

	if _, ok := selected["block-api"]; !ok {
		t.Fatalf("receiver owner type was not selected: %#v", selected)
	}
	if !hasGraphEdge(edges, "block-api", "stop") {
		t.Fatalf("missing ownership graph edge: %#v", edges)
	}
	if edges[0].EdgeKind != "owns_method" {
		t.Fatalf("edge kind = %q, want owns_method", edges[0].EdgeKind)
	}
}

func TestSelectVisibleGraphIncludesChangedNodeCallersAndCallees(t *testing.T) {
	selected, edges := selectVisibleGraph(
		[]string{"changed"},
		[]graphEdgeRow{
			{sourceID: "caller", destinationID: "changed", edgeKind: "calls"},
			{sourceID: "changed", destinationID: "callee", edgeKind: "calls"},
			{sourceID: "callee", destinationID: "second-hop", edgeKind: "calls"},
		},
		map[string]int{"changed": 8},
	)

	for _, id := range []string{"changed", "caller", "callee"} {
		if _, ok := selected[id]; !ok {
			t.Fatalf("expected %s to be selected: %#v", id, selected)
		}
	}
	if _, ok := selected["second-hop"]; ok {
		t.Fatalf("second-hop context node should not be selected: %#v", selected)
	}
	if !hasGraphEdge(edges, "caller", "changed") {
		t.Fatalf("missing incoming caller edge: %#v", edges)
	}
	if !hasGraphEdge(edges, "changed", "callee") {
		t.Fatalf("missing outgoing callee edge: %#v", edges)
	}
	if hasGraphEdge(edges, "callee", "second-hop") {
		t.Fatalf("edge to second-hop node should not be visible: %#v", edges)
	}
	if selected["changed"].depth != 0 {
		t.Fatalf("changed depth = %d, want 0", selected["changed"].depth)
	}
	if selected["caller"].depth != 1 || selected["callee"].depth != 1 {
		t.Fatalf("one-hop depths = caller %d callee %d, want 1", selected["caller"].depth, selected["callee"].depth)
	}
}

func TestSelectExpansionNeighborsSkipsVisibleAndRanksChangedNodes(t *testing.T) {
	changeType := "modified"
	nodeMap := map[string]models.GraphNode{
		"expanded": {ID: "expanded", FullName: "pkg.Expanded", FilePath: "pkg/a.go", PackagePath: "pkg"},
		"visible":  {ID: "visible", FullName: "pkg.Visible", FilePath: "pkg/a.go", PackagePath: "pkg"},
		"changed":  {ID: "changed", FullName: "pkg.Changed", FilePath: "pkg/b.go", PackagePath: "pkg", ChangeType: &changeType},
		"entry":    {ID: "entry", FullName: "main", FilePath: "cmd/main.go", PackagePath: "cmd", IsEntrypoint: true},
		"far":      {ID: "far", FullName: "other.Far", FilePath: "other/far.go", PackagePath: "other"},
	}
	visible := graphVisibleSet([]string{"expanded", "visible"}, "expanded")
	neighbors, hiddenCount, hasMore := selectExpansionNeighbors("expanded", visible, nodeMap, []graphEdgeRow{
		{sourceID: "visible", destinationID: "expanded", edgeKind: "calls"},
		{sourceID: "changed", destinationID: "expanded", edgeKind: "calls"},
		{sourceID: "expanded", destinationID: "entry", edgeKind: "calls"},
		{sourceID: "expanded", destinationID: "far", edgeKind: "calls"},
		{sourceID: "far", destinationID: "entry", edgeKind: "calls"},
	})

	if hasMore {
		t.Fatalf("hasMore = true for small neighborhood")
	}
	if hiddenCount != 3 {
		t.Fatalf("hiddenCount = %d, want 3", hiddenCount)
	}
	if len(neighbors) != 3 {
		t.Fatalf("neighbors = %#v, want 3 hidden nodes", neighbors)
	}
	if neighbors[0] != "changed" {
		t.Fatalf("first neighbor = %q, want changed node first; all neighbors %#v", neighbors[0], neighbors)
	}
	for _, id := range neighbors {
		if id == "visible" {
			t.Fatalf("visible node should not be returned: %#v", neighbors)
		}
	}
}

func TestGraphEdgesForVisibleSetIncludesNewAndExistingEdges(t *testing.T) {
	visible := graphVisibleSet([]string{"expanded", "existing"}, "expanded")
	visible["new"] = true
	edges := graphEdgesForVisibleSet([]graphEdgeRow{
		{sourceID: "existing", destinationID: "expanded", edgeKind: "calls"},
		{sourceID: "expanded", destinationID: "new", edgeKind: "calls"},
		{sourceID: "new", destinationID: "outside", edgeKind: "calls"},
		{sourceID: "expanded", destinationID: "new", edgeKind: "calls"},
	}, visible)

	if len(edges) != 2 {
		t.Fatalf("edges = %#v, want only two visible deduped edges", edges)
	}
	if !hasGraphEdge(edges, "existing", "expanded") {
		t.Fatalf("missing existing -> expanded edge: %#v", edges)
	}
	if !hasGraphEdge(edges, "expanded", "new") {
		t.Fatalf("missing expanded -> new edge: %#v", edges)
	}
}

func TestGraphHasHiddenNeighborClearsBoundaryWhenFullyVisible(t *testing.T) {
	visible := graphVisibleSet([]string{"expanded", "neighbor"}, "expanded")
	if graphHasHiddenNeighbor("expanded", visible, []graphEdgeRow{{sourceID: "expanded", destinationID: "neighbor", edgeKind: "calls"}}) {
		t.Fatalf("expanded node should not have hidden neighbors")
	}
	if !graphHasHiddenNeighbor("expanded", visible, []graphEdgeRow{{sourceID: "expanded", destinationID: "outside", edgeKind: "calls"}}) {
		t.Fatalf("expanded node should have hidden neighbors")
	}
}

func hasGraphEdge(edges []models.GraphEdge, sourceID, destinationID string) bool {
	for _, edge := range edges {
		if edge.SourceID == sourceID && edge.DestinationID == destinationID {
			return true
		}
	}
	return false
}
