package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxHTTPRedirects = 10

// ErrUnsafeNetworkTarget identifies a syntactically invalid or non-public
// target. Resolver and connection failures do not wrap this sentinel, so
// callers can distinguish a durable safety rejection from a transient outage.
var ErrUnsafeNetworkTarget = errors.New("unsafe network target")

var blockedAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// NetworkResolver is the DNS boundary used by NetworkGuard. The interface
// keeps resolution deterministic in tests and lets callers provide a
// controlled resolver without weakening the address policy.
type NetworkResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// NetworkDialer is the connection boundary used after DNS answers pass the
// public-address policy.
type NetworkDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// NetworkGuard validates HTTP targets and dials only the exact IP addresses it
// has approved. Passing the validated address, rather than the original
// hostname, to the underlying dialer closes the DNS rebinding gap.
type NetworkGuard struct {
	resolver NetworkResolver
	dialer   NetworkDialer
}

// NetworkGuardOption replaces an injectable NetworkGuard dependency.
type NetworkGuardOption func(*NetworkGuard)

// WithResolver installs the resolver used by a NetworkGuard.
func WithResolver(resolver NetworkResolver) NetworkGuardOption {
	return func(guard *NetworkGuard) {
		if resolver != nil {
			guard.resolver = resolver
		}
	}
}

// WithDialer installs the final connection dialer used by a NetworkGuard.
func WithDialer(dialer NetworkDialer) NetworkGuardOption {
	return func(guard *NetworkGuard) {
		if dialer != nil {
			guard.dialer = dialer
		}
	}
}

// NewNetworkGuard returns a fail-closed public-network boundary.
func NewNetworkGuard(options ...NetworkGuardOption) *NetworkGuard {
	guard := &NetworkGuard{
		resolver: net.DefaultResolver,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
	for _, option := range options {
		if option != nil {
			option(guard)
		}
	}
	return guard
}

// IsPublicAddress reports whether address is ordinary public unicast space.
// It rejects private, loopback, link-local, multicast, documentation,
// benchmarking, carrier-grade NAT, transition, and other special-use ranges.
func IsPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, blocked := range blockedAddressRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func normalizeNetworkHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func unsafeNetworkTarget(reason string) error {
	return fmt.Errorf("%w: %s", ErrUnsafeNetworkTarget, reason)
}

func (guard *NetworkGuard) resolvePublicAddresses(
	ctx context.Context,
	host string,
) ([]netip.Addr, error) {
	if guard == nil || guard.resolver == nil {
		return nil, errors.New("network resolver is unavailable")
	}
	host = normalizeNetworkHost(host)
	if host == "" {
		return nil, unsafeNetworkTarget("missing hostname")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, unsafeNetworkTarget("loopback hostname")
	}

	if literal, err := netip.ParseAddr(host); err == nil {
		literal = literal.Unmap()
		if !IsPublicAddress(literal) {
			return nil, unsafeNetworkTarget("non-public address")
		}
		return []netip.Addr{literal}, nil
	}

	addresses, err := guard.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve network target: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("network target resolved to no addresses")
	}

	unique := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !IsPublicAddress(address) {
			return nil, unsafeNetworkTarget("non-public DNS answer")
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		unique = append(unique, address)
	}
	if len(unique) == 0 {
		return nil, errors.New("network target resolved to no usable addresses")
	}
	return unique, nil
}

func validateNetworkPort(port string) error {
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return unsafeNetworkTarget("invalid port")
	}
	return nil
}

// ValidateURL accepts only HTTP and HTTPS URLs whose complete DNS answer set
// consists of public addresses.
func (guard *NetworkGuard) ValidateURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse target: %v", ErrUnsafeNetworkTarget, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return unsafeNetworkTarget("scheme must be HTTP or HTTPS")
	}
	if parsed.Hostname() == "" {
		return unsafeNetworkTarget("missing hostname")
	}
	if parsed.User != nil {
		return unsafeNetworkTarget("user information is forbidden")
	}
	if port := parsed.Port(); port != "" {
		if err := validateNetworkPort(port); err != nil {
			return err
		}
	}
	_, err = guard.resolvePublicAddresses(ctx, parsed.Hostname())
	return err
}

// DialContext resolves the target, rejects the entire answer set if any
// address is non-public, and passes only validated IP literals to the final
// dialer.
func (guard *NetworkGuard) DialContext(
	ctx context.Context,
	network string,
	target string,
) (net.Conn, error) {
	if guard == nil || guard.dialer == nil {
		return nil, errors.New("network dialer is unavailable")
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("unsupported network %q", network)
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("parse network target address: %w", err)
	}
	if err := validateNetworkPort(port); err != nil {
		return nil, err
	}
	addresses, err := guard.resolvePublicAddresses(ctx, host)
	if err != nil {
		return nil, err
	}

	var dialErrors []error
	for _, address := range addresses {
		if network == "tcp4" && !address.Is4() {
			continue
		}
		if network == "tcp6" && address.Is4() {
			continue
		}
		validatedTarget := net.JoinHostPort(address.String(), port)
		connection, dialErr := guard.dialer.DialContext(
			ctx,
			network,
			validatedTarget,
		)
		if dialErr == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, dialErr)
		if ctx.Err() != nil {
			break
		}
	}
	if len(dialErrors) == 0 {
		return nil, fmt.Errorf("network target has no address for %s", network)
	}
	return nil, fmt.Errorf("connect to validated network target: %w", errors.Join(dialErrors...))
}

type validatingRoundTripper struct {
	guard *NetworkGuard
	next  http.RoundTripper
}

func (transport *validatingRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("HTTP request has no target URL")
	}
	if err := transport.guard.ValidateURL(
		request.Context(),
		request.URL.String(),
	); err != nil {
		return nil, fmt.Errorf("blocked unsafe HTTP request: %w", err)
	}
	return transport.next.RoundTrip(request)
}

func (guard *NetworkGuard) newHTTPTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A configured forward proxy would receive the original target and could
	// resolve it outside this boundary. Direct guarded dialing is required.
	transport.Proxy = nil
	transport.DialContext = guard.DialContext
	return transport
}

// HTTPClient returns a client that validates initial requests and redirects,
// then binds each new connection to a validated public IP.
func (guard *NetworkGuard) HTTPClient(timeout time.Duration) *http.Client {
	transport := guard.newHTTPTransport()
	return &http.Client{
		Timeout: timeout,
		Transport: &validatingRoundTripper{
			guard: guard,
			next:  transport,
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxHTTPRedirects {
				return fmt.Errorf("stopped after %d redirects", maxHTTPRedirects)
			}
			if err := guard.ValidateURL(
				request.Context(),
				request.URL.String(),
			); err != nil {
				return fmt.Errorf("blocked unsafe HTTP redirect: %w", err)
			}
			return nil
		},
	}
}

// NewSafeHTTPClient constructs an HTTP client with the default NetworkGuard.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	return NewNetworkGuard().HTTPClient(timeout)
}

// HTTPProxy is a temporary authenticated loopback proxy. Browser contexts use
// it so Chromium's actual connections pass through NetworkGuard.DialContext
// instead of relying on a separate preflight DNS check.
type HTTPProxy struct {
	guard     *NetworkGuard
	listener  net.Listener
	server    *http.Server
	transport *http.Transport
	username  string
	password  string

	closeOnce sync.Once
	connMu    sync.Mutex
	hijacked  map[net.Conn]struct{}
}

func randomProxyCredential() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate proxy credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// StartHTTPProxy starts an authenticated proxy on a random loopback port.
func (guard *NetworkGuard) StartHTTPProxy() (*HTTPProxy, error) {
	if guard == nil {
		return nil, errors.New("network guard is nil")
	}
	username, err := randomProxyCredential()
	if err != nil {
		return nil, err
	}
	password, err := randomProxyCredential()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start browser security proxy: %w", err)
	}

	proxy := &HTTPProxy{
		guard:     guard,
		listener:  listener,
		transport: guard.newHTTPTransport(),
		username:  username,
		password:  password,
		hijacked:  make(map[net.Conn]struct{}),
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		_ = proxy.server.Serve(listener)
	}()
	return proxy, nil
}

// URL returns the loopback proxy endpoint without credentials.
func (proxy *HTTPProxy) URL() string {
	if proxy == nil || proxy.listener == nil {
		return ""
	}
	return "http://" + proxy.listener.Addr().String()
}

// Username returns the ephemeral proxy username.
func (proxy *HTTPProxy) Username() string {
	if proxy == nil {
		return ""
	}
	return proxy.username
}

// Password returns the ephemeral proxy password.
func (proxy *HTTPProxy) Password() string {
	if proxy == nil {
		return ""
	}
	return proxy.password
}

func (proxy *HTTPProxy) authorized(request *http.Request) bool {
	got := request.Header.Get("Proxy-Authorization")
	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte(proxy.username+":"+proxy.password),
	)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func removeHopHeaders(header http.Header) {
	for _, name := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(name)
	}
}

func copyHTTPHeaders(destination, source http.Header) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func (proxy *HTTPProxy) ServeHTTP(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if !proxy.authorized(request) {
		responseWriter.Header().Set("Proxy-Authenticate", `Basic realm="career-agent"`)
		http.Error(
			responseWriter,
			"proxy authentication required",
			http.StatusProxyAuthRequired,
		)
		return
	}
	if request.Method == http.MethodConnect {
		proxy.serveConnect(responseWriter, request)
		return
	}
	proxy.serveHTTP(responseWriter, request)
}

func (proxy *HTTPProxy) serveHTTP(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.URL == nil ||
		(request.URL.Scheme != "http" && request.URL.Scheme != "https") {
		http.Error(responseWriter, "invalid proxy target", http.StatusBadRequest)
		return
	}
	outbound := request.Clone(request.Context())
	outbound.RequestURI = ""
	outbound.Header = request.Header.Clone()
	removeHopHeaders(outbound.Header)

	response, err := (&validatingRoundTripper{
		guard: proxy.guard,
		next:  proxy.transport,
	}).RoundTrip(outbound)
	if err != nil {
		http.Error(responseWriter, "proxy target blocked", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	headers := response.Header.Clone()
	removeHopHeaders(headers)
	copyHTTPHeaders(responseWriter.Header(), headers)
	responseWriter.WriteHeader(response.StatusCode)
	_, _ = io.Copy(responseWriter, response.Body)
}

func (proxy *HTTPProxy) trackConnection(connection net.Conn) func() {
	proxy.connMu.Lock()
	proxy.hijacked[connection] = struct{}{}
	proxy.connMu.Unlock()
	return func() {
		proxy.connMu.Lock()
		delete(proxy.hijacked, connection)
		proxy.connMu.Unlock()
	}
}

func closeWrite(connection net.Conn) {
	if tcpConnection, ok := connection.(*net.TCPConn); ok {
		_ = tcpConnection.CloseWrite()
	}
}

func (proxy *HTTPProxy) serveConnect(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if _, _, err := net.SplitHostPort(request.Host); err != nil {
		http.Error(responseWriter, "invalid proxy target", http.StatusBadRequest)
		return
	}
	upstream, err := proxy.guard.DialContext(
		request.Context(),
		"tcp",
		request.Host,
	)
	if err != nil {
		http.Error(responseWriter, "proxy target blocked", http.StatusBadGateway)
		return
	}

	hijacker, ok := responseWriter.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(responseWriter, "proxy tunnel unavailable", http.StatusInternalServerError)
		return
	}
	client, readWriter, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	removeClient := proxy.trackConnection(client)
	removeUpstream := proxy.trackConnection(upstream)
	defer removeClient()
	defer removeUpstream()
	defer client.Close()
	defer upstream.Close()

	if _, err := readWriter.WriteString(
		"HTTP/1.1 200 Connection Established\r\n\r\n",
	); err != nil {
		return
	}
	if err := readWriter.Flush(); err != nil {
		return
	}

	clientToUpstreamDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, readWriter)
		closeWrite(upstream)
		close(clientToUpstreamDone)
	}()
	_, _ = io.Copy(client, upstream)
	closeWrite(client)
	<-clientToUpstreamDone
}

// Close stops the proxy and closes idle or hijacked connections.
func (proxy *HTTPProxy) Close() error {
	if proxy == nil {
		return nil
	}
	var closeErr error
	proxy.closeOnce.Do(func() {
		if proxy.transport != nil {
			proxy.transport.CloseIdleConnections()
		}
		if proxy.server != nil {
			closeErr = proxy.server.Close()
			if errors.Is(closeErr, http.ErrServerClosed) {
				closeErr = nil
			}
		}
		proxy.connMu.Lock()
		for connection := range proxy.hijacked {
			closeErr = errors.Join(closeErr, connection.Close())
		}
		proxy.connMu.Unlock()
	})
	return closeErr
}
