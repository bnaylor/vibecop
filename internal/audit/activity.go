package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bnaylor/vibecop/internal/config"
)

// ActivityEntry is a single verdict in the rolling activity log.
type ActivityEntry struct {
	Tool      string `json:"tool"`
	Input     string `json:"input,omitempty"`
	Verdict   string `json:"verdict"`
	Timestamp string `json:"timestamp"`
}

// ActivityStore maintains a rolling window of recent verdicts per project.
type ActivityStore struct {
	projectHash string
	window      int
	entries     []ActivityEntry
	mu          sync.Mutex
}

// NewActivityStore creates a store with the given window size.
// Call Load to hydrate from disk.
func NewActivityStore(projectHash string, window int) *ActivityStore {
	return &ActivityStore{
		projectHash: projectHash,
		window:      window,
	}
}

// Load reads existing activity from disk.
func (s *ActivityStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := config.ActivityLogPath(s.projectHash)
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = nil
			return nil
		}
		return fmt.Errorf("open activity log: %w", err)
	}
	defer f.Close()

	var entries []ActivityEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e ActivityEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip corrupt lines
		}
		entries = append(entries, e)
	}

	// Trim to window from the end.
	if len(entries) > s.window {
		entries = entries[len(entries)-s.window:]
	}
	s.entries = entries
	return scanner.Err()
}

// Append adds a verdict and trims to the window.
func (s *ActivityStore) Append(tool, input, verdict string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, ActivityEntry{
		Tool:      tool,
		Input:     input,
		Verdict:   verdict,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	if len(s.entries) > s.window {
		s.entries = s.entries[len(s.entries)-s.window:]
	}
}

// Recent returns the current window of entries (for inclusion in LLM requests).
func (s *ActivityStore) Recent() []ActivityEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ActivityEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Save persists the current window to disk.
func (s *ActivityStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := config.ActivityLogPath(s.projectHash)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create activity dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write activity log: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range s.entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encode activity: %w", err)
		}
	}
	return nil
}
