package localgraph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type gitClient struct {
	root string
}

func repoRoot(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g gitClient) run(ctx context.Context, args ...string) (string, error) {
	return runGit(ctx, g.root, args...)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func (g gitClient) resolveDefaultBranch(ctx context.Context) (string, error) {
	if out, err := g.run(ctx, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		if branch := strings.TrimPrefix(ref, "refs/remotes/origin/"); branch != "" && branch != ref {
			return branch, nil
		}
	}
	if out, err := g.run(ctx, "remote", "show", "origin"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if branch, ok := strings.CutPrefix(line, "HEAD branch: "); ok && branch != "" && branch != "(unknown)" {
				return branch, nil
			}
		}
	}
	if _, err := g.run(ctx, "rev-parse", "--verify", "--quiet", "main^{commit}"); err == nil {
		return "main", nil
	}
	if _, err := g.run(ctx, "rev-parse", "--verify", "--quiet", "master^{commit}"); err == nil {
		return "master", nil
	}
	return "", fmt.Errorf("could not detect the default branch; pass an explicit ref")
}

func (g gitClient) resolveCommit(ctx context.Context, ref string) (string, error) {
	if ref == worktreeTreeRef {
		head, err := g.resolveCommit(ctx, "HEAD")
		if err != nil {
			return "", err
		}
		return "worktree-" + head, nil
	}
	candidates := []string{ref}
	if !strings.HasPrefix(ref, "origin/") {
		candidates = append(candidates, "origin/"+ref)
	}
	for _, candidate := range candidates {
		out, err := g.run(ctx, "rev-parse", "--verify", candidate+"^{commit}")
		if err == nil {
			return strings.TrimSpace(out), nil
		}
	}
	return "", fmt.Errorf("could not resolve %q as a commit, local branch, or origin branch", ref)
}

func (g gitClient) resolveObject(ctx context.Context, ref string) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g gitClient) writeIndexTree(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g gitClient) listTree(ctx context.Context, ref string) (map[string]string, error) {
	if ref == indexTreeRef {
		return g.listIndex(ctx)
	}
	if ref == worktreeTreeRef {
		return g.listWorktree(ctx)
	}
	out, err := g.run(ctx, "ls-tree", "-r", "-z", ref)
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		parts := strings.Fields(meta)
		if len(parts) >= 3 && parts[1] == "blob" {
			tree[path] = parts[2]
		}
	}
	return tree, nil
}

func (g gitClient) listWorktree(ctx context.Context) (map[string]string, error) {
	out, err := g.run(ctx, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	for _, path := range strings.Split(out, "\x00") {
		if path == "" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(path)))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(content)
		tree[path] = "worktree-" + hex.EncodeToString(sum[:])
	}
	return tree, nil
}

func (g gitClient) listIndex(ctx context.Context) (map[string]string, error) {
	out, err := g.run(ctx, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		parts := strings.Fields(meta)
		if len(parts) >= 2 {
			tree[path] = parts[1]
		}
	}
	return tree, nil
}

func (g gitClient) showFile(ctx context.Context, ref, path string) ([]byte, error) {
	if ref == worktreeTreeRef {
		return os.ReadFile(filepath.Join(g.root, filepath.FromSlash(path)))
	}
	if ref == indexTreeRef {
		tree, err := g.listIndex(ctx)
		if err != nil {
			return nil, err
		}
		blob := tree[path]
		if blob == "" {
			return nil, fmt.Errorf("path %s is not present in the git index", path)
		}
		out, err := g.run(ctx, "cat-file", "blob", blob)
		if err != nil {
			return nil, err
		}
		return []byte(out), nil
	}
	out, err := g.run(ctx, "show", ref+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

func (g gitClient) diffPatch(ctx context.Context, from, to string, paths ...string) (string, error) {
	if to == indexTreeRef {
		args := []string{"diff", "--cached", "--patch", from, "--"}
		args = append(args, paths...)
		out, err := g.run(ctx, args...)
		if err != nil {
			return "", err
		}
		return out, nil
	}
	if to == worktreeTreeRef {
		args := []string{"diff", "--patch", from, "--"}
		args = append(args, paths...)
		out, err := g.run(ctx, args...)
		if err != nil {
			return "", err
		}
		return out, nil
	}
	args := []string{"diff", "--patch", from, to, "--"}
	args = append(args, paths...)
	out, err := g.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func (g gitClient) diffNameStatus(ctx context.Context, from, to string) ([]fileChange, error) {
	if to == indexTreeRef {
		return g.diffNameStatusArgs(ctx, "diff", "--cached", "--name-status", "--find-renames", "-z", from)
	}
	if to == worktreeTreeRef {
		return g.diffNameStatusArgs(ctx, "diff", "--name-status", "--find-renames", "-z", from)
	}
	return g.diffNameStatusArgs(ctx, "diff", "--name-status", "--find-renames", "-z", from, to)
}

func (g gitClient) diffNameStatusArgs(ctx context.Context, args ...string) ([]fileChange, error) {
	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, "\x00")
	var changes []fileChange
	for i := 0; i < len(fields); i++ {
		status := fields[i]
		if status == "" {
			continue
		}
		code := status[:1]
		change := fileChange{}
		switch code {
		case "R", "C":
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("malformed git name-status rename record")
			}
			change.Status = "renamed"
			change.PreviousFilename = fields[i+1]
			change.Filename = fields[i+2]
			i += 2
		default:
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("malformed git name-status record")
			}
			change.Filename = fields[i+1]
			i++
			switch code {
			case "A":
				change.Status = "added"
			case "D":
				change.Status = "removed"
			default:
				change.Status = "modified"
			}
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func (g gitClient) diffNumstat(ctx context.Context, from, to string) (map[string][2]int, error) {
	args := []string{"diff", "--numstat", "-z", from, to}
	if to == indexTreeRef {
		args = []string{"diff", "--cached", "--numstat", "-z", from}
	}
	if to == worktreeTreeRef {
		args = []string{"diff", "--numstat", "-z", from}
	}
	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, "\x00")
	stats := map[string][2]int{}
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\t")
		if len(parts) < 3 {
			continue
		}
		added := parseNumstat(parts[0])
		deleted := parseNumstat(parts[1])
		path := parts[2]
		if path == "" && i+1 < len(fields) {
			path = fields[i+1]
			i++
		}
		stats[path] = [2]int{added, deleted}
	}
	return stats, nil
}

func parseNumstat(value string) int {
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
