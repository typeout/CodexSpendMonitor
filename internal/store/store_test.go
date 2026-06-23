package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCalculateCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      int64
		cached     int64
		output     int64
		inputRate  float64
		cachedRate float64
		outputRate float64
		want       float64
	}{
		{
			name:       "mixed cached and uncached",
			input:      1000,
			cached:     400,
			output:     100,
			inputRate:  5,
			cachedRate: 0.5,
			outputRate: 30,
			want:       0.0062,
		},
		{
			name:       "cached greater than input clamps uncached",
			input:      100,
			cached:     200,
			output:     0,
			inputRate:  5,
			cachedRate: 0.5,
			outputRate: 30,
			want:       0.0001,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateCost(tt.input, tt.cached, tt.output, tt.inputRate, tt.cachedRate, tt.outputRate)
			if got != tt.want {
				t.Fatalf("CalculateCost() = %.8f, want %.8f", got, tt.want)
			}
		})
	}
}

func TestDashboardUsesPricingTierAndContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	startedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	session := Session{
		ID:            "pricing-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\pricing.jsonl`,
		StartedAt:     startedAt,
		CWD:           `C:\Working\Pricing`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Pricing",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	if err := db.UpsertPricing(ctx, PricingSnapshot{
		SourceURL:             "test",
		FetchedAt:             startedAt,
		Model:                 "gpt-test",
		BillingTier:           "standard",
		ContextKind:           "short",
		InputPerMillion:       10,
		CachedInputPerMillion: 1,
		OutputPerMillion:      20,
	}); err != nil {
		t.Fatalf("UpsertPricing(standard short) error = %v", err)
	}
	if err := db.UpsertPricing(ctx, PricingSnapshot{
		SourceURL:             "test",
		FetchedAt:             startedAt,
		Model:                 "gpt-test",
		BillingTier:           "batch",
		ContextKind:           "long",
		InputPerMillion:       2,
		CachedInputPerMillion: 0.5,
		OutputPerMillion:      4,
	}); err != nil {
		t.Fatalf("UpsertPricing(batch long) error = %v", err)
	}
	if err := db.UpsertUsageEvent(ctx, UsageEvent{
		SessionID:         session.ID,
		EventIndex:        1,
		Timestamp:         startedAt,
		Model:             "gpt-test",
		BillingTier:       "batch",
		ContextKind:       "long",
		InputTokens:       1_000_000,
		CachedInputTokens: 500_000,
		OutputTokens:      250_000,
		TotalTokens:       1_250_000,
		RawJSON:           "{}",
	}); err != nil {
		t.Fatalf("UpsertUsageEvent() error = %v", err)
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	const want = 1.25 + 1.0
	if sessions[0].TotalCost != want {
		t.Fatalf("TotalCost = %.4f, want %.4f", sessions[0].TotalCost, want)
	}
}

func TestDashboardIncludesToolUsageCost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	startedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	session := Session{
		ID:            "tool-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\tool.jsonl`,
		StartedAt:     startedAt,
		CWD:           `C:\Working\Tools`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Tools",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	if err := db.UpsertToolPricing(ctx, ToolPricingSnapshot{
		SourceURL:    "test",
		FetchedAt:    startedAt,
		ToolKey:      "web_search",
		DisplayName:  "Web search",
		UnitLabel:    "1k calls",
		UnitSize:     1000,
		PricePerUnit: 10,
	}); err != nil {
		t.Fatalf("UpsertToolPricing() error = %v", err)
	}
	if err := db.UpsertToolUsageEvent(ctx, ToolUsageEvent{
		SessionID:  session.ID,
		EventIndex: 2,
		Timestamp:  startedAt,
		ToolKey:    "web_search",
		ToolName:   "web.run",
		Quantity:   2,
		RawJSON:    "{}",
	}); err != nil {
		t.Fatalf("UpsertToolUsageEvent() error = %v", err)
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	if sessions[0].TotalCost != 0.02 {
		t.Fatalf("TotalCost = %.4f, want 0.0200", sessions[0].TotalCost)
	}
	if sessions[0].ToolCalls != 2 {
		t.Fatalf("ToolCalls = %.2f, want 2", sessions[0].ToolCalls)
	}
}

func TestEnsureOwnerClearsForeignSessionData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	insertTestSession(t, ctx, db, `C:\Users\Bob\.codex\sessions\session-1.jsonl`)
	reset, err := db.EnsureOwner(ctx, "current-owner", []string{"Alice"})
	if err != nil {
		t.Fatalf("EnsureOwner() error = %v", err)
	}
	if !reset {
		t.Fatalf("EnsureOwner() reset = false, want true")
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestEnsureOwnerKeepsCurrentUserSessionData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	insertTestSession(t, ctx, db, `C:\Users\Alice\.codex\sessions\session-1.jsonl`)
	reset, err := db.EnsureOwner(ctx, "current-owner", []string{"Alice"})
	if err != nil {
		t.Fatalf("EnsureOwner() error = %v", err)
	}
	if reset {
		t.Fatalf("EnsureOwner() reset = true, want false")
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
}

func TestEnsureOwnerClearsWhenStoredOwnerDiffers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	if err := db.SetSetting(ctx, SettingDBOwner, "old-owner"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}
	insertTestSession(t, ctx, db, `C:\Users\Alice\.codex\sessions\session-1.jsonl`)

	reset, err := db.EnsureOwner(ctx, "new-owner", []string{"Alice"})
	if err != nil {
		t.Fatalf("EnsureOwner() error = %v", err)
	}
	if !reset {
		t.Fatalf("EnsureOwner() reset = false, want true")
	}

	_, sessions, err := db.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func openTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	db, err := Open(ctx, filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return db
}

func insertTestSession(t *testing.T, ctx context.Context, db *Store, sourcePath string) {
	t.Helper()

	session := Session{
		ID:            "session-1",
		SourcePath:    sourcePath,
		StartedAt:     time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
		CWD:           filepath.Dir(sourcePath),
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Test Session",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	if err := db.UpsertUsageEvent(ctx, UsageEvent{
		SessionID:   session.ID,
		EventIndex:  1,
		Timestamp:   session.StartedAt,
		Model:       "gpt-5",
		InputTokens: 100,
		TotalTokens: 100,
		RawJSON:     "{}",
	}); err != nil {
		t.Fatalf("UpsertUsageEvent() error = %v", err)
	}
}
