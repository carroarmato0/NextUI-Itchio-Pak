package itchio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock drives rateLimitTransport without real sleeps: sleeping advances
// the clock and records how long was waited.
type fakeClock struct {
	mu    sync.Mutex
	t     time.Time
	slept []time.Duration
}

// Anchored to the real time so request deadlines built from it are live.
func newFakeClock() *fakeClock { return &fakeClock{t: time.Now().Truncate(time.Second)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.t = c.t.Add(d)
	return ctx.Err()
}

func (c *fakeClock) totalSlept() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	var total time.Duration
	for _, d := range c.slept {
		total += d
	}
	return total
}

func newTestRateLimit(clock *fakeClock) *rateLimitTransport {
	rt := newRateLimitTransport(http.DefaultTransport)
	rt.now = clock.now
	rt.sleep = clock.sleep
	return rt
}

// server answers 429 (with the given Retry-After) for the first n requests,
// then 200.
func rateLimitedServer(t *testing.T, n int32, retryAfter string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) <= n {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func get(t *testing.T, rt http.RoundTripper, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
	return resp
}

func TestRateLimit_honoursRetryAfterSeconds(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, hits := rateLimitedServer(t, 1, "7")

	if resp := get(t, rt, srv.URL); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after retry", resp.StatusCode)
	}
	if *hits != 2 {
		t.Errorf("server hits = %d, want 2", *hits)
	}
	if got := clock.totalSlept(); got != 7*time.Second {
		t.Errorf("waited %v, want exactly the 7s Retry-After", got)
	}
}

func TestRateLimit_honoursRetryAfterDate(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	at := clock.now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	srv, _ := rateLimitedServer(t, 1, at)

	get(t, rt, srv.URL)
	if got := clock.totalSlept(); got != 30*time.Second {
		t.Errorf("waited %v, want 30s from the HTTP-date", got)
	}
}

func TestRateLimit_capsRetryAfter(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, _ := rateLimitedServer(t, 1, "86400")

	get(t, rt, srv.URL)
	if got := clock.totalSlept(); got != rateLimitMaxRetryAfter {
		t.Errorf("waited %v, want the %v cap", got, rateLimitMaxRetryAfter)
	}
}

func TestRateLimit_exponentialBackoffWithoutRetryAfter(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, hits := rateLimitedServer(t, 100, "")

	resp := get(t, rt, srv.URL)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want the 429 handed back after retries", resp.StatusCode)
	}
	if *hits != 1+rateLimitMaxRetries {
		t.Errorf("server hits = %d, want %d", *hits, 1+rateLimitMaxRetries)
	}
	// Two waits: ~2s then ~4s, each with at most +20% jitter.
	if len(clock.slept) != 2 {
		t.Fatalf("slept %v, want two waits", clock.slept)
	}
	for i, base := range []time.Duration{2 * time.Second, 4 * time.Second} {
		if d := clock.slept[i]; d < base || d > base+base/5 {
			t.Errorf("wait %d = %v, want %v..%v", i, d, base, base+base/5)
		}
	}
}

func TestRateLimit_successResetsBackoff(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, _ := rateLimitedServer(t, 1, "")
	get(t, rt, srv.URL) // 429 then 200

	if hc := rt.hosts[strings.TrimPrefix(srv.URL, "http://")]; hc == nil || hc.strikes != 0 {
		t.Errorf("strikes not reset after a success: %+v", hc)
	}
}

// A 429 on one request must hold back every other request to the same host.
func TestRateLimit_cooldownIsSharedAcrossRequests(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)

	var reached int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reached, 1)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	rt.record429(host, "10")

	get(t, rt, srv.URL)
	if got := clock.totalSlept(); got != 10*time.Second {
		t.Errorf("an unrelated request waited %v, want the host's 10s cooldown", got)
	}
	if reached != 1 {
		t.Errorf("request reached server %d times", reached)
	}
}

func TestRateLimit_otherHostsUnaffected(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	rt.record429("img.itch.zone", "60")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	get(t, rt, srv.URL)
	if got := clock.totalSlept(); got != 0 {
		t.Errorf("waited %v for a host that never answered 429", got)
	}
}

// Responses to requests already in flight when the cooldown began are one
// episode, not three strikes.
func TestRateLimit_concurrent429sAreOneStrike(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	for i := 0; i < 3; i++ {
		rt.record429("itch.io", "")
	}
	if s := rt.hosts["itch.io"].strikes; s != 1 {
		t.Errorf("strikes = %d, want 1", s)
	}
}

func TestRateLimit_postIsNotRetried(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, hits := rateLimitedServer(t, 1, "5")

	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("csrf_token=x"))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || *hits != 1 {
		t.Errorf("POST: status %d after %d hits, want the 429 after 1", resp.StatusCode, *hits)
	}
	// ...but the next request still honours the cooldown.
	get(t, rt, srv.URL)
	if got := clock.totalSlept(); got != 5*time.Second {
		t.Errorf("next request waited %v, want 5s", got)
	}
}

func TestRateLimit_failsFastPastDeadline(t *testing.T) {
	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	srv, hits := rateLimitedServer(t, 0, "")
	host := strings.TrimPrefix(srv.URL, "http://")
	rt.record429(host, "120")

	ctx, cancel := context.WithDeadline(context.Background(), clock.now().Add(30*time.Second))
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := rt.RoundTrip(req)

	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want *RateLimitedError", err)
	}
	if *hits != 0 || len(clock.slept) != 0 {
		t.Errorf("sent %d request(s) and slept %v; should fail without either", *hits, clock.slept)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"", 0, false},
		{"garbage", 0, false},
		{"0", time.Second, true},
		{"-5", time.Second, true},
		{"12", 12 * time.Second, true},
		{now.Add(-time.Minute).Format(http.TimeFormat), time.Second, true},
		{"999999", rateLimitMaxRetryAfter, true},
	} {
		d, src := parseRetryAfter(tc.in, now)
		if (src != "") != tc.ok || (tc.ok && d != tc.want) {
			t.Errorf("parseRetryAfter(%q) = %v, %q; want %v, ok=%v", tc.in, d, src, tc.want, tc.ok)
		}
	}
}

// Every 429 must appear in the log, including ones that join a pause already
// running — otherwise the log shows more retries than 429s.
func TestRateLimit_logsEvery429(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	clock := newFakeClock()
	rt := newTestRateLimit(clock)
	rt.record429("itch.io", "30")
	rt.record429("itch.io", "5") // inside the 30s pause

	if n := strings.Count(buf.String(), "ratelimit: HTTP 429 from itch.io"); n != 2 {
		t.Errorf("logged %d 429 line(s), want 2:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "already paused for another 30s") {
		t.Errorf("second 429 should report the running pause:\n%s", buf.String())
	}
}

func TestLoggablePath(t *testing.T) {
	got := loggablePath("/virtual-aquarium/download/eyJpZCI6Mjg5NTY1MCwiZXhwaXJlcyI6MTc5MDI1NzYyNn0=.Y3cLM")
	if strings.Contains(got, "MTc5MDI1") || !strings.HasPrefix(got, "/virtual-aquarium/download/eyJpZC") {
		t.Errorf("loggablePath = %q", got)
	}
	if p := "/games/tag-gameboy-rom.xml"; loggablePath(p) != p {
		t.Errorf("ordinary path changed: %q", loggablePath(p))
	}
}
