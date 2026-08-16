package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

func TestAssistedBrowserProfileDirUsesPrivateCacheSubdirectory(t *testing.T) {
	cacheDir := t.TempDir()
	profileDir, err := assistedBrowserProfileDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cacheDir, "career-agent", "assisted-browser-profile")
	if profileDir != want {
		t.Fatalf("profile directory = %q, want %q", profileDir, want)
	}
	for _, path := range []string{filepath.Join(cacheDir, "career-agent"), profileDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != security.PrivateDirMode {
			t.Fatalf("mode for %q = %o, want %o", path, info.Mode().Perm(), security.PrivateDirMode)
		}
	}
}

func TestContinueAssistedApplicationKeepsBrowserOpenForManualReview(t *testing.T) {
	oldLoadDocument := loadAssistedDocument
	oldLoadPII := loadAssistedPII
	oldFillPage := fillAssistedPage
	oldRecordRefill := recordAssistedRefill
	oldRecordManualReview := recordAssistedManualReview
	t.Cleanup(func() {
		loadAssistedDocument = oldLoadDocument
		loadAssistedPII = oldLoadPII
		fillAssistedPage = oldFillPage
		recordAssistedRefill = oldRecordRefill
		recordAssistedManualReview = oldRecordManualReview
	})

	for _, tc := range []struct {
		name     string
		docErr   error
		fillErr  error
		wantFill bool
	}{
		{name: "documents unavailable", docErr: errors.New("missing document")},
		{name: "refill rejected", fillErr: errors.New("mapping no longer usable"), wantFill: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manualReviewRecorded := false
			fillCalled := false
			loadAssistedDocument = func(_ *sql.DB, _, _ string) (storage.AssistedDocument, error) {
				return storage.AssistedDocument{Path: "fixture"}, tc.docErr
			}
			loadAssistedPII = func(string) (*config.PII, error) { return &config.PII{}, nil }
			fillAssistedPage = func(submitter.AssistedFillPlan) (submitter.FillReport, error) {
				fillCalled = true
				return submitter.FillReport{}, tc.fillErr
			}
			recordAssistedRefill = func(*sql.DB, string, string, time.Time) error {
				t.Fatal("refill state recorded after continuation failure")
				return nil
			}
			recordAssistedManualReview = func(*sql.DB, string, string, time.Time) error {
				manualReviewRecorded = true
				return nil
			}

			keepOpen := continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "1"}, "owner")
			if !keepOpen || !manualReviewRecorded || fillCalled != tc.wantFill {
				t.Fatalf("keepOpen=%v manualReviewRecorded=%v fillCalled=%v", keepOpen, manualReviewRecorded, fillCalled)
			}
		})
	}
}

// TestContinueAssistedApplicationUsesResolvedPIIPath verifies that the PII
// file path computed from the workspace root is the one passed to the loader,
// so a launch from a subdirectory still reads the repository's pii.yaml
// (bugs.md #555).
func TestContinueAssistedApplicationUsesResolvedPIIPath(t *testing.T) {
	oldLoadDocument := loadAssistedDocument
	oldLoadPII := loadAssistedPII
	oldFillPage := fillAssistedPage
	oldRecordManualReview := recordAssistedManualReview
	t.Cleanup(func() {
		loadAssistedDocument = oldLoadDocument
		loadAssistedPII = oldLoadPII
		fillAssistedPage = oldFillPage
		recordAssistedManualReview = oldRecordManualReview
	})

	loadAssistedDocument = func(_ *sql.DB, _, _ string) (storage.AssistedDocument, error) {
		return storage.AssistedDocument{Path: "fixture"}, nil
	}
	// A refill error keeps the test on the manual-review path, avoiding the
	// success-path storage calls while still exercising the PII load.
	fillAssistedPage = func(submitter.AssistedFillPlan) (submitter.FillReport, error) {
		return submitter.FillReport{}, errors.New("unclassified")
	}
	recordAssistedManualReview = func(*sql.DB, string, string, time.Time) error { return nil }

	wantPath := filepath.Join(t.TempDir(), "pii.yaml")
	assistedPIIPath = wantPath
	var gotPath string
	loadAssistedPII = func(path string) (*config.PII, error) {
		gotPath = path
		return &config.PII{}, nil
	}

	continueAssistedApplication(nil, storage.AssistedLaunchInfo{JobID: "1"}, "owner")
	if gotPath != wantPath {
		t.Fatalf("loadAssistedPII called with %q, want %q", gotPath, wantPath)
	}
}

// TestFindWorkspaceRootWalksUpToGoMod verifies that cmd/assist resolves the
// repository root even when it is launched from a subdirectory, so relative
// paths like pii.yaml and applications.db point at the same files the
// dashboard sees (bugs.md #555).
func TestFindWorkspaceRootWalksUpToGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "cmd", "assist")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	got := findWorkspaceRoot()
	if got != root {
		t.Fatalf("findWorkspaceRoot from %q = %q, want %q", sub, got, root)
	}
}

func TestAssistedBrowserProfileDirRejectsEmptyCacheDirectory(t *testing.T) {
	if _, err := assistedBrowserProfileDir(""); err == nil {
		t.Fatal("expected empty cache directory to be rejected")
	}
}

func TestAssistedBrowserLaunchOptionsSuppressAutomationSignals(t *testing.T) {
	proxy := &playwright.Proxy{Server: "http://127.0.0.1:1234"}
	options := assistedBrowserLaunchOptions(proxy)
	if options.Proxy != proxy || options.Headless == nil || *options.Headless {
		t.Fatalf("proxy/headless options = %+v", options)
	}
	if len(options.IgnoreDefaultArgs) != 1 || options.IgnoreDefaultArgs[0] != "--enable-automation" {
		t.Fatalf("ignored default args = %#v", options.IgnoreDefaultArgs)
	}
	found := false
	for _, argument := range options.Args {
		if argument == "--disable-blink-features=AutomationControlled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("launch args = %#v", options.Args)
	}
}

func TestDirectAssistedBrowserArgumentsStartBlankBehindGuardedProxy(t *testing.T) {
	arguments := directAssistedBrowserArguments(
		"/private/profile",
		"/private/extension",
		"http://127.0.0.1:4321",
	)
	joined := strings.Join(arguments, "\n")
	for _, required := range []string{
		"--user-data-dir=/private/profile",
		"--proxy-server=http://127.0.0.1:4321",
		"--proxy-bypass-list=127.0.0.1;localhost",
		"--disable-quic",
		"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--disable-extensions-except=/private/extension",
		"--load-extension=/private/extension",
		"about:blank",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("browser arguments omit %q: %#v", required, arguments)
		}
	}
}

func TestWriteProxyAuthenticationExtensionUsesPrivateEphemeralCredentials(t *testing.T) {
	guard := security.NewNetworkGuard()
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	target := "https://apply.workable.com/example/j/ABC/apply/"
	readyURL := "http://127.0.0.1:54321/ready/token"
	extensionDir, err := writeProxyAuthenticationExtension(t.TempDir(), proxy, target, "Example Engineer", readyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(extensionDir)
	info, err := os.Stat(extensionDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != security.PrivateDirMode {
		t.Fatalf("extension directory mode = %o", info.Mode().Perm())
	}
	background, err := os.ReadFile(filepath.Join(extensionDir, "background.js"))
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, _ := url.Parse(proxy.URL())
	for _, expected := range []string{proxyURL.Hostname(), proxyURL.Port(), proxy.Username(), proxy.Password(), "details.isProxy", target, "example engineer", readyURL, "webNavigation.onCompleted"} {
		if !strings.Contains(string(background), expected) {
			t.Fatalf("proxy extension omits expected guarded value")
		}
	}
	for _, name := range []string{"manifest.json", "background.js"} {
		fileInfo, err := os.Stat(filepath.Join(extensionDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fileInfo.Mode().Perm() != security.PrivateFileMode {
			t.Fatalf("%s mode = %o", name, fileInfo.Mode().Perm())
		}
	}
}

func TestDirectBrowserReadinessRequiresPrivateCompletionURL(t *testing.T) {
	readyURL, ready, server, err := startDirectBrowserReadiness()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	response, err := http.Get(readyURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("readiness status = %d", response.StatusCode)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("readiness listener did not signal completion")
	}
}

func TestAssistedPageTitleMatchesRole(t *testing.T) {
	for _, tc := range []struct {
		title string
		role  string
		want  bool
	}{
		{"Meesho - Platform Engineer", "Platform Engineer", true},
		{"Meesho - Forward Deployed Engineer II", "Platform Engineer", false},
		{"CloudCo | Senior Platform Engineer", "Platform Engineer", true},
		{"CloudCo - Platform Engineering Manager", "Platform Engineer", false},
		{"Meesho", "Platform Engineer", false},
	} {
		if got := assistedPageTitleMatchesRole(tc.title, tc.role); got != tc.want {
			t.Errorf("assistedPageTitleMatchesRole(%q, %q) = %v, want %v", tc.title, tc.role, got, tc.want)
		}
	}
}
