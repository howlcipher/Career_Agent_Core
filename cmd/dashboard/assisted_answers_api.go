package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// maxAnswersRequestBytes bounds an answers submission. A screening form can
// legitimately ask for a few paragraphs, so this is larger than the 1 KiB
// decodeBoundedJSON allows elsewhere, but it is still a hard cap.
const maxAnswersRequestBytes = 64 * 1024

// answerSubmission is one question the operator has dealt with.
type answerSubmission struct {
	Key    string `json:"key"`
	Answer string `json:"answer"`
	// SaveForReuse is the operator asking Career Agent to remember this answer
	// for equivalent questions in future. It defaults to false, and for a
	// sensitive question it is not sufficient on its own — see
	// AllowSensitiveReuse.
	SaveForReuse bool `json:"save_for_reuse"`
	// AllowSensitiveReuse is the second, separate acknowledgement required
	// before a legal attestation or protected-class answer may ever be stored.
	// Two checkboxes rather than one is deliberate: remembering "my GitHub URL"
	// and remembering "my answer to a work-authorization attestation" are not
	// the same decision and must not share a control.
	AllowSensitiveReuse bool `json:"allow_sensitive_reuse"`
	// Scope is where the operator wants the answer to apply.
	Scope string `json:"scope"`
}

// serveAssistedAnswers takes the operator's answers to the questions one
// refill surfaced, hands them to the open assisted browser, and optionally
// records the ones they asked to have remembered.
//
// The order matters. The answers reach the browser first; storing them in the
// vault is a separate, best-effort step afterwards, so a vault write that fails
// never costs the operator the answers they just typed.
func serveAssistedAnswers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JobID   string             `json:"job_id"`
		Answers []answerSubmission `json:"answers"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAnswersRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.JobID) == "" {
		http.Error(w, "an assisted job identifier and at least one answer are required", http.StatusBadRequest)
		return
	}
	if _, err := strconv.ParseInt(request.JobID, 10, 64); err != nil {
		http.Error(w, "invalid assisted job identifier", http.StatusBadRequest)
		return
	}
	if len(request.Answers) == 0 {
		http.Error(w, "no answers were provided", http.StatusBadRequest)
		return
	}

	pending, err := storage.GetPendingQuestions(db, request.JobID)
	if err != nil {
		log.Printf("serveAssistedAnswers: %v", err)
		http.Error(w, "could not load this application's questions", http.StatusInternalServerError)
		return
	}
	known := make(map[string]storage.ApplicationQuestion, len(pending))
	for _, question := range pending {
		known[question.Key] = question
	}

	values := map[string]string{}
	for _, submission := range request.Answers {
		question, ok := known[submission.Key]
		if !ok {
			// An answer to a question this application never asked is refused
			// outright rather than passed to the browser. The key is what
			// selects which control gets typed into, so accepting an
			// unrecognized one would let a request fill a field nobody was
			// shown.
			http.Error(w, "an answer referred to a question this application did not ask", http.StatusBadRequest)
			return
		}
		answer := strings.TrimSpace(submission.Answer)
		if answer == "" {
			continue
		}
		values[question.Key] = answer
	}
	if len(values) == 0 {
		http.Error(w, "no answers were provided", http.StatusBadRequest)
		return
	}

	if err := storage.SubmitAssistedAnswers(db, request.JobID, values, time.Now()); err != nil {
		log.Printf("serveAssistedAnswers: %v", err)
		http.Error(w, "the assisted browser is no longer waiting for answers; reopen the application and try again", http.StatusConflict)
		return
	}

	// How long the operator spent answering, measured from when the questions
	// were recorded to now — both server timestamps, neither supplied by the
	// browser.
	if len(pending) > 0 {
		if askedAt, err := time.Parse(time.RFC3339, pending[0].CreatedAt); err == nil {
			if err := storage.RecordHumanInteraction(db, request.JobID, storage.InteractionAnswering, askedAt, time.Now().UTC()); err != nil {
				log.Printf("serveAssistedAnswers: could not record answering time: %v", err)
			}
		}
	}

	saved, refused := rememberApprovedAnswers(request.Answers, known)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "sent",
		"answers_sent":    len(values),
		"answers_saved":   saved,
		"answers_refused": refused,
	})
}

// rememberApprovedAnswers stores the answers the operator explicitly asked to
// have remembered, and reports how many were refused.
//
// Refusals are counted and returned rather than silently swallowed. A
// sensitive answer the operator ticked "save" on but not "allow reuse" is
// legitimately refused by the vault, and the operator should be told that
// rather than left believing Career Agent will use it next time.
func rememberApprovedAnswers(submissions []answerSubmission, known map[string]storage.ApplicationQuestion) (saved, refused int) {
	wanted := false
	for _, submission := range submissions {
		if submission.SaveForReuse {
			wanted = true
			break
		}
	}
	if !wanted {
		return 0, 0
	}
	vault, err := answers.OpenStore(db)
	if err != nil {
		log.Printf("rememberApprovedAnswers: answer vault unavailable: %v", err)
		return 0, 0
	}
	for _, submission := range submissions {
		if !submission.SaveForReuse {
			continue
		}
		question, ok := known[submission.Key]
		if !ok {
			continue
		}
		answer := strings.TrimSpace(submission.Answer)
		if answer == "" {
			continue
		}
		sensitive := question.Sensitivity == string(answers.Sensitive)
		// For a sensitive answer the reuse decision is the second checkbox,
		// never the first. The vault re-checks this itself; passing the
		// operator's actual answer through rather than a convenient default is
		// what makes that check meaningful.
		reuseAllowed := true
		if sensitive {
			reuseAllowed = submission.AllowSensitiveReuse
		}
		provenance := answers.OperatorApproved
		if answer != strings.TrimSpace(question.Suggested) {
			provenance = answers.OperatorEdited
		}
		if _, err := vault.Save(answers.SaveRequest{
			Question: answers.Question{
				Key:         question.Key,
				Prompt:      question.Prompt,
				ControlType: question.ControlType,
				Options:     question.Options,
				Required:    question.Required,
			},
			Answer:            answer,
			Scope:             normalizeRequestedScope(submission.Scope),
			Provenance:        provenance,
			ReuseAllowed:      reuseAllowed,
			ReuseDecisionMade: true,
		}); err != nil {
			log.Printf("rememberApprovedAnswers: refused to store an answer: %v", err)
			refused++
			continue
		}
		saved++
	}
	return saved, refused
}

// normalizeRequestedScope accepts only the scopes the UI can offer. An
// unrecognized scope becomes global rather than being stored verbatim, so a
// request cannot invent scope names that would never match on resolution and
// would leave the operator with an answer that silently never applies.
func normalizeRequestedScope(scope string) string {
	scope = strings.TrimSpace(scope)
	switch {
	case scope == "" || scope == answers.ScopeGlobal:
		return answers.ScopeGlobal
	case strings.HasPrefix(scope, "ats:"):
		return answers.ATSScope(strings.TrimPrefix(scope, "ats:"))
	case strings.HasPrefix(scope, "company:"):
		return answers.CompanyScope(strings.TrimPrefix(scope, "company:"))
	default:
		return answers.ScopeGlobal
	}
}

// serveApplicationPacket exposes the prepared values for one application so
// they can be copied by hand.
//
// This is the fallback that keeps an unsupported ATS worth Career Agent's
// preparation: even when nothing can be filled automatically, the operator
// still gets every value in one place instead of hunting through pii.yaml.
//
// It serves the operator their own data back to their own loopback dashboard,
// which is the same boundary /api/assisted/document already crosses. It is
// same-origin guarded, it is never logged, and it never includes an answer the
// operator has not approved.
func serveApplicationPacket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if _, err := strconv.ParseInt(jobID, 10, 64); err != nil {
		http.Error(w, "invalid assisted job identifier", http.StatusBadRequest)
		return
	}
	pii, err := config.LoadPII("pii.yaml")
	if err != nil {
		log.Printf("serveApplicationPacket: %v", err)
		http.Error(w, "could not read your configured details", http.StatusInternalServerError)
		return
	}
	type packetEntry struct {
		Label     string `json:"label"`
		Value     string `json:"value"`
		Sensitive bool   `json:"sensitive"`
	}
	entries := []packetEntry{}
	add := func(label, value string, sensitive bool) {
		if strings.TrimSpace(value) != "" {
			entries = append(entries, packetEntry{Label: label, Value: value, Sensitive: sensitive})
		}
	}
	add("Full name", strings.TrimSpace(pii.FirstName+" "+pii.LastName), false)
	add("Email", pii.Email, false)
	add("Phone", pii.Phone, false)
	add("City", pii.City, false)
	add("State", pii.FullState, false)
	add("ZIP", pii.Zip, false)
	add("Country", pii.FullCountry, false)
	add("LinkedIn", pii.Links.LinkedIn, false)
	add("GitHub", pii.Links.GitHub, false)
	add("Portfolio", pii.Links.Portfolio, false)
	add("Website", pii.Links.Website, false)
	add("Current title", pii.Work.CurrentTitle, false)
	add("Current employer", pii.Work.CurrentEmployer, false)
	add("Years of experience", pii.Work.YearsExperience, false)
	add("Earliest start date", pii.EarliestStartDate(), false)

	// Approved answers are included so the operator can copy the exact wording
	// they approved, rather than reconstructing it. Sensitive ones are flagged
	// so the UI can require a deliberate reveal rather than printing an
	// attestation on screen by default.
	if vault, err := answers.OpenStore(db); err == nil {
		if stored, err := vault.List(); err == nil {
			for _, entry := range stored {
				add(entry.CanonicalQuestion, entry.AnswerText, entry.Sensitivity == answers.Sensitive)
			}
		}
	}

	summary, _ := storage.GetFillSummary(db, jobID)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{
		"job_id":  jobID,
		"entries": entries,
		"documents": map[string]bool{
			"resume":       contains(summary.Documents, "resume"),
			"cover_letter": contains(summary.Documents, "cover_letter"),
		},
	})
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// serveAnswerVault lists and revokes approved answers, so the operator can see
// exactly what Career Agent has been given permission to reuse and withdraw
// any of it.
func serveAnswerVault(w http.ResponseWriter, r *http.Request) {
	vault, err := answers.OpenStore(db)
	if err != nil {
		log.Printf("serveAnswerVault: %v", err)
		http.Error(w, "the answer vault is unavailable", http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		stored, err := vault.List()
		if err != nil {
			log.Printf("serveAnswerVault: %v", err)
			http.Error(w, "could not read the answer vault", http.StatusInternalServerError)
			return
		}
		type vaultEntry struct {
			ID           int64  `json:"id"`
			Question     string `json:"question"`
			Answer       string `json:"answer"`
			Sensitivity  string `json:"sensitivity"`
			Scope        string `json:"scope"`
			ReuseAllowed bool   `json:"reuse_allowed"`
			Provenance   string `json:"provenance"`
			UseCount     int    `json:"use_count"`
		}
		out := make([]vaultEntry, 0, len(stored))
		for _, entry := range stored {
			out = append(out, vaultEntry{
				ID: entry.ID, Question: entry.CanonicalQuestion, Answer: entry.AnswerText,
				Sensitivity: string(entry.Sensitivity), Scope: entry.Scope,
				ReuseAllowed: entry.ReuseAllowed, Provenance: string(entry.Provenance),
				UseCount: entry.UseCount,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{"answers": out})
	case http.MethodDelete:
		var request struct {
			ID int64 `json:"id"`
		}
		if err := decodeBoundedJSON(w, r, &request); err != nil {
			return
		}
		if err := vault.Revoke(request.ID); err != nil {
			http.Error(w, fmt.Sprintf("could not revoke that answer: %v", err), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
