package itchio

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func captureRefusalLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func getThroughRefusalLog(t *testing.T, srv *httptest.Server, path string) int {
	t.Helper()
	c := &http.Client{Transport: &refusalLogTransport{wrapped: http.DefaultTransport}}
	resp, err := c.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func TestRefusalLog_LogsCloudflareHeadersOn403(t *testing.T) {
	buf := captureRefusalLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Cf-Ray", "a57f11b19c208b7c-CDG")
		w.Header().Set("Cf-Mitigated", "challenge")
		w.Header().Set("Date", "Mon, 09 Nov 2026 11:19:35 GMT")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if code := getThroughRefusalLog(t, srv, "/2048-advance/data.json?token=secret"); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 passed through", code)
	}
	out := buf.String()
	for _, want := range []string{
		"[WARN]", "GET", "/2048-advance/data.json", "HTTP 403",
		"ray=a57f11b19c208b7c-CDG", "mitigated=challenge", "server=cloudflare",
		`date="Mon, 09 Nov 2026 11:19:35 GMT"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret") {
		t.Errorf("log leaks the query string:\n%s", out)
	}
}

func TestRefusalLog_DashesForMissingHeaders(t *testing.T) {
	buf := captureRefusalLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	getThroughRefusalLog(t, srv, "/games/tag-gbc.xml")
	out := buf.String()
	if !strings.Contains(out, "HTTP 429 ray=- mitigated=-") {
		t.Errorf("want dashes for absent cf headers:\n%s", out)
	}
}

func TestRefusalLog_SilentOnSuccessAndNotFound(t *testing.T) {
	buf := captureRefusalLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Ray", "abc-AMS")
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	getThroughRefusalLog(t, srv, "/ok")
	getThroughRefusalLog(t, srv, "/missing")
	if buf.Len() != 0 {
		t.Errorf("want no log for 200 and 404, got:\n%s", buf.String())
	}
}
