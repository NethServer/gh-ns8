package module_release

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/NethServer/gh-ns8/internal/github"
)

var ns8ModulePattern = regexp.MustCompile(`^[^/]+/ns8-`)

// ValidateRepository checks if the repository exists and follows NS8 naming convention
func ValidateRepository(client *github.Client, repo string) error {
	// Check if repo matches the ns8-* pattern
	if !ns8ModulePattern.MatchString(repo) {
		return fmt.Errorf("invalid NS8 module name: %s (must match owner/ns8-*)", repo)
	}

	// Verify repository exists and is accessible
	_, err := client.GetRepository(repo)
	if err != nil {
		return fmt.Errorf("invalid repo: %s (%w)", repo, err)
	}

	return nil
}

// GetOrValidateRepo returns the provided repo or gets the current directory's repo
func GetOrValidateRepo(client *github.Client, repo string) (string, error) {
	// If repo is not provided, get it from current directory
	if repo == "" {
		currentRepo, err := github.GetCurrentRepository()
		if err != nil {
			return "", fmt.Errorf("could not determine the repo. Please provide the repo name using the --repo flag")
		}
		repo = currentRepo
	}

	// Validate the repository
	if err := ValidateRepository(client, repo); err != nil {
		return "", err
	}

	return repo, nil
}

// CommitInfo holds commit SHA and target flag
type CommitInfo struct {
	SHA    string
	Target string // Target flag for release (e.g., "commit-sha" or empty)
}

// GetOrValidateCommit returns the latest commit or validates the provided one
func GetOrValidateCommit(client *github.Client, repo, commitSHA string) (*CommitInfo, error) {
	info := &CommitInfo{}

	// If no commit SHA provided, get the latest
	if commitSHA == "" {
		sha, err := client.GetLatestCommit(repo)
		if err != nil {
			return nil, fmt.Errorf("could not determine the latest commit sha. Please provide the commit sha using the --release-refs flag")
		}
		info.SHA = sha
		return info, nil
	}

	// Validate that commit is on the default branch
	repoInfo, err := client.GetRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository info: %w", err)
	}

	defaultBranch := repoInfo.DefaultBranchRef.Name
	mergeBase, err := client.GetMergeBase(repo, defaultBranch, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to check if commit is on default branch: %w", err)
	}

	if mergeBase != commitSHA {
		return nil, fmt.Errorf("the commit sha is not on the default branch: %s", defaultBranch)
	}

	info.SHA = commitSHA
	info.Target = commitSHA // Set target for release command

	return info, nil
}

// GetLatestRelease gets the latest release (optionally excluding pre-releases)
func GetLatestRelease(client *github.Client, repo string, excludePreReleases bool) (*github.Release, error) {
	releases, err := client.ListReleases(repo, 1, excludePreReleases)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}

	return &releases[0], nil
}

// GetReleaseCommitSHA gets the commit SHA for a release tag
func GetReleaseCommitSHA(client *github.Client, repo, tag string) (string, error) {
	sha, err := client.GetCommitSHA(repo, fmt.Sprintf("tags/%s", tag))
	if err != nil {
		return "", fmt.Errorf("failed to get commit SHA for tag %s: %w", tag, err)
	}
	return sha, nil
}

// GetMainBranchSHA gets the current SHA of the main branch
func GetMainBranchSHA(client *github.Client, repo string) (string, error) {
	sha, err := client.GetCommitSHA(repo, "heads/main")
	if err != nil {
		return "", fmt.Errorf("failed to get main branch SHA: %w", err)
	}
	return sha, nil
}

// ScanForPRs scans commits between two refs and returns unique PR numbers
func ScanForPRs(client *github.Client, repo, startRef, endRef string) ([]int, error) {
	// Compare commits
	comparison, err := client.CompareCommits(repo, startRef, endRef)
	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}

	if len(comparison.Commits) == 0 {
		return nil, fmt.Errorf("no commits found in the specified range")
	}

	// Collect unique PR numbers
	prMap := make(map[int]bool)
	for _, commit := range comparison.Commits {
		prs, err := client.GetPullRequestsForCommit(repo, commit.SHA)
		if err != nil {
			continue // Skip commits that fail
		}
		for _, prNum := range prs {
			prMap[prNum] = true
		}
	}

	if len(prMap) == 0 {
		return nil, fmt.Errorf("no pull requests found for the commits in the specified range")
	}

	// Convert to slice
	prNumbers := make([]int, 0, len(prMap))
	for prNum := range prMap {
		prNumbers = append(prNumbers, prNum)
	}

	return prNumbers, nil
}

// GetLinkedIssues extracts issue numbers from a PR body
func GetLinkedIssues(prBody, issuesRepo string) []int {
	parts := strings.Split(issuesRepo, "/")
	if len(parts) != 2 {
		return nil
	}
	owner, repo := parts[0], parts[1]

	// Build regex pattern to match:
	// owner/issues/1234
	// owner/repo#1234
	// https://github.com/owner/repo/issues/1234
	patterns := []string{
		fmt.Sprintf(`%s/issues/(\d+)`, owner),
		fmt.Sprintf(`%s/%s#(\d+)`, owner, repo),
		fmt.Sprintf(`https://github\.com/%s/%s/issues/(\d+)`, owner, repo),
	}

	var issueNumbers []int
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(prBody, -1)
		for _, match := range matches {
			if len(match) > 1 {
				var num int
				fmt.Sscanf(match[1], "%d", &num)
				if num > 0 {
					issueNumbers = append(issueNumbers, num)
				}
			}
		}
	}

	return issueNumbers
}
