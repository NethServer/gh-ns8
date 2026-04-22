package module_release

import (
	"bytes"
	"errors"
	"testing"

	ghgithub "github.com/NethServer/gh-ns8/internal/github"
)

type fakeCleanClient struct {
	releasesByExclude map[bool][]ghgithub.Release
	listErr           error
	listCalls         []bool
	deleteErrs        map[string]error
	deleted           []string
}

func (f *fakeCleanClient) ListReleases(_ string, _ int, excludePreReleases bool) ([]ghgithub.Release, error) {
	f.listCalls = append(f.listCalls, excludePreReleases)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.releasesByExclude[excludePreReleases], nil
}

func (f *fakeCleanClient) DeleteRelease(_ string, tag string) error {
	f.deleted = append(f.deleted, tag)
	if err, ok := f.deleteErrs[tag]; ok {
		return err
	}
	return nil
}

func TestResolveStableRelease(t *testing.T) {
	t.Run("uses explicit argument", func(t *testing.T) {
		client := &fakeCleanClient{}

		got, err := resolveStableRelease(client, "NethServer/ns8-mail", []string{"1.2.3"})
		if err != nil {
			t.Fatalf("resolveStableRelease() returned error: %v", err)
		}
		if got != "1.2.3" {
			t.Fatalf("resolveStableRelease() = %q, want %q", got, "1.2.3")
		}
		if len(client.listCalls) != 0 {
			t.Fatalf("resolveStableRelease() listCalls = %v, want no list calls", client.listCalls)
		}
	})

	t.Run("uses latest stable release", func(t *testing.T) {
		client := &fakeCleanClient{
			releasesByExclude: map[bool][]ghgithub.Release{
				true: {
					{TagName: "1.2.4"},
				},
			},
		}

		got, err := resolveStableRelease(client, "NethServer/ns8-mail", nil)
		if err != nil {
			t.Fatalf("resolveStableRelease() returned error: %v", err)
		}
		if got != "1.2.4" {
			t.Fatalf("resolveStableRelease() = %q, want %q", got, "1.2.4")
		}
		if len(client.listCalls) != 1 || !client.listCalls[0] {
			t.Fatalf("resolveStableRelease() listCalls = %v, want [true]", client.listCalls)
		}
	})

	t.Run("returns stable release error", func(t *testing.T) {
		_, err := resolveStableRelease(&fakeCleanClient{}, "NethServer/ns8-mail", nil)
		if err == nil || err.Error() != "no stable release found in the repository" {
			t.Fatalf("resolveStableRelease() error = %v, want stable release error", err)
		}
	})
}

func TestDeletePreReleasesReportsProgressAndContinues(t *testing.T) {
	client := &fakeCleanClient{
		deleteErrs: map[string]error{
			"1.2.4-testing.2": errors.New("delete failed"),
		},
	}

	var out bytes.Buffer
	deletedCount := deletePreReleases(&out, client, "NethServer/ns8-mail", "1.2.3", "1.2.4", []string{"1.2.4-testing.1", "1.2.4-testing.2"})

	if deletedCount != 1 {
		t.Fatalf("deletePreReleases() = %d, want %d", deletedCount, 1)
	}
	if len(client.deleted) != 2 || client.deleted[0] != "1.2.4-testing.1" || client.deleted[1] != "1.2.4-testing.2" {
		t.Fatalf("deletePreReleases() deleted = %v, want both tags", client.deleted)
	}

	want := "Found 2 pre-release(s) to delete between 1.2.3 and 1.2.4:\n" +
		"  - 1.2.4-testing.1\n" +
		"  - 1.2.4-testing.2\n\n" +
		"Deleting 1.2.4-testing.1... ✅\n" +
		"Deleting 1.2.4-testing.2... ❌ Failed: delete failed\n" +
		"\n✅ Deleted 1 pre-release(s) successfully\n"
	if out.String() != want {
		t.Fatalf("deletePreReleases() output = %q, want %q", out.String(), want)
	}
}

func TestDeletePreReleasesReportsWhenNoneAreFound(t *testing.T) {
	var out bytes.Buffer

	deletedCount := deletePreReleases(&out, &fakeCleanClient{}, "NethServer/ns8-mail", "1.2.3", "1.2.4", nil)
	if deletedCount != 0 {
		t.Fatalf("deletePreReleases() = %d, want %d", deletedCount, 0)
	}

	want := "No pre-releases found between 1.2.3 and 1.2.4\n"
	if out.String() != want {
		t.Fatalf("deletePreReleases() output = %q, want %q", out.String(), want)
	}
}
