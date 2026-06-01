package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/isoprism/api/internal/parser"
)

// TestExtractComponentHunkKeepsOnlyComponentLines verifies extract component hunk keeps only component lines.
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

// TestExtractComponentHunkSupportsDeletedComponent verifies extract component hunk supports deleted component.
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

// TestComponentDiffHunkTreatsAddedComponentBodyAsAdded verifies component diff hunk treats added component body as added.
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

// TestComponentDiffHunkTreatsDeletedComponentPatchLinesAsRemoved verifies component diff hunk treats deleted component patch lines as removed.
func TestComponentDiffHunkTreatsDeletedComponentPatchLinesAsRemoved(t *testing.T) {
	patch := strings.Join([]string{
		"@@ -28,7 +28,3 @@",
		" func Keep() {",
		" }",
		"-func Removed() {",
		"-\tcleanup()",
		"-}",
		" func AlsoKeep() {",
		" }",
	}, "\n")
	body := strings.Join([]string{
		"func Removed() {",
		"\tcleanup()",
		"}",
	}, "\n")

	got := componentDiffHunk("deleted", patch, body, 30, 32, 0, 0, nil, nil)
	added, removed := countDiffLines(got)

	if added != 0 || removed != 3 {
		t.Fatalf("deleted component stats = +%d -%d, want +0 -3\n%s", added, removed, got)
	}
	if strings.Contains(got, "Keep") || strings.Contains(got, "AlsoKeep") {
		t.Fatalf("deleted component diff leaked neighboring lines:\n%s", got)
	}
}

// TestComponentDiffHunkTreatsRenameOnlyAsMetadata verifies component diff hunk treats rename only as metadata.
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

// TestClassifyHeadNodeChangeKeepsDifferentBodyOverlapAdded verifies classify head node change keeps different body overlap added.
func TestClassifyHeadNodeChangeKeepsDifferentBodyOverlapAdded(t *testing.T) {
	base := parserNode("BlockAPI.closeAllListeners", "rpc/grpc/api.go", "method", 193, 203)
	base.BodyHash = "old-body"
	head := parserNode("BlockAPI.sendNonBlocking", "rpc/grpc/api.go", "method", 186, 198)
	head.BodyHash = "new-body"

	changeType, baseNode, unchanged := classifyHeadNodeChange(
		head,
		"modified",
		map[string]parser.Node{base.FullName: base},
		map[string][]parser.Node{base.BodyHash: {base}},
		map[string]bool{},
	)

	if changeType != "added" || baseNode != nil || unchanged {
		t.Fatalf("overlapping different-body node classified as %q base=%v unchanged=%v, want added", changeType, baseNode, unchanged)
	}
	if base.FilePath != head.FilePath || base.LineStart > head.LineEnd || head.LineStart > base.LineEnd {
		t.Fatal("test setup should keep same-file line overlap")
	}
}

// TestClassifyHeadNodeChangeUsesBodyHashForConservativeRename verifies classify head node change uses body hash for conservative rename.
func TestClassifyHeadNodeChangeUsesBodyHashForConservativeRename(t *testing.T) {
	base := parserNode("BlockAPI.closeAllListeners", "rpc/grpc/api.go", "method", 193, 203)
	base.BodyHash = "same-body"
	head := parserNode("rpc/grpc:coregrpc.BlockAPI.closeAllListeners", "rpc/grpc/api.go", "method", 214, 224)
	head.BodyHash = "same-body"

	changeType, baseNode, unchanged := classifyHeadNodeChange(
		head,
		"modified",
		map[string]parser.Node{base.FullName: base},
		map[string][]parser.Node{base.BodyHash: {base}},
		map[string]bool{},
	)

	if changeType != "renamed" || baseNode == nil || unchanged {
		t.Fatalf("same-body node classified as %q base=%v unchanged=%v, want renamed", changeType, baseNode, unchanged)
	}
}

// TestCurrentProcessorCommitSHAPrefersExplicitEnv verifies current processor commit SHA prefers explicit env.
func TestCurrentProcessorCommitSHAPrefersExplicitEnv(t *testing.T) {
	t.Setenv("ISOPRISM_COMMIT_SHA", "app-commit")
	t.Setenv("RAILWAY_GIT_COMMIT_SHA", "railway-commit")

	if got := currentProcessorCommitSHA(); got != "app-commit" {
		t.Fatalf("commit sha = %q, want explicit app commit", got)
	}
}

// TestPRProcessingStatsJSONUsesDiagnosticNames verifies PR processing stats JSON uses diagnostic names.
func TestPRProcessingStatsJSONUsesDiagnosticNames(t *testing.T) {
	stats := prProcessingStats{
		ChangedFiles:                  4,
		SupportedChangedFiles:         2,
		ChangedNodesDetected:          3,
		TestNodesDetected:             1,
		NodeChangesPersisted:          3,
		NodeChangePersistErrors:       1,
		CallEdgesExtracted:            5,
		CallEdgesPersisted:            4,
		UnsupportedChangedFiles:       2,
		NodeChangesSkippedMissingNode: 1,
	}

	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(raw)
	for _, want := range []string{
		`"changed_files":4`,
		`"supported_changed_files":2`,
		`"changed_nodes_detected":3`,
		`"test_nodes_detected":1`,
		`"node_changes_persisted":3`,
		`"node_change_persist_errors":1`,
		`"call_edges_extracted":5`,
		`"call_edges_persisted":4`,
		`"unsupported_changed_files":2`,
		`"node_changes_skipped_missing_node":1`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("processing stats json %s missing %s", jsonText, want)
		}
	}
}

// parserNode builds a parser node fixture for tests.
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
