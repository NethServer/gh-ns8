package module_release

import (
	"bytes"
	"errors"
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
	openPRs      map[string][]ghgithub.OpenPullRequest
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
	return f.openPRs[author], nil
}

func TestPopulateCheckSummaryCategorizesPRsAndOrphans(t *testing.T) {
	var errBuf bytes.Buffer
	client := fakeCheckSummaryClient{
		commitPRs: map[string][]int{
			"commit-a": {1},
			"commit-c": {2, 3, 5},
		},
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10 and NethServer/dev#20"},
			2: {
				Body: "Translation update",
				User: struct {
					Login string `json:"login"`
				}{Login: "weblate"},
			},
			3: {Body: "No linked issues"},
			5: {
				Body: "Bump dependency",
				User: struct {
					Login string `json:"login"`
				}{Login: "renovate[bot]"},
			},
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
	populateCheckSummary(&errBuf, client, summary, "NethServer/ns8-mail", makeCommandCompareResult("commit-a", "commit-b", "commit-c"), []int{1, 2, 3, 4, 5})

	if len(summary.WeblatePRs) != 1 || summary.WeblatePRs[0] != "https://github.com/NethServer/ns8-mail/pull/2" {
		t.Fatalf("WeblatePRs = %v, want weblate PR URL", summary.WeblatePRs)
	}
	if len(summary.RenovatePRs) != 1 || summary.RenovatePRs[0] != "https://github.com/NethServer/ns8-mail/pull/5" {
		t.Fatalf("RenovatePRs = %v, want renovate PR URL", summary.RenovatePRs)
	}
	if len(summary.UnlinkedPRs) != 1 || summary.UnlinkedPRs[0] != "https://github.com/NethServer/ns8-mail/pull/3" {
		t.Fatalf("UnlinkedPRs = %v, want unlinked PR URL", summary.UnlinkedPRs)
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
