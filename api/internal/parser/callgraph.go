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
			callee := callExprToName(call.Fun)
			if callee == "" {
				return true
			}
			// Only include calls to symbols we've parsed
			if nodeByName[callee] && callee != callerName {
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
		// Could be pkg.Func or recv.Method; we return just the method name
		// so it can match unqualified parsed names.
		return t.Sel.Name
	default:
		return ""
	}
}
