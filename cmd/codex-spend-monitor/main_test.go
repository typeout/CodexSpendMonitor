package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIsCodexSpendMonitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response *http.Response
		expected bool
	}{
		{
			name: "header marker",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X-Codex-Spend-Monitor": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader("")),
			},
			expected: true,
		},
		{
			name: "dashboard title fallback",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("<title>Codex Spend Monitor</title>")),
			},
			expected: true,
		},
		{
			name: "unrelated page",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("<title>Other App</title>")),
			},
			expected: false,
		},
		{
			name: "not found",
			response: &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"X-Codex-Spend-Monitor": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader("<title>Codex Spend Monitor</title>")),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isCodexSpendMonitor(tt.response)
			if got != tt.expected {
				t.Fatalf("isCodexSpendMonitor() = %v, want %v", got, tt.expected)
			}
		})
	}
}
