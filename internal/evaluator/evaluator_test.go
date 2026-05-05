package evaluator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnaylor/vibecop/internal/config"
)

func TestExtractVerdict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "plain json",
			input:   `{ "verdict": "approve", "reason": "ok" }`,
			want:    "approve",
			wantErr: false,
		},
		{
			name:    "markdown fences",
			input:   "```json\n{ \"verdict\": \"deny\", \"reason\": \"bad\" }\n```",
			want:    "deny",
			wantErr: false,
		},
		{
			name:    "bare fences",
			input:   "```\n{ \"verdict\": \"escalate\", \"reason\": \"maybe\" }\n```",
			want:    "escalate",
			wantErr: false,
		},
		{
			name:    "surrounding whitespace",
			input:   "\n  \n{ \"verdict\": \"approve\" }\n  \n",
			want:    "approve",
			wantErr: false,
		},
		{
			name:    "invalid verdict",
			input:   `{ "verdict": "maybe", "reason": "invalid" }`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `{ bad json }`,
			wantErr: true,
		},
		{
			name:    "empty",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractVerdict([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tt.want {
				t.Errorf("expected verdict %q, got %q", tt.want, got.Verdict)
			}
		})
	}
}

func TestParseOpenAIResponse(t *testing.T) {
	body := []byte(`{
		"choices": [{
			"message": {
				"content": "{ \"verdict\": \"approve\", \"reason\": \"looks good\" }"
			}
		}]
	}`)
	v, err := parseOpenAIResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "approve" {
		t.Errorf("expected approve, got %s", v.Verdict)
	}
	if v.Reason != "looks good" {
		t.Errorf("expected 'looks good', got %s", v.Reason)
	}
}

func TestParseOpenAIResponseNoChoices(t *testing.T) {
	_, err := parseOpenAIResponse([]byte(`{"choices": []}`))
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestParseAnthropicResponse(t *testing.T) {
	body := []byte(`{
		"content": [{
			"text": "{ \"verdict\": \"deny\", \"reason\": \"suspicious\" }"
		}]
	}`)
	v, err := parseAnthropicResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "deny" {
		t.Errorf("expected deny, got %s", v.Verdict)
	}
	if v.Reason != "suspicious" {
		t.Errorf("expected 'suspicious', got %s", v.Reason)
	}
}

func TestParseAnthropicResponseNoContent(t *testing.T) {
	_, err := parseAnthropicResponse([]byte(`{"content": []}`))
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestBaselinePrompt(t *testing.T) {
	if BaselinePrompt == "" {
		t.Fatal("baseline prompt should not be empty")
	}
	if !contains(BaselinePrompt, "VibeCop") {
		t.Error("baseline prompt should mention VibeCop")
	}
	if !contains(BaselinePrompt, "approve") {
		t.Error("baseline prompt should mention approve")
	}
}

func TestClientNoEndpoint(t *testing.T) {
	c := New("", "", "openai", "test-model", 0)
	_, err := c.Evaluate(context.Background(), ToolRequest{}, "system prompt")
	if err == nil {
		t.Fatal("expected error with no endpoint")
	}
}

func TestResolvePromptBaselineFallback(t *testing.T) {
	// Non-existent hash should return baseline prompt.
	prompt, err := ResolvePrompt("nonexistent-hash")
	if err != nil {
		t.Fatal(err)
	}
	if prompt != BaselinePrompt {
		t.Error("expected baseline prompt for non-existent project")
	}
}

func TestResolvePromptGuardianMode(t *testing.T) {
	// Create a project dir with a system-prompt.md to simulate Guardian mode.
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// ResolvePrompt uses config.SystemPromptPath which needs VibecopDir.
	vd, err := config.VibecopDir()
	if err != nil {
		t.Fatal(err)
	}

	ph := config.ProjectHash("/test/project/guardian")
	pd := filepath.Join(vd, "projects", ph)
	if err := os.MkdirAll(pd, 0755); err != nil {
		t.Fatal(err)
	}

	guardianPrompt := "You are VibeCop, guardian of this specific project."
	if err := os.WriteFile(filepath.Join(pd, "system-prompt.md"), []byte(guardianPrompt), 0644); err != nil {
		t.Fatal(err)
	}

	prompt, err := ResolvePrompt(ph)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != guardianPrompt {
		t.Errorf("expected guardian prompt, got %q", prompt)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
