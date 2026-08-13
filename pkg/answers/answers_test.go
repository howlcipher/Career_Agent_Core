package answers

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	_ "modernc.org/sqlite"
)

// newTestStore opens a per-test database on disk rather than ":memory:", for
// the reason documented on storage.setupTestDB: a pooled ":memory:" connection
// opens its own empty database, so a second connection sees no tables.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "answers.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store, err := OpenStore(conn)
	if err != nil {
		t.Fatalf("prepare answer vault: %v", err)
	}
	fixed := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }
	return store
}

func routineQuestion(prompt string) Question {
	return Question{Key: "k", Prompt: prompt, ControlType: "text"}
}

// --- The safety rule ------------------------------------------------------

func TestSave_RefusesSensitiveAnswerWithoutExplicitApprovalAndReuseDecision(t *testing.T) {
	store := newTestStore(t)
	sensitive := routineQuestion("Are you legally authorized to work in the United States?")

	cases := []struct {
		name    string
		request SaveRequest
	}{
		{"no provenance at all", SaveRequest{Question: sensitive, Answer: "Yes", ReuseAllowed: true, ReuseDecisionMade: true}},
		{"seeded rather than approved", SaveRequest{Question: sensitive, Answer: "Yes", Provenance: SeededFromPII, ReuseAllowed: true, ReuseDecisionMade: true}},
		{"approved but never asked about reuse", SaveRequest{Question: sensitive, Answer: "Yes", Provenance: OperatorApproved, ReuseAllowed: true}},
		{"approved but reuse withheld", SaveRequest{Question: sensitive, Answer: "Yes", Provenance: OperatorApproved, ReuseDecisionMade: true}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := store.Save(testCase.request); !errors.Is(err, ErrSensitiveNeedsApproval) {
				t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
			}
			live, err := store.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(live) != 0 {
				t.Fatalf("a refused sensitive answer must not be persisted; found %d rows", len(live))
			}
		})
	}
}

// A client that mislabels an attestation as routine must not be believed. The
// store classifies the question itself, and a caller may only raise the
// classification.
func TestSave_ClientCannotDowngradeSensitivityToBypassApproval(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Save(SaveRequest{
		Question:    routineQuestion("Have you ever been convicted of a felony?"),
		Answer:      "No",
		Sensitivity: Routine,
		Provenance:  OperatorApproved,
	})
	if !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected the store's own classification to force approval, got %v", err)
	}
}

func TestSave_StoresSensitiveAnswerWithFullApprovalAndProvenance(t *testing.T) {
	store := newTestStore(t)
	saved, err := store.Save(SaveRequest{
		Question:          routineQuestion("Are you legally authorized to work in the United States?"),
		Answer:            "Yes",
		Kind:              KindBoolean,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("a fully approved sensitive answer must be storable: %v", err)
	}
	if saved.Sensitivity != Sensitive {
		t.Errorf("expected the stored answer to be classified sensitive, got %q", saved.Sensitivity)
	}
	if saved.Provenance != OperatorApproved {
		t.Errorf("provenance must survive the write, got %q", saved.Provenance)
	}
	if saved.ApprovedAt.IsZero() {
		t.Error("an approval must carry a timestamp")
	}
}

func TestSave_RefusesToStoreAPerJobGeneratedAnswer(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Save(SaveRequest{
		Question:          Question{Prompt: "Why do you want to work at Grafana?", ControlType: "textarea"},
		Answer:            "Because of their observability work.",
		Provenance:        OperatorEdited,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if !errors.Is(err, ErrNotReusable) {
		t.Fatalf("expected ErrNotReusable for a per-job question, got %v", err)
	}
}

// --- Resolution chain -----------------------------------------------------

func TestResolve_ApprovedAnswerAutoFillsAndADifferentPhrasingBecomesAnAlias(t *testing.T) {
	store := newTestStore(t)
	asked := routineQuestion("What is your current job title?")
	if _, err := store.Save(SaveRequest{
		Question: asked, Answer: "Platform Engineer", Kind: KindText,
		Provenance: OperatorEdited, ReuseAllowed: true, ReuseDecisionMade: true,
	}); err != nil {
		t.Fatal(err)
	}

	// The exact phrasing that was approved resolves through its alias.
	resolution := store.Resolve(asked, Context{}, nil)
	if !resolution.Resolved || !resolution.AutoFill {
		t.Fatalf("an approved reusable answer must auto-fill: %+v", resolution)
	}
	if resolution.Source != SourceAlias {
		t.Errorf("expected the alias written at approval time to match, got %q", resolution.Source)
	}
	if resolution.Answer != "Platform Engineer" {
		t.Errorf("unexpected answer %q", resolution.Answer)
	}

	// A different board wording of the same question is a fresh approval the
	// first time, and an alias forever after.
	other := routineQuestion("Current Title *")
	if store.Resolve(other, Context{}, nil).Source == SourceAlias {
		t.Fatal("a phrasing the operator has never approved must not resolve from an alias")
	}
	if _, err := store.Save(SaveRequest{
		Question: other, Answer: "Platform Engineer", Kind: KindText,
		Provenance: OperatorApproved, ReuseAllowed: true, ReuseDecisionMade: true,
	}); err != nil {
		t.Fatal(err)
	}
	again := store.Resolve(other, Context{}, nil)
	if again.Source != SourceAlias || !again.AutoFill {
		t.Fatalf("the approved phrasing must resolve deterministically next time: %+v", again)
	}
}

func TestResolve_StoredAnswerWithReuseWithheldSuggestsButDoesNotAutoFill(t *testing.T) {
	store := newTestStore(t)
	question := routineQuestion("What is your notice period?")
	if _, err := store.Save(SaveRequest{
		Question: question, Answer: "Two weeks", Kind: KindText,
		Provenance: OperatorEdited, ReuseDecisionMade: true, ReuseAllowed: false,
	}); err != nil {
		t.Fatal(err)
	}
	resolution := store.Resolve(question, Context{}, nil)
	if !resolution.Resolved {
		t.Fatal("the operator's own previous answer should still come back as a suggestion")
	}
	if resolution.AutoFill {
		t.Fatal("reuse was withheld, so Career Agent must not fill this on its own")
	}
}

func TestResolve_PrefersTheMostSpecificScope(t *testing.T) {
	store := newTestStore(t)
	question := routineQuestion("How did you hear about this role?")
	for _, entry := range []struct {
		scope, answer string
	}{
		{ScopeGlobal, "Company website"},
		{ATSScope("greenhouse"), "Greenhouse job board"},
		{CompanyScope("Grafana Labs, Inc."), "A friend at Grafana"},
	} {
		if _, err := store.Save(SaveRequest{
			Question: question, Answer: entry.answer, Scope: entry.scope,
			Provenance: OperatorEdited, ReuseAllowed: true, ReuseDecisionMade: true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if got := store.Resolve(question, Context{ATS: "greenhouse", Company: "Grafana Labs"}, nil).Answer; got != "A friend at Grafana" {
		t.Errorf("company scope should win, got %q", got)
	}
	if got := store.Resolve(question, Context{ATS: "greenhouse", Company: "Other Co"}, nil).Answer; got != "Greenhouse job board" {
		t.Errorf("ATS scope should win when no company answer exists, got %q", got)
	}
	if got := store.Resolve(question, Context{ATS: "lever", Company: "Other Co"}, nil).Answer; got != "Company website" {
		t.Errorf("global scope should be the fallback, got %q", got)
	}
}

func TestRevoke_StopsResolutionButKeepsTheAuditRow(t *testing.T) {
	store := newTestStore(t)
	question := routineQuestion("LinkedIn profile URL")
	saved, err := store.Save(SaveRequest{
		Question: question, Answer: "https://example.invalid/in/someone", Kind: KindURL,
		Provenance: OperatorEdited, ReuseAllowed: true, ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(saved.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if store.Resolve(question, Context{}, nil).Resolved {
		t.Fatal("a revoked answer must never resolve again")
	}
	var revoked int
	if err := store.conn.QueryRow(`SELECT COUNT(*) FROM approved_answers WHERE id = ? AND revoked_at IS NOT NULL`, saved.ID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 1 {
		t.Fatal("the revoked row must be kept so its provenance stays auditable")
	}
	if err := store.Revoke(saved.ID); err == nil {
		t.Fatal("revoking twice should report that there is no live answer")
	}
}

// --- Patterns -------------------------------------------------------------

func TestResolveFromPattern_ReachesTheSameAnswerAcrossBoardPhrasings(t *testing.T) {
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	store := newTestStore(t)

	for _, prompt := range []string{
		"Are you legally authorized to work in the United States?",
		"Are you authorized to work in the US?",
		"Do you currently have authorization to work in the United States?",
		"Are you legally eligible for employment in the United States? *",
	} {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || resolution.PatternID != "work_authorization" {
			t.Errorf("%q did not reach the work-authorization pattern: %+v", prompt, resolution)
		}
	}
}

// The single most dangerous confusion in this table: a sponsorship question
// contains the authorization vocabulary, and answering it with the
// work-authorization answer would put a flatly wrong attestation on a real
// application.
func TestResolveFromPattern_SponsorshipIsNotAnsweredWithWorkAuthorization(t *testing.T) {
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Work.RequiresSponsorship = "No"
	store := newTestStore(t)

	resolution := store.Resolve(routineQuestion("Will you now or in the future require sponsorship for employment visa status?"), Context{}, pii)
	if resolution.PatternID != "sponsorship" {
		t.Fatalf("expected the sponsorship pattern, got %q", resolution.PatternID)
	}
	if resolution.Answer != "No" {
		t.Fatalf("expected the configured sponsorship answer, got %q", resolution.Answer)
	}
}

// Every sensitive pattern is a proposal, never a fill. This is the safe live
// verification improvements.md #497 asks for, run as a unit test over the
// table itself so a pattern added later cannot quietly opt out.
func TestResolveFromPattern_NoSensitivePatternEverAutoFills(t *testing.T) {
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Work.RequiresSponsorship = "No"
	pii.Work.SecurityClearance = "None"
	pii.Work.CriminalHistory = "No"
	pii.Work.DesiredSalary = "$160,000"
	pii.Work.Over18 = "Yes"
	store := newTestStore(t)

	for _, id := range SensitivePatternIDs() {
		prompt := seedQuestions[id]
		if prompt == "" {
			t.Fatalf("sensitive pattern %q has no canonical question to verify against", id)
		}
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved {
			t.Fatalf("pattern %q should propose the configured value", id)
		}
		if resolution.AutoFill {
			t.Fatalf("pattern %q is sensitive and must never auto-fill", id)
		}
		if resolution.Sensitivity != Sensitive {
			t.Fatalf("pattern %q lost its sensitive classification", id)
		}
	}
}

func TestResolveFromPattern_UnconfiguredFactStaysUnresolvedRatherThanGuessed(t *testing.T) {
	store := newTestStore(t)
	resolution := store.Resolve(routineQuestion("Are you legally authorized to work in the United States?"), Context{}, &config.PII{})
	if resolution.Resolved {
		t.Fatalf("an unconfigured attestation must stay unresolved, got %+v", resolution)
	}
	if resolution.Sensitivity != Sensitive {
		t.Errorf("an unresolved attestation is still sensitive, got %q", resolution.Sensitivity)
	}
}

// --- Classification -------------------------------------------------------

func TestClassify(t *testing.T) {
	cases := []struct {
		prompt      string
		controlType string
		want        Sensitivity
	}{
		{"Are you legally authorized to work in the United States?", "radio", Sensitive},
		{"Will you require visa sponsorship?", "radio", Sensitive},
		{"Have you ever been convicted of a felony?", "radio", Sensitive},
		{"Do you have an active security clearance?", "select", Sensitive},
		{"Please self-identify your gender", "select", Sensitive},
		{"Are you a protected veteran?", "select", Sensitive},
		{"Do you have a disability?", "select", Sensitive},
		{"What are your salary expectations?", "text", Sensitive},
		{"I certify that the information provided is accurate", "checkbox", Sensitive},
		{"Why do you want to work here?", "textarea", GeneratePerJob},
		{"Tell us what interests you about this role", "textarea", GeneratePerJob},
		{"Describe your experience with Kubernetes", "textarea", GeneratePerJob},
		{"LinkedIn profile URL", "text", Routine},
		{"How many years of experience do you have with Go?", "text", Routine},
		{"What is your current job title?", "text", Routine},
		{"", "text", Sensitive},
	}
	for _, testCase := range cases {
		got := Classify(Question{Prompt: testCase.prompt, ControlType: testCase.controlType})
		if got != testCase.want {
			t.Errorf("Classify(%q) = %q, want %q", testCase.prompt, got, testCase.want)
		}
	}
}

func TestNormalize_CollapsesPresentationalDifferencesOnly(t *testing.T) {
	same := []string{
		"Have you ever been convicted of a felony?",
		"Have you been convicted of a felony? *",
		"HAVE YOU BEEN CONVICTED OF A FELONY (required)",
	}
	want := Normalize(same[0])
	for _, prompt := range same[1:] {
		if got := Normalize(prompt); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", prompt, got, want)
		}
	}
	// Two genuinely different attestations must not collapse together.
	if Normalize("Are you authorized to work in the US?") == Normalize("Will you require sponsorship to work in the US?") {
		t.Fatal("normalization must not equate work authorization with sponsorship")
	}
}

// --- Seeding --------------------------------------------------------------

func TestSeedFromPII_WritesSuggestionsThatCannotAutoFill(t *testing.T) {
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Work.CurrentTitle = "Platform Engineer"
	pii.Links.LinkedIn = "https://example.invalid/in/someone"
	store := newTestStore(t)

	seeded, err := store.SeedFromPII(pii)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded == 0 {
		t.Fatal("expected the configured facts to seed suggestions")
	}
	for _, answer := range mustList(t, store) {
		if answer.Provenance != SeededFromPII {
			t.Errorf("expected seeded provenance, got %q", answer.Provenance)
		}
		if answer.ReuseAllowed {
			t.Errorf("a seeded suggestion must never carry reuse permission: %q", answer.CanonicalQuestion)
		}
	}
	// Seeding is idempotent and must not disturb an operator approval.
	approved := routineQuestion(seedQuestions["current_title"])
	if _, err := store.Save(SaveRequest{
		Question: approved, Answer: "Senior Platform Engineer", Kind: KindText,
		Provenance: OperatorEdited, ReuseAllowed: true, ReuseDecisionMade: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SeedFromPII(pii); err != nil {
		t.Fatal(err)
	}
	resolution := store.Resolve(approved, Context{}, pii)
	if resolution.Answer != "Senior Platform Engineer" || !resolution.AutoFill {
		t.Fatalf("re-seeding must not overwrite an operator approval: %+v", resolution)
	}
}

func TestResolveAll_SplitsFilledFromWhatNeedsTheOperator(t *testing.T) {
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Links.GitHub = "https://example.invalid/someone"
	store := newTestStore(t)

	batch := store.ResolveAll([]Question{
		{Key: "github", Prompt: "GitHub URL", ControlType: "text"},
		{Key: "auth", Prompt: "Are you authorized to work in the US?", ControlType: "radio"},
		{Key: "why", Prompt: "Why do you want to join us?", ControlType: "textarea"},
		{Key: "backstage", Prompt: "Have you used Backstage professionally?", ControlType: "radio"},
	}, Context{}, pii)

	if len(batch.Filled) != 1 || batch.Filled[0].Question.Key != "github" {
		t.Fatalf("only the routine, known fact should be filled: %+v", batch.Filled)
	}
	if len(batch.NeedsOperator) != 3 {
		t.Fatalf("expected three questions to need the operator, got %d", len(batch.NeedsOperator))
	}
	for _, entry := range batch.NeedsOperator {
		if entry.Question.Key == "auth" && entry.Resolution.Answer != "Yes" {
			t.Error("a sensitive question should arrive pre-filled with its suggestion")
		}
	}
}

func mustList(t *testing.T, store *Store) []Answer {
	t.Helper()
	live, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	return live
}

// --- bugs.md #540 / #541: found live on a real Greenhouse form ---

// The employer's own name is not attestation vocabulary. Affirm, Consent and
// Certify are real companies, and "Have you previously been employed at Affirm?"
// classified as a legal declaration purely because of the company's name.
func TestClassify_DoesNotTreatTheEmployersOwnNameAsADeclaration(t *testing.T) {
	cases := []struct {
		prompt, company string
		want            Sensitivity
	}{
		{"Have you previously been employed at Affirm for any length of time?", "Affirm", Routine},
		{"How did you first learn about Affirm as an employer?", "Affirm", Routine},
		{"Do you agree to our terms?", "Affirm", Sensitive},
		// The marker groups are redundant enough that a real attestation from a
		// company with an awkward name still matches on its other vocabulary.
		{"Do you consent to a background check?", "Consent Systems", Sensitive},
		{"Have you been convicted of a felony?", "Certify Inc", Sensitive},
		// Without the company, the false positive is still there — which is why
		// callers pass it.
		{"Have you previously been employed at Affirm for any length of time?", "", Sensitive},
	}
	for _, testCase := range cases {
		got := Classify(Question{Prompt: testCase.prompt, ControlType: "combobox", Company: testCase.company})
		if got != testCase.want {
			t.Errorf("Classify(%q, company=%q) = %q, want %q", testCase.prompt, testCase.company, got, testCase.want)
		}
	}
}

// The two-checkbox guarantee depends on the classification the operator was
// shown matching the one the store enforces with. When a curated pattern
// declares a question Routine but the classifier reads it as Sensitive, the
// resolution must come back Sensitive — otherwise the operator sees one
// checkbox and no declaration warning while the store treats their answer as a
// declaration, which is how a declaration got stored with reuse permission
// nobody granted.
func TestResolve_EscalatesAPatternWhoseQuestionClassifiesSensitive(t *testing.T) {
	pii := &config.PII{}
	pii.Work.PreviouslyEmployed = "No"
	store := newTestStore(t)

	// No company supplied, so "Affirm" still trips the declaration markers —
	// standing in for any pattern/classifier disagreement.
	question := Question{Prompt: "Have you previously been employed at Affirm for any length of time?", ControlType: "combobox"}
	resolution := store.Resolve(question, Context{}, pii)

	if resolution.PatternID != "previously_employed" {
		t.Fatalf("expected the previously-employed pattern to match, got %q", resolution.PatternID)
	}
	if resolution.Sensitivity != Sensitive {
		t.Fatalf("a pattern must not downgrade a question the classifier calls sensitive: %+v", resolution)
	}
	if resolution.AutoFill {
		t.Fatal("an escalated resolution must stop auto-filling")
	}
}

// A stored answer whose reuse the operator explicitly granted keeps auto-
// filling even after escalation — that grant is exactly what it is for.
func TestResolve_EscalationDoesNotRevokeAnExplicitReuseGrant(t *testing.T) {
	store := newTestStore(t)
	question := Question{Prompt: "Do you agree to the terms?", ControlType: "combobox"}
	if _, err := store.Save(SaveRequest{
		Question: question, Answer: "Yes", Provenance: OperatorApproved,
		ReuseAllowed: true, ReuseDecisionMade: true,
	}); err != nil {
		t.Fatal(err)
	}
	resolution := store.Resolve(question, Context{}, nil)
	if resolution.Sensitivity != Sensitive {
		t.Fatalf("expected the answer to stay sensitive, got %q", resolution.Sensitivity)
	}
	if !resolution.AutoFill {
		t.Fatal("an explicitly granted reuse must survive escalation")
	}
}
