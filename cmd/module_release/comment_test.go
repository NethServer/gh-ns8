package module_release

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

func TestReleaseCommentBody(t *testing.T) {
	testCases := []struct {
		name       string
		prerelease bool
		want       string
	}{
		{
			name:       "stable",
			prerelease: false,
			want:       "Release `NethServer/ns8-mail` [1.2.3](https://github.com/NethServer/ns8-mail/releases/tag/1.2.3)",
		},
		{
			name:       "testing",
			prerelease: true,
			want:       "Testing release `NethServer/ns8-mail` [1.2.3-testing.1](https://github.com/NethServer/ns8-mail/releases/tag/1.2.3-testing.1)",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			releaseName := "1.2.3"
			if testCase.prerelease {
				releaseName = "1.2.3-testing.1"
			}

			got := releaseCommentBody("NethServer/ns8-mail", releaseName, testCase.prerelease)
			if got != testCase.want {
				t.Fatalf("releaseCommentBody() = %q, want %q", got, testCase.want)
			}
		})
	}
}

type fakeCommentClient struct {
	prs          map[int]*ghgithub.PullRequest
	prErrs       map[int]error
	issues       map[int]*ghgithub.Issue
	issueErrs    map[int]error
	commentErrs  map[int]error
	commentURLs  map[int]string
	parentIssues map[int]int
	parentErrs   map[int]error
	commented    []int
}

func (f *fakeCommentClient) GetPullRequest(_ string, number int) (*ghgithub.PullRequest, error) {
	if err, ok := f.prErrs[number]; ok {
		return nil, err
	}
	return f.prs[number], nil
}

func (f *fakeCommentClient) GetIssue(_ string, number int) (*ghgithub.Issue, error) {
	if err, ok := f.issueErrs[number]; ok {
		return nil, err
	}
	return f.issues[number], nil
}

func (f *fakeCommentClient) CreateIssueComment(_ string, number int, _ string) (string, error) {
	f.commented = append(f.commented, number)
	if err, ok := f.commentErrs[number]; ok {
		return "", err
	}
	if url, ok := f.commentURLs[number]; ok {
		return url, nil
	}
	return "", nil
}

func (f *fakeCommentClient) GetParentIssueNumber(_ string, issueNumber int) (int, error) {
	if err, ok := f.parentErrs[issueNumber]; ok {
		return 0, err
	}
	return f.parentIssues[issueNumber], nil
}

func TestCollectLinkedIssuesDeduplicatesAndSkipsPRFailures(t *testing.T) {
	client := &fakeCommentClient{
		prs: map[int]*ghgithub.PullRequest{
			1: {Body: "Refs NethServer/dev#10 and NethServer/dev#11"},
			2: {Body: "Refs NethServer/dev#11"},
		},
		prErrs: map[int]error{
			3: errors.New("failed"),
		},
	}

	got := collectLinkedIssues(client, "NethServer/ns8-mail", "NethServer/dev", []int{1, 2, 3})
	if len(got) != 2 || !got[10] || !got[11] {
		t.Fatalf("collectLinkedIssues() = %v, want issues 10 and 11", got)
	}
}

func TestPostReleaseCommentsHandlesOpenClosedAndParentIssues(t *testing.T) {
	client := &fakeCommentClient{
		issues: map[int]*ghgithub.Issue{
			10: {State: "OPEN"},
			11: {State: "closed"},
			12: {State: "OPEN"},
			20: {State: "OPEN"},
		},
		issueErrs: map[int]error{
			13: errors.New("issue lookup failed"),
		},
		commentErrs: map[int]error{
			12: errors.New("comment failed"),
		},
		commentURLs: map[int]string{
			10: "https://example.test/issues/10#comment",
			20: "https://example.test/issues/20#comment",
		},
		parentIssues: map[int]int{
			10: 20,
		},
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	commentedCount := postReleaseComments(&out, &errBuf, client, "NethServer/dev", "body", map[int]bool{
		10: true,
		11: true,
		12: true,
		13: true,
	})

	if commentedCount != 2 {
		t.Fatalf("postReleaseComments() = %d, want %d", commentedCount, 2)
	}

	wantCommented := []int{10, 20, 12}
	if !reflect.DeepEqual(client.commented, wantCommented) {
		t.Fatalf("postReleaseComments() commented = %v, want %v", client.commented, wantCommented)
	}

	wantOut := "✅ Commented on issue NethServer/dev#10\n" +
		"   https://example.test/issues/10#comment\n" +
		"✅ Commented on parent issue NethServer/dev#20\n" +
		"   https://example.test/issues/20#comment\n" +
		"\n✅ Posted 2 comment(s) successfully\n"
	if out.String() != wantOut {
		t.Fatalf("postReleaseComments() stdout = %q, want %q", out.String(), wantOut)
	}

	wantErr := "Warning: failed to comment on issue 12: comment failed\n" +
		"Warning: failed to get issue 13: issue lookup failed\n"
	if errBuf.String() != wantErr {
		t.Fatalf("postReleaseComments() stderr = %q, want %q", errBuf.String(), wantErr)
	}
}

func TestPostReleaseCommentsReportsWhenNoOpenIssuesRemain(t *testing.T) {
	client := &fakeCommentClient{
		issues: map[int]*ghgithub.Issue{
			10: {State: "CLOSED"},
		},
		issueErrs: map[int]error{
			11: errors.New("issue lookup failed"),
		},
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	commentedCount := postReleaseComments(&out, &errBuf, client, "NethServer/dev", "body", map[int]bool{
		10: true,
		11: true,
	})

	if commentedCount != 0 {
		t.Fatalf("postReleaseComments() = %d, want %d", commentedCount, 0)
	}
	if out.String() != "No open issues to comment on.\n" {
		t.Fatalf("postReleaseComments() stdout = %q, want %q", out.String(), "No open issues to comment on.\n")
	}
	if errBuf.String() != "Warning: failed to get issue 11: issue lookup failed\n" {
		t.Fatalf("postReleaseComments() stderr = %q, want warning", errBuf.String())
	}
}

func TestPostReleaseCommentsWarnsWhenParentCommentFails(t *testing.T) {
	client := &fakeCommentClient{
		issues: map[int]*ghgithub.Issue{
			10: {State: "OPEN"},
			20: {State: "OPEN"},
		},
		commentErrs: map[int]error{
			20: errors.New("parent comment failed"),
		},
		commentURLs: map[int]string{
			10: "https://example.test/issues/10#comment",
		},
		parentIssues: map[int]int{
			10: 20,
		},
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	commentedCount := postReleaseComments(&out, &errBuf, client, "NethServer/dev", "body", map[int]bool{
		10: true,
	})

	if commentedCount != 1 {
		t.Fatalf("postReleaseComments() = %d, want %d", commentedCount, 1)
	}

	wantOut := "✅ Commented on issue NethServer/dev#10\n" +
		"   https://example.test/issues/10#comment\n" +
		"\n✅ Posted 1 comment(s) successfully\n"
	if out.String() != wantOut {
		t.Fatalf("postReleaseComments() stdout = %q, want %q", out.String(), wantOut)
	}

	wantErr := "Warning: failed to comment on parent issue 20: parent comment failed\n"
	if errBuf.String() != wantErr {
		t.Fatalf("postReleaseComments() stderr = %q, want %q", errBuf.String(), wantErr)
	}
}
