package parser

import (
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// CallEdge represents a caller -> callee relationship.
type CallEdge struct {
	CallerFullName string
	CalleeFullName string
}

// ExtractCallEdges finds call relationships within source for the given parsed
// node set. nodeByName maps full_name -> true so we can filter to known symbols.
func ExtractCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	return ExtractCallEdgesWithResolver(src, filePath, BuildResolverIndex(map[string][]byte{filePath: src}, nodeByName))
}

// ExtractCallEdgesWithResolver finds call relationships using a prebuilt
// resolver index. Prefer this for repository/PR indexing so cross-file type
// facts are available during field-chain resolution.
func ExtractCallEdgesWithResolver(src []byte, filePath string, index ResolverIndex) []CallEdge {
	pf, ok := parseTree(src, filePath)
	if !ok {
		return nil
	}
	defer pf.tree.Close()

	switch languageFor(filePath) {
	case "go":
		return extractGoCallEdges(src, filePath, pf.root, index)
	case "typescript", "javascript":
		return extractScriptCallEdges(src, filePath, pf.root, index.NodeByName)
	default:
		return nil
	}
}

func extractGoCallEdges(src []byte, filePath string, root *sitter.Node, index ResolverIndex) []CallEdge {
	pkg := goPackageName(src, root)
	prefix := goPackagePrefix(filePath, pkg)
	imports := goImports(src, root)
	var edges []CallEdge

	for _, fn := range goFunctionNodes(root) {
		caller := goCallableFullName(src, fn, prefix)
		if caller == "" {
			continue
		}
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}
		scope := buildGoScope(src, fn, prefix, imports, index)
		seen := map[string]bool{}
		walk(body, func(n *sitter.Node) bool {
			if n.Kind() != "call_expression" {
				return true
			}
			callee := resolveGoCall(src, n.ChildByFieldName("function"), prefix, imports, index.NodeByName, index, scope)
			if callee != "" && callee != caller && !seen[callee] {
				seen[callee] = true
				edges = append(edges, CallEdge{CallerFullName: caller, CalleeFullName: callee})
			}
			return true
		})
	}
	return edges
}

func goFunctionNodes(root *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	walk(root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "function_declaration", "method_declaration":
			out = append(out, n)
			return false
		default:
			return true
		}
	})
	return out
}

func goCallableFullName(src []byte, fn *sitter.Node, prefix string) string {
	name := childText(src, fn, "name")
	if name == "" {
		return ""
	}
	if fn.Kind() == "method_declaration" {
		if recv := goReceiverName(src, fn.ChildByFieldName("receiver")); recv != "" {
			return prefix + "." + recv + "." + name
		}
	}
	return prefix + "." + name
}

func resolveGoCall(src []byte, fun *sitter.Node, prefix string, imports map[string]string, nodeByName map[string]bool, index ResolverIndex, scope goScope) string {
	if fun == nil {
		return ""
	}
	switch fun.Kind() {
	case "identifier":
		name := text(src, fun)
		if callee := known(prefix+"."+name, nodeByName); callee != "" {
			return callee
		}
		return known(name, nodeByName)
	case "selector_expression":
		if callee := resolveGoFieldChainCall(src, fun, index, scope); callee != "" {
			return callee
		}
		root := selectorRoot(src, fun)
		sel := selectorName(src, fun)
		if root == "" || sel == "" {
			return ""
		}
		if importPath, ok := imports[root]; ok {
			return resolveImportedGoSelector(importPath, sel, nodeByName)
		}
		// Same-package type or package-qualified references are safe only when
		// their package/type prefix is explicit and indexed in this package.
		if callee := known(prefix+"."+root+"."+sel, nodeByName); callee != "" {
			return callee
		}
		return ""
	default:
		return ""
	}
}

func resolveImportedGoSelector(importPath, selector string, nodeByName map[string]bool) string {
	cleanPath := strings.Trim(importPath, `"`)
	dir := filepath.ToSlash(cleanPath)
	pkg := filepath.Base(dir)
	suffixes := repoRelativeImportSuffix(dir, selector)
	suffixes = append(suffixes, dir+":"+pkg+"."+selector)
	var match string
	for name := range nodeByName {
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				if match != "" && match != name {
					return ""
				}
				match = name
			}
		}
	}
	return match
}

func goImports(src []byte, root *sitter.Node) map[string]string {
	imports := map[string]string{}
	walk(root, func(n *sitter.Node) bool {
		if n.Kind() != "import_spec" {
			return true
		}
		pathNode := n.ChildByFieldName("path")
		if pathNode == nil {
			return false
		}
		path := strings.Trim(text(src, pathNode), `"`)
		if path == "" {
			return false
		}
		alias := ""
		for i := uint(0); i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "package_identifier", "identifier":
				alias = text(src, child)
			}
		}
		if alias == "" {
			alias = filepath.Base(path)
		}
		if alias != "." && alias != "_" {
			imports[alias] = path
		}
		return false
	})
	return imports
}

func selectorRoot(src []byte, n *sitter.Node) string {
	operand := n.ChildByFieldName("operand")
	if operand == nil {
		operand = n.ChildByFieldName("object")
	}
	for operand != nil && operand.Kind() == "selector_expression" {
		next := operand.ChildByFieldName("operand")
		if next == nil {
			next = operand.ChildByFieldName("object")
		}
		operand = next
	}
	if operand != nil && (operand.Kind() == "identifier" || operand.Kind() == "package_identifier") {
		return text(src, operand)
	}
	return ""
}

func selectorName(src []byte, n *sitter.Node) string {
	field := n.ChildByFieldName("field")
	if field == nil {
		field = n.ChildByFieldName("property")
	}
	if field == nil {
		return ""
	}
	return text(src, field)
}

func extractScriptCallEdges(src []byte, filePath string, root *sitter.Node, nodeByName map[string]bool) []CallEdge {
	nodes := parseScriptTree(src, filePath, languageFor(filePath), root)
	prefix := scriptModulePrefix(filePath)
	var edges []CallEdge
	for _, node := range nodes {
		seen := map[string]bool{}
		for _, call := range scriptCallNames(src, enclosingBody(root, node.LineStart, node.LineEnd)) {
			callee := resolveScriptCall(prefix, call, nodeByName)
			if callee != "" && callee != node.FullName && !seen[callee] {
				seen[callee] = true
				edges = append(edges, CallEdge{CallerFullName: node.FullName, CalleeFullName: callee})
			}
		}
	}
	return edges
}

func scriptCallNames(src []byte, root *sitter.Node) []string {
	seen := map[string]bool{}
	var names []string
	walk(root, func(n *sitter.Node) bool {
		if n.Kind() != "call_expression" {
			return true
		}
		name := scriptCallName(src, n.ChildByFieldName("function"))
		if name != "" && !isIgnoredScriptCallName(name) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return true
	})
	return names
}

func scriptCallName(src []byte, fun *sitter.Node) string {
	if fun == nil {
		return ""
	}
	switch fun.Kind() {
	case "identifier":
		return text(src, fun)
	default:
		return ""
	}
}

func resolveScriptCall(prefix, call string, nodeByName map[string]bool) string {
	if call == "" {
		return ""
	}
	if callee := known(prefix+"."+call, nodeByName); callee != "" {
		return callee
	}
	if callee := known(call, nodeByName); callee != "" {
		return callee
	}
	var match string
	for name := range nodeByName {
		if strings.HasSuffix(name, "."+call) {
			if match != "" && match != name {
				return ""
			}
			match = name
		}
	}
	return match
}

func enclosingBody(root *sitter.Node, lineStart, lineEnd int) *sitter.Node {
	var found *sitter.Node
	walk(root, func(n *sitter.Node) bool {
		start := int(n.StartPosition().Row) + 1
		end := int(n.EndPosition().Row) + 1
		if start == lineStart && end == lineEnd {
			found = n
			return false
		}
		return true
	})
	if found == nil {
		return root
	}
	return found
}

func isScriptTestCall(src []byte, n *sitter.Node) bool {
	name := scriptCallName(src, n.ChildByFieldName("function"))
	return name == "test" || name == "it"
}

func scriptTestLabel(src []byte, n *sitter.Node) string {
	args := n.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := uint(0); i < args.NamedChildCount(); i++ {
		child := args.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "string", "template_string":
			return strings.Trim(text(src, child), "`\"'")
		}
	}
	return ""
}

func isIgnoredScriptCallName(name string) bool {
	switch name {
	case "describe", "test", "it", "expect", "beforeEach", "afterEach", "beforeAll", "afterAll":
		return true
	default:
		return false
	}
}

func known(name string, nodeByName map[string]bool) string {
	if nodeByName[name] {
		return name
	}
	return ""
}
