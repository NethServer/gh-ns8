package module_release

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/NethServer/gh-ns8/internal/github"
	"github.com/NethServer/gh-ns8/internal/module_release"
	"github.com/spf13/cobra"
)

var (
	releaseRefsFlag      string
	testingFlag          bool
	draftFlag            bool
	withLinkedIssuesFlag bool
)

type linkedIssuesNotesClient interface {
	CompareCommits(repo, base, head string) (*github.CompareResult, error)
	GetPullRequestsForCommit(repo, sha string) ([]int, error)
	GetPullRequest(repo string, number int) (*github.PullRequest, error)
	GetIssue(repo string, number int) (*github.Issue, error)
}

type createReleaseFlowClient interface {
	ListReleases(repo string, limit int, excludePreReleases bool) ([]github.Release, error)
	GetCommitSHA(repo, ref string) (string, error)
}

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create [VERSION]",
	Short: "Create a new release",
	Long:  `Create a new release for a NethServer 8 module with automatic version generation and release notes.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVar(&releaseRefsFlag, "release-refs", "", "Commit SHA to associate with the release")
	createCmd.Flags().BoolVar(&testingFlag, "testing", false, "Create a testing release")
	createCmd.Flags().BoolVar(&draftFlag, "draft", false, "Create a draft release")
	createCmd.Flags().BoolVar(&withLinkedIssuesFlag, "with-linked-issues", false, "Include linked issues from PRs in release notes")
}

func runCreate(cmd *cobra.Command, args []string) error {
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

	// Get or validate commit
	commitInfo, err := module_release.GetOrValidateCommit(client, repo, releaseRefsFlag)
	if err != nil {
		return err
	}

	releaseName, isPrerelease, err := resolveCreateReleaseName(client, repo, args, testingFlag)
	if err != nil {
		return err
	}

	previousRelease := previousReleaseForCreate(client, repo, isPrerelease)
	notesReader := linkedIssuesNotesReader(client, repo, previousRelease, issuesRepoFlag, withLinkedIssuesFlag)

	// Create the release
	target := commitInfo.Target
	if err := client.CreateRelease(repo, releaseName, releaseName, draftFlag, isPrerelease, target, notesReader); err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	fmt.Printf("✅ Release %s created successfully\n", releaseName)
	return nil
}

func resolveCreateReleaseName(client createReleaseFlowClient, repo string, args []string, testing bool) (string, bool, error) {
	releaseName := ""
	if len(args) > 0 {
		releaseName = args[0]
	}

	isPrerelease := testing || strings.Contains(releaseName, "-")

	if testing && releaseName == "" {
		nextRelease, err := module_release.NextTestingRelease(client, repo)
		if err != nil {
			return "", false, fmt.Errorf("failed to generate testing release name: %w", err)
		}
		releaseName = nextRelease
	}

	if releaseName == "" && !testing {
		return "", false, fmt.Errorf("please provide the release name as an argument")
	}

	if releaseName != "" && !module_release.IsSemver(releaseName) {
		return "", false, fmt.Errorf("invalid semver format for release name: %s", releaseName)
	}

	return releaseName, isPrerelease, nil
}

func previousReleaseForCreate(client createReleaseFlowClient, repo string, isPrerelease bool) string {
	release, err := module_release.GetLatestRelease(client, repo, !isPrerelease)
	if err != nil {
		return ""
	}

	return release.TagName
}

func linkedIssuesNotesReader(client linkedIssuesNotesClient, repo, previousRelease, issuesRepo string, include bool) io.Reader {
	if !include || previousRelease == "" {
		return nil
	}

	notes, err := generateLinkedIssuesNotes(client, repo, previousRelease, issuesRepo)
	if err != nil || notes == "" {
		return nil
	}

	return bytes.NewBufferString(notes)
}

// generateLinkedIssuesNotes generates release notes with linked issues
func generateLinkedIssuesNotes(client linkedIssuesNotesClient, repo, previousRelease, issuesRepo string) (string, error) {
	// Scan for PRs
	prNumbers, err := module_release.ScanForPRs(client, repo, previousRelease, "main")
	if err != nil {
		return "", err
	}

	// Collect linked issues
	issueMap := make(map[int]string)
	for _, prNum := range prNumbers {
		pr, err := client.GetPullRequest(repo, prNum)
		if err != nil {
			continue
		}

		linkedIssues := module_release.GetLinkedIssues(pr.Body, issuesRepo)
		for _, issueNum := range linkedIssues {
			if _, exists := issueMap[issueNum]; !exists {
				// Get issue title
				issue, err := client.GetIssue(issuesRepo, issueNum)
				if err == nil {
					issueMap[issueNum] = issue.Title
				}
			}
		}
	}

	if len(issueMap) == 0 {
		return "", nil
	}

	// Format notes
	var notes strings.Builder
	notes.WriteString("## Linked Issues\n")
	issueNumbers := make([]int, 0, len(issueMap))
	for issueNum := range issueMap {
		issueNumbers = append(issueNumbers, issueNum)
	}
	sort.Ints(issueNumbers)
	for _, issueNum := range issueNumbers {
		title := issueMap[issueNum]
		notes.WriteString(fmt.Sprintf("- [%s#%d](https://github.com/%s/issues/%d): %s\n",
			issuesRepo, issueNum, issuesRepo, issueNum, title))
	}

	return notes.String(), nil
}
