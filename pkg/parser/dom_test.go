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

	// aria-invalid was deliberately removed from this list in bugs.md #64: it
	// is the only attribute identifying which field a form rejected, and
	// stripping it forced SolveValidationErrors to re-send the entire form on
	// every retry. Its retention is pinned by
	// TestStripPresentationalAttrs_KeepsAriaInvalid.
	//
	// aria-describedby was likewise removed from this list in bugs.md #70: it
	// points at the element holding the page's own reason for rejecting the
	// field, which PruneDOMToInvalidFields now follows. Its retention is
	// pinned by TestPruneDOMToInvalidFields_KeepsAriaDescribedByErrorText.
	for _, attr := range []string{"class=", "style=", "role=", "tabindex=", "autocomplete=", "spellcheck=", "inputmode=", "aria-hidden=", "aria-required="} {
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

// bugs.md #64: the validation-retry payload must narrow to the rejected
// fields. A large ATS form is ~55k chars, which exceeded the practical
// inference budget on this hardware and made big forms fail on time.
func TestPruneDOMToInvalidFields(t *testing.T) {
	form := `<form>
		<label for="fn">First Name</label><input id="fn" name="first_name" value="Will">
		<label for="ln">Last Name</label><input id="ln" name="last_name" value="Elias">
		<label for="ph">Phone</label><input id="ph" name="phone" aria-invalid="true">
		<textarea id="why" name="why" data-invalid="true"></textarea>
		<select id="src" name="source"><option>LinkedIn</option></select>
	</form>`

	out, narrowed, err := PruneDOMToInvalidFields(form)
	if err != nil {
		t.Fatalf("PruneDOMToInvalidFields: %v", err)
	}
	if !narrowed {
		t.Fatal("expected the payload to be narrowed, got narrowed=false")
	}
	if !strings.Contains(out, `name="phone"`) || !strings.Contains(out, `name="why"`) {
		t.Errorf("both rejected fields must be kept, got: %s", out)
	}
	if !strings.Contains(out, "Phone") {
		t.Errorf("the rejected field's label must be kept so the model knows what it wants, got: %s", out)
	}
	// The whole point: passing fields must not be re-sent.
	if strings.Contains(out, `name="first_name"`) || strings.Contains(out, `name="source"`) {
		t.Errorf("fields that passed validation must be dropped, got: %s", out)
	}
	if len(out) >= len(form) {
		t.Errorf("narrowed payload (%d) should be smaller than the form (%d)", len(out), len(form))
	}
}

// An unreadable theme must fall back to sending the whole form. Sending
// nothing would guarantee the retry fixes nothing.
func TestPruneDOMToInvalidFields_NoMarkersFallsBackToFullForm(t *testing.T) {
	form := `<form><input id="fn" name="first_name"><input id="ph" name="phone"></form>`
	out, narrowed, err := PruneDOMToInvalidFields(form)
	if err != nil {
		t.Fatalf("PruneDOMToInvalidFields: %v", err)
	}
	if narrowed {
		t.Error("expected narrowed=false when no invalid marker is present")
	}
	if out != form {
		t.Errorf("expected the untouched form back, got: %s", out)
	}
}

// aria-invalid must survive attribute stripping, or #64's narrowing has no
// signal left to work with.
func TestStripPresentationalAttrs_KeepsAriaInvalid(t *testing.T) {
	in := `<form><input id="ph" name="phone" class="err ring" aria-invalid="true" aria-hidden="false"></form>`
	out, err := StripPresentationalAttrs(in)
	if err != nil {
		t.Fatalf("StripPresentationalAttrs: %v", err)
	}
	if !strings.Contains(out, `aria-invalid="true"`) {
		t.Errorf("aria-invalid must be preserved — it is the only signal of which field failed. got: %s", out)
	}
	if strings.Contains(out, "class=") {
		t.Errorf("presentational attrs should still be stripped, got: %s", out)
	}
}

// bugs.md #70: the narrowed retry payload kept the rejected control and its
// label but dropped the page's own error text, which is the only thing that
// says *why* the field bounced. "Phone" plus aria-invalid tells the model the
// field is wrong; "Please enter a valid phone number, e.g. +1 555 555 5555"
// tells it what to actually write.
func TestPruneDOMToInvalidFields_KeepsAriaDescribedByErrorText(t *testing.T) {
	form := `<form>
		<label for="ph">Phone</label>
		<input id="ph" name="phone" aria-invalid="true" aria-describedby="ph-err">
		<div id="ph-err">Please enter a valid phone number</div>
		<label for="fn">First Name</label><input id="fn" name="first_name" value="Will">
	</form>`

	out, narrowed, err := PruneDOMToInvalidFields(form)
	if err != nil {
		t.Fatalf("PruneDOMToInvalidFields: %v", err)
	}
	if !narrowed {
		t.Fatal("expected the payload to be narrowed, got narrowed=false")
	}
	if !strings.Contains(out, "Please enter a valid phone number") {
		t.Errorf("the page's own error text must be kept — it is the only statement of why the field was rejected. got: %s", out)
	}
	if strings.Contains(out, `name="first_name"`) {
		t.Errorf("fields that passed validation must still be dropped, got: %s", out)
	}
}

// aria-errormessage is the other half of the WCAG pairing and some themes use
// it instead of aria-describedby.
func TestPruneDOMToInvalidFields_KeepsAriaErrorMessageText(t *testing.T) {
	form := `<form>
		<label for="vs">Visa Status</label>
		<select id="vs" name="visa" aria-invalid="true" aria-errormessage="vs-err"><option value="">Select</option></select>
		<span id="vs-err">This question is required.</span>
	</form>`

	out, _, err := PruneDOMToInvalidFields(form)
	if err != nil {
		t.Fatalf("PruneDOMToInvalidFields: %v", err)
	}
	if !strings.Contains(out, "This question is required.") {
		t.Errorf("aria-errormessage text must be kept, got: %s", out)
	}
}
