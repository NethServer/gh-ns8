package module_release

import (
	"fmt"
	"io"
	"sort"

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

type linkedIssueCollector interface {
	GetPullRequest(repo string, number int) (*github.PullRequest, error)
}

type issueCommentClient interface {
	GetIssue(repo string, number int) (*github.Issue, error)
	CreateIssueComment(repo string, number int, body string) (string, error)
	GetParentIssueNumber(repo string, issueNumber int) (int, error)
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

	issueMap := collectLinkedIssues(client, repo, issuesRepoFlag, prNumbers)

	if len(issueMap) == 0 {
		fmt.Println("No linked issues found for this release.")
		return nil
	}

	// Create comment based on release type
	commentBody := releaseCommentBody(repo, releaseName, release.IsPrerelease)

	postReleaseComments(cmd.OutOrStdout(), cmd.ErrOrStderr(), client, issuesRepoFlag, commentBody, issueMap)

	return nil
}

func collectLinkedIssues(client linkedIssueCollector, repo, issuesRepo string, prNumbers []int) map[int]bool {
	issueMap := make(map[int]bool)
	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}

		linkedIssues := module_release.GetLinkedIssues(pr.Body, issuesRepo)
		for _, issueNum := range linkedIssues {
			issueMap[issueNum] = true
		}
	}

	return issueMap
}

func postReleaseComments(out, errWriter io.Writer, client issueCommentClient, issuesRepo, commentBody string, issueMap map[int]bool) int {
	issueNumbers := make([]int, 0, len(issueMap))
	for issueNum := range issueMap {
		issueNumbers = append(issueNumbers, issueNum)
	}
	sort.Ints(issueNumbers)

	commentedCount := 0
	for _, issueNum := range issueNumbers {
		issue, err := client.GetIssue(issuesRepo, issueNum)
		if err != nil {
			fmt.Fprintf(errWriter, "Warning: failed to get issue %d: %v\n", issueNum, err)
			continue
		}

		if issue.State == "CLOSED" || issue.State == "closed" {
			continue
		}

		commentURL, err := client.CreateIssueComment(issuesRepo, issueNum, commentBody)
		if err != nil {
			fmt.Fprintf(errWriter, "Warning: failed to comment on issue %d: %v\n", issueNum, err)
			continue
		}

		fmt.Fprintf(out, "✅ Commented on issue %s#%d\n   %s\n", issuesRepo, issueNum, commentURL)
		commentedCount++

		parentNum, err := client.GetParentIssueNumber(issuesRepo, issueNum)
		if err != nil || parentNum <= 0 {
			continue
		}

		parentIssue, err := client.GetIssue(issuesRepo, parentNum)
		if err != nil || parentIssue.State == "CLOSED" || parentIssue.State == "closed" {
			continue
		}

		parentCommentURL, err := client.CreateIssueComment(issuesRepo, parentNum, commentBody)
		if err != nil {
			fmt.Fprintf(errWriter, "Warning: failed to comment on parent issue %d: %v\n", parentNum, err)
			continue
		}

		fmt.Fprintf(out, "✅ Commented on parent issue %s#%d\n   %s\n", issuesRepo, parentNum, parentCommentURL)
		commentedCount++
	}

	if commentedCount == 0 {
		fmt.Fprintln(out, "No open issues to comment on.")
	} else {
		fmt.Fprintf(out, "\n✅ Posted %d comment(s) successfully\n", commentedCount)
	}

	return commentedCount
}

func releaseCommentBody(repo, releaseName string, prerelease bool) string {
	if prerelease {
		return fmt.Sprintf("Testing release `%s` [%s](https://github.com/%s/releases/tag/%s)",
			repo, releaseName, repo, releaseName)
	}

	return fmt.Sprintf("Release `%s` [%s](https://github.com/%s/releases/tag/%s)",
		repo, releaseName, repo, releaseName)
}
