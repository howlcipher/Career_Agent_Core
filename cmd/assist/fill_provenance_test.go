package main

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
)

// bugs.md #548, the producer half. The storage layer cannot tell whether a fill
// was attempted; only this process knows, and only at one moment -- the
// instant before it touches the employer's controls.
//
// What these tests pin is the thing the audit found hardest to see: an attempt
// must be recorded on the paths that record nothing else. A fill that errors
// writes no summary at all, and a browser that never fills must not be
// mistaken for one that did.

// stubContinuationInputs makes continueAssistedApplication reach (or not reach)
// its fill, and reports whether the fill-attempt marker was written.
type continuationProbe struct {
	marked     bool
	markedJob  string
	fillCalled bool
}

// reachedForm says whether the stubbed fill got as far as a real form surface.
// FillAssistedMappedPage returns before touching any control on an expired
// posting, a bot-check page or a quarantined DOM, and it signals the
// difference by calling plan.OnFormReached only once past those guards. A stub
// that always invoked the callback would test a fill that cannot fail early,
// which is the case that matters least.
type reachedForm bool

const (
	formReached    reachedForm = true
	formNotReached reachedForm = false
)

func stubContinuation(t *testing.T, docErr, fillErr error, report submitter.FillReport, reached reachedForm) *continuationProbe {
	t.Helper()
	probe := &continuationProbe{}

	oldLoadDocument := loadAssistedDocument
	oldLoadPII := loadAssistedPII
	oldFillPage := fillAssistedPage
	oldMark := markFillAttempted
	oldRefill := recordAssistedRefill
	oldManualReview := recordAssistedManualReview
	t.Cleanup(func() {
		loadAssistedDocument = oldLoadDocument
		loadAssistedPII = oldLoadPII
		fillAssistedPage = oldFillPage
		markFillAttempted = oldMark
		recordAssistedRefill = oldRefill
		recordAssistedManualReview = oldManualReview
	})

	loadAssistedDocument = func(*sql.DB, string, string) (storage.AssistedDocument, error) {
		return storage.AssistedDocument{Path: "fixture"}, docErr
	}
	loadAssistedPII = func(string) (*config.PII, error) { return &config.PII{}, nil }
	fillAssistedPage = func(plan submitter.AssistedFillPlan) (submitter.FillReport, error) {
		probe.fillCalled = true
		if reached && plan.OnFormReached != nil {
			plan.OnFormReached()
		}
		return report, fillErr
	}
	markFillAttempted = func(_ *sql.DB, jobID string, _ time.Time) error {
		probe.marked = true
		probe.markedJob = jobID
		return nil
	}
	recordAssistedRefill = func(*sql.DB, string, string, time.Time) error { return nil }
	recordAssistedManualReview = func(*sql.DB, string, string, time.Time) error { return nil }
	return probe
}

// 3 + 5. The ordinary case: a fill reaches the form, types something, and the
// attempt is recorded alongside the work.
func TestContinueAssistedApplication_MarksAFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, nil, submitter.FillReport{
		Filled:        []submitter.FilledField{{Key: "first_name", Label: "First Name"}},
		ReusedAnswers: 1,
	}, formReached)

	continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !probe.fillCalled {
		t.Fatal("precondition: the fill should have run")
	}
	if !probe.marked {
		t.Fatal("a fill ran without recording that it was attempted")
	}
	if probe.markedJob != "42" {
		t.Fatalf("attempt recorded against the wrong application: %q", probe.markedJob)
	}
}

// 9. The path that used to leave no trace whatsoever. The fill reaches the
// form, then the page or the handler fails; cmd/assist preserves manual review
// and returns, writing no summary at all. Before this fix nothing anywhere
// recorded that Career Agent had tried.
func TestContinueAssistedApplication_MarksAFailedFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, errors.New("mapping no longer usable"), submitter.FillReport{}, formReached)

	keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !keepOpen {
		t.Fatal("a failed fill must still preserve the open browser for manual completion")
	}
	if !probe.marked {
		t.Fatal("a fill that started and then failed recorded no attempt")
	}
}

// 4. A fill that reaches the form, runs to completion, and types nothing. This
// is the only state that entitles the card to say the attempt completed no
// work, so the marker has to be there to license the sentence -- and unlike the
// failure case above it must reach the summary writer, not the error branch.
func TestContinueAssistedApplication_MarksAZeroResultFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, nil, submitter.FillReport{}, formReached)

	keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !keepOpen {
		t.Fatal("a completed zero-result fill should still preserve the browser for review")
	}
	if !probe.fillCalled {
		t.Fatal("precondition: the fill should have run")
	}
	if !probe.marked {
		t.Fatal("a fill that completed nothing recorded no attempt")
	}
}

// The defect the independent review found in the first cut of this fix, and the
// one most worth a test: an expired posting and a bot-check page both return
// from the fill before a single control is read.
//
// Marking an attempt in that state made the card say "Career Agent attempted
// this form" about a page with no form on it, and sent the operator off to
// hand-fill a posting that no longer exists -- bugs.md #548 with its sign
// flipped. The marker now travels inside the fill and fires only past those
// guards, so a fill that never reaches a form records nothing.
func TestContinueAssistedApplication_DoesNotMarkWhenTheFormWasNeverReached(t *testing.T) {
	probe := stubContinuation(t, nil, errors.New("job posting is expired"), submitter.FillReport{}, formNotReached)

	keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !keepOpen {
		t.Fatal("an expired posting must still preserve the open browser")
	}
	if !probe.fillCalled {
		t.Fatal("precondition: the fill should have been invoked")
	}
	if probe.marked {
		t.Fatal("a dead posting was recorded as a form Career Agent attempted to fill")
	}
}

// 8. A browser opened, and no fill was invoked. The continuation bails before
// the fill when its documents or PII are unavailable, and an application in
// that state has had nothing done to it -- claiming an attempt would be #548
// rebuilt in a new column.
func TestContinueAssistedApplication_DoesNotMarkWhenNoFillIsInvoked(t *testing.T) {
	probe := stubContinuation(t, errors.New("missing document"), nil, submitter.FillReport{}, formNotReached)

	continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if probe.fillCalled {
		t.Fatal("precondition: no fill should have run")
	}
	if probe.marked {
		t.Fatal("a browser that never filled anything was recorded as a fill attempt")
	}
}

// A bookkeeping failure must not cost the operator their application. Losing
// the marker degrades the card to "no fill result recorded", which is a worse
// answer but an honest one; refusing to fill would not be.
func TestContinueAssistedApplication_FillsEvenIfTheMarkerCannotBeWritten(t *testing.T) {
	probe := stubContinuation(t, nil, nil, submitter.FillReport{}, formReached)
	markFillAttempted = func(*sql.DB, string, time.Time) error {
		return errors.New("database is locked")
	}

	if keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner"); !keepOpen {
		t.Fatal("a failed marker write ended the session")
	}
	if !probe.fillCalled {
		t.Fatal("a failed marker write prevented the fill from running")
	}
}

// 8, second half. Not every Continue runs a fill. The direct-browser path
// (Workable) records manual review and hands the form to the operator without
// ever filling it, so it must not reach the marker.
//
// Asserted against the source because the path needs a live browser process to
// exercise, and the property is structural: the function has no fill in it.
func TestDirectAssistedBrowser_NeverMarksAFillAttempt(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body, ok := functionBody(string(source), "func runDirectAssistedBrowser(")
	if !ok {
		t.Fatal("runDirectAssistedBrowser no longer exists; this assertion measures nothing")
	}
	for _, forbidden := range []string{"markFillAttempted", "fillAssistedPage", "applyAssistedAnswers"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the direct assisted browser reached %q, but it never fills the form", forbidden)
		}
	}
	// And it must still be doing its actual job, or an empty function would
	// pass this test.
	if !strings.Contains(body, "recordAssistedManualReview") {
		t.Fatal("the direct assisted browser no longer preserves manual review")
	}
}

// The two marker call sites are placed differently on purpose, and each
// placement is the only correct one for its path. This asserts both, because
// getting either wrong reintroduces a defect this fix already made once.
//
//   - The refill hands the marker to the fill as OnFormReached, so it fires
//     only once the fill is past its expired-posting, bot-check and quarantine
//     guards. Calling it *before* fillAssistedPage is what made the card claim
//     Career Agent had attempted a form on a page that had none.
//   - The answers path marks before its call, which is right there: the
//     operator is answering questions read off the form in front of them, and
//     ApplyApprovedAnswers has no guards to clear -- it goes straight to the
//     controls.
func TestFillProvenance_MarkerIsPlacedCorrectlyOnBothPaths(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	refill, ok := functionBody(text, "func continueAssistedApplication(")
	if !ok {
		t.Fatal("continueAssistedApplication no longer exists; this assertion measures nothing")
	}
	fillAt := strings.Index(refill, "fillAssistedPage(")
	if fillAt < 0 {
		t.Fatal("the refill no longer calls fillAssistedPage; this assertion measures nothing")
	}
	markerAt := strings.Index(refill, "markFillAttempted")
	if markerAt < 0 {
		t.Fatal("the refill records no fill attempt at all")
	}
	// Inside the plan literal, therefore after the call begins -- not before it.
	if markerAt < fillAt {
		t.Error("the refill marks an attempt before the fill runs, so a dead posting or a bot check would be reported as a form Career Agent tried to fill")
	}
	if !strings.Contains(refill, "OnFormReached") {
		t.Error("the refill no longer defers its marker to OnFormReached, so the marker no longer means the form was reached")
	}

	answers, ok := functionBody(text, "func applyOperatorAnswers(")
	if !ok {
		t.Fatal("applyOperatorAnswers no longer exists; this assertion measures nothing")
	}
	applyAt := strings.Index(answers, "applyAssistedAnswers(")
	answerMarkerAt := strings.Index(answers, "markFillAttempted")
	if applyAt < 0 {
		t.Fatal("the answers path no longer applies answers; this assertion measures nothing")
	}
	if answerMarkerAt < 0 {
		t.Fatal("the answers path records no fill attempt at all")
	}
	if answerMarkerAt > applyAt {
		t.Error("the answers path marks after typing, so answers typed into a browser that then died would leave no record")
	}
}

// The marker must survive the writer that runs after the operator's answers
// are applied. RecordAssistedAnswersApplied is a bare UPDATE that writes real
// counts, and if it left fill_attempted_at alone, a marker write that had
// failed earlier -- deliberately non-fatal -- would leave a row carrying counts
// and no marker, which the card reads as "no fill has been recorded" about
// answers the operator watched being typed.
func TestFillProvenance_AnswersWriterRecordsAnAttemptOfItsOwn(t *testing.T) {
	source, err := os.ReadFile("../../pkg/storage/assisted.go")
	if err != nil {
		t.Fatal(err)
	}
	body, ok := functionBody(string(source), "func RecordAssistedAnswersApplied(")
	if !ok {
		t.Fatal("RecordAssistedAnswersApplied no longer exists; this assertion measures nothing")
	}
	if !strings.Contains(body, "fill_attempted_at") {
		t.Fatal("the answers writer records counts without recording that a fill ran")
	}
	if !strings.Contains(body, "COALESCE(fill_attempted_at") {
		t.Error("the answers writer overwrites the moment the fill began instead of preserving it")
	}
}

// functionBody returns the text of the function whose declaration starts with
// prefix, from the declaration to the first line that closes it at column zero.
func functionBody(source, prefix string) (string, bool) {
	start := strings.Index(source, prefix)
	if start < 0 {
		return "", false
	}
	rest := source[start:]
	if end := strings.Index(rest, "\n}\n"); end >= 0 {
		return rest[:end], true
	}
	return rest, true
}
