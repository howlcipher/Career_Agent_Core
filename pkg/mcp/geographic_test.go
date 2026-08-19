package mcp

import "testing"

func TestIsForeignLocationRestricted(t *testing.T) {
	tests := []struct {
		title    string
		expected bool
	}{
		{"Site Reliability Engineer - Remote from Romania or Hungary", true},
		{"Senior Go Backend Engineer (Remote in UK)", true},
		{"DevOps Platform Engineer - Remote - EMEA", true},
		{"Senior Software Engineer - Remote (US)", false},
		{"Full Stack Developer - Remote", false},
		{"Site Reliability Engineer - Remote from Poland", true},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := isForeignLocationRestricted(tt.title)
			if got != tt.expected {
				t.Errorf("isForeignLocationRestricted(%q) = %v; want %v", tt.title, got, tt.expected)
			}
		})
	}
}
