package store

import "testing"

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
