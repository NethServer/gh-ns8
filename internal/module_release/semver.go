package module_release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/NethServer/gh-ns8/internal/github"
)

// Official semver regex from semver.org
var semverRegex = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$`)

// IsSemver checks if a string is valid semver format
func IsSemver(version string) bool {
	return semverRegex.MatchString(version)
}

// NextTestingRelease generates the next testing release name
func NextTestingRelease(client *github.Client, repo string) (string, error) {
	// Get the latest release (including pre-releases)
	releases, err := client.ListReleases(repo, 1, false)
	if err != nil {
		return "", fmt.Errorf("failed to get latest release: %w", err)
	}

	if len(releases) == 0 {
		return "", fmt.Errorf("no releases found")
	}

	latestRelease := releases[0]

	// Validate semver
	if !IsSemver(latestRelease.TagName) {
		return "", fmt.Errorf("invalid semver format for the latest release: %s", latestRelease.TagName)
	}

	// Check if the latest release is at HEAD of main
	latestSHA, err := GetReleaseCommitSHA(client, repo, latestRelease.TagName)
	if err != nil {
		return "", fmt.Errorf("failed to get release commit SHA: %w", err)
	}

	mainSHA, err := GetMainBranchSHA(client, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get main branch SHA: %w", err)
	}

	if latestSHA == mainSHA {
		return "", fmt.Errorf("the latest release tag is the HEAD of the main branch")
	}

	// Determine next version based on whether current is prerelease
	if latestRelease.IsPrerelease {
		// Increment testing number: 1.0.1-testing.1 -> 1.0.1-testing.2
		return incrementTestingNumber(latestRelease.TagName)
	}

	// Increment patch and add -testing.1: 1.0.0 -> 1.0.1-testing.1
	return incrementPatchAndAddTesting(latestRelease.TagName)
}

// incrementTestingNumber increments the testing number in a prerelease version
func incrementTestingNumber(version string) (string, error) {
	// Match version-testing.N pattern
	re := regexp.MustCompile(`^(.*-testing\.)(\d+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) != 3 {
		return "", fmt.Errorf("invalid testing version format: %s", version)
	}

	prefix := matches[1]
	num, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", fmt.Errorf("failed to parse testing number: %w", err)
	}

	return fmt.Sprintf("%s%d", prefix, num+1), nil
}

// incrementPatchAndAddTesting increments the patch version and adds -testing.1
func incrementPatchAndAddTesting(version string) (string, error) {
	// Match X.Y.Z pattern
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(version)
	if len(matches) != 4 {
		return "", fmt.Errorf("invalid semver format: %s", version)
	}

	major := matches[1]
	minor := matches[2]
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return "", fmt.Errorf("failed to parse patch version: %w", err)
	}

	return fmt.Sprintf("%s.%s.%d-testing.1", major, minor, patch+1), nil
}

// FindPreviousRelease finds the previous release based on creation date
func FindPreviousRelease(client *github.Client, repo, currentTag string) (string, error) {
	// Check if current release is a pre-release
	currentRelease, err := client.ViewRelease(repo, currentTag)
	if err != nil {
		return "", fmt.Errorf("failed to view current release: %w", err)
	}

	// Get all releases (up to 1000)
	allReleases, err := client.ListReleases(repo, 1000, false)
	if err != nil {
		return "", fmt.Errorf("failed to list releases: %w", err)
	}

	// Find current release index
	currentIndex := -1
	for i, r := range allReleases {
		if r.TagName == currentTag {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return "", fmt.Errorf("current release not found in release list")
	}

	if currentIndex == len(allReleases)-1 {
		return "", fmt.Errorf("no previous release found")
	}

	// If current is prerelease, return previous release (any type)
	if currentRelease.IsPrerelease {
		return allReleases[currentIndex+1].TagName, nil
	}

	// If current is stable, find previous stable release
	for i := currentIndex + 1; i < len(allReleases); i++ {
		if !allReleases[i].IsPrerelease {
			return allReleases[i].TagName, nil
		}
	}

	return "", fmt.Errorf("no previous stable release found")
}

// GetPreReleasesBetween gets pre-releases between two releases
func GetPreReleasesBetween(client *github.Client, repo, startTag, endTag string) ([]string, error) {
	allReleases, err := client.ListReleases(repo, 1000, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var startTime, endTime string
	for _, r := range allReleases {
		if r.TagName == startTag {
			startTime = r.CreatedAt
		}
		if r.TagName == endTag {
			endTime = r.CreatedAt
		}
	}

	if startTime == "" || endTime == "" {
		return nil, fmt.Errorf("could not find start or end release")
	}

	var preReleases []string
	for _, r := range allReleases {
		if r.IsPrerelease && r.CreatedAt > startTime && r.CreatedAt <= endTime {
			preReleases = append(preReleases, r.TagName)
		}
	}

	return preReleases, nil
}

// IsPrerelease checks if a version string indicates a prerelease
func IsPrerelease(version string) bool {
	return strings.Contains(version, "-")
}
