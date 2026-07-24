package main

import (
	"reflect"
	"testing"
)

func TestParseTargetJobURLs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]bool
	}{
		{
			name: "empty input returns nil, not an empty map",
			raw:  "",
			want: nil,
		},
		{
			name: "single URL, backward compatible with the original TARGET_JOB_URL usage",
			raw:  "https://jobs.lever.co/acme/abc-123",
			want: map[string]bool{"https://jobs.lever.co/acme/abc-123": true},
		},
		{
			name: "comma-separated list with surrounding whitespace trimmed",
			raw:  " https://jobs.lever.co/a , https://jobs.lever.co/b ,https://jobs.lever.co/c",
			want: map[string]bool{
				"https://jobs.lever.co/a": true,
				"https://jobs.lever.co/b": true,
				"https://jobs.lever.co/c": true,
			},
		},
		{
			name: "empty entries from trailing/double commas are dropped",
			raw:  "https://jobs.lever.co/a,,https://jobs.lever.co/b,",
			want: map[string]bool{
				"https://jobs.lever.co/a": true,
				"https://jobs.lever.co/b": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTargetJobURLs(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTargetJobURLs(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
