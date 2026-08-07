package tracker

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/util"
	"os"
	"path/filepath"
)

// IMAPConfig holds credentials for the tracker
type IMAPConfig struct {
	Server   string
	Username string
	Password string
}

// StartTracker connects to the IMAP server, loops continuously, and scans for application updates.
func StartTracker(cfg IMAPConfig) error {
	log.Printf("[Tracker] Connecting to %s...", cfg.Server)

	c, err := client.DialTLS(cfg.Server, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer c.Logout()

	if err := c.Login(cfg.Username, cfg.Password); err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}
	log.Println("[Tracker] Successfully logged in to email account.")

	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return err
	}

	if mbox.Messages == 0 {
		log.Println("[Tracker] Inbox is empty. Nothing to track.")
		return nil
	}

	// Fetch last 50 emails to evaluate (in a real scenario, use search for UNSEEN)
	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > 50 {
		from = mbox.Messages - 50
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	// Fetch ENVELOPE (subject, sender) and BODY
	section := &imap.BodySectionName{}
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}, messages)
	}()

	trackedCompanies, err := storage.GetTrackedCompanies()
	if err != nil {
		log.Printf("[Tracker] Could not load tracked companies (DB not initialized?): %v — running detection-only, no status updates.", err)
	}

	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		if storage.WasEmailProcessed(msg.Envelope.MessageId) {
			continue
		}
		subject := strings.ToLower(msg.Envelope.Subject)

		var senderDomain string
		if len(msg.Envelope.From) > 0 {
			senderDomain = strings.ToLower(msg.Envelope.From[0].HostName)
		}

		bodyText := extractBody(msg, section)
		bodyLower := strings.ToLower(bodyText)

		status := classifyEmail(subject, bodyLower)
		if status != "" {
			company := matchTrackedCompany(trackedCompanies, senderDomain, subject)
			result, err := updateDBWithTrackerResult(
				msg.Envelope.MessageId,
				company,
				status,
				subject,
				bodyLower,
				senderDomain,
			)
			if err != nil {
				log.Printf(
					"[Tracker] Failed to persist %s outcome for %q: %v. Email left unprocessed for retry.",
					status,
					company,
					err,
				)
				continue
			}

			switch result {
			case trackerUpdateUnmatched:
				log.Printf("[Tracker] Detected %s-shaped email from %s (%s) but it matches no tracked application — ignoring.", status, senderDomain, subject)
			case trackerUpdateNoop:
				log.Printf("[Tracker] Detected %s for tracked company %q, but no APPLIED row required an update.", status, company)
			case trackerUpdateAmbiguous:
				log.Printf("[Tracker] Found %s for %q, but multiple APPLIED roles matched. Logged for manual review.", status, company)
			case trackerUpdateUpdated:
				log.Printf("[Tracker] Persisted %s for exactly one application at tracked company %q.", status, company)
				if status == "REJECTED" {
					llmClient := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
					reason, err := llmClient.ExtractRejectionReason(bodyText)
					if err != nil {
						reason = "Generic templated rejection (no specific reason provided)"
					}
					logRejectionFeedback(company, subject, reason)
				}
			}
			continue
		}
		if err := storage.MarkEmailProcessed(msg.Envelope.MessageId); err != nil {
			log.Printf("[Tracker] Failed to mark email processed: %v", err)
		}
	}

	if err := <-done; err != nil {
		return err
	}

	reportUnmatchedOutcomes()
	return nil
}

// reportUnmatchedOutcomes surfaces the outcomes that were acknowledged but
// reached no application. Without this the record exists only in the database
// and nothing ever points at it; bug #533's whole purpose is that a lost
// outcome stays visible rather than merely stored. Domains are counted, not
// listed, so a scan summary never enumerates who is emailing the user.
func reportUnmatchedOutcomes() {
	lost, err := storage.UnmatchedOutcomeCounts()
	if err != nil {
		log.Printf("[Tracker] Could not read unmatched outcomes: %v", err)
		return
	}
	if len(lost) == 0 {
		return
	}
	domains := make(map[string]bool, len(lost))
	var rejected, interviews int
	for _, l := range lost {
		domains[l.SenderDomain] = true
		if l.Status == "REJECTED" {
			rejected++
		} else {
			interviews++
		}
	}
	log.Printf(
		"[Tracker] %d outcome email(s) across %d sender domain(s) matched no application (%d rejection-shaped, %d interview-shaped). These are recorded in unmatched_outcomes for correlation once the matching application is confirmed.",
		len(lost), len(domains), rejected, interviews,
	)
}

// notJobPhrases short-circuit classification: emails that are structurally
// about something else entirely (receipts, marketing, application-sent
// confirmations) routinely contain words like "next steps" and must never
// produce a status (bug #20 — a Google payment receipt and a LinkedIn
// "application sent" notice were both classified INTERVIEW_REQUESTED).
var notJobPhrases = []string{
	"we've received your payment",
	"received your payment",
	"payment receipt",
	"your invoice",
	"order confirmation",
	"your application was sent",
	"application has been submitted",
	"automated message",
}

// classifyEmail maps an email (lowercased subject and body) to a funnel
// status candidate, or "" when the email shouldn't affect any application.
// It is only a candidate: the caller must still match the email to a
// company we actually applied to before anything is written.
func classifyEmail(subjectLower, bodyLower string) string {
	combined := subjectLower + " " + bodyLower
	for _, phrase := range notJobPhrases {
		if strings.Contains(combined, phrase) {
			return ""
		}
	}
	if strings.Contains(combined, "unfortunately") || strings.Contains(combined, "not moving forward") || strings.Contains(combined, "decided to pursue other candidates") {
		return "REJECTED"
	}
	if strings.Contains(combined, "interview") || strings.Contains(combined, "next steps") || strings.Contains(combined, "availability") {
		return "INTERVIEW_REQUESTED"
	}
	return ""
}

// genericCompanyLabels are job_funnel company names that must never be
// matched against email content — URL-parsing artifacts from before bug
// #19's fix and placeholder values.
var genericCompanyLabels = map[string]bool{
	"unknown company": true, "en-us": true, "en_us": true, "en": true,
	"apply": true, "jobs": true, "careers": true, "external_career_site": true,
}

// commonWordCompanies are tracked companies whose names are ordinary
// job-email vocabulary ("Remote" — remote.com): confirmed live 2026-07-22
// matching a recruiter thread whose subject merely said "remote". These may
// only match via the sender's domain, never subject text.
var commonWordCompanies = map[string]bool{
	"remote": true, "indeed": true, "hired": true, "wellfound": true,
}

// matchTrackedCompany returns the exact stored company name whose
// (lowercased) value appears in the sender's domain or the subject line —
// covering both direct company senders (glimpse.io) and ATS relays
// (no-reply@greenhouse.io with the company in the subject). Names shorter
// than 4 characters or in the generic-label list never match — a fuzzy hit
// on a label like "en" is how junk updates happen (bug #20) — and
// common-word names only count when they appear in the sender's domain.
func matchTrackedCompany(companies []string, senderDomain, subjectLower string) string {
	for _, company := range companies {
		cl := strings.ToLower(strings.TrimSpace(company))
		if len(cl) < 4 || genericCompanyLabels[cl] {
			continue
		}
		if strings.Contains(senderDomain, cl) {
			return company
		}
		if !commonWordCompanies[cl] && strings.Contains(subjectLower, cl) {
			return company
		}
	}
	return ""
}

type trackerUpdateResult string

const (
	trackerUpdateUnmatched trackerUpdateResult = "unmatched"
	trackerUpdateNoop      trackerUpdateResult = "no_op"
	trackerUpdateUpdated   trackerUpdateResult = "updated"
	trackerUpdateAmbiguous trackerUpdateResult = "ambiguous"
)

type trackCandidate struct {
	id    int
	title string
	url   string
}

// Bounded status_reason codes for outcomes the tracker records. They must
// stay free of email content: the funnel stage ledger copies status_reason
// into reason_code verbatim, and that ledger is explicitly documented to hold
// "only state metadata, never job content".
const (
	OutcomeReasonRejected  = "outcome_email_rejected"
	OutcomeReasonInterview = "outcome_email_interview"
)

func outcomeReason(status string) string {
	if status == "REJECTED" {
		return OutcomeReasonRejected
	}
	return OutcomeReasonInterview
}

// applyOutcome writes the outcome onto exactly one funnel row.
//
// Bug #529: this used to set status alone. The stage ledger is a database
// trigger (storage.createFunnelStageLedgerTrigger) that derives its
// occurred_at from NEW.last_updated and its reason_code from
// NEW.status_reason, so leaving both untouched meant every outcome the
// tracker recorded landed in the ledger backdated to the row's *submission*
// time and labelled with the *previous* state's reason — a rejection arriving
// weeks after the application was filed was written to the ledger as
// occurring at submission time with reason_code "submitted_ok". The funnel
// status itself was always correct; the event record of it was not, and the
// event record is what the outcome feedback loop consumes.
func applyOutcome(tx *sql.Tx, id int, status string) error {
	if _, err := tx.Exec(
		`UPDATE job_funnel SET status = ?, status_reason = ?, last_updated = ? WHERE id = ?`,
		status,
		outcomeReason(status),
		time.Now().UTC(),
		id,
	); err != nil {
		return fmt.Errorf("update tracker outcome: %w", err)
	}
	return nil
}

func filterCandidates(candidates []trackCandidate, subjectLower, bodyLower string) []trackCandidate {
	combined := subjectLower + " " + bodyLower

	var idMatched []trackCandidate
	for _, c := range candidates {
		id := extractATSID(c.url)
		if id != "" && strings.Contains(combined, id) {
			idMatched = append(idMatched, c)
		}
	}
	if len(idMatched) > 0 {
		return idMatched
	}

	var titleMatched []trackCandidate
	for _, c := range candidates {
		if titleInSubject(c.title, subjectLower) {
			titleMatched = append(titleMatched, c)
		}
	}
	if len(titleMatched) > 0 {
		return titleMatched
	}

	return candidates
}

func extractATSID(u string) string {
	parts := strings.Split(u, "?")[0]
	parts = strings.TrimRight(parts, "/")
	segs := strings.Split(parts, "/")
	if len(segs) > 0 {
		last := segs[len(segs)-1]
		if strings.ContainsAny(last, "0123456789") || len(last) > 10 {
			return strings.ToLower(last)
		}
	}
	return ""
}

func titleInSubject(title, subjectLower string) bool {
	f := func(r rune) bool {
		return r < 'a' || r > 'z'
	}
	words := strings.FieldsFunc(strings.ToLower(title), f)
	for _, w := range words {
		if len(w) >= 4 && strings.Contains(subjectLower, w) {
			return true
		}
	}
	return false
}

func updateDBWithTrackerResult(
	messageID,
	companyExact,
	status,
	subjectLower,
	bodyLower,
	senderDomain string,
) (trackerUpdateResult, error) {
	if status != "REJECTED" && status != "INTERVIEW_REQUESTED" {
		return "", fmt.Errorf("unsupported tracker status %q", status)
	}

	db := storage.GetDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tracker update: %w", err)
	}
	defer tx.Rollback()

	result := trackerUpdateUnmatched
	if companyExact != "" {
		// Bug #434: this used to match only APPLIED, which made the whole
		// tracker a no-op for hand-off applications. MANUAL_REQUIRED was
		// already in GetTrackedCompanies' match set, so those emails were
		// fetched, recognised as belonging to a tracked company, and then
		// silently dropped here for want of a candidate row. A real rejection
		// or interview email from a company is strong evidence the user did
		// submit the application the agent handed them, so the outcome is
		// recorded rather than discarded.
		rows, err := tx.Query(
			`SELECT id, job_title, url FROM job_funnel
			 WHERE company_name = ? AND status IN ('APPLIED', 'MANUAL_REQUIRED', 'AWAITING_REVIEW')`,
			companyExact,
		)
		if err != nil {
			return "", fmt.Errorf("query applied applications: %w", err)
		}

		var candidates []trackCandidate
		for rows.Next() {
			var c trackCandidate
			if err := rows.Scan(&c.id, &c.title, &c.url); err == nil {
				candidates = append(candidates, c)
			}
		}
		rows.Close()

		if len(candidates) == 0 {
			result = trackerUpdateNoop
		} else if len(candidates) == 1 {
			if err := applyOutcome(tx, candidates[0].id, status); err != nil {
				return "", err
			}
			result = trackerUpdateUpdated
		} else {
			filtered := filterCandidates(candidates, subjectLower, bodyLower)
			if len(filtered) == 1 {
				if err := applyOutcome(tx, filtered[0].id, status); err != nil {
					return "", err
				}
				result = trackerUpdateUpdated
			} else {
				if err := storage.LogManualRequired(companyExact, "Ambiguous "+status, "Multiple open applications match this outcome email", ""); err != nil {
					log.Printf("[Tracker] WARNING: failed to log manual review for ambiguous email: %v", err)
				}
				result = trackerUpdateAmbiguous
			}
		}
	}

	// Bug #533: an outcome-shaped email that reaches no application is about
	// to be acknowledged, and StartTracker never revisits an acknowledged
	// Message-ID. Record the fact before that happens, in this same
	// transaction, so the evidence survives the decision to stop retrying it.
	// trackerUpdateNoop counts too: the company was recognised and the outcome
	// still landed nowhere.
	if result == trackerUpdateUnmatched || result == trackerUpdateNoop {
		if err := storage.RecordUnmatchedOutcomeTx(tx, messageID, status, senderDomain); err != nil {
			return "", err
		}
	}

	if messageID != "" {
		if _, err := tx.Exec(
			`INSERT INTO processed_emails (message_id, processed_at)
			 VALUES (?, CURRENT_TIMESTAMP)
			 ON CONFLICT(message_id) DO NOTHING`,
			messageID,
		); err != nil {
			return "", fmt.Errorf("acknowledge tracker email: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tracker outcome: %w", err)
	}
	return result, nil
}

func logRejectionFeedback(company, subject, reason string) {
	reportPath := filepath.Join("applications", "rejection_feedback.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		if err := os.MkdirAll("applications", security.PrivateDirMode); err != nil {
			log.Printf("[Tracker] WARNING: could not create private feedback directory: %v", err)
			return
		}
		header := "# 📉 Rejection Analytics\n\nThis file tracks the exact reasons why companies are rejecting your applications so you can improve your resume.\n\n"
		if err := os.WriteFile(reportPath, []byte(header), security.PrivateFileMode); err != nil {
			log.Printf("[Tracker] WARNING: could not initialize private feedback report: %v", err)
			return
		}
	} else if err != nil {
		log.Printf("[Tracker] WARNING: could not inspect private feedback report: %v", err)
		return
	}

	entry := fmt.Sprintf("### 🏢 %s\n- **Email Subject:** %s\n- **HR Feedback:** %s\n\n", company, subject, reason)

	f, err := os.OpenFile(
		reportPath,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		security.PrivateFileMode,
	)
	if err != nil {
		log.Printf("[Tracker] WARNING: could not open private feedback report: %v", err)
		return
	}
	defer f.Close()
	if err := f.Chmod(security.PrivateFileMode); err != nil {
		log.Printf("[Tracker] WARNING: could not secure feedback report: %v", err)
		return
	}
	if _, err := f.WriteString(entry); err != nil {
		log.Printf("[Tracker] WARNING: could not write private feedback report: %v", err)
	}
}

func extractBody(msg *imap.Message, section *imap.BodySectionName) string {
	r := msg.GetBody(section)
	if r == nil {
		return ""
	}

	mr, err := mail.CreateReader(r)
	if err != nil {
		return ""
	}

	var textBody strings.Builder
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			break
		}

		switch p.Header.(type) {
		case *mail.InlineHeader:
			b, _ := util.ReadAll(p.Body)
			textBody.Write(b)
		}
	}
	return textBody.String()
}
