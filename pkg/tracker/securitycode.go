package tracker

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

// improvements.md #32: retrieving one-time codes an ATS emails mid-application.
//
// bugs.md #93 established the need: Greenhouse issued a security code at the
// exact second of a submit and the application could not complete until it was
// typed back into the form. The agent had no way to read it, so such jobs could
// only ever reach MANUAL_REQUIRED.
//
// This deliberately reuses the IMAP credentials the email tracker already
// uses -- no OAuth, no new dependency, no additional setup. It also keeps the
// search as narrow as the task allows: only messages from known ATS senders,
// only ones whose subject reads like a code notification, and only ones that
// arrived after the submit that triggered them. Nothing else in the mailbox is
// read, which matters because this is the user's personal email.

// atsCodeSenders are the domains an ATS sends one-time codes from. A message
// from anywhere else is never examined.
var atsCodeSenders = []string{
	"greenhouse-mail.io",
	"greenhouse.io",
	"lever.co",
	"hire.lever.co",
	"ashbyhq.com",
	"workable.com",
	"smartrecruiters.com",
}

// codeSubjectHints keep the search to messages that actually announce a code.
var codeSubjectHints = []string{
	"security code",
	"verification code",
	"confirmation code",
	"one-time code",
	"your code",
}

// codePattern matches the code itself. ATS codes are a short run of mixed
// letters and digits on a line of their own, or immediately after a colon.
// Requiring 6-12 characters and rejecting all-lowercase words keeps ordinary
// prose from matching.
var codePattern = regexp.MustCompile(`\b([A-Za-z0-9]{6,12})\b`)

// FetchSecurityCode returns the most recent one-time code sent by an ATS after
// notBefore, or an empty string when there is none.
//
// The caller is expected to poll: the email arrives seconds to a minute after
// the submit that triggers it.
func FetchSecurityCode(cfg IMAPConfig, notBefore time.Time) (code string, subject string, err error) {
	c, err := client.DialTLS(cfg.Server, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer c.Logout()

	if err := c.Login(cfg.Username, cfg.Password); err != nil {
		return "", "", fmt.Errorf("failed to login: %w", err)
	}
	if _, err := c.Select("INBOX", true); err != nil { // read-only
		return "", "", err
	}

	// IMAP SINCE has date granularity, so widen by a day and filter precisely
	// in Go below.
	criteria := imap.NewSearchCriteria()
	criteria.Since = notBefore.Add(-24 * time.Hour)
	ids, err := c.Search(criteria)
	if err != nil {
		return "", "", err
	}
	if len(ids) == 0 {
		return "", "", nil
	}

	// Newest first, and never walk far back: a code is minutes old by design.
	if len(ids) > 40 {
		ids = ids[len(ids)-40:]
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	section := &imap.BodySectionName{}
	messages := make(chan *imap.Message, 20)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}, messages)
	}()

	var bestTime time.Time
	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}
		if !msg.InternalDate.IsZero() && msg.InternalDate.Before(notBefore) {
			continue
		}
		if !senderIsATS(msg.Envelope) || !subjectAnnouncesCode(msg.Envelope.Subject) {
			continue
		}
		body := readMessageBody(msg, section)
		found := extractSecurityCode(body)
		if found == "" {
			continue
		}
		if msg.InternalDate.After(bestTime) {
			bestTime = msg.InternalDate
			code = found
			subject = msg.Envelope.Subject
		}
	}
	if ferr := <-done; ferr != nil {
		return "", "", ferr
	}
	return code, subject, nil
}

func senderIsATS(env *imap.Envelope) bool {
	for _, a := range env.From {
		host := strings.ToLower(a.HostName)
		for _, s := range atsCodeSenders {
			if host == s || strings.HasSuffix(host, "."+s) {
				return true
			}
		}
	}
	return false
}

func subjectAnnouncesCode(subject string) bool {
	lower := strings.ToLower(subject)
	for _, h := range codeSubjectHints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func readMessageBody(msg *imap.Message, section *imap.BodySectionName) string {
	r := msg.GetBody(section)
	if r == nil {
		return ""
	}
	mr, err := mail.CreateReader(r)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			break
		}
		if _, ok := part.Header.(*mail.InlineHeader); ok {
			b, _ := io.ReadAll(part.Body)
			sb.Write(b)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// extractSecurityCode pulls the code out of a message body.
//
// Anchored on the sentence that introduces it rather than scanning the whole
// message, because a marketing footer is full of 6-12 character tokens. Falls
// back to a standalone line only when the anchor is absent.
func extractSecurityCode(body string) string {
	stripped := stripHTMLTags(body)
	lower := strings.ToLower(stripped)

	anchors := []string{
		"paste this code", "copy and paste this code", "your code is",
		"security code:", "verification code:", "code:",
	}
	for _, a := range anchors {
		i := strings.Index(lower, a)
		if i < 0 {
			continue
		}
		tail := stripped[i+len(a):]
		if m := codePattern.FindStringSubmatch(tail); m != nil {
			if isPlausibleCode(m[1]) {
				return m[1]
			}
			// The first token after the anchor can be a stray word ("into",
			// "field"); scan a little further before giving up.
			for _, cand := range codePattern.FindAllStringSubmatch(tail, 8) {
				if isPlausibleCode(cand[1]) {
					return cand[1]
				}
			}
		}
	}

	for _, line := range strings.Split(stripped, "\n") {
		line = strings.TrimSpace(line)
		if isPlausibleCode(line) {
			return line
		}
	}
	return ""
}

// isPlausibleCode rejects ordinary words. A real ATS code mixes cases or
// digits; "application" and "greenhouse" do not.
func isPlausibleCode(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 6 || len(s) > 12 {
		return false
	}
	var hasUpper, hasLower, hasDigit bool
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	// Mixed case, or letters with digits. A plain lowercase word is prose.
	return (hasUpper && hasLower) || (hasDigit && (hasUpper || hasLower)) || (hasDigit && !hasUpper && !hasLower)
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return htmlTagPattern.ReplaceAllString(s, "\n")
}
