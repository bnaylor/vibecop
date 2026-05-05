package cmd

import (
	"fmt"
	"os"

	"github.com/bnaylor/vibecop/internal/hooks"
	"github.com/spf13/cobra"
)

var uninstallHarness string

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed hook scripts",
	Long:  "Remove vibecop hook entries from the specified harness settings file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if uninstallHarness == "" {
			return fmt.Errorf("specify --harness (claude|gemini)")
		}

		if err := hooks.UninstallHooks(uninstallHarness); err != nil {
			return fmt.Errorf("uninstall %s: %w", uninstallHarness, err)
		}
		fmt.Fprintf(os.Stderr, "vibecop: removed hook for %s\n", uninstallHarness)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().StringVar(&uninstallHarness, "harness", "", "Harness to remove from (claude|gemini)")
	uninstallCmd.MarkFlagRequired("harness")
}
