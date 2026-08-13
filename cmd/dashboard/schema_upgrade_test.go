package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	_ "modernc.org/sqlite"
)

// legacySchema is a database as an earlier release left it: job_funnel,
// applied_jobs and the original assisted_applications table, and none of the
// tables Assisted Apply gained afterwards.
//
// This is the shape that matters, and the shape the rest of this package's
// tests do not have. setupTestDB calls the Ensure* functions directly, so it
// can never observe a database that has not had them run — which is precisely
// how bugs.md #538 shipped: the dashboard opens a ReaderDSN connection, never
// calls storage.InitDBWithPath, and was therefore the one process that could
// meet a database missing these tables.
const legacySchema = `
CREATE TABLE job_funnel (
	url TEXT PRIMARY KEY, id INTEGER, company_name TEXT, job_title TEXT,
	status TEXT, status_reason TEXT, last_updated DATETIME, discovered_at DATETIME,
	applied_at DATETIME, fit_score INTEGER, job_location TEXT, is_remote INTEGER
);
CREATE TABLE applied_jobs (company_name TEXT, job_title TEXT, url TEXT UNIQUE, applied_at DATETIME);
CREATE TABLE assisted_applications (
	job_id INTEGER PRIMARY KEY, original_status TEXT NOT NULL, next_action_code TEXT NOT NULL,
	interruption_reason TEXT NOT NULL DEFAULT '', assisted_state TEXT NOT NULL DEFAULT 'waiting_human',
	is_legacy INTEGER NOT NULL DEFAULT 0, assisted_attempt_count INTEGER NOT NULL DEFAULT 0,
	lease_owner TEXT NOT NULL DEFAULT '', lease_expires_at DATETIME,
	confirmation_provenance TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
);`

// openLegacyDatabase builds that database on disk and returns a connection
// opened exactly the way the dashboard opens one.
func openLegacyDatabase(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite", storage.ReaderDSN(path))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	return conn
}

// prepareDashboardSchema is the startup sequence under test. Keeping it as one
// named list means main() and this test cannot drift onto different sets.
func prepareDashboardSchema(conn *sql.DB) error {
	for _, ensure := range []func(*sql.DB) error{
		storage.EnsureAssistedSchema,
		storage.EnsureQuestionSchema,
		storage.EnsureApplySessionSchema,
		storage.EnsureHumanInteractionSchema,
		answers.EnsureSchema,
	} {
		if err := ensure(conn); err != nil {
			return err
		}
	}
	return nil
}

func TestDashboardStartup_CreatesEveryTableAssistedApplyNeeds(t *testing.T) {
	conn := openLegacyDatabase(t)
	if err := prepareDashboardSchema(conn); err != nil {
		t.Fatalf("prepare dashboard schema: %v", err)
	}
	for _, table := range []string{
		"assisted_applications", "application_questions", "assisted_fill_summary",
		"pending_answers", "application_sessions", "application_session_items",
		"human_interactions", "approved_answers", "answer_aliases",
	} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("dashboard startup did not create %q: %v", table, err)
		}
	}
}

// The behavioural half: starting a session is the operation that actually
// failed. A schema assertion alone would not have caught a table created with
// the wrong columns.
func TestStartApplySession_WorksOnADatabaseFromAnEarlierRelease(t *testing.T) {
	previous := db
	t.Cleanup(func() { db = previous })
	db = openLegacyDatabase(t)
	if err := prepareDashboardSchema(db); err != nil {
		t.Fatalf("prepare dashboard schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, discovered_at, last_updated)
		VALUES ('https://boards.greenhouse.io/example/jobs/1', 1, 'Example', 'Engineer', 'AWAITING_REVIEW', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, created_at, updated_at)
		VALUES (1, 'AWAITING_REVIEW', 'review_and_submit', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"job_ids": []string{"1"}})
	request := httptest.NewRequest(http.MethodPost, "/api/apply-session/start", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	serveApplySessionStart(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("starting a session on an upgraded database failed: %d %q", recorder.Code, recorder.Body.String())
	}
}

// --- bugs.md #539: the origin gate must follow -addr, not a hardcoded port ---

// controlRequest posts an apply-session control through the same
// requireSameOrigin wrapper the router installs, so the test exercises the real
// gate rather than the handler in isolation.
func controlRequest(t *testing.T, origin, host string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"action": "pause", "job_id": ""})
	request := httptest.NewRequest(http.MethodPost, "/api/apply-session/control", bytes.NewReader(body))
	request.Host = host
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	recorder := httptest.NewRecorder()
	requireSameOrigin(serveApplySessionControl)(recorder, request)
	return recorder
}

func TestApplySessionControl_AcceptsTheDashboardsOwnPortWhateverItIs(t *testing.T) {
	previous := db
	t.Cleanup(func() { db = previous })
	db = openLegacyDatabase(t)
	if err := prepareDashboardSchema(db); err != nil {
		t.Fatal(err)
	}

	// A dashboard started with -addr 127.0.0.1:8099 serves pages whose Origin
	// is that address. Before #539 this was rejected outright, which broke
	// pause, resume, skip and stop on every non-default port.
	recorder := controlRequest(t, "http://127.0.0.1:8099", "127.0.0.1:8099")
	if recorder.Code == http.StatusForbidden {
		t.Fatalf("the dashboard's own origin was refused: %d %q", recorder.Code, recorder.Body.String())
	}
	// No session is open, so a Conflict is the expected outcome — what matters
	// is that the request reached the handler at all.
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected the request to reach the handler, got %d %q", recorder.Code, recorder.Body.String())
	}
}

// Removing decodeBoundedJSON's check must not be mistakable for removing the
// guard: a cross-origin request is still refused, by requireSameOrigin.
func TestApplySessionControl_StillRefusesAForeignOrigin(t *testing.T) {
	previous := db
	t.Cleanup(func() { db = previous })
	db = openLegacyDatabase(t)
	if err := prepareDashboardSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{
		"http://evil.example.com",
		"http://127.0.0.1:9999", // right host, wrong port
		"null",                  // sandboxed iframe
		"not a url at all",
	} {
		recorder := controlRequest(t, origin, "127.0.0.1:8099")
		if recorder.Code != http.StatusForbidden {
			t.Errorf("origin %q should have been refused, got %d", origin, recorder.Code)
		}
	}

	// Deliberately not asserted: "https://127.0.0.1:8099" against the same
	// host is *accepted*. hostMatchesRequest compares hosts and ignores the
	// scheme by documented design, and reaching that origin would require an
	// attacker to already be serving TLS on the operator's own loopback at the
	// dashboard's exact port — which means local code execution, at which point
	// this guard is not the relevant control. Noted here rather than tightened,
	// because narrowing a documented security decision is not a drive-by change
	// to make inside a bug fix for something else.
}
