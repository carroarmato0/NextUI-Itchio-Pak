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

	"golang.org/x/net/http2"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const apiItchIO = "https://api.itch.io"

// errH1Negotiated is returned by dialTLS when the server selects http/1.1
// via ALPN. h2FallbackTransport catches it to route the request (and all
// future requests to that host) through the h1 transport instead.
var errH1Negotiated = errors.New("server negotiated http/1.1")

// uaTransport identifies the app on every outbound request that does not set
// its own headers, then delegates to the wrapped RoundTripper. The app says
// what it is rather than posing as a browser: itch.io asked for a real
// User-Agent so it can see and support this traffic (issue #4).
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
	setDefaultHeader(req, "User-Agent", UserAgent())
	setDefaultHeader(req, "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	setDefaultHeader(req, "Accept-Language", "en-US,en;q=0.9")
	return t.wrapped.RoundTrip(req)
}

// dialTLSWithALPN dials a standard crypto/tls connection offering the given
// ALPN protocols.
func dialTLSWithALPN(ctx context.Context, network, addr string, protos []string) (*tls.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	d := &tls.Dialer{Config: &tls.Config{ServerName: host, NextProtos: protos}}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return conn.(*tls.Conn), nil
}

// dialTLS advertises ["h2", "http/1.1"] ALPN. If the server selects h2 the
// conn is returned to http2.Transport. If it selects http/1.1, the conn is
// closed and errH1Negotiated is returned so h2FallbackTransport can retry
// over the h1 transport. The cfg parameter satisfies http2.Transport's
// DialTLSContext signature but is ignored — ALPN is chosen here.
func dialTLS(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
	conn, err := dialTLSWithALPN(ctx, network, addr, []string{"h2", "http/1.1"})
	if err != nil {
		return nil, err
	}
	proto := conn.ConnectionState().NegotiatedProtocol
	logger.Debug("client: TLS addr=%s proto=%s", addr, proto)
	if proto != "h2" {
		conn.Close()
		return nil, errH1Negotiated
	}
	return conn, nil
}

// dialTLSH1 is the http.Transport-compatible dialer (no *tls.Config param)
// with http/1.1-only ALPN, for servers that do not support h2 (signed
// download CDNs, custom game hosting).
func dialTLSH1(ctx context.Context, network, addr string) (net.Conn, error) {
	return dialTLSWithALPN(ctx, network, addr, []string{"http/1.1"})
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
			wrapped: newRateLimitTransport(&h2FallbackTransport{
				h2:      h2t,
				h1:      h1t,
				h1hosts: make(map[string]struct{}),
			}),
		},
	}
}

type Client struct {
	http   *http.Client
	base   string // itch.io/api/1/... base URL
	butler string // api.itch.io base URL (butler-style endpoints)

	// purchaseCounts maps purchase_id to the number of distinct games it
	// covers, from the last full owned-keys scan. Lets a game_id-filtered
	// owned-keys answer still tell bundles from individual purchases.
	ownedMu        sync.Mutex
	purchaseCounts map[int64]int

	authTokenField
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

// DownloadURL streams directly from a pre-resolved CDN URL to dest.
// Use when the CDN URL was already resolved by ResolveFreeURL or ResolveAuthURL.
func (c *Client) DownloadURL(cdnURL, dest string, progress func(int64, int64)) error {
	return c.streamToFile(cdnURL, dest, progress)
}
