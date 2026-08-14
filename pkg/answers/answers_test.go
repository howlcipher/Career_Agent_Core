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

// --- Skill-scoped experience (bugs.md #544) -------------------------------

// testPII is the shape these tests need: a configured career total that must
// never end up answering a question about one technology.
func testPII() *config.PII {
	pii := &config.PII{}
	pii.Work.YearsExperience = "12"
	return pii
}

func TestResolve_SkillScopedExperienceNeverAutoFillsTheCareerTotal(t *testing.T) {
	store := newTestStore(t)
	pii := testPII()

	// Every one of these is a question about a single technology. Answering any
	// of them with the operator's 12-year career total states a qualification
	// they do not have, on a real employer's screening question.
	skillScoped := []string{
		"How many years of Kubernetes experience do you have?",
		"Years of professional Kubernetes experience?",
		"How long have you worked with Kubernetes?",
		"How many years of Terraform experience do you have?",
		"Years of experience with Azure",
		"How many years of experience do you have with Go?",
		"Python experience (years)",
		"How many years have you used Docker?",
	}
	for _, prompt := range skillScoped {
		t.Run(prompt, func(t *testing.T) {
			resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
			if resolution.AutoFill {
				t.Fatalf("a skill-scoped experience question auto-filled %q from the career total", resolution.Answer)
			}
			if resolution.Answer == pii.Work.YearsExperience {
				t.Fatalf("the career total leaked into a skill-scoped question as a suggestion")
			}
			if resolution.Resolved {
				t.Fatalf("expected unresolved with nothing approved for this skill, got %+v", resolution)
			}
		})
	}
}

func TestResolve_GeneralExperienceStillAnswersFromTheCareerTotal(t *testing.T) {
	store := newTestStore(t)
	pii := testPII()

	// The other half of #544: refusing skill questions must not cost us the
	// general one, which pii.Work.YearsExperience genuinely does answer.
	general := []string{
		"How many years of professional experience do you have?",
		"Years of experience",
		"How many years of relevant work experience do you have?",
		"Total years of experience",
	}
	for _, prompt := range general {
		t.Run(prompt, func(t *testing.T) {
			resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
			if !resolution.Resolved || !resolution.AutoFill {
				t.Fatalf("a general experience question must still resolve and auto-fill, got %+v", resolution)
			}
			if resolution.Answer != "12" || resolution.PatternID != "years_experience" {
				t.Fatalf("expected the career total from years_experience, got %+v", resolution)
			}
		})
	}
}

func TestResolve_OneApprovedSkillValueAnswersEveryPhrasingOfThatSkill(t *testing.T) {
	store := newTestStore(t)
	pii := testPII()

	subject := SkillExperienceSubject(routineQuestion("How many years of Kubernetes experience do you have?"))
	if subject != "kubernetes" {
		t.Fatalf("subject = %q, want %q", subject, "kubernetes")
	}
	if _, err := store.Save(SaveRequest{
		Question:          routineQuestion(SkillExperienceQuestion(subject)),
		Answer:            "3",
		Kind:              KindNumber,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	}); err != nil {
		t.Fatalf("save approved skill experience: %v", err)
	}

	// Phrasings the operator never saw must reach the approved value, and must
	// reach it deterministically rather than through anything resembling a
	// similarity score.
	for _, prompt := range []string{
		"How many years of Kubernetes experience do you have?",
		"Years of professional Kubernetes experience?",
		"How long have you worked with Kubernetes?",
		"Kubernetes experience (years)",
	} {
		t.Run(prompt, func(t *testing.T) {
			resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
			if !resolution.Resolved || !resolution.AutoFill {
				t.Fatalf("expected the approved skill value to resolve, got %+v", resolution)
			}
			if resolution.Answer != "3" {
				t.Fatalf("answer = %q, want the approved %q", resolution.Answer, "3")
			}
		})
	}

	// A skill nobody approved is still nobody's business to guess at.
	other := store.Resolve(routineQuestion("How many years of Rust experience do you have?"), Context{}, pii)
	if other.Resolved {
		t.Fatalf("an unapproved skill resolved to %+v", other)
	}
}

func TestSkillExperienceSubject_DistinguishesSkillsFromEverythingElse(t *testing.T) {
	cases := []struct {
		prompt  string
		company string
		want    string
	}{
		{"How many years of Kubernetes experience do you have?", "", "kubernetes"},
		{"How long have you worked with Terraform?", "", "terraform"},
		{"Years of experience with Amazon Web Services", "", "amazon web services"},
		{"How many years of professional experience do you have?", "", ""},
		{"Years of experience", "", ""},
		{"How many years of full-time work experience?", "", ""},
		// Not duration questions at all.
		{"Describe your experience with Kubernetes", "", ""},
		{"What is your current job title?", "", ""},
		{"Are you legally authorized to work in the United States?", "", ""},
		// bugs.md #540's reasoning applies here too: the employer's own name is
		// in its questions, and tenure at a company is not a skill.
		{"How many years have you worked at Affirm?", "Affirm", ""},
		{"", "", ""},
	}
	for _, testCase := range cases {
		got := SkillExperienceSubject(Question{Prompt: testCase.prompt, Company: testCase.company, ControlType: "text"})
		if got != testCase.want {
			t.Errorf("SkillExperienceSubject(%q) = %q, want %q", testCase.prompt, got, testCase.want)
		}
	}
}

// --- Alias, edit and revoke primitives ------------------------------------

func approvedRoutine(t *testing.T, store *Store, prompt, answer string, reuse bool) Answer {
	t.Helper()
	saved, err := store.Save(SaveRequest{
		Question:          routineQuestion(prompt),
		Answer:            answer,
		Provenance:        OperatorApproved,
		ReuseAllowed:      reuse,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("save %q: %v", prompt, err)
	}
	return saved
}

func TestAddAliases_RefusesToGuessForADeclaration(t *testing.T) {
	store := newTestStore(t)
	declaration, err := store.Save(SaveRequest{
		Question:          routineQuestion("Have you ever been convicted of a felony?"),
		Answer:            "No",
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Career Agent may propose that two attestations are the same question.
	// Only the operator may accept it -- the vocabulary is small and
	// "authorized to work" and "require sponsorship" share most of it while
	// meaning opposite things.
	if _, err := store.AddAliases(declaration.ID, []string{"Have you been convicted of a criminal offense?"}, false); !errors.Is(err, ErrSensitiveAliasNeedsConfirmation) {
		t.Fatalf("expected ErrSensitiveAliasNeedsConfirmation, got %v", err)
	}
	aliases, err := store.Aliases(declaration.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range aliases {
		if alias == Normalize("Have you been convicted of a criminal offense?") {
			t.Fatal("an unconfirmed phrasing was bound to a declaration anyway")
		}
	}

	added, err := store.AddAliases(declaration.ID, []string{"Have you been convicted of a criminal offense?"}, true)
	if err != nil || added != 1 {
		t.Fatalf("a confirmed equivalence should bind: added=%d err=%v", added, err)
	}
	resolution := store.Resolve(routineQuestion("Have you been convicted of a criminal offense?"), Context{}, &config.PII{})
	if resolution.Source != SourceAlias || !resolution.AutoFill {
		t.Fatalf("the bound phrasing should resolve through the alias: %+v", resolution)
	}
}

func TestAddAliases_RefusesAnAnswerWhoseReuseIsWithheld(t *testing.T) {
	store := newTestStore(t)
	withheld := approvedRoutine(t, store, "What is your notice period?", "Two weeks", false)

	// An answer Career Agent will not type is not made more useful by
	// recognising more ways of asking for it, and reporting success would leave
	// the operator believing the vault knows something it does not.
	if _, err := store.AddAliases(withheld.ID, []string{"How much notice do you need to give?"}, true); err == nil {
		t.Fatal("aliasing a suggestion-only answer must be refused, not silently ignored")
	}
}

func TestUpdateAnswer_CannotRestoreADeclarationsReuseGrant(t *testing.T) {
	store := newTestStore(t)
	declaration, err := store.Save(SaveRequest{
		Question:          routineQuestion("Have you ever been convicted of a felony?"),
		Answer:            "No",
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Withdrawing reuse also drops the aliases, because an alias exists to make
	// an answer auto-fill.
	if _, err := store.AddAliases(declaration.ID, []string{"Any felony convictions?"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAnswer(declaration.ID, "No", false); err != nil {
		t.Fatal(err)
	}
	aliases, err := store.Aliases(declaration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("withdrawing reuse should drop the aliases, got %+v", aliases)
	}

	// Re-granting reuse to a declaration is the two-decision path in Save, and
	// an edit is not allowed to be a way around it.
	if _, err := store.UpdateAnswer(declaration.ID, "No", true); !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}
}

func TestUpdateAnswer_ChangesTheTextAndRecordsThatTheOperatorEditedIt(t *testing.T) {
	store := newTestStore(t)
	stored := approvedRoutine(t, store, "What is your notice period?", "Two weeks", true)

	updated, err := store.UpdateAnswer(stored.ID, "Four weeks", true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AnswerText != "Four weeks" || updated.Provenance != OperatorEdited {
		t.Fatalf("update did not record the edit: %+v", updated)
	}
	resolution := store.Resolve(routineQuestion("What is your notice period?"), Context{}, &config.PII{})
	if resolution.Answer != "Four weeks" {
		t.Fatalf("the edited answer should resolve, got %q", resolution.Answer)
	}
}

// --- Identity and contact facts -------------------------------------------

func identityPII() *config.PII {
	pii := &config.PII{
		FirstName:   "Ada",
		LastName:    "Lovelace",
		Email:       "ada@example.com",
		Phone:       "555-0100",
		Street:      "12 Analytical Way",
		City:        "Denver",
		State:       "CO",
		FullState:   "Colorado",
		Zip:         "80202",
		Country:     "US",
		FullCountry: "United States",
	}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Work.RequiresSponsorship = "No"
	return pii
}

func TestResolve_AnswersTheOperatorsOwnContactDetails(t *testing.T) {
	store := newTestStore(t)
	pii := identityPII()

	// Observed live on a real Grafana Labs form: 6 of 19 "questions" preflight
	// reported were the operator's own name, email, phone and location, because
	// the vault had no pattern for any of them.
	cases := map[string]string{
		"First Name":                 "Ada",
		"Preferred First Name":       "Ada",
		"Last Name":                  "Lovelace",
		"Email":                      "ada@example.com",
		"Phone":                      "555-0100",
		"Location (City)":            "Denver",
		"Country":                    "United States",
		"Zip code":                   "80202",
		"What state do you live in?": "Colorado",
		"Street address":             "12 Analytical Way",
	}
	for prompt, want := range cases {
		t.Run(prompt, func(t *testing.T) {
			resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
			if !resolution.Resolved || !resolution.AutoFill {
				t.Fatalf("expected %q to resolve and auto-fill, got %+v", prompt, resolution)
			}
			if resolution.Answer != want {
				t.Fatalf("answer = %q, want %q", resolution.Answer, want)
			}
		})
	}
}

func TestResolve_IdentityPatternsNeverClaimAnAttestation(t *testing.T) {
	store := newTestStore(t)
	pii := identityPII()

	// These share vocabulary with the identity patterns and are legal questions.
	// The attestation families are earlier in the table and must claim them
	// first; if one ever did not, escalateSensitivity would still stop the
	// auto-fill, but the operator would be shown a country name as the proposed
	// answer to a yes/no question.
	cases := []struct {
		prompt    string
		wantMatch string
	}{
		{"Are you currently eligible to work in your country of residence?", "work_authorization"},
		{"Do you now or in the future require visa sponsorship to work in this country?", "sponsorship"},
		// Nothing claims this one: "citizenship" is not sponsorship vocabulary,
		// and the country pattern denies it. Unresolved and sensitive is the
		// honest outcome -- the answer to "which country are you a citizen of"
		// is not the country you live in, and is not "No".
		{"What is your country of citizenship?", ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.prompt, func(t *testing.T) {
			question := routineQuestion(testCase.prompt)
			if got := MatchedPatternID(question); got != testCase.wantMatch {
				t.Fatalf("pattern = %q, want %q", got, testCase.wantMatch)
			}
			resolution := store.Resolve(question, Context{}, pii)
			if resolution.AutoFill {
				t.Fatalf("a legal question auto-filled %q", resolution.Answer)
			}
			if resolution.Sensitivity != Sensitive {
				t.Fatalf("sensitivity = %q, want sensitive", resolution.Sensitivity)
			}
		})
	}
}

func TestResolve_IdentityPatternsNeverAnswerForSomebodyElse(t *testing.T) {
	store := newTestStore(t)
	pii := identityPII()

	// A reference's phone number and the applicant's phone number are different
	// facts, and only one of them is in pii.yaml. Handing over the wrong one is
	// not a formatting error, it is a false statement about a third party.
	for _, prompt := range []string{
		"Reference name",
		"Reference email address",
		"Manager's phone number",
		"Emergency contact name",
		"Emergency contact phone",
		"Recruiter email",
		"Name of the company you currently work for",
		"What city is your university in?",
	} {
		t.Run(prompt, func(t *testing.T) {
			resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
			if resolution.AutoFill {
				t.Fatalf("a question about somebody else auto-filled %q", resolution.Answer)
			}
		})
	}
}

func TestResolve_ACompositeLocationQuestionIsLeftToTheOperator(t *testing.T) {
	store := newTestStore(t)
	pii := identityPII()

	// Observed live: "What country and time zone are you based in?" wants two
	// things. Filling only the country would be a wrong answer that looks like a
	// right one, so it is left unresolved.
	resolution := store.Resolve(routineQuestion("What country and time zone are you based in?"), Context{}, pii)
	if resolution.AutoFill {
		t.Fatalf("a composite question auto-filled the half it knew: %q", resolution.Answer)
	}
}

func TestResolve_AnswersTheConfiguredSocialLink(t *testing.T) {
	store := newTestStore(t)
	pii := identityPII()
	pii.Links.Twitter = "https://twitter.com/example"
	pii.Links.LinkedIn = "https://linkedin.com/in/example"
	pii.Links.GitHub = "https://github.com/example"

	// pii.Links.Twitter had existed since the vault shipped and nothing read it,
	// so "Twitter" was an operator interruption on four of six real forms.
	resolution := store.Resolve(routineQuestion("Twitter"), Context{}, pii)
	if !resolution.AutoFill || resolution.Answer != "https://twitter.com/example" {
		t.Fatalf("Twitter should resolve from configured links, got %+v", resolution)
	}

	// The link patterns must stay apart: each has exactly one right answer.
	for prompt, want := range map[string]string{
		"LinkedIn profile URL": "https://linkedin.com/in/example",
		"GitHub":               "https://github.com/example",
	} {
		got := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if got.Answer != want {
			t.Errorf("%q resolved to %q, want %q", prompt, got.Answer, want)
		}
	}
}
