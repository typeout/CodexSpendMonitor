package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
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
	CodexPath      string
	PricingRefresh string
	Totals         periodTotals
	TokenChart     dashboardChart
	CreditChart    dashboardChart
	Projects       []projectView
	Message        string
	Error          string
}

type periodTotals struct {
	Today       float64
	Yesterday   float64
	WeekToDate  float64
	MonthToDate float64
}

type projectView struct {
	Name           string
	Path           string
	SessionCount   int
	ToolCalls      float64
	TotalTokens    int64
	TotalCost      float64
	UnpricedEvents int
	Sessions       []store.SessionSummary
}

type sessionView struct {
	Session    store.Session
	Events     []store.EventDetail
	ToolEvents []store.ToolUsageDetail
}

type pricingView struct {
	Prices     []store.PricingSnapshot
	ToolPrices []store.ToolPricingSnapshot
	Message    string
	Error      string
}

type dashboardChart struct {
	ID       string
	DataID   string
	Title    string
	Subtitle string
	Empty    bool
	DataJSON template.JS
}

type dashboardChartPayload struct {
	ValueKind       string            `json:"valueKind"`
	Labels          []string          `json:"labels"`
	Series          []dashboardSeries `json:"series"`
	AlternateSeries []dashboardSeries `json:"alternateSeries,omitempty"`
}

type dashboardSeries struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Color  string    `json:"color"`
	Values []float64 `json:"values"`
}

const dashboardHistoryDays = 30
const dollarsPerCredit = 0.04

func NewServer(db *store.Store, scanner *ingest.Scanner, pricingService *pricing.Service, logger *slog.Logger) *Server {
	funcs := template.FuncMap{
		"money": func(v float64) string { return fmt.Sprintf("$%.2f", store.RoundUpToCent(v)) },
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
		"eq": func(a, b string) bool { return a == b },
		"quantity": func(v float64) string {
			if v == float64(int64(v)) {
				return fmt.Sprintf("%.0f", v)
			}
			return fmt.Sprintf("%.2f", v)
		},
	}

	return &Server{
		store:   db,
		scanner: scanner,
		pricing: pricingService,
		logger:  logger,
		tmpl:    template.Must(template.New("pages").Funcs(funcs).Parse(pagesHTML)),
	}
}

func (s *Server) Routes() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Codex-Spend-Monitor", "1")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			s.dashboard(w, r)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sessions/"):
			s.session(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/pricing":
			s.pricingPage(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/pricing":
			s.addPricing(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/pricing/tools":
			s.addToolPricing(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/pricing/refresh":
			s.refreshPricing(w, r)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/pricing/tools/"):
			s.updateToolPricing(w, r)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/pricing/"):
			s.updatePricing(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/settings/codex-path":
			s.updateCodexPath(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/scan":
			s.scan(w, r)
		default:
			http.NotFound(w, r)
		}
	})
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
		s.renderDashboard(w, r, "Could not refresh live OpenAI pricing: "+err.Error()+". Continuing with saved pricing.", "")
		return
	}
	s.renderDashboard(w, r, fmt.Sprintf("Pricing refreshed: %d models.", count), "")
}

func (s *Server) pricingPage(w http.ResponseWriter, r *http.Request) {
	s.renderPricing(w, r, "", "")
}

func (s *Server) addPricing(w http.ResponseWriter, r *http.Request) {
	price, err := pricingFromForm(r, 0)
	if err != nil {
		s.renderPricing(w, r, "", err.Error())
		return
	}
	if err := s.store.UpsertPricing(r.Context(), price); err != nil {
		s.renderPricing(w, r, "", "Could not save pricing option.")
		return
	}
	s.renderPricing(w, r, "Pricing option saved.", "")
}

func (s *Server) updatePricing(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pricing/")
	idText, action, ok := strings.Cut(path, "/")
	if !ok {
		s.renderPricing(w, r, "", "Unknown pricing action.")
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		s.renderPricing(w, r, "", "Invalid pricing option.")
		return
	}
	switch action {
	case "update":
		price, err := pricingFromForm(r, id)
		if err != nil {
			s.renderPricing(w, r, "", err.Error())
			return
		}
		if err := s.store.UpdatePricing(r.Context(), price); err != nil {
			s.renderPricing(w, r, "", "Could not update pricing option.")
			return
		}
		s.renderPricing(w, r, "Pricing option updated.", "")
	case "delete":
		if err := s.store.DeletePricing(r.Context(), id); err != nil {
			s.renderPricing(w, r, "", "Could not delete pricing option.")
			return
		}
		s.renderPricing(w, r, "Pricing option deleted.", "")
	default:
		s.renderPricing(w, r, "", "Unknown pricing action.")
	}
}

func (s *Server) addToolPricing(w http.ResponseWriter, r *http.Request) {
	price, err := toolPricingFromForm(r, 0)
	if err != nil {
		s.renderPricing(w, r, "", err.Error())
		return
	}
	if err := s.store.UpsertToolPricing(r.Context(), price); err != nil {
		s.renderPricing(w, r, "", "Could not save tool pricing option.")
		return
	}
	s.renderPricing(w, r, "Tool pricing option saved.", "")
}

func (s *Server) updateToolPricing(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pricing/tools/")
	idText, action, ok := strings.Cut(path, "/")
	if !ok {
		s.renderPricing(w, r, "", "Unknown tool pricing action.")
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		s.renderPricing(w, r, "", "Invalid tool pricing option.")
		return
	}
	switch action {
	case "update":
		price, err := toolPricingFromForm(r, id)
		if err != nil {
			s.renderPricing(w, r, "", err.Error())
			return
		}
		if err := s.store.UpdateToolPricing(r.Context(), price); err != nil {
			s.renderPricing(w, r, "", "Could not update tool pricing option.")
			return
		}
		s.renderPricing(w, r, "Tool pricing option updated.", "")
	case "delete":
		if err := s.store.DeleteToolPricing(r.Context(), id); err != nil {
			s.renderPricing(w, r, "", "Could not delete tool pricing option.")
			return
		}
		s.renderPricing(w, r, "Tool pricing option deleted.", "")
	default:
		s.renderPricing(w, r, "", "Unknown tool pricing action.")
	}
}

func (s *Server) renderPricing(w http.ResponseWriter, r *http.Request, message, renderErr string) {
	prices, err := s.store.Pricing(r.Context())
	if err != nil {
		s.logger.Warn("load pricing", "error", err)
		renderErr = "Could not load pricing options."
	}
	toolPrices, err := s.store.ToolPricing(r.Context())
	if err != nil {
		s.logger.Warn("load tool pricing", "error", err)
		renderErr = "Could not load tool pricing options."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "pricing", pricingView{Prices: prices, ToolPrices: toolPrices, Message: message, Error: renderErr}); err != nil {
		s.logger.Warn("render pricing", "error", err)
	}
}

func pricingFromForm(r *http.Request, id int64) (store.PricingSnapshot, error) {
	if err := r.ParseForm(); err != nil {
		return store.PricingSnapshot{}, fmt.Errorf("could not read form data")
	}
	input, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("input_per_million")), 64)
	if err != nil || input <= 0 {
		return store.PricingSnapshot{}, fmt.Errorf("input price is required")
	}
	cachedText := strings.TrimSpace(r.FormValue("cached_input_per_million"))
	cached := input
	if cachedText != "" {
		cached, err = strconv.ParseFloat(cachedText, 64)
		if err != nil || cached < 0 {
			return store.PricingSnapshot{}, fmt.Errorf("cached input price is invalid")
		}
	}
	output, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("output_per_million")), 64)
	if err != nil || output <= 0 {
		return store.PricingSnapshot{}, fmt.Errorf("output price is required")
	}
	model := strings.ToLower(strings.TrimSpace(r.FormValue("model")))
	if model == "" {
		return store.PricingSnapshot{}, fmt.Errorf("model is required")
	}
	source := strings.TrimSpace(r.FormValue("source_url"))
	if source == "" {
		source = "manual"
	}
	return store.PricingSnapshot{
		ID:                    id,
		SourceURL:             source,
		FetchedAt:             time.Now().UTC(),
		Model:                 model,
		BillingTier:           strings.TrimSpace(r.FormValue("billing_tier")),
		ContextKind:           strings.TrimSpace(r.FormValue("context_kind")),
		InputPerMillion:       input,
		CachedInputPerMillion: cached,
		OutputPerMillion:      output,
	}, nil
}

func toolPricingFromForm(r *http.Request, id int64) (store.ToolPricingSnapshot, error) {
	if err := r.ParseForm(); err != nil {
		return store.ToolPricingSnapshot{}, fmt.Errorf("could not read form data")
	}
	key := strings.ToLower(strings.TrimSpace(r.FormValue("tool_key")))
	if key == "" {
		return store.ToolPricingSnapshot{}, fmt.Errorf("tool key is required")
	}
	name := strings.TrimSpace(r.FormValue("display_name"))
	if name == "" {
		return store.ToolPricingSnapshot{}, fmt.Errorf("tool name is required")
	}
	unitLabel := strings.TrimSpace(r.FormValue("unit_label"))
	if unitLabel == "" {
		return store.ToolPricingSnapshot{}, fmt.Errorf("unit label is required")
	}
	unitSize, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("unit_size")), 64)
	if err != nil || unitSize <= 0 {
		return store.ToolPricingSnapshot{}, fmt.Errorf("unit size is required")
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("price_per_unit")), 64)
	if err != nil || price < 0 {
		return store.ToolPricingSnapshot{}, fmt.Errorf("price is required")
	}
	source := strings.TrimSpace(r.FormValue("source_url"))
	if source == "" {
		source = "manual"
	}
	return store.ToolPricingSnapshot{
		ID:           id,
		SourceURL:    source,
		FetchedAt:    time.Now().UTC(),
		ToolKey:      key,
		DisplayName:  name,
		UnitLabel:    unitLabel,
		UnitSize:     unitSize,
		PricePerUnit: price,
	}, nil
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	session, events, toolEvents, err := s.store.Session(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Could not load session.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "session", sessionView{Session: session, Events: events, ToolEvents: toolEvents}); err != nil {
		s.logger.Warn("render session", "error", err)
	}
}

func (s *Server) renderDashboard(w http.ResponseWriter, r *http.Request, message, renderErr string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	now := time.Now()
	since := startOfDay(now).AddDate(0, 0, -(dashboardHistoryDays - 1))

	path, _, err := s.store.Setting(ctx, store.SettingCodexPath)
	if err != nil {
		renderErr = "Could not load Codex path."
	}
	daily, sessions, err := s.store.Dashboard(ctx)
	if err != nil {
		s.logger.Warn("load dashboard", "error", err)
		renderErr = "Could not load dashboard data."
	}
	tokenSeries, err := s.store.UsageTokenSeries(ctx, since)
	if err != nil {
		s.logger.Warn("load usage token series", "error", err)
		renderErr = "Could not load dashboard data."
	}
	creditSeries, err := s.store.UsageCreditSeries(ctx, since)
	if err != nil {
		s.logger.Warn("load usage credit series", "error", err)
		renderErr = "Could not load dashboard data."
	}
	refreshText := "never"
	if refreshed, ok, err := s.store.LatestPricingRefresh(ctx); err == nil && ok {
		refreshText = refreshed.Local().Format("2006-01-02 15:04:05")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	view := dashboardView{
		CodexPath:      path,
		PricingRefresh: refreshText,
		Totals:         buildPeriodTotals(daily, now),
		TokenChart:     buildTokenChart(tokenSeries, since, now),
		CreditChart:    buildCreditChart(creditSeries, since, now),
		Projects:       buildProjects(sessions),
		Message:        message,
		Error:          renderErr,
	}
	if err := s.tmpl.ExecuteTemplate(w, "dashboard", view); err != nil {
		s.logger.Warn("render dashboard", "error", err)
	}
}

func buildPeriodTotals(daily []store.DailySpend, now time.Time) periodTotals {
	loc := now.Location()
	today := startOfDay(now.In(loc))
	yesterday := today.AddDate(0, 0, -1)
	weekStart := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)

	var totals periodTotals
	for _, spend := range daily {
		day, err := time.ParseInLocation("2006-01-02", spend.Day, loc)
		if err != nil {
			continue
		}
		switch {
		case day.Equal(today):
			totals.Today += spend.TotalCost
		case day.Equal(yesterday):
			totals.Yesterday += spend.TotalCost
		}
		if !day.Before(weekStart) && !day.After(today) {
			totals.WeekToDate += spend.TotalCost
		}
		if !day.Before(monthStart) && !day.After(today) {
			totals.MonthToDate += spend.TotalCost
		}
	}
	return totals
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func buildProjects(sessions []store.SessionSummary) []projectView {
	byPath := map[string]*projectView{}
	for _, session := range sessions {
		path := strings.TrimSpace(session.CWD)
		if path == "" {
			path = "(unknown project)"
		}
		project := byPath[path]
		if project == nil {
			project = &projectView{
				Name: projectName(path),
				Path: path,
			}
			byPath[path] = project
		}
		project.SessionCount++
		project.ToolCalls += session.ToolCalls
		project.TotalTokens += session.TotalTokens
		project.TotalCost += session.TotalCost
		project.UnpricedEvents += session.UnpricedEvents
		project.Sessions = append(project.Sessions, session)
	}

	projects := make([]projectView, 0, len(byPath))
	for _, project := range byPath {
		sort.SliceStable(project.Sessions, func(i, j int) bool {
			return project.Sessions[i].StartedAt.After(project.Sessions[j].StartedAt)
		})
		projects = append(projects, *project)
	}
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].TotalCost == projects[j].TotalCost {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].TotalCost > projects[j].TotalCost
	})
	return projects
}

func projectName(path string) string {
	if path == "(unknown project)" {
		return path
	}
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return path
	}
	return name
}

func buildTokenChart(points []store.UsageTokenSeriesPoint, since, now time.Time) dashboardChart {
	labels := buildChartLabels(since, now)
	indexByDay := make(map[string]int, len(labels))
	for i, label := range labels {
		indexByDay[label] = i
	}

	type tokenSeriesData struct {
		total    []float64
		uncached []float64
		cached   []float64
		output   []float64
	}

	byModel := map[string]*tokenSeriesData{}
	modelTotals := map[string]float64{}
	for _, point := range points {
		index, ok := indexByDay[point.Day]
		if !ok {
			continue
		}
		model := byModel[point.Model]
		if model == nil {
			model = &tokenSeriesData{
				total:    make([]float64, len(labels)),
				uncached: make([]float64, len(labels)),
				cached:   make([]float64, len(labels)),
				output:   make([]float64, len(labels)),
			}
			byModel[point.Model] = model
		}
		uncached := float64(point.UncachedInputTokens)
		cached := float64(point.CachedInputTokens)
		output := float64(point.OutputTokens)
		total := uncached + cached + output
		model.total[index] += total
		model.uncached[index] += uncached
		model.cached[index] += cached
		model.output[index] += output
		modelTotals[point.Model] += total
	}

	models := sortedModelsByTotal(modelTotals)
	payload := dashboardChartPayload{
		ValueKind: "tokens",
		Labels:    labels,
	}
	for _, model := range models {
		data := byModel[model]
		if data == nil {
			continue
		}
		totalSeries := dashboardSeries{
			Key:    model,
			Label:  model,
			Color:  modelColorVariant(model, "total"),
			Values: data.total,
		}
		variants := []struct {
			key    string
			label  string
			color  string
			values []float64
		}{
			{
				key:    model + ":uncached_input",
				label:  model + " uncached",
				color:  modelColorVariant(model, "uncached"),
				values: data.uncached,
			},
			{
				key:    model + ":cached_input",
				label:  model + " cached",
				color:  modelColorVariant(model, "cached"),
				values: data.cached,
			},
			{
				key:    model + ":output",
				label:  model + " output",
				color:  modelColorVariant(model, "output"),
				values: data.output,
			},
		}
		if seriesTotal(totalSeries.Values) > 0 {
			payload.Series = append(payload.Series, totalSeries)
		}
		for _, variant := range variants {
			series := dashboardSeries{
				Key:    variant.key,
				Label:  variant.label,
				Color:  variant.color,
				Values: variant.values,
			}
			if seriesTotal(series.Values) == 0 {
				continue
			}
			payload.AlternateSeries = append(payload.AlternateSeries, series)
		}
	}

	return dashboardChart{
		ID:       "token-history-chart",
		DataID:   "token-history-chart-data",
		Title:    "Tokens by model",
		Subtitle: "Last 30 local days",
		Empty:    len(payload.Series) == 0,
		DataJSON: mustChartJSON(payload),
	}
}

func buildCreditChart(points []store.UsageCreditSeriesPoint, since, now time.Time) dashboardChart {
	labels := buildChartLabels(since, now)
	indexByDay := make(map[string]int, len(labels))
	for i, label := range labels {
		indexByDay[label] = i
	}

	byModel := map[string]float64{}
	for _, point := range points {
		byModel[point.Model] += point.Cost
	}

	models := sortedModelsByTotal(byModel)

	payload := dashboardChartPayload{
		ValueKind: "credits",
		Labels:    labels,
	}
	for _, model := range models {
		color := modelColorVariant(model, "credit")
		series := dashboardSeries{
			Key:    model,
			Label:  model,
			Color:  color,
			Values: make([]float64, len(labels)),
		}
		for _, point := range points {
			if point.Model != model {
				continue
			}
			index, ok := indexByDay[point.Day]
			if !ok {
				continue
			}
			series.Values[index] += point.Cost / dollarsPerCredit
		}
		if seriesTotal(series.Values) == 0 {
			continue
		}
		payload.Series = append(payload.Series, series)
	}

	return dashboardChart{
		ID:       "credit-history-chart",
		DataID:   "credit-history-chart-data",
		Title:    "Credits by model",
		Subtitle: "Last 30 local days",
		Empty:    len(payload.Series) == 0,
		DataJSON: mustChartJSON(payload),
	}
}

func buildChartLabels(since, now time.Time) []string {
	start := startOfDay(since)
	end := startOfDay(now)
	labels := make([]string, 0, dashboardHistoryDays)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		labels = append(labels, day.Format("2006-01-02"))
	}
	return labels
}

func mustChartJSON(payload dashboardChartPayload) template.JS {
	body, err := json.Marshal(payload)
	if err != nil {
		return template.JS(`{"valueKind":"tokens","labels":[],"series":[]}`)
	}
	return template.JS(body)
}

func sortedModelsByTotal(values map[string]float64) []string {
	models := make([]string, 0, len(values))
	for model, total := range values {
		if total <= 0 {
			continue
		}
		models = append(models, model)
	}
	sort.SliceStable(models, func(i, j int) bool {
		left := values[models[i]]
		right := values[models[j]]
		if left == right {
			return models[i] < models[j]
		}
		return left > right
	})
	return models
}

func seriesTotal(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func modelColorVariant(model, kind string) string {
	h := modelHue(model)
	switch kind {
	case "total":
		return fmt.Sprintf("hsl(%d 58%% 72%%)", h)
	case "cached":
		return fmt.Sprintf("hsl(%d 42%% 82%%)", h)
	case "output":
		return fmt.Sprintf("hsl(%d 64%% 66%%)", h)
	case "credit":
		return fmt.Sprintf("hsl(%d 54%% 76%%)", h)
	default:
		return fmt.Sprintf("hsl(%d 50%% 78%%)", h)
	}
}

func modelHue(model string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(strings.TrimSpace(model))))
	return hash.Sum32() % 360
}
