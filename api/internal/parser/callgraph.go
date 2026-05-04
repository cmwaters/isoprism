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

// TestReference represents one test entrypoint that reaches a production node.
type TestReference struct {
	TestName       string
	TestFullName   string
	TestFilePath   string
	LineStart      int
	LineEnd        int
	TargetFullName string
}

// ExtractCallEdges finds call relationships within source for the given parsed
// node set. nodeByName maps full_name -> true so we can filter to known symbols.
func ExtractCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	pf, ok := parseTree(src, filePath)
	if !ok {
		return nil
	}
	defer pf.tree.Close()

	switch languageFor(filePath) {
	case "go":
		return extractGoCallEdges(src, filePath, pf.root, nodeByName)
	case "typescript", "javascript":
		return extractScriptCallEdges(src, filePath, pf.root, nodeByName)
	default:
		return nil
	}
}

// ExtractTestReferences finds test entrypoints that call known production nodes.
func ExtractTestReferences(src []byte, filePath string, nodeByName map[string]bool) []TestReference {
	pf, ok := parseTree(src, filePath)
	if !ok {
		return nil
	}
	defer pf.tree.Close()

	switch languageFor(filePath) {
	case "go":
		return extractGoTestReferences(src, filePath, pf.root, nodeByName)
	case "typescript", "javascript":
		return extractScriptTestReferences(src, filePath, pf.root, nodeByName)
	default:
		return nil
	}
}

func extractGoCallEdges(src []byte, filePath string, root *sitter.Node, nodeByName map[string]bool) []CallEdge {
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
		seen := map[string]bool{}
		walk(body, func(n *sitter.Node) bool {
			if n.Kind() != "call_expression" {
				return true
			}
			callee := resolveGoCall(src, n.ChildByFieldName("function"), prefix, imports, nodeByName)
			if callee != "" && callee != caller && !seen[callee] {
				seen[callee] = true
				edges = append(edges, CallEdge{CallerFullName: caller, CalleeFullName: callee})
			}
			return true
		})
	}
	return edges
}

func extractGoTestReferences(src []byte, filePath string, root *sitter.Node, nodeByName map[string]bool) []TestReference {
	pkg := goPackageName(src, root)
	if !IsTestFile(filePath) && !strings.HasSuffix(pkg, "_test") {
		return nil
	}
	prefix := goPackagePrefix(filePath, pkg)
	imports := goImports(src, root)
	testFuncs := map[string]*sitter.Node{}
	helperCalls := map[string]map[string]bool{}
	productionCalls := map[string]map[string]bool{}

	for _, fn := range goFunctionNodes(root) {
		name := childText(src, fn, "name")
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "Test") {
			testFuncs[name] = fn
		}
		helperCalls[name] = map[string]bool{}
		productionCalls[name] = map[string]bool{}
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}
		walk(body, func(n *sitter.Node) bool {
			if n.Kind() != "call_expression" {
				return true
			}
			fun := n.ChildByFieldName("function")
			if callee := resolveGoCall(src, fun, prefix, imports, nodeByName); callee != "" {
				productionCalls[name][callee] = true
				return true
			}
			if bare := goDirectCallName(src, fun); bare != "" {
				helperCalls[name][bare] = true
			}
			return true
		})
	}

	var refs []TestReference
	for testName, fn := range testFuncs {
		targets := map[string]bool{}
		visited := map[string]bool{}
		var trace func(string)
		trace = func(name string) {
			if visited[name] {
				return
			}
			visited[name] = true
			for target := range productionCalls[name] {
				targets[target] = true
			}
			for helper := range helperCalls[name] {
				if _, ok := helperCalls[helper]; ok {
					trace(helper)
				}
			}
		}
		trace(testName)
		for target := range targets {
			refs = append(refs, TestReference{
				TestName:       testName,
				TestFullName:   prefix + "." + testName,
				TestFilePath:   filePath,
				LineStart:      int(fn.StartPosition().Row) + 1,
				LineEnd:        int(fn.EndPosition().Row) + 1,
				TargetFullName: target,
			})
		}
	}
	return refs
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

func goDirectCallName(src []byte, fun *sitter.Node) string {
	if fun != nil && fun.Kind() == "identifier" {
		return text(src, fun)
	}
	return ""
}

func resolveGoCall(src []byte, fun *sitter.Node, prefix string, imports map[string]string, nodeByName map[string]bool) string {
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
	suffix := dir + ":" + pkg + "." + selector
	var match string
	for name := range nodeByName {
		if strings.HasSuffix(name, suffix) {
			if match != "" && match != name {
				return ""
			}
			match = name
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
		if node.IsTestCode {
			continue
		}
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

func extractScriptTestReferences(src []byte, filePath string, root *sitter.Node, nodeByName map[string]bool) []TestReference {
	if !IsTestFile(filePath) {
		return nil
	}
	prefix := scriptModulePrefix(filePath)
	var refs []TestReference
	walk(root, func(n *sitter.Node) bool {
		if n.Kind() != "call_expression" || !isScriptTestCall(src, n) {
			return true
		}
		label := scriptTestLabel(src, n)
		if label == "" {
			return true
		}
		targets := map[string]bool{}
		for _, call := range scriptCallNames(src, n) {
			if callee := resolveScriptCall(prefix, call, nodeByName); callee != "" {
				targets[callee] = true
			}
		}
		for target := range targets {
			refs = append(refs, TestReference{
				TestName:       label,
				TestFullName:   prefix + "." + label,
				TestFilePath:   filePath,
				LineStart:      int(n.StartPosition().Row) + 1,
				LineEnd:        int(n.EndPosition().Row) + 1,
				TargetFullName: target,
			})
		}
		return true
	})
	return refs
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
