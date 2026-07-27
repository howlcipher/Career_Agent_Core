package submitter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

const browserRouteValidationTimeout = 10 * time.Second

type secureBrowserSession struct {
	context playwright.BrowserContext
	guard   *security.NetworkGuard
	proxy   *security.HTTPProxy
}

func newSecureBrowserSession(
	browser playwright.Browser,
	options playwright.BrowserNewContextOptions,
) (*secureBrowserSession, error) {
	return newSecureBrowserSessionWithGuard(
		browser,
		options,
		security.NewNetworkGuard(),
	)
}

func newSecureBrowserSessionWithGuard(
	browser playwright.Browser,
	options playwright.BrowserNewContextOptions,
	guard *security.NetworkGuard,
) (*secureBrowserSession, error) {
	if browser == nil {
		return nil, errors.New("browser is nil")
	}
	if guard == nil {
		return nil, errors.New("network guard is nil")
	}
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		return nil, err
	}

	options.Proxy = &playwright.Proxy{
		Server: proxy.URL(),
		// Chromium otherwise applies an implicit loopback bypass even when a
		// proxy is configured. Subtracting that rule ensures localhost and
		// other loopback targets cannot evade the guarded proxy.
		Bypass:   playwright.String("<-loopback>"),
		Username: playwright.String(proxy.Username()),
		Password: playwright.String(proxy.Password()),
	}
	browserContext, err := browser.NewContext(options)
	if err != nil {
		return nil, errors.Join(err, proxy.Close())
	}
	return &secureBrowserSession{
		context: browserContext,
		guard:   guard,
		proxy:   proxy,
	}, nil
}

func (session *secureBrowserSession) Close() error {
	if session == nil {
		return nil
	}
	var contextErr error
	if session.context != nil {
		contextErr = session.context.Close()
	}
	return errors.Join(contextErr, session.proxy.Close())
}

func installSafeBrowserRoutes(
	page playwright.Page,
	guard *security.NetworkGuard,
) error {
	if page == nil || guard == nil {
		return errors.New("browser network boundary is incomplete")
	}
	err := page.Route("**/*", func(route playwright.Route) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			browserRouteValidationTimeout,
		)
		defer cancel()

		if err := guard.ValidateURL(ctx, route.Request().URL()); err != nil {
			if abortErr := route.Abort("accessdenied"); abortErr != nil {
				log.Printf(
					"[Auto-Submit] Failed to abort an unsafe browser request: %v",
					abortErr,
				)
			}
			return
		}
		if continueErr := route.Continue(); continueErr != nil {
			log.Printf(
				"[Auto-Submit] Failed to continue a validated browser request: %v",
				continueErr,
			)
		}
	})
	if err != nil {
		return fmt.Errorf("install browser network boundary: %w", err)
	}
	return nil
}
