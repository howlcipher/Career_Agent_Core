// Command assist-migrate safely makes legacy human-handoff jobs available in
// the Assisted Apply dashboard. It does not open browsers or alter funnel
// statuses. Mutation is deliberately opt-in with -confirm.
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	databasePath := flag.String("db", storage.DefaultDatabasePath, "SQLite database path")
	statusCSV := flag.String("status", "AWAITING_REVIEW,MANUAL_REQUIRED,BLOCKED_CAPTCHA", "comma-separated eligible statuses")
	limit := flag.Int("limit", 0, "maximum rows to inspect (0 means no limit)")
	jobID := flag.String("job", "", "one stable numeric job identifier")
	confirm := flag.Bool("confirm", false, "perform the otherwise dry-run migration")
	flag.Parse()

	if *limit < 0 {
		log.Fatal("-limit must be zero or positive")
	}
	if err := security.PreparePrivateWorkspace(".", nil); err != nil {
		log.Fatalf("secure workspace: %v", err)
	}
	if err := storage.InitDBWithPath(*databasePath); err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer storage.CloseDB()

	report, err := storage.MigrateLegacyAssisted(storage.AssistedMigrationOptions{
		Statuses: strings.Split(*statusCSV, ","), Limit: *limit, JobID: *jobID, Confirm: *confirm,
	})
	if err != nil {
		log.Fatalf("migrate assisted queue: %v", err)
	}
	mode := "dry run"
	if *confirm {
		mode = "confirmed migration"
	}
	fmt.Printf("%s: eligible=%d imported=%d already_in_queue=%d exclusions=%v\n", mode, report.Eligible, report.Imported, report.AlreadyIn, report.Excluded)
}
