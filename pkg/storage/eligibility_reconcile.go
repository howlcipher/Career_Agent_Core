package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

// resolveEligibilityProfile loads the profile GetAssistedQueue,
// GetAssistedLaunchInfo and PromoteJobToAssisted re-check every active row
// against. It is a variable, like resolveMasterCoverLetter above, so tests can
// supply a profile without writing one to disk.
//
// It resolves the *effective* profile -- profile.yaml with
// applications/operator_settings.yaml applied over it -- so the operator's
// dashboard geography selector is authoritative on every one of those paths
// and cannot disagree with what discovery enforces (bugs.md #554).
//
// A load failure used to be treated as "no profile to enforce", and every
// caller quietly proceeded. That is how #554 stayed invisible: commit ebe0863
// deleted profile.yaml on 2026-08-12 and the country allowlist, the
// remote-only gate and the role gate all switched off at once, silently, with
// a Jordan posting left one click from an Assisted Apply browser. A missing
// policy is now a reason to refuse, never a reason to allow.
var resolveEligibilityProfile = func() (*config.Profile, error) {
	profile, err := config.LoadProfile(profileConfigPath)
	if err != nil {
		return nil, fmt.Errorf("eligibility policy unavailable: %w", err)
	}
	// Deliberately not config.GetEffectiveSettings: that helper substitutes an
	// empty Profile when profile.yaml is missing, which is precisely the
	// fail-open this fix exists to remove. Load it strictly, then layer the
	// operator's settings over it by hand.
	op, err := config.LoadOperatorSettings(operatorSettingsPath)
	if err != nil {
		return nil, fmt.Errorf("eligibility policy unavailable: %w", err)
	}
	if err := config.ApplyOperatorSettings(profile, op); err != nil {
		return nil, fmt.Errorf("eligibility policy unavailable: %w", err)
	}
	return profile, nil
}

// profileConfigPath and operatorSettingsPath are where the operator's policy
// lives. Relative for the same reason every other path in this process is:
// every binary runs from the repo root.
const (
	profileConfigPath    = "profile.yaml"
	operatorSettingsPath = "applications/operator_settings.yaml"
)

// AssistedQueueReconciliationReport is a privacy-safe summary of one
// reconciliation pass: counts only, never a company name, title, or URL.
type AssistedQueueReconciliationReport struct {
	Examined            int
	RemovedRemote       int // failed the fully-remote hard gate
	RemovedRole         int // failed the title/role hard gate on a plain role mismatch
	RemovedManagement   int // title's primary track is management/leadership, not IC engineering
	RemovedSeniority    int // Staff/Principal stretch match rejected because stretch is disabled
	RemovedGeography    int // named only countries outside the configured allowlist
	HeldUnknownLocation int // otherwise eligible, but no country evidence to screen
	RemovedDuplicate    int // same posting (scheme-normalized URL) as another active row
	Remaining           int
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
		verdict := config.ScreenJob(config.JobEligibilityInput{
			Title:         r.title,
			Location:      r.location,
			RemoteClaimed: r.isRemote,
		}, profile)
		if verdict.Eligible {
			survivors = append(survivors, r)
			continue
		}
		// The reason code comes back structured rather than being recovered
		// from the prose with strings.Contains, which silently filed every
		// unrecognised reason as a role mismatch (bugs.md #554).
		counter := &report.RemovedRole
		switch verdict.Code {
		case config.ReasonIneligibleRemote:
			counter = &report.RemovedRemote
		case config.ReasonOutsideAllowedCountries:
			counter = &report.RemovedGeography
		case config.ReasonLocationUnknown:
			counter = &report.HeldUnknownLocation
		case config.ReasonManagementTrackExcluded:
			counter = &report.RemovedManagement
		case config.ReasonSeniorityOutsideTarget:
			counter = &report.RemovedSeniority
		}
		*counter++
		if err := removeRow(r.jobID, verdict.Code); err != nil {
			log.Printf("[Storage] Failed to prune ineligible assisted queue row: %v", err)
			// The write failed, so the row is still actually active; undo
			// the counter increment and keep it in survivors to be
			// re-evaluated on the next pass rather than silently dropping it
			// from both the queue and the accounting.
			*counter--
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

	report.Remaining = report.Examined - report.RemovedRemote - report.RemovedRole - report.RemovedManagement - report.RemovedSeniority - report.RemovedGeography - report.HeldUnknownLocation - report.RemovedDuplicate
	return report, nil
}
