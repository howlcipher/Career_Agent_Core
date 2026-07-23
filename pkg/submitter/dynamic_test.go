package submitter

import (
	"testing"
)

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://lever.co/jobs/123", "lever.co/jobs"},
		{"https://boards.greenhouse.io/company/jobs/456", "boards.greenhouse.io/company"},
		{"https://linkedin.com/jobs/view/789", "linkedin.com/jobs"},
		{"http://example.com", "example.com"},
		{"invalid-url", "invalid-url"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := ExtractDomain(tt.url)
			if got != tt.expected {
				t.Errorf("ExtractDomain(%q) = %q; want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestTemplateMatchingLoop(t *testing.T) {
	p := NewPipeline(nil, nil, nil, nil)
	
	// Test matching a known template
	jobURL := "https://lever.co/jobs/123"
	domHTML := "<html><body>Welcome to lever.co careers</body></html>"
	
	templateName, err := p.TemplateMatchingLoop(jobURL, domHTML)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if templateName != "LeverTemplate" {
		t.Errorf("Expected LeverTemplate, got %s", templateName)
	}
	
	// Test unknown template
	jobURLUnknown := "https://example.com/jobs/123"
	domHTMLUnknown := "<html><body>Welcome to unknown careers</body></html>"
	
	templateName, err = p.TemplateMatchingLoop(jobURLUnknown, domHTMLUnknown)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if templateName != "DynamicGeneratedScript" {
		t.Errorf("Expected DynamicGeneratedScript, got %s", templateName)
	}
}
