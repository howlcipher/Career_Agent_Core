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

	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		subject := strings.ToLower(msg.Envelope.Subject)
		
		var senderEmail string
		if len(msg.Envelope.From) > 0 {
			senderEmail = msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName
		}
		
		bodyText := extractBody(msg, section)
		bodyLower := strings.ToLower(bodyText)

		// Analyze email for rejection or interview
		status := ""
		if strings.Contains(bodyLower, "unfortunately") || strings.Contains(bodyLower, "not moving forward") || strings.Contains(bodyLower, "decided to pursue other candidates") {
			status = "REJECTED"
		} else if strings.Contains(bodyLower, "interview") || strings.Contains(bodyLower, "next steps") || strings.Contains(bodyLower, "availability") {
			if !strings.Contains(bodyLower, "automated message") {
				status = "INTERVIEW_REQUESTED"
			}
		}

		if status != "" {
			parts := strings.Split(senderEmail, "@")
			if len(parts) == 2 {
				domain := parts[1]
				companyGuess := strings.Split(domain, ".")[0]
				
				log.Printf("[Tracker] Detected %s from %s (%s). Updating database.", status, companyGuess, subject)
				updateDBWithTrackerResult(companyGuess, status)

				if status == "REJECTED" {
					geminiClient := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
					reason, err := geminiClient.ExtractRejectionReason(bodyText)
					if err != nil {
						reason = "Generic templated rejection (no specific reason provided)"
					}
					logRejectionFeedback(companyGuess, subject, reason)
				}
			}
		}
	}

	if err := <-done; err != nil {
		return err
	}
	
	return nil
}

func updateDBWithTrackerResult(companyQuery, status string) {
	db := storage.GetDB()
	if db == nil {
		return
	}
	query := "UPDATE job_funnel SET status = ? WHERE company_name LIKE ? AND status = 'APPLIED'"
	db.Exec(query, status, "%"+companyQuery+"%")
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
