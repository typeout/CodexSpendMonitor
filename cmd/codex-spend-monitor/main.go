package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"codexspendmonitor/internal/ingest"
	"codexspendmonitor/internal/pricing"
	"codexspendmonitor/internal/store"
	"codexspendmonitor/internal/tray"
	"codexspendmonitor/internal/trayutil"
	"codexspendmonitor/internal/web"
)

type app struct {
	url     string
	icon    string
	ctx     context.Context
	stop    context.CancelFunc
	logger  *slog.Logger
	db      *store.Store
	scanner *ingest.Scanner
	pricer  *pricing.Service
	server  *http.Server
}

func main() {
	var (
		addr     = flag.String("addr", "127.0.0.1:5077", "HTTP listen address")
		dbPath   = flag.String("db", defaultDBPath(), "SQLite database path")
		iconPath = flag.String("icon", defaultIconPath(), "PNG file to use for the Windows tray icon")
	)
	flag.Parse()

	logger := newLogger()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}

	if err := ensureDefaults(ctx, db); err != nil {
		logger.Error("initialize defaults", "error", err)
		os.Exit(1)
	}

	scanner := ingest.NewScanner(db, logger)
	pricer := pricing.NewService(db, logger)
	handler := web.NewServer(db, scanner, pricer, logger)

	application := &app{
		url:     "http://" + *addr,
		icon:    *iconPath,
		ctx:     ctx,
		stop:    stop,
		logger:  logger,
		db:      db,
		scanner: scanner,
		pricer:  pricer,
		server: &http.Server{
			Addr:              *addr,
			Handler:           handler.Routes(),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	if err := application.run(); err != nil {
		logger.Error("run tray app", "error", err)
		application.shutdown()
		os.Exit(1)
	}
}

func (a *app) run() error {
	go poll(a.ctx, a.db, a.scanner, a.logger, 10*time.Second)
	go func() {
		a.logger.Info("listening", "addr", a.url)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("http server", "error", err)
			a.stop()
		}
	}()

	go func() {
		<-a.ctx.Done()
		tray.Quit()
	}()

	err := tray.Run(tray.Options{
		Tooltip:      "Codex Spend Monitor",
		IconPath:     a.icon,
		TodaySummary: a.todaySummary,
		OnOpen:       a.openDashboard,
		OnQuit:       a.stop,
	})
	a.shutdown()
	return err
}

func (a *app) openDashboard() {
	if err := trayutil.OpenURL(a.url); err != nil {
		a.logger.Warn("open dashboard", "error", err)
	}
}

func (a *app) todaySummary() string {
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()

	spend, err := a.db.DailySpendForDay(ctx, time.Now())
	if err != nil {
		a.logger.Warn("load daily spend for tray", "error", err)
		return "Today: unavailable"
	}
	text := fmt.Sprintf("Today: $%.6f", spend.TotalCost)
	if spend.UnpricedEvents > 0 {
		text += fmt.Sprintf(" (%d unpriced)", spend.UnpricedEvents)
	}
	return text
}

func (a *app) shutdown() {
	a.stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("shutdown server", "error", err)
	}
	if err := a.db.Close(); err != nil {
		a.logger.Error("close database", "error", err)
	}
}

func ensureDefaults(ctx context.Context, db *store.Store) error {
	if _, ok, err := db.Setting(ctx, store.SettingCodexPath); err != nil {
		return err
	} else if !ok {
		if err := db.SetSetting(ctx, store.SettingCodexPath, defaultCodexPath()); err != nil {
			return err
		}
	}

	if err := db.SeedPricing(ctx, pricing.FallbackPrices()); err != nil {
		return err
	}
	return nil
}

func poll(ctx context.Context, db *store.Store, scanner *ingest.Scanner, logger *slog.Logger, interval time.Duration) {
	runScan(ctx, db, scanner, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runScan(ctx, db, scanner, logger)
		}
	}
}

func runScan(ctx context.Context, db *store.Store, scanner *ingest.Scanner, logger *slog.Logger) {
	codexPath, ok, err := db.Setting(ctx, store.SettingCodexPath)
	if err != nil {
		logger.Warn("load codex path", "error", err)
		return
	}
	if !ok || codexPath == "" {
		return
	}

	result, err := scanner.Scan(ctx, codexPath)
	if err != nil {
		logger.Warn("scan codex directory", "path", codexPath, "error", err)
		return
	}
	logger.Info("scan complete", "files", result.Files, "sessions", result.Sessions, "events", result.Events, "malformed_lines", result.MalformedLines)
}

func defaultDBPath() string {
	if data, err := os.UserConfigDir(); err == nil && data != "" {
		return filepath.Join(data, "CodexSpendMonitor", "codex-spend-monitor.sqlite")
	}
	return "codex-spend-monitor.sqlite"
}

func defaultCodexPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

func defaultIconPath() string {
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "ChatGPT Image Jun 23, 2026, 12_33_06 PM.png")
	}
	return "ChatGPT Image Jun 23, 2026, 12_33_06 PM.png"
}

func newLogger() *slog.Logger {
	writers := []io.Writer{os.Stdout}
	if data, err := os.UserConfigDir(); err == nil && data != "" {
		dir := filepath.Join(data, "CodexSpendMonitor")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			if file, err := os.OpenFile(filepath.Join(dir, "codex-spend-monitor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				writers = append(writers, file)
			}
		}
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo}))
}
