package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"codexspendmonitor/internal/ingest"
	"codexspendmonitor/internal/pricing"
	"codexspendmonitor/internal/store"
)

type Server struct {
	store   *store.Store
	scanner *ingest.Scanner
	pricing *pricing.Service
	logger  *slog.Logger
	tmpl    *template.Template
}

type dashboardView struct {
	CodexPath       string
	PricingRefresh string
	Daily           []store.DailySpend
	Sessions        []store.SessionSummary
	Message         string
	Error           string
}

type sessionView struct {
	Session store.Session
	Events  []store.EventDetail
}

func NewServer(store *store.Store, scanner *ingest.Scanner, pricingService *pricing.Service, logger *slog.Logger) *Server {
	funcs := template.FuncMap{
		"money": func(v float64) string { return fmt.Sprintf("$%.6f", v) },
		"shortID": func(v string) string {
			if len(v) <= 8 {
				return v
			}
			return v[:8]
		},
		"dateTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Local().Format("2006-01-02 15:04:05")
		},
	}

	return &Server{
		store:   store,
		scanner: scanner,
		pricing: pricingService,
		logger:  logger,
		tmpl:    template.Must(template.New("pages").Funcs(funcs).Parse(pagesHTML)),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /sessions/", s.session)
	mux.HandleFunc("POST /settings/codex-path", s.updateCodexPath)
	mux.HandleFunc("POST /scan", s.scan)
	mux.HandleFunc("POST /pricing/refresh", s.refreshPricing)
	return mux
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.renderDashboard(w, r, "", "")
}

func (s *Server) updateCodexPath(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderDashboard(w, r, "", "Could not read form data.")
		return
	}
	path := strings.TrimSpace(r.FormValue("codex_path"))
	if path == "" {
		s.renderDashboard(w, r, "", "Codex path is required.")
		return
	}
	if err := s.store.SetSetting(r.Context(), store.SettingCodexPath, path); err != nil {
		s.logger.Warn("update codex path", "error", err)
		s.renderDashboard(w, r, "", "Could not save Codex path.")
		return
	}
	s.renderDashboard(w, r, "Codex path updated.", "")
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	path, ok, err := s.store.Setting(r.Context(), store.SettingCodexPath)
	if err != nil || !ok || path == "" {
		s.renderDashboard(w, r, "", "Set a Codex path before scanning.")
		return
	}
	result, err := s.scanner.Scan(r.Context(), path)
	if err != nil {
		s.renderDashboard(w, r, "", err.Error())
		return
	}
	s.renderDashboard(w, r, fmt.Sprintf("Scan complete: %d files, %d sessions, %d events.", result.Files, result.Sessions, result.Events), "")
}

func (s *Server) refreshPricing(w http.ResponseWriter, r *http.Request) {
	count, err := s.pricing.Refresh(r.Context())
	if err != nil {
		s.renderDashboard(w, r, "", err.Error())
		return
	}
	s.renderDashboard(w, r, fmt.Sprintf("Pricing refreshed: %d models.", count), "")
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	session, events, err := s.store.Session(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Could not load session.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "session", sessionView{Session: session, Events: events}); err != nil {
		s.logger.Warn("render session", "error", err)
	}
}

func (s *Server) renderDashboard(w http.ResponseWriter, r *http.Request, message, renderErr string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	path, _, err := s.store.Setting(ctx, store.SettingCodexPath)
	if err != nil {
		renderErr = "Could not load Codex path."
	}
	daily, sessions, err := s.store.Dashboard(ctx)
	if err != nil {
		s.logger.Warn("load dashboard", "error", err)
		renderErr = "Could not load dashboard data."
	}
	refreshText := "never"
	if refreshed, ok, err := s.store.LatestPricingRefresh(ctx); err == nil && ok {
		refreshText = refreshed.Local().Format("2006-01-02 15:04:05")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	view := dashboardView{
		CodexPath:       path,
		PricingRefresh: refreshText,
		Daily:           daily,
		Sessions:        sessions,
		Message:         message,
		Error:           renderErr,
	}
	if err := s.tmpl.ExecuteTemplate(w, "dashboard", view); err != nil {
		s.logger.Warn("render dashboard", "error", err)
	}
}
