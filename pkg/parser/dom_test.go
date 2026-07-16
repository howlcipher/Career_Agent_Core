package parser

import (
	"strings"
	"testing"
)

func TestPruneDOM(t *testing.T) {
	input := `<html><head><script>alert(1);</script><style>body{}</style></head><body><div>Hello</div></body></html>`
	
	output, err := PruneDOM(input)
	if err != nil {
		t.Fatalf("PruneDOM error: %v", err)
	}
	
	if !strings.Contains(output, "<div>Hello</div>") {
		t.Errorf("expected output to contain div, got: %s", output)
	}
	if strings.Contains(output, "<script>") || strings.Contains(output, "<style>") {
		t.Errorf("expected script and style to be removed, got: %s", output)
	}
}

func TestPruneDOMToText(t *testing.T) {
	input := `<html>
		<head><title>Test</title></head>
		<body>
			<nav>Skip me</nav>
			<script>var x = 1;</script>
			<div>
				Hello <span>World!</span>
			</div>
			<footer>Skip this too</footer>
		</body>
	</html>`
	
	output, err := PruneDOMToText(input)
	if err != nil {
		t.Fatalf("PruneDOMToText error: %v", err)
	}
	
	if output != "Hello World!" {
		t.Errorf("expected 'Hello World!', got %q", output)
	}
}
