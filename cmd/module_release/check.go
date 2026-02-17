package module_release

import (
	"fmt"

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

	// Track commits that are in PRs
	commitsInPRs := make(map[string]bool)

	// Scan for PRs
	prNumbers, err := module_release.ScanForPRs(client, repo, latestRelease.TagName, "main")
	if err != nil {
		return fmt.Errorf("error processing PRs: %w", err)
	}

	// Mark all commits that belong to PRs
	for _, commit := range comparison.Commits {
		prs, err := client.GetPullRequestsForCommit(repo, commit.SHA)
		if err == nil && len(prs) > 0 {
			commitsInPRs[commit.SHA] = true
		}
	}

	// Process each PR
	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}

		// Check for linked issues
		linkedIssues := module_release.GetLinkedIssues(pr.Body, issuesRepoFlag)
		
		if len(linkedIssues) > 0 {
			// Process linked issues
			for _, issueNum := range linkedIssues {
				if err := summary.ProcessIssue(client, issueNum); err != nil {
					// Log error but continue
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to process issue %d: %v\n", issueNum, err)
				}
			}
		} else {
			// Check if it's a translation PR
			isTranslation := false
			for _, label := range pr.Labels {
				if label.Name == "translation" {
					isTranslation = true
					break
				}
			}

			prURL := fmt.Sprintf("https://github.com/%s/pull/%d", repo, prNum)
			if isTranslation {
				summary.TranslationPRs = append(summary.TranslationPRs, prURL)
			} else {
				summary.UnlinkedPRs = append(summary.UnlinkedPRs, prURL)
			}
		}
	}

	// Find orphan commits (not in any PR)
	for _, commit := range comparison.Commits {
		if !commitsInPRs[commit.SHA] {
			commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", repo, commit.SHA)
			summary.OrphanCommits = append(summary.OrphanCommits, commitURL)
		}
	}

	// Display summary
	summary.Display()

	return nil
}
