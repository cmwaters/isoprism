package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/isoprism/api/internal/localgraph"
)

// main starts the process and reports fatal startup errors.
func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "isoprism:", err)
		os.Exit(1)
	}
}

// run dispatches the requested CLI subcommand.
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "diff":
		return runDiff(ctx, args[1:])
	case "serve":
		return runServe(ctx, args[1:])
	case "annotate":
		return runAnnotate(ctx, args[1:])
	default:
		return usage()
	}
}

// runDiff builds the local diff payload and writes or opens the chosen output.
func runDiff(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	output := fs.String("output", "", "write static HTML to path")
	openBrowser := fs.Bool("open", true, "open generated HTML in the browser")
	noOpen := fs.Bool("no-open", false, "do not open generated HTML")
	jsonOut := fs.Bool("json", false, "write ReviewGraphPayload JSON to stdout")
	cacheDir := fs.String("cache-dir", "", "cache directory")
	rebuild := fs.Bool("rebuild-cache", false, "rebuild local semantic cache")
	share := fs.Bool("share", false, "upload generated payload")
	flagArgs, posArgs := splitInterspersedFlags(args, map[string]bool{"output": true, "cache-dir": true})
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if *share {
		return fmt.Errorf("--share is phase 2 and is not implemented yet")
	}
	payload, err := localgraph.GenerateDiff(ctx, localgraph.Options{RepoDir: ".", Args: posArgs, CacheDir: *cacheDir, RebuildCache: *rebuild})
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	html, err := localgraph.RenderStaticHTML(payload)
	if err != nil {
		return err
	}
	path := *output
	if path == "" {
		path = filepath.Join(os.TempDir(), "isoprism-diff-"+shortSHA(payload.Diff.HeadSHA)+".html")
	}
	if err := os.WriteFile(path, html, 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	if *openBrowser && !*noOpen {
		_ = openPath(path)
	}
	return nil
}

// runServe starts the local graph daemon and viewer.
func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "host to bind")
	port := fs.Int("port", 3717, "port to bind")
	webPort := fs.Int("web-port", 3000, "development Next.js viewer port used with --web-dir")
	webDir := fs.String("web-dir", "", "development path to the Isoprism web app directory")
	cacheDir := fs.String("cache-dir", "", "cache directory")
	noWeb := fs.Bool("no-web", false, "serve only the local API")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return localgraph.Serve(ctx, localgraph.ServeOptions{RepoDir: ".", WebDir: *webDir, Host: *host, Port: *port, WebPort: *webPort, CacheDir: *cacheDir, NoWeb: *noWeb})
}

// runAnnotate records local review annotations for the current diff range.
func runAnnotate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("annotate requires diff, node, or test")
	}
	base, head, err := currentAnnotationRange(ctx)
	if err != nil {
		return err
	}
	root, err := repoRoot(ctx)
	if err != nil {
		return err
	}
	store := localgraph.AnnotationStore{RepoDir: root, BaseSHA: base, HeadSHA: head}
	switch args[0] {
	case "diff":
		fs := flag.NewFlagSet("annotate diff", flag.ContinueOnError)
		issueLink := fs.String("issue-link", "", "issue URL")
		prLink := fs.String("pr-link", "", "PR URL")
		reason := fs.String("reason-for-change", "", "reason")
		outcome := fs.String("expected-outcome", "", "expected outcome")
		alternatives := fs.String("alternatives-considered", "", "alternatives")
		var gaps multiFlag
		fs.Var(&gaps, "known-gap", "known gap")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		summary := localgraph.DiffSummaryAnnotation{IssueLink: stringPtr(*issueLink), PRLink: stringPtr(*prLink), ReasonForChange: *reason, ExpectedOutcome: *outcome, AlternativesConsidered: stringPtr(*alternatives), KnownGaps: gaps}
		return store.WriteDiff(summary)
	case "node":
		if len(args) < 2 {
			return fmt.Errorf("annotate node requires <node-sha256>")
		}
		nodeID := args[1]
		fs := flag.NewFlagSet("annotate node", flag.ContinueOnError)
		description := fs.String("description", "", "description")
		reasoning := fs.String("reasoning", "", "reasoning")
		confidence := fs.String("confidence", "high", "confidence")
		risks := fs.String("risks", "", "risks")
		followUp := fs.String("follow-up", "", "follow-up")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		return store.WriteNode(nodeID, localgraph.NodeChangeAnnotation{Description: *description, Reasoning: *reasoning, Confidence: *confidence, Risks: stringPtr(*risks), FollowUp: stringPtr(*followUp)})
	case "test":
		fs := flag.NewFlagSet("annotate test", flag.ContinueOnError)
		node := fs.String("node", "", "node sha")
		description := fs.String("description", "", "description")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return store.WriteTest(localgraph.TestAssertionAnnotation{NodeSHA256: *node, Description: *description})
	default:
		return fmt.Errorf("unknown annotate subcommand %q", args[0])
	}
}

// currentAnnotationRange resolves the base and head SHAs for the annotation target.
func currentAnnotationRange(ctx context.Context) (string, string, error) {
	args := []string{}
	if hasStagedChanges(ctx) {
		args = []string{"staged"}
	}
	payload, err := localgraph.GenerateDiff(ctx, localgraph.Options{RepoDir: ".", Args: args})
	if err != nil {
		return "", "", err
	}
	return payload.Diff.BaseSHA, payload.Diff.HeadSHA, nil
}

// hasStagedChanges reports whether the current checkout has staged changes.
func hasStagedChanges(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	return cmd.Run() != nil
}

// repoRoot resolves the root directory of the current git checkout.
func repoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// openPath opens a generated local artifact with the operating system.
func openPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// shortSHA shortens a commit SHA for display.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// splitInterspersedFlags separates positional arguments from flags that may appear between them.
func splitInterspersedFlags(args []string, valueFlags map[string]bool) ([]string, []string) {
	var flagArgs, posArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			posArgs = append(posArgs, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		if valueFlags[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, posArgs
}

// stringPtr returns a pointer to the provided string.
func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

type multiFlag []string

// String returns the joined values for a repeatable CLI flag.
func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

// Set appends a value to a repeatable CLI flag.
func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// usage prints CLI usage text.
func usage() error {
	return fmt.Errorf("usage: isoprism diff|serve|annotate")
}
