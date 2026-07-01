package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codexspendmonitor/internal/pricing"
	"codexspendmonitor/internal/store"
)

func TestServerRoutesDashboard(t *testing.T) {
	t.Parallel()

	server, db := newTestServer(t)
	if err := db.SetSetting(context.Background(), store.SettingCodexPath, t.TempDir()); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Codex Spend Monitor") {
		t.Fatalf("response body does not contain dashboard title")
	}
	if !strings.Contains(rec.Body.String(), "Usage Over Time") {
		t.Fatalf("response body does not contain usage history section")
	}
	if !strings.Contains(rec.Body.String(), `aria-label="Dashboard sections"`) {
		t.Fatalf("response body does not contain dashboard section tablist")
	}
	if !strings.Contains(rec.Body.String(), ">Projects</button>") {
		t.Fatalf("response body does not contain projects section tab")
	}
	if !strings.Contains(rec.Body.String(), ">Usage Over Time</button>") {
		t.Fatalf("response body does not contain usage section tab")
	}
	if !strings.Contains(rec.Body.String(), `aria-label="Usage graphs"`) {
		t.Fatalf("response body does not contain usage graph tablist")
	}
	if !strings.Contains(rec.Body.String(), "cdn.jsdelivr.net/npm/apexcharts") {
		t.Fatalf("response body does not load ApexCharts")
	}
}

func TestServerUpdateCodexPath(t *testing.T) {
	t.Parallel()

	server, db := newTestServer(t)
	body := strings.NewReader("codex_path=C%3A%5CUsers%5CAlice%5C.codex")
	req := httptest.NewRequest(http.MethodPost, "/settings/codex-path", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	got, ok, err := db.Setting(context.Background(), store.SettingCodexPath)
	if err != nil {
		t.Fatalf("Setting() error = %v", err)
	}
	if !ok || got != `C:\Users\Alice\.codex` {
		t.Fatalf("codex path = %q, %v; want saved path", got, ok)
	}
}

func TestServerSessionNotFound(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/sessions/missing", nil)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServerDashboardRendersChartData(t *testing.T) {
	t.Parallel()

	server, db := newTestServer(t)
	ctx := context.Background()
	startedAt := time.Now().Add(-24 * time.Hour).UTC()
	session := store.Session{
		ID:            "chart-session",
		SourcePath:    `C:\Users\Alice\.codex\sessions\chart.jsonl`,
		StartedAt:     startedAt,
		CWD:           `C:\Working\Charts`,
		Originator:    "Codex Desktop",
		CLIVersion:    "test",
		ModelProvider: "openai",
		Title:         "Charts",
	}
	if err := db.UpsertSession(ctx, session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}
	for _, model := range []string{"gpt-5.5", "gpt-5.4"} {
		if err := db.UpsertPricing(ctx, store.PricingSnapshot{
			SourceURL:             "test",
			FetchedAt:             startedAt,
			Model:                 model,
			BillingTier:           "standard",
			ContextKind:           "short",
			InputPerMillion:       2,
			CachedInputPerMillion: 1,
			OutputPerMillion:      4,
		}); err != nil {
			t.Fatalf("UpsertPricing(%s) error = %v", model, err)
		}
	}
	events := []store.UsageEvent{
		{
			SessionID:             session.ID,
			EventIndex:            1,
			Timestamp:             startedAt,
			Model:                 "gpt-5.5",
			InputTokens:           1000,
			CachedInputTokens:     250,
			OutputTokens:          80,
			ReasoningOutputTokens: 20,
			TotalTokens:           1100,
			RawJSON:               "{}",
		},
		{
			SessionID:         session.ID,
			EventIndex:        2,
			Timestamp:         startedAt.Add(2 * time.Hour),
			Model:             "gpt-5.4",
			InputTokens:       500,
			CachedInputTokens: 100,
			OutputTokens:      25,
			TotalTokens:       525,
			RawJSON:           "{}",
		},
	}
	for _, event := range events {
		if err := db.UpsertUsageEvent(ctx, event); err != nil {
			t.Fatalf("UpsertUsageEvent() error = %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"token-history-chart-data",
		"credit-history-chart-data",
		"token-chart-mode-total",
		"token-chart-mode-split",
		"section-tab-projects",
		"section-tab-usage",
		`"alternateSeries"`,
		"gpt-5.5 uncached",
		"gpt-5.5",
		"gpt-5.4",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body does not contain %q", want)
		}
	}
}

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()

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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(db, nil, pricing.NewService(db, logger), logger), db
}
