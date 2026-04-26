package events

import (
	"strings"
	"testing"
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
