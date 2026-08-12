//go:build ui

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// TestUILifecycleStartStop drives the dashboard in a real Chromium browser,
// clicks Start and Stop, and verifies the UI reflects truthful lifecycle state.
// It is gated behind the "ui" build tag because it downloads/launches a browser.
func TestUILifecycleStartStop(t *testing.T) {
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		t.Fatal(err)
	}

	dashboardBin := filepath.Join(dir, "career_dashboard_bin")
	agentBin := filepath.Join(dir, "career_agent_bin")

	repoRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	repoRootStr := strings.TrimSpace(string(repoRoot))

	build := func(dst, pkg string) {
		cmd := exec.Command("go", "build", "-o", dst, pkg)
		cmd.Dir = repoRootStr
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, out)
		}
	}
	build(dashboardBin, "./cmd/dashboard")
	build(agentBin, "./cmd/dashboard/testdata/fake_agent")

	// Minimal fixtures so the dashboard can start without touching production data.
	writeFile := func(name, data string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("profile.yaml", "auto_submit: false\nskip_scoring: false\n")
	writeFile("operator_settings.yaml", "application_mode: find_only\nminimum_fit_score: 50\n")
	writeFile("applications/active_operator_settings.json",
		`{"application_mode":"find_only","minimum_fit_score":50,"scoring_active":true,"automatic_submit_click_active":false,"daemon_active":true}`)

	// Reserve an ephemeral port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	// Start the dashboard from the temp directory so it sees our synthetic fixtures.
	cmd := exec.Command(dashboardBin, "-addr", addr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"CAREER_AGENT_BIN="+agentBin,
		"FAKE_AGENT_STOP_DELAY=200ms",
		"CAREER_AGENT_STOP_TIMEOUT=5s",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dashboard: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	url := "http://" + addr
	if err := waitForDashboard(url, 10*time.Second); err != nil {
		t.Fatalf("dashboard did not become ready: %v", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("start playwright: %v", err)
	}
	defer func() { _ = pw.Stop() }()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args:     []string{"--no-sandbox"},
	})
	if err != nil {
		t.Fatalf("launch chromium: %v", err)
	}
	defer func() { _ = browser.Close() }()

	ctx, err := browser.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer func() { _ = ctx.Close() }()

	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	defer func() { _ = page.Close() }()

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		t.Fatalf("goto dashboard: %v", err)
	}

	// Initial state: SYSTEM STANDBY.
	if err := page.Locator(".system-state.standby").WaitFor(); err != nil {
		t.Fatalf("initial standby state not rendered: %v", err)
	}
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(dir, "01-standby.png"))})

	// Click Start and wait for the running state.
	if err := page.Locator(".btn.btn-start").Click(); err != nil {
		t.Fatalf("click start: %v", err)
	}
	if err := page.Locator(".system-state.online").WaitFor(); err != nil {
		t.Fatalf("running/online state not rendered after start: %v", err)
	}
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(dir, "02-running.png"))})

	// Click Stop and wait for the standby state to return.
	if err := page.Locator(".btn.btn-stop").Click(); err != nil {
		t.Fatalf("click stop: %v", err)
	}
	if err := page.Locator(".system-state.standby").WaitFor(); err != nil {
		t.Fatalf("standby state not rendered after stop: %v", err)
	}
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(dir, "03-stopped.png"))})

	// Confirm the backend agrees.
	resp, err := http.Get(url + "/api/agent/status")
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"running":false`) {
		t.Fatalf("backend status after UI stop = %q, want running=false", body)
	}
	t.Logf("UI lifecycle test passed; screenshots in %s", dir)
}

func waitForDashboard(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(baseURL + "/api/agent/status")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s: %w", baseURL, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
