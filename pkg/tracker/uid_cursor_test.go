package tracker

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// insertTrackerTestJobWithDiscovered seeds a job_funnel row with an explicit
// discovered_at, which is what bug #534's historical catch-up floor is
// derived from (storage.EarliestTrackableApplicationTime).
func insertTrackerTestJobWithDiscovered(t *testing.T, db *sql.DB, company, status, url string, discoveredAt time.Time) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO job_funnel (company_name, job_title, url, status, discovered_at)
		 VALUES (?, 'Engineer', ?, ?, ?)`,
		company, url, status, discoveredAt,
	); err != nil {
		t.Fatalf("insert tracker test job: %v", err)
	}
}

func withSmallBatch(t *testing.T, n uint32) {
	t.Helper()
	orig := trackerRangeBatchSize
	trackerRangeBatchSize = n
	t.Cleanup(func() { trackerRangeBatchSize = orig })
}

// TestTrackerCursorBootstrapFromPreFix534State covers the "cursor/bootstrap"
// requirement: a database that predates #534 has no tracker_cursor row at
// all (GetTrackerCursor must return nil, not an error), and bootstrapping
// installs a usable checkpoint.
func TestTrackerCursorBootstrapFromPreFix534State(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	_ = db

	cursor, err := storage.GetTrackerCursor()
	if err != nil {
		t.Fatalf("GetTrackerCursor on a pre-#534 database: %v", err)
	}
	if cursor != nil {
		t.Fatalf("expected no cursor on a fresh database, got %+v", cursor)
	}

	built := storage.TrackerCursor{
		UidValidity:       100,
		ForwardUID:        5,
		CatchupFloorUID:   1,
		CatchupCeilingUID: 5,
		CatchupNextUID:    1,
		CatchupComplete:   false,
	}
	if err := storage.BootstrapTrackerCursor(built); err != nil {
		t.Fatalf("BootstrapTrackerCursor: %v", err)
	}
	got, err := storage.GetTrackerCursor()
	if err != nil {
		t.Fatalf("GetTrackerCursor after bootstrap: %v", err)
	}
	if got == nil || *got != built {
		t.Fatalf("cursor after bootstrap = %+v, want %+v", got, built)
	}

	if err := storage.AdvanceForwardUID(6); err != nil {
		t.Fatalf("AdvanceForwardUID: %v", err)
	}
	if err := storage.AdvanceForwardUID(3); err != nil {
		t.Fatalf("AdvanceForwardUID (regression attempt): %v", err)
	}
	got, _ = storage.GetTrackerCursor()
	if got.ForwardUID != 6 {
		t.Errorf("forward UID = %d after a lower advance call, want 6 — checkpoint must never move backwards", got.ForwardUID)
	}
}

// TestSteadyStateForwardPathAndOlderMailIsUntouched covers "steady state":
// new mail above the checkpoint is processed, and mail that existed before
// the checkpoint was established is never swept in without evidence to
// justify it (no open application existed at bootstrap time, so nothing
// pre-dates a real recovery need).
func TestSteadyStateForwardPathAndOlderMailIsUntouched(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	mbox.CreateMessage(nil, time.Now(), rawMessage("<pre-existing@example.com>", "noise@newsletter.com", "weekly digest", "nothing relevant"))

	if err := scanInbox(c); err != nil {
		t.Fatalf("bootstrap scan: %v", err)
	}
	if storage.WasEmailProcessed("<pre-existing@example.com>") {
		t.Fatal("pre-existing mail was processed at bootstrap with no open-application evidence to justify it")
	}
	cursor, err := storage.GetTrackerCursor()
	if err != nil || cursor == nil {
		t.Fatalf("expected a cursor after bootstrap: cursor=%v err=%v", cursor, err)
	}
	if !cursor.CatchupComplete {
		t.Fatal("catch-up should be immediately complete when there is no open-application evidence")
	}

	db := storage.GetDB()
	insertTrackerTestJob(t, db, "Steady Corp", "APPLIED", "https://example.com/steady")

	mbox.CreateMessage(nil, time.Now(), rawMessage("<new-mail@example.com>", "hr@steadycorp.com", "Steady Corp - update",
		"unfortunately we are not moving forward with your application."))

	if err := scanInbox(c); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if !storage.WasEmailProcessed("<new-mail@example.com>") {
		t.Fatal("new mail above the checkpoint was not processed")
	}
	assertTrackerTestStatus(t, db, "Steady Corp", "REJECTED")

	if storage.WasEmailProcessed("<pre-existing@example.com>") {
		t.Fatal("pre-checkpoint mail was reclassified by a later scan; it must stay untouched")
	}

	// A third scan with no new mail must be a clean no-op.
	if err := scanInbox(c); err != nil {
		t.Fatalf("third (idle) scan: %v", err)
	}
	var ledgerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerCount); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("ledger has %d event(s) after an idle scan, want exactly 1 (from the one real outcome)", ledgerCount)
	}
}

// TestHistoricalBacklogBeyondOldWindowIsRecovered is the ">50 messages"
// regression: an outcome email older than the old ~51-message window must
// still be recovered. Against the pre-#534 implementation
// (pkg/tracker/imap.go:54-62, `from = mbox.Messages - 50`) this fails: with
// 60 messages in the mailbox, the oldest (sequence 1, holding the outcome)
// falls in seq range [1,9], strictly below the old `from = 60-50 = 10`
// boundary, so it would never be fetched at all.
func TestHistoricalBacklogBeyondOldWindowIsRecovered(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	evidenceDate := time.Now().AddDate(0, 0, -20)
	insertTrackerTestJobWithDiscovered(t, db, "Historic Corp", "APPLIED", "https://example.com/historic", evidenceDate)

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	outcomeDate := time.Now().AddDate(0, 0, -15)
	mbox.CreateMessage(nil, outcomeDate, rawMessage("<old-outcome@example.com>", "hr@historiccorp.com", "Historic Corp - update",
		"unfortunately we are not moving forward with your application."))
	for i := 0; i < 59; i++ {
		mbox.CreateMessage(nil, time.Now(), rawMessage(
			"<filler-"+string(rune('a'+i%26))+string(rune('0'+i/26))+"@example.com>",
			"noise@newsletter.com", "weekly digest", "nothing relevant"))
	}
	if len(mbox.messages) != 60 {
		t.Fatalf("test setup: mailbox has %d messages, want 60", len(mbox.messages))
	}

	if err := scanInbox(c); err != nil {
		t.Fatalf("scan: %v", err)
	}

	assertTrackerTestStatus(t, db, "Historic Corp", "REJECTED")
	if !storage.WasEmailProcessed("<old-outcome@example.com>") {
		t.Fatal("the historical outcome email was not acknowledged")
	}
}

// TestMultiBatchCatchUpDrainsAcrossScans covers "multi-batch catch-up": a
// backlog larger than one bounded batch drains across subsequent scans
// without losing anything, and completion is reported accurately.
func TestMultiBatchCatchUpDrainsAcrossScans(t *testing.T) {
	withSmallBatch(t, 2)
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	evidenceDate := time.Now().AddDate(0, 0, -20)
	companies := []string{"Batch1 Corp", "Batch2 Corp", "Batch3 Corp", "Batch4 Corp", "Batch5 Corp"}
	for i, company := range companies {
		insertTrackerTestJobWithDiscovered(t, db, company, "APPLIED", "https://example.com/batch"+string(rune('0'+i)), evidenceDate)
	}

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	outcomeDate := time.Now().AddDate(0, 0, -15)
	for i, company := range companies {
		domain := "hr@" + companySlug(company) + ".com"
		mbox.CreateMessage(nil, outcomeDate, rawMessage(
			"<batch-outcome-"+string(rune('0'+i))+"@example.com>", domain, company+" - update",
			"unfortunately we are not moving forward with your application."))
	}

	// Batch size 2 against 5 historical messages: 3 scans to fully drain.
	for scan := 0; scan < 3; scan++ {
		if err := scanInbox(c); err != nil {
			t.Fatalf("scan %d: %v", scan, err)
		}
	}

	for _, company := range companies {
		assertTrackerTestStatus(t, db, company, "REJECTED")
	}
	cursor, err := storage.GetTrackerCursor()
	if err != nil || cursor == nil {
		t.Fatalf("cursor missing after catch-up: %v", err)
	}
	if !cursor.CatchupComplete {
		t.Error("catch-up should be complete after enough scans to drain the whole backlog")
	}

	// A 4th scan (nothing left to do) must not error or reprocess anything.
	if err := scanInbox(c); err != nil {
		t.Fatalf("scan after completion: %v", err)
	}
}

// TestCatchUpAdvancesPastAGapWithNoSurvivingMessages is the regression for a
// bug this task's own live production validation found: IMAP UIDs are not
// contiguous (deleted mail leaves gaps), so a bounded batch can legitimately
// fetch zero messages even though catch-up is not complete. The original fix
// only advanced the checkpoint to the highest UID a message actually
// carried, so a fetched-but-message-free batch left the checkpoint
// unmoved — and because the next scan re-requests the exact same empty
// range, catch-up stalled on that gap forever. Confirmed live 2026-08-08: a
// real inbox had a ~200-UID gap that froze catch-up indefinitely with zero
// errors and zero forward progress. Against that pre-fix logic this test
// fails: catchup_next_uid never moves past the gap.
func TestCatchUpAdvancesPastAGapWithNoSurvivingMessages(t *testing.T) {
	withSmallBatch(t, 50)
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	evidenceDate := time.Now().AddDate(0, 0, -20)
	insertTrackerTestJobWithDiscovered(t, db, "Before Gap Corp", "APPLIED", "https://example.com/before", evidenceDate)
	insertTrackerTestJobWithDiscovered(t, db, "After Gap Corp", "APPLIED", "https://example.com/after", evidenceDate)

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	outcomeDate := time.Now().AddDate(0, 0, -15)
	mbox.CreateMessage(nil, outcomeDate, rawMessage("<before-gap@example.com>", "hr@beforegapcorp.com", "Before Gap Corp - update",
		"unfortunately we are not moving forward with your application."))
	// Simulate ~250 UIDs' worth of mail that existed once but is gone by the
	// time the tracker ever looks (deleted before any scan ran) — the
	// backend never materializes messages for these UIDs at all.
	mbox.nextUID += 250
	mbox.CreateMessage(nil, outcomeDate, rawMessage("<after-gap@example.com>", "hr@aftergapcorp.com", "After Gap Corp - update",
		"unfortunately we are not moving forward with your application."))

	// Scan 1: batch [1,50] contains only <before-gap> at UID 1, then 49 UIDs
	// of pure gap. The whole batch is handled cleanly (nothing failed), so
	// the checkpoint must advance across the gap too, to the batch's end
	// (UID 51) — not stop at UID 2, the highest UID a real message carried.
	if err := scanInbox(c); err != nil {
		t.Fatalf("scan 1: %v", err)
	}
	assertTrackerTestStatus(t, db, "Before Gap Corp", "REJECTED")
	cursor, err := storage.GetTrackerCursor()
	if err != nil || cursor == nil {
		t.Fatalf("cursor missing: %v", err)
	}
	if cursor.CatchupNextUID != 51 {
		t.Fatalf("catchup_next_uid after scan 1 = %d, want 51 — the checkpoint must cross the gap tail of the batch it already fetched, not stop at the last real message", cursor.CatchupNextUID)
	}

	// Scan 2: batch [51,100] is entirely empty (deep in the gap, no message
	// at all). Under the pre-fix logic this batch would return highest=0 and
	// the checkpoint would never move — this is the exact stall confirmed
	// live 2026-08-08.
	if err := scanInbox(c); err != nil {
		t.Fatalf("scan 2: %v", err)
	}
	cursor, _ = storage.GetTrackerCursor()
	if cursor.CatchupNextUID != 101 {
		t.Fatalf("catchup_next_uid after scan 2 (empty batch) = %d, want 101 — an empty batch must still advance the checkpoint across the whole fetched range", cursor.CatchupNextUID)
	}

	// Keep scanning until the gap is fully crossed and <after-gap> (UID 252)
	// is reached and processed. Bounded loop so a real stall fails the test
	// instead of hanging.
	for i := 0; i < 20 && !cursor.CatchupComplete; i++ {
		if err := scanInbox(c); err != nil {
			t.Fatalf("scan %d: %v", i+3, err)
		}
		cursor, _ = storage.GetTrackerCursor()
	}
	if !cursor.CatchupComplete {
		t.Fatal("catch-up never completed — the gap stalled it")
	}
	assertTrackerTestStatus(t, db, "After Gap Corp", "REJECTED")
}

func companySlug(company string) string {
	out := make([]rune, 0, len(company))
	for _, r := range company {
		if r == ' ' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// TestNewMailNotStarvedByHistoricalBacklog covers "new mail during backlog":
// a large historical catch-up backlog must not prevent freshly arriving mail
// from being seen, because the forward and catch-up ranges are independent.
func TestNewMailNotStarvedByHistoricalBacklog(t *testing.T) {
	withSmallBatch(t, 2)
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	evidenceDate := time.Now().AddDate(0, 0, -20)
	insertTrackerTestJobWithDiscovered(t, db, "Old Corp", "APPLIED", "https://example.com/old", evidenceDate)

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	outcomeDate := time.Now().AddDate(0, 0, -15)
	// 5 historical messages, all unrelated noise (keeps this test focused on
	// starvation, not matching), draining at 2/scan.
	for i := 0; i < 5; i++ {
		mbox.CreateMessage(nil, outcomeDate, rawMessage(
			"<historical-"+string(rune('a'+i))+"@example.com>",
			"noise@newsletter.com", "weekly digest", "nothing relevant"))
	}

	if err := scanInbox(c); err != nil { // bootstrap + first catch-up batch (2 of 5)
		t.Fatalf("scan 1: %v", err)
	}

	// New mail arrives after the backlog was already established.
	insertTrackerTestJob(t, db, "New Corp", "APPLIED", "https://example.com/new")
	mbox.CreateMessage(nil, time.Now(), rawMessage("<brand-new@example.com>", "hr@newcorp.com", "New Corp - update",
		"we would like to schedule an interview. what is your availability?"))

	if err := scanInbox(c); err != nil { // forward range must pick this up now
		t.Fatalf("scan 2: %v", err)
	}

	if !storage.WasEmailProcessed("<brand-new@example.com>") {
		t.Fatal("new mail was starved by an incomplete historical catch-up")
	}
	assertTrackerTestStatus(t, db, "New Corp", "INTERVIEW_REQUESTED")

	cursor, err := storage.GetTrackerCursor()
	if err != nil || cursor == nil {
		t.Fatalf("cursor missing: %v", err)
	}
	if cursor.CatchupComplete {
		t.Error("catch-up completed too early in this test setup; the starvation check is only meaningful while backlog remains")
	}
}

// TestUIDVALIDITYChangeForcesSafeResync covers "mailbox generation change":
// when UIDVALIDITY changes, the tracker must not silently trust the old
// cursor. It must resynchronize, and it must not duplicate an outcome for a
// message it already durably processed under the old generation.
func TestUIDVALIDITYChangeForcesSafeResync(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	mbox := &testMailbox{uidValidity: 100, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	// A harmless message so the mailbox isn't empty at bootstrap (an empty
	// mailbox short-circuits before the cursor is even touched).
	mbox.CreateMessage(nil, time.Now(), rawMessage("<filler@example.com>", "noise@newsletter.com", "digest", "nothing"))
	if err := scanInbox(c); err != nil {
		t.Fatalf("bootstrap scan under generation 100: %v", err)
	}

	// Now a real outcome arrives, still under generation 100, and is picked
	// up by the ordinary forward path — mirrors
	// TestSteadyStateForwardPathAndOlderMailIsUntouched's proven shape.
	// "Persistent Corp" stays open (never receives an outcome) purely so its
	// discovered_at remains live evidence at resync time below — Stable
	// Corp's own discovered_at stops counting as evidence the moment it
	// leaves the open-status set via the REJECTED write.
	insertTrackerTestJobWithDiscovered(t, db, "Persistent Corp", "APPLIED", "https://example.com/persistent", time.Now().AddDate(0, 0, -5))
	insertTrackerTestJob(t, db, "Stable Corp", "APPLIED", "https://example.com/stable")
	mbox.CreateMessage(nil, time.Now(), rawMessage("<gen1-outcome@example.com>", "hr@stablecorp.com", "Stable Corp - update",
		"unfortunately we are not moving forward with your application."))
	if err := scanInbox(c); err != nil {
		t.Fatalf("scan under generation 100: %v", err)
	}
	assertTrackerTestStatus(t, db, "Stable Corp", "REJECTED")
	if !storage.WasEmailProcessed("<gen1-outcome@example.com>") {
		t.Fatal("the message was not acknowledged under the first UID generation")
	}
	var ledgerBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerBefore); err != nil {
		t.Fatalf("count ledger: %v", err)
	}

	// Simulate a mailbox generation change: UIDVALIDITY changes underneath
	// the tracker. The same (already-acknowledged) message is still present
	// — this is the replay scenario the spec calls out, since a real
	// UIDVALIDITY bump is exactly when old UIDs (and their message content)
	// can resurface under new numbering.
	mbox.uidValidity = 200

	if err := scanInbox(c); err != nil {
		t.Fatalf("scan after UIDVALIDITY change: %v", err)
	}

	cursor, err := storage.GetTrackerCursor()
	if err != nil || cursor == nil {
		t.Fatalf("cursor missing after resync: %v", err)
	}
	if cursor.UidValidity != 200 {
		t.Errorf("cursor UidValidity = %d, want 200 — the tracker must adopt the new generation, not silently keep the old one", cursor.UidValidity)
	}

	var ledgerAfter int
	if err := db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerAfter); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if ledgerAfter != ledgerBefore {
		t.Errorf("ledger grew from %d to %d across the UIDVALIDITY resync; the same message replayed under new numbering must not duplicate its outcome", ledgerBefore, ledgerAfter)
	}
}

// TestCrashMidBatchRetriesWithoutSkipping covers "failure/restart": if a
// batch's processing fails partway through, the checkpoint must not advance
// past the failure, and a retry (after whatever broke is fixed) must not
// skip the messages that failed.
func TestCrashMidBatchRetriesWithoutSkipping(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	insertTrackerTestJob(t, db, "Corp One", "APPLIED", "https://example.com/one")
	insertTrackerTestJob(t, db, "Corp Two", "APPLIED", "https://example.com/two")

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	mbox.CreateMessage(nil, time.Now(), rawMessage("<msg1@example.com>", "hr@corpone.com", "Corp One - update",
		"unfortunately we are not moving forward with your application."))
	mbox.CreateMessage(nil, time.Now(), rawMessage("<msg2@example.com>", "hr@corptwo.com", "Corp Two - update",
		"unfortunately we are not moving forward with your application."))

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, section.FetchItem()}
	if _, err := c.Select("INBOX", false); err != nil {
		t.Fatalf("select: %v", err)
	}
	msgs, err := fetchUIDRange(c, 1, 2, items)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("fetched %d messages, want 2", len(msgs))
	}

	// Break persistence (mirrors the existing "acknowledgement error" test
	// pattern in imap_test.go) so message 2's write fails while message 1's
	// already succeeded outside this window.
	if _, err := db.Exec(`UPDATE job_funnel SET status = 'APPLIED' WHERE company_name = 'Corp One'`); err != nil {
		t.Fatalf("reseed corp one: %v", err)
	}
	highest, complete := processMessages(msgs[:1], section, []string{"Corp One", "Corp Two"})
	if highest != msgs[0].Uid || !complete {
		t.Fatalf("message 1 should have succeeded cleanly: highest = %d, complete = %v, want %d/true", highest, complete, msgs[0].Uid)
	}
	assertTrackerTestStatus(t, db, "Corp One", "REJECTED")

	if _, err := db.Exec("DROP TABLE processed_emails"); err != nil {
		t.Fatalf("simulate failure: %v", err)
	}
	highest, complete = processMessages(msgs[1:], section, []string{"Corp One", "Corp Two"})
	if highest != 0 || complete {
		t.Fatalf("message 2 should have failed to ack: highest = %d, complete = %v, want 0/false", highest, complete)
	}
	assertTrackerTestStatus(t, db, "Corp Two", "APPLIED") // untouched — rolled back

	if _, err := db.Exec(`CREATE TABLE processed_emails (message_id TEXT PRIMARY KEY, processed_at DATETIME)`); err != nil {
		t.Fatalf("repair: %v", err)
	}
	// A real restart re-fetches from IMAP rather than reusing an in-memory
	// *imap.Message — its body reader was already consumed classifying it
	// during the failed attempt above. Re-fetching is also the faithful
	// simulation of "next scan retries the same UID range".
	retry, err := fetchUIDRange(c, msgs[1].Uid, msgs[1].Uid, items)
	if err != nil || len(retry) != 1 {
		t.Fatalf("re-fetch for retry: msgs=%d err=%v", len(retry), err)
	}
	// The first message's acknowledgement was lost along with the dropped
	// table, so a real restart would legitimately retry it too — that's
	// processed_emails' own idempotency (TestUpdateDBWithTrackerResultStates
	// already covers a REJECTED->REJECTED retry landing on trackerUpdateNoop
	// rather than erroring). What matters here is message 2 is retried and
	// not skipped.
	highest, complete = processMessages(retry, section, []string{"Corp One", "Corp Two"})
	if highest != msgs[1].Uid || !complete {
		t.Fatalf("retry of message 2 should have succeeded cleanly: highest = %d, complete = %v, want %d/true", highest, complete, msgs[1].Uid)
	}
	assertTrackerTestStatus(t, db, "Corp Two", "REJECTED")
}

// TestReplayedMessageAfterCursorRollbackIsIdempotent covers "processed
// Message-ID replay": if the checkpoint rolls back (e.g. after a crash
// before it was persisted) and the same UID range is fetched again, a
// message already durably acknowledged must not be reprocessed or
// duplicated — even though it is presented to processMessages a second time.
func TestReplayedMessageAfterCursorRollbackIsIdempotent(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()
	insertTrackerTestJob(t, db, "Replay Corp", "APPLIED", "https://example.com/replay")

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)
	mbox.CreateMessage(nil, time.Now(), rawMessage("<replay@example.com>", "hr@replaycorp.com", "Replay Corp - update",
		"unfortunately we are not moving forward with your application."))

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, section.FetchItem()}
	if _, err := c.Select("INBOX", false); err != nil {
		t.Fatalf("select: %v", err)
	}
	msgs, err := fetchUIDRange(c, 1, 1, items)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("fetch: msgs=%d err=%v", len(msgs), err)
	}

	first, complete := processMessages(msgs, section, []string{"Replay Corp"})
	if first != msgs[0].Uid || !complete {
		t.Fatalf("first pass should succeed cleanly: highest = %d, complete = %v", first, complete)
	}
	assertTrackerTestStatus(t, db, "Replay Corp", "REJECTED")
	var ledgerBefore int
	db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerBefore)

	// Same UID range fetched again — the cursor "rolled back" in this
	// scenario. WasEmailProcessed dedup must make this a no-op.
	replayed, err := fetchUIDRange(c, 1, 1, items)
	if err != nil || len(replayed) != 1 {
		t.Fatalf("re-fetch: msgs=%d err=%v", len(replayed), err)
	}
	second, complete := processMessages(replayed, section, []string{"Replay Corp"})
	if second != replayed[0].Uid || !complete {
		t.Fatalf("replayed message should still count as handled for cursor purposes: highest = %d, complete = %v", second, complete)
	}

	var ledgerAfter int
	db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerAfter)
	if ledgerAfter != ledgerBefore {
		t.Errorf("ledger grew from %d to %d on a replayed message; Message-ID dedup must prevent reprocessing", ledgerBefore, ledgerAfter)
	}
}

// TestHistoricalUnmatchedAndAmbiguousOutcomesArePreserved confirms bug
// #533's guarantees (unmatched outcomes recorded exactly once, ambiguous
// matches never guessed) still hold when outcomes are discovered through the
// new UID-based catch-up path rather than the old sequence-number window.
func TestHistoricalUnmatchedAndAmbiguousOutcomesArePreserved(t *testing.T) {
	setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	db := storage.GetDB()

	evidenceDate := time.Now().AddDate(0, 0, -20)
	insertTrackerTestJobWithDiscovered(t, db, "Ambiguous Corp", "APPLIED", "https://example.com/amb1", evidenceDate)
	insertTrackerTestJobWithDiscovered(t, db, "Ambiguous Corp", "APPLIED", "https://example.com/amb2", evidenceDate)

	mbox := &testMailbox{uidValidity: 1, nextUID: 1}
	c := startTestIMAPServer(t, mbox)

	outcomeDate := time.Now().AddDate(0, 0, -15)
	mbox.CreateMessage(nil, outcomeDate, rawMessage("<unmatched-hist@example.com>", "hr@unknownco.com", "your application",
		"unfortunately we are not moving forward."))
	mbox.CreateMessage(nil, outcomeDate, rawMessage("<ambiguous-hist@example.com>", "hr@ambiguouscorp.com", "Ambiguous Corp - update",
		"unfortunately we are not moving forward."))

	if err := scanInbox(c); err != nil {
		t.Fatalf("scan: %v", err)
	}

	var unmatchedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM unmatched_outcomes WHERE message_id = ?`, "<unmatched-hist@example.com>").Scan(&unmatchedCount); err != nil {
		t.Fatalf("count unmatched: %v", err)
	}
	if unmatchedCount != 1 {
		t.Errorf("unmatched outcome recorded %d time(s), want exactly 1", unmatchedCount)
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_funnel WHERE company_name = 'Ambiguous Corp' AND status = 'APPLIED'`).Scan(&applied); err != nil {
		t.Fatalf("count applied: %v", err)
	}
	if applied != 2 {
		t.Errorf("ambiguous match changed %d row(s); both must stay APPLIED, not be guessed at", 2-applied)
	}
}
