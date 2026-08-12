package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

// resolveEligibilityProfile loads the profile GetAssistedQueue and
// PromoteJobToAssisted re-check every active row against. It is a variable,
// like resolveMasterCoverLetter above, so tests can supply a profile without
// writing one to disk. A load failure is treated as "no profile to enforce"
// rather than a hard error: a queue read must not fail outright just because
// profile.yaml is momentarily unreadable, and this matches the fail-safe
// GetAssistedQueue already had before this eligibility check existed.
var resolveEligibilityProfile = func() (*config.Profile, error) {
	return config.LoadProfile("profile.yaml")
}

// AssistedQueueReconciliationReport is a privacy-safe summary of one
// reconciliation pass: counts only, never a company name, title, or URL.
type AssistedQueueReconciliationReport struct {
	Examined         int
	RemovedRemote    int // failed the fully-remote hard gate
	RemovedRole      int // failed the title/role hard gate
	RemovedDuplicate int // same posting (scheme-normalized URL) as another active row
	Remaining        int
}

// ReconcileAssistedQueueEligibility re-evaluates every active assisted-apply
// row (an assisted_applications row whose job_funnel status is still one of
// eligibleAssistedStatuses -- i.e. not yet completed or already applied)
// against the current profile's role list and remote-only requirement, using
// exactly the same config.IsEligibleJob gate the discovery and automatic
// pipelines apply to brand-new postings. A row that fails either half is
// removed from the active queue: its assisted_applications row is deleted so
// it cannot render or be launched, and its job_funnel row is marked SKIPPED
// with a reason recording why, so it is preserved as an audit trail rather
// than silently disappearing.
//
// This is the single reconciliation path both GetAssistedQueue (called on
// every poll, so a persisted row can never survive a reload without passing
// eligibility again) and any explicit one-time migration call it.
//
// Only rows already in job_funnel's job_location/is_remote/job_title columns
// are available here -- unlike a freshly-scored posting, a persisted queue
// row's full description is never stored (bugs.md architecture note: storing
// full descriptions was deliberately never added). "Software Engineer" is
// therefore never removed for role mismatch: TitleEligible already treats it
// as a configured role, and the task's instruction is to keep it because a
// generic title can legitimately describe platform/DevOps/infrastructure
// work that cannot be re-verified from title alone once the description is
// gone.
//
// Already-applied jobs (job_funnel.status == APPLIED, or any row recorded in
// applied_jobs) are never touched: they are excluded by construction, since
// eligibleAssistedStatusList() never includes APPLIED.
func ReconcileAssistedQueueEligibility(conn *sql.DB, profile *config.Profile) (AssistedQueueReconciliationReport, error) {
	var report AssistedQueueReconciliationReport
	if conn == nil || profile == nil {
		return report, nil
	}

	eligible, statusArgs := eligibleAssistedStatusList()
	rows, err := conn.Query(`SELECT aa.job_id, jf.url, COALESCE(jf.job_title, ''), COALESCE(jf.job_location, ''), jf.is_remote, jf.discovered_at
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE aa.assisted_state != 'completed' AND jf.status IN (`+eligible+`)`, statusArgs...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return report, nil
		}
		return report, fmt.Errorf("load active assisted queue for reconciliation: %w", err)
	}

	type activeRow struct {
		jobID        int64
		url          string
		title        string
		location     string
		isRemote     bool
		discoveredAt time.Time
	}
	var active []activeRow
	for rows.Next() {
		var r activeRow
		var isRemote sql.NullInt64
		var discoveredAt sql.NullTime
		if err := rows.Scan(&r.jobID, &r.url, &r.title, &r.location, &isRemote, &discoveredAt); err != nil {
			rows.Close()
			return report, fmt.Errorf("scan active assisted queue row: %w", err)
		}
		r.isRemote = isRemote.Valid && isRemote.Int64 != 0
		if discoveredAt.Valid {
			r.discoveredAt = discoveredAt.Time
		}
		active = append(active, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	report.Examined = len(active)

	removeRow := func(jobID int64, statusReason string) error {
		now := time.Now().UTC()
		if _, err := conn.Exec(`UPDATE job_funnel SET status = 'SKIPPED', status_reason = ?, last_updated = ? WHERE id = ?`,
			statusReason, now, jobID); err != nil {
			return fmt.Errorf("mark ineligible job_funnel row skipped: %w", err)
		}
		if _, err := conn.Exec(`DELETE FROM assisted_applications WHERE job_id = ?`, jobID); err != nil {
			return fmt.Errorf("remove ineligible assisted queue row: %w", err)
		}
		return nil
	}

	var survivors []activeRow
	for _, r := range active {
		ok, reason := config.IsEligibleJob(config.JobEligibilityInput{
			Title:         r.title,
			Location:      r.location,
			RemoteClaimed: r.isRemote,
		}, profile)
		if ok {
			survivors = append(survivors, r)
			continue
		}
		statusReason := "pruned_ineligible_role"
		if strings.Contains(reason, "remote") {
			report.RemovedRemote++
			statusReason = "pruned_ineligible_remote"
		} else {
			report.RemovedRole++
		}
		if err := removeRow(r.jobID, statusReason); err != nil {
			log.Printf("[Storage] Failed to prune ineligible assisted queue row: %v", err)
			// The write failed, so the row is still actually active; undo
			// the counter increment and keep it in survivors to be
			// re-evaluated on the next pass rather than silently dropping it
			// from both the queue and the accounting.
			if statusReason == "pruned_ineligible_remote" {
				report.RemovedRemote--
			} else {
				report.RemovedRole--
			}
			survivors = append(survivors, r)
			continue
		}
	}

	// Deduplicate remaining active rows that refer to the same posting under
	// a scheme-variant URL (bugs.md #112 class: the same posting recorded
	// once as http:// and once as https://). AddToFunnel has normalized new
	// inserts since that fix, so this only ever catches legacy rows; the
	// most recently discovered copy is kept.
	byNormalizedURL := make(map[string]activeRow, len(survivors))
	for _, r := range survivors {
		key := NormalizeURL(r.url)
		existing, seen := byNormalizedURL[key]
		if !seen {
			byNormalizedURL[key] = r
			continue
		}
		keep, drop := existing, r
		if r.discoveredAt.After(existing.discoveredAt) {
			keep, drop = r, existing
		}
		byNormalizedURL[key] = keep
		if err := removeRow(drop.jobID, "pruned_duplicate_active_posting"); err != nil {
			log.Printf("[Storage] Failed to prune duplicate assisted queue row: %v", err)
			continue
		}
		report.RemovedDuplicate++
	}

	report.Remaining = report.Examined - report.RemovedRemote - report.RemovedRole - report.RemovedDuplicate
	return report, nil
}
