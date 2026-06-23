package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	SettingCodexPath = "codex_path"
	SettingDBOwner   = "database_owner"
)

var windowsUserPathRE = regexp.MustCompile(`(?i)(?:^|[\\/])Users[\\/]([^\\/]+)(?:[\\/]|$)`)

var knownSQLiteIdentifiers = map[string]bool{
	"pricing_snapshots": true,
	"usage_events":      true,
}

var knownColumnDefinitions = map[string]string{
	"billing_tier": "TEXT NOT NULL DEFAULT 'standard'",
	"context_kind": "TEXT NOT NULL DEFAULT 'short'",
}

// Store wraps the application's SQLite database.
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
	BillingTier           string
	ContextKind           string
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
	RawJSON               string
}

type ToolUsageEvent struct {
	SessionID  string
	EventIndex int
	Timestamp  time.Time
	ToolKey    string
	ToolName   string
	Quantity   float64
	RawJSON    string
}

type PricingSnapshot struct {
	ID                    int64
	SourceURL             string
	FetchedAt             time.Time
	Model                 string
	BillingTier           string
	ContextKind           string
	InputPerMillion       float64
	CachedInputPerMillion float64
	OutputPerMillion      float64
}

type ToolPricingSnapshot struct {
	ID           int64
	SourceURL    string
	FetchedAt    time.Time
	ToolKey      string
	DisplayName  string
	UnitLabel    string
	UnitSize     float64
	PricePerUnit float64
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
	ToolCalls             float64
}

type EventDetail struct {
	ID                    int64
	SessionID             string
	EventIndex            int
	Timestamp             time.Time
	Model                 string
	BillingTier           string
	ContextKind           string
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

type ToolUsageDetail struct {
	ID           int64
	SessionID    string
	EventIndex   int
	Timestamp    time.Time
	ToolKey      string
	ToolName     string
	Quantity     float64
	UnitLabel    sql.NullString
	UnitSize     sql.NullFloat64
	PricePerUnit sql.NullFloat64
	Cost         float64
	Priced       bool
}

type DailySpend struct {
	Day            string
	TotalCost      float64
	UnpricedEvents int
}

// Open creates or opens the SQLite database at path and applies the app schema.
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
		return nil, errors.Join(err, db.Close())
	}
	return store, nil
}

// Close closes the underlying database connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	toolUsageExisted, err := s.tableExists(ctx, "tool_usage_events")
	if err != nil {
		return err
	}
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
			billing_tier TEXT NOT NULL DEFAULT 'standard',
			context_kind TEXT NOT NULL DEFAULT 'short',
			input_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			raw_json TEXT NOT NULL,
			UNIQUE(session_id, event_index)
		)`,
		`CREATE TABLE IF NOT EXISTS tool_usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			event_index INTEGER NOT NULL,
			timestamp TEXT NOT NULL,
			tool_key TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			quantity REAL NOT NULL,
			raw_json TEXT NOT NULL,
			UNIQUE(session_id, event_index, tool_key)
		)`,
		`CREATE TABLE IF NOT EXISTS pricing_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_url TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			model TEXT NOT NULL,
			billing_tier TEXT NOT NULL DEFAULT 'standard',
			context_kind TEXT NOT NULL DEFAULT 'short',
			input_per_million REAL NOT NULL,
			cached_input_per_million REAL NOT NULL,
			output_per_million REAL NOT NULL,
			UNIQUE(model, billing_tier, context_kind, fetched_at)
		)`,
		`CREATE TABLE IF NOT EXISTS tool_pricing_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_url TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			tool_key TEXT NOT NULL,
			display_name TEXT NOT NULL,
			unit_label TEXT NOT NULL,
			unit_size REAL NOT NULL,
			price_per_unit REAL NOT NULL,
			UNIQUE(tool_key, fetched_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_session ON usage_events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_timestamp ON usage_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_events_session ON tool_usage_events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_events_timestamp ON tool_usage_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_pricing_model_fetched ON pricing_snapshots(model, fetched_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_pricing_key_fetched ON tool_pricing_snapshots(tool_key, fetched_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("applying migration statement %q: %w", statement, err)
		}
	}
	if err := s.ensureColumn(ctx, "usage_events", "billing_tier", "TEXT NOT NULL DEFAULT 'standard'"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "usage_events", "context_kind", "TEXT NOT NULL DEFAULT 'short'"); err != nil {
		return err
	}
	if err := s.ensurePricingDimensions(ctx); err != nil {
		return err
	}
	if !toolUsageExisted {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM import_files`); err != nil {
			return fmt.Errorf("clearing import fingerprints for tool usage backfill: %w", err)
		}
	}
	return nil
}

func (s *Store) tableExists(ctx context.Context, table string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking table %s: %w", table, err)
	}
	return true, nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) (err error) {
	if err := validateColumnMigration(table, column, definition); err != nil {
		return err
	}

	rows, err := s.tableInfo(ctx, table)
	if err != nil {
		return fmt.Errorf("inspecting %s columns: %w", table, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing %s column rows: %w", table, closeErr))
		}
	}()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scanning %s columns: %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating %s columns: %w", table, err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition); err != nil {
		return fmt.Errorf("adding %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) ensurePricingDimensions(ctx context.Context) (err error) {
	hasTier, err := s.columnExists(ctx, "pricing_snapshots", "billing_tier")
	if err != nil {
		return err
	}
	if hasTier {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting pricing migration: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("rolling back pricing migration: %w", rollbackErr))
		}
	}()

	statements := []string{
		`CREATE TABLE pricing_snapshots_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_url TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			model TEXT NOT NULL,
			billing_tier TEXT NOT NULL DEFAULT 'standard',
			context_kind TEXT NOT NULL DEFAULT 'short',
			input_per_million REAL NOT NULL,
			cached_input_per_million REAL NOT NULL,
			output_per_million REAL NOT NULL,
			UNIQUE(model, billing_tier, context_kind, fetched_at)
		)`,
		`INSERT INTO pricing_snapshots_new (
			id, source_url, fetched_at, model, billing_tier, context_kind,
			input_per_million, cached_input_per_million, output_per_million
		)
		SELECT id, source_url, fetched_at, model, 'standard', 'short',
			input_per_million, cached_input_per_million, output_per_million
		FROM pricing_snapshots`,
		`DROP TABLE pricing_snapshots`,
		`ALTER TABLE pricing_snapshots_new RENAME TO pricing_snapshots`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrating pricing dimensions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) columnExists(ctx context.Context, table, column string) (exists bool, err error) {
	if !knownSQLiteIdentifiers[table] {
		return false, fmt.Errorf("unknown sqlite table identifier %q", table)
	}

	rows, err := s.tableInfo(ctx, table)
	if err != nil {
		return false, fmt.Errorf("inspecting %s columns: %w", table, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing %s column rows: %w", table, closeErr))
		}
	}()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scanning %s columns: %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating %s columns: %w", table, err)
	}
	return false, nil
}

func (s *Store) tableInfo(ctx context.Context, table string) (*sql.Rows, error) {
	if !knownSQLiteIdentifiers[table] {
		return nil, fmt.Errorf("unknown sqlite table identifier %q", table)
	}
	return s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
}

func validateColumnMigration(table, column, definition string) error {
	if !knownSQLiteIdentifiers[table] {
		return fmt.Errorf("unknown sqlite table identifier %q", table)
	}
	if knownColumnDefinitions[column] != definition {
		return fmt.Errorf("unknown sqlite column migration %s.%s", table, column)
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

// SetSetting stores a string setting by key.
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

// EnsureOwner records the current database owner and clears imported data when
// the existing database appears to belong to a different Windows user.
func (s *Store) EnsureOwner(ctx context.Context, owner string, currentUsernames []string) (bool, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return false, nil
	}

	existing, ok, err := s.Setting(ctx, SettingDBOwner)
	if err != nil {
		return false, err
	}
	if ok {
		if strings.EqualFold(existing, owner) {
			return false, nil
		}
		if err := s.ClearImportedData(ctx); err != nil {
			return false, err
		}
		return true, s.SetSetting(ctx, SettingDBOwner, owner)
	}

	different, err := s.hasDifferentSessionUser(ctx, currentUsernames)
	if err != nil {
		return false, err
	}
	if different {
		if err := s.ClearImportedData(ctx); err != nil {
			return false, err
		}
	}
	return different, s.SetSetting(ctx, SettingDBOwner, owner)
}

// ClearImportedData removes imported sessions and file fingerprints while
// preserving settings and pricing snapshots.
func (s *Store) ClearImportedData(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting cleanup transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("rolling back cleanup transaction: %w", rollbackErr))
		}
	}()

	statements := []string{
		`DELETE FROM tool_usage_events`,
		`DELETE FROM usage_events`,
		`DELETE FROM sessions`,
		`DELETE FROM import_files`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cleaning imported data: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing cleanup transaction: %w", err)
	}
	committed = true
	return nil
}

func (s *Store) hasDifferentSessionUser(ctx context.Context, currentUsernames []string) (different bool, err error) {
	current := map[string]bool{}
	for _, username := range currentUsernames {
		username = strings.ToLower(strings.TrimSpace(username))
		if username != "" {
			current[username] = true
		}
	}
	if len(current) == 0 {
		return false, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT source_path FROM sessions
		UNION ALL SELECT cwd FROM sessions
		UNION ALL SELECT path FROM import_files
	`)
	if err != nil {
		return false, fmt.Errorf("querying stored session users: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing stored session user rows: %w", closeErr))
		}
	}()

	foundOther := false
	foundCurrent := false
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return false, fmt.Errorf("scanning stored session user: %w", err)
		}
		username := windowsUsernameFromPath(value)
		if username == "" {
			continue
		}
		if current[strings.ToLower(username)] {
			foundCurrent = true
			continue
		}
		foundOther = true
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating stored session users: %w", err)
	}
	return foundOther && !foundCurrent, nil
}

func windowsUsernameFromPath(path string) string {
	matches := windowsUserPathRE.FindStringSubmatch(path)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
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
	event.BillingTier = normalizePricingDimension(event.BillingTier, "standard")
	event.ContextKind = normalizePricingDimension(event.ContextKind, "short")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_events (
			session_id, event_index, timestamp, model, billing_tier, context_kind, input_tokens, cached_input_tokens,
			output_tokens, reasoning_output_tokens, total_tokens, raw_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, event_index) DO UPDATE SET
			timestamp = excluded.timestamp,
			model = excluded.model,
			billing_tier = excluded.billing_tier,
			context_kind = excluded.context_kind,
			input_tokens = excluded.input_tokens,
			cached_input_tokens = excluded.cached_input_tokens,
			output_tokens = excluded.output_tokens,
			reasoning_output_tokens = excluded.reasoning_output_tokens,
			total_tokens = excluded.total_tokens,
			raw_json = excluded.raw_json
	`, event.SessionID, event.EventIndex, event.Timestamp.UTC().Format(time.RFC3339Nano), event.Model, event.BillingTier, event.ContextKind, event.InputTokens, event.CachedInputTokens, event.OutputTokens, event.ReasoningOutputTokens, event.TotalTokens, event.RawJSON)
	if err != nil {
		return fmt.Errorf("upserting usage event %s/%d: %w", event.SessionID, event.EventIndex, err)
	}
	return nil
}

func (s *Store) UpsertToolUsageEvent(ctx context.Context, event ToolUsageEvent) error {
	event.ToolKey = normalizePricingDimension(event.ToolKey, "")
	event.ToolName = strings.TrimSpace(event.ToolName)
	if event.ToolKey == "" {
		return nil
	}
	if event.ToolName == "" {
		event.ToolName = event.ToolKey
	}
	if event.Quantity <= 0 {
		event.Quantity = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_usage_events (
			session_id, event_index, timestamp, tool_key, tool_name, quantity, raw_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, event_index, tool_key) DO UPDATE SET
			timestamp = excluded.timestamp,
			tool_name = excluded.tool_name,
			quantity = excluded.quantity,
			raw_json = excluded.raw_json
	`, event.SessionID, event.EventIndex, event.Timestamp.UTC().Format(time.RFC3339Nano), event.ToolKey, event.ToolName, event.Quantity, event.RawJSON)
	if err != nil {
		return fmt.Errorf("upserting tool usage event %s/%d/%s: %w", event.SessionID, event.EventIndex, event.ToolKey, err)
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

func (s *Store) SeedToolPricing(ctx context.Context, prices []ToolPricingSnapshot) error {
	for _, price := range prices {
		if err := s.UpsertToolPricing(ctx, price); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertPricing(ctx context.Context, price PricingSnapshot) error {
	price.BillingTier = normalizePricingDimension(price.BillingTier, "standard")
	price.ContextKind = normalizePricingDimension(price.ContextKind, "short")
	if price.CachedInputPerMillion == 0 {
		price.CachedInputPerMillion = price.InputPerMillion
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pricing_snapshots (
			source_url, fetched_at, model, billing_tier, context_kind, input_per_million, cached_input_per_million, output_per_million
		)
		VALUES (?, ?, lower(?), ?, ?, ?, ?, ?)
		ON CONFLICT(model, billing_tier, context_kind, fetched_at) DO UPDATE SET
			source_url = excluded.source_url,
			input_per_million = excluded.input_per_million,
			cached_input_per_million = excluded.cached_input_per_million,
			output_per_million = excluded.output_per_million
	`, price.SourceURL, price.FetchedAt.UTC().Format(time.RFC3339Nano), price.Model, price.BillingTier, price.ContextKind, price.InputPerMillion, price.CachedInputPerMillion, price.OutputPerMillion)
	if err != nil {
		return fmt.Errorf("upserting price for %s: %w", price.Model, err)
	}
	return nil
}

func (s *Store) UpdatePricing(ctx context.Context, price PricingSnapshot) error {
	price.BillingTier = normalizePricingDimension(price.BillingTier, "standard")
	price.ContextKind = normalizePricingDimension(price.ContextKind, "short")
	if price.CachedInputPerMillion == 0 {
		price.CachedInputPerMillion = price.InputPerMillion
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE pricing_snapshots
		SET source_url = ?, fetched_at = ?, model = lower(?), billing_tier = ?, context_kind = ?,
			input_per_million = ?, cached_input_per_million = ?, output_per_million = ?
		WHERE id = ?
	`, price.SourceURL, price.FetchedAt.UTC().Format(time.RFC3339Nano), price.Model, price.BillingTier, price.ContextKind, price.InputPerMillion, price.CachedInputPerMillion, price.OutputPerMillion, price.ID)
	if err != nil {
		return fmt.Errorf("updating price %d: %w", price.ID, err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeletePricing(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM pricing_snapshots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting price %d: %w", id, err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Pricing(ctx context.Context) (prices []PricingSnapshot, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_url, fetched_at, model, billing_tier, context_kind,
			input_per_million, cached_input_per_million, output_per_million
		FROM pricing_snapshots
		ORDER BY model, billing_tier, context_kind, fetched_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying pricing: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing pricing rows: %w", closeErr))
		}
	}()

	for rows.Next() {
		var fetchedAt string
		var price PricingSnapshot
		if err := rows.Scan(&price.ID, &price.SourceURL, &fetchedAt, &price.Model, &price.BillingTier, &price.ContextKind, &price.InputPerMillion, &price.CachedInputPerMillion, &price.OutputPerMillion); err != nil {
			return nil, fmt.Errorf("scanning pricing: %w", err)
		}
		price.FetchedAt = parseStoredTime(fetchedAt)
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pricing: %w", err)
	}
	return prices, nil
}

func (s *Store) UpsertToolPricing(ctx context.Context, price ToolPricingSnapshot) error {
	price.ToolKey = normalizePricingDimension(price.ToolKey, "")
	price.DisplayName = strings.TrimSpace(price.DisplayName)
	price.UnitLabel = strings.TrimSpace(price.UnitLabel)
	if price.ToolKey == "" {
		return nil
	}
	if price.DisplayName == "" {
		price.DisplayName = price.ToolKey
	}
	if price.UnitLabel == "" {
		price.UnitLabel = "calls"
	}
	if price.UnitSize <= 0 {
		price.UnitSize = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_pricing_snapshots (
			source_url, fetched_at, tool_key, display_name, unit_label, unit_size, price_per_unit
		)
		VALUES (?, ?, lower(?), ?, ?, ?, ?)
		ON CONFLICT(tool_key, fetched_at) DO UPDATE SET
			source_url = excluded.source_url,
			display_name = excluded.display_name,
			unit_label = excluded.unit_label,
			unit_size = excluded.unit_size,
			price_per_unit = excluded.price_per_unit
	`, price.SourceURL, price.FetchedAt.UTC().Format(time.RFC3339Nano), price.ToolKey, price.DisplayName, price.UnitLabel, price.UnitSize, price.PricePerUnit)
	if err != nil {
		return fmt.Errorf("upserting tool price for %s: %w", price.ToolKey, err)
	}
	return nil
}

func (s *Store) UpdateToolPricing(ctx context.Context, price ToolPricingSnapshot) error {
	price.ToolKey = normalizePricingDimension(price.ToolKey, "")
	price.DisplayName = strings.TrimSpace(price.DisplayName)
	price.UnitLabel = strings.TrimSpace(price.UnitLabel)
	if price.UnitSize <= 0 {
		price.UnitSize = 1
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE tool_pricing_snapshots
		SET source_url = ?, fetched_at = ?, tool_key = lower(?), display_name = ?,
			unit_label = ?, unit_size = ?, price_per_unit = ?
		WHERE id = ?
	`, price.SourceURL, price.FetchedAt.UTC().Format(time.RFC3339Nano), price.ToolKey, price.DisplayName, price.UnitLabel, price.UnitSize, price.PricePerUnit, price.ID)
	if err != nil {
		return fmt.Errorf("updating tool price %d: %w", price.ID, err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteToolPricing(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tool_pricing_snapshots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting tool price %d: %w", id, err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ToolPricing(ctx context.Context) (prices []ToolPricingSnapshot, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_url, fetched_at, tool_key, display_name, unit_label, unit_size, price_per_unit
		FROM tool_pricing_snapshots
		ORDER BY tool_key, fetched_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tool pricing: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing tool pricing rows: %w", closeErr))
		}
	}()

	for rows.Next() {
		var fetchedAt string
		var price ToolPricingSnapshot
		if err := rows.Scan(&price.ID, &price.SourceURL, &fetchedAt, &price.ToolKey, &price.DisplayName, &price.UnitLabel, &price.UnitSize, &price.PricePerUnit); err != nil {
			return nil, fmt.Errorf("scanning tool pricing: %w", err)
		}
		price.FetchedAt = parseStoredTime(fetchedAt)
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tool pricing: %w", err)
	}
	return prices, nil
}

func (s *Store) Dashboard(ctx context.Context) (daily []DailySpend, ordered []SessionSummary, err error) {
	events, err := s.allEventDetails(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	toolEvents, err := s.allToolDetails(ctx, "")
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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing session summary rows: %w", closeErr))
		}
	}()

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
	for _, event := range toolEvents {
		summary := summaryByID[event.SessionID]
		if summary == nil {
			continue
		}
		summary.TotalCost += event.Cost
		summary.ToolCalls += event.Quantity
		if !event.Priced {
			summary.UnpricedEvents++
		}
		if event.Timestamp.After(summary.LastSeenAt) {
			summary.LastSeenAt = event.Timestamp
		}

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

	for _, spend := range dailyByDay {
		daily = append(daily, *spend)
	}
	sortDailyDesc(daily)
	return daily, ordered, nil
}

func (s *Store) Session(ctx context.Context, id string) (Session, []EventDetail, []ToolUsageDetail, error) {
	var session Session
	var started string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, source_path, started_at, cwd, originator, cli_version, model_provider, title
		FROM sessions
		WHERE id = ?
	`, id).Scan(&session.ID, &session.SourcePath, &started, &session.CWD, &session.Originator, &session.CLIVersion, &session.ModelProvider, &session.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, nil, nil, sql.ErrNoRows
	}
	if err != nil {
		return Session{}, nil, nil, fmt.Errorf("querying session %s: %w", id, err)
	}
	session.StartedAt = parseStoredTime(started)

	events, err := s.allEventDetails(ctx, id)
	if err != nil {
		return Session{}, nil, nil, err
	}
	toolEvents, err := s.allToolDetails(ctx, id)
	if err != nil {
		return Session{}, nil, nil, err
	}
	return session, events, toolEvents, nil
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
	toolEvents, err := s.allToolDetails(ctx, "")
	if err != nil {
		return DailySpend{}, err
	}
	for _, event := range toolEvents {
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

func (s *Store) allEventDetails(ctx context.Context, sessionID string) (events []EventDetail, err error) {
	query := `
		SELECT
			e.id, e.session_id, e.event_index, e.timestamp, e.model,
			e.billing_tier, e.context_kind,
			e.input_tokens, e.cached_input_tokens, e.output_tokens, e.reasoning_output_tokens, e.total_tokens,
			p.input_per_million, p.cached_input_per_million, p.output_per_million
		FROM usage_events e
		LEFT JOIN pricing_snapshots p ON p.id = (
			SELECT id FROM pricing_snapshots
			WHERE model = lower(e.model)
				AND billing_tier = e.billing_tier
				AND context_kind = e.context_kind
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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing usage event rows: %w", closeErr))
		}
	}()

	for rows.Next() {
		var ts string
		var event EventDetail
		if err := rows.Scan(
			&event.ID, &event.SessionID, &event.EventIndex, &ts, &event.Model,
			&event.BillingTier, &event.ContextKind,
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

func (s *Store) allToolDetails(ctx context.Context, sessionID string) (events []ToolUsageDetail, err error) {
	query := `
		SELECT
			e.id, e.session_id, e.event_index, e.timestamp, e.tool_key, e.tool_name, e.quantity,
			p.unit_label, p.unit_size, p.price_per_unit
		FROM tool_usage_events e
		LEFT JOIN tool_pricing_snapshots p ON p.id = (
			SELECT id FROM tool_pricing_snapshots
			WHERE tool_key = e.tool_key
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
		return nil, fmt.Errorf("querying tool usage events: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing tool usage event rows: %w", closeErr))
		}
	}()

	for rows.Next() {
		var ts string
		var event ToolUsageDetail
		if err := rows.Scan(
			&event.ID, &event.SessionID, &event.EventIndex, &ts, &event.ToolKey, &event.ToolName, &event.Quantity,
			&event.UnitLabel, &event.UnitSize, &event.PricePerUnit,
		); err != nil {
			return nil, fmt.Errorf("scanning tool usage event: %w", err)
		}
		event.Timestamp = parseStoredTime(ts)
		if event.UnitSize.Valid && event.PricePerUnit.Valid && event.UnitSize.Float64 > 0 {
			event.Priced = true
			event.Cost = CalculateToolCost(event.Quantity, event.UnitSize.Float64, event.PricePerUnit.Float64)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tool usage events: %w", err)
	}
	return events, nil
}

func CalculateToolCost(quantity, unitSize, pricePerUnit float64) float64 {
	if quantity <= 0 || unitSize <= 0 || pricePerUnit <= 0 {
		return 0
	}
	return quantity / unitSize * pricePerUnit
}

func normalizePricingDimension(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func RoundUpToCent(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Ceil(value*100) / 100
}

func parseStoredTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
