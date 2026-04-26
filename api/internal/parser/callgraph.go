package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// CallEdge represents a caller → callee relationship.
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
// node set.  nodeByName maps full_name → true so we can filter to known symbols.
func ExtractCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	lang := languageFor(filePath)
	switch lang {
	case "go":
		return extractGoCallEdges(src, filePath, nodeByName)
	case "typescript", "javascript":
		return extractTSCallEdges(src, filePath, nodeByName)
	default:
		return nil
	}
}

// ExtractTestReferences finds test entrypoints that call known production nodes.
func ExtractTestReferences(src []byte, filePath string, nodeByName map[string]bool) []TestReference {
	lang := languageFor(filePath)
	switch lang {
	case "go":
		return extractGoTestReferences(src, filePath, nodeByName)
	case "typescript", "javascript":
		return extractTSTestReferences(src, filePath, nodeByName)
	default:
		return nil
	}
}

func extractGoCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil {
		return nil
	}

	methodLookup := buildMethodLookup(nodeByName)

	var edges []CallEdge

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		callerName := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			typeName := strings.TrimPrefix(exprToString(fn.Recv.List[0].Type), "*")
			callerName = typeName + "." + callerName
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			bare := callExprToName(call.Fun)
			if bare == "" {
				return true
			}

			callee := resolveKnownName(bare, nodeByName, methodLookup)

			if callee != "" && callee != callerName {
				edges = append(edges, CallEdge{
					CallerFullName: callerName,
					CalleeFullName: callee,
				})
			}
			return true
		})
	}

	return edges
}

func extractGoTestReferences(src []byte, filePath string, nodeByName map[string]bool) []TestReference {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil {
		return nil
	}
	if !IsTestFile(filePath) && (f.Name == nil || !strings.HasSuffix(f.Name.Name, "_test")) {
		return nil
	}

	methodLookup := buildMethodLookup(nodeByName)
	testFuncs := map[string]*ast.FuncDecl{}
	helperCalls := map[string]map[string]bool{}
	productionCalls := map[string]map[string]bool{}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			continue
		}

		name := fn.Name.Name
		if strings.HasPrefix(name, "Test") {
			testFuncs[name] = fn
		}
		helperCalls[name] = map[string]bool{}
		productionCalls[name] = map[string]bool{}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			bare := callExprToName(call.Fun)
			if bare == "" {
				return true
			}
			if callee := resolveKnownName(bare, nodeByName, methodLookup); callee != "" {
				productionCalls[name][callee] = true
			} else {
				helperCalls[name][bare] = true
			}
			return true
		})
	}

	var refs []TestReference
	for testName, fn := range testFuncs {
		start := fset.Position(fn.Pos())
		end := fset.Position(fn.End())
		targets := map[string]bool{}
		visited := map[string]bool{}
		var walk func(string)
		walk = func(name string) {
			if visited[name] {
				return
			}
			visited[name] = true
			for target := range productionCalls[name] {
				targets[target] = true
			}
			for helper := range helperCalls[name] {
				if _, ok := helperCalls[helper]; ok {
					walk(helper)
				}
			}
		}
		walk(testName)
		for target := range targets {
			refs = append(refs, TestReference{
				TestName:       testName,
				TestFullName:   testName,
				TestFilePath:   filePath,
				LineStart:      start.Line,
				LineEnd:        end.Line,
				TargetFullName: target,
			})
		}
	}
	return refs
}

func extractTSCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	nodes := parseTS(src, filePath, languageFor(filePath))
	methodLookup := buildMethodLookup(nodeByName)
	var edges []CallEdge

	for _, node := range nodes {
		if node.IsTestCode {
			continue
		}
		for _, call := range extractTSCallNames(node.Body) {
			callee := resolveKnownName(call, nodeByName, methodLookup)
			if callee != "" && callee != node.FullName {
				edges = append(edges, CallEdge{
					CallerFullName: node.FullName,
					CalleeFullName: callee,
				})
			}
		}
	}

	return edges
}

var tsTestCallPattern = regexp.MustCompile("(?s)\\b(?:test|it)\\s*\\(\\s*(?:\"([^\"]+)\"|'([^']+)'|`([^`]+)`)\\s*,")

func extractTSTestReferences(src []byte, filePath string, nodeByName map[string]bool) []TestReference {
	if !IsTestFile(filePath) {
		return nil
	}

	srcStr := string(src)
	methodLookup := buildMethodLookup(nodeByName)
	matches := tsTestCallPattern.FindAllStringSubmatchIndex(srcStr, -1)
	var refs []TestReference

	for _, match := range matches {
		if len(match) < 8 {
			continue
		}
		label := ""
		for i := 2; i <= 6; i += 2 {
			if match[i] >= 0 && match[i+1] >= 0 {
				label = srcStr[match[i]:match[i+1]]
				break
			}
		}
		if label == "" {
			continue
		}
		startByte := match[0]
		endByte := findEnclosingCallEnd(srcStr, match[1])
		if endByte <= startByte {
			endByte = match[1]
		}
		block := srcStr[match[1]:endByte]
		lineStart := strings.Count(srcStr[:startByte], "\n") + 1
		lineEnd := lineStart + strings.Count(srcStr[startByte:endByte], "\n")

		targets := map[string]bool{}
		for _, call := range extractTSCallNames(block) {
			if callee := resolveKnownName(call, nodeByName, methodLookup); callee != "" {
				targets[callee] = true
			}
		}

		testFullName := label
		for target := range targets {
			refs = append(refs, TestReference{
				TestName:       label,
				TestFullName:   testFullName,
				TestFilePath:   filePath,
				LineStart:      lineStart,
				LineEnd:        lineEnd,
				TargetFullName: target,
			})
		}
	}

	return refs
}

func buildMethodLookup(nodeByName map[string]bool) map[string]string {
	methodLookup := map[string]string{}
	for fn := range nodeByName {
		if dot := strings.Index(fn, "."); dot != -1 {
			bare := fn[dot+1:]
			if prev, exists := methodLookup[bare]; !exists {
				methodLookup[bare] = fn
			} else if prev != fn {
				methodLookup[bare] = ""
			}
		}
	}
	return methodLookup
}

func resolveKnownName(bare string, nodeByName map[string]bool, methodLookup map[string]string) string {
	if nodeByName[bare] {
		return bare
	}
	if resolved, ok := methodLookup[bare]; ok && resolved != "" {
		return resolved
	}
	return ""
}

func callExprToName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

var tsCallPattern = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
var tsSelectorCallPattern = regexp.MustCompile(`\.([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)

func extractTSCallNames(src string) []string {
	seen := map[string]bool{}
	var names []string
	for _, match := range tsCallPattern.FindAllStringSubmatch(src, -1) {
		if len(match) > 1 && !isIgnoredTSCallName(match[1]) && !seen[match[1]] {
			seen[match[1]] = true
			names = append(names, match[1])
		}
	}
	for _, match := range tsSelectorCallPattern.FindAllStringSubmatch(src, -1) {
		if len(match) > 1 && !isIgnoredTSCallName(match[1]) && !seen[match[1]] {
			seen[match[1]] = true
			names = append(names, match[1])
		}
	}
	return names
}

func isIgnoredTSCallName(name string) bool {
	switch name {
	case "describe", "test", "it", "expect", "beforeEach", "afterEach", "beforeAll", "afterAll":
		return true
	default:
		return false
	}
}

func findEnclosingCallEnd(src string, start int) int {
	depth := 1
	inString := byte(0)
	escaped := false
	for i := start; i < len(src); i++ {
		ch := src[i]
		if inString != 0 {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == inString {
				inString = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			inString = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return start
}
