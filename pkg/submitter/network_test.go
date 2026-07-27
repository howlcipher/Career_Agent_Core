package submitter

import (
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestNewSecureBrowserSessionConfiguresAuthenticatedLoopbackProxy(
	t *testing.T,
) {
	t.Parallel()

	var captured playwright.BrowserNewContextOptions
	mockContext := &MockContext{}
	mockBrowser := &MockBrowser{
		newContextFunc: func(
			options ...playwright.BrowserNewContextOptions,
		) (playwright.BrowserContext, error) {
			if len(options) != 1 {
				t.Fatalf("NewContext options count = %d, want 1", len(options))
			}
			captured = options[0]
			return mockContext, nil
		},
	}

	session, err := newSecureBrowserSession(
		mockBrowser,
		playwright.BrowserNewContextOptions{},
	)
	if err != nil {
		t.Fatalf("newSecureBrowserSession failed: %v", err)
	}
	defer session.Close()

	if captured.Proxy == nil {
		t.Fatal("browser context has no security proxy")
	}
	proxyURL, err := url.Parse(captured.Proxy.Server)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	host, _, err := net.SplitHostPort(proxyURL.Host)
	if err != nil {
		t.Fatalf("parse proxy address: %v", err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("proxy host = %q, want 127.0.0.1", host)
	}
	if captured.Proxy.Username == nil ||
		strings.TrimSpace(*captured.Proxy.Username) == "" {
		t.Fatal("browser proxy username is empty")
	}
	if captured.Proxy.Password == nil ||
		strings.TrimSpace(*captured.Proxy.Password) == "" {
		t.Fatal("browser proxy password is empty")
	}
	if captured.Proxy.Bypass == nil || *captured.Proxy.Bypass != "<-loopback>" {
		t.Fatalf("browser proxy bypass = %v, want <-loopback>", captured.Proxy.Bypass)
	}
}
