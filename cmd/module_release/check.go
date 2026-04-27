package module_release

import (
	"fmt"
	"io"

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
	ListOpenPullRequestsByAuthor(repo, author string) ([]github.OpenPullRequest, error)
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

	if latestSHA == mainSHA {
		fmt.Println("The latest release tag is the HEAD of the main branch, there is nothing to release")
		return nil
	}

	// Create summary
	summary := module_release.NewCheckSummary(issuesRepoFlag)

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

	populateCheckSummary(cmd.ErrOrStderr(), client, summary, repo, comparison, prNumbers)

	// Check for open Weblate PRs
	openWeblatePRs, err := client.ListOpenPullRequestsByAuthor(repo, "weblate")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to check open Weblate PRs: %v\n", err)
	} else {
		for _, pr := range openWeblatePRs {
			summary.OpenWeblatePRs = append(summary.OpenWeblatePRs, pr.URL)
		}
	}

	// Display summary
	summary.Display()

	return nil
}

func populateCheckSummary(errWriter io.Writer, client checkSummaryClient, summary *module_release.CheckSummary, repo string, comparison *github.CompareResult, prNumbers []int) {
	commitsInPRs := make(map[string]bool)
	for _, commit := range comparison.Commits {
		prs, err := client.GetPullRequestsForCommit(repo, commit.SHA)
		if err == nil && len(prs) > 0 {
			commitsInPRs[commit.SHA] = true
		}
	}

	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}

		prURL := fmt.Sprintf("https://github.com/%s/pull/%d", repo, prNum)

		switch pr.User.Login {
		case "weblate":
			summary.WeblatePRs = append(summary.WeblatePRs, prURL)
			continue
		case "renovate[bot]":
			summary.RenovatePRs = append(summary.RenovatePRs, prURL)
			continue
		}

		linkedIssues := module_release.GetLinkedIssues(pr.Body, summary.IssuesRepo)
		if len(linkedIssues) > 0 {
			for _, issueNum := range linkedIssues {
				if err := summary.ProcessIssue(client, issueNum); err != nil {
					fmt.Fprintf(errWriter, "Warning: failed to process issue %d: %v\n", issueNum, err)
				}
			}
			continue
		}

		summary.UnlinkedPRs = append(summary.UnlinkedPRs, prURL)
	}

	for _, commit := range comparison.Commits {
		if !commitsInPRs[commit.SHA] {
			commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit.SHA)
			summary.OrphanCommits = append(summary.OrphanCommits, commitURL)
		}
	}
}
