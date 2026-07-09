package module_release

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

type stubIssueProvider struct {
	issues  map[int]*ghgithub.Issue
	parents map[int]int
}

func (s stubIssueProvider) GetIssue(_ string, number int) (*ghgithub.Issue, error) {
	return s.issues[number], nil
}

func (s stubIssueProvider) GetParentIssueNumber(_ string, issueNumber int) (int, error) {
	return s.parents[issueNumber], nil
}

func TestProcessIssueKeepsParentRefsAtZeroUntilDirectlyLinked(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	provider := stubIssueProvider{
		issues: map[int]*ghgithub.Issue{
			7310: {
				Number: 7310,
				State:  "OPEN",
				Labels: []struct {
					Name string "json:\"name\""
				}{
					{Name: "nethvoice"},
				},
			},
			7878: {
				Number: 7878,
				State:  "OPEN",
				Labels: []struct {
					Name string "json:\"name\""
				}{
					{Name: "nethvoice"},
				},
			},
		},
		parents: map[int]int{
			7878: 7310,
		},
	}

	if err := summary.ProcessIssue(provider, 7878); err != nil {
		t.Fatalf("ProcessIssue(child) returned error: %v", err)
	}

	if got := summary.Issues[7310].RefCount; got != 0 {
		t.Fatalf("parent refcount = %d, want 0", got)
	}
	if got := summary.Issues[7878].RefCount; got != 1 {
		t.Fatalf("child refcount = %d, want 1", got)
	}
	if len(summary.issueOrder) != 1 || summary.issueOrder[0] != 7310 {
		t.Fatalf("issueOrder = %v, want [7310]", summary.issueOrder)
	}

	if err := summary.ProcessIssue(provider, 7310); err != nil {
		t.Fatalf("ProcessIssue(parent) returned error: %v", err)
	}
	if got := summary.Issues[7310].RefCount; got != 1 {
		t.Fatalf("parent direct refcount = %d, want 1", got)
	}
}

func TestBashAssocKeyOrderMatchesLegacyOrdering(t *testing.T) {
	got := bashAssocKeyOrder([]int{7692, 7691, 7953, 7833, 7840, 7958, 7927, 7764, 7959, 7478, 7332, 7964, 7310})
	want := []int{7927, 7953, 7958, 7959, 7833, 7332, 7840, 7764, 7310, 7478, 7691, 7692, 7964}

	if len(got) != len(want) {
		t.Fatalf("bashAssocKeyOrder() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bashAssocKeyOrder() = %v, want %v", got, want)
		}
	}
}

func TestDisplayUsesLegacyIssueFormatting(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	summary.Issues[7927] = &IssueInfo{
		Number:   7927,
		Title:    "Verified issue title",
		Status:   EmojiOpenIssue,
		Progress: EmojiVerified,
		RefCount: 3,
		Labels:   "nethvoice",
	}
	summary.Issues[7310] = &IssueInfo{
		Number:   7310,
		Title:    "Parent issue title",
		Status:   EmojiOpenIssue,
		Progress: EmojiInProgress,
		RefCount: 0,
		Labels:   "nethvoice",
		Children: []int{7878},
	}
	summary.Issues[7878] = &IssueInfo{
		Number:       7878,
		Title:        "Child issue title",
		Status:       EmojiOpenIssue,
		Progress:     EmojiInProgress,
		RefCount:     2,
		Labels:       "nethvoice",
		ParentNumber: 7310,
	}
	summary.issueOrder = []int{7310, 7927}

	output := captureStdout(t, summary.Display)
	wantTop := "🟢── ✅ " + titleLink(7927, "Verified issue title", "https://github.com/NethServer/dev/issues/7927")
	if !strings.Contains(output, wantTop) {
		t.Fatalf("missing top-level formatting in output:\n%s", output)
	}
	wantParent := "🟢   🚧 " + titleLink(7310, "Parent issue title", "https://github.com/NethServer/dev/issues/7310")
	if !strings.Contains(output, wantParent) {
		t.Fatalf("missing parent formatting in output:\n%s", output)
	}
	wantChild := "└─🟢 🚧 " + titleLink(7878, "Child issue title", "https://github.com/NethServer/dev/issues/7878")
	if !strings.Contains(output, wantChild) {
		t.Fatalf("missing child formatting in output:\n%s", output)
	}
}

func TestDisplayShowsCategorizedPullRequests(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	mergeable := true

	summary.AddPullRequest("NethServer/ns8-test", makeDisplayPullRequest(12, "closed", true, nil, "", false, "dependencies"), PRCategoryRenovate)
	summary.AddPullRequest("NethServer/ns8-test", makeDisplayPullRequest(13, "open", false, nil, "unknown", false), PRCategoryTranslation)
	summary.AddPullRequest("NethServer/ns8-test", makeDisplayPullRequest(14, "closed", true, nil, "", false), PRCategoryMerged)
	summary.AddPullRequest("NethServer/ns8-test", makeDisplayPullRequest(10, "open", false, &mergeable, "clean", false, "verified", "nethvoice"), PRCategoryGeneric)

	output := captureStdout(t, summary.Display)
	if !strings.Contains(output, "PRs:") {
		t.Fatalf("missing PRs header in output:\n%s", output)
	}
	for _, unwanted := range []string{
		"Verified PRs:",
		"Testing PRs:",
		"Renovate PRs:",
		"Translation PRs:",
		"Merged PRs:",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("unexpected subgroup header %q in output:\n%s", unwanted, output)
		}
	}

	wantOrder := []string{
		"🟩    " + titleLink(13, "PR 13 title", "https://github.com/NethServer/ns8-test/pull/13"),
		"🟩    " + titleLink(10, "PR 10 title", "https://github.com/NethServer/ns8-test/pull/10") + " nethvoice",
		"🟪   🤖 " + titleLink(12, "PR 12 title", "https://github.com/NethServer/ns8-test/pull/12") + " dependencies",
		"🟪   🔀 " + titleLink(14, "PR 14 title", "https://github.com/NethServer/ns8-test/pull/14"),
	}
	lastIndex := -1
	for _, want := range wantOrder {
		index := strings.Index(output, want)
		if index == -1 {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
		if index <= lastIndex {
			t.Fatalf("pull request output is not in expected order:\n%s", output)
		}
		lastIndex = index
	}
}

func TestDisplayShowsLinkedPullRequestsUnderIssues(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	mergeable := true
	issue := &IssueInfo{
		Number:   100,
		Title:    "Linked issue title",
		Status:   EmojiOpenIssue,
		Progress: EmojiTesting,
		RefCount: 2,
	}
	issue.LinkedPRs = []PRInfo{
		newPRInfo("NethServer/ns8-test", makeDisplayPullRequest(11, "closed", true, nil, "", false), PRCategoryMerged),
		newPRInfo("NethServer/ns8-test", makeDisplayPullRequest(10, "open", false, &mergeable, "clean", false), PRCategoryGeneric),
	}
	summary.Issues[100] = issue
	summary.issueOrder = []int{100}

	output := captureStdout(t, summary.Display)
	if strings.Contains(output, "PRs:") {
		t.Fatalf("linked PRs should not create a top-level PR section:\n%s", output)
	}
	for _, want := range []string{
		"🟢── 🔨 " + titleLink(100, "Linked issue title", "https://github.com/NethServer/dev/issues/100"),
		"        • 🟩 " + titleLink(10, "PR 10 title", "https://github.com/NethServer/ns8-test/pull/10"),
		"        • 🟪 " + titleLink(11, "PR 11 title", "https://github.com/NethServer/ns8-test/pull/11"),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestDisplayGroupsIssuesByReleaseReadiness(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	summary.Issues[1] = &IssueInfo{
		Number:   1,
		Title:    "Ready issue",
		Status:   EmojiOpenIssue,
		Progress: EmojiVerified,
		LinkedPRs: []PRInfo{
			{Number: 11, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/11"},
		},
	}
	summary.Issues[2] = &IssueInfo{
		Number:   2,
		Title:    "Blocker issue",
		Status:   EmojiOpenIssue,
		Progress: EmojiTesting,
		LinkedPRs: []PRInfo{
			{Number: 12, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/12"},
			{Number: 13, Status: EmojiOpenPR, URL: "https://github.com/NethServer/ns8-test/pull/13"},
		},
	}
	summary.Issues[3] = &IssueInfo{
		Number:   3,
		Title:    "To be released issue",
		Status:   EmojiOpenIssue,
		Progress: EmojiVerified,
		LinkedPRs: []PRInfo{
			{Number: 14, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/14"},
			{Number: 15, Status: EmojiOpenPR, URL: "https://github.com/NethServer/ns8-test/pull/15"},
		},
	}
	summary.Issues[4] = &IssueInfo{
		Number:   4,
		Title:    "Other issue",
		Status:   EmojiOpenIssue,
		Progress: EmojiTesting,
		LinkedPRs: []PRInfo{
			{Number: 16, Status: EmojiOpenPR, URL: "https://github.com/NethServer/ns8-test/pull/16"},
		},
	}
	summary.issueOrder = []int{1, 2, 3, 4}

	output := captureStdout(t, summary.Display)
	readyIndex := strings.Index(output, "Ready to release:")
	readyIssueIndex := strings.Index(output, titleLink(1, "Ready issue", "https://github.com/NethServer/dev/issues/1"))
	toBeReleasedIndex := strings.Index(output, "To be released:")
	toBeReleasedIssueIndex := strings.Index(output, titleLink(3, "To be released issue", "https://github.com/NethServer/dev/issues/3"))
	blockerIndex := strings.Index(output, "Release blockers:")
	blockerIssueIndex := strings.Index(output, titleLink(2, "Blocker issue", "https://github.com/NethServer/dev/issues/2"))
	otherIndex := strings.Index(output, "Other issues:")
	otherIssueIndex := strings.Index(output, titleLink(4, "Other issue", "https://github.com/NethServer/dev/issues/4"))

	for label, index := range map[string]int{
		"ready heading":          readyIndex,
		"ready issue":            readyIssueIndex,
		"to be released heading": toBeReleasedIndex,
		"to be released issue":   toBeReleasedIssueIndex,
		"blocker heading":        blockerIndex,
		"blocker issue":          blockerIssueIndex,
		"other heading":          otherIndex,
		"other issue":            otherIssueIndex,
	} {
		if index == -1 {
			t.Fatalf("missing %s in output:\n%s", label, output)
		}
	}
	if !(readyIndex < readyIssueIndex && readyIssueIndex < toBeReleasedIndex &&
		toBeReleasedIndex < toBeReleasedIssueIndex && toBeReleasedIssueIndex < blockerIndex &&
		blockerIndex < blockerIssueIndex && blockerIssueIndex < otherIndex &&
		otherIndex < otherIssueIndex) {
		t.Fatalf("issue groups are not in expected order:\n%s", output)
	}
}

func TestDisplayGroupsChildrenIgnoringParentProgress(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	summary.Issues[100] = &IssueInfo{
		Number:   100,
		Title:    "Unverified parent",
		Status:   EmojiOpenIssue,
		Progress: EmojiInProgress,
		Children: []int{101},
	}
	summary.Issues[101] = &IssueInfo{
		Number:       101,
		Title:        "Verified child",
		Status:       EmojiOpenIssue,
		Progress:     EmojiVerified,
		ParentNumber: 100,
		LinkedPRs: []PRInfo{
			{Number: 20, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/20"},
		},
	}
	summary.issueOrder = []int{100}

	output := captureStdout(t, summary.Display)
	readyIndex := strings.Index(output, "Ready to release:")
	parentIndex := strings.Index(output, titleLink(100, "Unverified parent", "https://github.com/NethServer/dev/issues/100"))
	childIndex := strings.Index(output, titleLink(101, "Verified child", "https://github.com/NethServer/dev/issues/101"))
	blockerIndex := strings.Index(output, "Release blockers:")

	if readyIndex == -1 || parentIndex == -1 || childIndex == -1 {
		t.Fatalf("missing ready parent/child group in output:\n%s", output)
	}
	if blockerIndex != -1 {
		t.Fatalf("parent progress should not create a release blocker group:\n%s", output)
	}
	if !(readyIndex < parentIndex && parentIndex < childIndex) {
		t.Fatalf("parent and child are not shown in the ready group:\n%s", output)
	}
}

func TestDisplayPlacesLegendsUnderTheirLists(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	summary.AddPullRequest("NethServer/ns8-test", makeDisplayPullRequest(10, "closed", true, nil, "", false), PRCategoryMerged)
	summary.Issues[1] = &IssueInfo{Number: 1, Status: EmojiOpenIssue, Progress: EmojiInProgress}

	output := captureStdout(t, summary.Display)
	prIndex := strings.Index(output, "https://github.com/NethServer/ns8-test/pull/10")
	prLegendIndex := strings.Index(output, "PR status:")
	issuesIndex := strings.Index(output, "Issues:")
	issueIndex := strings.Index(output, "https://github.com/NethServer/dev/issues/1")
	issueLegendIndex := strings.Index(output, "Issue status:")
	if prIndex == -1 || prLegendIndex == -1 || issuesIndex == -1 || issueIndex == -1 || issueLegendIndex == -1 {
		t.Fatalf("missing PR, issue, or legend sections in output:\n%s", output)
	}
	if !(prIndex < prLegendIndex && prLegendIndex < issuesIndex) {
		t.Fatalf("PR legend should appear after PRs and before issues:\n%s", output)
	}
	if !(issueIndex < issueLegendIndex) {
		t.Fatalf("issue legend should appear after issue list:\n%s", output)
	}
	prGap := output[prIndex:prLegendIndex]
	if strings.Contains(prGap, "\n\n") {
		t.Fatalf("unexpected blank line between PR list and PR legend:\n%s", output)
	}
	gap := output[strings.Index(output, "PR type:"):issuesIndex]
	if !strings.Contains(gap, "\n\n") {
		t.Fatalf("expected blank line between PR legend and issues:\n%s", output)
	}
}

func TestDisplayReadyRequiresNoRemainingOrBlockedPRs(t *testing.T) {
	ready := NewCheckSummary("NethServer/dev")
	ready.Issues[1] = &IssueInfo{
		Number:   1,
		Progress: EmojiVerified,
		LinkedPRs: []PRInfo{
			{Number: 10, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/10"},
		},
	}
	output := captureStdout(t, ready.Display)
	if !strings.Contains(output, "All checks passed! Ready to release.") {
		t.Fatalf("missing ready message in output:\n%s", output)
	}

	withRemaining := NewCheckSummary("NethServer/dev")
	withRemaining.Issues[1] = &IssueInfo{
		Number:   1,
		Progress: EmojiVerified,
		LinkedPRs: []PRInfo{
			{Number: 10, Status: EmojiMergedPR, URL: "https://github.com/NethServer/ns8-test/pull/10"},
		},
	}
	withRemaining.MergedPRs = []PRInfo{{URL: "https://github.com/NethServer/ns8-test/pull/20"}}
	output = captureStdout(t, withRemaining.Display)
	if strings.Contains(output, "All checks passed! Ready to release.") {
		t.Fatalf("ready message should be hidden with remaining PRs:\n%s", output)
	}

	withUnmerged := NewCheckSummary("NethServer/dev")
	withUnmerged.Issues[1] = &IssueInfo{Number: 1, Progress: EmojiVerified}
	withUnmerged.Issues[1].LinkedPRs = []PRInfo{{URL: "https://github.com/NethServer/ns8-test/pull/21", Status: EmojiOpenPR, Mergeability: PRMergeable}}
	output = captureStdout(t, withUnmerged.Display)
	if strings.Contains(output, "All checks passed! Ready to release.") {
		t.Fatalf("ready message should be hidden with unmerged linked PRs:\n%s", output)
	}

	withBlocked := NewCheckSummary("NethServer/dev")
	withBlocked.Issues[1] = &IssueInfo{Number: 1, Progress: EmojiVerified}
	withBlocked.Issues[1].LinkedPRs = []PRInfo{{URL: "https://github.com/NethServer/ns8-test/pull/22", Status: EmojiOpenPR, Mergeability: PRBlocked}}
	output = captureStdout(t, withBlocked.Display)
	if strings.Contains(output, "All checks passed! Ready to release.") {
		t.Fatalf("ready message should be hidden with blocked open PRs:\n%s", output)
	}
}

func TestDisplayShowsOpenWeblateWarning(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")
	summary.OpenWeblatePRs = []string{
		"https://github.com/NethServer/ns8-test/pull/30",
	}

	output := captureStdout(t, summary.Display)
	if !strings.Contains(output, "Open Weblate PRs detected:") {
		t.Fatalf("missing open Weblate warning in output:\n%s", output)
	}
	if !strings.Contains(output, "pull/30") {
		t.Fatalf("missing open Weblate PR URL in output:\n%s", output)
	}
}

func TestDisplayHidesEmptySections(t *testing.T) {
	summary := NewCheckSummary("NethServer/dev")

	output := captureStdout(t, summary.Display)
	if strings.Contains(output, "PRs:") {
		t.Fatalf("should not show PR section when empty:\n%s", output)
	}
	if strings.Contains(output, "PR status:") {
		t.Fatalf("should not show PR legend when PR section is empty:\n%s", output)
	}
	if strings.Contains(output, "Renovate PRs:") {
		t.Fatalf("should not show Renovate section when empty:\n%s", output)
	}
	if strings.Contains(output, "Translation PRs:") {
		t.Fatalf("should not show Translation section when empty:\n%s", output)
	}
	if strings.Contains(output, "Open Weblate PRs detected:") {
		t.Fatalf("should not show open Weblate warning when empty:\n%s", output)
	}
}

func makeDisplayPullRequest(number int, state string, merged bool, mergeable *bool, mergeableState string, draft bool, labels ...string) *ghgithub.PullRequest {
	pr := &ghgithub.PullRequest{
		Number:         number,
		Title:          fmt.Sprintf("PR %d title", number),
		State:          state,
		Merged:         merged,
		Mergeable:      mergeable,
		MergeableState: mergeableState,
		Draft:          draft,
	}
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}

	os.Stdout = writer
	fn()
	writer.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("io.Copy() returned error: %v", err)
	}

	return buf.String()
}
