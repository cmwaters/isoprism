// Package parser extracts function-level code nodes from source files.
// It uses tree-sitter grammars for Go, TypeScript, TSX, JavaScript, and JSX.
package parser

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Node represents one extracted code element (function, method, type, etc.).
type Node struct {
	Name             string
	FullName         string
	FilePath         string
	LineStart        int
	LineEnd          int
	Inputs           []Param
	Outputs          []Param
	Language         string
	Kind             string
	BodyHash         string
	Body             string
	IsTestCode       bool
	IsTestEntrypoint bool
}

type Param struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

type parsedFile struct {
	tree *sitter.Tree
	root *sitter.Node
}

// Parse extracts code nodes from the given source bytes.
// language is derived from the file extension passed in filePath.
func Parse(src []byte, filePath string) []Node {
	lang := languageFor(filePath)
	pf, ok := parseTree(src, filePath)
	if !ok {
		return nil
	}
	defer pf.tree.Close()

	switch lang {
	case "go":
		return parseGoTree(src, filePath, pf.root)
	case "typescript", "javascript":
		return parseScriptTree(src, filePath, lang, pf.root)
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

// IsTestFile returns true for supported language test file naming conventions.
func IsTestFile(path string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(path), "\\", "/")
	base := filepath.Base(normalized)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	switch languageFor(path) {
	case "go":
		return strings.HasSuffix(base, "_test.go")
	case "typescript", "javascript":
		if strings.Contains(normalized, "/__tests__/") {
			return true
		}
		return strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, ".spec")
	default:
		return false
	}
}

func bodyHash(src []byte) string {
	h := sha256.Sum256(src)
	return fmt.Sprintf("%x", h)[:16]
}

func parseTree(src []byte, filePath string) (parsedFile, bool) {
	var lang *sitter.Language
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".go":
		lang = sitter.NewLanguage(tree_sitter_go.Language())
	case ".ts":
		lang = sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case ".tsx":
		lang = sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	case ".js", ".jsx":
		lang = sitter.NewLanguage(tree_sitter_javascript.Language())
	default:
		return parsedFile{}, false
	}

	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(lang); err != nil {
		return parsedFile{}, false
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		return parsedFile{}, false
	}
	return parsedFile{tree: tree, root: tree.RootNode()}, true
}

func parseGoTree(src []byte, filePath string, root *sitter.Node) []Node {
	pkg := goPackageName(src, root)
	prefix := goPackagePrefix(filePath, pkg)
	isTestFile := IsTestFile(filePath)
	isTestPackage := strings.HasSuffix(pkg, "_test")

	var nodes []Node
	walk(root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			bodyNode := n.ChildByFieldName("body")
			if nameNode == nil || bodyNode == nil {
				return true
			}
			name := text(src, nameNode)
			nodes = append(nodes, makeGoNode(src, filePath, n, name, prefix+"."+name, "function", isTestFile || isTestPackage || strings.HasPrefix(name, "Test")))
		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			bodyNode := n.ChildByFieldName("body")
			if nameNode == nil || bodyNode == nil {
				return true
			}
			name := text(src, nameNode)
			recv := goReceiverName(src, n.ChildByFieldName("receiver"))
			fullName := prefix + "." + name
			if recv != "" {
				fullName = prefix + "." + recv + "." + name
			}
			nodes = append(nodes, makeGoNode(src, filePath, n, name, fullName, "method", isTestFile || isTestPackage || strings.HasPrefix(name, "Test")))
		case "type_declaration":
			forEachDescendant(n, "type_spec", func(spec *sitter.Node) {
				nameNode := spec.ChildByFieldName("name")
				if nameNode == nil {
					return
				}
				kind := "type"
				if typeNode := spec.ChildByFieldName("type"); typeNode != nil {
					switch typeNode.Kind() {
					case "struct_type":
						kind = "struct"
					case "interface_type":
						kind = "interface"
					}
				}
				name := text(src, nameNode)
				nodes = append(nodes, makeBaseNode(src, filePath, spec, name, prefix+"."+name, "go", kind, isTestFile || isTestPackage, false))
			})
			return false
		}
		return true
	})
	return nodes
}

func makeGoNode(src []byte, filePath string, n *sitter.Node, name, fullName, kind string, isTestCode bool) Node {
	return Node{
		Name:             name,
		FullName:         fullName,
		FilePath:         filePath,
		LineStart:        int(n.StartPosition().Row) + 1,
		LineEnd:          int(n.EndPosition().Row) + 1,
		Inputs:           goParams(src, n.ChildByFieldName("parameters")),
		Outputs:          goOutputs(src, n.ChildByFieldName("result")),
		Language:         "go",
		Kind:             kind,
		BodyHash:         bodyHash(nodeBytes(src, n)),
		Body:             text(src, n),
		IsTestCode:       isTestCode,
		IsTestEntrypoint: strings.HasPrefix(name, "Test"),
	}
}

func makeBaseNode(src []byte, filePath string, n *sitter.Node, name, fullName, lang, kind string, isTestCode, isTestEntrypoint bool) Node {
	return Node{
		Name:             name,
		FullName:         fullName,
		FilePath:         filePath,
		LineStart:        int(n.StartPosition().Row) + 1,
		LineEnd:          int(n.EndPosition().Row) + 1,
		Language:         lang,
		Kind:             kind,
		BodyHash:         bodyHash(nodeBytes(src, n)),
		Body:             text(src, n),
		IsTestCode:       isTestCode,
		IsTestEntrypoint: isTestEntrypoint,
	}
}

func goPackageName(src []byte, root *sitter.Node) string {
	var pkg string
	walk(root, func(n *sitter.Node) bool {
		if n.Kind() != "package_clause" {
			return true
		}
		for i := uint(0); i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if child != nil && child.Kind() == "package_identifier" {
				pkg = text(src, child)
				return false
			}
		}
		return false
	})
	return pkg
}

func goPackagePrefix(filePath, pkg string) string {
	dir := filepath.ToSlash(filepath.Dir(filePath))
	if dir == "." || dir == "" {
		return pkg
	}
	return dir + ":" + pkg
}

func goReceiverName(src []byte, receiver *sitter.Node) string {
	if receiver == nil {
		return ""
	}
	var names []string
	walk(receiver, func(n *sitter.Node) bool {
		if n.Kind() == "type_identifier" || n.Kind() == "qualified_type" {
			names = append(names, strings.TrimPrefix(text(src, n), "*"))
		}
		return true
	})
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[len(names)-1], "*")
}

func goParams(src []byte, params *sitter.Node) []Param {
	if params == nil {
		return nil
	}
	var out []Param
	forEachDescendant(params, "parameter_declaration", func(n *sitter.Node) {
		typeNode := n.ChildByFieldName("type")
		if typeNode == nil {
			return
		}
		typ := text(src, typeNode)
		names := childTextsByKinds(src, n, "identifier")
		if len(names) == 0 {
			out = append(out, Param{Type: typ})
			return
		}
		for _, name := range names {
			out = append(out, Param{Name: name, Type: typ})
		}
	})
	return out
}

func goOutputs(src []byte, result *sitter.Node) []Param {
	if result == nil {
		return nil
	}
	if result.Kind() != "parameter_list" {
		return []Param{{Type: text(src, result)}}
	}
	return goParams(src, result)
}

func parseScriptTree(src []byte, filePath, lang string, root *sitter.Node) []Node {
	prefix := scriptModulePrefix(filePath)
	isTestFile := IsTestFile(filePath)
	var nodes []Node
	seen := map[string]bool{}

	walk(root, func(n *sitter.Node) bool {
		name := ""
		kind := "function"
		switch n.Kind() {
		case "function_declaration":
			name = childText(src, n, "name")
		case "lexical_declaration", "variable_declaration":
			for i := uint(0); i < n.NamedChildCount(); i++ {
				decl := n.NamedChild(i)
				if decl == nil || decl.Kind() != "variable_declarator" {
					continue
				}
				value := decl.ChildByFieldName("value")
				if value == nil || (value.Kind() != "arrow_function" && value.Kind() != "function") {
					continue
				}
				name = childText(src, decl, "name")
				if name != "" && !seen[name] {
					seen[name] = true
					nodes = append(nodes, makeScriptNode(src, filePath, value, name, prefix+"."+name, lang, kind, isTestFile))
				}
			}
			return false
		case "method_definition", "public_field_definition":
			name = childText(src, n, "name")
			kind = "method"
			if className := enclosingClassName(src, n); className != "" {
				name = className + "." + name
			}
		}
		if name == "" || seen[name] {
			return true
		}
		seen[name] = true
		nodes = append(nodes, makeScriptNode(src, filePath, n, leafName(name), prefix+"."+name, lang, kind, isTestFile))
		return true
	})
	return nodes
}

func makeScriptNode(src []byte, filePath string, n *sitter.Node, name, fullName, lang, kind string, isTestCode bool) Node {
	node := makeBaseNode(src, filePath, n, name, fullName, lang, kind, isTestCode, false)
	node.Inputs = scriptParams(src, n.ChildByFieldName("parameters"))
	return node
}

func scriptModulePrefix(filePath string) string {
	ext := filepath.Ext(filePath)
	noExt := strings.TrimSuffix(filepath.ToSlash(filePath), ext)
	return noExt
}

func scriptParams(src []byte, params *sitter.Node) []Param {
	if params == nil {
		return nil
	}
	var out []Param
	for i := uint(0); i < params.NamedChildCount(); i++ {
		child := params.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "required_parameter", "optional_parameter":
			name := childText(src, child, "pattern")
			if name == "" {
				name = childText(src, child, "name")
			}
			typ := childText(src, child, "type")
			out = append(out, Param{Name: name, Type: strings.TrimPrefix(typ, ":")})
		case "identifier":
			out = append(out, Param{Name: text(src, child)})
		}
	}
	return out
}

func enclosingClassName(src []byte, n *sitter.Node) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == "class_declaration" {
			return childText(src, p, "name")
		}
	}
	return ""
}

func childText(src []byte, n *sitter.Node, field string) string {
	child := n.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return text(src, child)
}

func childTextsByKinds(src []byte, n *sitter.Node, kinds ...string) []string {
	allowed := map[string]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	var out []string
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if child != nil && allowed[child.Kind()] {
			out = append(out, text(src, child))
		}
	}
	return out
}

func nodeBytes(src []byte, n *sitter.Node) []byte {
	start, end := int(n.StartByte()), int(n.EndByte())
	if start < 0 || end > len(src) || start > end {
		return nil
	}
	return src[start:end]
}

func text(src []byte, n *sitter.Node) string {
	return string(nodeBytes(src, n))
}

func walk(n *sitter.Node, visit func(*sitter.Node) bool) {
	if n == nil {
		return
	}
	if !visit(n) {
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walk(n.NamedChild(i), visit)
	}
}

func forEachDescendant(n *sitter.Node, kind string, fn func(*sitter.Node)) {
	walk(n, func(child *sitter.Node) bool {
		if child.Kind() == kind {
			fn(child)
			return false
		}
		return true
	})
}

func leafName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
