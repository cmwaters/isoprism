package parser

import (
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ResolverIndex is a language-neutral home for semantic facts collected before
// edge extraction. The first adapter implemented here is Go; other languages
// should add their own symbol/type facts without changing callgraph callers.
type ResolverIndex struct {
	NodeByName map[string]bool
	GoTypes    map[string]GoTypeInfo
}

type GoTypeInfo struct {
	FullName string
	Fields   map[string]string
}

// BuildResolverIndex extracts reusable symbol facts from a repository snapshot.
// fileContents should contain full source files, not function snippets.
func BuildResolverIndex(fileContents map[string][]byte, nodeByName map[string]bool) ResolverIndex {
	index := ResolverIndex{
		NodeByName: nodeByName,
		GoTypes:    map[string]GoTypeInfo{},
	}
	if index.NodeByName == nil {
		index.NodeByName = map[string]bool{}
	}

	for filePath, src := range fileContents {
		if languageFor(filePath) != "go" {
			continue
		}
		pf, ok := parseTree(src, filePath)
		if !ok {
			continue
		}
		extractGoTypeInfo(src, filePath, pf.root, index.GoTypes, index.NodeByName)
		pf.tree.Close()
	}
	return index
}

// GoImportDirSuffixes returns repository-relative directory suffix candidates
// for Go imports in a file. Callers can use these to fetch package files from a
// repo-local file list without knowing the module path.
func GoImportDirSuffixes(src []byte, filePath string) map[string]bool {
	out := map[string]bool{}
	if languageFor(filePath) != "go" {
		return out
	}
	pf, ok := parseTree(src, filePath)
	if !ok {
		return out
	}
	defer pf.tree.Close()

	for _, importPath := range goImports(src, pf.root) {
		parts := strings.Split(filepath.ToSlash(strings.Trim(importPath, `"`)), "/")
		for i := range parts {
			suffix := strings.Join(parts[i:], "/")
			if suffix != "" {
				out[suffix] = true
			}
		}
	}
	return out
}

func extractGoTypeInfo(src []byte, filePath string, root *sitter.Node, out map[string]GoTypeInfo, nodeByName map[string]bool) {
	pkg := goPackageName(src, root)
	prefix := goPackagePrefix(filePath, pkg)
	imports := goImports(src, root)

	walk(root, func(n *sitter.Node) bool {
		if n.Kind() != "type_declaration" {
			return true
		}
		forEachDescendant(n, "type_spec", func(spec *sitter.Node) {
			nameNode := spec.ChildByFieldName("name")
			typeNode := spec.ChildByFieldName("type")
			if nameNode == nil || typeNode == nil || typeNode.Kind() != "struct_type" {
				return
			}
			fullName := prefix + "." + text(src, nameNode)
			info := GoTypeInfo{FullName: fullName, Fields: map[string]string{}}
			forEachDescendant(typeNode, "field_declaration", func(field *sitter.Node) {
				fieldTypeNode := field.ChildByFieldName("type")
				if fieldTypeNode == nil {
					return
				}
				fieldType := resolveGoTypeExpr(text(src, fieldTypeNode), prefix, imports, nodeByName)
				if fieldType == "" {
					return
				}
				names := goFieldNames(src, field, fieldTypeNode)
				for _, name := range names {
					info.Fields[name] = fieldType
				}
			})
			out[fullName] = info
		})
		return false
	})
}

func goFieldNames(src []byte, field *sitter.Node, typeNode *sitter.Node) []string {
	var names []string
	for i := uint(0); i < field.NamedChildCount(); i++ {
		child := field.NamedChild(i)
		if child == nil || sameTreeSitterNode(child, typeNode) {
			break
		}
		switch child.Kind() {
		case "field_identifier", "identifier":
			names = append(names, text(src, child))
		}
	}
	if len(names) > 0 {
		return names
	}
	if embedded := embeddedGoFieldName(src, typeNode); embedded != "" {
		return []string{embedded}
	}
	return nil
}

func sameTreeSitterNode(a, b *sitter.Node) bool {
	if a == nil || b == nil {
		return false
	}
	return a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte() && a.Kind() == b.Kind()
}

func embeddedGoFieldName(src []byte, typeNode *sitter.Node) string {
	typeText := strings.TrimPrefix(strings.TrimSpace(text(src, typeNode)), "*")
	typeText = strings.TrimPrefix(typeText, "[]")
	if idx := strings.LastIndex(typeText, "."); idx >= 0 {
		return typeText[idx+1:]
	}
	return typeText
}

type goScope map[string]string

func buildGoScope(src []byte, fn *sitter.Node, prefix string, imports map[string]string, index ResolverIndex) goScope {
	scope := goScope{}
	if fn.Kind() == "method_declaration" {
		name, typ := goReceiverBinding(src, fn.ChildByFieldName("receiver"))
		if name != "" && typ != "" {
			if resolved := resolveGoTypeExpr(typ, prefix, imports, index.NodeByName); resolved != "" {
				scope[name] = resolved
			}
		}
	}
	addGoParamBindings(src, fn.ChildByFieldName("parameters"), prefix, imports, index.NodeByName, scope)
	if body := fn.ChildByFieldName("body"); body != nil {
		addGoLocalBindings(src, body, prefix, imports, index.NodeByName, scope)
	}
	return scope
}

func goReceiverBinding(src []byte, receiver *sitter.Node) (string, string) {
	if receiver == nil {
		return "", ""
	}
	var name, typ string
	forEachDescendant(receiver, "parameter_declaration", func(n *sitter.Node) {
		if typ == "" {
			if typeNode := n.ChildByFieldName("type"); typeNode != nil {
				typ = text(src, typeNode)
			}
		}
		if name == "" {
			for i := uint(0); i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child != nil && child.Kind() == "identifier" {
					name = text(src, child)
					break
				}
			}
		}
	})
	return name, typ
}

func addGoParamBindings(src []byte, params *sitter.Node, prefix string, imports map[string]string, nodeByName map[string]bool, scope goScope) {
	if params == nil {
		return
	}
	forEachDescendant(params, "parameter_declaration", func(n *sitter.Node) {
		typeNode := n.ChildByFieldName("type")
		if typeNode == nil {
			return
		}
		resolved := resolveGoTypeExpr(text(src, typeNode), prefix, imports, nodeByName)
		if resolved == "" {
			return
		}
		for _, name := range childTextsByKinds(src, n, "identifier") {
			scope[name] = resolved
		}
	})
}

func addGoLocalBindings(src []byte, body *sitter.Node, prefix string, imports map[string]string, nodeByName map[string]bool, scope goScope) {
	walk(body, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "var_spec":
			typeNode := n.ChildByFieldName("type")
			if typeNode == nil {
				return true
			}
			resolved := resolveGoTypeExpr(text(src, typeNode), prefix, imports, nodeByName)
			if resolved == "" {
				return true
			}
			for _, name := range childTextsByKinds(src, n, "identifier") {
				scope[name] = resolved
			}
		case "short_var_declaration":
			left := n.ChildByFieldName("left")
			right := n.ChildByFieldName("right")
			if left == nil || right == nil {
				return true
			}
			names := childTextsByKinds(src, left, "identifier")
			values := goExpressionListChildren(right)
			for i, name := range names {
				if i >= len(values) {
					continue
				}
				if resolved := inferGoExprType(src, values[i], prefix, imports, nodeByName); resolved != "" {
					scope[name] = resolved
				}
			}
		}
		return true
	})
}

func goExpressionListChildren(n *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	if n.Kind() != "expression_list" {
		return []*sitter.Node{n}
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); child != nil {
			out = append(out, child)
		}
	}
	return out
}

func inferGoExprType(src []byte, expr *sitter.Node, prefix string, imports map[string]string, nodeByName map[string]bool) string {
	if expr == nil {
		return ""
	}
	if expr.Kind() == "unary_expression" && expr.NamedChildCount() > 0 {
		return inferGoExprType(src, expr.NamedChild(0), prefix, imports, nodeByName)
	}
	if expr.Kind() == "composite_literal" {
		if typ := expr.ChildByFieldName("type"); typ != nil {
			return resolveGoTypeExpr(text(src, typ), prefix, imports, nodeByName)
		}
	}
	return ""
}

func resolveGoTypeExpr(typeExpr, prefix string, imports map[string]string, nodeByName map[string]bool) string {
	typeExpr = cleanGoTypeExpr(typeExpr)
	if typeExpr == "" {
		return ""
	}
	if strings.Contains(typeExpr, ".") {
		parts := strings.Split(typeExpr, ".")
		if len(parts) != 2 {
			return ""
		}
		if importPath, ok := imports[parts[0]]; ok {
			return resolveImportedGoSelector(importPath, parts[1], nodeByName)
		}
		return known(prefix+"."+typeExpr, nodeByName)
	}
	if callee := known(prefix+"."+typeExpr, nodeByName); callee != "" {
		return callee
	}
	return known(typeExpr, nodeByName)
}

func cleanGoTypeExpr(typeExpr string) string {
	typeExpr = strings.TrimSpace(typeExpr)
	for {
		switch {
		case strings.HasPrefix(typeExpr, "*"):
			typeExpr = strings.TrimSpace(strings.TrimPrefix(typeExpr, "*"))
		case strings.HasPrefix(typeExpr, "[]"):
			typeExpr = strings.TrimSpace(strings.TrimPrefix(typeExpr, "[]"))
		default:
			return typeExpr
		}
	}
}

func goSelectorParts(src []byte, n *sitter.Node) []string {
	if n == nil {
		return nil
	}
	switch n.Kind() {
	case "identifier", "package_identifier", "field_identifier":
		return []string{text(src, n)}
	case "selector_expression":
		operand := n.ChildByFieldName("operand")
		if operand == nil {
			operand = n.ChildByFieldName("object")
		}
		field := n.ChildByFieldName("field")
		if field == nil {
			field = n.ChildByFieldName("property")
		}
		parts := goSelectorParts(src, operand)
		if field != nil {
			parts = append(parts, text(src, field))
		}
		return parts
	default:
		return nil
	}
}

func resolveGoFieldChainCall(src []byte, fun *sitter.Node, index ResolverIndex, scope goScope) string {
	if scope == nil || len(index.GoTypes) == 0 {
		return ""
	}
	parts := goSelectorParts(src, fun)
	if len(parts) < 2 {
		return ""
	}
	rootType := scope[parts[0]]
	if rootType == "" {
		return ""
	}
	currentType := rootType
	for _, field := range parts[1 : len(parts)-1] {
		info, ok := index.GoTypes[currentType]
		if !ok {
			return ""
		}
		nextType := info.Fields[field]
		if nextType == "" {
			return ""
		}
		currentType = nextType
	}
	methodFullName := currentType + "." + parts[len(parts)-1]
	return known(methodFullName, index.NodeByName)
}

func repoRelativeImportSuffix(importPath, selector string) []string {
	cleanPath := filepath.ToSlash(strings.Trim(importPath, `"`))
	pkg := filepath.Base(cleanPath)
	suffixes := []string{
		cleanPath + ":" + pkg + "." + selector,
		"/" + cleanPath + ":" + pkg + "." + selector,
	}
	parts := strings.Split(cleanPath, "/")
	for i := range parts {
		dir := strings.Join(parts[i:], "/")
		if dir != "" {
			suffixes = append(suffixes, dir+":"+pkg+"."+selector)
		}
	}
	return suffixes
}
