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
		[]models.GraphEdge{{CallerID: "prod-a", CalleeID: "prod-b"}},
		[]graphEdgeRow{
			{callerID: "test-entry", calleeID: "test-helper"},
			{callerID: "test-helper", calleeID: "prod-a"},
			{callerID: "prod-a", calleeID: "test-helper"},
			{callerID: "test-entry", calleeID: "other-prod"},
		},
		[]models.GraphNode{
			{ID: "test-entry", IsTestCode: true, IsTestEntrypoint: true},
			{ID: "test-helper", IsTestCode: true, IsTestEntrypoint: false},
		},
		map[string]models.GraphNode{"prod-a": {ID: "prod-a"}},
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

func hasGraphEdge(edges []models.GraphEdge, callerID, calleeID string) bool {
	for _, edge := range edges {
		if edge.CallerID == callerID && edge.CalleeID == calleeID {
			return true
		}
	}
	return false
}
