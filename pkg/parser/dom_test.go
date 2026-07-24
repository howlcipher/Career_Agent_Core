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

func TestPruneDOMResilience(t *testing.T) {
	input := `<html><body><div>Hello</div><svg><path d="M0 0"/></svg><iframe src="evil.com"></iframe><script>alert(1);</body></html>`
	
	output, err := PruneDOM(input)
	if err != nil {
		t.Fatalf("PruneDOM error: %v", err)
	}
	
	if !strings.Contains(output, "<div>Hello</div>") {
		t.Errorf("expected output to contain div, got: %s", output)
	}
	if strings.Contains(output, "<script>") {
		t.Errorf("expected script to be removed, got: %s", output)
	}
	if strings.Contains(output, "<svg>") || strings.Contains(output, "<path>") {
		t.Errorf("expected svg to be removed, got: %s", output)
	}
	if strings.Contains(output, "<iframe") {
		t.Errorf("expected iframe to be removed, got: %s", output)
	}
}

func TestPruneDOMToForm_ScopesDownToFormWhenPresent(t *testing.T) {
	input := `<html><body>
		<nav>Skip this whole nav bar</nav>
		<div class="marketing">Lots of unrelated marketing copy here</div>
		<form id="application-form">
			<input name="first_name" />
			<input name="last_name" />
		</form>
		<footer>Skip this footer too</footer>
	</body></html>`

	output, err := PruneDOMToForm(input)
	if err != nil {
		t.Fatalf("PruneDOMToForm error: %v", err)
	}

	if !strings.Contains(output, `name="first_name"`) {
		t.Errorf("expected output to contain the form's fields, got: %s", output)
	}
	if strings.Contains(output, "marketing copy") {
		t.Errorf("expected output to exclude content outside the form, got: %s", output)
	}
	if strings.Contains(output, "Skip this whole nav bar") || strings.Contains(output, "Skip this footer too") {
		t.Errorf("expected output to exclude nav/footer content, got: %s", output)
	}
}

func TestPruneDOMToForm_FallsBackToFullDocumentWhenNoFormTag(t *testing.T) {
	input := `<html><body>
		<div class="fake-form">
			<input name="first_name" />
		</div>
	</body></html>`

	output, err := PruneDOMToForm(input)
	if err != nil {
		t.Fatalf("PruneDOMToForm error: %v", err)
	}

	if !strings.Contains(output, `name="first_name"`) {
		t.Errorf("expected fallback output to still contain the field, got: %s", output)
	}
}
