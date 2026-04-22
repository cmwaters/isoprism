// Package parser extracts function-level code nodes from source files.
// For Go it uses the standard go/parser and go/ast packages.
// For TypeScript/JavaScript it uses a lightweight regex approach.
package parser

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// Node represents one extracted code element (function, method, type, etc.).
type Node struct {
	Name      string
	FullName  string // e.g. "MyStruct.MyMethod" or bare "myFunc"
	FilePath  string
	LineStart int
	LineEnd   int
	Signature string
	Language  string
	Kind      string // function | method | type
	BodyHash  string
	Body      string // raw source text of the node body (for AI enrichment)
}

// Parse extracts code nodes from the given source bytes.
// language is derived from the file extension passed in filePath.
func Parse(src []byte, filePath string) []Node {
	lang := languageFor(filePath)
	switch lang {
	case "go":
		return parseGo(src, filePath)
	case "typescript", "javascript":
		return parseTS(src, filePath, lang)
	default:
		return nil
	}
}

func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	default:
		return ""
	}
}

// IsSupportedFile returns true if the file extension is one we can parse.
func IsSupportedFile(path string) bool {
	return languageFor(path) != ""
}

func bodyHash(src []byte) string {
	h := sha256.Sum256(src)
	return fmt.Sprintf("%x", h)[:16]
}

// ── Go parser ─────────────────────────────────────────────────────────────────

func parseGo(src []byte, filePath string) []Node {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil {
		return nil
	}

	srcStr := string(src)
	lines := strings.Split(srcStr, "\n")

	var nodes []Node

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || d.Body == nil {
				continue
			}

			start := fset.Position(d.Pos())
			end := fset.Position(d.End())

			// Extract raw body text
			startLine := start.Line - 1
			endLine := end.Line - 1
			if startLine < 0 { startLine = 0 }
			if endLine >= len(lines) { endLine = len(lines) - 1 }
			bodyLines := lines[startLine : endLine+1]
			body := strings.Join(bodyLines, "\n")

			name := d.Name.Name
			fullName := name
			kind := "function"

			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := d.Recv.List[0]
				typeName := exprToString(recv.Type)
				// Strip pointer: *MyStruct → MyStruct
				typeName = strings.TrimPrefix(typeName, "*")
				fullName = typeName + "." + name
				kind = "method"
			}

			sig := buildGoSignature(d)

			nodes = append(nodes, Node{
				Name:      name,
				FullName:  fullName,
				FilePath:  filePath,
				LineStart: start.Line,
				LineEnd:   end.Line,
				Signature: sig,
				Language:  "go",
				Kind:      kind,
				BodyHash:  bodyHash([]byte(body)),
				Body:      body,
			})

		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil {
					continue
				}
				start := fset.Position(ts.Pos())
				end := fset.Position(ts.End())

				startLine := start.Line - 1
				endLine := end.Line - 1
				if startLine < 0 { startLine = 0 }
				if endLine >= len(lines) { endLine = len(lines) - 1 }
				bodyLines := lines[startLine : endLine+1]
				body := strings.Join(bodyLines, "\n")

				kind := "type"
				switch ts.Type.(type) {
				case *ast.StructType:
					kind = "struct"
				case *ast.InterfaceType:
					kind = "interface"
				}

				nodes = append(nodes, Node{
					Name:      ts.Name.Name,
					FullName:  ts.Name.Name,
					FilePath:  filePath,
					LineStart: start.Line,
					LineEnd:   end.Line,
					Signature: "type " + ts.Name.Name,
					Language:  "go",
					Kind:      kind,
					BodyHash:  bodyHash([]byte(body)),
					Body:      body,
				})
			}
		}
	}

	return nodes
}

func exprToString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	default:
		return "..."
	}
}

func buildGoSignature(fn *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sb.WriteString("(")
		for i, f := range fn.Recv.List {
			if i > 0 { sb.WriteString(", ") }
			if len(f.Names) > 0 {
				sb.WriteString(f.Names[0].Name)
				sb.WriteString(" ")
			}
			sb.WriteString(exprToString(f.Type))
		}
		sb.WriteString(") ")
	}
	sb.WriteString(fn.Name.Name)
	sb.WriteString("(")
	if fn.Type.Params != nil {
		for i, f := range fn.Type.Params.List {
			if i > 0 { sb.WriteString(", ") }
			names := make([]string, len(f.Names))
			for j, n := range f.Names { names[j] = n.Name }
			if len(names) > 0 {
				sb.WriteString(strings.Join(names, ", "))
				sb.WriteString(" ")
			}
			sb.WriteString(exprToString(f.Type))
		}
	}
	sb.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		results := fn.Type.Results.List
		if len(results) == 1 && len(results[0].Names) == 0 {
			sb.WriteString(" ")
			sb.WriteString(exprToString(results[0].Type))
		} else {
			sb.WriteString(" (")
			for i, f := range results {
				if i > 0 { sb.WriteString(", ") }
				sb.WriteString(exprToString(f.Type))
			}
			sb.WriteString(")")
		}
	}
	return sb.String()
}

// ── TypeScript / JavaScript parser (regex-based) ──────────────────────────────

var tsFuncPatterns = []*regexp.Regexp{
	// export async function foo(...)
	regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`),
	// export const foo = async (...) =>
	regexp.MustCompile(`(?m)^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(`),
	// class method: methodName(...)
	regexp.MustCompile(`(?m)^\s+(?:async\s+)?(\w+)\s*\([^)]*\)\s*(?::\s*\S+\s*)?\{`),
}

func parseTS(src []byte, filePath, lang string) []Node {
	srcStr := string(src)
	lines := strings.Split(srcStr, "\n")
	var nodes []Node
	seen := map[string]bool{}

	for _, pat := range tsFuncPatterns {
		matches := pat.FindAllStringSubmatchIndex(srcStr, -1)
		for _, m := range matches {
			if len(m) < 4 { continue }
			nameStart, nameEnd := m[2], m[3]
			name := srcStr[nameStart:nameEnd]

			if seen[name] { continue }
			seen[name] = true

			// Find the line number
			startByte := m[0]
			lineNum := strings.Count(srcStr[:startByte], "\n") + 1
			lineEnd := lineNum + 10
			if lineEnd >= len(lines) { lineEnd = len(lines) }

			body := strings.Join(lines[lineNum-1:lineEnd], "\n")

			nodes = append(nodes, Node{
				Name:      name,
				FullName:  name,
				FilePath:  filePath,
				LineStart: lineNum,
				LineEnd:   lineEnd,
				Signature: name + "(...)",
				Language:  lang,
				Kind:      "function",
				BodyHash:  bodyHash([]byte(body)),
				Body:      body,
			})
		}
	}
	return nodes
}
