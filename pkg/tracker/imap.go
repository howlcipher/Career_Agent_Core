package tracker

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
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

		status := classifyEmail(subject, strings.ToLower(bodyText))
		if status != "" {
			company := matchTrackedCompany(trackedCompanies, senderDomain, subject)
			if company == "" {
				log.Printf("[Tracker] Detected %s-shaped email from %s (%s) but it matches no tracked application — ignoring.", status, senderDomain, subject)
			} else {
				log.Printf("[Tracker] Detected %s for tracked company %q from %s (%s). Updating database.", status, company, senderDomain, subject)
				updateDBWithTrackerResult(company, status)

				if status == "REJECTED" {
					llmClient := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
					reason, err := llmClient.ExtractRejectionReason(bodyText)
					if err != nil {
						reason = "Generic templated rejection (no specific reason provided)"
					}
					logRejectionFeedback(company, subject, reason)
				}
			}
		}
		if err := storage.MarkEmailProcessed(msg.Envelope.MessageId); err != nil {
			log.Printf("[Tracker] Failed to mark email processed: %v", err)
		}
	}

	if err := <-done; err != nil {
		return err
	}

	return nil
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

func updateDBWithTrackerResult(companyExact, status string) {
	db := storage.GetDB()
	if db == nil {
		return
	}
	// Exact company match only, and only forward from APPLIED — never
	// touch rows the email cannot legitimately be about.
	query := "UPDATE job_funnel SET status = ? WHERE company_name = ? AND status = 'APPLIED'"
	db.Exec(query, status, companyExact)
}

func logRejectionFeedback(company, subject, reason string) {
	reportPath := filepath.Join("applications", "rejection_feedback.md")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		os.MkdirAll("applications", 0755)
		header := "# 📉 Rejection Analytics\n\nThis file tracks the exact reasons why companies are rejecting your applications so you can improve your resume.\n\n"
		os.WriteFile(reportPath, []byte(header), 0644)
	}

	entry := fmt.Sprintf("### 🏢 %s\n- **Email Subject:** %s\n- **HR Feedback:** %s\n\n", company, subject, reason)
	
	f, err := os.OpenFile(reportPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err == nil {
		f.WriteString(entry)
		f.Close()
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
			b, _ := io.ReadAll(p.Body)
			textBody.Write(b)
		}
	}
	return textBody.String()
}
