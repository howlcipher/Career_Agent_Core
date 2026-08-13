// Command assist opens exactly one selected Assisted Apply job in a dedicated,
// visible, proxy-guarded browser profile. It never submits or solves a human
// gate. The dashboard owns the user-facing launch action; this command owns
// only the ephemeral browser process and its database lease.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
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
	applyAssistedAnswers       = submitter.ApplyApprovedAnswers
	takePendingAnswers         = storage.TakePendingAnswers
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
		// Bug #520: this ATS refuses applications completed in the assisted
		// browser. Career Agent does not try to disguise the browser to get
		// past that; it tells the operator to finish in their own, which is
		// how such applications actually get submitted. Not a fatal error --
		// the run simply has nothing to open.
		if errors.Is(err, storage.ErrAssistedBrowserRejected) {
			log.Printf("No assisted browser was opened: %v Open this posting in your own browser and submit it there; the prepared resume and cover letter remain available from the dashboard.", err)
			return
		}
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
	// Everything this process leaves behind is cleaned up on the way out, in
	// one place, whichever of the many early returns below is taken.
	//
	// The apply-session pause is the load-bearing part. A browser that closed
	// is not an application that was submitted, and it is not an application
	// that was abandoned either — Career Agent genuinely does not know which.
	// So the session stops and asks, rather than advancing to the next employer
	// on the strength of a window disappearing.
	defer func() {
		storage.DiscardPendingAnswers(storage.GetDB(), info.JobID)
		if err := storage.PauseApplySessionForClosedBrowser(storage.GetDB(), info.JobID); err != nil {
			log.Printf("Apply session could not be paused after this browser closed: %v", err)
		}
		storage.ReleaseAssistedLease(storage.GetDB(), info.JobID, owner, time.Now())
	}()
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
		log.Printf("install Playwright: reason=%q", security.BrowserFailureReason(err))
		return
	}
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("start Playwright: reason=%q", security.BrowserFailureReason(err))
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
	if target := submitter.AssistedApplicationEntryURL(info.URL); target != "" {
		executable, commandPrefix, directProfileDir, err := assistedDirectBrowserLaunch(pw.Chromium.ExecutablePath(), profileDir)
		if err != nil {
			log.Printf("prepare direct assisted browser: %v", err)
			return
		}
		if err := runDirectAssistedBrowser(executable, commandPrefix, directProfileDir, target, proxy, info, owner); err != nil {
			log.Printf("open exact assisted destination: %v", err)
		}
		return
	}
	browserProxy := &playwright.Proxy{Server: proxy.URL(), Bypass: playwright.String("<-loopback>"), Username: playwright.String(proxy.Username()), Password: playwright.String(proxy.Password())}
	browserContext, err := pw.Chromium.LaunchPersistentContext(profileDir, assistedBrowserLaunchOptions(browserProxy))
	if err != nil {
		log.Printf("launch visible assisted browser: reason=%q", security.BrowserFailureReason(err))
		return
	}
	defer browserContext.Close()
	if err := browserContext.AddInitScript(playwright.Script{
		Content: playwright.String(`
			Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
			window.chrome = { runtime: {} };
			Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
			Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
		`),
	}); err != nil {
		log.Printf("install assisted browser compatibility script: reason=%q", security.BrowserFailureReason(err))
		return
	}
	pages := browserContext.Pages()
	var page playwright.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = browserContext.NewPage()
		if err != nil {
			log.Printf("open assisted browser page: reason=%q", security.BrowserFailureReason(err))
			return
		}
	}
	if err := installAssistedContextGuard(browserContext, guard); err != nil {
		log.Printf("install browser network guard: reason=%q", security.BrowserFailureReason(err))
		return
	}
	page.SetDefaultTimeout(45000)
	if _, err := page.Goto(info.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateNetworkidle}); err != nil {
		// Analytics and bot-protection assets can keep an otherwise usable
		// employer page busy indefinitely. DOM content is still enough to verify
		// that the visible document is the role the user selected.
		log.Printf("assisted application network-idle wait failed; checking loaded document: reason=%q", security.BrowserFailureReason(err))
		if _, err = page.Goto(info.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
			log.Printf("open assisted application: reason=%q", security.BrowserFailureReason(err))
			return
		}
	}
	page.WaitForTimeout(1500)
	title, err := page.Title()
	if err != nil {
		log.Printf("verify assisted application title: reason=%q", security.BrowserFailureReason(err))
		return
	}
	if !assistedPageTitleMatchesRole(title, info.Role) {
		log.Printf("refusing assisted application page whose title %q does not match expected role %q", title, info.Role)
		return
	}
	destinationPage, destination, err := submitter.ReachAssistedDestination(page)
	if err != nil {
		log.Printf("open exact assisted destination: reason=%q", security.BrowserFailureReason(err))
		return
	}
	if destinationPage != page {
		page = destinationPage
	}
	if page.IsClosed() {
		log.Print("open assisted application: browser page closed before it became usable")
		return
	}
	log.Printf("Assisted application is open. Verified destination: %s. Complete the stated human step, then return to the dashboard and click Continue. Closing this browser releases the assisted lease.", destination)
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
			// Skipping or stopping a session is also an outcome for this
			// application, and it has to close this browser — it holds the only
			// visible-browser lease, so leaving it open means the session can
			// never open the next application (bugs.md #542).
			if done, err := storage.AssistedWorkFinished(storage.GetDB(), info.JobID); err == nil && done {
				log.Print("This application's place in the apply session reached an outcome; closing the browser.")
				return
			}
			if state == "continue_requested" {
				goto continueFill
			}
			if state == "answers_provided" {
				goto applyAnswers
			}
		}
	}

continueFill:
	if !continueAssistedApplication(page, info, owner) {
		return
	}
	goto waitForSignal

applyAnswers:
	if !applyOperatorAnswers(page, info, owner) {
		return
	}
	goto waitForSignal
}

type directAssistedBrowser struct {
	command      *exec.Cmd
	done         chan error
	ready        chan struct{}
	readyServer  *http.Server
	extensionDir string
	profileDir   string
}

func runDirectAssistedBrowser(executable string, commandPrefix []string, profileDir, target string, proxy *security.HTTPProxy, info storage.AssistedLaunchInfo, owner string) error {
	browser, err := launchDirectAssistedBrowser(executable, commandPrefix, profileDir+"-direct", target, info.Role, proxy)
	if err != nil {
		return err
	}
	defer browser.close()

	select {
	case err := <-browser.done:
		if err == nil {
			return errors.New("direct assisted browser exited before it became usable")
		}
		return fmt.Errorf("direct assisted browser exited before it became usable: %w", err)
	case <-browser.ready:
		_ = browser.readyServer.Close()
	case <-time.After(30 * time.Second):
		return errors.New("direct assisted browser did not complete the verified destination")
	}
	log.Print("Assisted application is open. Verified destination: application. Complete the stated human step, then return to the dashboard and click Continue. Closing this browser releases the assisted lease.")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-signals:
			log.Print("Assisted application stopped; releasing its lease.")
			return nil
		case err := <-browser.done:
			log.Printf("Assisted browser closed; releasing its lease: %v", err)
			return nil
		case <-ticker.C:
			if err := storage.RenewAssistedLease(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
				return fmt.Errorf("assisted browser lease expired: %w", err)
			}
			var state string
			if err := storage.GetDB().QueryRow("SELECT assisted_state FROM assisted_applications WHERE job_id = ?", info.JobID).Scan(&state); err != nil {
				return fmt.Errorf("read assisted browser state: %w", err)
			}
			switch state {
			case "completed":
				log.Print("Assisted application was confirmed; closing the browser.")
				return nil
			case "continue_requested":
				if err := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
					return fmt.Errorf("preserve direct assisted manual review: %w", err)
				}
				log.Print("The application remains open for manual completion and review. Career Agent will not click Submit.")
			}
		}
	}
}

func launchDirectAssistedBrowser(executable string, commandPrefix []string, profileDir, target, role string, proxy *security.HTTPProxy) (*directAssistedBrowser, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, fmt.Errorf("Chromium executable is empty")
	}
	if proxy == nil {
		return nil, fmt.Errorf("browser security proxy is nil")
	}
	targetURL, err := url.Parse(target)
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") || targetURL.Hostname() == "" {
		return nil, fmt.Errorf("assisted destination URL is invalid")
	}
	profileDir, err = os.MkdirTemp(filepath.Dir(profileDir), "assisted-direct-profile-")
	if err != nil {
		return nil, fmt.Errorf("prepare direct browser profile: %w", err)
	}
	if err := os.Chmod(profileDir, security.PrivateDirMode); err != nil {
		_ = os.RemoveAll(profileDir)
		return nil, fmt.Errorf("secure direct browser profile: %w", err)
	}
	readyURL, ready, readyServer, err := startDirectBrowserReadiness()
	if err != nil {
		_ = os.RemoveAll(profileDir)
		return nil, err
	}
	extensionDir, err := writeProxyAuthenticationExtension(filepath.Dir(profileDir), proxy, target, role, readyURL)
	if err != nil {
		_ = readyServer.Close()
		_ = os.RemoveAll(profileDir)
		return nil, err
	}
	arguments := directAssistedBrowserArguments(profileDir, extensionDir, proxy.URL())
	arguments = append(append([]string{}, commandPrefix...), arguments...)
	command := exec.Command(executable, arguments...)
	// The browser's own streams are discarded rather than inherited. This
	// process's stderr is read and persisted by the dashboard, so inheriting
	// here would put a third party's unfiltered output into a log file this
	// project is answerable for (bugs.md #543). Nothing reads these streams:
	// readiness arrives over startDirectBrowserReadiness, and a launch failure
	// surfaces through Start, Wait and the readiness timeout.
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Start(); err != nil {
		_ = readyServer.Close()
		_ = os.RemoveAll(extensionDir)
		_ = os.RemoveAll(profileDir)
		return nil, fmt.Errorf("launch direct assisted browser: %w", err)
	}
	browser := &directAssistedBrowser{command: command, done: make(chan error, 1), ready: ready, readyServer: readyServer, extensionDir: extensionDir, profileDir: profileDir}
	go func() { browser.done <- command.Wait() }()
	return browser, nil
}

func startDirectBrowserReadiness() (string, chan struct{}, *http.Server, error) {
	token, err := randomOwner()
	if err != nil {
		return "", nil, nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, fmt.Errorf("start direct browser readiness listener: %w", err)
	}
	ready := make(chan struct{})
	path := "/ready/" + token
	server := &http.Server{ReadHeaderTimeout: 2 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(responseWriter, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		select {
		case <-ready:
		default:
			close(ready)
		}
		responseWriter.WriteHeader(http.StatusNoContent)
	})
	server.Handler = mux
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String() + path, ready, server, nil
}

func assistedDirectBrowserLaunch(playwrightExecutable, profileDir string) (string, []string, string, error) {
	if flatpak, err := exec.LookPath("flatpak"); err == nil {
		if commandErr := exec.Command(flatpak, "info", "com.google.Chrome").Run(); commandErr == nil {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", nil, "", homeErr
			}
			root := filepath.Join(homeDir, ".var", "app", "com.google.Chrome", "cache", "career-agent")
			if err := ensurePrivateDirectory(root); err != nil {
				return "", nil, "", err
			}
			browserRoot := filepath.Dir(filepath.Dir(playwrightExecutable))
			prefix := []string{
				"run",
				"--filesystem=" + browserRoot + ":ro",
				"--command=" + playwrightExecutable,
				"com.google.Chrome",
				// Chrome for Testing cannot create its inner user-namespace
				// sandbox from inside Flatpak. The outer Flatpak sandbox remains
				// active and grants only network, display, and the read-only
				// browser bundle needed for this process.
				"--no-sandbox",
			}
			return flatpak, prefix, filepath.Join(root, "assisted-browser-profile-direct"), nil
		}
	}
	for _, name := range []string{"chromium", "chromium-browser"} {
		if candidate, err := exec.LookPath(name); err == nil {
			return candidate, nil, profileDir, nil
		}
	}
	return playwrightExecutable, nil, profileDir, nil
}

func directAssistedBrowserArguments(profileDir, extensionDir, proxyURL string) []string {
	return []string{
		"--user-data-dir=" + profileDir,
		"--proxy-server=" + proxyURL,
		"--proxy-bypass-list=127.0.0.1;localhost",
		"--disable-quic",
		"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--disable-extensions-except=" + extensionDir,
		"--load-extension=" + extensionDir,
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	}
}

func writeProxyAuthenticationExtension(parentDir string, proxy *security.HTTPProxy, target, role, readyURL string) (string, error) {
	proxyURL, err := url.Parse(proxy.URL())
	if err != nil || proxyURL.Hostname() == "" {
		return "", fmt.Errorf("browser security proxy URL is invalid")
	}
	port, err := strconv.Atoi(proxyURL.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("browser security proxy port is invalid")
	}
	extensionDir, err := os.MkdirTemp(parentDir, "assisted-proxy-auth-")
	if err != nil {
		return "", fmt.Errorf("create private proxy extension: %w", err)
	}
	if err := os.Chmod(extensionDir, security.PrivateDirMode); err != nil {
		_ = os.RemoveAll(extensionDir)
		return "", err
	}
	manifest := map[string]any{
		"manifest_version": 3,
		"name":             "Career Agent guarded proxy",
		"version":          "1.0.0",
		"permissions":      []string{"proxy", "tabs", "webNavigation", "webRequest", "webRequestAuthProvider"},
		"host_permissions": []string{"<all_urls>"},
		"background":       map[string]string{"service_worker": "background.js"},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		_ = os.RemoveAll(extensionDir)
		return "", err
	}
	hostJSON, _ := json.Marshal(proxyURL.Hostname())
	usernameJSON, _ := json.Marshal(proxy.Username())
	passwordJSON, _ := json.Marshal(proxy.Password())
	targetJSON, _ := json.Marshal(target)
	roleJSON, _ := json.Marshal(normalizeAssistedTitle(role))
	readyURLJSON, _ := json.Marshal(readyURL)
	background := fmt.Sprintf(`const config = {mode: 'fixed_servers', rules: {singleProxy: {scheme: 'http', host: %s, port: %d}, bypassList: ['127.0.0.1', 'localhost']}};
chrome.webRequest.onAuthRequired.addListener(
  details => details.isProxy ? {authCredentials: {username: %s, password: %s}} : {},
  {urls: ['<all_urls>']},
  ['blocking']
);
chrome.proxy.settings.set({value: config, scope: 'regular'}, () => {
  chrome.tabs.query({active: true, currentWindow: true}, tabs => {
    if (tabs.length > 0) chrome.tabs.update(tabs[0].id, {url: %s});
  });
});
chrome.webNavigation.onCompleted.addListener(details => {
  if (details.frameId !== 0 || !details.url.startsWith(%s)) return;
  chrome.tabs.get(details.tabId, tab => {
    const title = (tab.title || '').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim();
    if ((' ' + title + ' ').includes(' ' + %s + ' ')) fetch(%s);
  });
});
`, hostJSON, port, usernameJSON, passwordJSON, targetJSON, targetJSON, roleJSON, readyURLJSON)
	for name, contents := range map[string][]byte{"manifest.json": manifestBytes, "background.js": []byte(background)} {
		if err := os.WriteFile(filepath.Join(extensionDir, name), contents, security.PrivateFileMode); err != nil {
			_ = os.RemoveAll(extensionDir)
			return "", fmt.Errorf("write private proxy extension: %w", err)
		}
	}
	return extensionDir, nil
}

func (browser *directAssistedBrowser) close() {
	if browser == nil {
		return
	}
	if browser.command != nil && browser.command.Process != nil {
		_ = browser.command.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-browser.done:
	case <-time.After(2 * time.Second):
		if browser.command != nil && browser.command.Process != nil {
			_ = browser.command.Process.Kill()
		}
		select {
		case <-browser.done:
		case <-time.After(2 * time.Second):
		}
	}
	if browser.readyServer != nil {
		_ = browser.readyServer.Close()
	}
	if browser.extensionDir != "" {
		_ = os.RemoveAll(browser.extensionDir)
	}
	if browser.profileDir != "" {
		_ = os.RemoveAll(browser.profileDir)
	}
}

func assistedBrowserLaunchOptions(proxy *playwright.Proxy) playwright.BrowserTypeLaunchPersistentContextOptions {
	return playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:          playwright.Bool(false),
		Proxy:             proxy,
		Args:              []string{"--disable-blink-features=AutomationControlled"},
		IgnoreDefaultArgs: []string{"--enable-automation"},
	}
}

// assistedRoute is the part of playwright.Route the network guard callback
// uses. Naming it lets the guard decision be exercised without a browser.
type assistedRoute interface {
	Abort(errorCode ...string) error
	Continue(options ...playwright.RouteContinueOptions) error
}

// unknownAssistedHost stands in for a host that could not be read safely. The
// raw value is never substituted: an unparseable request URL is exactly the
// case where its contents are least trustworthy.
const unknownAssistedHost = "unknown"

// maxAssistedHostLength is the longest legal DNS name. Anything longer is a
// malformed or hostile value, not a host worth naming in the log.
const maxAssistedHostLength = 253

// safeAssistedHost reduces a request URL to the one component that can be
// logged: its hostname. Everything else this path sees -- userinfo, the
// employer's application path, query parameters, fragments -- is either a
// credential, a challenge token, or an answer the operator typed, and must
// never reach the log (bug #523).
func safeAssistedHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return unknownAssistedHost
	}
	host := parsed.Hostname()
	if host == "" || len(host) > maxAssistedHostLength {
		return unknownAssistedHost
	}
	// A hostname is chosen by whatever page issued the request, so bound its
	// alphabet rather than trusting url.Parse to have rejected every value that
	// could break a log line into two.
	for _, character := range host {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '.', character == '-', character == ':':
		default:
			return unknownAssistedHost
		}
	}
	return host
}

// guardAssistedRequest applies the network guard to one intercepted request and
// makes a rejection visible without disclosing the request. It records at most
// one rejection line per blocked request, plus a second line only if the abort
// itself failed.
func guardAssistedRequest(
	ctx context.Context,
	guard *security.NetworkGuard,
	rawURL string,
	route assistedRoute,
) {
	if err := guard.ValidateURL(ctx, rawURL); err != nil {
		host := safeAssistedHost(rawURL)
		log.Printf(
			"Assisted network guard blocked request: host=%q reason=%q",
			host,
			security.NetworkRejectionReason(err),
		)
		// Playwright's abort error can quote the request it refers to, so it is
		// reported by fact rather than by message.
		if abortErr := route.Abort("accessdenied"); abortErr != nil {
			log.Printf(
				"Assisted network guard could not abort the blocked request: host=%q",
				host,
			)
		}
		return
	}
	_ = route.Continue()
}

func installAssistedContextGuard(browserContext playwright.BrowserContext, guard *security.NetworkGuard) error {
	return browserContext.Route("**/*", func(route playwright.Route) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		guardAssistedRequest(ctx, guard, route.Request().URL(), route)
	})
}

// continueAssistedApplication handles the deterministic refill attempt while
// preserving the visible browser as the user's safe fallback. Returning true
// means the browser must stay open; false means the lease transition itself
// failed and the session can no longer be represented truthfully.
//
// It now ends in one of three places rather than one. A refill that leaves
// nothing unanswered goes straight to review, exactly as before. A refill that
// leaves questions goes to needs_answers, so the operator is shown the short
// list rather than being handed the whole form to re-read. A refill that fails
// still preserves the open browser for manual completion.
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
	vault, vaultErr := answers.OpenStore(storage.GetDB())
	if vaultErr != nil {
		// The vault being unavailable degrades the fill to what it always did
		// before the vault existed. It is not a reason to refuse the
		// application.
		log.Printf("Approved Answer Vault unavailable; continuing without reusable answers: %v", vaultErr)
	}
	report, err := fillAssistedPage(submitter.AssistedFillPlan{
		Page:        page,
		Filter:      security.NewQuarantineLayer(),
		Vault:       vault,
		CompanyName: info.Company,
		ApplyURL:    info.URL,
		ResumePath:  resume.Path,
		CoverPath:   cover.Path,
		PII:         pii,
		ATSName:     storage.SupportedAssistedATS(info.URL),
	})
	if err != nil {
		if stateErr := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); stateErr != nil {
			log.Printf("Assisted refill stopped and manual review could not be preserved: %v", stateErr)
			return false
		}
		log.Printf("Assisted refill stopped safely; the verified application remains open for manual completion: reason=%q", security.BrowserFailureReason(err))
		return true
	}

	summary := fillSummary(info.JobID, report)
	if questions := questionsFromReport(info.JobID, report); len(questions) > 0 {
		if err := storage.RecordAssistedQuestions(storage.GetDB(), info.JobID, owner, questions, summary, time.Now()); err != nil {
			log.Printf("Assisted refill completed but its question list could not be recorded: %v", err)
			return false
		}
		log.Printf("Filled %d field(s) and reused %d approved answer(s). %d question(s) need you; answer them in the dashboard and Career Agent will finish filling the form.",
			report.FilledCount(), report.ReusedAnswers, len(questions))
		return true
	}
	if err := storage.ReplaceApplicationQuestions(storage.GetDB(), info.JobID, nil, summary); err != nil {
		log.Printf("Assisted refill completed but its summary could not be recorded: %v", err)
	}
	if err := recordAssistedRefill(storage.GetDB(), info.JobID, owner, time.Now()); err != nil {
		log.Printf("Assisted refill completed but could not preserve the review state: %v", err)
		return false
	}
	log.Printf("Filled %d field(s) and reused %d approved answer(s) with nothing left unanswered. Review the form and submit only when the employer site is ready; Career Agent will not click Submit.",
		report.FilledCount(), report.ReusedAnswers)
	return true
}

// applyOperatorAnswers types the answers the operator just gave into the open
// application, then advances to review.
//
// It never clicks Submit, and it cannot: ApplyApprovedAnswers has no access to
// a submit control. A failure here keeps the browser open for manual
// completion, which is the same fallback every other assisted failure uses.
func applyOperatorAnswers(page playwright.Page, info storage.AssistedLaunchInfo, owner string) bool {
	values, err := takePendingAnswers(storage.GetDB(), info.JobID)
	if err != nil || len(values) == 0 {
		if stateErr := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); stateErr != nil {
			log.Printf("Operator answers were unavailable and manual review could not be preserved: %v", stateErr)
			return false
		}
		log.Printf("No operator answers were waiting for this application; it remains open for manual completion: %v", err)
		return true
	}
	report, err := applyAssistedAnswers(page, security.NewQuarantineLayer(), values)
	if err != nil {
		if stateErr := recordAssistedManualReview(storage.GetDB(), info.JobID, owner, time.Now()); stateErr != nil {
			log.Printf("Answers could not be applied and manual review could not be preserved: %v", stateErr)
			return false
		}
		log.Printf("Your answers could not be entered automatically; the application remains open so you can complete it yourself: reason=%q", security.BrowserFailureReason(err))
		return true
	}
	if len(report.Unresolved) > 0 {
		log.Printf("%d of your answers could not be entered automatically; enter them yourself before submitting.", len(report.Unresolved))
	}
	summary := fillSummary(info.JobID, report)
	if err := storage.RecordAssistedAnswersApplied(storage.GetDB(), info.JobID, owner, summary, time.Now()); err != nil {
		log.Printf("Answers were entered but the review state could not be preserved: %v", err)
		return false
	}
	log.Printf("Entered %d of your answers. Review the form and submit it yourself; Career Agent will not click Submit.", len(report.Filled))
	return true
}

// fillSummary converts a fill report into the storable summary. Labels are
// kept, values are not: the summary exists so the operator can see what was
// done for them, not so this project can keep a copy of their application.
func fillSummary(jobID string, report submitter.FillReport) storage.AssistedFillSummary {
	labels := make([]string, 0, len(report.Filled))
	for _, field := range report.Filled {
		labels = append(labels, field.Label)
	}
	return storage.AssistedFillSummary{
		JobID:         jobID,
		FilledCount:   report.FilledCount(),
		ReusedAnswers: report.ReusedAnswers,
		Documents:     report.Documents,
		FilledLabels:  labels,
	}
}

func questionsFromReport(jobID string, report submitter.FillReport) []storage.ApplicationQuestion {
	var out []storage.ApplicationQuestion
	for _, question := range report.Unresolved {
		out = append(out, storage.ApplicationQuestion{
			JobID:       jobID,
			Key:         question.Key,
			Prompt:      question.Label,
			ControlType: question.ControlType,
			Options:     question.Options,
			Required:    question.Required,
			Sensitivity: question.Sensitivity,
			Suggested:   question.Suggested,
			Source:      question.Source,
			LabelUnsafe: question.LabelUnsafe,
		})
	}
	return out
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
