package cmd

import (
	"github.com/spf13/cobra"
)

var (
	verbose bool
	noCache bool
	jsonOut bool
)

var rootCmd = &cobra.Command{
	Use:   "wt-cycle",
	Short: "Git worktree lifecycle manager",
	Long:  "Create, recycle, and clean numbered wt-N worktrees.",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&noCache, "no-cache", false, "bypass GitHub API cache")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "JSON output")
}

func SetVersion(v string) {
	rootCmd.Version = v
}

func Execute() error {
	return rootCmd.Execute()
}
