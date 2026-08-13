package submitter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestPreflight_CannotReachAnythingThatSubmits pins the boundary at the source
// level, in the same spirit as
// TestControlInventory_NeverEnumeratesAControlThatCouldSubmit.
//
// A behavioural test cannot prove this: it would have to enumerate every real
// employer form to show that none of them was ever submitted. What can be
// proved is that no call path exists. Preflight loads a page, reads its
// controls and closes it; if a future change gives it a way to click a submit
// control, or to type a value into a form, this test is where that shows up
// rather than on a real application.
func TestPreflight_CannotReachAnythingThatSubmits(t *testing.T) {
	source, err := os.ReadFile("preflight.go")
	if err != nil {
		t.Fatalf("read preflight.go: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "preflight.go", source, 0)
	if err != nil {
		t.Fatalf("parse preflight.go: %v", err)
	}

	// Anything that locates, presses or fills. clickApplyIfPresent is the one
	// deliberate exception: it presses the employer's own Apply affordance,
	// which is how their form is reached at all, and it is not a submit
	// control. Everything else on this list would be.
	forbidden := map[string]string{
		"findSubmitControl":         "preflight must never locate a submit control",
		"AttemptSubmit":             "preflight must never enter the submission path",
		"applyAnswerToControl":      "preflight must never type a value into a form",
		"ApplyApprovedAnswers":      "preflight must never commit an operator's answers",
		"FillAssistedMappedPage":    "preflight must never fill",
		"handleDynamic":             "preflight must never run a fill handler",
		"runAssistedHandler":        "preflight must never run an ATS fill handler",
		"safeFillWithLabelFallback": "preflight must never fill",
		"commitComboboxSelection":   "preflight must never commit a selection",
	}

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		if reason, banned := forbidden[name]; banned {
			t.Errorf("preflight.go calls %s: %s", name, reason)
		}
		return true
	})

	// The submit selector table must not be referenced either, since naming it
	// is how a locator for one gets built.
	if strings.Contains(string(source), "submitControlSelectors") {
		t.Error("preflight.go references submitControlSelectors; it has no business knowing where a submit button is")
	}
}

// TestPreflightReasons_AreAClosedVocabulary keeps the reason codes constants
// rather than error text.
//
// The point is the same one ADR-006 makes about BrowserFailureReason: a caller
// logging one of these is logging a value declared in this file, so no wording
// of any page or any driver diagnostic can travel with it. A reason built by
// interpolating an error would defeat that silently.
func TestPreflightReasons_AreAClosedVocabulary(t *testing.T) {
	vocabulary := map[string]bool{
		PreflightOK:               true,
		PreflightCaptchaBlocked:   true,
		PreflightAuthRequired:     true,
		PreflightPostingDead:      true,
		PreflightQuarantined:      true,
		PreflightNavigationFailed: true,
		PreflightNoFormFound:      true,
		PreflightBrowserRejected:  true,
		PreflightUnclassified:     true,
	}
	if len(vocabulary) != 9 {
		t.Fatalf("the reason vocabulary has %d distinct values; two constants collide", len(vocabulary))
	}
	for reason := range vocabulary {
		if reason == PreflightOK {
			continue
		}
		// Lowercase, underscore-separated, no spaces or punctuation: a shape
		// that cannot accidentally be a sentence from somewhere else.
		if strings.ToLower(reason) != reason || strings.ContainsAny(reason, " \t\"<>=") {
			t.Errorf("reason %q is not a bare code", reason)
		}
	}
}

// TestPreflightRefusals_HappenBeforeAnyPageIsLoaded proves the two refusals
// that must not cost a page load: an ATS that rejects Career Agent's browser,
// and one with no pre-auth form at all.
//
// Passing a nil browser is the assertion. If either refusal were made after the
// navigation rather than before it, this would panic instead of returning a
// reason.
func TestPreflightRefusals_HappenBeforeAnyPageIsLoaded(t *testing.T) {
	original := assistedBrowserRejectionForPreflight
	t.Cleanup(func() { assistedBrowserRejectionForPreflight = original })
	SetAssistedBrowserRejectionCheck(func(rawURL string) string {
		if strings.Contains(rawURL, "lever.co") {
			return "this ATS rejects the assisted browser"
		}
		return ""
	})

	rejected := InspectApplication(nil, nil, "Lever Co", "https://jobs.lever.co/example/abc")
	if rejected.Reason != PreflightBrowserRejected {
		t.Fatalf("reason = %q, want %q", rejected.Reason, PreflightBrowserRejected)
	}
	if rejected.Inspected() {
		t.Fatal("a rejected ATS must not report an inspection")
	}
	if len(rejected.Controls) != 0 {
		t.Fatal("a refusal must not carry controls")
	}

	// Workday has no form before sign-in (bug #18). "Preflight unavailable
	// until authenticated" is the answer, not something to work around.
	gated := InspectApplication(nil, nil, "Workiva", "https://workiva.wd503.myworkdayjobs.com/careers/job/123")
	if gated.Reason != PreflightAuthRequired {
		t.Fatalf("reason = %q, want %q", gated.Reason, PreflightAuthRequired)
	}
}

func TestLooksLikePasswordGate_DistinguishesSignInFromApplication(t *testing.T) {
	// An empty application form and a sign-in wall both look like "no questions
	// to answer". Reporting a sign-in wall as an application needing nothing
	// would tell the operator to expect an easy application and hand them a
	// login page.
	signIn := []FormControl{
		{Key: "email", ControlType: "email"},
		{Key: "password", ControlType: "password"},
	}
	if !looksLikePasswordGate(signIn) {
		t.Error("a password field means this is a sign-in form, not an application")
	}
	application := []FormControl{
		{Key: "first_name", ControlType: "text"},
		{Key: "resume", ControlType: "file"},
		{Key: "notice", ControlType: "text"},
	}
	if looksLikePasswordGate(application) {
		t.Error("an ordinary application form was mistaken for a sign-in wall")
	}
}
