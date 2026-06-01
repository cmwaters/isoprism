package events

import (
	"testing"

	"github.com/isoprism/api/internal/parser"
)

// TestReceiverOwnershipEdgesConnectTypeToMethods verifies receiver ownership edges connect type to methods.
func TestReceiverOwnershipEdgesConnectTypeToMethods(t *testing.T) {
	nodes := []parser.Node{
		{FullName: "rpc/grpc:coregrpc.BlockAPI", Kind: "struct"},
		{FullName: "rpc/grpc:coregrpc.BlockAPI.Stop", Kind: "method"},
		{FullName: "rpc/grpc:coregrpc.BlockAPI.closeAllListenersLocked", Kind: "method"},
		{FullName: "rpc/grpc:coregrpc.NewBlockAPI", Kind: "function"},
	}

	edges := receiverOwnershipEdges(nodes)

	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/grpc:coregrpc.BlockAPI.Stop", edgeKindOwnsMethod) {
		t.Fatalf("missing BlockAPI -> Stop ownership edge: %#v", edges)
	}
	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/grpc:coregrpc.BlockAPI.closeAllListenersLocked", edgeKindOwnsMethod) {
		t.Fatalf("missing BlockAPI -> closeAllListenersLocked ownership edge: %#v", edges)
	}
	if hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/grpc:coregrpc.NewBlockAPI", edgeKindOwnsMethod) {
		t.Fatalf("function should not be owned as a method: %#v", edges)
	}
}

// TestTypeUsageEdgesConnectStructToResolvableFieldTypes verifies type usage edges connect struct to resolvable field types.
func TestTypeUsageEdgesConnectStructToResolvableFieldTypes(t *testing.T) {
	nodes := []parser.Node{
		{FullName: "rpc/grpc:coregrpc.BlockAPI", Kind: "struct", Fields: []parser.Param{
			{Name: "env", Type: "*core.Environment"},
			{Name: "heightListeners", Type: "map[chan SubscribeNewHeightsResponse]struct{}"},
			{Name: "subscription", Type: "eventstypes.Subscription"},
			{Name: "query", Type: "pubsub.Query"},
			{Name: "external", Type: "sync.Mutex"},
			{Name: "id", Type: "string"},
		}},
		{FullName: "rpc/core:core.Environment", Kind: "struct"},
		{FullName: "rpc/grpc:coregrpc.SubscribeNewHeightsResponse", Kind: "struct"},
		{FullName: "libs/pubsub:pubsub.Query", Kind: "type"},
	}

	edges := typeUsageEdges(nodes, nodes)

	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/core:core.Environment", edgeKindUsesType) {
		t.Fatalf("missing BlockAPI -> Environment type usage edge: %#v", edges)
	}
	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/grpc:coregrpc.SubscribeNewHeightsResponse", edgeKindUsesType) {
		t.Fatalf("missing BlockAPI -> SubscribeNewHeightsResponse type usage edge: %#v", edges)
	}
	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "libs/pubsub:pubsub.Query", edgeKindUsesType) {
		t.Fatalf("missing BlockAPI -> Query type usage edge: %#v", edges)
	}
	if hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "sync.Mutex", edgeKindUsesType) {
		t.Fatalf("external field type should not produce a graph edge: %#v", edges)
	}
	if hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "string", edgeKindUsesType) {
		t.Fatalf("builtin field type should not produce a graph edge: %#v", edges)
	}
}

// TestTypeUsageEdgesSkipAmbiguousShortTypeNames verifies type usage edges skip ambiguous short type names.
func TestTypeUsageEdgesSkipAmbiguousShortTypeNames(t *testing.T) {
	nodes := []parser.Node{
		{FullName: "pkg:pkg.Owner", Kind: "struct", Fields: []parser.Param{{Name: "target", Type: "Target"}}},
		{FullName: "one:one.Target", Kind: "struct"},
		{FullName: "two:two.Target", Kind: "struct"},
	}

	edges := typeUsageEdges(nodes, nodes)

	if len(edges) != 0 {
		t.Fatalf("ambiguous Target type should not produce edges: %#v", edges)
	}
}

// hasSemanticEdge reports whether semantic edge is present.
func hasSemanticEdge(edges []semanticEdge, source, destination, kind string) bool {
	for _, edge := range edges {
		if edge.SourceFullName == source && edge.DestinationFullName == destination && edge.Kind == kind {
			return true
		}
	}
	return false
}
