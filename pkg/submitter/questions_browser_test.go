package submitter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

// This file is the real-browser regression for bugs.md #545.
//
// It runs the *shipped* controlInventoryJS against a *real* DOM in Chromium.
// That is deliberate and it is the only honest way to test this: the label
// chain is JavaScript executed by a browser engine, and a Go reimplementation
// of it in a test would only ever assert that the copy agrees with itself
// while the shipped copy drifted.
//
// The fixtures are the real markup shapes, reduced to the parts that matter and
// with the employer's own question text kept. That text is public posting
// content read from a public /apply page; no answer, no pii.yaml value and no
// personal data appears anywhere in this file.

// requireChromium gates these tests the same way the log-privacy regression in
// this package does. They are real browser tests, so they are opt-in.
func requireChromium(t *testing.T) {
	t.Helper()
	if os.Getenv("CAREER_AGENT_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("set CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1 to run the Chromium form-inventory regressions")
	}
}

// inventoryFixture serves one synthetic page and returns its control inventory
// keyed by control key, which is how every assertion below checks the label and
// the control identity together. A right label on the wrong control is worse
// than an ugly label, so no test may assert one without the other.
func inventoryFixture(t *testing.T, body string) map[string]FormControl {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<!doctype html><html><body>%s</body></html>", body)
	}))
	t.Cleanup(server.Close)

	page, cleanup := openCanaryPage(t, server.URL)
	t.Cleanup(cleanup)

	controls, err := SnapshotControls(resolveFillTarget(page))
	if err != nil {
		t.Fatalf("inventory the fixture: %v", err)
	}
	controls = SanitizeControls(security.NewQuarantineLayer(), controls)
	byKey := make(map[string]FormControl, len(controls))
	for _, control := range controls {
		byKey[control.Key] = control
	}
	return byKey
}

// assertLabel checks one control's question text and reports the whole
// inventory on failure, because the useful debugging information for this bug
// is always "what did every other control come out as".
func assertLabel(t *testing.T, byKey map[string]FormControl, key, wantLabel string) FormControl {
	t.Helper()
	control, found := byKey[key]
	if !found {
		t.Fatalf("no control with key %q; inventory was %s", key, describe(byKey))
	}
	if control.Label != wantLabel {
		t.Errorf("control %q: label = %q, want %q", key, control.Label, wantLabel)
	}
	return control
}

func describe(byKey map[string]FormControl) string {
	var b strings.Builder
	for key, control := range byKey {
		fmt.Fprintf(&b, "\n  %q -> %q (%s, required=%v, options=%v)",
			key, control.Label, control.ControlType, control.Required, control.Options)
	}
	return b.String()
}

// leverCustomQuestionCard is the shape every Lever "cards" question uses, taken
// from a real posting. The control sits in its own div.application-field
// wrapper; the question lives in a *sibling* div.application-label, one level
// up. That is the entire bug: closest('… , div') stopped at the wrapper.
//
// Note what the control does not have: no id, no aria-label, no <label> of any
// kind, and a name generated from a card UUID. Every accessible-name route is
// genuinely absent, which is why the fallback has to be right.
const leverCustomQuestionCard = `
<div class="section page-centered application-form" data-qa="additional-cards">
<h4 data-qa="card-name">Application Form (USA)</h4>
<input type="hidden" value="{}" name="cards[24537723-bc35-433d-9173-d8f769ac0968][baseTemplate]">
<ul>

<li class="application-question custom-question"><div>
  <div class="application-label full-width text"><div class="text">Where do you currently reside? (City/State)?<span class="required">&#10033;</span></div></div>
  <div class="application-field full-width required-field">
    <input required="required" class="card-field-input" type="text" placeholder="Type your response" value="" name="cards[24537723-bc35-433d-9173-d8f769ac0968][field0]" />
  </div>
</div></li>

<li class="application-question custom-question"><div>
  <div class="application-label full-width multiple-choice"><div class="text">Are you willing to relocate if needed?<span class="required">&#10033;</span></div></div>
  <div class="application-field full-width required-field"><ul data-qa="multiple-choice">
    <li><label><input type="radio" name="cards[24537723-bc35-433d-9173-d8f769ac0968][field2]" value="Yes" required="required" /><span class="application-answer-alternative">Yes</span></label></li>
    <li><label><input type="radio" name="cards[24537723-bc35-433d-9173-d8f769ac0968][field2]" value="No" required="required" /><span class="application-answer-alternative">No</span></label></li>
  </ul></div>
</div></li>

<li class="application-question custom-question"><div>
  <div class="application-label full-width textarea"><div class="text">Why are you interested in Veeva?<span class="required">&#10033;</span></div></div>
  <div class="application-field full-width required-field">
    <textarea class="card-field-input" name="cards[24537723-bc35-433d-9173-d8f769ac0968][field1]" required="required"></textarea>
  </div>
</div></li>

<li class="application-question custom-question"><div>
  <div class="application-label full-width dropdown"><div class="text">Confirm the ability to perform the requisite duties of the role with or without reasonable accommodations<span class="required">&#10033;</span></div></div>
  <div class="application-field full-width required-field"><div class="application-dropdown">
    <select name="cards[24537723-bc35-433d-9173-d8f769ac0968][field3]" required><option value="">Select...</option><option value="Yes">Yes</option><option value="No">No</option><option value="Prefer not to say">Prefer not to say</option></select>
  </div></div>
</div></li>

<li class="application-question custom-question"><div>
  <div class="application-label full-width text"><div class="text">What are your salary expectations?</div></div>
  <div class="application-field full-width">
    <input class="card-field-input" type="text" placeholder="Type your response" value="" name="cards[24537723-bc35-433d-9173-d8f769ac0968][field4]" />
  </div>
</div></li>

</ul>
</div>`

// leverStandardFields are Lever's built-in fields. The text inputs already work
// today, because the input is wrapped in a <label> — they are here so the fix
// is proved not to break them. The pronouns checkbox group does not work today:
// its question is a sibling div and its options are wrapped labels, so the
// group label comes out as an option ("He/him").
const leverStandardFields = `
<form><ul>
<li class="application-question"><label><div class="application-label">Full name<span class="required">&#10033;</span></div><div class="application-field"><input type="text" data-qa="name-input" name="name" required></div></label></li>
<li class="application-question"><label><div class="application-label">Portfolio URL</div><div class="application-field"><input type="text" name="urls[Portfolio]"></div></label></li>
<li class="application-question"><div class="application-label multiple-select">Pronouns</div><div class="application-field"><ul id="candidatePronounsCheckboxes">
  <div class="column-wrapper">
    <div class="table-row">
      <li class="column"><label><input type="checkbox" name="pronouns" value="He/him" /><span class="application-answer-alternative">He/him</span></label></li>
      <li class="column"><label><input type="checkbox" name="pronouns" value="She/her" /><span class="application-answer-alternative">She/her</span></label></li>
    </div>
    <div class="table-row">
      <li class="column"><label><input type="checkbox" name="pronouns" value="They/them" /><span class="application-answer-alternative">They/them</span></label></li>
    </div>
  </div>
</ul></div></li>
</ul></form>`

const leverCardUUID = "cards[24537723-bc35-433d-9173-d8f769ac0968]"

// TestControlInventory_LeverCustomQuestionCardsReportTheQuestion is the core
// regression. Every one of these came back as "Type your response", an option
// label, or the raw cards[<uuid>][fieldN] name before the fix.
func TestControlInventory_LeverCustomQuestionCardsReportTheQuestion(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, leverCustomQuestionCard)

	// A text input whose only fallback was its placeholder.
	text := assertLabel(t, byKey, leverCardUUID+"[field0]", "Where do you currently reside? (City/State)?\u2731")
	if !text.Required {
		t.Error("the required marker on the card must still make the control required")
	}
	if strings.Contains(text.Label, "Type your response") {
		t.Error("the placeholder must not be reported as the question")
	}

	// A textarea, which has no placeholder at all, so the old chain reached the
	// name attribute.
	area := assertLabel(t, byKey, leverCardUUID+"[field1]", "Why are you interested in Veeva?\u2731")
	if area.ControlType != "textarea" {
		t.Errorf("control type = %q, want textarea", area.ControlType)
	}
	if strings.Contains(area.Label, "cards[") {
		t.Error("the raw name attribute must not be reported as the question")
	}

	// A radio group: the question is the card's label, not the first option.
	radio := assertLabel(t, byKey, "group:"+leverCardUUID+"[field2]", "Are you willing to relocate if needed?\u2731")
	if radio.ControlType != "radio" {
		t.Errorf("control type = %q, want radio", radio.ControlType)
	}
	if strings.Join(radio.Options, "|") != "Yes|No" {
		t.Errorf("the option text must be unchanged, got %v", radio.Options)
	}
	if !radio.Required {
		t.Error("a required radio group must stay required")
	}

	// A select, also with no placeholder. Its options must survive intact.
	choice := assertLabel(t, byKey, leverCardUUID+"[field3]",
		"Confirm the ability to perform the requisite duties of the role with or without reasonable accommodations\u2731")
	if strings.Join(choice.Options, "|") != "Yes|No|Prefer not to say" {
		t.Errorf("select options changed: %v", choice.Options)
	}

	// An optional question. Nothing about the fix may invent a required marker.
	optional := assertLabel(t, byKey, leverCardUUID+"[field4]", "What are your salary expectations?")
	if optional.Required {
		t.Error("an optional card question must not be reported as required")
	}

	// Five distinct questions, each attached to its own control: no card may
	// borrow the question text of the one above or below it.
	seen := map[string]string{}
	for key, control := range byKey {
		if previous, duplicate := seen[control.Label]; duplicate {
			t.Errorf("controls %q and %q share the label %q", previous, key, control.Label)
		}
		seen[control.Label] = key
	}
	if len(seen) != 5 {
		t.Errorf("expected 5 distinct questions, got %d: %s", len(seen), describe(byKey))
	}
}

// TestControlInventory_LeverStandardFieldsKeepWorkingAndFixThePronounGroup
// pins both directions at once: the wrapped-label fields that already worked
// still work, and the checkbox group that reported an option now reports its
// question.
func TestControlInventory_LeverStandardFieldsKeepWorkingAndFixThePronounGroup(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, leverStandardFields)

	// These two resolve through the wrapping <label>, a step this change does
	// not touch.
	//
	// The required glyph stays in the label, deliberately. Trimming it would be
	// a small improvement here and a change to Greenhouse's labels there
	// ("Country *"), and this fix must not touch Greenhouse. It costs nothing to
	// leave: Required already carries the fact as a field of its own, and
	// answers.Normalize reduces every non-alphanumeric to a space, so no
	// canonical key, alias or vault match can see the glyph either way.
	name := assertLabel(t, byKey, "name", "Full name\u2731")
	if !name.Required {
		t.Error("the built-in name field is required")
	}
	assertLabel(t, byKey, "urls[Portfolio]", "Portfolio URL")

	// The group whose label was an option's own text.
	pronouns := assertLabel(t, byKey, "group:pronouns", "Pronouns")
	if pronouns.ControlType != "checkbox" {
		t.Errorf("control type = %q, want checkbox", pronouns.ControlType)
	}
	if strings.Join(pronouns.Options, "|") != "He/him|She/her|They/them" {
		t.Errorf("the option text must be unchanged, got %v", pronouns.Options)
	}
	if pronouns.Required {
		t.Error("the pronouns group is optional")
	}
}

// greenhouseForm is the real Greenhouse shape: every control carries an
// accessible name, either aria-label or aria-labelledby. Those are steps 1 and
// 2 of the chain, so Greenhouse never reaches the fallback this change
// replaces — which is exactly what makes the change safe. This test is what
// turns that reasoning into something that fails if it stops being true.
const greenhouseForm = `
<form>
<div class="field"><label id="first_name-label" for="first_name">First Name *</label>
  <input id="first_name" class="input" aria-label="First Name" aria-describedby="first_name-description" required></div>
<div class="field"><label id="email-label" for="email">Email *</label>
  <input id="email" class="input" aria-label="Email" required></div>
<div class="field"><label id="question_31692483003-label" for="question_31692483003">LinkedIn Profile</label>
  <input id="question_31692483003" class="input" aria-label="LinkedIn Profile"></div>
<div class="field"><label id="country-label" for="country">Country *</label>
  <div class="select__control">
    <input id="country" class="select__input" role="combobox" aria-labelledby="country-label" aria-required="true" aria-autocomplete="list">
    <input required tabindex="-1" aria-hidden="true" class="requiredInput" value="">
  </div></div>
<div class="field"><label id="question_31692495003-label" for="question_31692495003">Other Links</label>
  <textarea id="question_31692495003" class="input" aria-label="Other Links"></textarea></div>
</form>`

// TestControlInventory_GreenhouseLabelsAreUnchanged is the shared-path guard.
// This code is used by the real Assisted Apply fill path, and Greenhouse is the
// one ATS that completes inside the assisted browser, so a regression here
// costs a real application.
func TestControlInventory_GreenhouseLabelsAreUnchanged(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, greenhouseForm)

	for key, want := range map[string]string{
		"first_name":           "First Name",
		"email":                "Email",
		"question_31692483003": "LinkedIn Profile",
		"country":              "Country *",
		"question_31692495003": "Other Links",
	} {
		assertLabel(t, byKey, key, want)
	}

	if got := byKey["country"].ControlType; got != "combobox" {
		t.Errorf("the country control type = %q, want combobox", got)
	}
	// The hidden validation input Greenhouse renders beside its comboboxes is
	// aria-hidden and must stay out of the inventory: it is not a question, and
	// surfacing it would put an unanswerable field in the operator's inbox.
	if len(byKey) != 5 {
		t.Errorf("expected exactly the 5 real controls, got %d: %s", len(byKey), describe(byKey))
	}
}

// genericGroupedFields is the shape the old closest('…, div') fallback handled
// correctly: a label and its input sharing a container with another field.
// The new walk must not lose it. Neither input can resolve through a label
// association, so both genuinely reach the fallback.
const genericGroupedFields = `
<form>
<div class="form-group">
  <label>Desired compensation</label>
  <input type="text" name="comp">
</div>
<fieldset>
  <legend>Are you legally authorized to work?</legend>
  <label><input type="radio" name="authorized" value="Yes">Yes</label>
  <label><input type="radio" name="authorized" value="No">No</label>
</fieldset>
<div class="field">
  How did you hear about us?
  <input type="text" name="source">
</div>
</form>`

func TestControlInventory_GenericFallbackShapesStillResolve(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, genericGroupedFields)

	// A sibling <label> with no `for` — the case the old fallback existed for.
	assertLabel(t, byKey, "comp", "Desired compensation")
	// A fieldset/legend group, resolved before the walk is ever consulted.
	authorized := assertLabel(t, byKey, "group:authorized", "Are you legally authorized to work?")
	if strings.Join(authorized.Options, "|") != "Yes|No" {
		t.Errorf("legend groups must keep their option text, got %v", authorized.Options)
	}
	// A bare text node beside the control, with no element wrapping it at all.
	assertLabel(t, byKey, "source", "How did you hear about us?")
}

// leverLabelWrappedControls are the three shapes the first pass of this fix did
// not reach. All three were found by running the read-only inspection against a
// real Lever posting after the unit fixtures already passed — which is the
// point of doing that run.
//
//   - a <label> wrapping a <select>: textContent includes the options, so the
//     EEO questions came out as "GenderSelect ...MaleFemaleDecline to
//     self-identify".
//   - a one-option attestation card: the group never climbed above the option's
//     own <label>, so the question was its single option, "I Acknowledge",
//     rather than the paragraph being acknowledged.
//   - a help note inside the group container: it outranked the group's own
//     label, so the pronouns group read as its description sentence.
const leverLabelWrappedControls = `
<div class="application-question"><label><div class="application-label">Gender</div><div class="application-field"><div class="application-dropdown">
  <select name="eeo[gender]"><option value="">Select ...</option><option value="Male">Male</option><option value="Female">Female</option><option value="Decline to self-identify">Decline to self-identify</option></select>
</div></div></label></div>

<li class="application-question custom-question"><div>
  <div class="application-label full-width multiple-select"><div class="text">AHEAD will consider the contents of an uploaded resume only insofar as it pertains to employment history<span class="required">&#10033;</span></div></div>
  <div class="application-field full-width required-field"><ul data-qa="multiple-select">
    <li><label><input type="checkbox" name="cards[6c653c7d][field5]" value="I Acknowledge" required="required" /><span class="application-answer-alternative">I Acknowledge</span></label></li>
  </ul></div>
</div></li>

<li class="application-question"><div class="application-label multiple-select">Pronouns</div><div class="application-field"><ul id="candidatePronounsCheckboxes">
  <div class="column-wrapper"><div class="table-row">
    <li class="column"><label><input type="checkbox" name="pronouns" value="He/him" /><span class="application-answer-alternative">He/him</span></label></li>
    <li class="column"><label><input type="checkbox" name="pronouns" value="She/her" /><span class="application-answer-alternative">She/her</span></label></li>
  </div></div>
  <p class="description">Let the employer know what pronouns you use so that they can address you correctly.</p>
</ul></div></li>`

func TestControlInventory_LabelWrappedAndSingleOptionControlsReportTheQuestion(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, leverLabelWrappedControls)

	// A <label> around a <select>. The choices belong in Options and nowhere
	// else; repeating them in the question is what made this unreadable.
	gender := assertLabel(t, byKey, "eeo[gender]", "Gender")
	if strings.Join(gender.Options, "|") != "Male|Female|Decline to self-identify" {
		t.Errorf("the options must still be reported, got %v", gender.Options)
	}

	// A one-option group: the question is the attestation, not the option.
	ack := assertLabel(t, byKey, "group:cards[6c653c7d][field5]",
		"AHEAD will consider the contents of an uploaded resume only insofar as it pertains to employment history✱")
	if strings.Join(ack.Options, "|") != "I Acknowledge" {
		t.Errorf("the single option must survive as an option, got %v", ack.Options)
	}
	if !ack.Required {
		t.Error("the attestation is required")
	}

	// A help note inside the group's own container must not outrank the group's
	// label outside it.
	pronouns := assertLabel(t, byKey, "group:pronouns", "Pronouns")
	if strings.Contains(pronouns.Label, "Let the employer know") {
		t.Error("the description is a help note, not the question")
	}
	if strings.Join(pronouns.Options, "|") != "He/him|She/her" {
		t.Errorf("the option text must be unchanged, got %v", pronouns.Options)
	}
}

// TestControlInventory_ATrailingLabelStillResolves pins the asymmetry between
// the two directions. Text before a control is read whatever it is; text after
// one is read only when it is marked up as a <label> or <legend>.
//
// That is deliberate, and it is what keeps Lever's pronouns description from
// outranking the group's own label. Loose prose after a field is a help note.
// The old fallback read trailing text only through querySelector('legend,
// label') too, so nothing that used to resolve stops resolving.
func TestControlInventory_ATrailingLabelStillResolves(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `<form>
<div class="field"><input type="text" name="trailing"><label>Referral code</label></div>
<div class="field"><input type="text" name="loose"> just a note about this field</div>
</form>`)
	assertLabel(t, byKey, "trailing", "Referral code")
	// Loose trailing prose is not a label, so this control keeps falling
	// through to its name rather than claiming a note as its question.
	assertLabel(t, byKey, "loose", "loose")
}

// The shapes below are the code review's findings on the first version of this
// fix. Each one is a real form layout that the walk got wrong, and each would
// have put a wrong label on a real employer's question — the exact failure
// bugs.md #545 is about, reintroduced through a different door.

// TestControlInventory_LabelSharedWithItsControlSurvives covers the most common
// checkbox markup there is. Reading a wrapping <label> by dropping every branch
// that contains a control loses the text entirely when the label and the input
// share one wrapper, and the control then falls through to whatever prose an
// ancestor happens to carry.
func TestControlInventory_LabelSharedWithItsControlSurvives(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `
<div class="section"><h3>Legal</h3>
  <label><span><input type="checkbox" name="agree"> I agree to the terms</span></label>
  <label>Desired salary <input type="text" name="salary"></label>
</div>`)
	agree := assertLabel(t, byKey, "group:agree", "I agree to the terms")
	if agree.Label == "Legal" {
		t.Error("the section heading is not this checkbox's question")
	}
	assertLabel(t, byKey, "salary", "Desired salary")
}

// TestControlInventory_AnAdjacentLabelBeatsASectionHeading pins the ordering.
// Sweeping the whole tree for preceding text before ever looking at trailing
// text let a heading three levels up win over the field's own label.
func TestControlInventory_AnAdjacentLabelBeatsASectionHeading(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `
<div class="section"><h3>Additional Information</h3>
  <div class="field"><input type="text" name="q1"><label>Portfolio URL</label></div>
</div>`)
	assertLabel(t, byKey, "q1", "Portfolio URL")
}

// TestControlInventory_AFlatFormDoesNotGlueQuestionsTogether covers layouts
// where labels and inputs are siblings rather than nested. Reading every
// preceding sibling accumulated each earlier question onto the next field.
func TestControlInventory_AFlatFormDoesNotGlueQuestionsTogether(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `
<div class="fields">
  <div class="q">Question A</div><input type="text" name="a">
  <div class="q">Question B</div><input type="text" name="b">
  <div class="q">Question C</div><input type="text" name="c">
</div>`)
	assertLabel(t, byKey, "a", "Question A")
	assertLabel(t, byKey, "b", "Question B")
	assertLabel(t, byKey, "c", "Question C")
}

// TestControlInventory_AHiddenInputDoesNotHideTheQuestion pins the walk to the
// same definition of "control" the enumeration uses. A hidden state input
// sharing a container with the real control is common — Lever emits one beside
// every custom-question card — and treating it as a control discarded the
// question's text.
func TestControlInventory_AHiddenInputDoesNotHideTheQuestion(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `
<div class="field">
  <div class="application-label">Country<input type="hidden" name="country_code" value="US"></div>
  <div><input type="text" name="country"></div>
</div>
<div class="field">
  <div class="application-label">Referral source</div>
  <input type="submit" value="Go">
  <div><input type="text" name="referral"></div>
</div>`)
	assertLabel(t, byKey, "country", "Country")
	assertLabel(t, byKey, "referral", "Referral source")
}

// TestControlInventory_ADuplicateFormDoesNotStrandAGroup pins the bound on the
// group climb. The member count is document-wide, so a second copy of the same
// form — a mobile duplicate, a hidden mirror — used to send the climb to
// <body>, where the walk stops immediately and the group falls back to its
// first option. That is the original bug, reached by walking too far.
func TestControlInventory_ADuplicateFormDoesNotStrandAGroup(t *testing.T) {
	requireChromium(t)
	byKey := inventoryFixture(t, `
<div class="desktop"><div class="card">
  <div class="application-label">Are you willing to relocate?</div>
  <div class="application-field">
    <label><input type="radio" name="relocate" value="Yes">Yes</label>
    <label><input type="radio" name="relocate" value="No">No</label>
  </div>
</div></div>
<div class="mobile" aria-hidden="true"><div class="card">
  <div class="application-label">Are you willing to relocate?</div>
  <div class="application-field">
    <label><input type="radio" name="relocate" value="Yes">Yes</label>
  </div>
</div></div>`)
	group := assertLabel(t, byKey, "group:relocate", "Are you willing to relocate?")
	if group.Label == "Yes" {
		t.Error("the group fell back to its first option, which is the original defect")
	}
}
