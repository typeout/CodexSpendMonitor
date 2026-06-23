package pricing

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"codexspendmonitor/internal/store"
)

const OpenAIPricingURL = "https://openai.com/api/pricing/"

type Service struct {
	store  *store.Store
	logger *slog.Logger
	client *http.Client
}

func NewService(store *store.Store, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		logger: logger,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func FallbackPrices() []store.PricingSnapshot {
	fetchedAt := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	return []store.PricingSnapshot{
		price("seed", fetchedAt, "gpt-5.5", "standard", "short", 5.00, 0.50, 30.00),
		price("seed", fetchedAt, "gpt-5.5", "standard", "long", 10.00, 1.00, 45.00),
		price("seed", fetchedAt, "gpt-5.5-pro", "standard", "short", 30.00, 30.00, 180.00),
		price("seed", fetchedAt, "gpt-5.5-pro", "standard", "long", 60.00, 60.00, 270.00),
		price("seed", fetchedAt, "gpt-5.4", "standard", "short", 2.50, 0.25, 15.00),
		price("seed", fetchedAt, "gpt-5.4", "standard", "long", 5.00, 0.50, 22.50),
		price("seed", fetchedAt, "gpt-5.4-mini", "standard", "short", 0.75, 0.075, 4.50),
		price("seed", fetchedAt, "gpt-5.4-nano", "standard", "short", 0.20, 0.02, 1.25),
		price("seed", fetchedAt, "gpt-5.4-pro", "standard", "short", 30.00, 30.00, 180.00),
		price("seed", fetchedAt, "gpt-5.4-pro", "standard", "long", 60.00, 60.00, 270.00),
		price("seed", fetchedAt, "gpt-5.5", "batch", "short", 2.50, 0.25, 15.00),
		price("seed", fetchedAt, "gpt-5.5", "batch", "long", 5.00, 0.50, 22.50),
		price("seed", fetchedAt, "gpt-5.5-pro", "batch", "short", 15.00, 15.00, 90.00),
		price("seed", fetchedAt, "gpt-5.4", "batch", "short", 1.25, 0.13, 7.50),
		price("seed", fetchedAt, "gpt-5.4", "batch", "long", 2.50, 0.25, 11.25),
		price("seed", fetchedAt, "gpt-5.4-mini", "batch", "short", 0.375, 0.0375, 2.25),
		price("seed", fetchedAt, "gpt-5.4-nano", "batch", "short", 0.10, 0.01, 0.625),
		price("seed", fetchedAt, "gpt-5.4-pro", "batch", "short", 15.00, 15.00, 90.00),
		price("seed", fetchedAt, "gpt-5.4-pro", "batch", "long", 30.00, 30.00, 135.00),
	}
}

func FallbackToolPrices() []store.ToolPricingSnapshot {
	fetchedAt := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	return []store.ToolPricingSnapshot{
		toolPrice("seed", fetchedAt, "web_search", "Web search", "1k calls", 1000, 10.00),
		toolPrice("seed", fetchedAt, "image_web_search", "Image web search", "1k calls", 1000, 10.00),
		toolPrice("seed", fetchedAt, "web_search_preview_reasoning", "Web search preview (reasoning)", "1k calls", 1000, 10.00),
		toolPrice("seed", fetchedAt, "web_search_preview_non_reasoning", "Web search preview (non-reasoning)", "1k calls", 1000, 25.00),
		toolPrice("seed", fetchedAt, "container_session", "Hosted shell and code interpreter", "20-minute sessions", 1, 0.03),
		toolPrice("seed", fetchedAt, "container_session_4gb", "Hosted shell and code interpreter (4 GB)", "20-minute sessions", 1, 0.12),
		toolPrice("seed", fetchedAt, "container_session_16gb", "Hosted shell and code interpreter (16 GB)", "20-minute sessions", 1, 0.48),
		toolPrice("seed", fetchedAt, "container_session_64gb", "Hosted shell and code interpreter (64 GB)", "20-minute sessions", 1, 1.92),
		toolPrice("seed", fetchedAt, "file_search_storage", "File search storage", "GB-days", 1, 0.10),
		toolPrice("seed", fetchedAt, "file_search_tool_call", "File search tool call", "1k calls", 1000, 2.50),
		toolPrice("seed", fetchedAt, "agentkit_storage", "AgentKit file and image upload storage", "GB-days", 1, 0.10),
	}
}

func (s *Service) Refresh(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OpenAIPricingURL, nil)
	if err != nil {
		return 0, fmt.Errorf("building pricing request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetching OpenAI pricing: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("fetching OpenAI pricing: status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return 0, fmt.Errorf("reading pricing response: %w", err)
	}

	prices := ParseOpenAIPricing(string(body), time.Now().UTC())
	if len(prices) == 0 {
		return 0, fmt.Errorf("no model prices found on pricing page")
	}
	for _, item := range prices {
		if err := s.store.UpsertPricing(ctx, item); err != nil {
			return 0, err
		}
	}
	return len(prices), nil
}

func ParseOpenAIPricing(page string, fetchedAt time.Time) []store.PricingSnapshot {
	modelRe := regexp.MustCompile(`(?is)##\s*(GPT-[^<\n]+).*?Input:\$([0-9.]+)\s*/\s*1M tokens\s*Cached input:\$([0-9.]+)\s*/\s*1M tokens\s*Output:\$([0-9.]+)\s*/\s*1M tokens`)
	matches := modelRe.FindAllStringSubmatch(page, -1)
	prices := make([]store.PricingSnapshot, 0, len(matches))
	for _, match := range matches {
		inputRate, inputErr := strconv.ParseFloat(match[2], 64)
		cachedRate, cachedErr := strconv.ParseFloat(match[3], 64)
		outputRate, outputErr := strconv.ParseFloat(match[4], 64)
		if inputErr != nil || cachedErr != nil || outputErr != nil {
			continue
		}
		model := strings.ToLower(strings.TrimSpace(match[1]))
		prices = append(prices, price(OpenAIPricingURL, fetchedAt, model, "standard", "short", inputRate, cachedRate, outputRate))
		if strings.Contains(model, " ") {
			prices = append(prices, price(OpenAIPricingURL, fetchedAt, strings.ReplaceAll(model, " ", "-"), "standard", "short", inputRate, cachedRate, outputRate))
		}
	}
	return prices
}

func price(source string, fetchedAt time.Time, model, tier, contextKind string, input, cached, output float64) store.PricingSnapshot {
	return store.PricingSnapshot{
		SourceURL:             source,
		FetchedAt:             fetchedAt,
		Model:                 strings.ToLower(model),
		BillingTier:           tier,
		ContextKind:           contextKind,
		InputPerMillion:       input,
		CachedInputPerMillion: cached,
		OutputPerMillion:      output,
	}
}

func toolPrice(source string, fetchedAt time.Time, key, displayName, unitLabel string, unitSize, pricePerUnit float64) store.ToolPricingSnapshot {
	return store.ToolPricingSnapshot{
		SourceURL:    source,
		FetchedAt:    fetchedAt,
		ToolKey:      strings.ToLower(key),
		DisplayName:  displayName,
		UnitLabel:    unitLabel,
		UnitSize:     unitSize,
		PricePerUnit: pricePerUnit,
	}
}
