package localgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/isoprism/api/internal/models"
	"github.com/isoprism/api/internal/parser"
)

const nodeSchemaVersion = "isoprism-node-v1"

// Options configures the local CLI graph runtime.
type Options struct {
	RepoDir      string
	Args         []string
	CacheDir     string
	RebuildCache bool
}

// ServeOptions configures the local CLI graph runtime.
type ServeOptions struct {
	RepoDir  string
	WebDir   string
	Host     string
	Port     int
	WebPort  int
	CacheDir string
	NoWeb    bool
}

// ReviewGraphPayload carries the payload exchanged by the local CLI graph runtime.
type ReviewGraphPayload struct {
	SchemaVersion string                 `json:"schema_version"`
	Mode          string                 `json:"mode"`
	Repository    LocalRepository        `json:"repository"`
	Diff          DiffMetadata           `json:"diff"`
	Graph         models.GraphResponse   `json:"graph"`
	Annotations   DiffAnnotations        `json:"annotations"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// DiffAnnotations stores the fields used by the local CLI graph runtime.
type DiffAnnotations struct {
	Summary        *DiffSummaryAnnotation          `json:"summary,omitempty"`
	NodeChanges    map[string]NodeChangeAnnotation `json:"node_changes,omitempty"`
	TestAssertions []TestAssertionAnnotation       `json:"test_assertions,omitempty"`
}

// DiffSummaryAnnotation stores the fields used by the local CLI graph runtime.
type DiffSummaryAnnotation struct {
	IssueLink              *string                   `json:"issue_link"`
	PRLink                 *string                   `json:"pr_link"`
	ReasonForChange        string                    `json:"reason_for_change"`
	ExpectedOutcome        string                    `json:"expected_outcome"`
	AlternativesConsidered *string                   `json:"alternatives_considered"`
	KnownGaps              []string                  `json:"known_gaps"`
	TestAssertions         []TestAssertionAnnotation `json:"test_assertions"`
}

// NodeChangeAnnotation describes a graph node used by the local CLI graph runtime.
type NodeChangeAnnotation struct {
	Description string  `json:"description"`
	Reasoning   string  `json:"reasoning"`
	Confidence  string  `json:"confidence"`
	Risks       *string `json:"risks"`
	FollowUp    *string `json:"follow_up"`
}

// TestAssertionAnnotation stores the fields used by the local CLI graph runtime.
type TestAssertionAnnotation struct {
	Description string `json:"description"`
	NodeSHA256  string `json:"node_sha256"`
}

// LocalRepository describes repository data used by the local CLI graph runtime.
type LocalRepository struct {
	Root          string `json:"root"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
}

// DiffMetadata stores the fields used by the local CLI graph runtime.
type DiffMetadata struct {
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref"`
	BaseSHA string `json:"base_sha"`
	HeadSHA string `json:"head_sha"`
}

// fileChange stores the fields used by the local CLI graph runtime.
type fileChange struct {
	Filename         string
	PreviousFilename string
	Status           string
	Additions        int
	Deletions        int
	Patch            string
}

// graphNodeObject describes a graph node used by the local CLI graph runtime.
type graphNodeObject struct {
	SchemaVersion string         `json:"schema_version"`
	Type          string         `json:"type"`
	FullName      string         `json:"full_name"`
	FilePath      string         `json:"filepath"`
	GitBlobSHA    *string        `json:"git_blob_sha"`
	LineStart     int            `json:"line_start"`
	LineEnd       int            `json:"line_end"`
	Inputs        []parser.Param `json:"inputs,omitempty"`
	Outputs       []parser.Param `json:"outputs,omitempty"`
	Fields        []parser.Param `json:"fields,omitempty"`
	Language      string         `json:"language"`
	Kind          string         `json:"kind"`
	BodyHash      string         `json:"body_hash"`
	Body          string         `json:"body"`
	DocComment    string         `json:"doc_comment,omitempty"`
	IsTest        bool           `json:"is_test"`
	IsEntrypoint  bool           `json:"is_entrypoint"`
	OutgoingLinks []linkObject   `json:"outgoing_links"`
}

// linkObject stores the fields used by the local CLI graph runtime.
type linkObject struct {
	RelationType string `json:"relation_type"`
	Target       string `json:"target"`
}

// treeGraph stores the fields used by the local CLI graph runtime.
type treeGraph struct {
	ref        string
	sha        string
	tree       map[string]string
	nodes      map[string]graphNodeObject
	nodesByRef map[string]graphNodeObject
	edges      []semanticEdge
	edgesByRef map[string][]semanticEdge
}

// semanticEdge describes a graph edge used by the local CLI graph runtime.
type semanticEdge struct {
	SourceRef string
	TargetRef string
	Kind      string
}

// semanticRef returns the stable file-plus-symbol reference for a local node.
func semanticRef(filePath, fullName string) string {
	return filePath + "::" + fullName
}

// nodeID builds a deterministic local graph node ID.
func nodeID(kind, fullName, filePath, blobSHA string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{nodeSchemaVersion, kind, fullName, filePath, blobSHA}, "\n")))
	return hex.EncodeToString(h[:])
}

// packagePath returns the directory path used as local package context.
func packagePath(filePath string) string {
	path := filepath.ToSlash(filePath)
	dir := filepath.Dir(path)
	if dir == "." {
		return ""
	}
	return dir
}

// nodeKind maps parser node kinds into graph node kinds.
func nodeKind(n parser.Node) string {
	if n.IsTest {
		return "test"
	}
	return n.Kind
}

// toTypeRefs converts parser parameters into graph type references.
func toTypeRefs(params []parser.Param) []models.TypeRef {
	out := make([]models.TypeRef, 0, len(params))
	for _, p := range params {
		out = append(out, models.TypeRef{Name: p.Name, Type: p.Type})
	}
	return out
}

// graphNodeFromObject converts a cached local node object into API graph shape.
func graphNodeFromObject(id string, obj graphNodeObject, nodeType string) models.GraphNode {
	var doc *string
	if strings.TrimSpace(obj.DocComment) != "" {
		value := obj.DocComment
		doc = &value
	}
	return models.GraphNode{
		ID:           id,
		FullName:     obj.FullName,
		FilePath:     obj.FilePath,
		PackagePath:  packagePath(obj.FilePath),
		LineStart:    obj.LineStart,
		LineEnd:      obj.LineEnd,
		Inputs:       toTypeRefs(obj.Inputs),
		Outputs:      toTypeRefs(obj.Outputs),
		Language:     obj.Language,
		Kind:         obj.Kind,
		IsTest:       obj.IsTest,
		IsEntrypoint: obj.IsEntrypoint,
		NodeType:     nodeType,
		DocComment:   doc,
		Summary:      doc,
		Weight:       1,
		Tests:        []models.GraphNodeTest{},
	}
}

// writeJSONAtomic writes JSON atomic for the local CLI graph runtime.
func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readJSON reads JSON for the local CLI graph runtime.
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

// RenderStaticHTML renders static HTML for the local CLI graph runtime.
func RenderStaticHTML(payload ReviewGraphPayload) ([]byte, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>Isoprism Diff</title>")
	b.WriteString("<style>body{font-family:Inter,system-ui,sans-serif;margin:0;background:#f7f5f0;color:#181817}main{max-width:1180px;margin:0 auto;padding:32px}h1{font-size:28px}section{margin:24px 0}.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.card{background:white;border:1px solid #ddd8ce;border-radius:8px;padding:16px}pre{white-space:pre-wrap;background:#1f2428;color:#f3f4f6;padding:12px;border-radius:6px;overflow:auto}.pill{display:inline-block;padding:2px 8px;border:1px solid #bbb;border-radius:999px;font-size:12px;margin-right:6px}.added{color:#166534}.deleted{color:#991b1b}.modified,.renamed{color:#92400e}table{border-collapse:collapse;width:100%}td,th{border-bottom:1px solid #e5e1d8;padding:8px;text-align:left}</style></head><body><main>")
	b.WriteString("<h1>Isoprism Diff</h1>")
	b.WriteString("<p><strong>" + html.EscapeString(payload.Diff.BaseRef) + "</strong> to <strong>" + html.EscapeString(payload.Diff.HeadRef) + "</strong></p>")
	b.WriteString("<section class=\"grid\"><div class=\"card\"><h2>Changed Nodes</h2>")
	if len(payload.Graph.Nodes) == 0 && len(payload.Graph.TestChanges) == 0 {
		b.WriteString("<p>No semantic node changes detected.</p>")
	} else {
		for _, n := range append(payload.Graph.Nodes, payload.Graph.TestChanges...) {
			change := ""
			if n.ChangeType != nil {
				change = *n.ChangeType
			}
			b.WriteString("<div><span class=\"pill " + html.EscapeString(change) + "\">" + html.EscapeString(change) + "</span><strong>" + html.EscapeString(n.FullName) + "</strong><br><small>" + html.EscapeString(n.FilePath) + ":" + fmt.Sprint(n.LineStart) + "</small></div><hr>")
		}
	}
	b.WriteString("</div><div class=\"card\"><h2>Graph Edges</h2><table><tr><th>Kind</th><th>Source</th><th>Destination</th></tr>")
	for _, e := range payload.Graph.Edges {
		b.WriteString("<tr><td>" + html.EscapeString(e.EdgeKind) + "</td><td>" + html.EscapeString(e.SourceID) + "</td><td>" + html.EscapeString(e.DestinationID) + "</td></tr>")
	}
	b.WriteString("</table></div></section><section class=\"card\"><h2>Files</h2>")
	for _, f := range payload.Graph.Files {
		b.WriteString("<h3>" + html.EscapeString(f.Status) + " " + html.EscapeString(f.Filename) + "</h3>")
		if f.Patch != nil {
			b.WriteString("<pre>" + html.EscapeString(*f.Patch) + "</pre>")
		}
	}
	b.WriteString("</section><section class=\"card\"><h2>Embedded ReviewGraphPayload</h2><pre id=\"payload\">" + html.EscapeString(string(data)) + "</pre></section>")
	b.WriteString("<script type=\"application/json\" id=\"isoprism-payload\">" + html.EscapeString(string(data)) + "</script>")
	b.WriteString("</main></body></html>")
	return []byte(b.String()), nil
}

// sortGraphPayload orders graph payload deterministically.
func sortGraphPayload(payload *ReviewGraphPayload) {
	sort.Slice(payload.Graph.Nodes, func(i, j int) bool { return payload.Graph.Nodes[i].ID < payload.Graph.Nodes[j].ID })
	sort.Slice(payload.Graph.TestChanges, func(i, j int) bool { return payload.Graph.TestChanges[i].ID < payload.Graph.TestChanges[j].ID })
	sort.Slice(payload.Graph.Edges, func(i, j int) bool {
		a, b := payload.Graph.Edges[i], payload.Graph.Edges[j]
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.DestinationID != b.DestinationID {
			return a.DestinationID < b.DestinationID
		}
		return a.EdgeKind < b.EdgeKind
	})
	sort.Slice(payload.Graph.Files, func(i, j int) bool { return payload.Graph.Files[i].Filename < payload.Graph.Files[j].Filename })
}
