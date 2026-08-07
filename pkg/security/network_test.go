package security

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type staticResolver map[string][]netip.Addr

func (r staticResolver) LookupNetIP(
	_ context.Context,
	_ string,
	host string,
) ([]netip.Addr, error) {
	addresses, ok := r[host]
	if !ok {
		return nil, errors.New("host not found")
	}
	return append([]netip.Addr(nil), addresses...), nil
}

type sequenceResolver struct {
	mu        sync.Mutex
	responses map[string][][]netip.Addr
}

func (r *sequenceResolver) LookupNetIP(
	_ context.Context,
	_ string,
	host string,
) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	responses := r.responses[host]
	if len(responses) == 0 {
		return nil, errors.New("host not found")
	}
	response := responses[0]
	if len(responses) > 1 {
		r.responses[host] = responses[1:]
	}
	return append([]netip.Addr(nil), response...), nil
}

type recordingDialer struct {
	mu        sync.Mutex
	addresses []string
	dial      func(context.Context, string, string) (net.Conn, error)
}

func (d *recordingDialer) DialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	d.mu.Lock()
	d.addresses = append(d.addresses, address)
	d.mu.Unlock()
	if d.dial != nil {
		return d.dial(ctx, network, address)
	}
	client, server := net.Pipe()
	go func() {
		_ = server.Close()
	}()
	return client, nil
}

func (d *recordingDialer) dialedAddresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.addresses...)
}

func mustAddress(t *testing.T, raw string) netip.Addr {
	t.Helper()
	address, err := netip.ParseAddr(raw)
	if err != nil {
		t.Fatalf("parse address %q: %v", raw, err)
	}
	return address
}

func TestIsPublicAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "93.184.216.34", want: true},
		{raw: "2606:4700:4700::1111", want: true},
		{raw: "0.0.0.0", want: false},
		{raw: "10.1.2.3", want: false},
		{raw: "100.64.0.1", want: false},
		{raw: "127.0.0.1", want: false},
		{raw: "169.254.1.2", want: false},
		{raw: "172.16.0.1", want: false},
		{raw: "192.168.1.2", want: false},
		{raw: "198.18.0.1", want: false},
		{raw: "224.0.0.1", want: false},
		{raw: "240.0.0.1", want: false},
		{raw: "::", want: false},
		{raw: "::1", want: false},
		{raw: "::ffff:127.0.0.1", want: false},
		{raw: "fc00::1", want: false},
		{raw: "fe80::1", want: false},
		{raw: "ff02::1", want: false},
		{raw: "2001:db8::1", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()
			if got := IsPublicAddress(mustAddress(t, test.raw)); got != test.want {
				t.Fatalf("IsPublicAddress(%s) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestNetworkGuardValidateURL(t *testing.T) {
	t.Parallel()

	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test": {
				mustAddress(t, "93.184.216.34"),
				mustAddress(t, "2606:4700:4700::1111"),
			},
			"private.test": {
				mustAddress(t, "10.0.0.4"),
			},
			"mixed.test": {
				mustAddress(t, "93.184.216.34"),
				mustAddress(t, "fd00::4"),
			},
		}),
	)

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "public hostname", rawURL: "https://public.test/jobs", wantErr: false},
		{name: "public IPv4 literal", rawURL: "https://93.184.216.34/jobs", wantErr: false},
		{name: "unsupported scheme", rawURL: "file:///etc/passwd", wantErr: true},
		{name: "missing host", rawURL: "https:///jobs", wantErr: true},
		{name: "userinfo", rawURL: "https://user:pass@public.test/jobs", wantErr: true},
		{name: "loopback IPv4", rawURL: "http://127.0.0.1/admin", wantErr: true},
		{name: "loopback IPv6", rawURL: "http://[::1]/admin", wantErr: true},
		{name: "private DNS answer", rawURL: "https://private.test/admin", wantErr: true},
		{name: "mixed DNS answers", rawURL: "https://mixed.test/admin", wantErr: true},
		{name: "unresolved hostname", rawURL: "https://missing.test/jobs", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := guard.ValidateURL(context.Background(), test.rawURL)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateURL(%q) error = %v, wantErr %v", test.rawURL, err, test.wantErr)
			}
		})
	}
}

func TestNetworkGuardDialContextUsesValidatedAddress(t *testing.T) {
	t.Parallel()

	dialer := &recordingDialer{}
	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test": {mustAddress(t, "93.184.216.34")},
		}),
		WithDialer(dialer),
	)

	conn, err := guard.DialContext(context.Background(), "tcp", "public.test:443")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	_ = conn.Close()

	addresses := dialer.dialedAddresses()
	if len(addresses) != 1 || addresses[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %v, want [93.184.216.34:443]", addresses)
	}
}

func TestNetworkGuardDialContextRejectsMixedAnswersBeforeDial(t *testing.T) {
	t.Parallel()

	dialer := &recordingDialer{}
	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"mixed.test": {
				mustAddress(t, "93.184.216.34"),
				mustAddress(t, "10.0.0.4"),
			},
		}),
		WithDialer(dialer),
	)

	if _, err := guard.DialContext(context.Background(), "tcp", "mixed.test:80"); err == nil {
		t.Fatal("DialContext accepted mixed public and private DNS answers")
	}
	if addresses := dialer.dialedAddresses(); len(addresses) != 0 {
		t.Fatalf("dialer was called for unsafe answers: %v", addresses)
	}
}

func TestSafeHTTPClientBlocksPrivateRedirect(t *testing.T) {
	t.Parallel()

	var privateRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case "public.test":
			http.Redirect(w, r, "http://private.test/secret", http.StatusFound)
		case "private.test":
			privateRequests++
			_, _ = io.WriteString(w, "secret")
		default:
			http.Error(w, "unexpected host", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	serverAddress := strings.TrimPrefix(server.URL, "http://")
	dialer := &recordingDialer{
		dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dial net.Dialer
			return dial.DialContext(ctx, network, serverAddress)
		},
	}
	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test":  {mustAddress(t, "93.184.216.34")},
			"private.test": {mustAddress(t, "10.0.0.4")},
		}),
		WithDialer(dialer),
	)

	_, err := guard.HTTPClient(2 * time.Second).Get("http://public.test/start")
	if err == nil {
		t.Fatal("safe client followed a redirect to a private address")
	}
	if privateRequests != 0 {
		t.Fatalf("private redirect endpoint received %d request(s)", privateRequests)
	}
	if addresses := dialer.dialedAddresses(); len(addresses) != 1 ||
		addresses[0] != "93.184.216.34:80" {
		t.Fatalf("dialed addresses = %v, want only the public target", addresses)
	}
}

func TestSafeHTTPClientRevalidatesAtDialToBlockRebinding(t *testing.T) {
	t.Parallel()

	resolver := &sequenceResolver{
		responses: map[string][][]netip.Addr{
			"rebind.test": {
				{mustAddress(t, "93.184.216.34")},
				{mustAddress(t, "127.0.0.1")},
			},
		},
	}
	dialer := &recordingDialer{}
	guard := NewNetworkGuard(
		WithResolver(resolver),
		WithDialer(dialer),
	)

	_, err := guard.HTTPClient(time.Second).Get("http://rebind.test/")
	if err == nil {
		t.Fatal("safe client accepted a hostname that rebound before dialing")
	}
	if addresses := dialer.dialedAddresses(); len(addresses) != 0 {
		t.Fatalf("dialer was called after rebinding: %v", addresses)
	}
}

func TestHTTPProxyBindsBrowserStyleRequestsToValidatedDial(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	targetAddress := strings.TrimPrefix(target.URL, "http://")
	dialer := &recordingDialer{
		dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dial net.Dialer
			return dial.DialContext(ctx, network, targetAddress)
		},
	}
	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test":  {mustAddress(t, "93.184.216.34")},
			"private.test": {mustAddress(t, "127.0.0.1")},
		}),
		WithDialer(dialer),
	)

	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		t.Fatalf("StartHTTPProxy failed: %v", err)
	}
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	proxyURL.User = url.UserPassword(proxy.Username(), proxy.Password())
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	resp, err := client.Get("http://public.test/jobs")
	if err != nil {
		t.Fatalf("proxy request to public host failed: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read proxy response: %v", readErr)
	}
	if string(body) != "ok" {
		t.Fatalf("proxy body = %q, want ok", body)
	}

	blockedResponse, err := client.Get("http://private.test/admin")
	if err != nil {
		t.Fatalf("read blocked proxy response: %v", err)
	}
	_ = blockedResponse.Body.Close()
	if blockedResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf(
			"private proxy status = %d, want %d",
			blockedResponse.StatusCode,
			http.StatusBadGateway,
		)
	}
	addresses := dialer.dialedAddresses()
	if len(addresses) != 1 || addresses[0] != "93.184.216.34:80" {
		t.Fatalf("dialed addresses = %v, want only the public target", addresses)
	}
}

func TestHTTPProxyRequiresAuthentication(t *testing.T) {
	t.Parallel()

	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test": {mustAddress(t, "93.184.216.34")},
		}),
	)
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		t.Fatalf("StartHTTPProxy failed: %v", err)
	}
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	resp, err := client.Get("http://public.test/jobs")
	if err != nil {
		t.Fatalf("unauthenticated proxy response failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("proxy status = %d, want %d", resp.StatusCode, http.StatusProxyAuthRequired)
	}
}

func TestHTTPProxyConnectUsesValidatedAddress(t *testing.T) {
	t.Parallel()

	dialer := &recordingDialer{}
	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test":  {mustAddress(t, "93.184.216.34")},
			"private.test": {mustAddress(t, "10.0.0.4")},
		}),
		WithDialer(dialer),
	)
	proxy, err := guard.StartHTTPProxy()
	if err != nil {
		t.Fatalf("StartHTTPProxy failed: %v", err)
	}
	defer proxy.Close()

	requestConnect := func(target string) int {
		t.Helper()
		connection, err := net.Dial(
			"tcp",
			strings.TrimPrefix(proxy.URL(), "http://"),
		)
		if err != nil {
			t.Fatalf("connect to proxy: %v", err)
		}
		defer connection.Close()

		credential := base64.StdEncoding.EncodeToString(
			[]byte(proxy.Username() + ":" + proxy.Password()),
		)
		request := &http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Host: target},
			Host:   target,
			Header: http.Header{
				"Proxy-Authorization": {"Basic " + credential},
			},
		}
		if err := request.WriteProxy(connection); err != nil {
			t.Fatalf("write CONNECT request: %v", err)
		}
		response, err := http.ReadResponse(bufio.NewReader(connection), request)
		if err != nil {
			t.Fatalf("read CONNECT response: %v", err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}

	if status := requestConnect("public.test:443"); status != http.StatusOK {
		t.Fatalf("public CONNECT status = %d, want %d", status, http.StatusOK)
	}
	if status := requestConnect("private.test:443"); status != http.StatusBadGateway {
		t.Fatalf(
			"private CONNECT status = %d, want %d",
			status,
			http.StatusBadGateway,
		)
	}
	addresses := dialer.dialedAddresses()
	if len(addresses) != 1 || addresses[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %v, want only the public target", addresses)
	}
}

// Every rejection a caller can log must reduce to a bounded code. Bug #523's
// contract is that a caller never has to touch the error's own message, which
// can quote the target URL.
func TestNetworkRejectionReasonClassifiesGuardErrors(t *testing.T) {
	t.Parallel()

	guard := NewNetworkGuard(
		WithResolver(staticResolver{
			"public.test":  {mustAddress(t, "93.184.216.34")},
			"private.test": {mustAddress(t, "10.0.0.4")},
		}),
	)

	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "unparseable URL", rawURL: "http://exa mple.test/jobs", want: RejectionInvalidURL},
		{name: "unsupported scheme", rawURL: "file:///etc/passwd", want: RejectionDisallowedScheme},
		{name: "missing host", rawURL: "https:///jobs", want: RejectionMissingHostname},
		{name: "userinfo", rawURL: "https://u:p@public.test/jobs", want: RejectionURLCredentials},
		{name: "invalid port", rawURL: "https://public.test:0/jobs", want: RejectionInvalidPort},
		{name: "loopback hostname", rawURL: "http://localhost/admin", want: RejectionLoopbackHostname},
		{name: "private literal", rawURL: "http://127.0.0.1/admin", want: RejectionPrivateAddress},
		{name: "private DNS answer", rawURL: "https://private.test/admin", want: RejectionPrivateDNSAnswer},
		{name: "resolution failure", rawURL: "https://missing.test/jobs", want: RejectionDNSFailed},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := guard.ValidateURL(context.Background(), test.rawURL)
			if err == nil {
				t.Fatalf("ValidateURL(%q) unexpectedly succeeded", test.rawURL)
			}
			if got := NetworkRejectionReason(err); got != test.want {
				t.Fatalf("NetworkRejectionReason(%v) = %q, want %q", err, got, test.want)
			}
		})
	}
}

func TestNetworkRejectionReasonFallsBackWithoutTheErrorText(t *testing.T) {
	t.Parallel()

	secret := "https://example.test/application/12345?token=secret-value"
	for _, err := range []error{
		nil,
		errors.New(secret),
		fmt.Errorf("wrapped: %w", errors.New(secret)),
	} {
		reason := NetworkRejectionReason(err)
		if reason != RejectionUnclassified {
			t.Fatalf("NetworkRejectionReason(%v) = %q, want %q", err, reason, RejectionUnclassified)
		}
		if strings.Contains(reason, "example.test") || strings.Contains(reason, "secret") {
			t.Fatalf("fallback reason %q carries request data", reason)
		}
	}
}

// The typed rejections must keep wrapping the sentinel: cmd/agent tells a
// durable safety refusal from a transient outage by errors.Is on it.
func TestNetworkRejectionsStillWrapTheSentinel(t *testing.T) {
	t.Parallel()

	guard := NewNetworkGuard(WithResolver(staticResolver{}))

	safetyRejection := guard.ValidateURL(context.Background(), "http://127.0.0.1/admin")
	if !errors.Is(safetyRejection, ErrUnsafeNetworkTarget) {
		t.Fatalf("safety rejection %v no longer wraps ErrUnsafeNetworkTarget", safetyRejection)
	}

	resolverOutage := guard.ValidateURL(context.Background(), "https://missing.test/jobs")
	if errors.Is(resolverOutage, ErrUnsafeNetworkTarget) {
		t.Fatalf("resolver outage %v must not wrap ErrUnsafeNetworkTarget", resolverOutage)
	}
}
