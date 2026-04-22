package module_release

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestModuleReleaseCommandConfiguration(t *testing.T) {
	if moduleReleaseCmd.Use != "module-release" {
		t.Fatalf("moduleReleaseCmd.Use = %q, want %q", moduleReleaseCmd.Use, "module-release")
	}

	repoFlag := moduleReleaseCmd.PersistentFlags().Lookup("repo")
	if repoFlag == nil {
		t.Fatal("moduleReleaseCmd repo flag is not registered")
	}
	if repoFlag.DefValue != "" {
		t.Fatalf("repo flag default = %q, want empty string", repoFlag.DefValue)
	}

	issuesRepoFlag := moduleReleaseCmd.PersistentFlags().Lookup("issues-repo")
	if issuesRepoFlag == nil {
		t.Fatal("moduleReleaseCmd issues-repo flag is not registered")
	}
	if issuesRepoFlag.DefValue != "NethServer/dev" {
		t.Fatalf("issues-repo flag default = %q, want %q", issuesRepoFlag.DefValue, "NethServer/dev")
	}

	testCases := map[string]*cobra.Command{
		"create":  createCmd,
		"check":   checkCmd,
		"comment": commentCmd,
		"clean":   cleanCmd,
	}
	for name, want := range testCases {
		got, _, err := moduleReleaseCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("moduleReleaseCmd.Find(%q) returned error: %v", name, err)
		}
		if got != want {
			t.Fatalf("moduleReleaseCmd.Find(%q) = %p, want %p", name, got, want)
		}
	}
}

func TestModuleReleaseRepoCompletion(t *testing.T) {
	completion, ok := moduleReleaseCmd.GetFlagCompletionFunc("repo")
	if !ok {
		t.Fatal("moduleReleaseCmd repo completion is not registered")
	}

	suggestions, directive := completion(moduleReleaseCmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("repo completion directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if len(suggestions) != 1 || suggestions[0] != "owner/ns8-module" {
		t.Fatalf("repo completion suggestions = %v, want [owner/ns8-module]", suggestions)
	}
}
