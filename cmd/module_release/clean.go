package module_release

import (
	"fmt"
	"io"

	"github.com/NethServer/gh-ns8/internal/github"
	"github.com/NethServer/gh-ns8/internal/module_release"
	"github.com/spf13/cobra"
)

// cleanCmd represents the clean command
var cleanCmd = &cobra.Command{
	Use:   "clean [VERSION]",
	Short: "Remove pre-releases between stable releases",
	Long:  `Delete all pre-release versions between two stable releases.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runClean,
}

type cleanReleaseLookupClient interface {
	ListReleases(repo string, limit int, excludePreReleases bool) ([]github.Release, error)
}

type releaseDeleter interface {
	DeleteRelease(repo, tag string) error
}

func runClean(cmd *cobra.Command, args []string) error {
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

	stableRelease, err := resolveStableRelease(client, repo, args)
	if err != nil {
		return err
	}

	// Find previous stable release
	previousRelease, err := module_release.FindPreviousRelease(client, repo, stableRelease)
	if err != nil {
		return fmt.Errorf("failed to find previous release: %w", err)
	}

	// Get pre-releases between the two stable releases
	preReleases, err := module_release.GetPreReleasesBetween(client, repo, previousRelease, stableRelease)
	if err != nil {
		return fmt.Errorf("failed to get pre-releases: %w", err)
	}

	deletePreReleases(cmd.OutOrStdout(), client, repo, previousRelease, stableRelease, preReleases)
	return nil
}

func resolveStableRelease(client cleanReleaseLookupClient, repo string, args []string) (string, error) {
	stableRelease := ""
	if len(args) > 0 {
		stableRelease = args[0]
	}

	if stableRelease != "" {
		return stableRelease, nil
	}

	release, err := module_release.GetLatestRelease(client, repo, true)
	if err != nil {
		return "", fmt.Errorf("no stable release found in the repository")
	}

	return release.TagName, nil
}

func deletePreReleases(out io.Writer, client releaseDeleter, repo, previousRelease, stableRelease string, preReleases []string) int {
	if len(preReleases) == 0 {
		fmt.Fprintf(out, "No pre-releases found between %s and %s\n", previousRelease, stableRelease)
		return 0
	}

	fmt.Fprintf(out, "Found %d pre-release(s) to delete between %s and %s:\n", len(preReleases), previousRelease, stableRelease)
	for _, tag := range preReleases {
		fmt.Fprintf(out, "  - %s\n", tag)
	}
	fmt.Fprintln(out)

	deletedCount := 0
	for _, tag := range preReleases {
		fmt.Fprintf(out, "Deleting %s... ", tag)
		if err := client.DeleteRelease(repo, tag); err != nil {
			fmt.Fprintf(out, "❌ Failed: %v\n", err)
			continue
		}
		fmt.Fprintln(out, "✅")
		deletedCount++
	}

	fmt.Fprintf(out, "\n✅ Deleted %d pre-release(s) successfully\n", deletedCount)
	return deletedCount
}
