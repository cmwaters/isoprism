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
	edgeKindUsesType   = "uses_type"
)

func semanticTypeEdges(nodes []parser.Node) []semanticEdge {
	return semanticTypeEdgesWithKnownTypes(nodes, nodes)
}

func semanticTypeEdgesWithKnownTypes(nodes []parser.Node, knownTypes []parser.Node) []semanticEdge {
	edges := receiverOwnershipEdges(nodes)
	edges = append(edges, typeUsageEdges(nodes, knownTypes)...)
	return edges
}

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

func typeUsageEdges(nodes []parser.Node, knownTypes []parser.Node) []semanticEdge {
	typeNames := map[string]bool{}
	byShortName := map[string]string{}
	ambiguousShortNames := map[string]bool{}
	for _, node := range knownTypes {
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		typeNames[node.FullName] = true
		short := lastTypeSegment(node.FullName)
		if existing, ok := byShortName[short]; ok && existing != node.FullName {
			ambiguousShortNames[short] = true
			continue
		}
		byShortName[short] = node.FullName
	}
	if len(typeNames) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var edges []semanticEdge
	for _, node := range nodes {
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		for _, field := range node.Fields {
			for _, targetShort := range typeSegments(field.Type) {
				if ambiguousShortNames[targetShort] {
					continue
				}
				target := byShortName[targetShort]
				if target == "" || target == node.FullName {
					continue
				}
				key := node.FullName + "\x00" + target
				if seen[key] {
					continue
				}
				seen[key] = true
				edges = append(edges, semanticEdge{
					SourceFullName:      node.FullName,
					DestinationFullName: target,
					Kind:                edgeKindUsesType,
				})
			}
		}
	}
	return edges
}

func typeSegments(typeName string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.FieldsFunc(typeName, func(r rune) bool {
		switch r {
		case '*', '[', ']', '{', '}', '(', ')', ',', ' ', '\t', '\n', '\r', '<', '-':
			return true
		default:
			return false
		}
	}) {
		segment := lastTypeSegment(raw)
		if segment == "" || isBuiltinGoTypeSegment(segment) || seen[segment] {
			continue
		}
		seen[segment] = true
		out = append(out, segment)
	}
	return out
}

func isBuiltinGoTypeSegment(segment string) bool {
	switch segment {
	case "any", "bool", "byte", "comparable", "complex64", "complex128", "error",
		"float32", "float64", "int", "int8", "int16", "int32", "int64",
		"rune", "string", "struct", "uint", "uint8", "uint16", "uint32",
		"uint64", "uintptr", "map", "chan", "func", "interface":
		return true
	default:
		return false
	}
}

func lastTypeSegment(typeName string) string {
	t := strings.TrimSpace(typeName)
	for strings.HasPrefix(t, "*") {
		t = strings.TrimPrefix(t, "*")
	}
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
		t = t[dot+1:]
	}
	if colon := strings.LastIndex(t, ":"); colon >= 0 {
		t = t[colon+1:]
	}
	return t
}
