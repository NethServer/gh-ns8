package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	debugMode bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ns8",
	Short: "NethServer 8 CLI extension",
	Long:  `A GitHub CLI extension for NethServer 8 module management and automation.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debugMode {
			os.Setenv("DEBUG", "1")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")
}

// AddModuleReleaseCommand adds the module-release command to root
func AddModuleReleaseCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}
