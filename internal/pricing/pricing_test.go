package pricing

import (
	"testing"
	"time"
)

func TestParseOpenAIPricing(t *testing.T) {
	t.Parallel()

	page := `## GPT-5.5
Input:$5.00 / 1M tokens Cached input:$0.50 / 1M tokens Output:$30.00 / 1M tokens
## GPT-5.4 mini
Input:$0.75 / 1M tokens Cached input:$0.075 / 1M tokens Output:$4.50 / 1M tokens
## GPT-5.4
Input:$2.50 / 1M tokens Cached input:$0.25 / 1M tokens Output:$15.00 / 1M tokens`

	got := ParseOpenAIPricing(page, time.Unix(0, 0).UTC())
	if len(got) != 4 {
		t.Fatalf("len(ParseOpenAIPricing()) = %d, want 4", len(got))
	}
	if got[0].Model != "gpt-5.5" || got[0].InputPerMillion != 5 || got[0].CachedInputPerMillion != 0.5 || got[0].OutputPerMillion != 30 {
		t.Fatalf("first parsed price = %+v", got[0])
	}
	if got[2].Model != "gpt-5.4-mini" {
		t.Fatalf("hyphenated alias = %q, want gpt-5.4-mini", got[2].Model)
	}
}
