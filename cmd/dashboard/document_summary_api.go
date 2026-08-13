package main

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// documentSummary describes one prepared document without making the operator
// read it.
type documentSummary struct {
	Kind     string   `json:"kind"`
	Ready    bool     `json:"ready"`
	Headline string   `json:"headline"`
	Changes  []string `json:"changes"`
	Note     string   `json:"note,omitempty"`
}

// serveAssistedDocumentSummary reports what is in this application's documents
// and how they differ from the master versions.
//
// The résumé and the cover letter get genuinely different treatment, because
// the truth about them is different. Assisted Apply always attaches the master
// résumé (storage.MasterResumePath, bugs.md #515) — there is no per-job résumé,
// so there is nothing to diff, and a "3 bullets adjusted" summary would be an
// invention. It says so instead. The cover letter is a real per-job artifact
// whenever the master letter is disabled, so that one gets an actual change
// summary against the master.
func serveAssistedDocumentSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if _, err := strconv.ParseInt(jobID, 10, 64); err != nil {
		http.Error(w, "invalid assisted job identifier", http.StatusBadRequest)
		return
	}

	summaries := []documentSummary{resumeSummary(jobID), coverLetterSummary(jobID)}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"documents": summaries})
}

func resumeSummary(jobID string) documentSummary {
	summary := documentSummary{Kind: "resume"}
	document, err := storage.GetAssistedDocument(db, jobID, "resume")
	if err != nil {
		summary.Headline = "Not prepared yet"
		summary.Note = "Career Agent will attach your master résumé once it is available."
		return summary
	}
	summary.Ready = true
	summary.Headline = "Master résumé, unmodified"
	summary.Changes = []string{
		"This is the same file the automatic pipeline uploads.",
		"No per-job résumé is generated, so nothing has been rewritten, and no bullet, metric, or claim has been added.",
	}
	if info, err := os.Stat(document.Path); err == nil {
		summary.Note = "Last updated " + info.ModTime().Format("2 Jan 2006")
	}
	return summary
}

func coverLetterSummary(jobID string) documentSummary {
	summary := documentSummary{Kind: "cover_letter"}
	document, err := storage.GetAssistedDocument(db, jobID, "cover_letter")
	if err != nil {
		summary.Headline = "Not prepared yet"
		return summary
	}
	summary.Ready = true
	generated, err := os.ReadFile(document.Path)
	if err != nil {
		summary.Headline = "Prepared"
		return summary
	}
	master, masterErr := os.ReadFile(masterCoverLetterPath())
	if masterErr != nil || strings.TrimSpace(string(master)) == "" {
		summary.Headline = "Master cover letter, unmodified"
		summary.Changes = []string{"The same letter is sent for every application."}
		return summary
	}
	if strings.TrimSpace(string(master)) == strings.TrimSpace(string(generated)) {
		summary.Headline = "Master cover letter, unmodified"
		summary.Changes = []string{"This application uses your master letter with no changes."}
		return summary
	}
	summary.Headline = "Tailored for this application"
	summary.Changes = describeLetterChanges(string(master), string(generated))
	return summary
}

// masterCoverLetterPath resolves the configured master letter, or the
// conventional path when no profile is readable.
func masterCoverLetterPath() string {
	profile, err := loadDashboardProfile()
	if err != nil || profile == nil {
		return "master_cover_letter.txt"
	}
	if path := profile.ResolvedMasterCoverLetterPath(); path != "" {
		return path
	}
	return "master_cover_letter.txt"
}

// numericToken finds anything that could be a claimed figure: years, counts,
// percentages, amounts.
var numericToken = regexp.MustCompile(`\d[\d,.]*%?`)

// describeLetterChanges reports meaningful differences rather than a raw diff.
//
// The last check is the important one and is the reason this is not just a line
// diff. A tailored letter that contains a number the master did not is exactly
// how an invented metric reaches an employer, so any new figure is surfaced to
// the operator by name instead of being buried in a paragraph they were told
// was "adjusted".
func describeLetterChanges(master, generated string) []string {
	masterParagraphs := splitParagraphs(master)
	generatedParagraphs := splitParagraphs(generated)

	known := make(map[string]bool, len(masterParagraphs))
	for _, paragraph := range masterParagraphs {
		known[paragraph] = true
	}
	added := 0
	for _, paragraph := range generatedParagraphs {
		if !known[paragraph] {
			added++
		}
	}

	var changes []string
	switch {
	case added == 0:
		changes = append(changes, "Wording adjusted; no new paragraphs.")
	case added == 1:
		changes = append(changes, "1 paragraph written for this application.")
	default:
		changes = append(changes, strconv.Itoa(added)+" paragraphs written for this application.")
	}
	if len(generatedParagraphs) < len(masterParagraphs) {
		changes = append(changes, strconv.Itoa(len(masterParagraphs)-len(generatedParagraphs))+" paragraph(s) from the master letter were dropped.")
	}

	newFigures := introducedFigures(master, generated)
	if len(newFigures) == 0 {
		changes = append(changes, "No figures appear that are not already in your master letter.")
	} else {
		changes = append(changes, "Check these figures — they do not appear in your master letter: "+strings.Join(newFigures, ", "))
	}
	return changes
}

func introducedFigures(master, generated string) []string {
	inMaster := map[string]bool{}
	for _, token := range numericToken.FindAllString(master, -1) {
		inMaster[token] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, token := range numericToken.FindAllString(generated, -1) {
		if inMaster[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	sort.Strings(out)
	const maxReportedFigures = 8
	if len(out) > maxReportedFigures {
		out = out[:maxReportedFigures]
	}
	return out
}

func splitParagraphs(text string) []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		block = strings.Join(strings.Fields(block), " ")
		if block != "" {
			out = append(out, block)
		}
	}
	return out
}

// loadDashboardProfile is indirected so a test can run without a profile.yaml
// on disk, and so a summary test can supply a specific master letter path.
var loadDashboardProfile = func() (*config.Profile, error) {
	return config.LoadProfile("profile.yaml")
}
