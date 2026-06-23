package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const SettingCodexPath = "codex_path"

type Store struct {
	db *sql.DB
}

type Session struct {
	ID            string
	SourcePath    string
	StartedAt     time.Time
	CWD           string
	Originator    string
	CLIVersion    string
	ModelProvider string
	Title         string
}

type UsageEvent struct {
	SessionID             string
	EventIndex            int
	Timestamp             time.Time
	Model                 string
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	RawJSON               string
}

type PricingSnapshot struct {
	SourceURL             string
	FetchedAt             time.Time
	Model                 string
	InputPerMillion       float64
	CachedInputPerMillion float64
	OutputPerMillion      float64
}

type SessionSummary struct {
	ID                    string
	Title                 string
	CWD                   string
	StartedAt             time.Time
	LastSeenAt            time.Time
	ModelProvider         string
	Models                string
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	TotalCost             float64
	UnpricedEvents        int
	EventCount            int
}

type EventDetail struct {
	ID                    int64
	SessionID             string
	EventIndex            int
	Timestamp             time.Time
	Model                 string
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	InputPerMillion       sql.NullFloat64
	CachedInputPerMillion sql.NullFloat64
	OutputPerMillion      sql.NullFloat64
	Cost                  float64
	Priced                bool
}

type DailySpend struct {
	Day            string
	TotalCost      float64
	UnpricedEvents int
}

func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS import_files (
			path TEXT PRIMARY KEY,
			size_bytes INTEGER NOT NULL,
			mtime TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			last_scanned_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			source_path TEXT NOT NULL,
			started_at TEXT NOT NULL,
			cwd TEXT NOT NULL,
			originator TEXT NOT NULL,
			cli_version TEXT NOT NULL,
			model_provider TEXT NOT NULL,
			title TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			event_index INTEGER NOT NULL,
			timestamp TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			raw_json TEXT NOT NULL,
			UNIQUE(session_id, event_index)
		)`,
		`CREATE TABLE IF NOT EXISTS pricing_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_url TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			model TEXT NOT NULL,
			input_per_million REAL NOT NULL,
			cached_input_per_million REAL NOT NULL,
			output_per_million REAL NOT NULL,
			UNIQUE(model, fetched_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_session ON usage_events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_timestamp ON usage_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_pricing_model_fetched ON pricing_snapshots(model, fetched_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("applying migration statement %q: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) Setting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying setting %s: %w", key, err)
	}
	return value, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upserting setting %s: %w", key, err)
	}
	return nil
}

func (s *Store) RecordImportedFile(ctx context.Context, path string, size int64, mtime time.Time, sha string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_files (path, size_bytes, mtime, sha256, last_scanned_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size_bytes = excluded.size_bytes,
			mtime = excluded.mtime,
			sha256 = excluded.sha256,
			last_scanned_at = excluded.last_scanned_at
	`, path, size, mtime.UTC().Format(time.RFC3339Nano), sha, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("recording imported file %s: %w", path, err)
	}
	return nil
}

func (s *Store) ImportedFileUnchanged(ctx context.Context, path string, size int64, mtime time.Time) (bool, error) {
	var existingSize int64
	var existingMtime string
	err := s.db.QueryRowContext(ctx, `
		SELECT size_bytes, mtime
		FROM import_files
		WHERE path = ?
	`, path).Scan(&existingSize, &existingMtime)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying imported file %s: %w", path, err)
	}
	return existingSize == size && existingMtime == mtime.UTC().Format(time.RFC3339Nano), nil
}

func (s *Store) UpsertSession(ctx context.Context, session Session) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, source_path, started_at, cwd, originator, cli_version, model_provider, title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_path = excluded.source_path,
			started_at = excluded.started_at,
			cwd = excluded.cwd,
			originator = excluded.originator,
			cli_version = excluded.cli_version,
			model_provider = excluded.model_provider,
			title = excluded.title
	`, session.ID, session.SourcePath, session.StartedAt.UTC().Format(time.RFC3339Nano), session.CWD, session.Originator, session.CLIVersion, session.ModelProvider, session.Title)
	if err != nil {
		return fmt.Errorf("upserting session %s: %w", session.ID, err)
	}
	return nil
}

func (s *Store) UpsertUsageEvent(ctx context.Context, event UsageEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_events (
			session_id, event_index, timestamp, model, input_tokens, cached_input_tokens,
			output_tokens, reasoning_output_tokens, total_tokens, raw_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, event_index) DO UPDATE SET
			timestamp = excluded.timestamp,
			model = excluded.model,
			input_tokens = excluded.input_tokens,
			cached_input_tokens = excluded.cached_input_tokens,
			output_tokens = excluded.output_tokens,
			reasoning_output_tokens = excluded.reasoning_output_tokens,
			total_tokens = excluded.total_tokens,
			raw_json = excluded.raw_json
	`, event.SessionID, event.EventIndex, event.Timestamp.UTC().Format(time.RFC3339Nano), event.Model, event.InputTokens, event.CachedInputTokens, event.OutputTokens, event.ReasoningOutputTokens, event.TotalTokens, event.RawJSON)
	if err != nil {
		return fmt.Errorf("upserting usage event %s/%d: %w", event.SessionID, event.EventIndex, err)
	}
	return nil
}

func (s *Store) SeedPricing(ctx context.Context, prices []PricingSnapshot) error {
	for _, price := range prices {
		if err := s.UpsertPricing(ctx, price); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertPricing(ctx context.Context, price PricingSnapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pricing_snapshots (
			source_url, fetched_at, model, input_per_million, cached_input_per_million, output_per_million
		)
		VALUES (?, ?, lower(?), ?, ?, ?)
		ON CONFLICT(model, fetched_at) DO UPDATE SET
			source_url = excluded.source_url,
			input_per_million = excluded.input_per_million,
			cached_input_per_million = excluded.cached_input_per_million,
			output_per_million = excluded.output_per_million
	`, price.SourceURL, price.FetchedAt.UTC().Format(time.RFC3339Nano), price.Model, price.InputPerMillion, price.CachedInputPerMillion, price.OutputPerMillion)
	if err != nil {
		return fmt.Errorf("upserting price for %s: %w", price.Model, err)
	}
	return nil
}

func (s *Store) Dashboard(ctx context.Context) ([]DailySpend, []SessionSummary, error) {
	events, err := s.allEventDetails(ctx, "")
	if err != nil {
		return nil, nil, err
	}

	summaryByID := map[string]*SessionSummary{}
	dailyByDay := map[string]*DailySpend{}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, cwd, started_at, model_provider
		FROM sessions
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var ordered []SessionSummary
	for rows.Next() {
		var started string
		var summary SessionSummary
		if err := rows.Scan(&summary.ID, &summary.Title, &summary.CWD, &started, &summary.ModelProvider); err != nil {
			return nil, nil, fmt.Errorf("scanning session summary: %w", err)
		}
		summary.StartedAt = parseStoredTime(started)
		summary.LastSeenAt = summary.StartedAt
		summaryByID[summary.ID] = &summary
		ordered = append(ordered, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating sessions: %w", err)
	}

	modelSets := map[string]map[string]bool{}
	for _, event := range events {
		summary := summaryByID[event.SessionID]
		if summary == nil {
			continue
		}
		summary.InputTokens += event.InputTokens
		summary.CachedInputTokens += event.CachedInputTokens
		summary.OutputTokens += event.OutputTokens
		summary.ReasoningOutputTokens += event.ReasoningOutputTokens
		summary.TotalTokens += event.TotalTokens
		summary.TotalCost += event.Cost
		summary.EventCount++
		if !event.Priced {
			summary.UnpricedEvents++
		}
		if event.Timestamp.After(summary.LastSeenAt) {
			summary.LastSeenAt = event.Timestamp
		}
		if modelSets[event.SessionID] == nil {
			modelSets[event.SessionID] = map[string]bool{}
		}
		modelSets[event.SessionID][event.Model] = true

		day := event.Timestamp.Local().Format("2006-01-02")
		if dailyByDay[day] == nil {
			dailyByDay[day] = &DailySpend{Day: day}
		}
		dailyByDay[day].TotalCost += event.Cost
		if !event.Priced {
			dailyByDay[day].UnpricedEvents++
		}
	}

	for i := range ordered {
		if live := summaryByID[ordered[i].ID]; live != nil {
			ordered[i] = *live
			ordered[i].Models = joinModels(modelSets[ordered[i].ID])
		}
	}

	var daily []DailySpend
	for _, spend := range dailyByDay {
		daily = append(daily, *spend)
	}
	sortDailyDesc(daily)
	return daily, ordered, nil
}

func (s *Store) Session(ctx context.Context, id string) (Session, []EventDetail, error) {
	var session Session
	var started string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, source_path, started_at, cwd, originator, cli_version, model_provider, title
		FROM sessions
		WHERE id = ?
	`, id).Scan(&session.ID, &session.SourcePath, &started, &session.CWD, &session.Originator, &session.CLIVersion, &session.ModelProvider, &session.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, nil, sql.ErrNoRows
	}
	if err != nil {
		return Session{}, nil, fmt.Errorf("querying session %s: %w", id, err)
	}
	session.StartedAt = parseStoredTime(started)

	events, err := s.allEventDetails(ctx, id)
	if err != nil {
		return Session{}, nil, err
	}
	return session, events, nil
}

func (s *Store) LatestPricingRefresh(ctx context.Context) (time.Time, bool, error) {
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT fetched_at FROM pricing_snapshots ORDER BY fetched_at DESC LIMIT 1`).Scan(&fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("querying latest pricing refresh: %w", err)
	}
	return parseStoredTime(fetched), true, nil
}

func (s *Store) DailySpendForDay(ctx context.Context, day time.Time) (DailySpend, error) {
	events, err := s.allEventDetails(ctx, "")
	if err != nil {
		return DailySpend{}, err
	}

	dayText := day.Local().Format("2006-01-02")
	spend := DailySpend{Day: dayText}
	for _, event := range events {
		if event.Timestamp.Local().Format("2006-01-02") != dayText {
			continue
		}
		spend.TotalCost += event.Cost
		if !event.Priced {
			spend.UnpricedEvents++
		}
	}
	return spend, nil
}

func (s *Store) allEventDetails(ctx context.Context, sessionID string) ([]EventDetail, error) {
	query := `
		SELECT
			e.id, e.session_id, e.event_index, e.timestamp, e.model,
			e.input_tokens, e.cached_input_tokens, e.output_tokens, e.reasoning_output_tokens, e.total_tokens,
			p.input_per_million, p.cached_input_per_million, p.output_per_million
		FROM usage_events e
		LEFT JOIN pricing_snapshots p ON p.id = (
			SELECT id FROM pricing_snapshots
			WHERE model = lower(e.model)
			ORDER BY fetched_at DESC
			LIMIT 1
		)
	`
	args := []any{}
	if sessionID != "" {
		query += ` WHERE e.session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY e.timestamp DESC, e.event_index DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying usage events: %w", err)
	}
	defer rows.Close()

	var events []EventDetail
	for rows.Next() {
		var ts string
		var event EventDetail
		if err := rows.Scan(
			&event.ID, &event.SessionID, &event.EventIndex, &ts, &event.Model,
			&event.InputTokens, &event.CachedInputTokens, &event.OutputTokens,
			&event.ReasoningOutputTokens, &event.TotalTokens,
			&event.InputPerMillion, &event.CachedInputPerMillion, &event.OutputPerMillion,
		); err != nil {
			return nil, fmt.Errorf("scanning usage event: %w", err)
		}
		event.Timestamp = parseStoredTime(ts)
		if event.InputPerMillion.Valid && event.CachedInputPerMillion.Valid && event.OutputPerMillion.Valid {
			event.Priced = true
			event.Cost = CalculateCost(event.InputTokens, event.CachedInputTokens, event.OutputTokens, event.InputPerMillion.Float64, event.CachedInputPerMillion.Float64, event.OutputPerMillion.Float64)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating usage events: %w", err)
	}
	return events, nil
}

func CalculateCost(inputTokens, cachedInputTokens, outputTokens int64, inputRate, cachedRate, outputRate float64) float64 {
	uncachedInputTokens := inputTokens - cachedInputTokens
	if uncachedInputTokens < 0 {
		uncachedInputTokens = 0
	}
	return (float64(uncachedInputTokens)*inputRate + float64(cachedInputTokens)*cachedRate + float64(outputTokens)*outputRate) / 1_000_000
}

func parseStoredTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
