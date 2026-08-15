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

func stubContinuation(t *testing.T, docErr, fillErr error, report submitter.FillReport) *continuationProbe {
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
	fillAssistedPage = func(submitter.AssistedFillPlan) (submitter.FillReport, error) {
		probe.fillCalled = true
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

// 3. The ordinary case: a fill runs, and the attempt is recorded.
func TestContinueAssistedApplication_MarksAFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, nil, submitter.FillReport{})

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

// 9. The path that used to leave no trace whatsoever. The fill starts, the
// browser or the page fails, cmd/assist preserves manual review and returns --
// and before this fix nothing anywhere recorded that Career Agent had tried.
func TestContinueAssistedApplication_MarksAFailedFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, errors.New("mapping no longer usable"), submitter.FillReport{})

	keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !keepOpen {
		t.Fatal("a failed fill must still preserve the open browser for manual completion")
	}
	if !probe.marked {
		t.Fatal("a fill that started and then failed recorded no attempt")
	}
}

// 4. A fill that runs to completion having typed nothing. This is the only
// state that entitles the card to say the attempt completed no work, so the
// marker has to be there to license the sentence.
func TestContinueAssistedApplication_MarksAZeroResultFillAsAttempted(t *testing.T) {
	probe := stubContinuation(t, nil, nil, submitter.FillReport{})

	continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "42"}, "owner")

	if !probe.marked {
		t.Fatal("a fill that completed nothing recorded no attempt")
	}
}

// 8. A browser opened, and no fill was invoked. The continuation bails before
// the fill when its documents or PII are unavailable, and an application in
// that state has had nothing done to it -- claiming an attempt would be #548
// rebuilt in a new column.
func TestContinueAssistedApplication_DoesNotMarkWhenNoFillIsInvoked(t *testing.T) {
	probe := stubContinuation(t, errors.New("missing document"), nil, submitter.FillReport{})

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
	probe := stubContinuation(t, nil, nil, submitter.FillReport{})
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

// The marker is written before the fill, not after. Every outcome that
// produces no summary depends on that ordering, so it is asserted directly
// rather than left to be inferred from the tests above.
func TestFillProvenance_MarkerPrecedesTheFillItRecords(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, pair := range []struct{ marker, fill string }{
		{"markFillAttempted", "fillAssistedPage("},
		{"markFillAttempted", "applyAssistedAnswers("},
	} {
		body, ok := functionBody(text, "func continueAssistedApplication(")
		if pair.fill == "applyAssistedAnswers(" {
			body, ok = functionBody(text, "func applyOperatorAnswers(")
		}
		if !ok {
			t.Fatalf("could not read the function containing %q", pair.fill)
		}
		markerAt := strings.Index(body, pair.marker)
		fillAt := strings.Index(body, pair.fill)
		if markerAt < 0 {
			t.Errorf("%q is never recorded before %q", pair.marker, pair.fill)
			continue
		}
		if fillAt < 0 {
			t.Errorf("%q no longer appears; this assertion measures nothing", pair.fill)
			continue
		}
		if markerAt > fillAt {
			t.Errorf("%q is recorded after %q, so a fill that fails would leave no trace", pair.marker, pair.fill)
		}
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
