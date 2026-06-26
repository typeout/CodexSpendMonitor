package ingest

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codexspendmonitor/internal/store"
)

func TestParseSessionFile(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-06-23T16:24:08Z","type":"session_meta","payload":{"id":"session-1","timestamp":"2026-06-23T16:24:08Z","cwd":"C:\\Working\\CodexSpendMonitor","originator":"Codex Desktop","cli_version":"0.140.0-alpha.2","model_provider":"openai"}}`,
		`{"timestamp":"2026-06-23T16:24:09Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-23T16:24:09Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"command\":\"dir\"}","call_id":"call-local"}}`,
		`{"timestamp":"2026-06-23T16:24:09Z","type":"response_item","payload":{"type":"function_call","name":"web.run","arguments":"{\"search_query\":[{\"q\":\"OpenAI pricing\"}]}","call_id":"call-web"}}`,
		`{"timestamp":"2026-06-23T16:24:10Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":250,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1050}}}}`,
		`not-json`,
	}, "\n"))

	got, err := parseSessionFile(input, "session.jsonl")
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}
	if got.Session.ID != "session-1" {
		t.Fatalf("session id = %q, want session-1", got.Session.ID)
	}
	if got.Session.Title != "CodexSpendMonitor" {
		t.Fatalf("session title = %q, want CodexSpendMonitor", got.Session.Title)
	}
	if got.MalformedLines != 1 {
		t.Fatalf("malformed lines = %d, want 1", got.MalformedLines)
	}
	if len(got.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(got.Events))
	}
	event := got.Events[0]
	if event.SessionID != "session-1" || event.Model != "gpt-5.5" || event.InputTokens != 1000 || event.CachedInputTokens != 250 || event.OutputTokens != 50 {
		t.Fatalf("event = %+v", event)
	}
	if len(got.ToolEvents) != 1 {
		t.Fatalf("len(tool events) = %d, want 1", len(got.ToolEvents))
	}
	toolEvent := got.ToolEvents[0]
	if toolEvent.SessionID != "session-1" || toolEvent.ToolKey != "web_search" || toolEvent.ToolName != "web.run" || toolEvent.Quantity != 1 {
		t.Fatalf("tool event = %+v", toolEvent)
	}
}

func TestParseSessionFileSkipsEmptyAndDuplicateTokenCounts(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-06-24T13:05:32Z","type":"session_meta","payload":{"id":"session-1","timestamp":"2026-06-24T13:05:12Z","cwd":"C:\\Working\\CodexSpendMonitor","originator":"Codex Desktop","cli_version":"0.140.0-alpha.2","model_provider":"openai"}}`,
		`{"timestamp":"2026-06-24T13:05:33Z","type":"turn_context","payload":{"model":"gpt-5.4"}}`,
		`{"timestamp":"2026-06-24T13:05:34Z","type":"event_msg","payload":{"type":"token_count","info":null}}`,
		`{"timestamp":"2026-06-24T13:05:41Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":17647,"cached_input_tokens":2432,"output_tokens":339,"reasoning_output_tokens":87,"total_tokens":17986},"last_token_usage":{"input_tokens":17647,"cached_input_tokens":2432,"output_tokens":339,"reasoning_output_tokens":87,"total_tokens":17986}}}}`,
		`{"timestamp":"2026-06-24T13:05:44Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":17647,"cached_input_tokens":2432,"output_tokens":339,"reasoning_output_tokens":87,"total_tokens":17986},"last_token_usage":{"input_tokens":17647,"cached_input_tokens":2432,"output_tokens":339,"reasoning_output_tokens":87,"total_tokens":17986}}}}`,
		`{"timestamp":"2026-06-24T13:05:52Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":35749,"cached_input_tokens":4864,"output_tokens":727,"reasoning_output_tokens":101,"total_tokens":36476},"last_token_usage":{"input_tokens":18102,"cached_input_tokens":2432,"output_tokens":388,"reasoning_output_tokens":14,"total_tokens":18490}}}}`,
	}, "\n"))

	got, err := parseSessionFile(input, "session.jsonl")
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}
	if len(got.Events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(got.Events))
	}

	first := got.Events[0]
	if first.Timestamp.Format(time.RFC3339) != "2026-06-24T13:05:41Z" {
		t.Fatalf("first timestamp = %s, want 2026-06-24T13:05:41Z", first.Timestamp.Format(time.RFC3339))
	}

	second := got.Events[1]
	if second.InputTokens != 18102 || second.CachedInputTokens != 2432 || second.OutputTokens != 388 {
		t.Fatalf("second event = %+v", second)
	}
}

func TestParseSessionFileFallsBackToLastTokenUsageWithoutCumulativeTotal(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Join([]string{
		`{"timestamp":"2026-06-23T16:24:08Z","type":"session_meta","payload":{"id":"session-1","timestamp":"2026-06-23T16:24:08Z","cwd":"C:\\Working\\CodexSpendMonitor","originator":"Codex Desktop","cli_version":"0.140.0-alpha.2","model_provider":"openai"}}`,
		`{"timestamp":"2026-06-23T16:24:09Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-23T16:24:10Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":250,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1050}}}}`,
	}, "\n"))

	got, err := parseSessionFile(input, "session.jsonl")
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(got.Events))
	}
	event := got.Events[0]
	if event.InputTokens != 1000 || event.CachedInputTokens != 250 || event.OutputTokens != 50 || event.TotalTokens != 1050 {
		t.Fatalf("event = %+v", event)
	}
}

func TestScanSkipsActiveFileWithoutSessionMeta(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	codexPath := t.TempDir()
	sessionsDir := filepath.Join(codexPath, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "active.jsonl"), []byte(`{"timestamp":"2026-06-23T16:24:08Z"`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scanner := NewScanner(db, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	result, err := scanner.Scan(ctx, codexPath)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if result.Files != 0 || result.Sessions != 0 || result.Events != 0 {
		t.Fatalf("Scan() = %+v, want no imported files", result)
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestSameFileSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(before) error = %v", err)
	}
	if err := os.WriteFile(path, []byte("one\ntwo"), 0o644); err != nil {
		t.Fatalf("WriteFile(update) error = %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(after) error = %v", err)
	}

	if sameFileSnapshot(before, after) {
		t.Fatalf("sameFileSnapshot() = true, want false")
	}
}
