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

func hasSemanticEdge(edges []semanticEdge, source, destination, kind string) bool {
	for _, edge := range edges {
		if edge.SourceFullName == source && edge.DestinationFullName == destination && edge.Kind == kind {
			return true
		}
	}
	return false
}
