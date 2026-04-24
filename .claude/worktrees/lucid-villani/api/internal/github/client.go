package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://api.github.com"

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
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

// ---- Types ----

type GHInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login     string `json:"login"`
		Type      string `json:"type"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
	} `json:"account"`
}

type GHOrgTeam struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
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
	ID     int64  `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	HTMLURL string `json:"html_url"`
	User   struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	ChangedFiles int        `json:"changed_files"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	MergedAt     *time.Time `json:"merged_at"`
	MergedBy     *struct {
		Login string `json:"login"`
	} `json:"merged_by"`
}

type GHReview struct {
	ID          int64     `json:"id"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
	User        struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// ---- Methods ----

func (c *Client) ListInstallationRepos(ctx context.Context) ([]InstallationRepo, error) {
	var result ListInstallationReposResponse
	if err := c.do(ctx, "GET", "/installation/repositories?per_page=100", &result); err != nil {
		return nil, err
	}
	return result.Repositories, nil
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

func (c *Client) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]GHReview, error) {
	var reviews []GHReview
	if err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, number), &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

func (c *Client) ListOrgTeams(ctx context.Context, org string) ([]GHOrgTeam, error) {
	var teams []GHOrgTeam
	if err := c.do(ctx, "GET", fmt.Sprintf("/orgs/%s/teams?per_page=100", org), &teams); err != nil {
		return nil, err
	}
	return teams, nil
}

func (c *Client) ListOrgTeamMembers(ctx context.Context, org, teamSlug string) ([]GHUser, error) {
	var members []GHUser
	if err := c.do(ctx, "GET", fmt.Sprintf("/orgs/%s/teams/%s/members?per_page=100", org, teamSlug), &members); err != nil {
		return nil, err
	}
	return members, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context) (*GHUser, error) {
	var user GHUser
	if err := c.do(ctx, "GET", "/user", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) ListUserOrgs(ctx context.Context) ([]struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}, error) {
	var orgs []struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := c.do(ctx, "GET", "/user/orgs?per_page=100", &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}
