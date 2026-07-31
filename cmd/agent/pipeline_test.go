package main

import (
	"errors"
	"testing"
)

// classifyGenerationError draws the line bugs.md #444 is about: only a
// genuine hard quota ("Quota exceeded") may cancel the whole batch. A bare
// "429" -- Anthropic's ordinary per-minute rate limit, and also what
// Gemini's SDK returns for its per-minute limit, not just its daily one --
// must be retried like any other transient network condition instead.
func TestClassifyGenerationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want genErrorClass
	}{
		{
			name: "gemini hard daily quota is fatal",
			err:  errors.New("failed to generate content from gemini: googleapi: Error 429: Quota exceeded for quota metric 'Generate Content API requests'"),
			want: genErrorFatalQuota,
		},
		{
			name: "bare 429 on claude is retryable, not fatal",
			err:  errors.New("claude request failed: 429 Too Many Requests"),
			want: genErrorRetryable,
		},
		{
			name: "bare 429 with no provider context is retryable, not fatal",
			err:  errors.New("unexpected status code 429"),
			want: genErrorRetryable,
		},
		{
			name: "connection refused is retryable",
			err:  errors.New("dial tcp: connect: connection refused"),
			want: genErrorRetryable,
		},
		{
			name: "no route to host is retryable",
			err:  errors.New("dial tcp: no route to host"),
			want: genErrorRetryable,
		},
		{
			name: "deadline exceeded is retryable",
			err:  errors.New("context deadline exceeded"),
			want: genErrorRetryable,
		},
		{
			name: "unrelated error is terminal",
			err:  errors.New("failed to parse json response: unexpected end of JSON input"),
			want: genErrorTerminal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyGenerationError(tc.err)
			if got != tc.want {
				t.Errorf("classifyGenerationError(%q) = %v, want %v", tc.err.Error(), got, tc.want)
			}
		})
	}
}
