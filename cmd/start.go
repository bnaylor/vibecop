package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bnaylor/vibecop/internal/config"
	"github.com/bnaylor/vibecop/internal/daemon"
	"github.com/bnaylor/vibecop/internal/evaluator"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background daemon",
	Long:  "Start the vibecop daemon. Runs in the foreground; send SIGTERM or use 'vibecop stop' to shut down.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := VibeCopConfig()

		vibecopDir, err := config.VibecopDir()
		if err != nil {
			return err
		}
		socketPath := daemon.DefaultSocketPath(vibecopDir)

		d := daemon.New(socketPath, cfg)

		// Create the LLM evaluator client.
		evalClient := evaluator.New(
			cfg.Model.Endpoint,
			cfg.Model.APIKey,
			cfg.Model.APIFormat,
			cfg.Model.Model,
			time.Duration(cfg.Daemon.TimeoutMs)*time.Millisecond,
		)

		d.OnPermission(makePermissionHandler(evalClient, cfg.Daemon.ActivityWindow))

		if err := d.Start(); err != nil {
			return fmt.Errorf("daemon start: %w", err)
		}

		fmt.Fprintf(os.Stderr, "vibecop: daemon started (pid %d)\n", os.Getpid())
		return d.Run()
	},
}

func makePermissionHandler(evalClient *evaluator.Client, activityWindow int) func(daemon.Request) daemon.Verdict {
	return func(req daemon.Request) daemon.Verdict {
		projectHash := config.ProjectHash(req.ProjectPath)

		systemPrompt, err := evaluator.ResolvePrompt(projectHash)
		if err != nil {
			log.Printf("evaluator: prompt resolution error: %v", err)
			return daemon.Verdict{
				Verdict: "escalate",
				Reason:  "VibeCop: failed to load configuration",
			}
		}

		// Build the tool request (activity window will be wired in step 8).
		toolReq := evaluator.ToolRequest{
			Tool:  req.Tool,
			Input: req.Input,
		}

		ctx, cancel := context.WithTimeout(context.Background(), evalClient.Timeout())
		defer cancel()

		v, err := evalClient.Evaluate(ctx, toolReq, systemPrompt)
		if err != nil {
			log.Printf("evaluator: %v", err)
			return daemon.Verdict{
				Verdict: "escalate",
				Reason:  "VibeCop: evaluation error — escalated",
			}
		}

		return daemon.Verdict{
			Verdict: v.Verdict,
			Reason:  v.Reason,
		}
	}
}

func init() {
	rootCmd.AddCommand(startCmd)
}
