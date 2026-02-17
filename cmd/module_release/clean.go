package module_release

import (
	"fmt"

	"github.com/NethServer/gh-ns8/internal/github"
	"github.com/NethServer/gh-ns8/internal/module_release"
	"github.com/spf13/cobra"
)

// cleanCmd represents the clean command
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove pre-releases between stable releases",
	Long:  `Delete all pre-release versions between two stable releases.`,
	RunE:  runClean,
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

	// Get stable release name (use latest stable if not provided)
	stableRelease := releaseNameFlag
	if stableRelease == "" {
		release, err := module_release.GetLatestRelease(client, repo, true)
		if err != nil {
			return fmt.Errorf("no stable release found in the repository")
		}
		stableRelease = release.TagName
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

	if len(preReleases) == 0 {
		fmt.Printf("No pre-releases found between %s and %s\n", previousRelease, stableRelease)
		return nil
	}

	fmt.Printf("Found %d pre-release(s) to delete between %s and %s:\n", len(preReleases), previousRelease, stableRelease)
	for _, tag := range preReleases {
		fmt.Printf("  - %s\n", tag)
	}
	fmt.Println()

	// Delete each pre-release
	deletedCount := 0
	for _, tag := range preReleases {
		fmt.Printf("Deleting %s... ", tag)
		if err := client.DeleteRelease(repo, tag); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			continue
		}
		fmt.Println("✅")
		deletedCount++
	}

	fmt.Printf("\n✅ Deleted %d pre-release(s) successfully\n", deletedCount)
	return nil
}
