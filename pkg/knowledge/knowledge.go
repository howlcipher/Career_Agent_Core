// Package knowledge turns the Approved Answer Vault from something that
// answers one question on one open form into something that knows what the
// whole application queue needs.
//
// The vault (pkg/answers) already decides, deterministically and safely, what
// the answer to one question is. What it could not do was tell the operator
// that nine queued applications ask the same thing, let them answer it once
// outside any browser session, or notice that approving one answer just
// resolved seven other questions. That is what this package adds, and it adds
// it *over* the vault rather than beside it: every answer written here goes
// through answers.Store.Save, so the sensitive two-decision rule and the
// refusal to store a per-job generated answer are enforced in exactly one
// place, as they always were.
//
// Two invariants are worth stating up front, because most of the design follows
// from them.
//
// **The inventory is advisory; the live browser is authoritative.** Nothing
// here changes a question's status, touches the assisted state machine, or
// writes to an application a browser currently holds. When cmd/assist opens a
// page it re-resolves every control from the vault and its answer wins. What
// this package stores is a cached opinion, so the dashboard can say "nine of
// these are now resolved" without opening nine browsers to find out.
//
// **Grouping stops before semantics.** Two questions are the same question only
// when a curated, deterministic rule says so -- a pattern from the vault's own
// table, the skill-experience reduction, or identical normalized text. There is
// no embedding, no similarity threshold and no model call anywhere in this
// package, for the same reason there is none in the vault: a heuristic that
// silently equates two different attestations puts a false statement on a real
// application, and no amount of average-case accuracy makes that acceptable.
package knowledge

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// Reuse policies, as the operator sees them.
//
// These are derived, never stored. Every one of them is already a fact about
// the (sensitivity, reuse-granted, source) triple the vault records, and adding
// a column would create a second place for the same truth to live and a second
// place for it to go stale. Naming them is a presentation decision; enforcing
// them is still the vault's.
const (
	// PolicySafeAutoFill: a stable fact from the operator's own configuration.
	PolicySafeAutoFill = "safe_auto_fill"
	// PolicyApprovedReusable: an answer the operator approved and allowed to be
	// reused automatically.
	PolicyApprovedReusable = "approved_reusable"
	// PolicySuggestAsk: Career Agent knows a previous answer and offers it, but
	// the operator decides each time. This is what withholding reuse means.
	PolicySuggestAsk = "suggest_ask"
	// PolicyHumanReview: a legal, demographic or compensation question. Career
	// Agent never answers one on the operator's behalf.
	PolicyHumanReview = "human_review"
	// PolicyGeneratePerJob: an answer that is only honest for one employer and
	// is therefore never stored or reused at all.
	PolicyGeneratePerJob = "generate_per_job"
	// PolicyUnknown: nothing in the vault or the pattern table answers this.
	PolicyUnknown = "unknown"
	// PolicyIntentionalAbsence: the operator explicitly decided they do not
	// have this information. The field is intentionally left blank on optional
	// forms and surfaces as a required-field conflict when an employer demands
	// a response.
	PolicyIntentionalAbsence = "intentional_absence"
)

// Policy names what Career Agent is allowed to do with a resolution.
func Policy(resolution answers.Resolution) string {
	switch {
	case resolution.IntentionalAbsence:
		return PolicyIntentionalAbsence
	case resolution.Sensitivity == answers.GeneratePerJob:
		return PolicyGeneratePerJob
	case resolution.AutoFill && resolution.Sensitivity == answers.Sensitive:
		// A declaration Career Agent actually fills. This can only arise from an
		// explicit operator grant -- Store.Save refuses any other route to it,
		// and a pattern's own resolution has AutoFill stripped by
		// escalateSensitivity. Reporting it as "always ask you" would be a
		// comfortable lie: observed live on 2026-08-13, an approved sponsorship
		// answer with reuse granted was labelled "Always ask you" in the vault
		// view while auto-filling three real questions. The operator is entitled
		// to see what they granted.
		return PolicyApprovedReusable
	case resolution.Sensitivity == answers.Sensitive:
		return PolicyHumanReview
	case !resolution.Resolved:
		return PolicyUnknown
	case resolution.Source == answers.SourcePattern:
		return PolicySafeAutoFill
	case resolution.AutoFill:
		return PolicyApprovedReusable
	default:
		return PolicySuggestAsk
	}
}

// RequiresHuman reports whether Career Agent must not commit this resolution
// itself. It is the same test the fill path uses -- AutoFill, and nothing else
// -- restated so a caller outside this repository (the browser companion of
// ADR-005) does not have to re-derive it and get it wrong.
func RequiresHuman(resolution answers.Resolution) bool { return !resolution.AutoFill }

// Group is one question the operator has to answer, however many applications
// are waiting on it.
type Group struct {
	// Key is the grouping key, prefixed with the rule that produced it
	// (`pattern:`, `experience:`, `q:`) so a reader can tell why two questions
	// were considered the same.
	Key string `json:"key"`
	// Prompt is the wording shown to the operator: the shortest phrasing seen,
	// which is normally the least cluttered with one employer's decorations.
	Prompt string `json:"prompt"`
	// Phrasings is every distinct wording observed. It is shown rather than
	// hidden, because "Career Agent thinks these mean the same thing" is a claim
	// the operator is entitled to check before they answer once for all of them.
	Phrasings []string `json:"phrasings"`
	// Occurrences counts fields; Applications counts jobs. They differ when one
	// form asks the same thing twice.
	Occurrences  int      `json:"occurrences"`
	Applications int      `json:"applications"`
	JobIDs       []string `json:"job_ids"`
	Companies    []string `json:"companies"`
	ControlType  string   `json:"control_type"`
	Options      []string `json:"options,omitempty"`
	// OptionsVary is set when employers offer genuinely different choices for
	// this question. One answer cannot be typed into all of them, so the group
	// is reported but not offered for bulk resolution -- claiming otherwise
	// would mean silently sending an option one employer does not have.
	OptionsVary bool   `json:"options_vary,omitempty"`
	Required    bool   `json:"required"`
	Sensitivity string `json:"sensitivity"`
	Policy      string `json:"policy"`
	// Suggested is what the vault would answer today, if anything. For a
	// sensitive group it is a proposal to confirm, never something already
	// filled.
	Suggested string `json:"suggested,omitempty"`
	Source    string `json:"source,omitempty"`
	Resolved  bool   `json:"resolved"`
	// CompanyScopeAvailable is true only when every occurrence is one employer.
	// Offering "this company only" for a question nine employers asked would
	// answer one of them and leave the other eight exactly as unresolved.
	CompanyScopeAvailable bool   `json:"company_scope_available"`
	CompanyScope          string `json:"company_scope,omitempty"`
	// SkillSubject is set for a years-of-<skill> group, so the UI can say what
	// is being asked about and the approval can be filed under the canonical
	// skill question rather than one employer's wording.
	SkillSubject string `json:"skill_subject,omitempty"`
	// AbsenceApproved is true when the vault holds an intentional-absence
	// answer for this group's key. This reaches the inbox only when the
	// absence cannot resolve the group (e.g., one employer marks the field
	// required). The UI uses it to explain why the operator is being asked
	// again: "Your saved answer is 'I don't have this', but this employer
	// requires a response."
	AbsenceApproved bool `json:"absence_approved,omitempty"`
}

// Readiness is the demand-driven summary: how ready the operator's knowledge is
// for the applications actually in front of them, never an abstract "profile
// 93% complete".
type Readiness struct {
	Applications        int `json:"applications"`
	Fields              int `json:"fields"`
	Resolved            int `json:"resolved"`
	Unresolved          int `json:"unresolved"`
	UniqueQuestions     int `json:"unique_questions"`
	SensitiveDecisions  int `json:"sensitive_decisions"`
	PerJobResponses     int `json:"per_job_responses"`
	FieldsUnlockable    int `json:"fields_unlockable"`
	AnswersNeeded       int `json:"answers_needed"`
	ApplicationsBlocked int `json:"applications_blocked"`
	// AbsenceResolved counts fields resolved by an intentional-absence
	// decision (the operator said "I don't have this"). These are part of
	// Resolved but not part of what was "filled" -- the distinction keeps
	// metrics semantically accurate. If the UI says "18/20 fields resolved",
	// AbsenceResolved tells it how many of those 18 were left blank by
	// operator decision vs. filled with a value.
	AbsenceResolved int `json:"absence_resolved"`
}

// KnownPercent is the share of discovered fields Career Agent can already
// handle. It returns 0 rather than 100 when nothing has been discovered:
// "everything is known" and "nothing has been looked at" must not render the
// same way.
func (r Readiness) KnownPercent() int {
	if r.Fields <= 0 {
		return 0
	}
	return int(float64(r.Resolved) / float64(r.Fields) * 100)
}

// Service reads and writes application knowledge.
type Service struct {
	conn  *sql.DB
	vault *answers.Store
	pii   *config.PII
}

// New builds a Service over an existing connection.
func New(conn *sql.DB, vault *answers.Store, pii *config.PII) *Service {
	return &Service{conn: conn, vault: vault, pii: pii}
}

// Open builds a Service, guaranteeing the vault's schema.
func Open(conn *sql.DB, pii *config.PII) (*Service, error) {
	vault, err := answers.OpenStore(conn)
	if err != nil {
		return nil, err
	}
	return New(conn, vault, pii), nil
}

var errNotReady = errors.New("application knowledge is not initialized")

// Inbox returns every unresolved question in the queue, deduplicated, most
// widely-asked first.
//
// "Unresolved" here means Career Agent cannot fill it without the operator --
// which includes a sensitive question it *can* answer but must not. Sorting by
// how many applications are waiting is the whole point: the operator should
// spend their next two minutes on the question that unblocks nine applications,
// not on whichever one happens to be first alphabetically.
func (s *Service) Inbox(now time.Time) ([]Group, error) {
	groups, _, err := s.collect(now)
	if err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sortGroups(out)
	return out, nil
}

// canonicalPromptFor picks the wording an approved answer is stored under.
func canonicalPromptFor(group *Group) string {
	if group.SkillSubject != "" {
		return answers.SkillExperienceQuestion(group.SkillSubject)
	}
	if id, found := strings.CutPrefix(group.Key, "pattern:"); found {
		if canonical := answers.CanonicalQuestionForPattern(id); canonical != "" {
			return canonical
		}
	}
	return group.Prompt
}

func sortGroups(groups []Group) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Applications != groups[j].Applications {
			return groups[i].Applications > groups[j].Applications
		}
		if groups[i].Occurrences != groups[j].Occurrences {
			return groups[i].Occurrences > groups[j].Occurrences
		}
		return groups[i].Key < groups[j].Key
	})
}

// groupKey decides which rule, if any, makes two questions the same question.
//
// The order is the point. A curated pattern is the strongest evidence available
// -- it is a hand-written recognizer for a question family every ATS words
// differently, and it already carries a Deny list precisely so families that
// share vocabulary stay apart. The skill reduction is next, and is equally
// closed. Identical normalized text is last and weakest: it collapses only
// presentation, which is exactly what it should do on its own.
//
// Nothing weaker than these three is used. A fourth layer -- "these look
// similar" -- is deliberately absent; see the package comment.
func groupKey(question answers.Question) (key string, skill string) {
	if id := answers.MatchedPatternID(question); id != "" {
		return "pattern:" + id, ""
	}
	if subject := answers.SkillExperienceSubject(question); subject != "" {
		return "experience:" + subject, subject
	}
	return "q:" + answers.QuestionKey(question.Prompt), ""
}

// collect builds every group in the queue and the readiness figures alongside
// them, from one pass over the pending questions.
func (s *Service) collect(now time.Time) (map[string]*Group, Readiness, error) {
	readiness := Readiness{}
	if s == nil || s.conn == nil {
		return nil, readiness, errNotReady
	}
	queued, err := storage.QueuedQuestions(s.conn, now)
	if err != nil {
		return nil, readiness, err
	}

	// The size of each form, from whoever inspected it. Counting question rows
	// instead would measure the operator's remaining work and call it the form,
	// so "Career Agent can handle N of M" would always read 0 of N.
	discovered, err := storage.DiscoveredFieldCounts(s.conn)
	if err != nil {
		return nil, readiness, err
	}

	groups := map[string]*Group{}
	applications := map[string]bool{}
	blocked := map[string]bool{}
	optionSets := map[string][]string{}

	questionsPerJob := map[string]int{}
	for _, queuedQuestion := range queued {
		applications[queuedQuestion.JobID] = true
		questionsPerJob[queuedQuestion.JobID]++

		question := answers.Question{
			Key:         queuedQuestion.Key,
			Prompt:      queuedQuestion.Prompt,
			ControlType: queuedQuestion.ControlType,
			Options:     queuedQuestion.Options,
			Required:    queuedQuestion.Required,
			Company:     queuedQuestion.Company,
		}
		resolution := s.vault.Resolve(question, answers.Context{
			ATS: queuedQuestion.ATS, Company: queuedQuestion.Company,
		}, s.pii)

		if resolution.AutoFill {
			// Career Agent can handle this one already; it is not something to
			// put in front of the operator. It reaches this loop at all only
			// when it became answerable after the inventory was taken.
			if resolution.IntentionalAbsence {
				readiness.AbsenceResolved++
			}
			continue
		}
		readiness.Unresolved++
		blocked[queuedQuestion.JobID] = true

		key, skill := groupKey(question)
		group := groups[key]
		if group == nil {
			group = &Group{
				Key:          key,
				Prompt:       queuedQuestion.Prompt,
				ControlType:  queuedQuestion.ControlType,
				SkillSubject: skill,
				Sensitivity:  string(resolution.Sensitivity),
			}
			groups[key] = group
		}
		mergeIntoGroup(group, queuedQuestion, resolution, optionSets)
	}

	for _, group := range groups {
		finalizeGroup(group, optionSets[group.Key])
		readiness.UniqueQuestions++
		switch group.Policy {
		case PolicyHumanReview:
			readiness.SensitiveDecisions++
		case PolicyGeneratePerJob:
			readiness.PerJobResponses++
		default:
			// Only a group the operator can actually answer once counts toward
			// "resolve these N and Career Agent handles M fields". A declaration
			// they must read on every application does not unlock anything in
			// bulk, and counting it would overstate what the number buys them.
			if !group.OptionsVary {
				readiness.AnswersNeeded++
				readiness.FieldsUnlockable += group.Occurrences
			}
		}
	}
	for jobID := range discovered {
		applications[jobID] = true
	}
	for _, count := range discovered {
		readiness.Fields += count
	}
	// An application inspected by nothing still contributes its known questions,
	// so a queue whose inventory came from live sessions rather than preflight
	// is not reported as having no fields at all.
	for jobID := range applications {
		if _, known := discovered[jobID]; !known {
			readiness.Fields += questionsPerJob[jobID]
		}
	}
	if readiness.Fields < readiness.Unresolved {
		// Cannot happen from consistent data, but reporting more unresolved
		// fields than fields would be visibly nonsense rather than merely wrong.
		readiness.Fields = readiness.Unresolved
	}
	readiness.Resolved = readiness.Fields - readiness.Unresolved
	readiness.Applications = len(applications)
	readiness.ApplicationsBlocked = len(blocked)
	return groups, readiness, nil
}

// mergeIntoGroup folds one occurrence into its group.
func mergeIntoGroup(group *Group, queuedQuestion storage.QueuedQuestion, resolution answers.Resolution, optionSets map[string][]string) {
	group.Occurrences++
	if !containsString(group.JobIDs, queuedQuestion.JobID) {
		group.JobIDs = append(group.JobIDs, queuedQuestion.JobID)
		group.Applications++
	}
	if queuedQuestion.Company != "" && !containsString(group.Companies, queuedQuestion.Company) {
		group.Companies = append(group.Companies, queuedQuestion.Company)
	}
	phrasing := strings.TrimSpace(queuedQuestion.Prompt)
	if phrasing != "" && !containsString(group.Phrasings, phrasing) {
		group.Phrasings = append(group.Phrasings, phrasing)
	}
	if queuedQuestion.Required {
		group.Required = true
	}
	// The strictest reading wins. A question one employer words neutrally and
	// another words as an attestation is an attestation.
	group.Sensitivity = stricterSensitivity(group.Sensitivity, string(resolution.Sensitivity))
	if resolution.Resolved && group.Suggested == "" {
		group.Suggested = resolution.Answer
		group.Source = string(resolution.Source)
		group.Resolved = true
	}
	// An intentional-absence answer that landed in the inbox (because the
	// field is required) should be flagged so the UI can explain the conflict.
	if resolution.IntentionalAbsence {
		group.AbsenceApproved = true
	}
	// Free-text beats a choice control when they disagree: a text box accepts
	// what a dropdown offers, and the reverse is not true.
	if group.ControlType != "textarea" && queuedQuestion.ControlType == "textarea" {
		group.ControlType = "textarea"
	}
	if len(queuedQuestion.Options) > 0 {
		optionSets[group.Key] = appendOptionSignature(optionSets[group.Key], queuedQuestion.Options)
		group.Options = unionOptions(group.Options, queuedQuestion.Options)
	}
}

// finalizeGroup computes the fields that can only be known once every
// occurrence has been seen.
func finalizeGroup(group *Group, signatures []string) {
	group.Policy = Policy(answers.Resolution{
		Resolved:    group.Resolved,
		AutoFill:    false, // by construction: an auto-fillable question never reaches a group
		Sensitivity: answers.Sensitivity(group.Sensitivity),
		Source:      answers.Source(group.Source),
	})
	// A group Career Agent could answer but must not is "suggest and ask", not
	// "unknown" -- the operator should see their previous wording, not a blank.
	if group.Resolved && group.Policy == PolicyUnknown {
		group.Policy = PolicySuggestAsk
	}
	group.OptionsVary = len(signatures) > 1
	sort.Strings(group.Phrasings)
	// The shortest phrasing is normally the one with the least of one
	// employer's decoration on it, which makes it the best label for a question
	// several employers ask.
	for _, phrasing := range group.Phrasings {
		if group.Prompt == "" || len(phrasing) < len(group.Prompt) {
			group.Prompt = phrasing
		}
	}
	if len(group.Companies) == 1 {
		group.CompanyScopeAvailable = true
		group.CompanyScope = answers.CompanyScope(group.Companies[0])
	}
	sort.Strings(group.JobIDs)
	sort.Strings(group.Companies)
}

// appendOptionSignature records one employer's option set in a comparable form,
// so "these employers offer different choices" is a fact rather than a guess.
func appendOptionSignature(existing []string, options []string) []string {
	normalized := make([]string, 0, len(options))
	for _, option := range options {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(option)))
	}
	sort.Strings(normalized)
	signature := strings.Join(normalized, "\x00")
	if containsString(existing, signature) {
		return existing
	}
	return append(existing, signature)
}

// unionOptions merges option lists case-insensitively, keeping the casing first
// seen so the operator reads an employer's own words.
func unionOptions(existing []string, incoming []string) []string {
	seen := map[string]bool{}
	for _, option := range existing {
		seen[strings.ToLower(strings.TrimSpace(option))] = true
	}
	for _, option := range incoming {
		option = strings.TrimSpace(option)
		key := strings.ToLower(option)
		if option == "" || seen[key] {
			continue
		}
		seen[key] = true
		existing = append(existing, option)
	}
	return existing
}

func stricterSensitivity(current, incoming string) string {
	rank := func(value string) int {
		switch answers.Sensitivity(value) {
		case answers.Sensitive:
			return 3
		case answers.GeneratePerJob:
			return 2
		default:
			return 1
		}
	}
	if rank(incoming) > rank(current) {
		return incoming
	}
	if current == "" {
		return incoming
	}
	return current
}

func containsString(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}

// Readiness returns the queue-grounded summary on its own.
func (s *Service) Readiness(now time.Time) (Readiness, error) {
	_, readiness, err := s.collect(now)
	return readiness, err
}

// Snapshot is everything the dashboard's Application Knowledge section needs in
// one round trip, so the two halves can never disagree about the same queue.
type Snapshot struct {
	Readiness Readiness                 `json:"readiness"`
	Groups    []Group                   `json:"groups"`
	Preflight []storage.PreflightResult `json:"preflight"`
}

// Snapshot builds the whole view.
func (s *Service) Snapshot(now time.Time) (Snapshot, error) {
	groups, readiness, err := s.collect(now)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{Readiness: readiness, Groups: make([]Group, 0, len(groups))}
	for _, group := range groups {
		out.Groups = append(out.Groups, *group)
	}
	sortGroups(out.Groups)
	if preflight, err := storage.PreflightResults(s.conn, nil); err == nil {
		out.Preflight = preflight
	} else {
		out.Preflight = []storage.PreflightResult{}
	}
	return out, nil
}

// ApproveRequest is the operator answering one group, once, for every
// application waiting on it.
type ApproveRequest struct {
	// GroupKey identifies which group is being answered. It is required, and it
	// is checked against the current queue: an answer to a group that is not
	// there is refused rather than stored, for the same reason the assisted
	// answers endpoint refuses a key the application never asked.
	GroupKey string
	Answer   string
	// SaveForReuse is the operator asking Career Agent to remember this.
	// Without it, nothing is stored and nothing is re-evaluated.
	SaveForReuse bool
	// AllowSensitiveReuse is the separate second acknowledgement a declaration
	// needs. It is never inferred from SaveForReuse.
	AllowSensitiveReuse bool
	// ConfirmedEquivalent is the operator agreeing that the group's phrasings
	// really do ask the same thing. Required before a sensitive group's extra
	// phrasings become aliases.
	ConfirmedEquivalent bool
	// Scope is "global" or "company:<name>"; anything else is treated as global
	// by answers' own normalization.
	Scope string
}

// ApproveResult is what the operator is told happened, in the terms that make
// the feature visibly worth using.
type ApproveResult struct {
	Saved              bool   `json:"saved"`
	AnswerID           int64  `json:"answer_id,omitempty"`
	CanonicalQuestion  string `json:"canonical_question,omitempty"`
	AliasesAdded       int    `json:"aliases_added"`
	QuestionsResolved  int    `json:"questions_resolved"`
	ApplicationsHelped int    `json:"applications_helped"`
	StillUnresolved    int    `json:"still_unresolved"`
}

// Approve stores one answer and immediately re-evaluates the queue.
//
// The re-evaluation is not a nicety. Without it the operator approves an answer
// and nothing visible happens until they next open each of the other six
// applications one at a time, which is the exact friction this whole feature
// exists to remove.
func (s *Service) Approve(request ApproveRequest, now time.Time) (ApproveResult, error) {
	result := ApproveResult{}
	if s == nil || s.conn == nil {
		return result, errNotReady
	}
	answerText := strings.TrimSpace(request.Answer)
	if answerText == "" {
		return result, errors.New("an approved answer needs an answer")
	}

	groups, _, err := s.collect(now)
	if err != nil {
		return result, err
	}
	group := groups[request.GroupKey]
	if group == nil {
		return result, errors.New("that question is no longer in the queue")
	}
	if group.OptionsVary {
		return result, errors.New("these employers offer different choices for this question, so it has to be answered per application")
	}
	if !request.SaveForReuse {
		// Nothing to do, and saying so is better than reporting a success that
		// changed nothing.
		return result, errors.New("this answer was not marked for reuse, so there is nothing to save")
	}

	// The canonical question a group is filed under is Career Agent's own
	// wording wherever it has one, not whichever employer happened to be asked
	// first. For a skill group that is what lets a future phrasing resolve
	// without an alias; for a pattern group it stops a global answer being
	// labelled with one company's name (observed live: a sponsorship answer
	// filed as "...to work for Affirm in the United States?*").
	prompt := canonicalPromptFor(group)
	question := answers.Question{
		Key:         group.Key,
		Prompt:      prompt,
		ControlType: group.ControlType,
		Options:     group.Options,
		Required:    group.Required,
	}
	if len(group.Companies) == 1 {
		question.Company = group.Companies[0]
	}

	sensitive := answers.Sensitivity(group.Sensitivity) == answers.Sensitive ||
		answers.Classify(question) == answers.Sensitive
	reuseAllowed := true
	if sensitive {
		reuseAllowed = request.AllowSensitiveReuse
	}
	provenance := answers.OperatorApproved
	if answerText != strings.TrimSpace(group.Suggested) {
		provenance = answers.OperatorEdited
	}

	saved, err := s.vault.Save(answers.SaveRequest{
		Question:          question,
		Answer:            answerText,
		Scope:             request.Scope,
		Provenance:        provenance,
		ReuseAllowed:      reuseAllowed,
		ReuseDecisionMade: true,
		Sensitivity:       answers.Sensitivity(group.Sensitivity),
	})
	if err != nil {
		return result, err
	}
	result.Saved = true
	result.AnswerID = saved.ID
	result.CanonicalQuestion = saved.CanonicalQuestion

	// Bind the other phrasings this group collapsed, so the next occurrence is
	// a lookup rather than another interruption. A declaration needs the
	// operator to have confirmed the phrasings are equivalent; a routine
	// question does not, because the cost of being wrong is a retyped answer
	// rather than a false statement.
	if reuseAllowed {
		extra := make([]string, 0, len(group.Phrasings))
		for _, phrasing := range group.Phrasings {
			if answers.Normalize(phrasing) != answers.Normalize(prompt) {
				extra = append(extra, phrasing)
			}
		}
		if len(extra) > 0 {
			added, aliasErr := s.vault.AddAliases(saved.ID, extra, request.ConfirmedEquivalent)
			if aliasErr != nil && !errors.Is(aliasErr, answers.ErrSensitiveAliasNeedsConfirmation) {
				return result, fmt.Errorf("bind the other phrasings of this question: %w", aliasErr)
			}
			result.AliasesAdded = added
		}
	}

	report, err := s.ReEvaluate(now)
	if err != nil {
		return result, err
	}
	result.QuestionsResolved = report.Resolved
	result.ApplicationsHelped = report.Applications
	result.StillUnresolved = report.StillUnresolved
	return result, nil
}

// ApproveAbsenceRequest is the operator declaring they do not have the thing
// a group's question asks for. It is an explicit decision, not a dismissal.
type ApproveAbsenceRequest struct {
	// GroupKey identifies which group is being declared absent.
	GroupKey string
	// Reason is shown in the vault management view, e.g. "No Twitter/X account".
	Reason string
	// SaveForReuse must be true (absence without reuse is pointless: it would
	// resolve nothing and the group stays in the inbox).
	SaveForReuse bool
	// ConfirmedEquivalent is the operator agreeing that the group's phrasings
	// all ask for the same thing and should all inherit the absence.
	ConfirmedEquivalent bool
	Scope               string
}

// ApproveAbsence stores an intentional-absence decision for a group and
// immediately re-evaluates the queue, just like Approve does for value answers.
func (s *Service) ApproveAbsence(request ApproveAbsenceRequest, now time.Time) (ApproveResult, error) {
	result := ApproveResult{}
	if s == nil || s.conn == nil {
		return result, errNotReady
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		return result, errors.New("an intentional absence needs a reason")
	}
	if !request.SaveForReuse {
		return result, errors.New("an absence that is not saved for reuse resolves nothing — use it only when the operator wants to permanently skip this question")
	}

	groups, _, err := s.collect(now)
	if err != nil {
		return result, err
	}
	group := groups[request.GroupKey]
	if group == nil {
		return result, errors.New("that question is no longer in the queue")
	}
	// Absence cannot apply to per-job questions (they need a per-employer answer).
	if group.Policy == PolicyGeneratePerJob {
		return result, errors.New("a per-job question needs a per-employer answer and cannot be marked absent")
	}

	prompt := canonicalPromptFor(group)
	question := answers.Question{
		Key:         group.Key,
		Prompt:      prompt,
		ControlType: group.ControlType,
		Options:     group.Options,
		Required:    group.Required,
	}
	if len(group.Companies) == 1 {
		question.Company = group.Companies[0]
	}

	saved, err := s.vault.SaveAbsence(answers.SaveAbsenceRequest{
		Question:          question,
		Reason:            reason,
		Scope:             request.Scope,
		ReuseAllowed:      true, // absence always grants reuse
		ReuseDecisionMade: true,
	})
	if err != nil {
		return result, err
	}
	result.Saved = true
	result.AnswerID = saved.ID
	result.CanonicalQuestion = saved.CanonicalQuestion

	// Bind the other phrasings as aliases so they inherit the absence.
	extra := make([]string, 0, len(group.Phrasings))
	for _, phrasing := range group.Phrasings {
		if answers.Normalize(phrasing) != answers.Normalize(prompt) {
			extra = append(extra, phrasing)
		}
	}
	if len(extra) > 0 {
		added, aliasErr := s.vault.AddAliases(saved.ID, extra, request.ConfirmedEquivalent)
		if aliasErr != nil && !errors.Is(aliasErr, answers.ErrSensitiveAliasNeedsConfirmation) {
			return result, fmt.Errorf("bind the other phrasings of this question: %w", aliasErr)
		}
		result.AliasesAdded = added
	}

	report, err := s.ReEvaluate(now)
	if err != nil {
		return result, err
	}
	result.QuestionsResolved = report.Resolved
	result.ApplicationsHelped = report.Applications
	result.StillUnresolved = report.StillUnresolved
	return result, nil
}

// Report is what a re-evaluation pass found.
type Report struct {
	Examined        int `json:"examined"`
	Resolved        int `json:"resolved"`
	Applications    int `json:"applications"`
	StillUnresolved int `json:"still_unresolved"`
}

// ReEvaluate re-resolves every pending question in the queue against the
// current state of the vault, and records what it concluded.
//
// It is cheap by construction: it reads the stored question inventory rather
// than opening any browser, and it touches only the advisory columns. Anything
// an assisted browser currently holds is excluded upstream by QueuedQuestions.
func (s *Service) ReEvaluate(now time.Time) (Report, error) {
	report := Report{}
	if s == nil || s.conn == nil {
		return report, errNotReady
	}
	queued, err := storage.QueuedQuestions(s.conn, now)
	if err != nil {
		return report, err
	}
	helped := map[string]bool{}
	for _, queuedQuestion := range queued {
		report.Examined++
		question := answers.Question{
			Key:         queuedQuestion.Key,
			Prompt:      queuedQuestion.Prompt,
			ControlType: queuedQuestion.ControlType,
			Options:     queuedQuestion.Options,
			Required:    queuedQuestion.Required,
			Company:     queuedQuestion.Company,
		}
		resolution := s.vault.Resolve(question, answers.Context{
			ATS: queuedQuestion.ATS, Company: queuedQuestion.Company,
		}, s.pii)

		if resolution.AutoFill {
			report.Resolved++
			helped[queuedQuestion.JobID] = true
		} else {
			report.StillUnresolved++
		}
		if err := storage.SetQuestionResolution(s.conn, queuedQuestion.ID,
			resolution.Answer, string(resolution.Source), resolution.AutoFill); err != nil {
			return report, err
		}
	}
	report.Applications = len(helped)
	return report, nil
}

// FieldQuery asks what Career Agent knows about one field, given only what a
// page can see about it. It is the shape ADR-005's browser companion needs, so
// that companion never has to implement an answer system of its own.
type FieldQuery struct {
	Prompt      string
	ControlType string
	Options     []string
	Company     string
	ATS         string
}

// FieldAnswer is the reply. Answer is populated only when the policy permits
// Career Agent to fill the field without asking -- a caller that receives an
// empty Answer with RequiresHuman set has been told everything it is entitled
// to know, and must put the question to the operator.
type FieldAnswer struct {
	NormalizedQuestion string `json:"normalized_question"`
	CanonicalQuestion  string `json:"canonical_question,omitempty"`
	Policy             string `json:"policy"`
	RequiresHuman      bool   `json:"requires_human"`
	Sensitivity        string `json:"sensitivity"`
	Scope              string `json:"scope,omitempty"`
	Provenance         string `json:"provenance,omitempty"`
	Source             string `json:"source,omitempty"`
	Answer             string `json:"answer,omitempty"`
	// Suggested carries a value the operator should confirm rather than one
	// Career Agent may type. It is deliberately a different field from Answer,
	// so a caller cannot fill a suggestion by reading the wrong key.
	Suggested    string `json:"suggested,omitempty"`
	SkillSubject string `json:"skill_subject,omitempty"`
	// IntentionalAbsence is true when the resolution is an operator's explicit
	// decision to leave this field blank. The fill path uses it to skip the
	// control entirely rather than typing an empty value.
	IntentionalAbsence bool `json:"intentional_absence,omitempty"`
}

// Field answers a single-field query.
func (s *Service) Field(query FieldQuery) (FieldAnswer, error) {
	if s == nil || s.conn == nil {
		return FieldAnswer{}, errNotReady
	}
	if strings.TrimSpace(query.Prompt) == "" {
		return FieldAnswer{}, errors.New("a field query needs the question's text")
	}
	question := answers.Question{
		Prompt:      query.Prompt,
		ControlType: query.ControlType,
		Options:     query.Options,
		Company:     query.Company,
	}
	resolution := s.vault.Resolve(question, answers.Context{ATS: query.ATS, Company: query.Company}, s.pii)
	reply := FieldAnswer{
		NormalizedQuestion: answers.Normalize(query.Prompt),
		CanonicalQuestion:  resolution.CanonicalQuestion,
		Policy:             Policy(resolution),
		RequiresHuman:      RequiresHuman(resolution),
		Sensitivity:        string(resolution.Sensitivity),
		Source:             string(resolution.Source),
		SkillSubject:       answers.SkillExperienceSubject(question),
		IntentionalAbsence: resolution.IntentionalAbsence,
	}
	if resolution.AutoFill && !resolution.IntentionalAbsence {
		reply.Answer = resolution.Answer
	} else if resolution.Resolved {
		reply.Suggested = resolution.Answer
	}
	if resolution.AnswerID != 0 {
		if stored, err := s.vault.Get(resolution.AnswerID); err == nil {
			reply.Scope = stored.Scope
			reply.Provenance = string(stored.Provenance)
		}
	}
	return reply, nil
}
