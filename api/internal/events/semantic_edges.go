package events

import (
	"strings"

	"github.com/isoprism/api/internal/parser"
)

type semanticEdge struct {
	SourceFullName      string
	DestinationFullName string
	Kind                string
}

const (
	edgeKindCalls      = "calls"
	edgeKindOwnsMethod = "owns_method"
)

func receiverOwnershipEdges(nodes []parser.Node) []semanticEdge {
	typeNames := map[string]bool{}
	for _, node := range nodes {
		if node.Kind == "struct" || node.Kind == "type" || node.Kind == "interface" {
			typeNames[node.FullName] = true
		}
	}
	if len(typeNames) == 0 {
		return nil
	}

	var edges []semanticEdge
	for _, node := range nodes {
		if node.Kind != "method" {
			continue
		}
		owner, ok := methodOwnerFullName(node.FullName, typeNames)
		if !ok {
			continue
		}
		edges = append(edges, semanticEdge{
			SourceFullName:      owner,
			DestinationFullName: node.FullName,
			Kind:                edgeKindOwnsMethod,
		})
	}
	return edges
}

func methodOwnerFullName(methodFullName string, typeNames map[string]bool) (string, bool) {
	for {
		idx := strings.LastIndex(methodFullName, ".")
		if idx < 0 {
			return "", false
		}
		methodFullName = methodFullName[:idx]
		if typeNames[methodFullName] {
			return methodFullName, true
		}
	}
}
