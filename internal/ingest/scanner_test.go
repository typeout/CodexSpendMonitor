package ingest

import (
	"strings"
	"testing"
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
