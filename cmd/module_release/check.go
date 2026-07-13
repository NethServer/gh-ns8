package module_release

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/NethServer/gh-ns8/internal/github"
	"github.com/NethServer/gh-ns8/internal/module_release"
	"github.com/spf13/cobra"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check the status of the main branch",
	Long:  `Check for PRs and issues since the latest release and verify readiness for a new release.`,
	RunE:  runCheck,
}

type checkSummaryClient interface {
	GetPullRequestsForCommit(repo, sha string) ([]int, error)
	GetPullRequest(repo string, number int) (*github.PullRequest, error)
	GetIssue(repo string, number int) (*github.Issue, error)
	GetParentIssueNumber(repo string, issueNumber int) (int, error)
	ListOpenPullRequests(repo string) ([]github.OpenPullRequest, error)
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Create GitHub client
	client, err := github.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Get and validate repository
	repo, err := module_release.GetOrValidateRepo(client, repoFlag)
	if err != nil {
		return err
	}

	// Get latest stable release
	latestRelease, err := module_release.GetLatestRelease(client, repo, true)
	if err != nil {
		return fmt.Errorf("no releases found")
	}

	fmt.Printf("Checking PRs and issues since %s...\n\n", latestRelease.TagName)

	// Check if release is needed
	latestSHA, err := module_release.GetReleaseCommitSHA(client, repo, latestRelease.TagName)
	if err != nil {
		return fmt.Errorf("failed to get release commit SHA: %w", err)
	}

	mainSHA, err := module_release.GetMainBranchSHA(client, repo)
	if err != nil {
		return fmt.Errorf("failed to get main branch SHA: %w", err)
	}

	summary := module_release.NewCheckSummary(issuesRepoFlag)

	if latestSHA == mainSHA {
		fmt.Println("The latest release tag is the HEAD of the main branch, there is nothing ready to release")
		populateOpenPullRequests(cmd.ErrOrStderr(), client, summary, repo, map[int]bool{})
		if len(summary.Issues) > 0 || len(summary.OpenWeblatePRs) > 0 {
			fmt.Println()
			summary.Display()
		}
		return nil
	}

	// Get all commits in range
	comparison, err := client.CompareCommits(repo, latestRelease.TagName, "main")
	if err != nil {
		return fmt.Errorf("failed to compare commits: %w", err)
	}

	if len(comparison.Commits) == 0 {
		fmt.Println("No commits found in the specified range.")
		return nil
	}

	// Scan for PRs
	prNumbers, err := module_release.ScanForPRs(client, repo, latestRelease.TagName, "main")
	if err != nil {
		return fmt.Errorf("error processing PRs: %w", err)
	}

	seenPRs := populateCheckSummary(cmd.ErrOrStderr(), client, summary, repo, comparison, prNumbers)
	populateOpenPullRequests(cmd.ErrOrStderr(), client, summary, repo, seenPRs)

	// Display summary
	summary.Display()

	return nil
}

func populateCheckSummary(errWriter io.Writer, client checkSummaryClient, summary *module_release.CheckSummary, repo string, comparison *github.CompareResult, prNumbers []int) map[int]bool {
	commitsInPRs := make(map[string]bool)
	for _, commit := range comparison.Commits {
		prs, err := client.GetPullRequestsForCommit(repo, commit.SHA)
		if err == nil && len(prs) > 0 {
			commitsInPRs[commit.SHA] = true
		}
	}

	seenPRs := make(map[int]bool, len(prNumbers))
	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}
		seenPRs[prNum] = true

		processPullRequest(errWriter, client, summary, repo, pr)
	}

	for _, commit := range comparison.Commits {
		if !commitsInPRs[commit.SHA] {
			commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit.SHA)
			summary.OrphanCommits = append(summary.OrphanCommits, commitURL)
		}
	}

	return seenPRs
}

func populateOpenPullRequests(errWriter io.Writer, client checkSummaryClient, summary *module_release.CheckSummary, repo string, seenPRs map[int]bool) {
	openPRs, err := client.ListOpenPullRequests(repo)
	if err != nil {
		fmt.Fprintf(errWriter, "Warning: failed to check open PRs: %v\n", err)
		return
	}

	sort.SliceStable(openPRs, func(i, j int) bool {
		return openPRs[i].Number < openPRs[j].Number
	})

	for _, openPR := range openPRs {
		if openPR.Author.Login == "weblate" {
			summary.OpenWeblatePRs = append(summary.OpenWeblatePRs, openPullRequestURL(repo, openPR))
		}
		if seenPRs[openPR.Number] {
			continue
		}
		if len(module_release.GetLinkedIssues(openPR.Body, summary.IssuesRepo)) == 0 {
			continue
		}

		pr, err := client.GetPullRequest(repo, openPR.Number)
		if err != nil {
			fmt.Fprintf(errWriter, "Warning: failed to get open PR %d: %v\n", openPR.Number, err)
			continue
		}

		processPullRequest(errWriter, client, summary, repo, pr)
		seenPRs[openPR.Number] = true
	}
}

func openPullRequestURL(repo string, pr github.OpenPullRequest) string {
	if pr.URL != "" {
		return pr.URL
	}
	return fmt.Sprintf("https://github.com/%s/pull/%d", repo, pr.Number)
}

func processPullRequest(errWriter io.Writer, client checkSummaryClient, summary *module_release.CheckSummary, repo string, pr *github.PullRequest) {
	category := categorizePullRequest(pr)

	linkedIssues := module_release.GetLinkedIssues(pr.Body, summary.IssuesRepo)
	if len(linkedIssues) == 0 {
		summary.AddPullRequest(repo, pr, category)
		return
	}

	for _, issueNum := range linkedIssues {
		if err := summary.ProcessIssue(client, issueNum); err != nil {
			fmt.Fprintf(errWriter, "Warning: failed to process issue %d: %v\n", issueNum, err)
			continue
		}
		summary.AddIssuePullRequest(repo, issueNum, pr, category)
	}
}

func categorizePullRequest(pr *github.PullRequest) module_release.PRCategory {
	switch {
	case pr.User.Login == "weblate":
		return module_release.PRCategoryTranslation
	case pr.User.Login == "renovate[bot]" && pr.Merged:
		return module_release.PRCategoryRenovate
	case strings.EqualFold(pr.State, "open"):
		return module_release.PRCategoryGeneric
	default:
		return module_release.PRCategoryMerged
	}
}
