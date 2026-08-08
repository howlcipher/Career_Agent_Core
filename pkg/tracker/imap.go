package tracker

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"sort"
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

// trackerRangeBatchSize bounds how many messages a single scan will fetch and
// classify from either the forward (new-mail) range or the historical
// catch-up range. Two ranges are each bounded independently, so a scan can
// process up to 2x this many messages. This exists so a large historical
// backlog (bug #534) cannot turn one scan into an uncontrolled burst of LLM
// calls, and so a crash mid-batch only has to replay a bounded amount of
// work, not the whole remaining backlog.
//
// A var, not a const: tests lower it to exercise multi-batch draining
// without creating hundreds of synthetic messages.
var trackerRangeBatchSize uint32 = 200

// IMAPConfig holds credentials for the tracker
type IMAPConfig struct {
	Server   string
	Username string
	Password string
}

// StartTracker connects to the IMAP server and scans for application
// updates using a durable UID checkpoint (bug #534).
//
// The previous implementation re-derived its fetch window from the newest
// ~50 mailbox sequence numbers on every scan. Sequence numbers are
// mailbox-relative and shift as messages are added or removed, so they
// cannot serve as a durable position, and anchoring to "the newest N" meant
// the window only ever moved forward: mail that fell out of it during any
// outage longer than the window was never fetched again. This version
// tracks two independent, durable ranges keyed by IMAP UID (stable within a
// mailbox's UIDVALIDITY generation, per storage.TrackerCursor):
//
//   - the forward range: ordinary new mail above the last durably-handled
//     UID, processed every scan.
//   - the historical catch-up range: a bounded window established once, at
//     bootstrap, from live evidence (storage.EarliestTrackableApplicationTime)
//     and drained in bounded batches across scans, independently of the
//     forward range so new mail is never starved by a large backlog.
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

	return scanInbox(c)
}

// scanInbox holds the IMAP protocol logic that runs once a client is
// authenticated. It is split out from StartTracker so tests can exercise the
// real UID fetch/search flow against an in-process IMAP server without
// needing TLS.
func scanInbox(c *client.Client) error {
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return err
	}

	if mbox.Messages == 0 {
		log.Println("[Tracker] Inbox is empty. Nothing to track.")
		return nil
	}

	trackedCompanies, err := storage.GetTrackedCompanies()
	if err != nil {
		log.Printf("[Tracker] Could not load tracked companies (DB not initialized?): %v — running detection-only, no status updates.", err)
	}

	cursor, err := storage.GetTrackerCursor()
	if err != nil {
		return fmt.Errorf("read tracker checkpoint: %w", err)
	}
	if cursor == nil || cursor.UidValidity != mbox.UidValidity {
		// A missing cursor is the pre-#534 (or brand-new database) state. A
		// UIDVALIDITY mismatch is IMAP's explicit signal that this mailbox's
		// old UIDs no longer mean anything — the safe response is a fresh
		// bounded resync, not trusting the stale numbers. processed_emails'
		// Message-ID dedup (untouched by either case) is what prevents this
		// from producing duplicate outcome writes if ranges overlap.
		if cursor != nil {
			log.Printf("[Tracker] Mailbox UID generation changed (was %d, now %d) — resynchronizing safely.", cursor.UidValidity, mbox.UidValidity)
		}
		built, err := bootstrapTrackerCursor(c, mbox)
		if err != nil {
			return fmt.Errorf("bootstrap tracker checkpoint: %w", err)
		}
		cursor = &built
	}

	section := &imap.BodySectionName{}
	fetchItems := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, section.FetchItem()}

	// Forward range: ordinary new mail above the last durably-handled UID.
	// mbox.UidNext is the UID that will be assigned to the *next* new
	// message, so mbox.UidNext-1 is the highest UID that exists right now.
	if mbox.UidNext > 0 && mbox.UidNext-1 > cursor.ForwardUID {
		from := cursor.ForwardUID + 1
		to := mbox.UidNext - 1
		if to-from+1 > trackerRangeBatchSize {
			to = from + trackerRangeBatchSize - 1
		}
		msgs, err := fetchUIDRange(c, from, to, fetchItems)
		if err != nil {
			return fmt.Errorf("fetch new mail: %w", err)
		}
		highest, complete := processMessages(msgs, section, trackedCompanies)
		// A gap (deleted mail, or simply no new messages this batch) must
		// still advance the checkpoint across the whole fetched range when
		// nothing in it failed — otherwise an empty stretch of UID space
		// re-fetches forever and never lets the checkpoint move past it.
		if complete {
			highest = to
		}
		if highest > cursor.ForwardUID {
			if err := storage.AdvanceForwardUID(highest); err != nil {
				return fmt.Errorf("advance forward checkpoint: %w", err)
			}
		}
	}

	// Historical catch-up range: bounded, independent of the forward range
	// above, so new mail is never starved by a large backlog.
	if !cursor.CatchupComplete {
		from := cursor.CatchupNextUID
		to := cursor.CatchupCeilingUID - 1
		if to-from+1 > trackerRangeBatchSize {
			to = from + trackerRangeBatchSize - 1
		}
		msgs, err := fetchUIDRange(c, from, to, fetchItems)
		if err != nil {
			return fmt.Errorf("fetch historical catch-up range: %w", err)
		}
		highest, complete := processMessages(msgs, section, trackedCompanies)
		// Same gap-safety as the forward range above: an empty or
		// partially-empty fetched range must still advance past the whole
		// range once nothing in it failed, or a long stretch of deleted mail
		// stalls catch-up forever (confirmed live 2026-08-08: a ~200-UID gap
		// with no surviving messages froze catch-up indefinitely under the
		// original highest-only logic).
		if complete {
			highest = to
		}
		if highest >= from {
			nextUID := highest + 1
			if err := storage.AdvanceCatchupUID(nextUID); err != nil {
				return fmt.Errorf("advance catch-up checkpoint: %w", err)
			}
			remaining := int64(cursor.CatchupCeilingUID) - int64(nextUID)
			if remaining <= 0 {
				log.Println("[Tracker] Historical catch-up complete — the mailbox is fully covered by the durable checkpoint.")
			} else {
				log.Printf("[Tracker] Historical catch-up: %d message(s) processed this scan, approximately %d remaining.", len(msgs), remaining)
			}
		}
	}

	reportUnmatchedOutcomes()
	return nil
}

// fetchUIDRange fetches [from, to] by UID and returns the messages sorted
// ascending by UID, so the caller can process oldest-first and advance its
// checkpoint monotonically. Returns (nil, nil) for an empty or invalid range.
func fetchUIDRange(c *client.Client, from, to uint32, items []imap.FetchItem) ([]*imap.Message, error) {
	if from == 0 || to == 0 || from > to {
		return nil, nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, items, messages)
	}()

	var msgs []*imap.Message
	for msg := range messages {
		msgs = append(msgs, msg)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Uid < msgs[j].Uid })
	return msgs, nil
}

// bootstrapTrackerCursor establishes the checkpoint from scratch: on the
// first-ever scan, and again whenever UIDVALIDITY has changed. The forward
// range is anchored at "now" (the mailbox's current highest UID) so ordinary
// new mail is covered starting this scan without re-fetching what already
// exists. The historical catch-up floor comes from live evidence — the
// earliest still-open application a real outcome email could possibly belong
// to — rather than a hardcoded lookback window, so it naturally covers
// whatever the actual outage was instead of one specific incident.
func bootstrapTrackerCursor(c *client.Client, mbox *imap.MailboxStatus) (storage.TrackerCursor, error) {
	ceiling := mbox.UidNext
	forwardUID := uint32(0)
	if mbox.UidNext > 0 {
		forwardUID = mbox.UidNext - 1
	}

	cursor := storage.TrackerCursor{
		UidValidity:       mbox.UidValidity,
		ForwardUID:        forwardUID,
		CatchupFloorUID:   ceiling,
		CatchupCeilingUID: ceiling,
		CatchupNextUID:    ceiling,
		CatchupComplete:   true,
	}

	earliest, ok, err := storage.EarliestTrackableApplicationTime()
	if err != nil {
		return cursor, fmt.Errorf("determine historical catch-up floor: %w", err)
	}
	if ok && ceiling > 1 {
		uids, err := c.UidSearch(&imap.SearchCriteria{Since: earliest})
		if err != nil {
			return cursor, fmt.Errorf("search historical catch-up floor: %w", err)
		}
		if len(uids) > 0 {
			floor := uids[0]
			for _, u := range uids[1:] {
				if u < floor {
					floor = u
				}
			}
			if floor < ceiling {
				cursor.CatchupFloorUID = floor
				cursor.CatchupNextUID = floor
				cursor.CatchupComplete = false
			}
		}
	}

	if err := storage.BootstrapTrackerCursor(cursor); err != nil {
		return cursor, err
	}

	if cursor.CatchupComplete {
		log.Println("[Tracker] Checkpoint bootstrapped. No historical catch-up needed — no open application predates this scan.")
	} else {
		log.Printf("[Tracker] Checkpoint bootstrapped. Historical catch-up window established, approximately %d message(s) to recover in bounded batches.", cursor.CatchupCeilingUID-cursor.CatchupFloorUID)
	}
	return cursor, nil
}

// processMessages handles a batch of messages already sorted ascending by
// UID and returns the highest UID that was durably handled — acknowledged
// via processed_emails, and if outcome-shaped, recorded — plus whether every
// message in msgs was handled without a failure.
//
// The two return values matter separately. IMAP UIDs are not contiguous:
// deleted mail leaves gaps, so a fetched range can contain far fewer
// messages than its UID span, including zero. The caller must still be able
// to advance its checkpoint across a gap it fetched and found empty — bug
// #534's fix originally only advanced past the highest UID a message
// actually carried, which meant a wide enough gap (confirmed live: a
// ~200-UID span with no surviving messages in it) stalled catch-up forever,
// silently re-fetching the same empty range every scan with nothing to show
// for it. complete=true tells the caller the whole fetched range — not just
// the highest message UID within it — is now safely covered, gaps included.
//
// Processing stops at the first message whose acknowledgement fails, which
// is when complete is false (0 is returned as highest if that is the very
// first message). This is deliberate: the checkpoint must never advance past
// a message whose handling is not known to have succeeded, so a later
// message in the same batch is not allowed to "skip over" an earlier one
// that failed — the failed message, and everything after it, is retried
// from the same UID on the next scan.
func processMessages(msgs []*imap.Message, section *imap.BodySectionName, trackedCompanies []string) (highest uint32, complete bool) {
	for _, msg := range msgs {
		if msg.Envelope == nil {
			// No Message-ID to key acknowledgement or dedup on, and nothing
			// to classify. Treated as handled — the alternative is a
			// malformed message permanently jamming the checkpoint, which is
			// worse than the (rare) risk of not re-examining it.
			highest = msg.Uid
			continue
		}
		if storage.WasEmailProcessed(msg.Envelope.MessageId) {
			highest = msg.Uid
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
		if status == "" {
			if err := storage.MarkEmailProcessed(msg.Envelope.MessageId); err != nil {
				log.Printf("[Tracker] Failed to mark email processed: %v. Stopping this batch for retry next scan.", err)
				return highest, false
			}
			highest = msg.Uid
			continue
		}

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
				"[Tracker] Failed to persist %s outcome for %q: %v. Stopping this batch for retry next scan.",
				status,
				company,
				err,
			)
			return highest, false
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
		highest = msg.Uid
	}
	return highest, true
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
