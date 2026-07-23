// cmd/requeue reports per-source job_funnel outcomes and, on request,
// resets stale bad-status rows back to DISCOVERED so a shipped fix actually
// gets a retry. GetDiscoveredJobs only ever pulls status='DISCOVERED'
// (pkg/storage/manager.go), so a bug fix does nothing for jobs already
// marked BLOCKED_CAPTCHA/FAILED_SUBMIT until something requeues them —
// confirmed live 2026-07-23: bugs #45/#46's CAPTCHA-detection fix produced
// no new APPLIED until 830 stale rows were manually reset. This tool makes
// that step repeatable instead of one-off SQL.
//
// Usage:
//
//	go run ./cmd/requeue -stats                                   # report only, no changes
//	go run ./cmd/requeue -stats -source lever,greenhouse           # report for specific sources
//	go run ./cmd/requeue -source ashby,workable -status BLOCKED_CAPTCHA -confirm
//	go run ./cmd/requeue -source applytojob -status FAILED_SUBMIT -confirm -clear-dedup
//
// -clear-dedup also deletes matching applied_jobs rows. Needed when
// requeuing FAILED_SUBMIT (documents were already generated before that
// failure, so HasApplied would otherwise skip the retry as a duplicate —
// confirmed live 2026-07-23 re-testing the Lever "smarsh" posting) but not
// needed for BLOCKED_CAPTCHA (both known CAPTCHA checks run before document
// generation, so no applied_jobs row exists yet).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// sourcePatterns must stay in sync with the tiers documented on
// sourcePriorityCASE in pkg/storage/manager.go — re-derive both from fresh
// -stats output as new bugs are fixed rather than trusting either
// indefinitely.
var sourcePatterns = map[string]string{
	"greenhouse":      "%greenhouse%",
	"lever":           "%lever.co%",
	"ashby":           "%ashbyhq%",
	"workable":        "%workable%",
	"pinpoint":        "%pinpointhq%",
	"homerun":         "%homerun.co%",
	"smartrecruiters": "%smartrecruiters%",
	"jobvite":         "%jobvite%",
	"applytojob":      "%applytojob%",
	"recruitee":       "%recruitee%",
	"bamboohr":        "%bamboohr%",
	"workday":         "%myworkdayjobs%",
	"breezy":          "%breezy%",
}

func main() {
	statsOnly := flag.Bool("stats", false, "report per-source outcome counts and exit without changing anything")
	sourceList := flag.String("source", "", "comma-separated source names to target (see -list-sources); required unless combined with -stats and omitted (then all known sources are reported)")
	pattern := flag.String("pattern", "", "raw SQL LIKE pattern to target instead of a named -source (e.g. '%example.com%')")
	fromStatus := flag.String("status", "BLOCKED_CAPTCHA", "job_funnel status to requeue from (BLOCKED_CAPTCHA or FAILED_SUBMIT)")
	confirm := flag.Bool("confirm", false, "actually apply the requeue; without this, only a dry-run count is printed")
	clearDedup := flag.Bool("clear-dedup", false, "also delete matching applied_jobs rows (needed for FAILED_SUBMIT requeues, not BLOCKED_CAPTCHA)")
	listSources := flag.Bool("list-sources", false, "print known -source names and their patterns, then exit")
	flag.Parse()

	if *listSources {
		names := make([]string, 0, len(sourcePatterns))
		for name := range sourcePatterns {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("%-16s %s\n", name, sourcePatterns[name])
		}
		return
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()

	patterns, err := resolvePatterns(*sourceList, *pattern)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if *statsOnly {
		printStats(patterns)
		return
	}

	if len(patterns) == 0 {
		log.Fatal("no -source or -pattern given; use -stats to just report, or -list-sources to see valid names")
	}

	for name, p := range patterns {
		stat, err := storage.SourceOutcomeBreakdown(p)
		if err != nil {
			log.Printf("[%s] stats query failed: %v", name, err)
			continue
		}
		var current int
		switch *fromStatus {
		case "BLOCKED_CAPTCHA":
			current = stat.Captcha
		case "FAILED_SUBMIT":
			current = stat.Failed
		default:
			log.Fatalf("-status must be BLOCKED_CAPTCHA or FAILED_SUBMIT, got %q", *fromStatus)
		}

		if !*confirm {
			fmt.Printf("[%s] DRY RUN: would requeue %d row(s) from %s to DISCOVERED (pattern %s). Re-run with -confirm to apply.\n", name, current, *fromStatus, p)
			continue
		}

		n, err := storage.RequeueByURLPattern(p, *fromStatus)
		if err != nil {
			log.Printf("[%s] requeue failed: %v", name, err)
			continue
		}
		fmt.Printf("[%s] requeued %d row(s) from %s to DISCOVERED\n", name, n, *fromStatus)

		if *clearDedup {
			d, err := storage.ClearApplicationRecordsByURLPattern(p)
			if err != nil {
				log.Printf("[%s] clearing applied_jobs failed: %v", name, err)
				continue
			}
			fmt.Printf("[%s] cleared %d applied_jobs dedup row(s)\n", name, d)
		}
	}
}

func resolvePatterns(sourceList, rawPattern string) (map[string]string, error) {
	patterns := map[string]string{}
	if rawPattern != "" {
		patterns["custom"] = rawPattern
	}
	if sourceList != "" {
		for _, name := range strings.Split(sourceList, ",") {
			name = strings.TrimSpace(name)
			p, ok := sourcePatterns[name]
			if !ok {
				return nil, fmt.Errorf("unknown source %q; run with -list-sources to see valid names", name)
			}
			patterns[name] = p
		}
		return patterns, nil
	}
	if rawPattern != "" {
		return patterns, nil
	}
	// Neither given: report every known source (only valid for -stats).
	for name, p := range sourcePatterns {
		patterns[name] = p
	}
	return patterns, nil
}

func printStats(patterns map[string]string) {
	names := make([]string, 0, len(patterns))
	for name := range patterns {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("%-16s %6s %8s %8s %8s %8s\n", "source", "total", "applied", "captcha", "failed", "manual")
	for _, name := range names {
		stat, err := storage.SourceOutcomeBreakdown(patterns[name])
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] stats query failed: %v\n", name, err)
			continue
		}
		if stat.Total == 0 {
			continue
		}
		fmt.Printf("%-16s %6d %8d %8d %8d %8d\n", name, stat.Total, stat.Applied, stat.Captcha, stat.Failed, stat.Manual)
	}
}
