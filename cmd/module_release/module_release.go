package module_release

import (
	"github.com/NethServer/gh-ns8/cmd"
	"github.com/spf13/cobra"
)

var (
	// Shared flags
	repoFlag       string
	issuesRepoFlag string
)

// moduleReleaseCmd represents the module-release command
var moduleReleaseCmd = &cobra.Command{
	Use:   "module-release",
	Short: "Manage releases for NethServer 8 modules",
	Long:  `Automate release creation, checking, commenting, and cleanup for NethServer 8 modules.`,
}

func init() {
	cmd.AddModuleReleaseCommand(moduleReleaseCmd)

	// Persistent flags for all subcommands
	moduleReleaseCmd.PersistentFlags().StringVar(&repoFlag, "repo", "", "The GitHub NethServer 8 module repository (e.g., owner/ns8-module)")
	moduleReleaseCmd.PersistentFlags().StringVar(&issuesRepoFlag, "issues-repo", "NethServer/dev", "Issues repository (default: NethServer/dev)")

	// Add subcommands
	moduleReleaseCmd.AddCommand(createCmd)
	moduleReleaseCmd.AddCommand(checkCmd)
	moduleReleaseCmd.AddCommand(commentCmd)
	moduleReleaseCmd.AddCommand(cleanCmd)
}
