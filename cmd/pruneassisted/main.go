// Command pruneassisted is a one-time operational tool: it re-runs the
// current role/remote/geography eligibility rules against the existing
// assisted-apply queue and prunes anything that no longer qualifies.
// GetAssistedQueue now does this automatically on every read, so this command
// exists only to produce an explicit before/after report for migrations like
// the profile.yaml role-list, remote-only, and geography hardening changes.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	profilePath := flag.String("profile", "profile.yaml", "path to profile.yaml")
	operatorSettingsPath := flag.String("operator-settings", "applications/operator_settings.yaml", "path to operator_settings.yaml")
	flag.Parse()

	if err := storage.InitDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer storage.CloseDB()

	profile, err := config.LoadProfile(*profilePath)
	if err != nil {
		log.Fatalf("failed to load profile: %v", err)
	}
	op, err := config.LoadOperatorSettings(*operatorSettingsPath)
	if err != nil {
		log.Fatalf("failed to load operator settings: %v", err)
	}
	if err := config.ApplyOperatorSettings(profile, op); err != nil {
		log.Fatalf("failed to apply operator settings: %v", err)
	}

	report, err := storage.ReconcileAssistedQueueEligibility(storage.GetDB(), profile)
	if err != nil {
		log.Fatalf("reconciliation failed: %v", err)
	}

	fmt.Printf("Examined:                     %d\n", report.Examined)
	fmt.Printf("Removed (remote):             %d\n", report.RemovedRemote)
	fmt.Printf("Removed (role):               %d\n", report.RemovedRole)
	fmt.Printf("Removed (geography):          %d\n", report.RemovedGeography)
	fmt.Printf("Held (unknown geography):     %d\n", report.HeldUnknownLocation)
	fmt.Printf("Removed (dup):                %d\n", report.RemovedDuplicate)
	fmt.Printf("Remaining:                    %d\n", report.Remaining)
}
