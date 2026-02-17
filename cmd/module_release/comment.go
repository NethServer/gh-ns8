package module_release

import (
	"fmt"

	"github.com/NethServer/gh-ns8/internal/github"
	"github.com/NethServer/gh-ns8/internal/module_release"
	"github.com/spf13/cobra"
)

// commentCmd represents the comment command
var commentCmd = &cobra.Command{
	Use:   "comment [VERSION]",
	Short: "Add comments to release issues",
	Long:  `Post release notifications on open linked issues and their parent issues.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runComment,
}

func runComment(cmd *cobra.Command, args []string) error {
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

	// Get release name from argument or use latest
	releaseName := ""
	if len(args) > 0 {
		releaseName = args[0]
	}

	if releaseName == "" {
		release, err := module_release.GetLatestRelease(client, repo, false)
		if err != nil {
			return fmt.Errorf("failed to get latest release: %w", err)
		}
		releaseName = release.TagName
	}

	// Get release details
	release, err := client.ViewRelease(repo, releaseName)
	if err != nil {
		return fmt.Errorf("failed to view release: %w", err)
	}

	// Find previous release
	previousRelease, err := module_release.FindPreviousRelease(client, repo, releaseName)
	if err != nil {
		return fmt.Errorf("failed to find previous release: %w", err)
	}

	// Get PRs between releases
	prNumbers, err := module_release.ScanForPRs(client, repo, previousRelease, releaseName)
	if err != nil {
		return fmt.Errorf("failed to scan PRs: %w", err)
	}

	// Collect all linked issues
	issueMap := make(map[int]bool)
	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}

		linkedIssues := module_release.GetLinkedIssues(pr.Body, issuesRepoFlag)
		for _, issueNum := range linkedIssues {
			issueMap[issueNum] = true
		}
	}

	if len(issueMap) == 0 {
		fmt.Println("No linked issues found for this release.")
		return nil
	}

	// Create comment based on release type
	var commentBody string
	if release.IsPrerelease {
		commentBody = fmt.Sprintf("Testing release `%s` [%s](https://github.com/%s/releases/tag/%s)",
			repo, releaseName, repo, releaseName)
	} else {
		commentBody = fmt.Sprintf("Release `%s` [%s](https://github.com/%s/releases/tag/%s)",
			repo, releaseName, repo, releaseName)
	}

	// Post comments on open issues
	commentedCount := 0
	for issueNum := range issueMap {
		// Check if issue is open
		issue, err := client.GetIssue(issuesRepoFlag, issueNum)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to get issue %d: %v\n", issueNum, err)
			continue
		}

		if issue.State == "CLOSED" || issue.State == "closed" {
			continue
		}

		// Post comment on issue
		commentURL, err := client.CreateIssueComment(issuesRepoFlag, issueNum, commentBody)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to comment on issue %d: %v\n", issueNum, err)
			continue
		}

		fmt.Printf("✅ Commented on issue %s#%d\n   %s\n", issuesRepoFlag, issueNum, commentURL)
		commentedCount++

		// Check for parent issue and comment there too
		parentNum, err := client.GetParentIssueNumber(issuesRepoFlag, issueNum)
		if err == nil && parentNum > 0 {
			// Check if parent is open
			parentIssue, err := client.GetIssue(issuesRepoFlag, parentNum)
			if err == nil && parentIssue.State != "CLOSED" && parentIssue.State != "closed" {
				parentCommentURL, err := client.CreateIssueComment(issuesRepoFlag, parentNum, commentBody)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to comment on parent issue %d: %v\n", parentNum, err)
				} else {
					fmt.Printf("✅ Commented on parent issue %s#%d\n   %s\n", issuesRepoFlag, parentNum, parentCommentURL)
					commentedCount++
				}
			}
		}
	}

	if commentedCount == 0 {
		fmt.Println("No open issues to comment on.")
	} else {
		fmt.Printf("\n✅ Posted %d comment(s) successfully\n", commentedCount)
	}

	return nil
}
