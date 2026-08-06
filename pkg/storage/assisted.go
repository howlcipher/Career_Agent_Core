package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AssistedNextAction is deliberately operational rather than a copy of the
// page. It never contains page text, credentials, cookies, challenge tokens,
// document paths, or user answers.
type AssistedNextAction struct {
	Code                   string `json:"code"`
	Title                  string `json:"title"`
	Instruction            string `json:"instruction"`
	PrimaryButton          string `json:"primary_button"`
	RequiresBrowser        bool   `json:"requires_browser"`
	DocumentsReady         bool   `json:"documents_ready"`
	RequiresExplicitSubmit bool   `json:"requires_explicit_submit"`
	CanContinue            bool   `json:"can_continue"`
}

// AssistedJob is the privacy-safe queue projection served to the dashboard.
// The canonical URL and local document paths remain server-side only.
type AssistedJob struct {
	ID               string             `json:"id"`
	Company          string             `json:"company"`
	Role             string             `json:"role"`
	FitScore         *int               `json:"fit_score,omitempty"`
	Provider         string             `json:"provider"`
	OriginalStatus   string             `json:"original_status"`
	Interruption     string             `json:"interruption"`
	LastUpdated      time.Time          `json:"last_updated"`
	ResumeReady      bool               `json:"resume_ready"`
	CoverLetterReady bool               `json:"cover_letter_ready"`
	MappingReady     bool               `json:"mapping_ready"`
	CompletedWork    string             `json:"completed_work"`
	Legacy           bool               `json:"legacy"`
	LiveBrowser      bool               `json:"live_browser"`
	AttemptCount     int                `json:"assisted_attempt_count"`
	PriorityReason   string             `json:"priority_reason"`
	NextAction       AssistedNextAction `json:"next_action"`
}

// AssistedMigrationOptions deliberately defaults to dry run. Statuses are
// explicit so an operator cannot accidentally broaden the one-time backfill.
type AssistedMigrationOptions struct {
	Statuses []string
	Limit    int
	JobID    string
	Confirm  bool
}

type AssistedMigrationReport struct {
	Eligible  int            `json:"eligible"`
	Imported  int            `json:"imported"`
	AlreadyIn int            `json:"already_in_queue"`
	Excluded  map[string]int `json:"excluded"`
}

// AssistedLaunchInfo is deliberately server/command-only. Its URL is never
// serialized to the dashboard; it is read from SQLite immediately before a
// guarded browser navigation.
type AssistedLaunchInfo struct {
	JobID   string
	Company string
	Role    string
	URL     string
}

// AssistedRevalidationInfo is the server-only target for a current-page
// check. It deliberately omits employer and job details from the dashboard.
type AssistedRevalidationInfo struct {
	JobID string
	URL   string
	Role  string
}

// AssistedDocument is a resolved private file, never a client-supplied path.
type AssistedDocument struct{ Path, Name string }

const AssistedLeaseDuration = 20 * time.Minute
const assistedLeaseHeartbeatTimeout = 30 * time.Second

// assistedRevalidationVersion invalidates results recorded by the original
// whole-document matcher. Job descriptions can mention another role (as the
// Meesho Forward Deployed Engineer posting did with "Platform Engineering"),
// so a role must match a page heading before the dashboard calls it ready.
const assistedRevalidationVersion = 3

// AcquireAssistedLease atomically claims the sole visible assisted browser.
// A persistent browser profile cannot safely serve concurrent employers, so
// another plan must wait until the active lease expires or is released.
func AcquireAssistedLease(conn *sql.DB, jobID, owner string, now time.Time) (bool, error) {
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(owner) == "" {
		return false, errors.New("assisted lease needs a job identifier and owner")
	}
	result, err := conn.Exec(`UPDATE assisted_applications
		SET lease_owner = ?, lease_expires_at = ?, assisted_state = 'waiting_human',
			assisted_attempt_count = assisted_attempt_count + 1, updated_at = ?
		WHERE job_id = ? AND assisted_state != 'completed'
			AND (lease_owner = '' OR lease_expires_at IS NULL OR lease_expires_at <= ? OR updated_at <= ?)
			AND NOT EXISTS (
				SELECT 1 FROM assisted_applications active
				WHERE active.job_id != ? AND active.lease_owner != '' AND active.lease_expires_at > ?
					AND active.updated_at > ?
			)`,
		owner, now.UTC().Add(AssistedLeaseDuration), now.UTC(), jobID, now.UTC(), now.UTC().Add(-assistedLeaseHeartbeatTimeout), jobID, now.UTC(), now.UTC().Add(-assistedLeaseHeartbeatTimeout))
	if err != nil {
		return false, fmt.Errorf("claim assisted application: %w", err)
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// RenewAssistedLease keeps a visible browser claim alive while its process is
// healthy. A short heartbeat window lets a crashed process be reopened soon,
// rather than leaving the dashboard stuck behind the full lease duration.
func RenewAssistedLease(conn *sql.DB, jobID, owner string, now time.Time) error {
	result, err := conn.Exec(`UPDATE assisted_applications
		SET lease_expires_at = ?, updated_at = ?
		WHERE job_id = ? AND lease_owner = ? AND assisted_state != 'completed'
			AND lease_expires_at > ?`, now.UTC().Add(AssistedLeaseDuration), now.UTC(), jobID, owner, now.UTC())
	if err != nil {
		return fmt.Errorf("renew assisted lease: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("assisted lease is no longer active")
	}
	return nil
}

func GetAssistedLaunchInfo(conn *sql.DB, jobID string) (AssistedLaunchInfo, error) {
	var info AssistedLaunchInfo
	var status, original, revalidationState string
	var revalidationVersion int
	err := conn.QueryRow(`SELECT CAST(jf.id AS TEXT), jf.company_name, COALESCE(jf.job_title, ''), jf.url, jf.status, aa.original_status, aa.revalidation_state, aa.revalidation_version
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE aa.job_id = ? AND aa.assisted_state != 'completed'
		AND NOT EXISTS (SELECT 1 FROM applied_jobs aj WHERE aj.url = jf.url)`, jobID).
		Scan(&info.JobID, &info.Company, &info.Role, &info.URL, &status, &original, &revalidationState, &revalidationVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return AssistedLaunchInfo{}, errors.New("assisted job is not available to launch")
	}
	if err != nil {
		return AssistedLaunchInfo{}, fmt.Errorf("load assisted launch plan: %w", err)
	}
	if status != original || !isAssistedEligibleStatus(status) {
		return AssistedLaunchInfo{}, fmt.Errorf("refusing to launch newer job status %q", status)
	}
	if revalidationVersion != assistedRevalidationVersion ||
		(revalidationState != "captcha_confirmed" && revalidationState != "application_ready") {
		return AssistedLaunchInfo{}, errors.New("assisted job must be revalidated before opening a browser")
	}
	return info, nil
}

// GetAssistedRevalidationInfo returns the current unfinished plan without
// treating its historic handoff as permission to launch a browser.
func GetAssistedRevalidationInfo(conn *sql.DB, jobID string) (AssistedRevalidationInfo, error) {
	var info AssistedRevalidationInfo
	var status, original string
	err := conn.QueryRow(`SELECT CAST(jf.id AS TEXT), jf.url, COALESCE(jf.job_title, ''), jf.status, aa.original_status
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE aa.job_id = ? AND aa.assisted_state != 'completed'
		AND NOT EXISTS (SELECT 1 FROM applied_jobs aj WHERE aj.url = jf.url)`, jobID).
		Scan(&info.JobID, &info.URL, &info.Role, &status, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return AssistedRevalidationInfo{}, errors.New("assisted job is not available to revalidate")
	}
	if err != nil {
		return AssistedRevalidationInfo{}, fmt.Errorf("load assisted revalidation plan: %w", err)
	}
	if status != original || !isAssistedEligibleStatus(status) {
		return AssistedRevalidationInfo{}, fmt.Errorf("refusing to revalidate newer job status %q", status)
	}
	return info, nil
}

// RecordAssistedRevalidation saves only a compact outcome class. Raw URLs,
// page text, HTTP status, and provider errors remain server-only.
func RecordAssistedRevalidation(conn *sql.DB, jobID, state string, now time.Time) error {
	switch state {
	case "captcha_confirmed", "application_ready", "unavailable", "unreachable":
	default:
		return errors.New("unsupported assisted revalidation state")
	}
	result, err := conn.Exec(`UPDATE assisted_applications
		SET revalidation_state = ?, revalidation_version = ?, revalidated_at = ?, updated_at = ?
		WHERE job_id = ? AND assisted_state != 'completed'`, state, assistedRevalidationVersion, now.UTC(), now.UTC(), jobID)
	if err != nil {
		return fmt.Errorf("record assisted revalidation: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("assisted job is no longer awaiting revalidation")
	}
	return nil
}

// MasterResumePath is the résumé every employer actually receives. cmd/agent
// returns it from both its tailored and its untailored document branch, so the
// per-job resume.md under applications/ is a saved reference document rather
// than an upload payload — when tailoring is skipped it holds only a short
// "master documents were used" note. Assisted Apply must resolve the same file
// the automatic pipeline uploads (bugs.md #515).
const MasterResumePath = "master_resume.pdf"

func GetAssistedDocument(conn *sql.DB, jobID, kind string) (AssistedDocument, error) {
	fileName := map[string]string{"resume": "resume.md", "cover_letter": "coverletter.txt"}[kind]
	if fileName == "" {
		return AssistedDocument{}, errors.New("unsupported assisted document")
	}
	var company, postingURL string
	err := conn.QueryRow(`SELECT jf.company_name, jf.url FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id WHERE aa.job_id = ?`, jobID).Scan(&company, &postingURL)
	if err != nil {
		return AssistedDocument{}, fmt.Errorf("load assisted document identity: %w", err)
	}
	// The résumé is deliberately not read from the application folder. Only
	// the cover letter is a genuine per-job artifact, matching the pair
	// cmd/agent hands the submitter.
	if kind == "resume" {
		if err := validateMasterResume(MasterResumePath); err != nil {
			return AssistedDocument{}, err
		}
		return AssistedDocument{Path: MasterResumePath, Name: MasterResumePath}, nil
	}
	path := filepath.Join(ResolveApplicationDir(company, postingURL), fileName)
	if err := validateAssistedDocument(path); err != nil {
		return AssistedDocument{}, err
	}
	return AssistedDocument{Path: path, Name: fileName}, nil
}

// validateMasterResume applies the symlink and regular-file guarantees
// validateAssistedDocument gives folder artifacts. The containment check does
// not carry over: the master résumé lives beside profile.yaml at the
// repository root, deliberately outside applications/.
func validateMasterResume(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect master resume: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing symlinked master resume")
	}
	if !info.Mode().IsRegular() {
		return errors.New("master resume is not a regular file")
	}
	if info.Size() == 0 {
		return errors.New("master resume is empty")
	}
	return nil
}

func validateAssistedDocument(path string) error {
	root, err := filepath.Abs("applications")
	if err != nil {
		return err
	}
	candidate, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("assisted document escapes private root")
	}
	current := root
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect assisted document: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("refusing symlinked assisted document")
		}
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("assisted document is not a regular file")
	}
	return nil
}

// RequestAssistedContinue is the sole dashboard transition after a human
// completes a CAPTCHA/login/field. It cannot mark an application successful
// and it retains the active browser claim for its owning process to observe.
func RequestAssistedContinue(conn *sql.DB, jobID string, now time.Time) error {
	result, err := conn.Exec(`UPDATE assisted_applications SET assisted_state = 'continue_requested', updated_at = ?
		WHERE job_id = ? AND assisted_state = 'waiting_human' AND lease_owner != '' AND lease_expires_at > ?`, now.UTC(), jobID, now.UTC())
	if err != nil {
		return fmt.Errorf("request assisted continuation: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("no active assisted browser is available to continue")
	}
	return nil
}

// RecordAssistedRefill advances a live handoff to the review stage after the
// visible browser has been refilled. The browser lease remains owned by the
// assist process so the user can review, submit manually, and confirm the
// employer's receipt before the browser is closed.
func RecordAssistedRefill(conn *sql.DB, jobID, owner string, now time.Time) error {
	result, err := conn.Exec(`UPDATE assisted_applications SET assisted_state = 'review_and_submit', updated_at = ?
		WHERE job_id = ? AND assisted_state = 'continue_requested' AND lease_owner = ? AND lease_expires_at > ?`,
		now.UTC(), jobID, owner, now.UTC())
	if err != nil {
		return fmt.Errorf("record assisted refill: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("assisted refill lease is no longer active")
	}
	return nil
}

// RecordAssistedManualReview preserves a live browser when deterministic
// refill cannot run. The user can still complete the verified employer form
// manually, but the queue must not claim that documents or fields were ready.
func RecordAssistedManualReview(conn *sql.DB, jobID, owner string, now time.Time) error {
	result, err := conn.Exec(`UPDATE assisted_applications SET assisted_state = 'manual_review', updated_at = ?
		WHERE job_id = ? AND assisted_state = 'continue_requested' AND lease_owner = ? AND lease_expires_at > ?`,
		now.UTC(), jobID, owner, now.UTC())
	if err != nil {
		return fmt.Errorf("record assisted manual review: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("assisted manual-review lease is no longer active")
	}
	return nil
}

func ReleaseAssistedLease(conn *sql.DB, jobID, owner string, now time.Time) error {
	result, err := conn.Exec(`UPDATE assisted_applications SET lease_owner = '', lease_expires_at = NULL,
		assisted_state = CASE WHEN assisted_state = 'continue_requested' THEN 'waiting_human' ELSE assisted_state END,
		updated_at = ? WHERE job_id = ? AND lease_owner = ?`, now.UTC(), jobID, owner)
	if err != nil {
		return fmt.Errorf("release assisted lease: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("assisted lease is not owned by this process")
	}
	return nil
}

var eligibleAssistedStatuses = map[string]struct{}{
	"AWAITING_REVIEW": {}, "MANUAL_REQUIRED": {}, "BLOCKED_CAPTCHA": {},
}

func migrateAssistedApplications() error {
	if db == nil {
		return errors.New("database not initialized")
	}
	return EnsureAssistedSchema(db)
}

// EnsureAssistedSchema makes the private handoff table readable by a
// dashboard connection even when it was created by an earlier release.
func EnsureAssistedSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("assisted schema connection is nil")
	}
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS assisted_applications (
		job_id INTEGER PRIMARY KEY,
		original_status TEXT NOT NULL,
		next_action_code TEXT NOT NULL,
		interruption_reason TEXT NOT NULL DEFAULT '',
		assisted_state TEXT NOT NULL DEFAULT 'waiting_human',
		is_legacy INTEGER NOT NULL DEFAULT 0,
		assisted_attempt_count INTEGER NOT NULL DEFAULT 0,
		lease_owner TEXT NOT NULL DEFAULT '',
		lease_expires_at DATETIME,
		confirmation_provenance TEXT NOT NULL DEFAULT '',
		revalidation_state TEXT NOT NULL DEFAULT 'required',
		revalidation_version INTEGER NOT NULL DEFAULT 0,
		revalidated_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(job_id) REFERENCES job_funnel(id)
	);
	CREATE INDEX IF NOT EXISTS idx_assisted_applications_state ON assisted_applications(assisted_state, lease_expires_at);`)
	if err != nil {
		return err
	}
	rows, err := conn.Query("PRAGMA table_info(assisted_applications)")
	if err != nil {
		return fmt.Errorf("inspect assisted schema: %w", err)
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan assisted schema: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"revalidation_state", "TEXT NOT NULL DEFAULT 'required'"},
		{"revalidation_version", "INTEGER NOT NULL DEFAULT 0"},
		{"revalidated_at", "DATETIME"},
	} {
		if existing[column.name] {
			continue
		}
		if _, err := conn.Exec("ALTER TABLE assisted_applications ADD COLUMN " + column.name + " " + column.definition); err != nil {
			return fmt.Errorf("add assisted schema column %s: %w", column.name, err)
		}
	}
	// #512: the prior release searched an entire document for the role, so a
	// different job could look valid when its description mentioned that role.
	// Keep the historic state for auditability but force a heading-aware check
	// before it can open a visible browser.
	if _, err := conn.Exec("UPDATE assisted_applications SET revalidation_state = 'required', revalidated_at = NULL WHERE revalidation_version < ?", assistedRevalidationVersion); err != nil {
		return fmt.Errorf("reset obsolete assisted review state: %w", err)
	}
	return nil
}

func actionForLegacy(status, reason string) AssistedNextAction {
	switch status {
	case "AWAITING_REVIEW":
		return AssistedNextAction{"review_and_submit", "Review and submit", "Review the prepared form and click Submit on the employer’s site.", "Open Prepared Application", true, false, true, false}
	case "MANUAL_REQUIRED":
		return AssistedNextAction{"login_or_create_account", "Log in or create an account", "Log in or create the required ATS account. Career Agent will not store your password.", "Open Login", true, false, false, true}
	default:
		instruction := "Solve the CAPTCHA so Career Agent can access the application form."
		if strings.Contains(strings.ToLower(reason), "submit") || strings.Contains(strings.ToLower(reason), "post") {
			instruction = "The application has been filled. Solve the CAPTCHA before submission can continue."
		}
		return AssistedNextAction{"solve_captcha", "Solve CAPTCHA", instruction, "Open CAPTCHA", true, false, false, true}
	}
}

func actionForRevalidation(status, reason, state string) AssistedNextAction {
	switch state {
	case "captcha_confirmed":
		return actionForLegacy(status, reason)
	case "application_ready":
		return AssistedNextAction{"open_verified_application", "Application ready", "The current page matches this role and shows an application entry point. Review it before providing any information or submitting.", "Open Verified Application", true, false, false, false}
	case "unavailable":
		return AssistedNextAction{"revalidate_current_page", "Current application is not ready", "The current page did not verify this role and an application entry point. No browser was opened.", "Check Current Page Again", false, false, false, false}
	case "unreachable":
		return AssistedNextAction{"revalidate_current_page", "Could not check current page", "The public employer page could not be checked. Try the safe check again later; no browser was opened.", "Check Current Page Again", false, false, false, false}
	default:
		return AssistedNextAction{"revalidate_current_page", "Check current page", "This historic handoff must be checked against the employer's current public page before it is treated as a CAPTCHA or opened in a browser.", "Check Current Page", false, false, false, false}
	}
}

func actionForReview() AssistedNextAction {
	return AssistedNextAction{
		"review_and_submit", "Review and submit",
		"Known fields were refilled in the visible browser. Review the form, complete any remaining human fields, and click Submit yourself. Then confirm that the employer received the application.",
		"Assisted Application Open", true, true, true, false,
	}
}

func actionForManualReview(liveBrowser bool) AssistedNextAction {
	instruction := "Automatic refill could not complete. Reopen the verified application, fill the required fields, and submit it yourself. Confirm only after the employer reports that it was received."
	button := "Reopen Verified Application"
	if liveBrowser {
		instruction = "Automatic refill could not complete. The verified application remains open. Fill the required fields and submit it yourself, then confirm only after the employer reports that it was received."
		button = "Assisted Application Open"
	}
	return AssistedNextAction{"manual_review", "Complete application manually", instruction, button, true, false, true, false}
}

func supportedAssistedStatuses(statuses []string) ([]string, error) {
	if len(statuses) == 0 {
		return []string{"AWAITING_REVIEW", "MANUAL_REQUIRED", "BLOCKED_CAPTCHA"}, nil
	}
	seen := map[string]struct{}{}
	for _, status := range statuses {
		status = strings.ToUpper(strings.TrimSpace(status))
		if _, ok := eligibleAssistedStatuses[status]; !ok {
			return nil, fmt.Errorf("unsupported assisted migration status %q", status)
		}
		seen[status] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for status := range seen {
		out = append(out, status)
	}
	sort.Strings(out)
	return out, nil
}

// MigrateLegacyAssisted imports only current, unfinished handoff rows. It is
// idempotent, never changes funnel status, never starts a browser, and its
// caller must pass Confirm to mutate.
func MigrateLegacyAssisted(opts AssistedMigrationOptions) (AssistedMigrationReport, error) {
	if db == nil {
		return AssistedMigrationReport{}, errors.New("database not initialized")
	}
	if err := migrateAssistedApplications(); err != nil {
		return AssistedMigrationReport{}, err
	}
	statuses, err := supportedAssistedStatuses(opts.Statuses)
	if err != nil {
		return AssistedMigrationReport{}, err
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(statuses)), ",")
	args := make([]any, 0, len(statuses)+2)
	for _, status := range statuses {
		args = append(args, status)
	}
	query := `SELECT jf.id, jf.status, COALESCE(jf.status_reason, ''), jf.fit_score,
		EXISTS(SELECT 1 FROM applied_jobs aj WHERE aj.url = jf.url),
		EXISTS(SELECT 1 FROM assisted_applications aa WHERE aa.job_id = jf.id)
		FROM job_funnel jf WHERE jf.status IN (` + placeholders + `)`
	if opts.JobID != "" {
		query += " AND jf.id = ?"
		args = append(args, opts.JobID)
	}
	query += " ORDER BY jf.last_updated DESC, jf.id DESC"
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return AssistedMigrationReport{}, fmt.Errorf("query legacy assisted jobs: %w", err)
	}
	defer rows.Close()
	type candidate struct {
		id             int
		status, reason string
	}
	var candidates []candidate
	report := AssistedMigrationReport{Excluded: map[string]int{}}
	for rows.Next() {
		var c candidate
		var applied, present bool
		var fit sql.NullInt64
		if err := rows.Scan(&c.id, &c.status, &c.reason, &fit, &applied, &present); err != nil {
			return report, err
		}
		switch {
		case c.reason == "expired":
			report.Excluded["posting_expired"]++
		case fit.Valid && fit.Int64 < 50:
			report.Excluded["below_fit_threshold"]++
		case applied:
			report.Excluded["confirmed_application"]++
		case present:
			report.AlreadyIn++
		default:
			report.Eligible++
			candidates = append(candidates, c)
		}
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	if !opts.Confirm {
		return report, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, c := range candidates {
		action := actionForLegacy(c.status, c.reason)
		result, err := tx.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason, is_legacy, created_at, updated_at)
			VALUES (?, ?, ?, ?, 1, ?, ?) ON CONFLICT(job_id) DO NOTHING`, c.id, c.status, action.Code, c.reason, now, now)
		if err != nil {
			return report, fmt.Errorf("insert assisted plan: %w", err)
		}
		n, _ := result.RowsAffected()
		report.Imported += int(n)
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

// EnsureAssistedPlanForURL creates the same privacy-safe plan for a newly
// interrupted pipeline job that the migration creates for historical work.
// Callers pass only a normalized funnel status; raw submitter errors and page
// text must never enter the plan.
func EnsureAssistedPlanForURL(rawURL, status string) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	if _, ok := eligibleAssistedStatuses[status]; !ok {
		return fmt.Errorf("status %q cannot create an assisted plan", status)
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM job_funnel WHERE url = ? AND status = ?`, NormalizeURL(rawURL), status).Scan(&id); err != nil {
		return fmt.Errorf("find interrupted job: %w", err)
	}
	action := actionForLegacy(status, "")
	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason, is_legacy, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?) ON CONFLICT(job_id) DO NOTHING`, id, status, action.Code, status, now, now)
	return err
}

// GetAssistedQueue reads a queue through an explicitly supplied connection so
// dashboard readers never need to initialize the storage package singleton.
func GetAssistedQueue(conn *sql.DB) ([]AssistedJob, error) {
	rows, err := conn.Query(`SELECT jf.id, COALESCE(jf.company_name, ''), COALESCE(jf.job_title, ''), jf.fit_score,
		CASE WHEN jf.url LIKE '%greenhouse%' THEN 'Greenhouse' WHEN jf.url LIKE '%lever.co%' THEN 'Lever'
		WHEN jf.url LIKE '%workday%' THEN 'Workday' WHEN jf.url LIKE '%ashby%' THEN 'Ashby' ELSE 'Other ATS' END,
		COALESCE(jf.status, ''), COALESCE(jf.status_reason, ''), COALESCE(jf.last_updated, jf.discovered_at, CURRENT_TIMESTAMP),
		 aa.original_status, aa.next_action_code, aa.interruption_reason, aa.is_legacy, aa.assisted_attempt_count,
		 COALESCE(aa.revalidation_state, 'required'),
		aa.assisted_state,
		aa.updated_at,
		aa.lease_owner, aa.lease_expires_at
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE aa.assisted_state != 'completed'
		ORDER BY CASE aa.next_action_code WHEN 'review_and_submit' THEN 1 WHEN 'solve_captcha' THEN 2 WHEN 'complete_legal_attestation' THEN 3 WHEN 'login_or_create_account' THEN 4 ELSE 5 END,
		jf.discovered_at DESC, jf.fit_score DESC, aa.assisted_attempt_count ASC`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []AssistedJob{}, nil
		}
		return nil, fmt.Errorf("query assisted queue: %w", err)
	}
	defer rows.Close()
	var jobs []AssistedJob
	for rows.Next() {
		var job AssistedJob
		var fit sql.NullInt64
		var lastUpdated sql.NullString
		var leaseOwner sql.NullString
		var leaseExpiry sql.NullTime
		var leaseUpdated sql.NullString
		var revalidationState string
		var assistedState string
		var currentStatus string
		if err := rows.Scan(&job.ID, &job.Company, &job.Role, &fit, &job.Provider, &currentStatus, &job.Interruption, &lastUpdated, &job.OriginalStatus, &job.NextAction.Code, &job.Interruption, &job.Legacy, &job.AttemptCount, &revalidationState, &assistedState, &leaseUpdated, &leaseOwner, &leaseExpiry); err != nil {
			return nil, err
		}
		job.LastUpdated = parseAssistedTime(lastUpdated.String)
		if fit.Valid {
			v := int(fit.Int64)
			job.FitScore = &v
		}
		now := time.Now().UTC()
		job.LiveBrowser = leaseOwner.Valid && leaseOwner.String != "" && leaseExpiry.Valid && leaseExpiry.Time.After(now) && parseAssistedTime(leaseUpdated.String).After(now.Add(-assistedLeaseHeartbeatTimeout))
		if assistedState == "review_and_submit" {
			job.NextAction = actionForReview()
		} else if assistedState == "manual_review" {
			job.NextAction = actionForManualReview(job.LiveBrowser)
		} else {
			job.NextAction = actionForRevalidation(job.OriginalStatus, job.Interruption, revalidationState)
		}
		job.CompletedWork = completedWork(job.OriginalStatus)
		job.ResumeReady = assistedDocumentExists(conn, job.ID, "resume")
		job.CoverLetterReady = assistedDocumentExists(conn, job.ID, "cover_letter")
		job.PriorityReason = priorityReason(job.NextAction.Code)
		if job.NextAction.Code == "open_verified_application" && job.LiveBrowser {
			job.NextAction.CanContinue = true
			job.NextAction.Instruction = "The verified application is open in the assisted browser. Complete the stated human step, then return here and click Continue."
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func assistedDocumentExists(conn *sql.DB, jobID, kind string) bool {
	_, err := GetAssistedDocument(conn, jobID, kind)
	return err == nil
}

// ConfirmAssistedSubmission records an explicit human confirmation only after
// the employer site displayed acceptance. Opening a browser, clicking submit,
// or clicking Continue never calls this function. The funnel and canonical
// dedup record commit together.
func ConfirmAssistedSubmission(conn *sql.DB, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return errors.New("assisted job identifier is required")
	}
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin assisted confirmation: %w", err)
	}
	defer tx.Rollback()
	var company, title, url, status, original string
	err = tx.QueryRow(`SELECT jf.company_name, jf.job_title, jf.url, jf.status, aa.original_status
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE aa.job_id = ? AND aa.assisted_state != 'completed'`, jobID).
		Scan(&company, &title, &url, &status, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("assisted job is no longer awaiting confirmation")
	}
	if err != nil {
		return fmt.Errorf("load assisted job for confirmation: %w", err)
	}
	if status != original || !isAssistedEligibleStatus(status) {
		return fmt.Errorf("refusing to overwrite newer job status %q", status)
	}
	now := time.Now().UTC()
	result, err := tx.Exec(`UPDATE assisted_applications SET assisted_state = 'completed', confirmation_provenance = 'manual_user_confirmation', updated_at = ?
		WHERE job_id = ? AND assisted_state != 'completed'`, now, jobID)
	if err != nil {
		return fmt.Errorf("record assisted confirmation: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("assisted job confirmation conflict")
	}
	if _, err := tx.Exec(`UPDATE job_funnel SET status = 'APPLIED', status_reason = NULL, applied_at = ?, last_updated = ?
		WHERE id = ? AND status = ?`, now, now, jobID, original); err != nil {
		return fmt.Errorf("mark confirmed assisted application: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO applied_jobs (company_name, job_title, url, applied_at) VALUES (?, ?, ?, ?) ON CONFLICT(url) DO NOTHING`, company, title, NormalizeURL(url), now); err != nil {
		return fmt.Errorf("record confirmed assisted application: %w", err)
	}
	return tx.Commit()
}

func isAssistedEligibleStatus(status string) bool {
	_, ok := eligibleAssistedStatuses[status]
	return ok
}

func parseAssistedTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func completedWork(status string) string {
	if status == "AWAITING_REVIEW" {
		return "Job validated; tailored documents prepared; application form reached and filled for review."
	}
	if status == "MANUAL_REQUIRED" {
		return "Job validated; fit checked; handoff preserved before the account gate."
	}
	return "Job validated; the interrupted application attempt and its current handoff state were preserved."
}
func priorityReason(action string) string {
	if action == "review_and_submit" {
		return "Quick completion: form already filled"
	}
	if action == "solve_captcha" {
		return "Quick completion: human verification is blocking progress"
	}
	return "Ready for the next human step"
}
