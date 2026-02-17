package github

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
)

// Client provides GitHub API access with both REST and GraphQL
type Client struct {
	rest    *api.RESTClient
	graphql *api.GraphQLClient
}

// NewClient creates a new GitHub API client using default gh configuration
func NewClient() (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	graphql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL client: %w", err)
	}

	return &Client{
		rest:    rest,
		graphql: graphql,
	}, nil
}

// Repository represents basic repo info
type Repository struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name              string `json:"name"`
	DefaultBranchRef  struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

// GetRepository fetches repository information
func (c *Client) GetRepository(repo string) (*Repository, error) {
	var result Repository
	err := c.rest.Get(fmt.Sprintf("repos/%s", repo), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	return &result, nil
}

// Commit represents a commit
type Commit struct {
	SHA string `json:"sha"`
}

// GetLatestCommit gets the latest commit SHA from default branch
func (c *Client) GetLatestCommit(repo string) (string, error) {
	var commits []Commit
	err := c.rest.Get(fmt.Sprintf("repos/%s/commits", repo), &commits)
	if err != nil {
		return "", fmt.Errorf("failed to get commits: %w", err)
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found")
	}
	return commits[0].SHA, nil
}

// GetCommitSHA gets the SHA for a git ref
func (c *Client) GetCommitSHA(repo, ref string) (string, error) {
	var result struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err := c.rest.Get(fmt.Sprintf("repos/%s/git/ref/%s", repo, ref), &result)
	if err != nil {
		return "", fmt.Errorf("failed to get ref: %w", err)
	}
	return result.Object.SHA, nil
}

// CompareCommits compares two commits
type CompareResult struct {
	Commits []struct {
		SHA string `json:"sha"`
	} `json:"commits"`
}

func (c *Client) CompareCommits(repo, base, head string) (*CompareResult, error) {
	var result CompareResult
	err := c.rest.Get(fmt.Sprintf("repos/%s/compare/%s...%s", repo, base, head), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}
	return &result, nil
}

// GetMergeBase gets the merge base between two commits
func (c *Client) GetMergeBase(repo, base, head string) (string, error) {
	var result struct {
		SHA string `json:"sha"`
	}
	body := strings.NewReader(fmt.Sprintf(`{"base":"%s","head":"%s"}`, base, head))
	err := c.rest.Post(fmt.Sprintf("repos/%s/merge-base", repo), body, &result)
	if err != nil {
		return "", fmt.Errorf("failed to get merge base: %w", err)
	}
	return result.SHA, nil
}

// Release represents a GitHub release
type Release struct {
	TagName      string `json:"tagName"`
	Name         string `json:"name"`
	IsPrerelease bool   `json:"isPrerelease"`
	CreatedAt    string `json:"createdAt"`
}

// ListReleases lists releases
func (c *Client) ListReleases(repo string, limit int, excludePreReleases bool) ([]Release, error) {
	args := []string{"release", "list", "--repo", repo, "--json", "tagName,name,isPrerelease,createdAt", "--limit", fmt.Sprintf("%d", limit)}
	if excludePreReleases {
		args = append(args, "--exclude-pre-releases")
	}
	
	stdout, _, err := gh.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var releases []Release
	if err := json.Unmarshal(stdout.Bytes(), &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases: %w", err)
	}

	return releases, nil
}

// ViewRelease gets details about a specific release
func (c *Client) ViewRelease(repo, tag string) (*Release, error) {
	args := []string{"release", "view", tag, "--repo", repo, "--json", "tagName,name,isPrerelease,createdAt"}
	
	stdout, _, err := gh.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to view release: %w", err)
	}

	var release Release
	if err := json.Unmarshal(stdout.Bytes(), &release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// CreateRelease creates a new release using gh release create
func (c *Client) CreateRelease(repo, tag, title string, draft, prerelease bool, target string, notesReader io.Reader) error {
	args := []string{"release", "create", tag, "--repo", repo, "--title", title, "--generate-notes"}
	
	if draft {
		args = append(args, "--draft")
	}
	if prerelease {
		args = append(args, "--prerelease")
	}
	if target != "" {
		args = append(args, "--target", target)
	}
	if notesReader != nil {
		args = append(args, "--notes-file", "-")
	}

	// For interactive operations with stdin, we need to use a different approach
	if notesReader != nil {
		// Read the notes into memory
		notesBytes, err := io.ReadAll(notesReader)
		if err != nil {
			return fmt.Errorf("failed to read notes: %w", err)
		}
		
		// Execute with notes as stdin (requires shell piping)
		cmd := fmt.Sprintf("echo %q | gh %s", string(notesBytes), strings.Join(args, " "))
		_, _, err = gh.Exec("sh", "-c", cmd)
		if err != nil {
			return fmt.Errorf("failed to create release: %w", err)
		}
	} else {
		_, _, err := gh.Exec(args...)
		if err != nil {
			return fmt.Errorf("failed to create release: %w", err)
		}
	}

	return nil
}

// DeleteRelease deletes a release
func (c *Client) DeleteRelease(repo, tag string) error {
	_, _, err := gh.Exec("release", "delete", tag, "--repo", repo, "--yes")
	if err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}
	return nil
}

// PullRequest represents a PR
type PullRequest struct {
	Number int    `json:"number"`
	Body   string `json:"body"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// GetPullRequestsForCommit gets PRs associated with a commit
func (c *Client) GetPullRequestsForCommit(repo, sha string) ([]int, error) {
	var prs []struct {
		Number int `json:"number"`
	}
	err := c.rest.Get(fmt.Sprintf("repos/%s/commits/%s/pulls", repo, sha), &prs)
	if err != nil {
		return nil, fmt.Errorf("failed to get PRs for commit: %w", err)
	}

	numbers := make([]int, len(prs))
	for i, pr := range prs {
		numbers[i] = pr.Number
	}
	return numbers, nil
}

// GetPullRequest gets PR details
func (c *Client) GetPullRequest(repo string, number int) (*PullRequest, error) {
	var pr PullRequest
	err := c.rest.Get(fmt.Sprintf("repos/%s/pulls/%d", repo, number), &pr)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}
	return &pr, nil
}

// Issue represents a GitHub issue
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// GetIssue gets issue details
func (c *Client) GetIssue(repo string, number int) (*Issue, error) {
	var issue Issue
	err := c.rest.Get(fmt.Sprintf("repos/%s/issues/%d", repo, number), &issue)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue: %w", err)
	}
	return &issue, nil
}

// CreateIssueComment posts a comment on an issue and returns the comment URL
func (c *Client) CreateIssueComment(repo string, number int, body string) (string, error) {
	_, _, err := gh.Exec("issue", "comment", fmt.Sprintf("%d", number), "--repo", repo, "--body", body)
	if err != nil {
		return "", fmt.Errorf("failed to create comment: %w", err)
	}

	// Get the last comment to retrieve its URL
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repo format: %s", repo)
	}
	owner, repoName := parts[0], parts[1]

	type Comment struct {
		ID  int    `json:"id"`
		URL string `json:"html_url"`
	}
	var comments []Comment
	err = c.rest.Get(fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repoName, number), &comments)
	if err != nil {
		return "", fmt.Errorf("failed to fetch comment URL: %w", err)
	}

	if len(comments) == 0 {
		return "", fmt.Errorf("no comments found after creation")
	}

	// Return the URL of the last comment (most recent)
	return comments[len(comments)-1].URL, nil
}

// GetParentIssueNumber gets the parent issue using GraphQL sub-issues API
func (c *Client) GetParentIssueNumber(repo string, issueNumber int) (int, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid repo format: %s", repo)
	}
	owner, repoName := parts[0], parts[1]

	query := `
		query($owner: String!, $repo: String!, $issueNumber: Int!) {
			repository(owner: $owner, name: $repo) {
				issue(number: $issueNumber) {
					parent {
						number
					}
				}
			}
		}
	`

	var response struct {
		Data struct {
			Repository struct {
				Issue struct {
					Parent *struct {
						Number int `json:"number"`
					} `json:"parent"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}

	// Use gh api with GraphQL-Features header for sub_issues
	args := []string{
		"api", "graphql",
		"-H", "GraphQL-Features: sub_issues",
		"-f", fmt.Sprintf("query=%s", query),
		"-F", fmt.Sprintf("owner=%s", owner),
		"-F", fmt.Sprintf("repo=%s", repoName),
		"-F", fmt.Sprintf("issueNumber=%d", issueNumber),
	}

	stdout, _, err := gh.Exec(args...)
	if err != nil {
		return 0, fmt.Errorf("failed to query parent issue: %w", err)
	}

	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return 0, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	if response.Data.Repository.Issue.Parent != nil {
		return response.Data.Repository.Issue.Parent.Number, nil
	}

	return 0, nil // No parent
}

// GetCurrentRepository gets the current repository from the working directory
func GetCurrentRepository() (string, error) {
	stdout, _, err := gh.Exec("repo", "view", "--json", "owner,name", "--jq", ".owner.login + \"/\" + .name")
	if err != nil {
		return "", fmt.Errorf("failed to get current repository: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
