package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DuplicateApplicationExists reports whether a confirmed application within
// cooldown has the same conservative identity as the proposed job. Missing
// location metadata is deliberately not a match: locations can make otherwise
// identical titles distinct opportunities.
func DuplicateApplicationExists(company, title, location string, remote bool, cooldown time.Duration) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if cooldown <= 0 {
		return false, nil
	}

	want := applicationIdentityFrom(company, title, location, remote)
	if !want.complete() {
		return false, nil
	}

	rows, err := db.Query(`SELECT company_name, job_title, job_location, is_remote
		FROM job_funnel
		WHERE status = 'APPLIED' AND applied_at >= ?`, time.Now().UTC().Add(-cooldown))
	if err != nil {
		return false, fmt.Errorf("query recent confirmed applications: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var candidateCompany, candidateTitle, candidateLocation sql.NullString
		var candidateRemote sql.NullBool
		if err := rows.Scan(&candidateCompany, &candidateTitle, &candidateLocation, &candidateRemote); err != nil {
			return false, fmt.Errorf("scan recent confirmed application: %w", err)
		}
		if !candidateRemote.Valid {
			continue
		}
		candidate := applicationIdentityFrom(candidateCompany.String, candidateTitle.String, candidateLocation.String, candidateRemote.Bool)
		if candidate.complete() && candidate == want {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate recent confirmed applications: %w", err)
	}
	return false, nil
}

// UpdateFunnelIdentity retains the metadata the duplicate matcher needs after
// discovery. It updates only the row's own identity fields, never its status.
func UpdateFunnelIdentity(rawURL, location string, remote bool) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec(`UPDATE job_funnel SET job_location = NULLIF(?, ''), is_remote = ? WHERE url = ?`,
		strings.TrimSpace(location), remote, NormalizeURL(rawURL))
	return err
}

type applicationIdentity struct {
	company   string
	role      string
	seniority string
	location  string
	remote    bool
}

func (i applicationIdentity) complete() bool {
	return i.company != "" && i.role != "" && i.location != ""
}

func applicationIdentityFrom(company, title, location string, remote bool) applicationIdentity {
	role, seniority := normalizedRole(title)
	return applicationIdentity{
		company:   normalizedCompany(company),
		role:      role,
		seniority: seniority,
		location:  normalizedWords(location),
		remote:    remote,
	}
}

func normalizedCompany(value string) string {
	words := strings.Fields(normalizedWords(value))
	for len(words) > 0 {
		switch words[len(words)-1] {
		case "inc", "incorporated", "llc", "ltd", "limited", "corp", "corporation", "co", "company":
			words = words[:len(words)-1]
		default:
			return strings.Join(words, " ")
		}
	}
	return ""
}

func normalizedRole(value string) (string, string) {
	var role, seniority []string
	for _, word := range strings.Fields(normalizedWords(value)) {
		switch word {
		case "intern", "junior", "jr", "senior", "sr", "staff", "principal", "lead", "manager", "director", "vp", "vice", "president", "i", "ii", "iii", "iv":
			seniority = append(seniority, word)
		default:
			role = append(role, word)
		}
	}
	return strings.Join(role, " "), strings.Join(seniority, " ")
}

func normalizedWords(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
