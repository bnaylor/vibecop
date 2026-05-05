package cmd

import (
	"fmt"

	"github.com/bnaylor/vibecop/internal/config"
	"github.com/bnaylor/vibecop/internal/daemon"
	"github.com/bnaylor/vibecop/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Attach the TUI to a running daemon",
	Long:  "Connect to the running vibecop daemon and display a terminal UI with activity feed, latency stats, and logs.",
	RunE: func(cmd *cobra.Command, args []string) error {
		vibecopDir, err := config.VibecopDir()
		if err != nil {
			return err
		}
		socketPath := daemon.DefaultSocketPath(vibecopDir)

		fmt.Fprintf(cmd.ErrOrStderr(), "vibecop: connecting to daemon at %s...\n", socketPath)
		return tui.Run(socketPath)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
