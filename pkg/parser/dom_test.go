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

func TestStripPresentationalAttrs_RemovesStylingAndStateAttrs(t *testing.T) {
	input := `<input id="first_name" name="first_name" type="text" class="input input__single-line" style="color:red" role="textbox" tabindex="0" autocomplete="given-name" spellcheck="false" inputmode="text" aria-describedby="first_name-description" aria-hidden="false" aria-invalid="false" aria-required="true" />`

	output, err := StripPresentationalAttrs(input)
	if err != nil {
		t.Fatalf("StripPresentationalAttrs error: %v", err)
	}

	for _, attr := range []string{"class=", "style=", "role=", "tabindex=", "autocomplete=", "spellcheck=", "inputmode=", "aria-describedby=", "aria-hidden=", "aria-invalid=", "aria-required="} {
		if strings.Contains(output, attr) {
			t.Errorf("expected %s to be stripped, got: %s", attr, output)
		}
	}
	if !strings.Contains(output, `id="first_name"`) || !strings.Contains(output, `name="first_name"`) || !strings.Contains(output, `type="text"`) {
		t.Errorf("expected selector-relevant attributes to survive, got: %s", output)
	}
}

// TestStripPresentationalAttrs_KeepsAriaLabel is the exact correctness
// requirement this fix depends on: aria-label/aria-labelledby are the
// fallback label source ExtractFormMapping and SolveValidationErrors both
// rely on for fields with no <label for> association, so stripping them
// alongside the other aria-* noise would silently break that fallback.
func TestStripPresentationalAttrs_KeepsAriaLabel(t *testing.T) {
	input := `<input id="q1" aria-label="Why do you want to work here?" aria-labelledby="q1-label" aria-hidden="false" />`

	output, err := StripPresentationalAttrs(input)
	if err != nil {
		t.Fatalf("StripPresentationalAttrs error: %v", err)
	}

	if !strings.Contains(output, `aria-label="Why do you want to work here?"`) {
		t.Errorf("expected aria-label to survive, got: %s", output)
	}
	if !strings.Contains(output, `aria-labelledby="q1-label"`) {
		t.Errorf("expected aria-labelledby to survive, got: %s", output)
	}
	if strings.Contains(output, "aria-hidden") {
		t.Errorf("expected aria-hidden to still be stripped, got: %s", output)
	}
}

// TestStripPresentationalAttrs_MeaningfullyReducesSize is the live-repro
// shape from bugs.md #52's Reddit/Greenhouse recurrence: a genuinely large,
// custom-question-heavy form can still exceed the 50k-char circuit breaker
// even after PruneDOMToForm's element-level scoping, because modern ATS
// themes wrap every field in several layers of styling divs and
// accessibility attributes.
func TestStripPresentationalAttrs_MeaningfullyReducesSize(t *testing.T) {
	input := strings.Repeat(`<div class="field-wrapper"><div class="text-input-wrapper"><div class="input-wrapper input-wrapper--active"><label id="q-label" for="q" class="label label">Question<span aria-hidden="true">*</span></label><input id="q" class="input input__single-line" aria-label="Question" aria-describedby="q-description" role="textbox" tabindex="0" /></div></div></div>`, 20)

	output, err := StripPresentationalAttrs(input)
	if err != nil {
		t.Fatalf("StripPresentationalAttrs error: %v", err)
	}

	if len(output) >= len(input) {
		t.Errorf("expected a meaningful size reduction, got %d bytes from %d input bytes", len(output), len(input))
	}
	if reduction := 1 - float64(len(output))/float64(len(input)); reduction < 0.3 {
		t.Errorf("expected at least a 30%% reduction, got %.0f%% (%d -> %d bytes)", reduction*100, len(input), len(output))
	}
}
