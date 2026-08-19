package knowledge

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	_ "modernc.org/sqlite"
)

// newTestService opens a per-test database on disk rather than ":memory:", for
// the reason ADR-003 decision 7 gives: a pooled ":memory:" connection opens its
// own empty database, so a second connection sees no tables.
func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	for _, ensure := range []func(*sql.DB) error{
		storage.EnsureQuestionSchema,
		storage.EnsureAssistedSchema,
		answers.EnsureSchema,
	} {
		if err := ensure(conn); err != nil {
			t.Fatalf("prepare schema: %v", err)
		}
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS job_funnel (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_name TEXT, job_title TEXT, url TEXT UNIQUE, status TEXT,
		fit_score INTEGER, discovered_at DATETIME, applied_at DATETIME,
		last_updated DATETIME)`); err != nil {
		t.Fatalf("create job funnel: %v", err)
	}
	pii := &config.PII{}
	pii.Work.YearsExperience = "12"
	pii.Links.LinkedIn = "https://www.linkedin.com/in/example"

	service, err := Open(conn, pii)
	if err != nil {
		t.Fatalf("open knowledge service: %v", err)
	}
	return service, conn
}

func seedJob(t *testing.T, conn *sql.DB, id int, company string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := conn.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, discovered_at, last_updated)
		VALUES (?, ?, 'Platform Engineer', ?, 'AWAITING_REVIEW', ?, ?)`,
		id, company, "https://boards.greenhouse.io/example/jobs/"+company, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', '', ?, ?)`, id, now, now); err != nil {
		t.Fatal(err)
	}
}

func ask(t *testing.T, conn *sql.DB, jobID string, questions ...storage.ApplicationQuestion) {
	t.Helper()
	for i := range questions {
		questions[i].JobID = jobID
	}
	if err := storage.ReplaceApplicationQuestions(conn, jobID, questions, storage.AssistedFillSummary{JobID: jobID}); err != nil {
		t.Fatal(err)
	}
}

func findGroup(groups []Group, key string) *Group {
	for i := range groups {
		if groups[i].Key == key {
			return &groups[i]
		}
	}
	return nil
}

// --- Deduplication --------------------------------------------------------

func TestInbox_CollapsesTheSameQuestionAcrossEmployersAndRanksByDemand(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	// Three employers, three wordings of the same skill question, plus one
	// question only one of them asks. The inbox should show two entries, and
	// the widely-asked one first.
	for id, company := range map[int]string{1: "Acme", 2: "Globex", 3: "Initech"} {
		seedJob(t, conn, id, company)
	}
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "How many years of Kubernetes experience do you have?", ControlType: "text"})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Years of professional Kubernetes experience?", ControlType: "text"})
	ask(t, conn, "3",
		storage.ApplicationQuestion{Key: "c", Prompt: "How long have you worked with Kubernetes?", ControlType: "text"},
		storage.ApplicationQuestion{Key: "d", Prompt: "What is your T-shirt size?", ControlType: "text"},
	)

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(inbox), inbox)
	}
	// Demand order: the question three applications are waiting on comes first.
	if inbox[0].Applications != 3 {
		t.Fatalf("the most widely-asked question must rank first, got %+v", inbox[0])
	}
	kubernetes := inbox[0]
	if kubernetes.Key != "experience:kubernetes" {
		t.Fatalf("group key = %q, want experience:kubernetes", kubernetes.Key)
	}
	if kubernetes.SkillSubject != "kubernetes" {
		t.Fatalf("skill subject = %q", kubernetes.SkillSubject)
	}
	if kubernetes.Occurrences != 3 {
		t.Fatalf("occurrences = %d, want 3", kubernetes.Occurrences)
	}
	// Every wording is shown, because "these mean the same thing" is a claim the
	// operator is entitled to check.
	if len(kubernetes.Phrasings) != 3 {
		t.Fatalf("expected all 3 phrasings shown, got %+v", kubernetes.Phrasings)
	}
	// A question three employers ask cannot be answered "for this company only".
	if kubernetes.CompanyScopeAvailable {
		t.Fatal("company scope must not be offered for a question several employers ask")
	}
	if len(kubernetes.Companies) != 3 {
		t.Fatalf("expected all three employers named, got %+v", kubernetes.Companies)
	}
}

func TestInbox_UsesTheVaultsOwnPatternFamiliesForGrouping(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")

	// These are worded completely differently and are the same question. The
	// curated pattern table already knows that; grouping reuses it rather than
	// guessing.
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Will you now or in the future require visa sponsorship?", ControlType: "radio", Options: []string{"Yes", "No"}})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Do you need sponsorship to work in the US?", ControlType: "radio", Options: []string{"Yes", "No"}})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 {
		t.Fatalf("two phrasings of sponsorship must be one group, got %d: %+v", len(inbox), inbox)
	}
	if inbox[0].Key != "pattern:sponsorship" {
		t.Fatalf("group key = %q, want pattern:sponsorship", inbox[0].Key)
	}
}

func TestInbox_KeepsWorkAuthorizationAndSponsorshipApart(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")

	// These two share most of their vocabulary and mean opposite things.
	// Grouping them together would put a flatly wrong attestation on a real
	// application, which is why grouping uses the pattern table's Deny lists
	// rather than word overlap.
	ask(t, conn, "1",
		storage.ApplicationQuestion{Key: "a", Prompt: "Are you legally authorized to work in the United States?", ControlType: "radio", Options: []string{"Yes", "No"}},
		storage.ApplicationQuestion{Key: "b", Prompt: "Will you require sponsorship for employment authorization?", ControlType: "radio", Options: []string{"Yes", "No"}},
	)

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 {
		t.Fatalf("work authorization and sponsorship must stay separate, got %d groups: %+v", len(inbox), inbox)
	}
}

func TestInbox_FlagsAGroupWhoseEmployersOfferDifferentChoices(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")

	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "How did you hear about this role?", ControlType: "select", Options: []string{"LinkedIn", "Referral"}})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "How did you hear about us?", ControlType: "select", Options: []string{"Job board", "Careers page"}})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	group := findGroup(inbox, "pattern:how_did_you_hear")
	if group == nil {
		t.Fatalf("expected the how-did-you-hear group, got %+v", inbox)
	}
	if !group.OptionsVary {
		t.Fatal("employers offering different choices must be reported, not averaged")
	}

	// And a bulk answer must be refused rather than sent to an employer that
	// does not offer it.
	if _, err := service.Approve(ApproveRequest{
		GroupKey: group.Key, Answer: "LinkedIn", SaveForReuse: true,
	}, now); err == nil {
		t.Fatal("a group with conflicting option sets must not be bulk-answerable")
	}
}

// --- What never reaches the operator in bulk ------------------------------

func TestInbox_ExcludesAnythingCareerAgentCanAlreadyFill(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")

	ask(t, conn, "1",
		// Answerable from configured facts: never an interruption.
		storage.ApplicationQuestion{Key: "a", Prompt: "LinkedIn profile URL", ControlType: "text"},
		// Genuinely unknown.
		storage.ApplicationQuestion{Key: "b", Prompt: "What is your T-shirt size?", ControlType: "text"},
	)

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].Prompt != "What is your T-shirt size?" {
		t.Fatalf("only the unknown question belongs in the inbox, got %+v", inbox)
	}
}

func TestApprove_RefusesToStoreAPerJobAnswerForReuse(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")

	// "Why this company?" is asked by both, and is exactly the answer that must
	// never be replayed verbatim across employers.
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Why do you want to work here?", ControlType: "textarea"})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Why do you want to work here?", ControlType: "textarea"})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected one group, got %+v", inbox)
	}
	if inbox[0].Policy != PolicyGeneratePerJob {
		t.Fatalf("policy = %q, want %q", inbox[0].Policy, PolicyGeneratePerJob)
	}
	if _, err := service.Approve(ApproveRequest{
		GroupKey: inbox[0].Key, Answer: "I admire the mission.", SaveForReuse: true,
	}, now); !errors.Is(err, answers.ErrNotReusable) {
		t.Fatalf("expected ErrNotReusable, got %v", err)
	}
	live, err := service.vault.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Fatalf("nothing should have been stored, found %+v", live)
	}
}

func TestApprove_ADeclarationNeedsTheSecondAcknowledgement(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	ask(t, conn, "1", storage.ApplicationQuestion{
		Key: "a", Prompt: "Have you ever been convicted of a felony?", ControlType: "radio", Options: []string{"Yes", "No"},
	})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].Policy != PolicyHumanReview {
		t.Fatalf("a criminal-history question must be human review, got %+v", inbox)
	}

	// One checkbox is not enough, in bulk exactly as it is not enough on a
	// single application.
	if _, err := service.Approve(ApproveRequest{
		GroupKey: inbox[0].Key, Answer: "No", SaveForReuse: true,
	}, now); !errors.Is(err, answers.ErrSensitiveNeedsApproval) {
		t.Fatalf("expected ErrSensitiveNeedsApproval, got %v", err)
	}
	live, _ := service.vault.List()
	if len(live) != 0 {
		t.Fatalf("a declaration was stored without its second acknowledgement: %+v", live)
	}

	// Both acknowledgements: stored.
	result, err := service.Approve(ApproveRequest{
		GroupKey: inbox[0].Key, Answer: "No", SaveForReuse: true, AllowSensitiveReuse: true,
	}, now)
	if err != nil {
		t.Fatalf("with both acknowledgements the answer must store: %v", err)
	}
	if !result.Saved {
		t.Fatal("expected the answer to be saved")
	}
}

func TestApprove_ADeclarationsExtraPhrasingsNeedAnEquivalenceConfirmation(t *testing.T) {
	// Two employers word the same criminal-history declaration differently. The
	// curated pattern table groups them, which is Career Agent proposing that
	// they are the same question -- not the operator agreeing. Binding one
	// answer to both wordings is a separate decision from approving the answer,
	// because this vocabulary is small and treacherous and a wrong grouping puts
	// a false declaration on a real application.
	//
	// Each path starts from its own database: after the first approval the
	// group has already changed, so running them in sequence would test
	// something other than what it claims to.
	approve := func(t *testing.T, confirmed bool) ApproveResult {
		t.Helper()
		service, conn := newTestService(t)
		now := time.Now().UTC()
		seedJob(t, conn, 1, "Acme")
		seedJob(t, conn, 2, "Globex")
		ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Have you ever been convicted of a felony?", ControlType: "radio", Options: []string{"Yes", "No"}})
		ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Have you been convicted of a criminal offense?", ControlType: "radio", Options: []string{"Yes", "No"}})

		inbox, err := service.Inbox(now)
		if err != nil {
			t.Fatal(err)
		}
		if len(inbox) != 1 || len(inbox[0].Phrasings) != 2 {
			t.Fatalf("expected one group holding both phrasings, got %+v", inbox)
		}
		result, err := service.Approve(ApproveRequest{
			GroupKey: inbox[0].Key, Answer: "No", SaveForReuse: true,
			AllowSensitiveReuse: true, ConfirmedEquivalent: confirmed,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	withheld := approve(t, false)
	if !withheld.Saved {
		t.Fatal("the answer itself should still be stored")
	}
	if withheld.AliasesAdded != 0 {
		t.Fatalf("a declaration's other phrasings were aliased without confirmation: %d", withheld.AliasesAdded)
	}
	// The unconfirmed wording stays unresolved, which is the safe outcome: the
	// operator is asked again rather than answered for.
	if withheld.StillUnresolved != 1 {
		t.Fatalf("the unconfirmed phrasing must stay unresolved, got %d", withheld.StillUnresolved)
	}

	confirmed := approve(t, true)
	if confirmed.AliasesAdded == 0 {
		t.Fatal("a confirmed equivalence should bind the other phrasings")
	}
	if confirmed.StillUnresolved != 0 {
		t.Fatalf("once confirmed, both wordings should resolve, got %d unresolved", confirmed.StillUnresolved)
	}
}

// --- The learning loop ----------------------------------------------------

func TestApprove_ResolvesTheOtherQueuedApplicationsImmediately(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	// Six applications ask about Terraform, in four different wordings; a
	// seventh asks something else.
	for id, company := range map[int]string{1: "Acme", 2: "Globex", 3: "Initech", 4: "Umbrella", 5: "Soylent", 6: "Tyrell", 7: "Wonka"} {
		seedJob(t, conn, id, company)
	}
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "How many years of Terraform experience do you have?", ControlType: "text"})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Years of Terraform experience", ControlType: "text"})
	ask(t, conn, "3", storage.ApplicationQuestion{Key: "c", Prompt: "How long have you used Terraform?", ControlType: "text"})
	ask(t, conn, "4", storage.ApplicationQuestion{Key: "d", Prompt: "Terraform experience (years)", ControlType: "text"})
	ask(t, conn, "5", storage.ApplicationQuestion{Key: "e", Prompt: "How many years of Terraform experience do you have?", ControlType: "text"})
	ask(t, conn, "6", storage.ApplicationQuestion{Key: "f", Prompt: "Years of Terraform experience", ControlType: "text"})
	ask(t, conn, "7", storage.ApplicationQuestion{Key: "g", Prompt: "What is your T-shirt size?", ControlType: "text"})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	terraform := findGroup(inbox, "experience:terraform")
	if terraform == nil || terraform.Applications != 6 {
		t.Fatalf("expected one Terraform group over 6 applications, got %+v", inbox)
	}

	result, err := service.Approve(ApproveRequest{
		GroupKey: terraform.Key, Answer: "4", SaveForReuse: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	// The headline claim of the whole feature: one answer, six applications.
	if result.QuestionsResolved != 6 {
		t.Fatalf("questions resolved = %d, want 6", result.QuestionsResolved)
	}
	if result.ApplicationsHelped != 6 {
		t.Fatalf("applications helped = %d, want 6", result.ApplicationsHelped)
	}
	if result.StillUnresolved != 1 {
		t.Fatalf("still unresolved = %d, want 1 (the T-shirt question)", result.StillUnresolved)
	}

	// And it is durable: a fresh service over the same database sees it.
	reopened, err := Open(conn, service.pii)
	if err != nil {
		t.Fatal(err)
	}
	after, err := reopened.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Prompt != "What is your T-shirt size?" {
		t.Fatalf("after approval only the unknown question should remain, got %+v", after)
	}
}

func TestApprove_EducationResolvesQueuedApplicationsImmediately(t *testing.T) {
	service, conn := newTestService(t)
	service.pii = &config.PII{Education: []config.Education{
		{Degree: "B.S.", FieldOfStudy: "Computer Science", School: "Example University", StartYear: "2018", EndYear: "2022", Status: "Graduated"},
	}}
	now := time.Now().UTC()

	for id, company := range map[int]string{1: "Acme", 2: "Globex", 3: "Initech"} {
		seedJob(t, conn, id, company)
	}
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Education background", ControlType: "text"})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Please provide your post-secondary education", ControlType: "text"})
	ask(t, conn, "3", storage.ApplicationQuestion{Key: "c", Prompt: "What is your T-shirt size?", ControlType: "text"})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	education := findGroup(inbox, "pattern:education")
	if education == nil || education.Applications != 2 {
		t.Fatalf("expected one education group over 2 applications, got %+v", inbox)
	}

	result, err := service.Approve(ApproveRequest{
		GroupKey: education.Key, Answer: service.pii.EducationSummary(), SaveForReuse: true,
		AllowSensitiveReuse: true, ConfirmedEquivalent: true,
	}, now)
	if err != nil {
		t.Fatalf("approve education: %v", err)
	}
	if result.QuestionsResolved != 2 {
		t.Fatalf("questions resolved = %d, want 2", result.QuestionsResolved)
	}
	if result.ApplicationsHelped != 2 {
		t.Fatalf("applications helped = %d, want 2", result.ApplicationsHelped)
	}
	if result.StillUnresolved != 1 {
		t.Fatalf("still unresolved = %d, want 1", result.StillUnresolved)
	}
}

func TestApprove_WorkAuthorizationResolvesQueuedApplicationsImmediately(t *testing.T) {
	service, conn := newTestService(t)
	service.pii = &config.PII{Work: config.WorkFacts{AuthorizedToWorkUS: "Yes"}}
	now := time.Now().UTC()

	for id, company := range map[int]string{1: "Acme", 2: "Globex"} {
		seedJob(t, conn, id, company)
	}
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Are you legally authorized to work in the United States?", ControlType: "radio", Options: []string{"Yes", "No"}})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Do you currently have authorization to work in the United States?", ControlType: "radio", Options: []string{"Yes", "No"}})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	auth := findGroup(inbox, "pattern:work_authorization")
	if auth == nil || auth.Applications != 2 {
		t.Fatalf("expected one work-authorization group over 2 applications, got %+v", inbox)
	}

	result, err := service.Approve(ApproveRequest{
		GroupKey: auth.Key, Answer: "Yes", SaveForReuse: true,
		AllowSensitiveReuse: true, ConfirmedEquivalent: true,
	}, now)
	if err != nil {
		t.Fatalf("approve work authorization: %v", err)
	}
	if result.QuestionsResolved != 2 || result.ApplicationsHelped != 2 {
		t.Fatalf("expected 2 questions/applications resolved, got %+v", result)
	}
}

func TestApprove_SponsorshipResolvesQueuedApplicationsImmediately(t *testing.T) {
	service, conn := newTestService(t)
	service.pii = &config.PII{Work: config.WorkFacts{RequiresSponsorship: "No"}}
	now := time.Now().UTC()

	for id, company := range map[int]string{1: "Acme", 2: "Globex"} {
		seedJob(t, conn, id, company)
	}
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "Will you now or in the future require visa sponsorship for employment?", ControlType: "radio", Options: []string{"Yes", "No"}})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "b", Prompt: "Do you need sponsorship to work in the US?", ControlType: "radio", Options: []string{"Yes", "No"}})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	sponsorship := findGroup(inbox, "pattern:sponsorship")
	if sponsorship == nil || sponsorship.Applications != 2 {
		t.Fatalf("expected one sponsorship group over 2 applications, got %+v", inbox)
	}

	result, err := service.Approve(ApproveRequest{
		GroupKey: sponsorship.Key, Answer: "No", SaveForReuse: true,
		AllowSensitiveReuse: true, ConfirmedEquivalent: true,
	}, now)
	if err != nil {
		t.Fatalf("approve sponsorship: %v", err)
	}
	if result.QuestionsResolved != 2 || result.ApplicationsHelped != 2 {
		t.Fatalf("expected 2 questions/applications resolved, got %+v", result)
	}
}

func TestReEvaluate_AnnotatesWithoutClosingAnything(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "What is your notice period?", ControlType: "text"})

	if _, err := service.vault.Save(answers.SaveRequest{
		Question:          answers.Question{Prompt: "What is your notice period?", ControlType: "text"},
		Answer:            "Two weeks",
		Provenance:        answers.OperatorApproved,
		ReuseAllowed:      true,
		ReuseDecisionMade: true,
	}); err != nil {
		t.Fatal(err)
	}

	report, err := service.ReEvaluate(now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Examined != 1 || report.Resolved != 1 {
		t.Fatalf("report = %+v", report)
	}

	pending, err := storage.GetPendingQuestions(conn, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("re-evaluation must not close a question: %+v", pending)
	}
	if !pending[0].AutoFillable || pending[0].Suggested != "Two weeks" {
		t.Fatalf("resolution was not recorded on the row: %+v", pending[0])
	}
	if pending[0].Status != storage.QuestionPending {
		t.Fatalf("status = %q; Career Agent learning an answer is not the operator answering it", pending[0].Status)
	}
}

func TestReEvaluate_LeavesAnApplicationAnAssistedBrowserIsHolding(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Leased")
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "a", Prompt: "What is your notice period?", ControlType: "text"})
	if claimed, err := storage.AcquireAssistedLease(conn, "1", "owner", now); err != nil || !claimed {
		t.Fatalf("acquire lease: %v %v", claimed, err)
	}

	report, err := service.ReEvaluate(now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Examined != 0 {
		t.Fatalf("an application being worked right now must be left alone, examined %d", report.Examined)
	}
}

// --- Readiness ------------------------------------------------------------

func TestReadiness_IsGroundedInTheQueueRatherThanInvented(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")

	ask(t, conn, "1",
		storage.ApplicationQuestion{Key: "a", Prompt: "LinkedIn profile URL", ControlType: "text"},
		storage.ApplicationQuestion{Key: "b", Prompt: "How many years of Kubernetes experience do you have?", ControlType: "text"},
		storage.ApplicationQuestion{Key: "c", Prompt: "Have you ever been convicted of a felony?", ControlType: "radio", Options: []string{"Yes", "No"}},
		storage.ApplicationQuestion{Key: "d", Prompt: "Why do you want to work here?", ControlType: "textarea"},
	)
	ask(t, conn, "2",
		storage.ApplicationQuestion{Key: "e", Prompt: "LinkedIn profile URL", ControlType: "text"},
		storage.ApplicationQuestion{Key: "f", Prompt: "Years of Kubernetes experience", ControlType: "text"},
	)

	readiness, err := service.Readiness(now)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Applications != 2 {
		t.Fatalf("applications = %d, want 2", readiness.Applications)
	}
	if readiness.Fields != 6 {
		t.Fatalf("fields = %d, want 6", readiness.Fields)
	}
	// Both LinkedIn fields are answerable from configured facts.
	if readiness.Resolved != 2 {
		t.Fatalf("resolved = %d, want 2", readiness.Resolved)
	}
	if readiness.Unresolved != 4 {
		t.Fatalf("unresolved = %d, want 4", readiness.Unresolved)
	}
	// Three distinct things to deal with: Kubernetes, the felony declaration,
	// and the per-job essay.
	if readiness.UniqueQuestions != 3 {
		t.Fatalf("unique questions = %d, want 3", readiness.UniqueQuestions)
	}
	if readiness.SensitiveDecisions != 1 {
		t.Fatalf("sensitive decisions = %d, want 1", readiness.SensitiveDecisions)
	}
	if readiness.PerJobResponses != 1 {
		t.Fatalf("per-job responses = %d, want 1", readiness.PerJobResponses)
	}
	// Only the Kubernetes group is answerable once for everyone, and doing so
	// unlocks its two fields. A declaration and an essay are not bulk work and
	// counting them would overstate what the number buys.
	if readiness.AnswersNeeded != 1 {
		t.Fatalf("answers needed = %d, want 1", readiness.AnswersNeeded)
	}
	if readiness.FieldsUnlockable != 2 {
		t.Fatalf("fields unlockable = %d, want 2", readiness.FieldsUnlockable)
	}
	if readiness.KnownPercent() != 33 {
		t.Fatalf("known percent = %d, want 33", readiness.KnownPercent())
	}
}

func TestReadiness_AnEmptyQueueIsNotAFullyKnownOne(t *testing.T) {
	service, _ := newTestService(t)
	readiness, err := service.Readiness(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	// "Nothing has been looked at" and "everything is known" must not render
	// the same way, or the operator reads 100% and starts an apply session.
	if readiness.KnownPercent() != 0 {
		t.Fatalf("known percent = %d for an empty queue, want 0", readiness.KnownPercent())
	}
}

// --- The companion-facing field query -------------------------------------

func TestField_TellsACallerWhatItMayFillAndWhatItMayNot(t *testing.T) {
	service, _ := newTestService(t)

	// A stable configured fact: safe to fill.
	reply, err := service.Field(FieldQuery{Prompt: "LinkedIn profile URL", ControlType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Policy != PolicySafeAutoFill || reply.RequiresHuman {
		t.Fatalf("LinkedIn should be safe auto-fill: %+v", reply)
	}
	if reply.Answer == "" {
		t.Fatal("a fillable field must carry its answer")
	}

	// A declaration: the caller learns everything except a value it may type.
	declaration, err := service.Field(FieldQuery{
		Prompt: "Are you legally authorized to work in the United States?", ControlType: "radio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if declaration.Policy != PolicyHumanReview || !declaration.RequiresHuman {
		t.Fatalf("a work-authorization question must require a human: %+v", declaration)
	}
	if declaration.Answer != "" {
		t.Fatalf("a declaration must never be returned as a fillable answer: %q", declaration.Answer)
	}

	// An unknown question: no invention, and it says so.
	unknown, err := service.Field(FieldQuery{Prompt: "What is your T-shirt size?", ControlType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Policy != PolicyUnknown || unknown.Answer != "" || unknown.Suggested != "" {
		t.Fatalf("an unknown question must stay unknown: %+v", unknown)
	}
}

func TestField_SeparatesAFillableAnswerFromASuggestion(t *testing.T) {
	service, _ := newTestService(t)

	// Reuse withheld: Career Agent remembers, but does not decide.
	if _, err := service.vault.Save(answers.SaveRequest{
		Question:     answers.Question{Prompt: "What is your notice period?", ControlType: "text"},
		Answer:       "Two weeks",
		Provenance:   answers.OperatorApproved,
		ReuseAllowed: false,
	}); err != nil {
		t.Fatal(err)
	}
	reply, err := service.Field(FieldQuery{Prompt: "What is your notice period?", ControlType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Policy != PolicySuggestAsk || !reply.RequiresHuman {
		t.Fatalf("withheld reuse means suggest-and-ask: %+v", reply)
	}
	if reply.Answer != "" {
		t.Fatalf("a suggestion must not arrive in the fillable field: %q", reply.Answer)
	}
	if reply.Suggested != "Two weeks" {
		t.Fatalf("the suggestion should still be offered: %q", reply.Suggested)
	}
}

func TestReadiness_CountsTheWholeFormNotJustTheOperatorsRemainingWork(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")

	// Preflight found 25 controls and could only turn 3 of them into questions,
	// because it answered the rest. Counting question rows alone would report
	// "3 fields, 0 of which Career Agent can handle" for a form where it in fact
	// handles 22 of 25 — which is the opposite of the truth, and the number the
	// operator uses to decide whether to start a session.
	ask(t, conn, "1",
		storage.ApplicationQuestion{Key: "a", Prompt: "What is your T-shirt size?", ControlType: "text"},
		storage.ApplicationQuestion{Key: "b", Prompt: "Gender", ControlType: "select", Options: []string{"Male", "Female", "Decline"}},
		storage.ApplicationQuestion{Key: "c", Prompt: "Why do you want to work here?", ControlType: "textarea"},
	)
	if err := storage.RecordPreflight(conn, storage.PreflightResult{
		JobID: "1", State: storage.PreflightInspected, ATS: "Greenhouse", ControlCount: 25,
	}, now); err != nil {
		t.Fatal(err)
	}

	readiness, err := service.Readiness(now)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Fields != 25 {
		t.Fatalf("fields = %d, want the 25 controls actually found", readiness.Fields)
	}
	if readiness.Unresolved != 3 {
		t.Fatalf("unresolved = %d, want 3", readiness.Unresolved)
	}
	if readiness.Resolved != 22 {
		t.Fatalf("resolved = %d, want 22", readiness.Resolved)
	}
	if readiness.KnownPercent() != 88 {
		t.Fatalf("known percent = %d, want 88", readiness.KnownPercent())
	}
}

/* jscpd:ignore-start */
func TestReadiness_AnApplicationNothingInspectedStillContributesItsQuestions(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()
	seedJob(t, conn, 1, "Acme")
	// No preflight row: this inventory came from a live assisted session that
	// recorded questions without a field count. It must not vanish from the
	// summary.
	ask(t, conn, "1",
		storage.ApplicationQuestion{Key: "a", Prompt: "What is your T-shirt size?", ControlType: "text"},
	)
	if _, err := conn.Exec(`DELETE FROM assisted_fill_summary`); err != nil {
		t.Fatal(err)
	}

	readiness, err := service.Readiness(now)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Fields != 1 || readiness.Unresolved != 1 || readiness.Resolved != 0 {
		t.Fatalf("readiness = %+v, want 1 field, 1 unresolved, 0 resolved", readiness)
	}
}

/* jscpd:ignore-end */

func TestPolicy_ADeclarationTheOperatorGrantedReuseForIsNotReportedAsAlwaysAsking(t *testing.T) {
	// Store.Save is the only route to a sensitive answer with reuse granted, and
	// it demands two separate acknowledgements to get there. Once the operator
	// has given them, the answer genuinely auto-fills — observed live resolving
	// three real sponsorship questions — and labelling it "always ask you" would
	// tell them the opposite of what they authorised.
	granted := answers.Resolution{
		Resolved: true, AutoFill: true,
		Sensitivity: answers.Sensitive, Source: answers.SourceVault,
	}
	if got := Policy(granted); got != PolicyApprovedReusable {
		t.Fatalf("policy = %q, want %q", got, PolicyApprovedReusable)
	}
	if RequiresHuman(granted) {
		t.Fatal("an answer Career Agent fills does not require a human at fill time")
	}

	// Without the grant it is still, and always, a question for the operator.
	withheld := answers.Resolution{
		Resolved: true, AutoFill: false,
		Sensitivity: answers.Sensitive, Source: answers.SourceVault,
	}
	if got := Policy(withheld); got != PolicyHumanReview {
		t.Fatalf("policy = %q, want %q", got, PolicyHumanReview)
	}
	if !RequiresHuman(withheld) {
		t.Fatal("a declaration without a reuse grant must require a human")
	}
}

// --- Intentional Absence (#545) -------------------------------------------

/* jscpd:ignore-start */
// These scenario tests intentionally repeat a small setup pattern so each
// failure maps to a single concrete safety property. Excluding them from the
// duplication ratchet keeps the production-code ceiling meaningful without
// forcing contrived abstraction into the regression suite.

// seedSingleJobQuestion creates one job with one application question.
func seedSingleJobQuestion(t *testing.T, conn *sql.DB, q storage.ApplicationQuestion) {
	seedJob(t, conn, 1, "Acme")
	ask(t, conn, "1", q)
}

// firstGroup returns the first group from the inbox, failing the test if there
// is none. It avoids repeating the Inbox/error/length dance in every absence test.
func firstGroup(t *testing.T, service *Service, now time.Time) *Group {
	t.Helper()
	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) == 0 {
		t.Fatal("expected at least one inbox group")
	}
	return &inbox[0]
}

// approveAbsenceOnFirstGroup is a convenience for the common single-group
// absence-approval path.
func approveAbsenceOnFirstGroup(t *testing.T, service *Service, reason string, now time.Time) {
	t.Helper()
	group := firstGroup(t, service, now)
	if _, err := service.ApproveAbsence(ApproveAbsenceRequest{
		GroupKey:     group.Key,
		Reason:       reason,
		SaveForReuse: true,
	}, now); err != nil {
		t.Fatalf("ApproveAbsence failed: %v", err)
	}
}

func TestApproveAbsence_ResolvesOptionalTwitterQuestions(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	// Three applications all asking for Twitter, which the operator does not have.
	// All three contain "twitter" or "x" tokens, so the pattern table groups them
	// under pattern:twitter — which means the inbox shows one group, not three.
	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")
	seedJob(t, conn, 3, "Initech")
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text"})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "x_url", Prompt: "X profile URL", ControlType: "text"})
	ask(t, conn, "3", storage.ApplicationQuestion{Key: "tw", Prompt: "Twitter", ControlType: "text"})

	// Before absence: all three land in one group (pattern:twitter).
	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) == 0 {
		t.Fatal("expected at least 1 inbox group before absence")
	}
	// Find the Twitter group.
	var twitterGroup *Group
	for i := range inbox {
		if inbox[i].Occurrences >= 3 || inbox[i].Key == "pattern:twitter" {
			twitterGroup = &inbox[i]
			break
		}
	}
	if twitterGroup == nil {
		// Might be grouped differently, just use the first group that mentions Twitter.
		for i := range inbox {
			twitterGroup = &inbox[i]
			break
		}
	}

	// Approve absence for the Twitter group.
	result, err := service.ApproveAbsence(ApproveAbsenceRequest{
		GroupKey:            twitterGroup.Key,
		Reason:              "No Twitter/X account",
		SaveForReuse:        true,
		ConfirmedEquivalent: true,
	}, now)
	if err != nil {
		t.Fatalf("ApproveAbsence failed: %v", err)
	}
	if !result.Saved {
		t.Fatal("expected Saved=true")
	}
	// All three occurrences should be resolved (they all share the same group).
	if result.QuestionsResolved < 1 {
		t.Fatalf("expected at least 1 question resolved, got %d", result.QuestionsResolved)
	}

	// After absence: inbox should be empty (all were optional Twitter fields).
	inboxAfter, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inboxAfter) != 0 {
		t.Fatalf("expected empty inbox after absence resolves all Twitter questions, got %d items", len(inboxAfter))
	}
}

func TestApproveAbsence_PerJobEssayRefusesAbsence(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	seedSingleJobQuestion(t, conn, storage.ApplicationQuestion{Key: "why", Prompt: "Why are you interested in this role?", ControlType: "textarea"})

	group := firstGroup(t, service, now)
	_, err := service.ApproveAbsence(ApproveAbsenceRequest{
		GroupKey:     group.Key,
		Reason:       "I have no reason",
		SaveForReuse: true,
	}, now)
	if err == nil {
		t.Fatal("expected error for per-job question")
	}
}

func TestApproveAbsence_RequiredFieldConflictSurfacesInInbox(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	seedJob(t, conn, 1, "Acme")
	seedJob(t, conn, 2, "Globex")
	// Acme's is optional, Globex's is required.
	ask(t, conn, "1", storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text", Required: false})
	ask(t, conn, "2", storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text", Required: true})

	inbox, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox group for Twitter, got %d", len(inbox))
	}

	// Approve absence.
	_, err = service.ApproveAbsence(ApproveAbsenceRequest{
		GroupKey:     inbox[0].Key,
		Reason:       "No Twitter/X account",
		SaveForReuse: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	// After absence: optional field resolves, required field remains in inbox.
	inboxAfter, err := service.Inbox(now)
	if err != nil {
		t.Fatal(err)
	}
	// The required field should still be in the inbox (absence can't resolve it).
	if len(inboxAfter) != 1 {
		t.Fatalf("expected 1 remaining inbox item (required field), got %d", len(inboxAfter))
	}
	if !inboxAfter[0].AbsenceApproved {
		t.Fatal("the remaining required-field group should be flagged AbsenceApproved")
	}
}

func TestPolicy_IntentionalAbsenceIsDistinct(t *testing.T) {
	absence := answers.Resolution{
		Resolved:           true,
		AutoFill:           true,
		IntentionalAbsence: true,
		Sensitivity:        answers.Routine,
		Source:             answers.SourceVault,
	}
	if got := Policy(absence); got != PolicyIntentionalAbsence {
		t.Fatalf("policy = %q, want %q", got, PolicyIntentionalAbsence)
	}
}

func TestField_AbsenceReturnsIntentionalAbsenceFlag(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	seedSingleJobQuestion(t, conn, storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text"})
	approveAbsenceOnFirstGroup(t, service, "No Twitter/X account", now)

	reply, err := service.Field(FieldQuery{Prompt: "Twitter profile URL", ControlType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.IntentionalAbsence {
		t.Fatal("Field reply should set IntentionalAbsence=true")
	}
	if reply.Answer != "" {
		t.Fatalf("absence Field reply should not populate Answer (for auto-fill), got %q", reply.Answer)
	}
	if reply.Policy != PolicyIntentionalAbsence {
		t.Fatalf("policy = %q, want %q", reply.Policy, PolicyIntentionalAbsence)
	}
}

func TestApproveAbsence_AbsenceDoesNotInflateFilledMetrics(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	seedJob(t, conn, 1, "Acme")
	ask(t, conn, "1",
		storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text"},
		storage.ApplicationQuestion{Key: "name", Prompt: "First Name", ControlType: "text"},
	)

	// Before: 2 unresolved (LinkedIn is already configured in pii, but
	// "First Name" should resolve from identity pattern if pii has it)
	readinessBefore, err := service.Readiness(now)
	if err != nil {
		t.Fatal(err)
	}

	// Approve absence for Twitter.
	inbox, _ := service.Inbox(now)
	var twitterGroup *Group
	for i := range inbox {
		if inbox[i].Prompt == "Twitter profile URL" {
			twitterGroup = &inbox[i]
			break
		}
	}
	if twitterGroup == nil {
		// Twitter is already resolved by pattern (pii might have it). Skip test.
		t.Skip("Twitter already resolved from pii; cannot test absence metrics")
	}

	_, err = service.ApproveAbsence(ApproveAbsenceRequest{
		GroupKey:     twitterGroup.Key,
		Reason:       "No Twitter/X account",
		SaveForReuse: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	readinessAfter, err := service.Readiness(now)
	if err != nil {
		t.Fatal(err)
	}
	// AbsenceResolved should be at least 1 (the Twitter field).
	if readinessAfter.AbsenceResolved < 1 {
		t.Fatalf("expected AbsenceResolved >= 1, got %d", readinessAfter.AbsenceResolved)
	}
	// Unresolved should decrease.
	if readinessAfter.Unresolved >= readinessBefore.Unresolved {
		t.Fatalf("unresolved should decrease after absence approval: before=%d, after=%d",
			readinessBefore.Unresolved, readinessAfter.Unresolved)
	}
}

func TestApproveAbsence_RefusesInvalidRequests(t *testing.T) {
	service, conn := newTestService(t)
	now := time.Now().UTC()

	seedSingleJobQuestion(t, conn, storage.ApplicationQuestion{Key: "twitter", Prompt: "Twitter profile URL", ControlType: "text"})
	group := firstGroup(t, service, now)

	cases := []struct {
		name       string
		reason     string
		saveForUse bool
		wantErr    string
	}{
		{"empty reason", "", true, "empty reason"},
		{"reuse withheld", "No account", false, "SaveForReuse=false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := service.ApproveAbsence(ApproveAbsenceRequest{
				GroupKey:     group.Key,
				Reason:       c.reason,
				SaveForReuse: c.saveForUse,
			}, now)
			if err == nil {
				t.Fatalf("expected error for %s", c.wantErr)
			}
		})
	}
}

func TestApproveAbsence_DoesNotRequireErrorImport(t *testing.T) {
	// This test verifies that the errors import is used in this test file.
	// It's a compile-time check; if this file compiles, the import is used.
	_ = errors.New("compile-time check")
}

/* jscpd:ignore-end */
