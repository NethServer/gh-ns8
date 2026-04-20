package module_release

import (
	"bytes"
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
		Status:   EmojiOpenIssue,
		Progress: EmojiVerified,
		RefCount: 3,
		Labels:   "nethvoice",
	}
	summary.Issues[7310] = &IssueInfo{
		Number:   7310,
		Status:   EmojiOpenIssue,
		Progress: EmojiInProgress,
		RefCount: 0,
		Labels:   "nethvoice",
		Children: []int{7878},
	}
	summary.Issues[7878] = &IssueInfo{
		Number:       7878,
		Status:       EmojiOpenIssue,
		Progress:     EmojiInProgress,
		RefCount:     2,
		Labels:       "nethvoice",
		ParentNumber: 7310,
	}
	summary.issueOrder = []int{7310, 7927}

	output := captureStdout(t, summary.Display)
	if !strings.Contains(output, "🟢   ✅ https://github.com/NethServer/dev/issues/7927 (3) nethvoice") {
		t.Fatalf("missing legacy top-level formatting in output:\n%s", output)
	}
	if !strings.Contains(output, "└─🟢 🚧 https://github.com/NethServer/dev/issues/7878 (2) nethvoice") {
		t.Fatalf("missing legacy child formatting in output:\n%s", output)
	}
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
