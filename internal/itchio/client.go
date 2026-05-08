package itchio

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	// userAgent is sent on every outbound request to avoid Cloudflare bot-protection
	// responses (which would return HTML instead of the expected XML/JSON payloads).
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	apiItchIO = "https://api.itch.io"
)

// errH1Negotiated is returned by dialTLS when the server selects http/1.1
// via ALPN. h2FallbackTransport catches it to route the request (and all
// future requests to that host) through the h1 transport instead.
var errH1Negotiated = errors.New("server negotiated http/1.1")

// uaTransport injects browser-compatible headers on every outbound request
// that does not already have them, then delegates to the wrapped RoundTripper.
type uaTransport struct {
	wrapped http.RoundTripper
}

func setDefaultHeader(req *http.Request, key, value string) {
	if req.Header.Get(key) == "" {
		req.Header.Set(key, value)
	}
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	setDefaultHeader(req, "User-Agent", userAgent)
	setDefaultHeader(req, "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	setDefaultHeader(req, "Accept-Language", "en-US,en;q=0.9")
	setDefaultHeader(req, "sec-ch-ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
	setDefaultHeader(req, "sec-ch-ua-mobile", "?0")
	setDefaultHeader(req, "sec-ch-ua-platform", `"Windows"`)
	setDefaultHeader(req, "Sec-Fetch-Dest", "document")
	setDefaultHeader(req, "Sec-Fetch-Mode", "navigate")
	setDefaultHeader(req, "Sec-Fetch-Site", "none")
	setDefaultHeader(req, "Sec-Fetch-User", "?1")
	setDefaultHeader(req, "Cache-Control", "max-age=0")
	return t.wrapped.RoundTrip(req)
}

// dialTLS dials a TLS connection using the Chrome ClientHello fingerprint via
// utls, advertising ["h2", "http/1.1"] ALPN. If the server selects h2 the
// conn is returned to http2.Transport. If it selects http/1.1, the conn is
// closed and errH1Negotiated is returned so h2FallbackTransport can retry
// over the h1 transport. The cfg parameter satisfies http2.Transport's
// DialTLSContext signature but is ignored — we build our own utls config.
func dialTLS(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
	if err := uconn.BuildHandshakeState(); err != nil {
		conn.Close()
		return nil, err
	}
	for _, ext := range uconn.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"h2", "http/1.1"}
			break
		}
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	proto := uconn.ConnectionState().NegotiatedProtocol
	logger.Debug("client: TLS addr=%s proto=%s", addr, proto)
	if proto != "h2" {
		uconn.Close()
		return nil, errH1Negotiated
	}
	return uconn, nil
}

// dialTLSH1 is the http.Transport-compatible dialer (no *tls.Config param)
// using the Chrome utls fingerprint with http/1.1-only ALPN, for servers
// that do not support h2 (signed download CDNs, custom game hosting).
func dialTLSH1(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
	if err := uconn.BuildHandshakeState(); err != nil {
		conn.Close()
		return nil, err
	}
	for _, ext := range uconn.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
			break
		}
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return uconn, nil
}

// h2FallbackTransport routes HTTPS requests through http2.Transport for h2
// servers (itch.io game pages, API, image CDN) and falls back to an h1
// transport for servers that only negotiate http/1.1 (Cloudflare R2 signed
// download URLs, custom game hosting). Per-host routing is cached so the
// extra handshake only occurs on the first request to each h1-only host.
// Plain HTTP requests (httptest servers in tests) always use the h1 transport.
type h2FallbackTransport struct {
	h2 *http2.Transport
	h1 *http.Transport

	mu      sync.RWMutex
	h1hosts map[string]struct{} // hosts that negotiated http/1.1
}

func (t *h2FallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.h1.RoundTrip(req)
	}

	host := req.URL.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "443")
	}

	t.mu.RLock()
	_, isH1 := t.h1hosts[host]
	t.mu.RUnlock()

	if isH1 {
		return t.h1.RoundTrip(req)
	}

	resp, err := t.h2.RoundTrip(req)
	if errors.Is(err, errH1Negotiated) {
		logger.Info("client: %s negotiates http/1.1, caching as h1-only", host)
		t.mu.Lock()
		t.h1hosts[host] = struct{}{}
		t.mu.Unlock()
		return t.h1.RoundTrip(req)
	}
	return resp, err
}

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	h2t := &http2.Transport{
		DialTLSContext: dialTLS,
	}
	h1t := &http.Transport{
		DialTLSContext: dialTLSH1,
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		Transport: &uaTransport{
			wrapped: &h2FallbackTransport{
				h2:      h2t,
				h1:      h1t,
				h1hosts: make(map[string]struct{}),
			},
		},
	}
}

type Client struct {
	http   *http.Client
	base   string // itch.io/api/1/... base URL
	butler string // api.itch.io base URL (butler-style endpoints)

	// Background API key validation state (atomic, written once per session).
	apiKeyStatus   int32 // stores APIKeyStatus constants
	apiKeyChecking int32 // 0 = not started, 1 = started (CAS gate)
}

func NewClient() *Client {
	return &Client{
		http:   newHTTPClient(),
		base:   "https://itch.io",
		butler: apiItchIO,
	}
}

func NewClientWithBase(base string) *Client {
	return &Client{
		http:   newHTTPClient(),
		base:   base,
		butler: apiItchIO,
	}
}

// NewClientWithBaseAndButler is used in tests to override both base URLs.
func NewClientWithBaseAndButler(base, butler string) *Client {
	return &Client{
		http:   newHTTPClient(),
		base:   base,
		butler: butler,
	}
}

// HTTPClient returns the underlying *http.Client used for all requests.
func (c *Client) HTTPClient() *http.Client {
	return c.http
}
