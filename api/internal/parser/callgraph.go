package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CallEdge represents a caller → callee relationship.
type CallEdge struct {
	CallerFullName string
	CalleeFullName string
}

// ExtractCallEdges finds call relationships within source for the given parsed
// node set.  nodeByName maps full_name → true so we can filter to known symbols.
func ExtractCallEdges(src []byte, filePath string, nodeByName map[string]bool) []CallEdge {
	lang := languageFor(filePath)
	switch lang {
	case "go":
		return extractGoCallEdges(src, filePath, nodeByName)
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

	// Build a secondary lookup: bare method name → full_name (e.g. "save" → "store.save").
	// Used to resolve receiver method calls where we only see the bare method name in the AST.
	// If two different types share a method name, mark as ambiguous (empty string).
	methodLookup := map[string]string{}
	for fn := range nodeByName {
		if dot := strings.Index(fn, "."); dot != -1 {
			bare := fn[dot+1:]
			if prev, exists := methodLookup[bare]; !exists {
				methodLookup[bare] = fn
			} else if prev != fn {
				methodLookup[bare] = "" // ambiguous
			}
		}
	}

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

			// Resolve to full_name: try exact match first, then method lookup.
			callee := ""
			if nodeByName[bare] {
				callee = bare
			} else if resolved, ok := methodLookup[bare]; ok && resolved != "" {
				callee = resolved
			}

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
