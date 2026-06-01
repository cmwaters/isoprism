package localgraph

import (
	"fmt"
	"os"
	"path/filepath"
)

// AnnotationStore stores the fields used by the local CLI graph runtime.
type AnnotationStore struct {
	RepoDir  string
	CacheDir string
	BaseSHA  string
	HeadSHA  string
}

// dir returns the annotation directory for a diff range.
func (s AnnotationStore) dir() string {
	cacheDir := s.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(s.RepoDir, ".isoprism")
	}
	return filepath.Join(cacheDir, "annotations", s.BaseSHA+".."+s.HeadSHA)
}

// WriteDiff writes diff for the local CLI graph runtime.
func (s AnnotationStore) WriteDiff(summary DiffSummaryAnnotation) error {
	return writeJSONAtomic(filepath.Join(s.dir(), "diff_summary"), summary)
}

// WriteNode renders the write node for the local CLI graph runtime.
func (s AnnotationStore) WriteNode(nodeSHA string, annotation NodeChangeAnnotation) error {
	if nodeSHA == "" {
		return fmt.Errorf("node sha is required")
	}
	return writeJSONAtomic(filepath.Join(s.dir(), "node_changes", nodeSHA), annotation)
}

// WriteTest writes test for the local CLI graph runtime.
func (s AnnotationStore) WriteTest(assertion TestAssertionAnnotation) error {
	path := filepath.Join(s.dir(), "test_assertions")
	var assertions []TestAssertionAnnotation
	if err := readJSON(path, &assertions); err != nil && !os.IsNotExist(err) {
		return err
	}
	assertions = append(assertions, assertion)
	return writeJSONAtomic(path, assertions)
}

// loadAnnotations loads annotations for the local CLI graph runtime.
func loadAnnotations(cacheDir, baseSHA, headSHA string) DiffAnnotations {
	dir := filepath.Join(cacheDir, "annotations", baseSHA+".."+headSHA)
	annotations := DiffAnnotations{NodeChanges: map[string]NodeChangeAnnotation{}}
	var summary DiffSummaryAnnotation
	if err := readJSON(filepath.Join(dir, "diff_summary"), &summary); err == nil {
		annotations.Summary = &summary
		annotations.TestAssertions = append(annotations.TestAssertions, summary.TestAssertions...)
	}
	var tests []TestAssertionAnnotation
	if err := readJSON(filepath.Join(dir, "test_assertions"), &tests); err == nil {
		annotations.TestAssertions = append(annotations.TestAssertions, tests...)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "node_changes"))
	if err != nil {
		return annotations
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var annotation NodeChangeAnnotation
		if err := readJSON(filepath.Join(dir, "node_changes", entry.Name()), &annotation); err == nil {
			annotations.NodeChanges[entry.Name()] = annotation
		}
	}
	return annotations
}
