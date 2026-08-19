package submitter

import (
	"errors"
	"log"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

// AssistedFillPlan is everything one assisted refill needs. It is a struct
// rather than a parameter list because the list had already reached seven
// positional arguments before the vault was added, and a caller swapping
// resumePath and coverPath by accident would attach the wrong document to a
// real application.
type AssistedFillPlan struct {
	Page        playwright.Page
	Filter      *security.QuarantineLayer
	Vault       *answers.Store
	CompanyName string
	ApplyURL    string
	ResumePath  string
	CoverPath   string
	PII         *config.PII
	// ATSName is a display label for the report. Empty is fine.
	ATSName string
	// OnFormReached is called once, at the moment this plan has cleared its
	// pre-flight guards and is about to touch the employer's own controls. It
	// is how a caller records that a fill was genuinely attempted, and its
	// placement is the whole of its meaning.
	//
	// The guards it sits behind are not bookkeeping: an expired posting, a
	// bot-check page and a quarantined DOM all return before any control is
	// read, and a caller that marked an attempt before calling this function
	// would report "Career Agent tried to fill this form" about a page that
	// has no form on it (found by the independent review of bugs.md #548 --
	// which is that same defect with its sign flipped, so it is pinned here
	// rather than left to a comment at the call site).
	//
	// Optional. Nil means the caller does not care.
	OnFormReached func()
}

// PreparedDocuments names the documents this plan carries, for the report's
// "Career Agent completed" list.
func (p AssistedFillPlan) PreparedDocuments() []string {
	var out []string
	if strings.TrimSpace(p.ResumePath) != "" {
		out = append(out, "resume")
	}
	if strings.TrimSpace(p.CoverPath) != "" {
		out = append(out, "cover_letter")
	}
	return out
}

// How a field came to be filled.
const (
	FilledByHandler = "handler"
	FilledByVault   = "approved_answer"
)

// FilledField is one control Career Agent completed.
type FilledField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	How   string `json:"how"`
}

// UnresolvedQuestion is one control Career Agent could not safely complete.
//
// Suggested is a proposal, not an answer that was typed anywhere. A sensitive
// question arrives here with its suggestion attached precisely so the operator
// confirms rather than retypes — but it arrives here, in the list of things
// that need them, which is the whole point of keeping the two lists separate.
type UnresolvedQuestion struct {
	Key         string   `json:"key"`
	Selector    string   `json:"selector"`
	Label       string   `json:"label"`
	ControlType string   `json:"control_type"`
	Options     []string `json:"options"`
	Required    bool     `json:"required"`
	Sensitivity string   `json:"sensitivity"`
	Suggested   string   `json:"suggested"`
	Source      string   `json:"source"`
	LabelUnsafe bool     `json:"label_unsafe"`
}

// FillReport is what one assisted refill accomplished, and what it could not.
type FillReport struct {
	ATS           string               `json:"ats"`
	Filled        []FilledField        `json:"filled"`
	Unresolved    []UnresolvedQuestion `json:"unresolved"`
	Documents     []string             `json:"documents"`
	ReusedAnswers int                  `json:"reused_answers"`
}

// FilledCount is the headline number on the operator's card.
func (r FillReport) FilledCount() int { return len(r.Filled) }

// absorb merges a second pass's results into the report.
func (r *FillReport) absorb(other FillReport) {
	r.Filled = append(r.Filled, other.Filled...)
	r.Unresolved = append(r.Unresolved, other.Unresolved...)
	r.ReusedAnswers += other.ReusedAnswers
}

// resolveAndApply takes the controls the handler left empty, asks the vault
// about each, fills the ones it is entitled to fill, and returns the rest as
// questions for the operator.
//
// The entitlement check is the vault's, not this function's: it fills exactly
// those resolutions whose AutoFill is set, which by construction excludes every
// sensitive category and every answer whose reuse the operator has not granted.
func resolveAndApply(target fillTarget, plan AssistedFillPlan, stillEmpty []FormControl) FillReport {
	report := FillReport{}
	if len(stillEmpty) == 0 {
		return report
	}
	byKey := make(map[string]FormControl, len(stillEmpty))
	questions := make([]answers.Question, 0, len(stillEmpty))
	for _, control := range stillEmpty {
		if documentControlTypes[control.ControlType] {
			// A missing document is a documents problem, not a question. It is
			// already reported through FillReport.Documents.
			continue
		}
		if IsWidgetArtifact(control) {
			// A combobox's own search box is not something the employer asked.
			continue
		}
		byKey[control.Key] = control
		questions = append(questions, control.AsQuestion())
	}
	if len(questions) == 0 {
		return report
	}

	context := answers.Context{ATS: plan.ATSName, Company: plan.CompanyName}
	batch := plan.Vault.ResolveAll(questions, context, plan.PII)

	for _, entry := range batch.Filled {
		control := byKey[entry.Question.Key]
		// Intentional absence means "leave this control untouched" — do not
		// type the reason text, do not write an empty string, do not report
		// it as a filled field. The operator's decision is that this optional
		// field has no applicable value. Skip it entirely.
		if entry.Resolution.IntentionalAbsence {
			continue
		}
		if err := applyAnswerToControl(target, control, entry.Resolution.Answer); err != nil {
			// A field the vault knew but could not commit is not silently
			// dropped: it goes back to the operator with the known value
			// pre-filled, which is strictly better than a blank box and
			// strictly more honest than claiming it was filled.
			log.Printf("[Assisted] Approved answer could not be committed to %q; returning it to the operator: reason=%q", control.ControlType, security.BrowserFailureReason(err))
			report.Unresolved = append(report.Unresolved, unresolvedFrom(control, entry.Resolution))
			continue
		}
		report.Filled = append(report.Filled, FilledField{Key: control.Key, Label: control.Label, How: FilledByVault})
		report.ReusedAnswers++
	}
	for _, entry := range batch.NeedsOperator {
		report.Unresolved = append(report.Unresolved, unresolvedFrom(byKey[entry.Question.Key], entry.Resolution))
	}
	return report
}

func unresolvedFrom(control FormControl, resolution answers.Resolution) UnresolvedQuestion {
	return UnresolvedQuestion{
		Key:         control.Key,
		Selector:    control.Selector,
		Label:       control.Label,
		ControlType: control.ControlType,
		Options:     control.Options,
		Required:    control.Required,
		Sensitivity: string(resolution.Sensitivity),
		Suggested:   resolution.Answer,
		Source:      string(resolution.Source),
		LabelUnsafe: control.LabelUnsafe,
	}
}

// ApplyApprovedAnswers commits a set of operator-approved answers to the form.
//
// It is the path used after the operator answers the questions the refill
// surfaced. It reuses the same field-commit machinery the automatic path has
// always used, and it has no access to a submit control: findSubmitControl is
// never reached from here, so the last step of an application stays where it
// has always been.
func ApplyApprovedAnswers(page playwright.Page, filter *security.QuarantineLayer, values map[string]string) (FillReport, error) {
	report := FillReport{}
	if len(values) == 0 {
		return report, nil
	}
	target := resolveFillTarget(page)
	controls, err := SnapshotControls(target)
	if err != nil {
		return report, err
	}
	controls = SanitizeControls(filter, controls)
	byKey := make(map[string]FormControl, len(controls))
	for _, control := range controls {
		byKey[control.Key] = control
	}
	for key, value := range values {
		control, known := byKey[key]
		if !known {
			// The form changed under the operator between the question being
			// asked and the answer arriving. Reporting it as unresolved keeps
			// the card honest rather than claiming a field was filled.
			report.Unresolved = append(report.Unresolved, UnresolvedQuestion{Key: key, Label: key, Suggested: value})
			continue
		}
		if err := applyAnswerToControl(target, control, value); err != nil {
			log.Printf("[Assisted] Operator answer could not be committed to a %q control: reason=%q", control.ControlType, security.BrowserFailureReason(err))
			report.Unresolved = append(report.Unresolved, UnresolvedQuestion{
				Key: control.Key, Selector: control.Selector, Label: control.Label,
				ControlType: control.ControlType, Options: control.Options,
				Required: control.Required, Suggested: value,
			})
			continue
		}
		report.Filled = append(report.Filled, FilledField{Key: control.Key, Label: control.Label, How: FilledByVault})
	}
	return report, nil
}

// applyAnswerToControl commits one value using the machinery appropriate to the
// control's shape, reusing the combobox, checkbox and label-fallback helpers the
// automatic submitter already relies on rather than adding a second way to fill
// a field.
func applyAnswerToControl(target fillTarget, control FormControl, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	switch control.ControlType {
	case "radio", "checkbox":
		return checkOptionByLabel(target, control, value)
	case "select", "combobox":
		committed, err := commitComboboxSelection(target, control.Selector, value)
		if err != nil {
			return err
		}
		if !committed {
			return errCouldNotCommit
		}
		return nil
	default:
		return safeFillWithLabelFallback(target, control.Selector, control.Label, value)
	}
}

// checkOptionByLabel ticks the option in a radio or checkbox group whose label
// is the answer. A negative answer to a lone checkbox means leaving it
// unchecked, which is what an unticked box already says — so this never
// unchecks anything an employer's form arrived with.
func checkOptionByLabel(target fillTarget, control FormControl, value string) error {
	if len(control.Options) == 0 && isNegativeCheckboxValue(value) {
		return nil
	}
	option := target.GetByLabelLoc(value)
	count, err := option.Count()
	if err != nil || count == 0 {
		return errCouldNotCommit
	}
	return option.Check(playwright.LocatorCheckOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
}

// HasDedicatedHandler reports whether this project has a hand-written handler
// for the ATS behind applyURL. Exported for the effort model, which needs the
// same answer dedicatedATSHandler already computes without wanting the handler
// itself.
func HasDedicatedHandler(applyURL string) bool {
	handler, _ := dedicatedATSHandler(applyURL)
	return handler != nil
}

// ATSName returns the display name of the ATS behind applyURL, or "" when this
// project has no dedicated handler for it.
func ATSName(applyURL string) string {
	_, name := dedicatedATSHandler(applyURL)
	return name
}

// errCouldNotCommit is returned when a value was applied but the control did
// not take it. Every caller treats it as "hand this back to the operator",
// never as success — a field that silently did not commit is exactly the
// failure bugs.md #74/#76 made observable in the automatic path.
var errCouldNotCommit = errors.New("the control did not accept the value")
