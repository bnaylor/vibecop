package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/bnaylor/vibecop/internal/audit"
	"github.com/bnaylor/vibecop/internal/config"
	"github.com/bnaylor/vibecop/internal/daemon"
	"github.com/bnaylor/vibecop/internal/evaluator"
	"github.com/spf13/cobra"
)

// evalClient is the interface the permission handler needs from the evaluator.
type evalClient interface {
	Evaluate(ctx context.Context, req evaluator.ToolRequest, systemPrompt string) (evaluator.Verdict, error)
	Timeout() time.Duration
}

const maxConsecutiveFailures = 3

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
		ec := evaluator.New(
			cfg.Model.Endpoint,
			cfg.Model.APIKey,
			cfg.Model.APIFormat,
			cfg.Model.Model,
			time.Duration(cfg.Daemon.TimeoutMs)*time.Millisecond,
		)

		// Per-project activity stores and audit loggers.
		stores := make(map[string]*audit.ActivityStore)
		loggers := make(map[string]*audit.Logger)
		var storesMu sync.Mutex

		d.OnPermission(makePermissionHandler(ec, d, cfg.Daemon.ActivityWindow, cfg.Daemon.AuditEnabled, stores, loggers, &storesMu))

		if err := d.Start(); err != nil {
			return fmt.Errorf("daemon start: %w", err)
		}

		fmt.Fprintf(os.Stderr, "vibecop: daemon started (pid %d)\n", os.Getpid())
		return d.Run()
	},
}

func makePermissionHandler(
	ec evalClient,
	d *daemon.Daemon,
	activityWindow int,
	auditEnabled bool,
	stores map[string]*audit.ActivityStore,
	loggers map[string]*audit.Logger,
	storesMu *sync.Mutex,
) func(daemon.Request) daemon.Verdict {
	var (
		failMu              sync.Mutex
		consecutiveFailures int
		suspended           bool
	)

	return func(req daemon.Request) daemon.Verdict {
		// Fail-open if the evaluator has had too many consecutive errors.
		failMu.Lock()
		isSuspended := suspended
		failMu.Unlock()

		if isSuspended {
			d.EmitEvent(daemon.Event{
				Tool:      req.Tool,
				Input:     req.Input,
				Verdict:   "approve",
				Reason:    "VibeCop suspended (pass-through)",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Level:     "warn",
				Message:   "VibeCop suspended after repeated failures — pass-through mode",
			})
			return daemon.Verdict{Verdict: "approve"}
		}

		projectHash := config.ProjectHash(req.ProjectPath)

		// Get or create per-project activity store and audit logger.
		storesMu.Lock()
		store, ok := stores[projectHash]
		if !ok {
			store = audit.NewActivityStore(projectHash, activityWindow)
			store.Load() // best-effort
			stores[projectHash] = store
		}
		logger, ok := loggers[projectHash]
		if !ok {
			logger = audit.NewLogger(projectHash, auditEnabled)
			loggers[projectHash] = logger
		}
		storesMu.Unlock()

		systemPrompt, err := evaluator.ResolvePrompt(projectHash)
		if err != nil {
			log.Printf("evaluator: prompt resolution error: %v", err)
			return daemon.Verdict{
				Verdict: "escalate",
				Reason:  "VibeCop: failed to load configuration",
			}
		}

		// Build tool request with recent activity context.
		recent := store.Recent()
		toolReq := evaluator.ToolRequest{
			Tool:           req.Tool,
			Input:          req.Input,
			RecentActivity: activityEntriesToVerdicts(recent),
		}

		ctx, cancel := context.WithTimeout(context.Background(), ec.Timeout())
		defer cancel()

		startTime := time.Now()
		v, evalErr := ec.Evaluate(ctx, toolReq, systemPrompt)
		latencyMs := time.Since(startTime).Milliseconds()

		verdictStr := v.Verdict
		reasonStr := v.Reason
		now := time.Now().UTC()

		if evalErr != nil {
			log.Printf("evaluator: %v", evalErr)
			verdictStr = "error"
			reasonStr = fmt.Sprintf("VibeCop: evaluation error — escalated (%v)", evalErr)

			failMu.Lock()
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveFailures {
				suspended = true
				log.Printf("evaluator: %d consecutive failures — entering pass-through mode", consecutiveFailures)
				d.EmitEvent(daemon.Event{
					Level:   "error",
					Message: fmt.Sprintf("VibeCop suspended after %d consecutive failures — run 'vibecop test' to resume", consecutiveFailures),
				})
			}
			failMu.Unlock()
		} else {
			failMu.Lock()
			consecutiveFailures = 0
			failMu.Unlock()
		}

		// Record in activity log.
		store.Append(req.Tool, req.Input, verdictStr)
		if err := store.Save(); err != nil {
			log.Printf("activity: save error: %v", err)
		}

		// Write audit record.
		lat := latencyMs
		rec := audit.AuditRecord{
			Timestamp:     now.Format(time.RFC3339),
			ToolName:      req.Tool,
			ToolInput:     req.Input,
			Verdict:       verdictStr,
			Reason:        reasonStr,
			HumanDecision: nil,
			LatencyMs:     &lat,
		}

		if verdictStr == "escalate" || verdictStr == "error" {
			if _, err := logger.WritePending(rec); err != nil {
				log.Printf("audit: write pending error: %v", err)
			}
		} else {
			if err := logger.Write(rec); err != nil {
				log.Printf("audit: write error: %v", err)
			}
		}

		// Emit event for TUI subscribers.
		d.EmitEvent(daemon.Event{
			Tool:      req.Tool,
			Input:     req.Input,
			Verdict:   verdictStr,
			Reason:    reasonStr,
			LatencyMs: latencyMs,
			Timestamp: now.Format(time.RFC3339),
		})

		return daemon.Verdict{
			Verdict: verdictStr,
			Reason:  reasonStr,
		}
	}
}

func activityEntriesToVerdicts(entries []audit.ActivityEntry) []evaluator.VerdictEntry {
	out := make([]evaluator.VerdictEntry, len(entries))
	for i, e := range entries {
		out[i] = evaluator.VerdictEntry{Tool: e.Tool, Verdict: e.Verdict}
	}
	return out
}

func init() {
	rootCmd.AddCommand(startCmd)
}
