package events

import (
	"testing"

	"github.com/isoprism/api/internal/parser"
)

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

func TestTypeUsageEdgesConnectStructToResolvableFieldTypes(t *testing.T) {
	nodes := []parser.Node{
		{FullName: "rpc/grpc:coregrpc.BlockAPI", Kind: "struct", Fields: []parser.Param{
			{Name: "env", Type: "*core.Environment"},
			{Name: "response", Type: "SubscribeNewHeightsResponse"},
			{Name: "external", Type: "sync.Mutex"},
		}},
		{FullName: "rpc/core:core.Environment", Kind: "struct"},
		{FullName: "rpc/grpc:coregrpc.SubscribeNewHeightsResponse", Kind: "struct"},
	}

	edges := typeUsageEdges(nodes, nodes)

	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/core:core.Environment", edgeKindUsesType) {
		t.Fatalf("missing BlockAPI -> Environment type usage edge: %#v", edges)
	}
	if !hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "rpc/grpc:coregrpc.SubscribeNewHeightsResponse", edgeKindUsesType) {
		t.Fatalf("missing BlockAPI -> SubscribeNewHeightsResponse type usage edge: %#v", edges)
	}
	if hasSemanticEdge(edges, "rpc/grpc:coregrpc.BlockAPI", "sync.Mutex", edgeKindUsesType) {
		t.Fatalf("external field type should not produce a graph edge: %#v", edges)
	}
}

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

func hasSemanticEdge(edges []semanticEdge, source, destination, kind string) bool {
	for _, edge := range edges {
		if edge.SourceFullName == source && edge.DestinationFullName == destination && edge.Kind == kind {
			return true
		}
	}
	return false
}
