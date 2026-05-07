package itchio

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/cookiejar"
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

// dialTLS dials a TCP connection and upgrades it with a Chrome-compatible TLS
// handshake using utls, then returns the conn for use by http2.Transport.
// The cfg parameter is ignored — we build our own utls config to keep the
// Chrome ClientHello fingerprint that bypasses Cloudflare bot-protection.
// Chrome's ALPN list ["h2", "http/1.1"] is preserved so the server negotiates h2.
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
	return uconn, nil
}

// h2FallbackTransport routes HTTPS requests through http2.Transport (which
// multiplexes concurrent image fetches over a single connection) and plain
// HTTP requests through a standard transport (used by httptest servers in tests).
type h2FallbackTransport struct {
	h2 *http2.Transport
	h1 *http.Transport
}

func (t *h2FallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		return t.h2.RoundTrip(req)
	}
	return t.h1.RoundTrip(req)
}

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	h2t := &http2.Transport{
		DialTLSContext: dialTLS,
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		Transport: &uaTransport{
			wrapped: &h2FallbackTransport{h2: h2t, h1: &http.Transport{}},
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
