package module_release

import (
	"bytes"
	"fmt"
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

	// Get release name from argument or auto-generate
	releaseName := ""
	if len(args) > 0 {
		releaseName = args[0]
	}

	// Determine if this is a prerelease
	isPrerelease := testingFlag || strings.Contains(releaseName, "-")

	// Generate release name if testing and not provided
	if testingFlag && releaseName == "" {
		releaseName, err = module_release.NextTestingRelease(client, repo)
		if err != nil {
			return fmt.Errorf("failed to generate testing release name: %w", err)
		}
	}

	// Validate release name if provided
	if releaseName == "" && !testingFlag {
		return fmt.Errorf("please provide the release name as an argument")
	}

	if releaseName != "" && !module_release.IsSemver(releaseName) {
		return fmt.Errorf("invalid semver format for release name: %s", releaseName)
	}

	// Get previous release for release notes
	var previousRelease string
	if isPrerelease {
		// Get latest release (any type)
		release, err := module_release.GetLatestRelease(client, repo, false)
		if err == nil {
			previousRelease = release.TagName
		}
	} else {
		// Get latest stable release
		release, err := module_release.GetLatestRelease(client, repo, true)
		if err == nil {
			previousRelease = release.TagName
		}
	}

	// Generate release notes with linked issues if requested
	var notesReader *bytes.Buffer
	if withLinkedIssuesFlag && previousRelease != "" {
		notes, err := generateLinkedIssuesNotes(client, repo, previousRelease, issuesRepoFlag)
		if err == nil && notes != "" {
			notesReader = bytes.NewBufferString(notes)
		}
	}

	// Create the release
	target := commitInfo.Target
	if err := client.CreateRelease(repo, releaseName, releaseName, draftFlag, isPrerelease, target, notesReader); err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	fmt.Printf("✅ Release %s created successfully\n", releaseName)
	return nil
}

// generateLinkedIssuesNotes generates release notes with linked issues
func generateLinkedIssuesNotes(client *github.Client, repo, previousRelease, issuesRepo string) (string, error) {
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
	for issueNum, title := range issueMap {
		notes.WriteString(fmt.Sprintf("- [%s#%d](https://github.com/%s/issues/%d): %s\n",
			issuesRepo, issueNum, issuesRepo, issueNum, title))
	}

	return notes.String(), nil
}
