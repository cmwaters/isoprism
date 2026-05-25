package localgraph

import "strings"

func semanticTypeEdges(nodes []graphNodeObject) []semanticEdge {
	typeRefs := map[string]graphNodeObject{}
	shortRefs := map[string]string{}
	ambiguous := map[string]bool{}
	for _, node := range nodes {
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		ref := semanticRef(node.FilePath, node.FullName)
		typeRefs[node.FullName] = node
		short := lastTypeSegment(node.FullName)
		if existing, ok := shortRefs[short]; ok && existing != ref {
			ambiguous[short] = true
			continue
		}
		shortRefs[short] = ref
	}
	var edges []semanticEdge
	for _, node := range nodes {
		if node.Kind == "method" {
			if ownerRef := methodOwnerRef(node.FullName, typeRefs); ownerRef != "" {
				edges = append(edges, semanticEdge{SourceRef: ownerRef, TargetRef: semanticRef(node.FilePath, node.FullName), Kind: "owns_method"})
			}
		}
		if node.Kind != "struct" && node.Kind != "type" && node.Kind != "interface" {
			continue
		}
		sourceRef := semanticRef(node.FilePath, node.FullName)
		seen := map[string]bool{}
		for _, field := range node.Fields {
			for _, segment := range typeSegments(field.Type) {
				if ambiguous[segment] {
					continue
				}
				targetRef := shortRefs[segment]
				if targetRef == "" || targetRef == sourceRef || seen[targetRef] {
					continue
				}
				seen[targetRef] = true
				edges = append(edges, semanticEdge{SourceRef: sourceRef, TargetRef: targetRef, Kind: "uses_type"})
			}
		}
	}
	return edges
}

func methodOwnerRef(methodFullName string, typeRefs map[string]graphNodeObject) string {
	for {
		idx := strings.LastIndex(methodFullName, ".")
		if idx < 0 {
			return ""
		}
		methodFullName = methodFullName[:idx]
		if owner, ok := typeRefs[methodFullName]; ok {
			return semanticRef(owner.FilePath, owner.FullName)
		}
	}
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
