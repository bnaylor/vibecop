package cmd

import (
	"fmt"
	"os"

	"github.com/bnaylor/vibecop/internal/hooks"
	"github.com/spf13/cobra"
)

var (
	installHarness string
	installAll     bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install hook scripts into coding harness configs",
	Long: `Install vibecop hook scripts into the specified harness.
Use --harness to target one (claude, gemini) or --all for all supported harnesses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targets := resolveInstallTargets()
		if len(targets) == 0 {
			return fmt.Errorf("specify --harness or --all")
		}

		for _, h := range targets {
			if err := hooks.InstallHooks(h); err != nil {
				fmt.Fprintf(os.Stderr, "vibecop: %s install failed: %v\n", h, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "vibecop: installed hook for %s\n", h)
		}
		return nil
	},
}

func resolveInstallTargets() []string {
	if installHarness != "" {
		return []string{installHarness}
	}
	if installAll {
		return []string{hooks.HarnessClaude, hooks.HarnessGemini}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringVar(&installHarness, "harness", "", "Harness to install into (claude|gemini)")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install into all supported harnesses")
}
