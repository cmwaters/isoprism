package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://api.github.com"

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("github API error: status %d for %s %s", resp.StatusCode, method, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ── Types ─────────────────────────────────────────────────────────────────────

type GHInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login     string `json:"login"`
		Type      string `json:"type"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
	} `json:"account"`
}

type GHUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
}

type InstallationRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

type ListInstallationReposResponse struct {
	TotalCount   int                `json:"total_count"`
	Repositories []InstallationRepo `json:"repositories"`
}

type GHPullRequest struct {
	ID      int64  `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Additions      int        `json:"additions"`
	Deletions      int        `json:"deletions"`
	ChangedFiles   int        `json:"changed_files"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA *string    `json:"merge_commit_sha"`
}

// GHTreeEntry is one file entry in the git tree.
type GHTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" | "tree"
	SHA  string `json:"sha"`
	Size int    `json:"size"`
}

// GHCompareFile is a changed file in a compare response.
type GHCompareFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // added | modified | removed | renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"` // unified diff
}

// ── Methods ───────────────────────────────────────────────────────────────────

func (c *Client) ListInstallationRepos(ctx context.Context) ([]InstallationRepo, error) {
	var result ListInstallationReposResponse
	if err := c.do(ctx, "GET", "/installation/repositories?per_page=100", &result); err != nil {
		return nil, err
	}
	return result.Repositories, nil
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*InstallationRepo, error) {
	var result InstallationRepo
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s", owner, repo), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListOpenPullRequests(ctx context.Context, owner, repo string) ([]GHPullRequest, error) {
	var prs []GHPullRequest
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/pulls?state=open&per_page=100", owner, repo), &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*GHPullRequest, error) {
	var pr GHPullRequest
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context) (*GHUser, error) {
	var user GHUser
	if err := c.do(ctx, "GET", "/user", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GetBranchSHA returns the HEAD commit SHA of a branch.
func (c *Client) GetBranchSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	var result struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch), &result); err != nil {
		return "", err
	}
	return result.Object.SHA, nil
}

// GetTree returns the full file tree at a given commit SHA.
func (c *Client) GetTree(ctx context.Context, owner, repo, sha string) ([]GHTreeEntry, error) {
	var result struct {
		Tree      []GHTreeEntry `json:"tree"`
		Truncated bool          `json:"truncated"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, sha), &result); err != nil {
		return nil, err
	}
	return result.Tree, nil
}

// GetFileContent fetches the raw bytes of a file at the given ref.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	url := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)
	if err := c.do(ctx, "GET", url, &result); err != nil {
		return nil, err
	}
	if result.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding %q for file %s", result.Encoding, path)
	}
	// GitHub wraps base64 with newlines — strip them before decoding.
	clean := strings.ReplaceAll(result.Content, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}

// CompareCommits returns the list of changed files between base and head.
func (c *Client) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]GHCompareFile, error) {
	var result struct {
		Files []GHCompareFile `json:"files"`
	}
	url := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repo, base, head)
	if err := c.do(ctx, "GET", url, &result); err != nil {
		return nil, err
	}
	return result.Files, nil
}
