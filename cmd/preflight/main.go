// Command preflight inspects a selected batch of queued applications and
// records what each one is going to ask, without filling or submitting
// anything.
//
// It is spawned by the dashboard the same way cmd/assist is, and for one of the
// same reasons: everything it writes to stderr passes through the dashboard's
// SanitizeChildLogLine filter, so a driver diagnostic that quotes page content
// cannot reach dashboard.log (ADR-006). The other reason is ordinary
// robustness -- a batch of page loads that hangs or crashes takes a child
// process with it, not the dashboard.
//
// The browser here is headless and takes no assisted lease. It is nonetheless
// refused while a visible assisted browser is open: two Chromium instances plus
// local inference on one machine is how this box falls over, and the operator
// is mid-application anyway.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/knowledge"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

// maxPreflightBatch bounds one run. The number is not about capability; it is
// about how much traffic a single operator action may send to employers'
// servers. A larger batch is two runs.
const maxPreflightBatch = 25

// preflightReadySentinel is printed once the browser is up and the first
// application is being inspected, so the dashboard can distinguish "starting"
// from "started" the same way it does for cmd/assist.
const preflightReadySentinel = "Preflight is running."

func main() {
	jobList := flag.String("jobs", "", "comma-separated job identifiers to inspect")
	databasePath := flag.String("db", storage.DefaultDatabasePath, "SQLite database path")
	flag.Parse()

	jobIDs, err := parseJobIDs(*jobList)
	if err != nil {
		log.Fatalf("preflight: %v", err)
	}
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("secure workspace: %v", err)
	}
	if err := storage.InitDBWithPath(*databasePath); err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer storage.CloseDB()

	// The rejection list lives in pkg/storage; pkg/submitter is given it here
	// rather than importing storage, which would be a dependency in the wrong
	// direction for a package the agent's own fill path depends on.
	submitter.SetAssistedBrowserRejectionCheck(storage.PreflightRefusalReason)

	candidates, err := storage.PreflightCandidates(storage.GetDB(), jobIDs)
	if err != nil {
		log.Fatalf("preflight: could not read the selected applications: %v", err)
	}
	if len(candidates) == 0 {
		log.Print("Preflight had nothing to inspect.")
		return
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("preflight: browser driver unavailable: reason=%q", security.BrowserFailureReason(err))
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-infobars",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		log.Fatalf("preflight: could not launch a browser: reason=%q", security.BrowserFailureReason(err))
	}
	defer browser.Close()

	log.Print(preflightReadySentinel)
	inspected, unavailable := inspectAll(browser, candidates)

	// Everything just learned changes what the queue needs, so re-resolve it
	// now rather than making the operator refresh into a stale summary.
	if service, err := openKnowledge(); err == nil {
		if report, err := service.ReEvaluate(time.Now().UTC()); err == nil {
			log.Printf("Preflight inspected %d application(s), could not inspect %d, and re-evaluated %d question(s): %d now resolved.",
				inspected, unavailable, report.Examined, report.Resolved)
			return
		}
	}
	log.Printf("Preflight inspected %d application(s) and could not inspect %d.", inspected, unavailable)
}

func openKnowledge() (*knowledge.Service, error) {
	pii, err := config.LoadPII("pii.yaml")
	if err != nil {
		return nil, err
	}
	return knowledge.Open(storage.GetDB(), pii)
}

// inspectAll walks the batch, recording a verdict for every application --
// including the ones that could not be inspected. A missing verdict and an
// unavailable one are different facts, and the dashboard shows both.
func inspectAll(browser playwright.Browser, candidates []storage.PreflightCandidate) (inspected, unavailable int) {
	vault, vaultErr := answers.OpenStore(storage.GetDB())
	if vaultErr != nil {
		// Without the vault every discovered control is simply unresolved,
		// which is a worse inventory but still an honest one.
		log.Printf("Approved Answer Vault unavailable; recording questions without resolving them: %v", vaultErr)
	}
	pii, piiErr := config.LoadPII("pii.yaml")
	if piiErr != nil {
		log.Printf("Configured details unavailable; recording questions without resolving them: %v", piiErr)
	}

	for _, candidate := range candidates {
		now := time.Now().UTC()
		if candidate.Skip != "" {
			record(storage.PreflightResult{
				JobID: candidate.JobID, State: storage.PreflightUnavailable,
				Reason: submitter.PreflightBrowserRejected, ATS: candidate.ATS,
			}, now)
			unavailable++
			continue
		}

		inventory := submitter.InspectApplication(
			browser, security.NewQuarantineLayer(), candidate.Company, candidate.URL)
		if !inventory.Inspected() {
			// A reason code, never a message: the vocabulary is closed in
			// pkg/submitter for the same reason ADR-006 closed the browser
			// failure vocabulary.
			log.Printf("Preflight unavailable for one application: reason=%q", inventory.Reason)
			record(storage.PreflightResult{
				JobID: candidate.JobID, State: storage.PreflightUnavailable,
				Reason: inventory.Reason, ATS: inventory.ATS,
			}, now)
			unavailable++
			continue
		}

		questions := questionsFrom(candidate, inventory, vault, pii)
		if err := storage.ReplaceApplicationQuestions(storage.GetDB(), candidate.JobID, questions,
			storage.AssistedFillSummary{JobID: candidate.JobID}); err != nil {
			log.Printf("Preflight could not record one application's questions: %v", err)
		}
		record(storage.PreflightResult{
			JobID: candidate.JobID, State: storage.PreflightInspected,
			ATS: inventory.ATS, ControlCount: len(inventory.Controls),
		}, now)
		// A count of fields and a count of questions. Never a label, never a
		// value: employer-authored text belongs in the database, not the log.
		log.Printf("Preflight inspected one application: fields=%d questions=%d", len(inventory.Controls), len(questions))
		inspected++
	}
	return inspected, unavailable
}

func record(result storage.PreflightResult, now time.Time) {
	if err := storage.RecordPreflight(storage.GetDB(), result, now); err != nil {
		log.Printf("Preflight could not record one verdict: %v", err)
	}
}

// questionsFrom converts a discovered form into the inventory rows the
// Application Knowledge inbox reads.
//
// Only what Career Agent cannot fill becomes a question. A control it can
// answer from an approved answer or a configured fact is not something to put
// in front of the operator, and recording it as one would make the inbox report
// work that does not exist. Document uploads are likewise not questions -- they
// are handled by the documents path.
func questionsFrom(candidate storage.PreflightCandidate, inventory submitter.Inventory, vault *answers.Store, pii *config.PII) []storage.ApplicationQuestion {
	context := answers.Context{ATS: inventory.ATS, Company: candidate.Company}
	questions := make([]storage.ApplicationQuestion, 0, len(inventory.Controls))
	for _, control := range inventory.Controls {
		if strings.EqualFold(control.ControlType, "file") {
			continue
		}
		if submitter.IsWidgetArtifact(control) {
			continue
		}
		question := control.AsQuestion()
		question.Company = candidate.Company
		resolution := vault.Resolve(question, context, pii)
		if resolution.AutoFill {
			continue
		}
		questions = append(questions, storage.ApplicationQuestion{
			JobID:        candidate.JobID,
			Key:          control.Key,
			Prompt:       control.Label,
			ControlType:  control.ControlType,
			Options:      control.Options,
			Required:     control.Required,
			Sensitivity:  string(resolution.Sensitivity),
			Suggested:    resolution.Answer,
			Source:       string(resolution.Source),
			LabelUnsafe:  control.LabelUnsafe,
			AutoFillable: false,
		})
	}
	return questions
}

func parseJobIDs(list string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Parsed rather than trusted: these become SQL parameters and a
		// filesystem-adjacent identifier downstream.
		if _, err := strconv.ParseInt(part, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid job identifier %q", part)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("-jobs is required")
	}
	if len(out) > maxPreflightBatch {
		return nil, fmt.Errorf("preflight inspects at most %d applications per run; %d were requested", maxPreflightBatch, len(out))
	}
	return out, nil
}
