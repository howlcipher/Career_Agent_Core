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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

var (
	loadAssistedDocument       = storage.GetAssistedDocument
	loadAssistedPII            = config.LoadPII
	fillAssistedPage           = submitter.FillAssistedMappedPage
	recordAssistedRefill       = storage.RecordAssistedRefill
	recordAssistedManualReview = storage.RecordAssistedManualReview
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
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Printf("locate assisted browser cache: %v", err)
		return
	}
	profileDir, err := assistedBrowserProfileDir(cacheDir)
	if err != nil {
		log.Printf("prepare assisted browser profile: %v", err)
		return
	}
	if err := playwright.Install(); err != nil {
		log.Printf("install Playwright: %v", err)
		return
	}
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("start Playwright: %v", err)
		return
	}
	defer pw.Stop()
	guard := security.NewNetworkGuard()
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		log.Print(err)
		return
	}
	defer proxy.Close()
	browserContext, err := pw.Chromium.LaunchPersistentContext(profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false), Proxy: &playwright.Proxy{Server: proxy.URL(), Bypass: playwright.String("<-loopback>"), Username: playwright.String(proxy.Username()), Password: playwright.String(proxy.Password())},
	})
	if err != nil {
		log.Printf("launch visible assisted browser: %v", err)
		return
	}
	defer browserContext.Close()
	pages := browserContext.Pages()
	var page playwright.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = browserContext.NewPage()
		if err != nil {
			log.Print(err)
			return
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
		log.Printf("install browser network guard: %v", err)
		return
	}
	page.SetDefaultTimeout(45000)
	if _, err := page.Goto(info.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
		// Analytics and bot-protection assets can keep an otherwise usable
		// employer page busy indefinitely. DOM content is still enough to verify
		// that the visible document is the role the user selected.
		log.Printf("assisted application network-idle wait failed; checking loaded document: %v", err)
		if _, err = page.Goto(info.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
			log.Printf("open assisted application: %v", err)
			return
		}
	}
	page.WaitForTimeout(1500)
	title, err := page.Title()
	if err != nil {
		log.Printf("verify assisted application title: %v", err)
		return
	}
	if !assistedPageTitleMatchesRole(title, info.Role) {
		log.Printf("refusing assisted application page whose title %q does not match expected role %q", title, info.Role)
		return
	}
	if page.IsClosed() {
		log.Print("open assisted application: browser page closed before it became usable")
		return
	}
	log.Print("Assisted application is open. Complete the stated human step, then return to the dashboard and click Continue. Closing this browser releases the assisted lease.")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
waitForSignal:
	for {
		select {
		case <-signals:
			log.Print("Assisted application stopped; releasing its lease.")
			return
		case <-ticker.C:
			if page.IsClosed() {
				log.Print("Assisted browser closed; releasing its lease.")
				return
			}
			if err := storage.RenewAssistedLease(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
				log.Printf("Assisted browser lease expired: %v", err)
				return
			}
			var state string
			err := storage.GetDB().QueryRow("SELECT assisted_state FROM assisted_applications WHERE job_id = ?", info.JobID).Scan(&state)
			if err != nil {
				log.Printf("Assisted browser state could not be read: %v", err)
				return
			}
			if state == "completed" {
				log.Print("Assisted application was confirmed; closing the browser.")
				return
			}
			if state == "continue_requested" {
				goto continueFill
			}
		}
	}

continueFill:
	if !continueAssistedApplication(page, info, owner) {
		return
	}
	goto waitForSignal
}

// continueAssistedApplication handles the deterministic refill attempt while
// preserving the visible browser as the user's safe fallback. Returning true
// means the browser must stay open; false means the lease transition itself
// failed and the session can no longer be represented truthfully.
func continueAssistedApplication(page playwright.Page, info storage.AssistedLaunchInfo, owner string) bool {
	resume, resumeErr := loadAssistedDocument(storage.GetDB(), info.JobID, "resume")
	cover, coverErr := loadAssistedDocument(storage.GetDB(), info.JobID, "cover_letter")
	pii, piiErr := loadAssistedPII("pii.yaml")
	if resumeErr != nil || coverErr != nil || piiErr != nil {
		if err := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
			log.Printf("Continuation inputs were unavailable and manual review could not be preserved: %v", err)
			return false
		}
		log.Print("Continuation inputs are unavailable. The verified application remains open for manual completion and review.")
		return true
	}
	if err := fillAssistedPage(page, security.NewQuarantineLayer(), info.Company, info.URL, resume.Path, cover.Path, pii); err != nil {
		if stateErr := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); stateErr != nil {
			log.Printf("Assisted refill stopped and manual review could not be preserved: %v", stateErr)
			return false
		}
		log.Printf("Assisted refill stopped safely; the verified application remains open for manual completion: %v", err)
		return true
	}
	if err := recordAssistedRefill(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
		log.Printf("Assisted refill completed but could not preserve the review state: %v", err)
		return false
	}
	log.Print("Known fields were refilled in the visible browser. Review the form and submit only when the employer site is ready; Career Agent will not click Submit. The browser remains open until you confirm the employer received the application or close it.")
	return true
}

// assistedPageTitleMatchesRole makes the browser handoff fail closed when an
// ATS redirect or stale URL displays a different job. The dashboard has
// already performed the same heading-aware check; repeating it here protects
// against a page that changed between that check and the visible navigation.
func assistedPageTitleMatchesRole(title, role string) bool {
	title = normalizeAssistedTitle(title)
	role = normalizeAssistedTitle(role)
	return role != "" && strings.Contains(" "+title+" ", " "+role+" ")
}

func normalizeAssistedTitle(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}), " ")
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

// assistedBrowserProfileDir keeps Chromium's Singleton* symlinks outside the
// workspace. Those locks are normal browser internals, but a private workspace
// must reject symlinks rather than trust them during dashboard startup.
func assistedBrowserProfileDir(cacheDir string) (string, error) {
	if cacheDir == "" {
		return "", fmt.Errorf("browser cache directory is empty")
	}
	root := filepath.Join(cacheDir, "career-agent")
	if err := ensurePrivateDirectory(root); err != nil {
		return "", err
	}
	profileDir := filepath.Join(root, "assisted-browser-profile")
	if err := ensurePrivateDirectory(profileDir); err != nil {
		return "", err
	}
	return profileDir, nil
}
