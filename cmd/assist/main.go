// Command assist opens exactly one selected Assisted Apply job in a dedicated,
// visible, proxy-guarded browser profile. It never submits or solves a human
// gate. The dashboard owns the user-facing launch action; this command owns
// only the ephemeral browser process and its database lease.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

func main() {
	jobID := flag.String("job", "", "stable assisted job identifier")
	databasePath := flag.String("db", storage.DefaultDatabasePath, "SQLite database path")
	flag.Parse()
	if *jobID == "" {
		log.Fatal("-job is required")
	}
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("secure workspace: %v", err)
	}
	if err := storage.InitDBWithPath(*databasePath); err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer storage.CloseDB()
	info, err := storage.GetAssistedLaunchInfo(storage.GetDB(), *jobID)
	if err != nil {
		log.Fatal(err)
	}
	owner, err := randomOwner()
	if err != nil {
		log.Fatal(err)
	}
	claimed, err := storage.AcquireAssistedLease(storage.GetDB(), info.JobID, owner, time.Now())
	if err != nil || !claimed {
		log.Fatalf("assisted application is already active: %v", err)
	}
	defer storage.ReleaseAssistedLease(storage.GetDB(), info.JobID, owner, time.Now())
	profileDir := filepath.Join("applications", "assisted-browser-profile")
	if err := ensurePrivateDirectory(profileDir); err != nil {
		log.Fatalf("prepare assisted browser profile: %v", err)
	}
	if err := playwright.Install(); err != nil {
		log.Fatalf("install Playwright: %v", err)
	}
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("start Playwright: %v", err)
	}
	defer pw.Stop()
	guard := security.NewNetworkGuard()
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		log.Fatal(err)
	}
	defer proxy.Close()
	browserContext, err := pw.Chromium.LaunchPersistentContext(profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false), Proxy: &playwright.Proxy{Server: proxy.URL(), Bypass: playwright.String("<-loopback>"), Username: playwright.String(proxy.Username()), Password: playwright.String(proxy.Password())},
	})
	if err != nil {
		log.Fatalf("launch visible assisted browser: %v", err)
	}
	defer browserContext.Close()
	pages := browserContext.Pages()
	var page playwright.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = browserContext.NewPage()
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := page.Route("**/*", func(route playwright.Route) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := guard.ValidateURL(ctx, route.Request().URL()); err != nil {
			_ = route.Abort("accessdenied")
			return
		}
		_ = route.Continue()
	}); err != nil {
		log.Fatalf("install browser network guard: %v", err)
	}
	if _, err := page.Goto(info.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		log.Fatalf("open assisted application: %v", err)
	}
	log.Print("Assisted application is open. Complete the stated human step, then return to the dashboard and click Continue. Closing this browser releases the assisted lease.")
	for {
		time.Sleep(time.Second)
		var state string
		err := storage.GetDB().QueryRow("SELECT assisted_state FROM assisted_applications WHERE job_id = ?", info.JobID).Scan(&state)
		if err != nil || state == "continue_requested" {
			break
		}
	}
	resume, resumeErr := storage.GetAssistedDocument(storage.GetDB(), info.JobID, "resume")
	cover, coverErr := storage.GetAssistedDocument(storage.GetDB(), info.JobID, "cover_letter")
	pii, piiErr := config.LoadPII("pii.yaml")
	if resumeErr != nil || coverErr != nil || piiErr != nil {
		log.Print("Continuation requested. Documents or PII are unavailable, so the form remains ready for your review.")
		return
	}
	if err := submitter.FillAssistedMappedPage(page, security.NewQuarantineLayer(), info.Company, info.URL, resume.Path, cover.Path, pii); err != nil {
		log.Printf("Assisted refill stopped safely: %v", err)
		return
	}
	log.Print("Known fields were refilled in the visible browser. Review the form and submit only when the employer site is ready; Career Agent will not click Submit.")
}

func randomOwner() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, security.PrivateDirMode); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("refusing unsafe profile directory")
	}
	return os.Chmod(path, security.PrivateDirMode)
}
