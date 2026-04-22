package module_release

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

func TestGetLinkedIssuesPreservesBodyOrderAcrossFormats(t *testing.T) {
	body := strings.Join([]string{
		"Refs NethServer/dev#42",
		"https://github.com/NethServer/dev/issues/7",
		"NethServer/issues/9",
	}, " then ")

	got := GetLinkedIssues(body, "NethServer/dev")
	want := []int{42, 7, 9}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetLinkedIssues() = %v, want %v", got, want)
	}
}

type fakeRepoClient struct {
	repositories    map[string]*ghgithub.Repository
	repositoryErrs  map[string]error
	latestCommit    string
	latestCommitErr error
	mergeBases      map[string]string
	mergeBaseErrs   map[string]error
	releases        []ghgithub.Release
	listReleasesErr error
	refs            map[string]string
	refErrs         map[string]error
	comparisons     map[string]*ghgithub.CompareResult
	compareErrs     map[string]error
	commitPRs       map[string][]int
	pullRequestErrs map[string]error
}

func (f fakeRepoClient) GetRepository(repo string) (*ghgithub.Repository, error) {
	if err, ok := f.repositoryErrs[repo]; ok {
		return nil, err
	}
	if repository, ok := f.repositories[repo]; ok {
		return repository, nil
	}
	return nil, errors.New("repository not found")
}

func (f fakeRepoClient) GetLatestCommit(repo string) (string, error) {
	if f.latestCommitErr != nil {
		return "", f.latestCommitErr
	}
	if f.latestCommit != "" {
		return f.latestCommit, nil
	}
	return "", errors.New("latest commit not found")
}

func (f fakeRepoClient) GetMergeBase(repo, base, head string) (string, error) {
	key := repo + "|" + base + "|" + head
	if err, ok := f.mergeBaseErrs[key]; ok {
		return "", err
	}
	if mergeBase, ok := f.mergeBases[key]; ok {
		return mergeBase, nil
	}
	return "", errors.New("merge base not found")
}

func (f fakeRepoClient) ListReleases(_ string, limit int, excludePreReleases bool) ([]ghgithub.Release, error) {
	if f.listReleasesErr != nil {
		return nil, f.listReleasesErr
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

func (f fakeRepoClient) GetCommitSHA(repo, ref string) (string, error) {
	key := repo + "|" + ref
	if err, ok := f.refErrs[key]; ok {
		return "", err
	}
	if sha, ok := f.refs[key]; ok {
		return sha, nil
	}
	return "", errors.New("ref not found")
}

func (f fakeRepoClient) CompareCommits(repo, base, head string) (*ghgithub.CompareResult, error) {
	key := repo + "|" + base + "|" + head
	if err, ok := f.compareErrs[key]; ok {
		return nil, err
	}
	if comparison, ok := f.comparisons[key]; ok {
		return comparison, nil
	}
	return nil, errors.New("comparison not found")
}

func (f fakeRepoClient) GetPullRequestsForCommit(repo, sha string) ([]int, error) {
	key := repo + "|" + sha
	if err, ok := f.pullRequestErrs[key]; ok {
		return nil, err
	}
	return f.commitPRs[key], nil
}

func makeRepository(defaultBranch string) *ghgithub.Repository {
	repository := &ghgithub.Repository{}
	repository.DefaultBranchRef.Name = defaultBranch
	return repository
}

func makeCompareResult(shas ...string) *ghgithub.CompareResult {
	result := &ghgithub.CompareResult{
		Commits: make([]struct {
			SHA string `json:"sha"`
		}, len(shas)),
	}

	for i, sha := range shas {
		result.Commits[i].SHA = sha
	}

	return result
}

func TestValidateRepositoryRejectsInvalidPattern(t *testing.T) {
	err := ValidateRepository(fakeRepoClient{}, "NethServer/module")
	if err == nil || !strings.Contains(err.Error(), "invalid NS8 module name") {
		t.Fatalf("ValidateRepository() error = %v, want invalid NS8 module name", err)
	}
}

func TestValidateRepositoryWrapsRepositoryLookupFailure(t *testing.T) {
	client := fakeRepoClient{
		repositoryErrs: map[string]error{
			"NethServer/ns8-mail": errors.New("boom"),
		},
	}

	err := ValidateRepository(client, "NethServer/ns8-mail")
	if err == nil || !strings.Contains(err.Error(), "invalid repo: NethServer/ns8-mail") {
		t.Fatalf("ValidateRepository() error = %v, want invalid repo error", err)
	}
}

func TestGetOrValidateRepoUsesCurrentRepository(t *testing.T) {
	originalGetCurrentRepository := getCurrentRepository
	getCurrentRepository = func() (string, error) {
		return "NethServer/ns8-mail", nil
	}
	defer func() {
		getCurrentRepository = originalGetCurrentRepository
	}()

	client := fakeRepoClient{
		repositories: map[string]*ghgithub.Repository{
			"NethServer/ns8-mail": makeRepository("main"),
		},
	}

	got, err := GetOrValidateRepo(client, "")
	if err != nil {
		t.Fatalf("GetOrValidateRepo() returned error: %v", err)
	}
	if got != "NethServer/ns8-mail" {
		t.Fatalf("GetOrValidateRepo() = %q, want %q", got, "NethServer/ns8-mail")
	}
}

func TestGetOrValidateRepoReturnsFriendlyErrorWhenCurrentRepoFails(t *testing.T) {
	originalGetCurrentRepository := getCurrentRepository
	getCurrentRepository = func() (string, error) {
		return "", errors.New("boom")
	}
	defer func() {
		getCurrentRepository = originalGetCurrentRepository
	}()

	_, err := GetOrValidateRepo(fakeRepoClient{}, "")
	if err == nil || !strings.Contains(err.Error(), "could not determine the repo") {
		t.Fatalf("GetOrValidateRepo() error = %v, want repo detection error", err)
	}
}

func TestGetOrValidateCommitUsesLatestCommitWhenReleaseRefMissing(t *testing.T) {
	client := fakeRepoClient{latestCommit: "abc123"}

	info, err := GetOrValidateCommit(client, "NethServer/ns8-mail", "")
	if err != nil {
		t.Fatalf("GetOrValidateCommit() returned error: %v", err)
	}
	if info.SHA != "abc123" || info.Target != "" {
		t.Fatalf("GetOrValidateCommit() = %+v, want SHA abc123 and empty target", info)
	}
}

func TestGetOrValidateCommitRejectsCommitOutsideDefaultBranch(t *testing.T) {
	client := fakeRepoClient{
		repositories: map[string]*ghgithub.Repository{
			"NethServer/ns8-mail": makeRepository("main"),
		},
		mergeBases: map[string]string{
			"NethServer/ns8-mail|main|deadbeef": "cafebabe",
		},
	}

	_, err := GetOrValidateCommit(client, "NethServer/ns8-mail", "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "the commit sha is not on the default branch: main") {
		t.Fatalf("GetOrValidateCommit() error = %v, want default branch error", err)
	}
}

func TestGetLatestReleaseSkipsPrereleasesWhenRequested(t *testing.T) {
	client := fakeRepoClient{
		releases: []ghgithub.Release{
			{TagName: "1.2.1-testing.1", IsPrerelease: true},
			{TagName: "1.2.0", IsPrerelease: false},
		},
	}

	release, err := GetLatestRelease(client, "NethServer/ns8-mail", true)
	if err != nil {
		t.Fatalf("GetLatestRelease() returned error: %v", err)
	}
	if release.TagName != "1.2.0" {
		t.Fatalf("GetLatestRelease() tag = %q, want %q", release.TagName, "1.2.0")
	}
}

func TestGetLatestReleaseReturnsErrorWhenNoReleasesExist(t *testing.T) {
	_, err := GetLatestRelease(fakeRepoClient{}, "NethServer/ns8-mail", false)
	if err == nil || !strings.Contains(err.Error(), "no releases found") {
		t.Fatalf("GetLatestRelease() error = %v, want no releases found", err)
	}
}

func TestGetReleaseCommitSHAFormatsTagRef(t *testing.T) {
	client := fakeRepoClient{
		refs: map[string]string{
			"NethServer/ns8-mail|tags/v1.2.0": "sha123",
		},
	}

	sha, err := GetReleaseCommitSHA(client, "NethServer/ns8-mail", "v1.2.0")
	if err != nil {
		t.Fatalf("GetReleaseCommitSHA() returned error: %v", err)
	}
	if sha != "sha123" {
		t.Fatalf("GetReleaseCommitSHA() = %q, want %q", sha, "sha123")
	}
}

func TestGetMainBranchSHAUsesHeadsMainRef(t *testing.T) {
	client := fakeRepoClient{
		refs: map[string]string{
			"NethServer/ns8-mail|heads/main": "mainsha",
		},
	}

	sha, err := GetMainBranchSHA(client, "NethServer/ns8-mail")
	if err != nil {
		t.Fatalf("GetMainBranchSHA() returned error: %v", err)
	}
	if sha != "mainsha" {
		t.Fatalf("GetMainBranchSHA() = %q, want %q", sha, "mainsha")
	}
}

func TestScanForPRsDeduplicatesSortsAndSkipsCommitLookupFailures(t *testing.T) {
	client := fakeRepoClient{
		comparisons: map[string]*ghgithub.CompareResult{
			"NethServer/ns8-mail|v1.2.0|main": makeCompareResult("a", "b", "c"),
		},
		commitPRs: map[string][]int{
			"NethServer/ns8-mail|a": {9, 3},
			"NethServer/ns8-mail|b": {3, 7},
		},
		pullRequestErrs: map[string]error{
			"NethServer/ns8-mail|c": errors.New("boom"),
		},
	}

	got, err := ScanForPRs(client, "NethServer/ns8-mail", "v1.2.0", "main")
	if err != nil {
		t.Fatalf("ScanForPRs() returned error: %v", err)
	}

	want := []int{3, 7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanForPRs() = %v, want %v", got, want)
	}
}

func TestScanForPRsReturnsErrorWhenCommitRangeIsEmpty(t *testing.T) {
	client := fakeRepoClient{
		comparisons: map[string]*ghgithub.CompareResult{
			"NethServer/ns8-mail|v1.2.0|main": makeCompareResult(),
		},
	}

	_, err := ScanForPRs(client, "NethServer/ns8-mail", "v1.2.0", "main")
	if err == nil || !strings.Contains(err.Error(), "no commits found in the specified range") {
		t.Fatalf("ScanForPRs() error = %v, want no commits found", err)
	}
}

func TestScanForPRsReturnsErrorWhenNoPullRequestsAreFound(t *testing.T) {
	client := fakeRepoClient{
		comparisons: map[string]*ghgithub.CompareResult{
			"NethServer/ns8-mail|v1.2.0|main": makeCompareResult("a"),
		},
	}

	_, err := ScanForPRs(client, "NethServer/ns8-mail", "v1.2.0", "main")
	if err == nil || !strings.Contains(err.Error(), "no pull requests found") {
		t.Fatalf("ScanForPRs() error = %v, want no pull requests found", err)
	}
}
