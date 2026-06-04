package module_release

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
	internalmodule "github.com/NethServer/gh-ns8/internal/module_release"
)

type fakeCheckSummaryClient struct {
	commitPRs    map[string][]int
	commitErrs   map[string]error
	prs          map[int]*ghgithub.PullRequest
	prErrs       map[int]error
	issues       map[int]*ghgithub.Issue
	issueErrs    map[int]error
	parentIssues map[int]int
	parentErrs   map[int]error
	openByAuthor map[string][]ghgithub.OpenPullRequest
	openByLabel  map[string][]ghgithub.OpenPullRequest
}

func (f fakeCheckSummaryClient) GetPullRequestsForCommit(_ string, sha string) ([]int, error) {
	if err, ok := f.commitErrs[sha]; ok {
		return nil, err
	}
	return f.commitPRs[sha], nil
}

func (f fakeCheckSummaryClient) GetPullRequest(_ string, number int) (*ghgithub.PullRequest, error) {
	if err, ok := f.prErrs[number]; ok {
		return nil, err
	}
	return f.prs[number], nil
}

func (f fakeCheckSummaryClient) GetIssue(_ string, number int) (*ghgithub.Issue, error) {
	if err, ok := f.issueErrs[number]; ok {
		return nil, err
	}
	return f.issues[number], nil
}

func (f fakeCheckSummaryClient) GetParentIssueNumber(_ string, issueNumber int) (int, error) {
	if err, ok := f.parentErrs[issueNumber]; ok {
		return 0, err
	}
	return f.parentIssues[issueNumber], nil
}

func (f fakeCheckSummaryClient) ListOpenPullRequestsByAuthor(_ string, author string) ([]ghgithub.OpenPullRequest, error) {
	return f.openByAuthor[author], nil
}

func (f fakeCheckSummaryClient) ListOpenPullRequestsByLabel(_ string, label string) ([]ghgithub.OpenPullRequest, error) {
	return f.openByLabel[label], nil
}

func TestPopulateCheckSummaryCategorizesPRsAndOrphans(t *testing.T) {
	var errBuf bytes.Buffer
	client := fakeCheckSummaryClient{
		commitPRs: map[string][]int{
			"commit-a": {1},
			"commit-c": {2, 3, 5},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: makeTestPullRequest(1, "Refs NethServer/dev#10 and NethServer/dev#20", "", "closed", true),
			2: makeTestPullRequest(2, "Translation update", "weblate", "closed", true),
			3: makeTestPullRequest(3, "No linked issues", "", "closed", true),
			5: makeTestPullRequest(5, "Bump dependency", "renovate[bot]", "closed", true),
		},
		prErrs: map[int]error{
			4: errors.New("missing PR"),
		},
		issues: map[int]*ghgithub.Issue{
			10: {
				State: "OPEN",
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "testing"},
				},
			},
			100: {
				State: "OPEN",
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "verified"},
				},
			},
		},
		issueErrs: map[int]error{
			20: errors.New("missing issue"),
		},
		parentIssues: map[int]int{
			10: 100,
		},
	}

	summary := internalmodule.NewCheckSummary("NethServer/dev")
	seenPRs := populateCheckSummary(&errBuf, client, summary, "NethServer/ns8-mail", makeCommandCompareResult("commit-a", "commit-b", "commit-c"), []int{1, 2, 3, 4, 5})

	if len(seenPRs) != 4 || !seenPRs[1] || !seenPRs[2] || !seenPRs[3] || !seenPRs[5] {
		t.Fatalf("seenPRs = %v, want successfully loaded PRs", seenPRs)
	}
	if len(summary.TranslationPRs) != 1 || summary.TranslationPRs[0].URL != "https://github.com/NethServer/ns8-mail/pull/2" {
		t.Fatalf("TranslationPRs = %v, want weblate PR URL", summary.TranslationPRs)
	}
	if len(summary.RenovatePRs) != 1 || summary.RenovatePRs[0].URL != "https://github.com/NethServer/ns8-mail/pull/5" {
		t.Fatalf("RenovatePRs = %v, want renovate PR URL", summary.RenovatePRs)
	}
	if len(summary.MergedPRs) != 2 || summary.MergedPRs[0].Number != 1 || summary.MergedPRs[1].Number != 3 {
		t.Fatalf("MergedPRs = %v, want remaining merged PRs 1 and 3", summary.MergedPRs)
	}
	if len(summary.OrphanCommits) != 1 || summary.OrphanCommits[0] != "https://github.com/NethServer/ns8-mail/commit/commit-b" {
		t.Fatalf("OrphanCommits = %v, want orphan commit URL", summary.OrphanCommits)
	}

	issue := summary.Issues[10]
	if issue == nil {
		t.Fatal("summary.Issues[10] = nil, want issue info")
	}
	if issue.ParentNumber != 100 {
		t.Fatalf("summary.Issues[10].ParentNumber = %d, want 100", issue.ParentNumber)
	}
	if issue.Progress != internalmodule.EmojiTesting {
		t.Fatalf("summary.Issues[10].Progress = %q, want %q", issue.Progress, internalmodule.EmojiTesting)
	}

	parent := summary.Issues[100]
	if parent == nil {
		t.Fatal("summary.Issues[100] = nil, want parent issue info")
	}
	if len(parent.Children) != 1 || parent.Children[0] != 10 {
		t.Fatalf("summary.Issues[100].Children = %v, want [10]", parent.Children)
	}
	if parent.Progress != internalmodule.EmojiVerified {
		t.Fatalf("summary.Issues[100].Progress = %q, want %q", parent.Progress, internalmodule.EmojiVerified)
	}

	wantWarning := "Warning: failed to process issue 20: failed to get issue 20: missing issue\n"
	if errBuf.String() != wantWarning {
		t.Fatalf("warnings = %q, want %q", errBuf.String(), wantWarning)
	}
}

func TestPopulateOpenPullRequestsAddsRelevantOpenPRs(t *testing.T) {
	var errBuf bytes.Buffer
	mergeable := true
	blocked := false
	client := fakeCheckSummaryClient{
		prs: map[int]*ghgithub.PullRequest{
			6:  makeTestPullRequest(6, "Translation update", "weblate", "open", false, "verified"),
			7:  makeTestPullRequest(7, "Refs NethServer/dev#30", "", "open", false, "verified"),
			8:  makeTestPullRequest(8, "Testing change", "", "open", false, "testing"),
			9:  makeTestPullRequest(9, "Renovate testing", "renovate[bot]", "open", false, "testing"),
			10: makeTestPullRequest(10, "Already handled", "", "open", false, "verified"),
		},
		issues: map[int]*ghgithub.Issue{
			30: {
				State: "OPEN",
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "verified"},
				},
			},
		},
		openByAuthor: map[string][]ghgithub.OpenPullRequest{
			"weblate": {
				{Number: 6, URL: "https://github.com/NethServer/ns8-mail/pull/6"},
			},
		},
		openByLabel: map[string][]ghgithub.OpenPullRequest{
			"verified": {
				{Number: 6, URL: "https://github.com/NethServer/ns8-mail/pull/6"},
				{Number: 7, URL: "https://github.com/NethServer/ns8-mail/pull/7"},
				{Number: 10, URL: "https://github.com/NethServer/ns8-mail/pull/10"},
			},
			"testing": {
				{Number: 8, URL: "https://github.com/NethServer/ns8-mail/pull/8"},
				{Number: 9, URL: "https://github.com/NethServer/ns8-mail/pull/9"},
			},
		},
	}
	client.prs[7].MergeableState = "unknown"
	client.prs[8].Mergeable = &blocked
	client.prs[8].MergeableState = "dirty"
	client.prs[9].Mergeable = &mergeable
	client.prs[9].MergeableState = "clean"

	summary := internalmodule.NewCheckSummary("NethServer/dev")
	populateOpenPullRequests(&errBuf, client, summary, "NethServer/ns8-mail", map[int]bool{10: true})

	if errBuf.Len() != 0 {
		t.Fatalf("warnings = %q, want none", errBuf.String())
	}
	if len(summary.OpenWeblatePRs) != 1 || summary.OpenWeblatePRs[0] != "https://github.com/NethServer/ns8-mail/pull/6" {
		t.Fatalf("OpenWeblatePRs = %v, want Weblate warning URL", summary.OpenWeblatePRs)
	}
	if len(summary.TranslationPRs) != 1 || summary.TranslationPRs[0].Number != 6 {
		t.Fatalf("TranslationPRs = %v, want open Weblate PR", summary.TranslationPRs)
	}
	if len(summary.VerifiedPRs) != 1 || summary.VerifiedPRs[0].Number != 7 {
		t.Fatalf("VerifiedPRs = %v, want open verified PR", summary.VerifiedPRs)
	}
	if len(summary.TestingPRs) != 2 || summary.TestingPRs[0].Number != 8 || summary.TestingPRs[1].Number != 9 {
		t.Fatalf("TestingPRs = %v, want open testing PRs", summary.TestingPRs)
	}
	if len(summary.RenovatePRs) != 0 {
		t.Fatalf("RenovatePRs = %v, want no open renovate PRs in renovate bucket", summary.RenovatePRs)
	}
	if summary.Issues[30] == nil || summary.Issues[30].Progress != internalmodule.EmojiVerified {
		t.Fatalf("Issues[30] = %v, want processed linked issue", summary.Issues[30])
	}
}

func TestCategorizePullRequestPrecedence(t *testing.T) {
	tests := []struct {
		name string
		pr   *ghgithub.PullRequest
		want internalmodule.PRCategory
	}{
		{
			name: "weblate wins over verified",
			pr:   makeTestPullRequest(1, "", "weblate", "open", false, "verified"),
			want: internalmodule.PRCategoryTranslation,
		},
		{
			name: "merged renovate wins over verified",
			pr:   makeTestPullRequest(2, "", "renovate[bot]", "closed", true, "verified"),
			want: internalmodule.PRCategoryRenovate,
		},
		{
			name: "open renovate can use testing label",
			pr:   makeTestPullRequest(3, "", "renovate[bot]", "open", false, "testing"),
			want: internalmodule.PRCategoryTesting,
		},
		{
			name: "verified label wins over testing",
			pr:   makeTestPullRequest(4, "", "", "open", false, "testing", "verified"),
			want: internalmodule.PRCategoryVerified,
		},
		{
			name: "merged testing label stays merged",
			pr:   makeTestPullRequest(5, "", "", "closed", true, "testing"),
			want: internalmodule.PRCategoryMerged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := categorizePullRequest(tt.pr); got != tt.want {
				t.Fatalf("categorizePullRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func makeTestPullRequest(number int, body, author, state string, merged bool, labels ...string) *ghgithub.PullRequest {
	pr := &ghgithub.PullRequest{
		Number:  number,
		Body:    body,
		State:   state,
		Merged:  merged,
		HTMLURL: "https://github.com/NethServer/ns8-mail/pull/" + strconv.Itoa(number),
	}
	pr.User.Login = author
	pr.Labels = make([]struct {
		Name string `json:"name"`
	}, 0, len(labels))
	for _, label := range labels {
		pr.Labels = append(pr.Labels, struct {
			Name string `json:"name"`
		}{Name: label})
	}
	return pr
}
