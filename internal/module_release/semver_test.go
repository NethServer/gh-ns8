package module_release

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

type fakeReleaseClient struct {
	releases     []ghgithub.Release
	viewReleases map[string]ghgithub.Release
	refs         map[string]string
	listErr      error
	viewErr      error
	refErrs      map[string]error
}

func (f fakeReleaseClient) ListReleases(_ string, limit int, excludePreReleases bool) ([]ghgithub.Release, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	releases := make([]ghgithub.Release, 0, len(f.releases))
	for _, release := range f.releases {
		if excludePreReleases && release.IsPrerelease {
			continue
		}
		releases = append(releases, release)
		if limit > 0 && len(releases) == limit {
			break
		}
	}

	return releases, nil
}

func (f fakeReleaseClient) ViewRelease(_ string, tag string) (*ghgithub.Release, error) {
	if f.viewErr != nil {
		return nil, f.viewErr
	}
	if release, ok := f.viewReleases[tag]; ok {
		copy := release
		return &copy, nil
	}
	for _, release := range f.releases {
		if release.TagName == tag {
			copy := release
			return &copy, nil
		}
	}
	return nil, errors.New("release not found")
}

func (f fakeReleaseClient) GetCommitSHA(repo, ref string) (string, error) {
	key := repo + "|" + ref
	if err, ok := f.refErrs[key]; ok {
		return "", err
	}
	if sha, ok := f.refs[key]; ok {
		return sha, nil
	}
	return "", errors.New("ref not found")
}

func TestIsSemver(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "stable", version: "1.2.3", want: true},
		{name: "prerelease", version: "1.2.3-testing.1", want: true},
		{name: "build metadata", version: "1.2.3+build.7", want: true},
		{name: "missing patch", version: "1.2", want: false},
		{name: "leading zero", version: "01.2.3", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := IsSemver(testCase.version); got != testCase.want {
				t.Fatalf("IsSemver(%q) = %v, want %v", testCase.version, got, testCase.want)
			}
		})
	}
}

func TestNextTestingReleaseFromStableRelease(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.3", IsPrerelease: false},
		},
		refs: map[string]string{
			"NethServer/ns8-mail|tags/1.2.3": "release-sha",
			"NethServer/ns8-mail|heads/main": "main-sha",
		},
	}

	got, err := NextTestingRelease(client, "NethServer/ns8-mail")
	if err != nil {
		t.Fatalf("NextTestingRelease() returned error: %v", err)
	}
	if got != "1.2.4-testing.1" {
		t.Fatalf("NextTestingRelease() = %q, want %q", got, "1.2.4-testing.1")
	}
}

func TestNextTestingReleaseFromPrerelease(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.4-testing.2", IsPrerelease: true},
		},
		refs: map[string]string{
			"NethServer/ns8-mail|tags/1.2.4-testing.2": "release-sha",
			"NethServer/ns8-mail|heads/main":           "main-sha",
		},
	}

	got, err := NextTestingRelease(client, "NethServer/ns8-mail")
	if err != nil {
		t.Fatalf("NextTestingRelease() returned error: %v", err)
	}
	if got != "1.2.4-testing.3" {
		t.Fatalf("NextTestingRelease() = %q, want %q", got, "1.2.4-testing.3")
	}
}

func TestNextTestingReleaseRejectsHeadRelease(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.3", IsPrerelease: false},
		},
		refs: map[string]string{
			"NethServer/ns8-mail|tags/1.2.3": "same-sha",
			"NethServer/ns8-mail|heads/main": "same-sha",
		},
	}

	_, err := NextTestingRelease(client, "NethServer/ns8-mail")
	if err == nil || !strings.Contains(err.Error(), "the latest release tag is the HEAD of the main branch") {
		t.Fatalf("NextTestingRelease() error = %v, want head release error", err)
	}
}

func TestNextTestingReleaseRejectsInvalidSemver(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "latest", IsPrerelease: false},
		},
	}

	_, err := NextTestingRelease(client, "NethServer/ns8-mail")
	if err == nil || !strings.Contains(err.Error(), "invalid semver format for the latest release") {
		t.Fatalf("NextTestingRelease() error = %v, want invalid semver error", err)
	}
}

func TestIncrementTestingNumber(t *testing.T) {
	got, err := incrementTestingNumber("1.2.3-testing.9")
	if err != nil {
		t.Fatalf("incrementTestingNumber() returned error: %v", err)
	}
	if got != "1.2.3-testing.10" {
		t.Fatalf("incrementTestingNumber() = %q, want %q", got, "1.2.3-testing.10")
	}

	_, err = incrementTestingNumber("1.2.3")
	if err == nil || !strings.Contains(err.Error(), "invalid testing version format") {
		t.Fatalf("incrementTestingNumber() error = %v, want invalid format error", err)
	}
}

func TestIncrementPatchAndAddTesting(t *testing.T) {
	got, err := incrementPatchAndAddTesting("1.2.3")
	if err != nil {
		t.Fatalf("incrementPatchAndAddTesting() returned error: %v", err)
	}
	if got != "1.2.4-testing.1" {
		t.Fatalf("incrementPatchAndAddTesting() = %q, want %q", got, "1.2.4-testing.1")
	}

	_, err = incrementPatchAndAddTesting("release")
	if err == nil || !strings.Contains(err.Error(), "invalid semver format") {
		t.Fatalf("incrementPatchAndAddTesting() error = %v, want invalid semver error", err)
	}
}

func TestFindPreviousReleaseForStableReleaseSkipsPrereleases(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.1", IsPrerelease: false},
			{TagName: "1.2.1-testing.2", IsPrerelease: true},
			{TagName: "1.2.1-testing.1", IsPrerelease: true},
			{TagName: "1.2.0", IsPrerelease: false},
		},
		viewReleases: map[string]ghgithub.Release{
			"1.2.1": {TagName: "1.2.1", IsPrerelease: false},
		},
	}

	got, err := FindPreviousRelease(client, "NethServer/ns8-mail", "1.2.1")
	if err != nil {
		t.Fatalf("FindPreviousRelease() returned error: %v", err)
	}
	if got != "1.2.0" {
		t.Fatalf("FindPreviousRelease() = %q, want %q", got, "1.2.0")
	}
}

func TestFindPreviousReleaseForPrereleaseReturnsPreviousEntry(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.1-testing.2", IsPrerelease: true},
			{TagName: "1.2.1-testing.1", IsPrerelease: true},
			{TagName: "1.2.0", IsPrerelease: false},
		},
		viewReleases: map[string]ghgithub.Release{
			"1.2.1-testing.2": {TagName: "1.2.1-testing.2", IsPrerelease: true},
		},
	}

	got, err := FindPreviousRelease(client, "NethServer/ns8-mail", "1.2.1-testing.2")
	if err != nil {
		t.Fatalf("FindPreviousRelease() returned error: %v", err)
	}
	if got != "1.2.1-testing.1" {
		t.Fatalf("FindPreviousRelease() = %q, want %q", got, "1.2.1-testing.1")
	}
}

func TestFindPreviousReleaseErrorsWhenCurrentReleaseIsMissing(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.0", IsPrerelease: false},
		},
		viewReleases: map[string]ghgithub.Release{
			"1.2.1": {TagName: "1.2.1", IsPrerelease: false},
		},
	}

	_, err := FindPreviousRelease(client, "NethServer/ns8-mail", "1.2.1")
	if err == nil || !strings.Contains(err.Error(), "current release not found in release list") {
		t.Fatalf("FindPreviousRelease() error = %v, want current release missing error", err)
	}
}

func TestGetPreReleasesBetweenFiltersReleaseWindow(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.2", IsPrerelease: false, CreatedAt: "2024-01-10T00:00:00Z"},
			{TagName: "1.2.2-testing.2", IsPrerelease: true, CreatedAt: "2024-01-08T00:00:00Z"},
			{TagName: "1.2.2-testing.1", IsPrerelease: true, CreatedAt: "2024-01-05T00:00:00Z"},
			{TagName: "1.2.1", IsPrerelease: false, CreatedAt: "2024-01-01T00:00:00Z"},
			{TagName: "1.2.0-testing.9", IsPrerelease: true, CreatedAt: "2023-12-31T23:59:59Z"},
		},
	}

	got, err := GetPreReleasesBetween(client, "NethServer/ns8-mail", "1.2.1", "1.2.2")
	if err != nil {
		t.Fatalf("GetPreReleasesBetween() returned error: %v", err)
	}

	want := []string{"1.2.2-testing.2", "1.2.2-testing.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetPreReleasesBetween() = %v, want %v", got, want)
	}
}

func TestGetPreReleasesBetweenErrorsWhenBoundsAreMissing(t *testing.T) {
	client := fakeReleaseClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.1", IsPrerelease: false, CreatedAt: "2024-01-01T00:00:00Z"},
		},
	}

	_, err := GetPreReleasesBetween(client, "NethServer/ns8-mail", "1.2.0", "1.2.1")
	if err == nil || !strings.Contains(err.Error(), "could not find start or end release") {
		t.Fatalf("GetPreReleasesBetween() error = %v, want missing bound error", err)
	}
}

func TestIsPrerelease(t *testing.T) {
	if !IsPrerelease("1.2.3-testing.1") {
		t.Fatal("IsPrerelease() = false, want true")
	}
	if IsPrerelease("1.2.3") {
		t.Fatal("IsPrerelease() = true, want false")
	}
}
