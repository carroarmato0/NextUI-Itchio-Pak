package itchio

import (
	"net/http"
	"net/http/cookiejar"
	"time"
)

// userAgent is sent on every outbound request to avoid Cloudflare bot-protection
// responses (which would return HTML instead of the expected XML/JSON payloads).
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

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

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:       jar,
		Timeout:   30 * time.Second,
		Transport: &uaTransport{wrapped: http.DefaultTransport},
	}
}

type Client struct {
	http   *http.Client
	base   string // itch.io/api/1/... base URL
	butler string // api.itch.io base URL (butler-style endpoints)
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
