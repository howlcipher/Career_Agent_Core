package answers

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
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
	pii.Education = []config.Education{{Degree: "B.S.", School: "Example University"}}
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

// --- Education ------------------------------------------------------------

// runPromptCases runs a table of prompts through the resolver and applies a
// per-case check. Shared by the education phrasing tests.
func runPromptCases(t *testing.T, store *Store, pii *config.PII, cases []string, check func(t *testing.T, prompt string, res Resolution)) {
	t.Helper()
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			res := store.Resolve(routineQuestion(prompt), Context{}, pii)
			check(t, prompt, res)
		})
	}
}

func educationPII() *config.PII {
	pii := &config.PII{}
	pii.Education = []config.Education{
		{Degree: "B.S.", FieldOfStudy: "Computer Science", School: "Example University", StartYear: "2018", EndYear: "2022", Status: "Graduated"},
	}
	return pii
}

func TestResolveFromPattern_EducationFamilyMatchesCommonPhrasings(t *testing.T) {
	store := newTestStore(t)
	pii := educationPII()

	cases := []string{
		"Education background",
		"Please provide your post-secondary education",
		"Highest level of education",
		"Educational background",
		"Education summary",
		"Academic background",
		"Degree earned",
		"College/University attended",
	}
	runPromptCases(t, store, pii, cases, func(t *testing.T, prompt string, resolution Resolution) {
		if !resolution.Resolved || resolution.PatternID != "education" {
			t.Fatalf("%q did not reach the education pattern: %+v", prompt, resolution)
		}
		if !strings.Contains(resolution.Answer, "B.S.") {
			t.Errorf("expected configured education in answer, got %q", resolution.Answer)
		}
	})
}

func TestResolveFromPattern_EducationVariantsCanonicalizeTogether(t *testing.T) {
	store := newTestStore(t)
	pii := educationPII()

	variants := []string{
		"Education Background",
		"EDUCATION BACKGROUND (required)",
		"education background?",
	}
	want := store.Resolve(routineQuestion(variants[0]), Context{}, pii).Answer
	for _, prompt := range variants[1:] {
		got := store.Resolve(routineQuestion(prompt), Context{}, pii).Answer
		if got != want {
			t.Errorf("%q resolved to %q, want %q", prompt, got, want)
		}
	}
}

func TestResolveFromPattern_EducationRequiresExplicitApproval(t *testing.T) {
	store := newTestStore(t)
	pii := educationPII()

	resolution := store.Resolve(routineQuestion("Education background"), Context{}, pii)
	if !resolution.Resolved || resolution.AutoFill {
		t.Fatalf("education must be a suggestion, not an auto-fill: %+v", resolution)
	}
	if resolution.Sensitivity != Sensitive {
		t.Errorf("education should be sensitive until approved, got %q", resolution.Sensitivity)
	}

	// With reuse withheld the vault refuses to store it as a reusable answer,
	// even when the caller passes the sensitive classification.
	if _, err := store.Save(SaveRequest{
		Question: routineQuestion("Education background"), Answer: resolution.Answer,
		Sensitivity: Sensitive, Provenance: OperatorApproved, ReuseAllowed: false, ReuseDecisionMade: true,
	}); !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}

	// Only an explicit approval *and* an explicit reuse decision can store it.
	if _, err := store.Save(SaveRequest{
		Question: routineQuestion("Education background"), Answer: resolution.Answer,
		Sensitivity: Sensitive, Provenance: OperatorApproved, ReuseAllowed: true, ReuseDecisionMade: true,
	}); err != nil {
		t.Fatalf("approved reusable education answer should store: %v", err)
	}
}

func TestResolveFromPattern_EducationIsNotFabricated(t *testing.T) {
	store := newTestStore(t)
	resolution := store.Resolve(routineQuestion("Education background"), Context{}, &config.PII{})
	if resolution.Resolved {
		t.Fatalf("unconfigured education must stay unresolved, got %+v", resolution)
	}
}

func TestResolveFromPattern_EducationDoesNotClaimRoleSpecificOrEssayQuestions(t *testing.T) {
	store := newTestStore(t)
	pii := educationPII()

	cases := []string{
		"Do you have a degree in computer science?",
		"How many years since graduation?",
		"Describe how your education prepared you for this role.",
		"Why did you choose your field of study?",
		"Do you have a CS degree?",
	}
	runPromptCases(t, store, pii, cases, func(t *testing.T, prompt string, resolution Resolution) {
		if resolution.PatternID == "education" {
			t.Fatalf("%q must not be treated as generic education summary: %+v", prompt, resolution)
		}
		if resolution.AutoFill {
			t.Fatalf("%q must not auto-fill: %+v", prompt, resolution)
		}
	})
}

func TestEducation_ApprovalAndReuseLifecycle(t *testing.T) {
	store := newTestStore(t)
	pii := educationPII()

	prompts := []string{
		"Education background",
		"Please provide your post-secondary education",
		"Highest level of education",
	}

	// Before approval: suggestions only.
	for _, prompt := range prompts {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || resolution.AutoFill {
			t.Fatalf("before approval %q should suggest but not fill: %+v", prompt, resolution)
		}
	}

	// Approve the canonical education question with reuse and bind the variants
	// as aliases, mirroring the knowledge-service approval flow.
	canonical := "What is your education background?"
	saved, err := store.Save(SaveRequest{
		Question:          routineQuestion(canonical),
		Answer:            pii.EducationSummary(),
		Kind:              KindText,
		Sensitivity:       Sensitive,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("approved education answer should store: %v", err)
	}
	if _, err := store.AddAliases(saved.ID, prompts, true); err != nil {
		t.Fatalf("bind education aliases: %v", err)
	}

	// After approval, equivalent prompts auto-fill.
	for _, prompt := range prompts {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || !resolution.AutoFill {
			t.Fatalf("after approval %q should auto-fill: %+v", prompt, resolution)
		}
	}

	// Revoke should stop automatic reuse; the pattern may still suggest, but
	// Career Agent must not type the answer without a fresh approval.
	if err := store.Revoke(saved.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, prompt := range prompts {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if resolution.AutoFill {
			t.Fatalf("after revoke %q must not auto-fill: %+v", prompt, resolution)
		}
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

func TestWorkAuthorizationAndSponsorship_StaySensitiveUntilApproved(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	pii.Work.AuthorizedToWorkUS = "Yes"
	pii.Work.RequiresSponsorship = "No"

	for _, prompt := range []string{
		"Are you legally authorized to work in the United States?",
		"Will you now or in the future require visa sponsorship for employment?",
	} {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || resolution.AutoFill {
			t.Fatalf("%q must be a suggestion before approval: %+v", prompt, resolution)
		}
		if resolution.Sensitivity != Sensitive {
			t.Errorf("%q should be sensitive, got %q", prompt, resolution.Sensitivity)
		}
	}
}

/* jscpd:ignore-start */
// Work-authorization and sponsorship lifecycle tests mirror each other by
// design: the safety property is that two similarly-shaped sensitive answers
// are handled identically by the approval/reuse/revoke path.
func TestWorkAuthorization_ApprovalAndReuseLifecycle(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{Work: config.WorkFacts{AuthorizedToWorkUS: "Yes"}}

	variants := []string{
		"Are you legally authorized to work in the United States?",
		"Are you authorized to work in the US?",
		"Do you currently have authorization to work in the United States?",
	}

	for _, prompt := range variants {
		if store.Resolve(routineQuestion(prompt), Context{}, pii).AutoFill {
			t.Fatalf("%q must not auto-fill before approval", prompt)
		}
	}

	saved, err := store.Save(SaveRequest{
		Question:          routineQuestion(variants[0]),
		Answer:            "Yes",
		Kind:              KindBoolean,
		Sensitivity:       Sensitive,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("save work authorization approval: %v", err)
	}
	if _, err := store.AddAliases(saved.ID, variants[1:], true); err != nil {
		t.Fatalf("bind work-authorization aliases: %v", err)
	}

	for _, prompt := range variants[1:] {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || !resolution.AutoFill || resolution.Answer != "Yes" {
			t.Fatalf("%q should auto-fill after approval: %+v", prompt, resolution)
		}
	}

	if err := store.Revoke(saved.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, prompt := range variants {
		if store.Resolve(routineQuestion(prompt), Context{}, pii).AutoFill {
			t.Fatalf("%q must not auto-fill after revoke", prompt)
		}
	}
}

func TestSponsorship_ApprovalAndReuseLifecycle(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{Work: config.WorkFacts{RequiresSponsorship: "No"}}

	variants := []string{
		"Will you now or in the future require visa sponsorship for employment?",
		"Do you need sponsorship to work in the US?",
	}

	for _, prompt := range variants {
		if store.Resolve(routineQuestion(prompt), Context{}, pii).AutoFill {
			t.Fatalf("%q must not auto-fill before approval", prompt)
		}
	}

	saved, err := store.Save(SaveRequest{
		Question:          routineQuestion(variants[0]),
		Answer:            "No",
		Kind:              KindBoolean,
		Sensitivity:       Sensitive,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("save sponsorship approval: %v", err)
	}
	if _, err := store.AddAliases(saved.ID, variants[1:], true); err != nil {
		t.Fatalf("bind sponsorship aliases: %v", err)
	}

	for _, prompt := range variants[1:] {
		resolution := store.Resolve(routineQuestion(prompt), Context{}, pii)
		if !resolution.Resolved || !resolution.AutoFill || resolution.Answer != "No" {
			t.Fatalf("%q should auto-fill after approval: %+v", prompt, resolution)
		}
	}

	if err := store.Revoke(saved.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, prompt := range variants {
		if store.Resolve(routineQuestion(prompt), Context{}, pii).AutoFill {
			t.Fatalf("%q must not auto-fill after revoke", prompt)
		}
	}
}

/* jscpd:ignore-end */

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

/* jscpd:ignore-start */
// These declaration-alias tests repeat the same felony-declaration setup to
// keep each test a self-contained safety scenario.
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

/* jscpd:ignore-end */

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

// --- Intentional Absence (#545) -------------------------------------------

/* jscpd:ignore-start */
// These scenario tests intentionally repeat a small setup pattern so each
// failure maps to a single concrete safety property. Excluding them from the
// duplication ratchet keeps the production-code ceiling meaningful without
// forcing contrived abstraction into the regression suite.

// absenceTestStore gives a fresh store and an empty PII config for the absence tests.
func absenceTestStore(t *testing.T) (*Store, *config.PII) {
	t.Helper()
	return newTestStore(t), &config.PII{}
}

// saveAbsence stores an absence answer, failing the test on error.
func saveAbsence(t *testing.T, store *Store, q Question, reason string) Answer {
	t.Helper()
	saved, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          q,
		Reason:            reason,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("SaveAbsence failed: %v", err)
	}
	return saved
}

// saveValueAnswer stores a normal value answer, failing the test on error.
func saveValueAnswer(t *testing.T, store *Store, q Question, answer string, kind Kind) Answer {
	t.Helper()
	saved, err := store.Save(SaveRequest{
		Question:          q,
		Answer:            answer,
		Kind:              kind,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	return saved
}

// resolveQuestion resolves a question against the given PII config.
func resolveQuestion(t *testing.T, store *Store, q Question, pii *config.PII) Resolution {
	t.Helper()
	return store.Resolve(q, Context{}, pii)
}

// twitterQuestion returns the Twitter question used across absence tests.
func twitterQuestion(required bool) Question {
	return Question{Key: "twitter_url", Prompt: "Twitter profile URL", ControlType: "text", Required: required}
}

// Test 1: Optional Twitter field + approved absence = resolved and left untouched.
func TestSaveAbsence_OptionalTwitterResolvesAsAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{} // no Twitter configured
	question := twitterQuestion(false)

	saved, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatalf("SaveAbsence failed: %v", err)
	}
	if saved.Kind != KindAbsence {
		t.Fatalf("expected Kind=absence, got %q", saved.Kind)
	}
	if saved.AnswerText != "No Twitter/X account" {
		t.Fatalf("expected reason stored, got %q", saved.AnswerText)
	}

	resolution := store.Resolve(question, Context{}, pii)
	if !resolution.Resolved {
		t.Fatal("absence answer should resolve")
	}
	if !resolution.IntentionalAbsence {
		t.Fatal("resolution should be marked as intentional absence")
	}
	if !resolution.AutoFill {
		t.Fatal("optional field with absence should have AutoFill=true")
	}
}

// Test 2: Equivalent Twitter wording reuses approved absence via alias.
func TestSaveAbsence_AliasReusesAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	question := twitterQuestion(false)

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Add alias for a different wording.
	listed, _ := store.List()
	if len(listed) == 0 {
		t.Fatal("expected saved answer")
	}
	_, err = store.AddAliases(listed[0].ID, []string{"X profile URL"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve via the alias.
	altQuestion := Question{Key: "x_url", Prompt: "X profile URL", ControlType: "text"}
	resolution := store.Resolve(altQuestion, Context{}, pii)
	if !resolution.IntentionalAbsence {
		t.Fatal("alias resolution should carry IntentionalAbsence")
	}
	if !resolution.AutoFill {
		t.Fatal("alias resolution should auto-fill (optional field)")
	}
}

// Test 3: Required Twitter field + approved absence = remains unresolved.
func TestSaveAbsence_RequiredFieldDemotesAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	optionalQ := twitterQuestion(false)
	requiredQ := twitterQuestion(true)

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          optionalQ,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(requiredQ, Context{}, pii)
	if !resolution.Resolved {
		t.Fatal("vault does know the answer; Resolved should be true")
	}
	if !resolution.IntentionalAbsence {
		t.Fatal("should still be flagged as intentional absence")
	}
	if resolution.AutoFill {
		t.Fatal("required field must NOT auto-fill from absence")
	}
}

// Test 4: Required field absence never becomes "N/A".
func TestSaveAbsence_RequiredFieldNeverBecomesNA(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	requiredQ := twitterQuestion(true)

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          twitterQuestion(false),
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(requiredQ, Context{}, pii)
	// The answer text should never be used to fill a required field.
	if resolution.AutoFill {
		t.Fatal("absence must never auto-fill a required field with the reason text")
	}
	// Confirm the answer is the reason, not "N/A".
	if resolution.Answer == "N/A" || resolution.Answer == "n/a" || resolution.Answer == "" {
		t.Fatalf("answer should be the stored reason, got %q", resolution.Answer)
	}
}

// Test 5: No empty-string magic encoding.
func TestSaveAbsence_RefusesEmptyReason(t *testing.T) {
	store := newTestStore(t)
	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          routineQuestion("Twitter profile URL"),
		Reason:            "",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
	if !strings.Contains(err.Error(), "reason") {
		t.Fatalf("error should mention reason, got %q", err.Error())
	}
}

// Test 6: Revoking absence restores unresolved behavior.
func TestSaveAbsence_RevokingRestoresUnresolved(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	question := twitterQuestion(false)

	saved, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(saved.ID); err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(question, Context{}, pii)
	if resolution.Resolved {
		t.Fatal("after revocation, question should be unresolved")
	}
	if resolution.IntentionalAbsence {
		t.Fatal("after revocation, IntentionalAbsence should be false")
	}
}

// Test 7: Existing normal value answers still behave unchanged.
func TestSaveAbsence_ValueAnswerStillWorks(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	question := routineQuestion("What is your LinkedIn URL?")

	_, err := store.Save(SaveRequest{
		Question:          question,
		Answer:            "https://linkedin.com/in/me",
		Kind:              KindURL,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(question, Context{}, pii)
	if !resolution.AutoFill {
		t.Fatal("value answer should auto-fill")
	}
	if resolution.IntentionalAbsence {
		t.Fatal("value answer must not be flagged as absence")
	}
	if resolution.Answer != "https://linkedin.com/in/me" {
		t.Fatalf("unexpected answer: %q", resolution.Answer)
	}
}

// Test 8: Value answer can replace an earlier absence answer.
func TestSaveAbsence_ValueReplacesAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	question := routineQuestion("Twitter profile URL")

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now save a value answer for the same question (operator got an account).
	_, err = store.Save(SaveRequest{
		Question:          question,
		Answer:            "https://x.com/newaccount",
		Kind:              KindURL,
		Provenance:        OperatorEdited,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(question, Context{}, pii)
	if resolution.IntentionalAbsence {
		t.Fatal("value answer should have replaced absence")
	}
	if resolution.Answer != "https://x.com/newaccount" {
		t.Fatalf("expected new value, got %q", resolution.Answer)
	}
}

// Test 9: Absence answer can replace an earlier value only through explicit operator action.
func TestSaveAbsence_AbsenceReplacesValue(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	question := routineQuestion("Twitter profile URL")

	_, err := store.Save(SaveRequest{
		Question:          question,
		Answer:            "https://x.com/oldaccount",
		Kind:              KindURL,
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Explicit operator absence replaces the value.
	_, err = store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "Account deleted",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(question, Context{}, pii)
	if !resolution.IntentionalAbsence {
		t.Fatal("absence should have replaced the value")
	}
}

// Test 10: Missing profile value alone does not create absence.
func TestSaveAbsence_EmptyProfileIsNotAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{} // Links.Twitter is empty
	question := twitterQuestion(false)

	resolution := store.Resolve(question, Context{}, pii)
	if resolution.IntentionalAbsence {
		t.Fatal("empty profile must not auto-create absence")
	}
	if resolution.Resolved {
		t.Fatal("empty profile should leave question unresolved, not absent")
	}
}

// Test 11: LinkedIn does not inherit Twitter absence.
func TestSaveAbsence_LinkedInDoesNotInheritTwitterAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	twitterQ := routineQuestion("Twitter profile URL")

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          twitterQ,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	linkedinQ := routineQuestion("LinkedIn profile URL")
	resolution := store.Resolve(linkedinQ, Context{}, pii)
	if resolution.IntentionalAbsence {
		t.Fatal("LinkedIn must NOT inherit Twitter's absence")
	}
}

// Test 12: Website does not inherit Twitter absence.
func TestSaveAbsence_WebsiteDoesNotInheritTwitterAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	twitterQ := routineQuestion("Twitter profile URL")

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          twitterQ,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	websiteQ := routineQuestion("Portfolio or personal website URL")
	resolution := store.Resolve(websiteQ, Context{}, pii)
	if resolution.IntentionalAbsence {
		t.Fatal("Website must NOT inherit Twitter's absence")
	}
}

// Test 13: GitHub does not inherit Twitter absence.
func TestSaveAbsence_GitHubDoesNotInheritTwitterAbsence(t *testing.T) {
	store := newTestStore(t)
	pii := &config.PII{}
	twitterQ := routineQuestion("Twitter profile URL")

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          twitterQ,
		Reason:            "No Twitter/X account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	githubQ := routineQuestion("GitHub profile URL")
	resolution := store.Resolve(githubQ, Context{}, pii)
	if resolution.IntentionalAbsence {
		t.Fatal("GitHub must NOT inherit Twitter's absence")
	}
}

// Test 14: EEO "decline to identify" is not converted into absence.
func TestSaveAbsence_EEODeclineIsNotAbsence(t *testing.T) {
	store := newTestStore(t)
	// "Decline to identify" is a real answer value, not an absence.
	eeoQ := Question{Key: "gender", Prompt: "What is your gender identity?", ControlType: "select", Options: []string{"Male", "Female", "Decline to self-identify"}}

	// SaveAbsence should refuse because EEO questions classify as Sensitive
	// and absence with Sensitive requires explicit reuse permission.
	// More fundamentally, absence is the wrong tool for EEO: "Decline to
	// self-identify" is a selectable answer value, not blank.
	_, err := store.Save(SaveRequest{
		Question:          eeoQ,
		Answer:            "Decline to self-identify",
		Provenance:        OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
		Sensitivity:       Sensitive,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolution := store.Resolve(eeoQ, Context{}, &config.PII{})
	if resolution.IntentionalAbsence {
		t.Fatal("EEO 'Decline to self-identify' must be a value answer, not absence")
	}
	if resolution.Answer != "Decline to self-identify" {
		t.Fatalf("EEO should resolve to the selectable option, got %q", resolution.Answer)
	}
}

// Test 15: Privacy consent cannot be bypassed via absence.
func TestSaveAbsence_PrivacyConsentCannotBeAbsent(t *testing.T) {
	store := newTestStore(t)
	consentQ := Question{Key: "consent", Prompt: "I acknowledge and consent to the privacy policy", ControlType: "checkbox"}

	// Privacy consent is Sensitive, so SaveAbsence without proper grant should fail.
	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          consentQ,
		Reason:            "I refuse",
		ReuseAllowed:      false,
		ReuseDecisionMade: false,
	})
	if err == nil {
		t.Fatal("privacy consent should not be saveable as absence without explicit sensitive approval")
	}
	if !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}
}

// Test 16: Work authorization cannot be bypassed via absence.
func TestSaveAbsence_WorkAuthCannotBeBypassedViaAbsence(t *testing.T) {
	store := newTestStore(t)
	authQ := Question{Key: "auth", Prompt: "Are you legally authorized to work in the United States?", ControlType: "select"}

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          authQ,
		Reason:            "I don't want to answer",
		ReuseAllowed:      false,
		ReuseDecisionMade: false,
	})
	if err == nil {
		t.Fatal("work auth should not be saveable as absence without explicit grant")
	}
	if !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}
}

// Test 17: Sponsorship cannot be bypassed via absence.
func TestSaveAbsence_SponsorshipCannotBeBypassedViaAbsence(t *testing.T) {
	store := newTestStore(t)
	sponsorQ := Question{Key: "sponsorship", Prompt: "Will you require visa sponsorship?", ControlType: "select"}

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          sponsorQ,
		Reason:            "I don't want to answer",
		ReuseAllowed:      false,
		ReuseDecisionMade: false,
	})
	if err == nil {
		t.Fatal("sponsorship should not be saveable as absence without explicit grant")
	}
	if !errors.Is(err, ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}
}

// Test 18: Legal attestations cannot be bypassed via absence.
func TestSaveAbsence_LegalAttestationCannotBeBypassedViaAbsence(t *testing.T) {
	store := newTestStore(t)
	attestQ := Question{Key: "attest", Prompt: "I certify that all information provided is truthful and accurate", ControlType: "checkbox"}

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          attestQ,
		Reason:            "I don't want to certify",
		ReuseAllowed:      false,
		ReuseDecisionMade: false,
	})
	if err == nil {
		t.Fatal("legal attestation should not be saveable as absence without explicit grant")
	}
}

// Test 19: GeneratePerJob questions cannot be marked absent.
func TestSaveAbsence_PerJobCannotBeAbsent(t *testing.T) {
	store := newTestStore(t)
	perJobQ := Question{Key: "why", Prompt: "Why are you interested in this role?", ControlType: "textarea"}

	_, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          perJobQ,
		Reason:            "I have no reason",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err == nil {
		t.Fatal("per-job question should refuse absence")
	}
	if !errors.Is(err, ErrNotReusable) {
		t.Fatalf("expected ErrNotReusable, got %v", err)
	}
}

// Test 20: Absence answer stored with KindAbsence, not empty string.
func TestSaveAbsence_StoresKindAbsenceNotEmptyString(t *testing.T) {
	store := newTestStore(t)
	question := routineQuestion("Twitter profile URL")

	saved, err := store.SaveAbsence(SaveAbsenceRequest{
		Question:          question,
		Reason:            "No account",
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Kind != KindAbsence {
		t.Fatalf("Kind should be 'absence', got %q", saved.Kind)
	}
	if saved.AnswerText == "" {
		t.Fatal("answer_text must not be empty (holds the reason)")
	}

	// Verify via Get that the DB persisted it correctly.
	retrieved, err := store.Get(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.Kind != KindAbsence {
		t.Fatalf("persisted Kind should be 'absence', got %q", retrieved.Kind)
	}
}

/* jscpd:ignore-end */
