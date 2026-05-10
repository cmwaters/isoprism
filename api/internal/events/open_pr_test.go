package events

import (
	"strings"
	"testing"

	"github.com/isoprism/api/internal/parser"
)

func TestExtractComponentHunkKeepsOnlyComponentLines(t *testing.T) {
	patch := strings.Join([]string{
		"@@ -10,9 +10,10 @@",
		" func A() {",
		"-\toldA()",
		"+\tnewA()",
		" }",
		" ",
		" func B() {",
		"+\tnewB()",
		" \tunchangedB()",
		" }",
	}, "\n")

	got := extractComponentHunk(patch, 10, 13, 10, 13)

	if strings.Contains(got, "newB") || strings.Contains(got, "unchangedB") {
		t.Fatalf("component diff leaked lines from another component:\n%s", got)
	}
	if !strings.Contains(got, "oldA") || !strings.Contains(got, "newA") {
		t.Fatalf("component diff omitted changed lines:\n%s", got)
	}

	added, removed := countDiffLines(got)
	if added != 1 || removed != 1 {
		t.Fatalf("component stats = +%d -%d, want +1 -1\n%s", added, removed, got)
	}
}

func TestExtractComponentHunkSupportsDeletedComponent(t *testing.T) {
	patch := strings.Join([]string{
		"@@ -30,4 +30,0 @@",
		"-func Removed() {",
		"-\tcleanup()",
		"-\treturn",
		"-}",
	}, "\n")

	got := extractComponentHunk(patch, 30, 33, 0, 0)

	if !strings.Contains(got, "Removed") || !strings.Contains(got, "cleanup") {
		t.Fatalf("deleted component diff omitted removed lines:\n%s", got)
	}

	added, removed := countDiffLines(got)
	if added != 0 || removed != 4 {
		t.Fatalf("deleted component stats = +%d -%d, want +0 -4\n%s", added, removed, got)
	}
}

func TestComponentDiffHunkTreatsAddedComponentBodyAsAdded(t *testing.T) {
	patch := strings.Join([]string{
		"@@ -10,7 +10,10 @@",
		"+func registerHandlers(mux *http.ServeMux, s *store) {",
		" \tmux.HandleFunc(\"POST /shorten\", func(w http.ResponseWriter, r *http.Request) {",
		" \t\twriteJSON(w)",
		" \t})",
		"+}",
	}, "\n")
	body := strings.Join([]string{
		"func registerHandlers(mux *http.ServeMux, s *store) {",
		"\tmux.HandleFunc(\"POST /shorten\", func(w http.ResponseWriter, r *http.Request) {",
		"\t\twriteJSON(w)",
		"\t})",
		"}",
	}, "\n")

	got := componentDiffHunk("added", patch, body, 0, 0, 55, 59, nil, nil)
	added, removed := countDiffLines(got)

	if added != 5 || removed != 0 {
		t.Fatalf("added component stats = +%d -%d, want +5 -0\n%s", added, removed, got)
	}
	if strings.Contains(got, "\n ") {
		t.Fatalf("added component diff should not keep context lines:\n%s", got)
	}
}

func TestComponentDiffHunkTreatsDeletedComponentBodyAsRemoved(t *testing.T) {
	body := strings.Join([]string{
		"func Removed() {",
		"\tcleanup()",
		"}",
	}, "\n")

	got := componentDiffHunk("deleted", "", body, 30, 32, 0, 0, nil, nil)
	added, removed := countDiffLines(got)

	if added != 0 || removed != 3 {
		t.Fatalf("deleted component stats = +%d -%d, want +0 -3\n%s", added, removed, got)
	}
}

func TestComponentDiffHunkTreatsRenameOnlyAsMetadata(t *testing.T) {
	oldName := "old/path:pkg.OldName"
	oldPath := "old/path/file.go"

	got := componentDiffHunk("renamed", "", "", 10, 12, 10, 12, &oldName, &oldPath)

	if !strings.Contains(got, "rename from old/path/file.go") {
		t.Fatalf("rename hunk omitted old path:\n%s", got)
	}
	if !strings.Contains(got, "rename symbol from old/path:pkg.OldName") {
		t.Fatalf("rename hunk omitted old symbol:\n%s", got)
	}
	added, removed := countDiffLines(got)
	if added != 0 || removed != 0 {
		t.Fatalf("rename metadata should not count as added/removed lines, got +%d -%d:\n%s", added, removed, got)
	}
}

func TestFirstUnmatchedOverlappingBaseNodeDetectsRenamedFunctionWithBodyChange(t *testing.T) {
	base := parserNode("oldName", "file.go", "function", 20, 30)
	head := parserNode("newName", "file.go", "function", 22, 33)

	got, ok := firstUnmatchedOverlappingBaseNode(map[string]parser.Node{base.FullName: base}, map[string]bool{}, head)
	if !ok {
		t.Fatal("expected overlapping unmatched base node")
	}
	if got.FullName != base.FullName {
		t.Fatalf("matched %q, want %q", got.FullName, base.FullName)
	}
}

func parserNode(name, path, kind string, start, end int) parser.Node {
	return parser.Node{
		Name:      name,
		FullName:  name,
		FilePath:  path,
		Kind:      kind,
		LineStart: start,
		LineEnd:   end,
	}
}
