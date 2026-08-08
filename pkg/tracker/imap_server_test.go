package tracker

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
)

// testMailbox is a minimal, purpose-built IMAP backend for bug #534's
// regression tests. github.com/emersion/go-imap's own memory backend
// (vendored as a dependency already, via backend/memory) hardcodes
// UidValidity to 1 in Mailbox.Status, so it cannot exercise a mailbox
// generation change. testMailbox reuses memory.Message (Fetch/Match are
// exported and unchanged) for envelope/body/search handling and only
// reimplements the thin Mailbox/User/Backend wrapper needed to make
// UidValidity test-controllable.
type testMailbox struct {
	messages    []*memory.Message
	uidValidity uint32
	nextUID     uint32
}

func (m *testMailbox) Name() string { return "INBOX" }

func (m *testMailbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{Delimiter: "/", Name: "INBOX"}, nil
}

func (m *testMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	status := imap.NewMailboxStatus("INBOX", items)
	status.PermanentFlags = []string{"\\*"}
	status.UnseenSeqNum = 0
	for _, item := range items {
		switch item {
		case imap.StatusMessages:
			status.Messages = uint32(len(m.messages))
		case imap.StatusUidNext:
			status.UidNext = m.nextUID
		case imap.StatusUidValidity:
			status.UidValidity = m.uidValidity
		}
	}
	return status, nil
}

func (m *testMailbox) SetSubscribed(bool) error { return nil }
func (m *testMailbox) Check() error             { return nil }

func (m *testMailbox) ListMessages(uid bool, seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)
	for i, msg := range m.messages {
		seqNum := uint32(i + 1)
		id := seqNum
		if uid {
			id = msg.Uid
		}
		if !seqset.Contains(id) {
			continue
		}
		fetched, err := msg.Fetch(seqNum, items)
		if err != nil {
			continue
		}
		ch <- fetched
	}
	return nil
}

func (m *testMailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	var ids []uint32
	for i, msg := range m.messages {
		seqNum := uint32(i + 1)
		ok, err := msg.Match(seqNum, criteria)
		if err != nil || !ok {
			continue
		}
		id := seqNum
		if uid {
			id = msg.Uid
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *testMailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(body); err != nil {
		return err
	}
	m.messages = append(m.messages, &memory.Message{
		Uid:   m.nextUID,
		Date:  date,
		Size:  uint32(buf.Len()),
		Flags: flags,
		Body:  buf.Bytes(),
	})
	m.nextUID++
	return nil
}

func (m *testMailbox) UpdateMessagesFlags(bool, *imap.SeqSet, imap.FlagsOp, []string) error {
	return nil
}
func (m *testMailbox) CopyMessages(bool, *imap.SeqSet, string) error { return nil }
func (m *testMailbox) Expunge() error                                { return nil }

type testUser struct {
	mailbox *testMailbox
}

func (u *testUser) Username() string { return "tester" }
func (u *testUser) ListMailboxes(bool) ([]backend.Mailbox, error) {
	return []backend.Mailbox{u.mailbox}, nil
}
func (u *testUser) GetMailbox(name string) (backend.Mailbox, error) {
	if name != "INBOX" {
		return nil, backend.ErrNoSuchMailbox
	}
	return u.mailbox, nil
}
func (u *testUser) CreateMailbox(string) error         { return nil }
func (u *testUser) DeleteMailbox(string) error         { return nil }
func (u *testUser) RenameMailbox(string, string) error { return nil }
func (u *testUser) Logout() error                      { return nil }

type testBackend struct {
	user *testUser
}

func (b *testBackend) Login(_ *imap.ConnInfo, username, password string) (backend.User, error) {
	if username != "tester" || password != "secret" {
		return nil, backend.ErrInvalidCredentials
	}
	return b.user, nil
}

// startTestIMAPServer boots a real go-imap server (protocol-complete, over a
// local TCP loopback listener) backed by testMailbox, and returns a logged-in
// client plus the mailbox for the test to seed with messages. The server is
// stopped via t.Cleanup.
func startTestIMAPServer(t *testing.T, mbox *testMailbox) *client.Client {
	t.Helper()

	be := &testBackend{user: &testUser{mailbox: mbox}}
	srv := server.New(be)
	srv.AllowInsecureAuth = true

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(l)
	t.Cleanup(func() {
		srv.Close()
	})

	c, err := client.Dial(l.Addr().String())
	if err != nil {
		t.Fatalf("dial test IMAP server: %v", err)
	}
	t.Cleanup(func() {
		c.Logout()
	})
	if err := c.Login("tester", "secret"); err != nil {
		t.Fatalf("login to test IMAP server: %v", err)
	}
	return c
}

// rawMessage builds a minimal RFC822 message with the headers the tracker's
// classification and matching logic reads.
func rawMessage(messageID, from, subject, body string) *bytes.Reader {
	msg := fmt.Sprintf(
		"From: %s\r\nSubject: %s\r\nMessage-Id: %s\r\nDate: %s\r\nContent-Type: text/plain\r\n\r\n%s\r\n",
		from, subject, messageID, time.Now().Format(time.RFC1123Z), body,
	)
	return bytes.NewReader([]byte(msg))
}
