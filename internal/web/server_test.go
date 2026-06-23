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
