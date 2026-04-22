package module_release

import (
	"errors"
	"io"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

type fakeLinkedIssuesNotesClient struct {
	comparison *ghgithub.CompareResult
	compareErr error
	commitPRs  map[string][]int
	prErrs     map[int]error
	prs        map[int]*ghgithub.PullRequest
	issueErrs  map[int]error
	issues     map[int]*ghgithub.Issue
}

func (f fakeLinkedIssuesNotesClient) CompareCommits(_, _, _ string) (*ghgithub.CompareResult, error) {
	if f.compareErr != nil {
		return nil, f.compareErr
	}
	return f.comparison, nil
}

func (f fakeLinkedIssuesNotesClient) GetPullRequestsForCommit(_ string, sha string) ([]int, error) {
	return f.commitPRs[sha], nil
}

func (f fakeLinkedIssuesNotesClient) GetPullRequest(_ string, number int) (*ghgithub.PullRequest, error) {
	if err, ok := f.prErrs[number]; ok {
		return nil, err
	}
	return f.prs[number], nil
}

func (f fakeLinkedIssuesNotesClient) GetIssue(_ string, number int) (*ghgithub.Issue, error) {
	if err, ok := f.issueErrs[number]; ok {
		return nil, err
	}
	return f.issues[number], nil
}

func makeCommandCompareResult(shas ...string) *ghgithub.CompareResult {
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

func TestGenerateLinkedIssuesNotesDeduplicatesAndSortsIssues(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a", "commit-b"),
		commitPRs: map[string][]int{
			"commit-a": {2, 1},
			"commit-b": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10 and NethServer/dev#3"},
			2: {Body: "Refs NethServer/dev#10"},
		},
		issues: map[int]*ghgithub.Issue{
			3:  {Title: "Third issue"},
			10: {Title: "Tenth issue"},
		},
	}

	got, err := generateLinkedIssuesNotes(client, "NethServer/ns8-mail", "1.2.0", "NethServer/dev")
	if err != nil {
		t.Fatalf("generateLinkedIssuesNotes() returned error: %v", err)
	}

	want := "## Linked Issues\n" +
		"- [NethServer/dev#3](https://github.com/NethServer/dev/issues/3): Third issue\n" +
		"- [NethServer/dev#10](https://github.com/NethServer/dev/issues/10): Tenth issue\n"
	if got != want {
		t.Fatalf("generateLinkedIssuesNotes() = %q, want %q", got, want)
	}
}

func TestGenerateLinkedIssuesNotesReturnsEmptyWhenNoIssuesAreLinked(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a"),
		commitPRs: map[string][]int{
			"commit-a": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "No linked issues here"},
		},
	}

	got, err := generateLinkedIssuesNotes(client, "NethServer/ns8-mail", "1.2.0", "NethServer/dev")
	if err != nil {
		t.Fatalf("generateLinkedIssuesNotes() returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("generateLinkedIssuesNotes() = %q, want empty string", got)
	}
}

func TestGenerateLinkedIssuesNotesSkipsUnavailableIssues(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a"),
		commitPRs: map[string][]int{
			"commit-a": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10"},
		},
		issueErrs: map[int]error{
			10: errors.New("boom"),
		},
	}

	got, err := generateLinkedIssuesNotes(client, "NethServer/ns8-mail", "1.2.0", "NethServer/dev")
	if err != nil {
		t.Fatalf("generateLinkedIssuesNotes() returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("generateLinkedIssuesNotes() = %q, want empty string", got)
	}
}

type fakeCreateReleaseFlowClient struct {
	releasesByExclude map[bool][]ghgithub.Release
	listErr           error
	listCalls         []bool
	commitSHAs        map[string]string
	commitErrs        map[string]error
}

func (f *fakeCreateReleaseFlowClient) ListReleases(_ string, _ int, excludePreReleases bool) ([]ghgithub.Release, error) {
	f.listCalls = append(f.listCalls, excludePreReleases)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.releasesByExclude[excludePreReleases], nil
}

func (f *fakeCreateReleaseFlowClient) GetCommitSHA(_, ref string) (string, error) {
	if err, ok := f.commitErrs[ref]; ok {
		return "", err
	}
	return f.commitSHAs[ref], nil
}

func TestResolveCreateReleaseNameAutoGeneratesTestingRelease(t *testing.T) {
	client := &fakeCreateReleaseFlowClient{
		releasesByExclude: map[bool][]ghgithub.Release{
			false: {
				{TagName: "1.2.3"},
			},
		},
		commitSHAs: map[string]string{
			"tags/1.2.3": "release-sha",
			"heads/main": "main-sha",
		},
	}

	gotName, gotPrerelease, err := resolveCreateReleaseName(client, "NethServer/ns8-mail", nil, true)
	if err != nil {
		t.Fatalf("resolveCreateReleaseName() returned error: %v", err)
	}
	if gotName != "1.2.4-testing.1" {
		t.Fatalf("resolveCreateReleaseName() name = %q, want %q", gotName, "1.2.4-testing.1")
	}
	if !gotPrerelease {
		t.Fatal("resolveCreateReleaseName() prerelease = false, want true")
	}
	if len(client.listCalls) != 1 || client.listCalls[0] {
		t.Fatalf("resolveCreateReleaseName() listCalls = %v, want [false]", client.listCalls)
	}
}

func TestResolveCreateReleaseNameRequiresVersionForStableRelease(t *testing.T) {
	_, _, err := resolveCreateReleaseName(&fakeCreateReleaseFlowClient{}, "NethServer/ns8-mail", nil, false)
	if err == nil || err.Error() != "please provide the release name as an argument" {
		t.Fatalf("resolveCreateReleaseName() error = %v, want missing release name error", err)
	}
}

func TestResolveCreateReleaseNameRejectsInvalidSemver(t *testing.T) {
	_, _, err := resolveCreateReleaseName(&fakeCreateReleaseFlowClient{}, "NethServer/ns8-mail", []string{"latest"}, false)
	if err == nil || err.Error() != "invalid semver format for release name: latest" {
		t.Fatalf("resolveCreateReleaseName() error = %v, want invalid semver error", err)
	}
}

func TestResolveCreateReleaseNameMarksExplicitPrerelease(t *testing.T) {
	gotName, gotPrerelease, err := resolveCreateReleaseName(&fakeCreateReleaseFlowClient{}, "NethServer/ns8-mail", []string{"1.2.4-testing.3"}, false)
	if err != nil {
		t.Fatalf("resolveCreateReleaseName() returned error: %v", err)
	}
	if gotName != "1.2.4-testing.3" {
		t.Fatalf("resolveCreateReleaseName() name = %q, want %q", gotName, "1.2.4-testing.3")
	}
	if !gotPrerelease {
		t.Fatal("resolveCreateReleaseName() prerelease = false, want true")
	}
}

func TestResolveCreateReleaseNameReturnsTestingGenerationError(t *testing.T) {
	client := &fakeCreateReleaseFlowClient{
		releasesByExclude: map[bool][]ghgithub.Release{
			false: {
				{TagName: "1.2.3"},
			},
		},
		commitSHAs: map[string]string{
			"tags/1.2.3": "same-sha",
			"heads/main": "same-sha",
		},
	}

	_, _, err := resolveCreateReleaseName(client, "NethServer/ns8-mail", nil, true)
	want := "failed to generate testing release name: the latest release tag is the HEAD of the main branch"
	if err == nil || err.Error() != want {
		t.Fatalf("resolveCreateReleaseName() error = %v, want %q", err, want)
	}
}

func TestPreviousReleaseForCreateChoosesStableOrAnyRelease(t *testing.T) {
	client := &fakeCreateReleaseFlowClient{
		releasesByExclude: map[bool][]ghgithub.Release{
			true: {
				{TagName: "1.2.3"},
			},
			false: {
				{TagName: "1.2.4-testing.1"},
			},
		},
	}

	gotStable := previousReleaseForCreate(client, "NethServer/ns8-mail", false)
	if gotStable != "1.2.3" {
		t.Fatalf("previousReleaseForCreate() stable = %q, want %q", gotStable, "1.2.3")
	}

	gotPrerelease := previousReleaseForCreate(client, "NethServer/ns8-mail", true)
	if gotPrerelease != "1.2.4-testing.1" {
		t.Fatalf("previousReleaseForCreate() prerelease = %q, want %q", gotPrerelease, "1.2.4-testing.1")
	}

	if len(client.listCalls) != 2 || !client.listCalls[0] || client.listCalls[1] {
		t.Fatalf("previousReleaseForCreate() listCalls = %v, want [true false]", client.listCalls)
	}
}

func TestPreviousReleaseForCreateReturnsEmptyOnLookupError(t *testing.T) {
	got := previousReleaseForCreate(&fakeCreateReleaseFlowClient{listErr: errors.New("boom")}, "NethServer/ns8-mail", false)
	if got != "" {
		t.Fatalf("previousReleaseForCreate() = %q, want empty string", got)
	}
}

func TestLinkedIssuesNotesReaderReturnsNotesWhenEnabled(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a"),
		commitPRs: map[string][]int{
			"commit-a": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10"},
		},
		issues: map[int]*ghgithub.Issue{
			10: {Title: "Issue title"},
		},
	}

	reader := linkedIssuesNotesReader(client, "NethServer/ns8-mail", "1.2.3", "NethServer/dev", true)
	if reader == nil {
		t.Fatal("linkedIssuesNotesReader() = nil, want reader")
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() returned error: %v", err)
	}

	want := "## Linked Issues\n" +
		"- [NethServer/dev#10](https://github.com/NethServer/dev/issues/10): Issue title\n"
	if string(got) != want {
		t.Fatalf("linkedIssuesNotesReader() = %q, want %q", string(got), want)
	}
}

func TestLinkedIssuesNotesReaderSkipsDisabledOrEmptyRanges(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a"),
		commitPRs: map[string][]int{
			"commit-a": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10"},
		},
		issues: map[int]*ghgithub.Issue{
			10: {Title: "Issue title"},
		},
	}

	if reader := linkedIssuesNotesReader(client, "NethServer/ns8-mail", "", "NethServer/dev", true); reader != nil {
		t.Fatalf("linkedIssuesNotesReader() = %v, want nil when previous release is empty", reader)
	}

	if reader := linkedIssuesNotesReader(client, "NethServer/ns8-mail", "1.2.3", "NethServer/dev", false); reader != nil {
		t.Fatalf("linkedIssuesNotesReader() = %v, want nil when notes are disabled", reader)
	}
}

func TestLinkedIssuesNotesReaderReturnsNilWhenNotesAreEmpty(t *testing.T) {
	client := fakeLinkedIssuesNotesClient{
		comparison: makeCommandCompareResult("commit-a"),
		commitPRs: map[string][]int{
			"commit-a": {1},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "No linked issues here"},
		},
	}

	if reader := linkedIssuesNotesReader(client, "NethServer/ns8-mail", "1.2.3", "NethServer/dev", true); reader != nil {
		t.Fatalf("linkedIssuesNotesReader() = %v, want nil when generated notes are empty", reader)
	}
}
