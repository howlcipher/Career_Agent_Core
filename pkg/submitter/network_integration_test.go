package submitter

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

type integrationResolver struct{}

func (integrationResolver) LookupNetIP(
	_ context.Context,
	_ string,
	host string,
) ([]netip.Addr, error) {
	if host != "public.test" {
		return nil, errors.New("unexpected hostname")
	}
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

type integrationDialer struct {
	target string
}

func (dialer integrationDialer) DialContext(
	ctx context.Context,
	network string,
	_ string,
) (net.Conn, error) {
	var systemDialer net.Dialer
	return systemDialer.DialContext(ctx, network, dialer.target)
}

func TestSecureBrowserProxyWithChromium(t *testing.T) {
	if os.Getenv("CAREER_AGENT_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("set CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1 to run Chromium integration")
	}

	var loopbackRequests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(
		responseWriter http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = responseWriter.Write([]byte("proxied"))
	}))
	defer target.Close()
	loopbackTarget := httptest.NewServer(http.HandlerFunc(func(
		responseWriter http.ResponseWriter,
		_ *http.Request,
	) {
		loopbackRequests.Add(1)
		_, _ = responseWriter.Write([]byte("bypassed"))
	}))
	defer loopbackTarget.Close()

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	guard := security.NewNetworkGuard(
		security.WithResolver(integrationResolver{}),
		security.WithDialer(integrationDialer{target: targetURL.Host}),
	)

	playwrightRuntime, err := playwright.Run()
	if err != nil {
		t.Fatalf("start Playwright: %v", err)
	}
	defer playwrightRuntime.Stop()
	browser, err := playwrightRuntime.Chromium.Launch(
		playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true),
			Args:     []string{"--no-sandbox"},
		},
	)
	if err != nil {
		t.Fatalf("launch Chromium: %v", err)
	}
	defer browser.Close()

	session, err := newSecureBrowserSessionWithGuard(
		browser,
		playwright.BrowserNewContextOptions{
			IgnoreHttpsErrors: playwright.Bool(true),
		},
		guard,
	)
	if err != nil {
		t.Fatalf("create secure browser session: %v", err)
	}
	defer session.Close()
	page, err := session.context.NewPage()
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	defer page.Close()

	response, err := page.Goto(
		"https://public.test/public",
		playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		},
	)
	if err != nil {
		t.Fatalf("navigate through guarded proxy: %v", err)
	}
	if response == nil || response.Status() != http.StatusOK {
		t.Fatalf("public response = %v, want HTTP 200", response)
	}
	content, err := page.Content()
	if err != nil {
		t.Fatalf("read public response content: %v", err)
	}
	if !strings.Contains(content, "proxied") {
		t.Fatalf("public response did not traverse mapped proxy: %q", content)
	}

	loopbackResponse, loopbackErr := page.Goto(
		loopbackTarget.URL+"/loopback",
		playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		},
	)
	if loopbackErr == nil &&
		loopbackResponse != nil &&
		loopbackResponse.Status() != http.StatusBadGateway {
		t.Fatalf(
			"loopback response status = %d, want proxy rejection",
			loopbackResponse.Status(),
		)
	}
	if loopbackRequests.Load() != 0 {
		t.Fatalf(
			"Chromium bypassed the guarded proxy for %d loopback request(s)",
			loopbackRequests.Load(),
		)
	}
}
