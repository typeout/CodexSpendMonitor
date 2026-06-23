package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codexspendmonitor/internal/store"
)

type Scanner struct {
	store  *store.Store
	logger *slog.Logger
}

type ScanResult struct {
	Files          int
	Sessions       int
	Events         int
	MalformedLines int
}

type envelope struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMetaPayload struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	CWD           string    `json:"cwd"`
	Originator    string    `json:"originator"`
	CLIVersion    string    `json:"cli_version"`
	ModelProvider string    `json:"model_provider"`
}

type turnContextPayload struct {
	Model string `json:"model"`
}

type eventMessagePayload struct {
	Type string         `json:"type"`
	Info tokenCountInfo `json:"info"`
}

type tokenCountInfo struct {
	LastTokenUsage tokenUsage `json:"last_token_usage"`
}

type tokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

type responseItemPayload struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Input     json.RawMessage `json:"input"`
}

const longContextInputThreshold = 128_000

func NewScanner(store *store.Store, logger *slog.Logger) *Scanner {
	return &Scanner{store: store, logger: logger}
}

func (s *Scanner) Scan(ctx context.Context, codexPath string) (ScanResult, error) {
	var result ScanResult
	files, err := sessionFiles(codexPath)
	if err != nil {
		return result, err
	}

	for _, path := range files {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		fileResult, err := s.importFile(ctx, path)
		if err != nil {
			s.logger.Warn("import session file", "path", path, "error", err)
			continue
		}
		result.Files++
		result.Sessions += fileResult.Sessions
		result.Events += fileResult.Events
		result.MalformedLines += fileResult.MalformedLines
	}
	return result, nil
}

func sessionFiles(codexPath string) ([]string, error) {
	var roots []string
	for _, dir := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(codexPath, dir)
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no sessions or archived_sessions directory found under %s", codexPath)
	}

	var files []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return files, nil
}

func (s *Scanner) importFile(ctx context.Context, path string) (ScanResult, error) {
	var result ScanResult

	info, err := os.Stat(path)
	if err != nil {
		return result, fmt.Errorf("stat file: %w", err)
	}
	unchanged, err := s.store.ImportedFileUnchanged(ctx, path, info.Size(), info.ModTime())
	if err != nil {
		return result, err
	}
	if unchanged {
		return result, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	reader := io.TeeReader(file, hash)
	parsed, err := parseSessionFile(reader, path)
	if err != nil {
		return result, err
	}

	if parsed.Session.ID == "" {
		return result, fmt.Errorf("missing session_meta id")
	}
	if err := s.store.UpsertSession(ctx, parsed.Session); err != nil {
		return result, err
	}
	result.Sessions = 1

	for _, event := range parsed.Events {
		if err := s.store.UpsertUsageEvent(ctx, event); err != nil {
			return result, err
		}
		result.Events++
	}
	for _, event := range parsed.ToolEvents {
		if err := s.store.UpsertToolUsageEvent(ctx, event); err != nil {
			return result, err
		}
		result.Events++
	}

	if err := s.store.RecordImportedFile(ctx, path, info.Size(), info.ModTime(), hex.EncodeToString(hash.Sum(nil))); err != nil {
		return result, err
	}
	result.MalformedLines = parsed.MalformedLines
	return result, nil
}

type parsedSession struct {
	Session        store.Session
	Events         []store.UsageEvent
	ToolEvents     []store.ToolUsageEvent
	MalformedLines int
}

func parseSessionFile(reader io.Reader, path string) (parsedSession, error) {
	var parsed parsedSession
	var currentModel string
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	lineIndex := 0
	for scanner.Scan() {
		lineIndex++
		line := scanner.Bytes()
		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			parsed.MalformedLines++
			continue
		}

		switch env.Type {
		case "session_meta":
			var meta sessionMetaPayload
			if err := json.Unmarshal(env.Payload, &meta); err != nil {
				parsed.MalformedLines++
				continue
			}
			parsed.Session = store.Session{
				ID:            meta.ID,
				SourcePath:    path,
				StartedAt:     firstTime(meta.Timestamp, env.Timestamp),
				CWD:           meta.CWD,
				Originator:    meta.Originator,
				CLIVersion:    meta.CLIVersion,
				ModelProvider: meta.ModelProvider,
				Title:         titleFromCWD(meta.CWD),
			}
		case "turn_context":
			var turn turnContextPayload
			if err := json.Unmarshal(env.Payload, &turn); err == nil && turn.Model != "" {
				currentModel = turn.Model
			}
		case "event_msg":
			var message eventMessagePayload
			if err := json.Unmarshal(env.Payload, &message); err != nil {
				parsed.MalformedLines++
				continue
			}
			if message.Type != "token_count" {
				continue
			}
			usage := message.Info.LastTokenUsage
			if currentModel == "" {
				currentModel = "unknown"
			}
			parsed.Events = append(parsed.Events, store.UsageEvent{
				SessionID:             parsed.Session.ID,
				EventIndex:            lineIndex,
				Timestamp:             env.Timestamp,
				Model:                 currentModel,
				BillingTier:           billingTierFromLine(line),
				ContextKind:           contextKindFromUsage(usage),
				InputTokens:           usage.InputTokens,
				CachedInputTokens:     usage.CachedInputTokens,
				OutputTokens:          usage.OutputTokens,
				ReasoningOutputTokens: usage.ReasoningOutputTokens,
				TotalTokens:           usage.TotalTokens,
				RawJSON:               string(line),
			})
		case "response_item":
			var item responseItemPayload
			if err := json.Unmarshal(env.Payload, &item); err != nil {
				parsed.MalformedLines++
				continue
			}
			toolKey, toolName, quantity := classifyToolCall(item)
			if toolKey == "" {
				continue
			}
			parsed.ToolEvents = append(parsed.ToolEvents, store.ToolUsageEvent{
				SessionID:  parsed.Session.ID,
				EventIndex: lineIndex,
				Timestamp:  env.Timestamp,
				ToolKey:    toolKey,
				ToolName:   toolName,
				Quantity:   quantity,
				RawJSON:    string(line),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return parsed, fmt.Errorf("reading jsonl: %w", err)
	}

	for i := range parsed.Events {
		parsed.Events[i].SessionID = parsed.Session.ID
	}
	for i := range parsed.ToolEvents {
		parsed.ToolEvents[i].SessionID = parsed.Session.ID
	}
	return parsed, nil
}

func classifyToolCall(item responseItemPayload) (string, string, float64) {
	if item.Type != "function_call" && item.Type != "tool_call" {
		return "", "", 0
	}
	name := strings.ToLower(strings.TrimSpace(item.Name))
	if name == "" {
		return "", "", 0
	}
	switch name {
	case "web_search", "web_search_preview":
		return "web_search", item.Name, 1
	case "image_web_search":
		return "image_web_search", item.Name, 1
	case "file_search":
		return "file_search_tool_call", item.Name, 1
	case "code_interpreter", "container", "hosted_shell", "hosted_shell_code_interpreter":
		return "container_session", item.Name, 1
	}
	if name == "web.run" || name == "web_run" {
		args := strings.ToLower(string(item.Arguments) + " " + string(item.Input))
		switch {
		case strings.Contains(args, "image_query"):
			return "image_web_search", item.Name, 1
		case strings.Contains(args, "search_query"):
			return "web_search", item.Name, 1
		}
	}
	return "", "", 0
}

func billingTierFromLine(line []byte) string {
	lower := strings.ToLower(string(line))
	if strings.Contains(lower, `"batch"`) || strings.Contains(lower, `"billing_tier":"batch"`) || strings.Contains(lower, `"service_tier":"batch"`) {
		return "batch"
	}
	return "standard"
}

func contextKindFromUsage(usage tokenUsage) string {
	if usage.InputTokens >= longContextInputThreshold {
		return "long"
	}
	return "short"
}

func firstTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

func titleFromCWD(cwd string) string {
	if cwd == "" {
		return "Untitled session"
	}
	base := filepath.Base(cwd)
	if base == "." || base == string(filepath.Separator) {
		return cwd
	}
	return base
}
