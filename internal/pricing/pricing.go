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
		price("seed", fetchedAt, "gpt-5.5", 5.00, 0.50, 30.00),
		price("seed", fetchedAt, "gpt-5.4", 2.50, 0.25, 15.00),
		price("seed", fetchedAt, "gpt-5.4 mini", 0.75, 0.075, 4.50),
		price("seed", fetchedAt, "gpt-5.4-mini", 0.75, 0.075, 4.50),
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
		prices = append(prices, price(OpenAIPricingURL, fetchedAt, model, inputRate, cachedRate, outputRate))
		if strings.Contains(model, " ") {
			prices = append(prices, price(OpenAIPricingURL, fetchedAt, strings.ReplaceAll(model, " ", "-"), inputRate, cachedRate, outputRate))
		}
	}
	return prices
}

func price(source string, fetchedAt time.Time, model string, input, cached, output float64) store.PricingSnapshot {
	return store.PricingSnapshot{
		SourceURL:             source,
		FetchedAt:             fetchedAt,
		Model:                 strings.ToLower(model),
		InputPerMillion:       input,
		CachedInputPerMillion: cached,
		OutputPerMillion:      output,
	}
}
