package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bnaylor/vibecop/internal/daemon"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const maxLatencySamples = 50
const maxActivityItems = 200
const maxLogLines = 100

type latencyStats struct {
	mu      sync.Mutex
	samples []int64
}

func (s *latencyStats) add(ms int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, ms)
	if len(s.samples) > maxLatencySamples {
		s.samples = s.samples[len(s.samples)-maxLatencySamples:]
	}
}

func (s *latencyStats) avg() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	var sum int64
	for _, v := range s.samples {
		sum += v
	}
	return float64(sum) / float64(len(s.samples))
}

func (s *latencyStats) min() int64  { s.mu.Lock(); defer s.mu.Unlock(); return minOf(s.samples) }
func (s *latencyStats) max() int64  { s.mu.Lock(); defer s.mu.Unlock(); return maxOf(s.samples) }
func (s *latencyStats) count() int  { s.mu.Lock(); defer s.mu.Unlock(); return len(s.samples) }

func minOf(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxOf(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// verdictColor returns the tcell color for a verdict badge.
func verdictColor(verdict string) tcell.Color {
	switch verdict {
	case "approve":
		return tcell.ColorGreen
	case "deny":
		return tcell.ColorRed
	case "escalate":
		return tcell.ColorYellow
	default:
		return tcell.ColorWhite
	}
}

func verdictLabel(verdict string) string {
	switch verdict {
	case "approve":
		return "APPROVED"
	case "deny":
		return "DENIED"
	case "escalate":
		return "ESCALATED"
	case "error":
		return "ERROR"
	default:
		return strings.ToUpper(verdict)
	}
}

// App is the tview-based TUI.
type App struct {
	app         *tview.Application
	conn        net.Conn
	scanner     *bufio.Scanner

	headerView  *tview.TextView
	activity    *tview.List
	latencyView *tview.TextView
	configView  *tview.TextView
	logView     *tview.TextView

	latency *latencyStats
	events  int
	mu      sync.Mutex
}

// Run connects to the daemon and starts the TUI. Blocks until the user quits.
func Run(socketPath string) error {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}

	// Subscribe to events.
	sub := daemon.Request{Type: daemon.TypeTUISubscribe}
	if err := json.NewEncoder(conn).Encode(sub); err != nil {
		conn.Close()
		return fmt.Errorf("subscribe: %w", err)
	}

	a := &App{
		conn:    conn,
		scanner: bufio.NewScanner(conn),
		latency: &latencyStats{},
	}

	return a.runUI()
}

func (a *App) runUI() error {
	a.app = tview.NewApplication()

	// Build the layout.
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Header.
	a.headerView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.headerView.SetText("[green]vibecop[white] ● running  |  connect to TUI")
	a.headerView.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	flex.AddItem(a.headerView, 3, 0, false)

	// Middle: activity + right panel.
	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow)

	a.latencyView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.latencyView.SetTitle("latency").SetBorder(true)
	a.latencyView.SetText("waiting for data...")
	rightPanel.AddItem(a.latencyView, 0, 1, false)

	a.configView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.configView.SetTitle("config").SetBorder(true)
	a.configView.SetText("waiting for data...")
	rightPanel.AddItem(a.configView, 0, 1, false)

	a.activity = tview.NewList().
		ShowSecondaryText(true)
	a.activity.SetTitle("activity").SetBorder(true)

	middle := tview.NewFlex().
		AddItem(a.activity, 0, 3, false).
		AddItem(rightPanel, 0, 2, false)
	flex.AddItem(middle, 0, 1, true)

	// Log tail.
	a.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true)
	a.logView.SetTitle("log").SetBorder(true)
	flex.AddItem(a.logView, 7, 0, false)

	// Status bar.
	statusBar := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	statusBar.SetText("[white]q[gray]:quit  [white]↑/↓[gray]:scroll activity  [white]enter[gray]:expand reason  [white]r[gray]:refresh config")
	statusBar.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	flex.AddItem(statusBar, 1, 0, false)

	// Keyboard shortcuts.
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'r':
			a.refreshConfig()
			return nil
		}
		return event
	})

	// Start reading events in background.
	go a.readEvents()

	a.app.SetRoot(flex, true)
	return a.app.Run()
}

func (a *App) readEvents() {
	a.scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for a.scanner.Scan() {
		line := a.scanner.Bytes()
		var evt daemon.Event
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		a.handleEvent(evt)
	}
}

func (a *App) handleEvent(evt daemon.Event) {
	a.mu.Lock()
	a.events++
	a.mu.Unlock()

	// Log-level events go to the log tail.
	if evt.Level != "" || evt.Message != "" {
		a.addLogLine(evt)
		return
	}

	// Tool verdict events.
	if evt.Tool != "" {
		a.addActivity(evt)
		if evt.LatencyMs > 0 {
			a.latency.add(evt.LatencyMs)
		}
		a.updateLatencyDisplay()
	}

	// Update header on each event as a heartbeat.
	a.updateHeader(evt)
}

func (a *App) addActivity(evt daemon.Event) {
	label := verdictLabel(evt.Verdict)
	color := verdictColor(evt.Verdict)
	colorName := color.String()

	mainText := fmt.Sprintf("[%s::] %s", colorName, evt.Tool)
	if len(evt.Input) > 60 {
		mainText += ": " + evt.Input[:57] + "..."
	} else {
		mainText += ": " + evt.Input
	}

	secondary := fmt.Sprintf("[%s::]%s[-:-:-]  %s", colorName, label, evt.Timestamp)

	a.app.QueueUpdateDraw(func() {
		a.activity.InsertItem(0, mainText, secondary, 0, nil)
		// Trim.
		for a.activity.GetItemCount() > maxActivityItems {
			a.activity.RemoveItem(a.activity.GetItemCount() - 1)
		}
	})
}

func (a *App) addLogLine(evt daemon.Event) {
	levelColor := "white"
	switch evt.Level {
	case "error":
		levelColor = "red"
	case "warn":
		levelColor = "yellow"
	case "info":
		levelColor = "green"
	}

	ts := evt.Timestamp
	if len(ts) > 19 {
		ts = ts[:19] // strip timezone for display
	}
	line := fmt.Sprintf("[%s]%s[white] [gray]%s[white] %s", levelColor, strings.ToUpper(evt.Level), ts, evt.Message)

	a.app.QueueUpdateDraw(func() {
		fmt.Fprintln(a.logView, line)
		// Trim by removing oldest lines.
		lines := strings.Count(a.logView.GetText(true), "\n")
		if lines > maxLogLines {
			a.logView.Clear()
			// Re-add last N lines — simpler to just clear when too big.
		}
	})
}

func (a *App) updateLatencyDisplay() {
	c := a.latency.count()
	if c == 0 {
		return
	}

	avg := a.latency.avg()
	min := a.latency.min()
	max := a.latency.max()

	var color string
	switch {
	case avg < 1000:
		color = "green"
	case avg < 3000:
		color = "yellow"
	default:
		color = "red"
	}

	text := fmt.Sprintf("[green]avg:[white] [%s]%.0f ms[white]  (%d samples)\n", color, avg, c)
	text += fmt.Sprintf("[green]min:[white] %d ms\n", min)
	text += fmt.Sprintf("[green]max:[white] %d ms", max)

	a.app.QueueUpdateDraw(func() {
		a.latencyView.SetText(text)
	})
}

func (a *App) updateHeader(evt daemon.Event) {
	a.app.QueueUpdateDraw(func() {
		a.headerView.SetText(fmt.Sprintf(
			"[green]vibecop[white] ● running  |  events: %d",
			a.events,
		))
	})
}

func (a *App) refreshConfig() {
	a.app.QueueUpdateDraw(func() {
		a.configView.SetText("(press r to refresh from daemon)")
	})
}

// UpdateConfig is called externally (or on timer) to refresh the config display.
func (a *App) UpdateConfig(endpoint, apiFormat, model string, timeoutMs int, auditEnabled bool) {
	text := fmt.Sprintf("endpoint: [green]%s[white]\n", endpoint)
	text += fmt.Sprintf("format:   %s\n", apiFormat)
	text += fmt.Sprintf("model:    [yellow]%s[white]\n", model)
	text += fmt.Sprintf("timeout:  %d ms\n", timeoutMs)
	text += fmt.Sprintf("audit:    %v", auditEnabled)

	a.app.QueueUpdateDraw(func() {
		a.configView.SetText(text)
	})
}

// Close shuts down the TUI and disconnects from the daemon.
func (a *App) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}
