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

func TestUsageTokenSeriesAggregatesByLocalDayAndType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	loc := time.FixedZone("CDT", -5*60*60)
	since := time.Date(2026, 6, 23, 0, 0, 0, 0, loc)
	session := Session{
		ID:            "token-series-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\token-series.jsonl`,
		StartedAt:     since.UTC(),
		CWD:           `C:\Working\Series`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Token Series",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	events := []UsageEvent{
		{
			SessionID:             session.ID,
			EventIndex:            1,
			Timestamp:             time.Date(2026, 6, 24, 3, 10, 0, 0, time.UTC),
			Model:                 "gpt-a",
			InputTokens:           100,
			CachedInputTokens:     25,
			OutputTokens:          10,
			ReasoningOutputTokens: 5,
			TotalTokens:           115,
			RawJSON:               "{}",
		},
		{
			SessionID:             session.ID,
			EventIndex:            2,
			Timestamp:             time.Date(2026, 6, 24, 4, 45, 0, 0, time.UTC),
			Model:                 "gpt-a",
			InputTokens:           60,
			CachedInputTokens:     10,
			OutputTokens:          4,
			ReasoningOutputTokens: 6,
			TotalTokens:           70,
			RawJSON:               "{}",
		},
		{
			SessionID:             session.ID,
			EventIndex:            3,
			Timestamp:             time.Date(2026, 6, 24, 7, 0, 0, 0, time.UTC),
			Model:                 "gpt-a",
			InputTokens:           40,
			CachedInputTokens:     5,
			OutputTokens:          3,
			ReasoningOutputTokens: 2,
			TotalTokens:           45,
			RawJSON:               "{}",
		},
	}
	for _, event := range events {
		if err := db.UpsertUsageEvent(ctx, event); err != nil {
			t.Fatalf("UpsertUsageEvent() error = %v", err)
		}
	}

	points, err := db.UsageTokenSeries(ctx, since)
	if err != nil {
		t.Fatalf("UsageTokenSeries() error = %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2", len(points))
	}

	if points[0].Day != "2026-06-23" || points[0].Model != "gpt-a" {
		t.Fatalf("points[0] = %+v, want first local day bucket", points[0])
	}
	if points[0].UncachedInputTokens != 125 || points[0].CachedInputTokens != 35 || points[0].OutputTokens != 25 {
		t.Fatalf("points[0] = %+v, want uncached=125 cached=35 output=25", points[0])
	}
	if points[1].Day != "2026-06-24" || points[1].OutputTokens != 5 {
		t.Fatalf("points[1] = %+v, want second local day bucket with output=5", points[1])
	}
}

func TestUsageCreditSeriesUsesLatestPricingAndExcludesUnpricedAndTools(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	loc := time.FixedZone("CDT", -5*60*60)
	since := time.Date(2026, 6, 23, 0, 0, 0, 0, loc)
	startedAt := since.UTC()
	session := Session{
		ID:            "credit-series-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\credit-series.jsonl`,
		StartedAt:     startedAt,
		CWD:           `C:\Working\Series`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Credit Series",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	if err := db.UpsertPricing(ctx, PricingSnapshot{
		SourceURL:             "test",
		FetchedAt:             time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC),
		Model:                 "gpt-a",
		BillingTier:           "standard",
		ContextKind:           "short",
		InputPerMillion:       1,
		CachedInputPerMillion: 0.5,
		OutputPerMillion:      2,
	}); err != nil {
		t.Fatalf("UpsertPricing(old) error = %v", err)
	}
	if err := db.UpsertPricing(ctx, PricingSnapshot{
		SourceURL:             "test",
		FetchedAt:             time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Model:                 "gpt-a",
		BillingTier:           "standard",
		ContextKind:           "short",
		InputPerMillion:       4,
		CachedInputPerMillion: 1,
		OutputPerMillion:      8,
	}); err != nil {
		t.Fatalf("UpsertPricing(new) error = %v", err)
	}
	if err := db.UpsertToolPricing(ctx, ToolPricingSnapshot{
		SourceURL:    "test",
		FetchedAt:    startedAt,
		ToolKey:      "web_search",
		DisplayName:  "Web search",
		UnitLabel:    "call",
		UnitSize:     1,
		PricePerUnit: 50,
	}); err != nil {
		t.Fatalf("UpsertToolPricing() error = %v", err)
	}

	usageEvents := []UsageEvent{
		{
			SessionID:         session.ID,
			EventIndex:        1,
			Timestamp:         time.Date(2026, 6, 24, 3, 0, 0, 0, time.UTC),
			Model:             "gpt-a",
			InputTokens:       1000,
			CachedInputTokens: 200,
			OutputTokens:      50,
			TotalTokens:       1050,
			RawJSON:           "{}",
		},
		{
			SessionID:   session.ID,
			EventIndex:  2,
			Timestamp:   time.Date(2026, 6, 24, 4, 0, 0, 0, time.UTC),
			Model:       "gpt-unpriced",
			InputTokens: 999,
			TotalTokens: 999,
			RawJSON:     "{}",
		},
	}
	for _, event := range usageEvents {
		if err := db.UpsertUsageEvent(ctx, event); err != nil {
			t.Fatalf("UpsertUsageEvent() error = %v", err)
		}
	}
	if err := db.UpsertToolUsageEvent(ctx, ToolUsageEvent{
		SessionID:  session.ID,
		EventIndex: 3,
		Timestamp:  time.Date(2026, 6, 24, 3, 30, 0, 0, time.UTC),
		ToolKey:    "web_search",
		ToolName:   "web.run",
		Quantity:   2,
		RawJSON:    "{}",
	}); err != nil {
		t.Fatalf("UpsertToolUsageEvent() error = %v", err)
	}

	points, err := db.UsageCreditSeries(ctx, since)
	if err != nil {
		t.Fatalf("UsageCreditSeries() error = %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	if points[0].Day != "2026-06-23" || points[0].Model != "gpt-a" {
		t.Fatalf("points[0] = %+v, want priced local day bucket for gpt-a", points[0])
	}
	const want = (800*4 + 200*1 + 50*8) / 1_000_000.0
	if points[0].Cost != want {
		t.Fatalf("points[0].Cost = %.8f, want %.8f", points[0].Cost, want)
	}
}

func TestUsageCreditSeriesStableOrderingAndEmptyWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestStore(t, ctx)
	t.Cleanup(func() { _ = db.Close() })

	loc := time.FixedZone("CDT", -5*60*60)
	since := time.Date(2026, 6, 23, 0, 0, 0, 0, loc)
	session := Session{
		ID:            "ordering-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\ordering.jsonl`,
		StartedAt:     since.UTC(),
		CWD:           `C:\Working\Series`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Ordering",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	for _, model := range []string{"gpt-b", "gpt-a"} {
		if err := db.UpsertPricing(ctx, PricingSnapshot{
			SourceURL:             "test",
			FetchedAt:             since.UTC(),
			Model:                 model,
			BillingTier:           "standard",
			ContextKind:           "short",
			InputPerMillion:       1,
			CachedInputPerMillion: 1,
			OutputPerMillion:      1,
		}); err != nil {
			t.Fatalf("UpsertPricing(%s) error = %v", model, err)
		}
	}
	for index, model := range []string{"gpt-b", "gpt-a"} {
		if err := db.UpsertUsageEvent(ctx, UsageEvent{
			SessionID:   session.ID,
			EventIndex:  index + 1,
			Timestamp:   time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
			Model:       model,
			InputTokens: 100,
			TotalTokens: 100,
			RawJSON:     "{}",
		}); err != nil {
			t.Fatalf("UpsertUsageEvent(%s) error = %v", model, err)
		}
	}

	points, err := db.UsageCreditSeries(ctx, since)
	if err != nil {
		t.Fatalf("UsageCreditSeries() error = %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2", len(points))
	}
	if points[0].Model != "gpt-a" || points[1].Model != "gpt-b" {
		t.Fatalf("points = %+v, want alphabetical ordering for equal totals", points)
	}

	empty, err := db.UsageCreditSeries(ctx, since.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("UsageCreditSeries(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("len(empty) = %d, want 0", len(empty))
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
