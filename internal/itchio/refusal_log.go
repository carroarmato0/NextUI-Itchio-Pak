package itchio

import (
	"net/http"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// refusalLogTransport logs the identifying headers of every refused request,
// so a user's log is enough for itch.io to find the request on their side.
// Cloudflare's cf-ray names the exact request in its dashboard; cf-mitigated
// says whether it was a bot challenge rather than an origin or WAF refusal;
// the Date header is the server's own clock, in UTC, which the device-local
// timestamps of the log are not.
//
// It sits below rateLimitTransport, so every attempt of a retried request is
// logged with its own ray. 404 is skipped: feeds paginate into it routinely
// and it is never a block. Only host and path are logged — signed download
// URLs carry their credentials in the query string.
type refusalLogTransport struct {
	wrapped http.RoundTripper
}

func (t *refusalLogTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrapped.RoundTrip(req)
	if err != nil || resp.StatusCode < 400 || resp.StatusCode == http.StatusNotFound {
		return resp, err
	}
	logger.Warn("client: %s %s%s -> HTTP %d ray=%s mitigated=%s server=%s date=%q",
		req.Method, req.URL.Host, loggablePath(req.URL.Path), resp.StatusCode,
		headerOrDash(resp.Header, "Cf-Ray"),
		headerOrDash(resp.Header, "Cf-Mitigated"),
		headerOrDash(resp.Header, "Server"),
		resp.Header.Get("Date"))
	return resp, nil
}

func headerOrDash(h http.Header, key string) string {
	if v := h.Get(key); v != "" {
		return v
	}
	return "-"
}
