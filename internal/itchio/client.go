package itchio

import (
	"net/http"
	"net/http/cookiejar"
	"time"
)

// userAgent is sent on every outbound request to avoid Cloudflare bot-protection
// responses (which would return HTML instead of the expected XML/JSON payloads).
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// uaTransport injects the User-Agent header on every request that does not
// already set one, then delegates to the wrapped RoundTripper.
type uaTransport struct {
	wrapped http.RoundTripper
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", userAgent)
	}
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
