package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

// stubResolver answers from a fixed table so guard decisions in these tests are
// deterministic and never touch the network. A host absent from the table fails
// with failure, which lets a test choose the shape of an unresolvable answer.
type stubResolver struct {
	addresses map[string][]netip.Addr
	failure   error
}

func (resolver stubResolver) LookupNetIP(
	_ context.Context,
	_ string,
	host string,
) ([]netip.Addr, error) {
	if addresses, ok := resolver.addresses[host]; ok {
		return append([]netip.Addr(nil), addresses...), nil
	}
	if resolver.failure != nil {
		return nil, resolver.failure
	}
	return nil, errors.New("host not found")
}

// recordingRoute stands in for playwright.Route. Only Abort and Continue are
// needed, so the guard decision is observable without launching a browser.
type recordingRoute struct {
	aborts    []string
	continues int
	abortErr  error
}

func (route *recordingRoute) Abort(errorCode ...string) error {
	code := ""
	if len(errorCode) > 0 {
		code = errorCode[0]
	}
	route.aborts = append(route.aborts, code)
	return route.abortErr
}

func (route *recordingRoute) Continue(_ ...playwright.RouteContinueOptions) error {
	route.continues++
	return nil
}

func mustParseAddress(t *testing.T, raw string) netip.Addr {
	t.Helper()
	address, err := netip.ParseAddr(raw)
	if err != nil {
		t.Fatalf("parse address %q: %v", raw, err)
	}
	return address
}

// captureAssistLog redirects the standard logger for the duration of one test
// and returns what was written to it.
func captureAssistLog(t *testing.T, run func()) string {
	t.Helper()
	var captured bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&captured)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()
	run()
	return captured.String()
}

func newTestGuard(t *testing.T, resolver stubResolver) *security.NetworkGuard {
	t.Helper()
	return security.NewNetworkGuard(security.WithResolver(resolver))
}

func newRejectingGuard(t *testing.T) *security.NetworkGuard {
	t.Helper()
	return newTestGuard(t, stubResolver{
		addresses: map[string][]netip.Addr{
			"internal.test": {mustParseAddress(t, "10.0.0.7")},
			"public.test":   {mustParseAddress(t, "93.184.216.34")},
		},
	})
}

// A rejected request must leave evidence. Before bug #523 this path aborted in
// silence, and an operator could not tell the guard from an employer outage.
func TestGuardAssistedRequestLogsRejectedRequest(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		wantHost   string
		wantReason string
	}{
		{
			name:       "hostname resolving to a private address",
			rawURL:     "https://internal.test/jobs",
			wantHost:   "internal.test",
			wantReason: "private_dns_answer",
		},
		{
			name:       "private address literal",
			rawURL:     "https://10.0.0.7/jobs",
			wantHost:   "10.0.0.7",
			wantReason: "private_address",
		},
		{
			name:       "loopback hostname",
			rawURL:     "http://localhost/admin",
			wantHost:   "localhost",
			wantReason: "loopback_hostname",
		},
		{
			name:       "unsupported scheme",
			rawURL:     "file:///etc/passwd",
			wantHost:   "unknown",
			wantReason: "disallowed_scheme",
		},
		{
			name:       "credentials in the URL",
			rawURL:     "https://operator:secret@public.test/jobs",
			wantHost:   "public.test",
			wantReason: "url_credentials",
		},
		{
			name:       "unresolvable hostname",
			rawURL:     "https://missing.test/jobs",
			wantHost:   "missing.test",
			wantReason: "dns_resolution_failed",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			guard := newTestGuard(t, stubResolver{
				addresses: map[string][]netip.Addr{
					"internal.test": {mustParseAddress(t, "10.0.0.7")},
					"public.test":   {mustParseAddress(t, "93.184.216.34")},
				},
				failure: &net.DNSError{
					Err:        "no such host",
					Name:       "missing.test",
					IsNotFound: true,
				},
			})
			route := &recordingRoute{}

			output := captureAssistLog(t, func() {
				guardAssistedRequest(context.Background(), guard, test.rawURL, route)
			})

			if len(route.aborts) != 1 || route.aborts[0] != "accessdenied" {
				t.Fatalf("aborts = %v, want exactly one \"accessdenied\"", route.aborts)
			}
			if route.continues != 0 {
				t.Fatalf("continues = %d, want 0", route.continues)
			}
			want := "Assisted network guard blocked request: host=" +
				strconv.Quote(test.wantHost) +
				" reason=" + strconv.Quote(test.wantReason) + "\n"
			if output != want {
				t.Fatalf("log = %q, want %q", output, want)
			}
		})
	}
}

// The rejection log sees employer application pages. It must carry the host and
// nothing else: no credentials, no path, no query, no fragment.
func TestGuardAssistedRequestLogNeverDisclosesTheRequest(t *testing.T) {
	const rawURL = "https://user:password@example.test/application/12345" +
		"?token=secret-value#private"

	guard := newTestGuard(t, stubResolver{
		addresses: map[string][]netip.Addr{
			"example.test": {mustParseAddress(t, "10.1.2.3")},
		},
	})
	route := &recordingRoute{}

	output := captureAssistLog(t, func() {
		guardAssistedRequest(context.Background(), guard, rawURL, route)
	})

	for _, forbidden := range []string{
		"user",
		"password",
		"/application/12345",
		"12345",
		"token",
		"secret-value",
		"#private",
		"private-",
		rawURL,
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log %q discloses %q", output, forbidden)
		}
	}
	if !strings.Contains(output, "example.test") {
		t.Fatalf("log %q does not name the blocked host", output)
	}
	if len(route.aborts) != 1 {
		t.Fatalf("aborts = %v, want exactly one", route.aborts)
	}
}

// A malformed target is the case where the raw value is least trustworthy, so
// it must be dropped rather than echoed.
func TestGuardAssistedRequestHandlesMalformedURLs(t *testing.T) {
	malformed := []string{
		"://missing-scheme",
		"http://exa mple.test/jobs",
		"not a url at all",
		"http://%zz/jobs",
		"https://" + strings.Repeat("a", 300) + ".test/jobs",
		"http://ho\tst.test/jobs",
		"",
	}

	guard := newRejectingGuard(t)

	for _, rawURL := range malformed {
		rawURL := rawURL
		t.Run(rawURL, func(t *testing.T) {
			route := &recordingRoute{}
			output := captureAssistLog(t, func() {
				guardAssistedRequest(context.Background(), guard, rawURL, route)
			})

			if len(route.aborts) != 1 {
				t.Fatalf("aborts = %v, want exactly one", route.aborts)
			}
			if !strings.Contains(output, `host="unknown"`) {
				t.Fatalf("log %q does not use the safe unknown host", output)
			}
			if !strings.Contains(output, "reason=") {
				t.Fatalf("log %q carries no bounded reason", output)
			}
			if strings.Count(output, "\n") != 1 {
				t.Fatalf("log %q is not a single record", output)
			}
			if trimmed := strings.TrimSpace(rawURL); trimmed != "" &&
				strings.Contains(output, trimmed) {
				t.Fatalf("log %q echoes the malformed value %q", output, rawURL)
			}
		})
	}
}

// An allowed request must behave exactly as it did before: continued, silent.
func TestGuardAssistedRequestLeavesAllowedRequestsSilent(t *testing.T) {
	guard := newRejectingGuard(t)
	route := &recordingRoute{}

	output := captureAssistLog(t, func() {
		guardAssistedRequest(
			context.Background(),
			guard,
			"https://public.test/jobs?token=secret-value",
			route,
		)
	})

	if route.continues != 1 {
		t.Fatalf("continues = %d, want 1", route.continues)
	}
	if len(route.aborts) != 0 {
		t.Fatalf("aborts = %v, want none", route.aborts)
	}
	if output != "" {
		t.Fatalf("allowed request logged %q, want no record", output)
	}
}

// An arbitrary error from underneath the guard must still reduce to a bounded
// code. Its own message never reaches the log, whatever it happens to contain.
// The generic fallback itself is pinned in pkg/security, where the
// classification lives: every path ValidateURL can currently take is classified,
// so the fallback is not reachable from here.
func TestGuardAssistedRequestNeverLogsRawErrorText(t *testing.T) {
	guard := newTestGuard(t, stubResolver{
		failure: errors.New("resolver exploded on https://example.test/secret-path"),
	})
	route := &recordingRoute{}

	output := captureAssistLog(t, func() {
		guardAssistedRequest(
			context.Background(),
			guard,
			"https://example.test/secret-path",
			route,
		)
	})

	bounded := []string{
		"invalid_url",
		"disallowed_scheme",
		"missing_hostname",
		"url_credentials",
		"invalid_port",
		"loopback_hostname",
		"private_address",
		"private_dns_answer",
		"dns_resolution_failed",
		"dns_no_addresses",
		"resolver_unavailable",
		"network_guard_rejected",
	}
	matched := false
	for _, reason := range bounded {
		if strings.Contains(output, `reason="`+reason+`"`) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("log %q carries no reason from the bounded set", output)
	}
	for _, forbidden := range []string{"exploded", "secret-path", "resolver "} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log %q leaks the raw error text %q", output, forbidden)
		}
	}
}

// One blocked request is one log record. A per-request path that duplicates
// records makes the log less usable than the silence it replaced.
func TestGuardAssistedRequestLogsOneRecordPerRejection(t *testing.T) {
	guard := newRejectingGuard(t)
	route := &recordingRoute{}

	output := captureAssistLog(t, func() {
		guardAssistedRequest(
			context.Background(),
			guard,
			"https://internal.test/jobs",
			route,
		)
	})

	if got := strings.Count(output, "Assisted network guard blocked request:"); got != 1 {
		t.Fatalf("rejection records = %d, want 1 (log %q)", got, output)
	}
	if strings.Count(output, "\n") != 1 {
		t.Fatalf("log %q is not a single record", output)
	}
}

// A failed abort is worth reporting, but Playwright's error can quote the
// request, so only the fact and the safe host may be recorded.
func TestGuardAssistedRequestReportsAbortFailureWithoutTheRequest(t *testing.T) {
	guard := newRejectingGuard(t)
	route := &recordingRoute{
		abortErr: errors.New("route https://internal.test/jobs?token=secret-value is gone"),
	}

	output := captureAssistLog(t, func() {
		guardAssistedRequest(
			context.Background(),
			guard,
			"https://internal.test/jobs?token=secret-value",
			route,
		)
	})

	if !strings.Contains(output, "could not abort the blocked request") {
		t.Fatalf("log %q does not report the failed abort", output)
	}
	for _, forbidden := range []string{"token", "secret-value", "/jobs", "is gone"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log %q discloses %q from the abort error", output, forbidden)
		}
	}
	if got := strings.Count(output, "Assisted network guard blocked request:"); got != 1 {
		t.Fatalf("rejection records = %d, want exactly 1 (log %q)", got, output)
	}
}

// safeAssistedHost is the whole privacy contract for the rejection log, so it is
// pinned directly rather than only through the route behavior.
func TestSafeAssistedHost(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "plain host", rawURL: "https://example.test/a/b?c=d#e", want: "example.test"},
		{name: "credentials stripped", rawURL: "https://u:p@example.test/x", want: "example.test"},
		{name: "port stripped", rawURL: "https://example.test:8443/x", want: "example.test"},
		{name: "IPv6 literal", rawURL: "http://[2606:4700::1111]/x", want: "2606:4700::1111"},
		{name: "no host", rawURL: "https:///jobs", want: "unknown"},
		{name: "not a URL", rawURL: "gibberish", want: "unknown"},
		{name: "parse failure", rawURL: "http://exa mple.test", want: "unknown"},
		{name: "empty", rawURL: "", want: "unknown"},
		{
			name:   "over-long host",
			rawURL: "https://" + strings.Repeat("a", 300) + ".test/x",
			want:   "unknown",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := safeAssistedHost(test.rawURL); got != test.want {
				t.Fatalf("safeAssistedHost(%q) = %q, want %q", test.rawURL, got, test.want)
			}
		})
	}
}
