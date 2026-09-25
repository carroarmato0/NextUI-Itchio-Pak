package itchio

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	// rateLimitBaseDelay is the first cooldown after a 429 with no Retry-After;
	// each further consecutive 429 from the same host doubles it.
	rateLimitBaseDelay = 2 * time.Second
	// rateLimitMaxBackoff caps the computed (not server-requested) cooldown.
	rateLimitMaxBackoff = 60 * time.Second
	// rateLimitMaxRetryAfter caps a server-requested cooldown, so a malformed
	// or hostile header cannot stall the app indefinitely.
	rateLimitMaxRetryAfter = 5 * time.Minute
	// rateLimitMaxRetries is how often the transport itself retries a 429 on
	// an idempotent request before handing the 429 back to the caller.
	rateLimitMaxRetries = 2
)

// RateLimitedError is returned when a request cannot be sent before its
// deadline because the host asked us to slow down.
type RateLimitedError struct {
	Host  string
	Until time.Time
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limited by %s for another %s", e.Host, time.Until(e.Until).Round(time.Second))
}

// hostCooldown is the shared back-off state for one host.
type hostCooldown struct {
	notBefore time.Time // no request to the host may start before this
	strikes   int       // consecutive 429s, drives the exponential back-off
}

// rateLimitTransport makes the whole client back off when a host answers
// HTTP 429. The cooldown is per host and shared by every request through the
// client, so parallel feed fetches, page scrapes, the update checker and API
// calls all pause together instead of each retrying on its own schedule.
type rateLimitTransport struct {
	wrapped http.RoundTripper

	mu    sync.Mutex
	hosts map[string]*hostCooldown

	now   func() time.Time                                 // replaced in tests
	sleep func(ctx context.Context, d time.Duration) error // replaced in tests
}

func newRateLimitTransport(wrapped http.RoundTripper) *rateLimitTransport {
	return &rateLimitTransport{
		wrapped: wrapped,
		hosts:   make(map[string]*hostCooldown),
		now:     time.Now,
		sleep:   sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	// Only requests without a body can be replayed safely.
	idempotent := (req.Method == http.MethodGet || req.Method == http.MethodHead) && req.Body == nil
	for attempt := 0; ; attempt++ {
		if err := t.waitTurn(req.Context(), host); err != nil {
			return nil, err
		}
		resp, err := t.wrapped.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			t.recordOK(host)
			return resp, nil
		}
		t.record429(host, resp.Header.Get("Retry-After"))
		if !idempotent || attempt >= rateLimitMaxRetries {
			return resp, nil
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		logger.Info("ratelimit: retrying %s %s (%d/%d)", req.Method, loggablePath(req.URL.Path), attempt+1, rateLimitMaxRetries)
	}
}

// waitTurn blocks until host's cooldown has passed. If the request's deadline
// would expire first, it fails immediately rather than sleeping for nothing.
func (t *rateLimitTransport) waitTurn(ctx context.Context, host string) error {
	t.mu.Lock()
	var until time.Time
	if hc := t.hosts[host]; hc != nil {
		until = hc.notBefore
	}
	t.mu.Unlock()

	wait := until.Sub(t.now())
	if wait <= 0 {
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(until) {
		logger.Warn("ratelimit: %s cooling down for %s, past this request's deadline — not sent", host, wait.Round(time.Second))
		return &RateLimitedError{Host: host, Until: until}
	}
	logger.Debug("ratelimit: waiting %s for %s cooldown", wait.Round(time.Millisecond), host)
	return t.sleep(ctx, wait)
}

func (t *rateLimitTransport) recordOK(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if hc := t.hosts[host]; hc != nil && hc.strikes > 0 {
		logger.Info("ratelimit: %s answering again after %d rate-limited response(s)", host, hc.strikes)
		hc.strikes = 0
	}
}

func (t *rateLimitTransport) record429(host, retryAfter string) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	hc := t.hosts[host]
	if hc == nil {
		hc = &hostCooldown{}
		t.hosts[host] = hc
	}
	// 429s to requests that were already in flight when the cooldown began
	// belong to the same episode; only a 429 after it counts as a new strike.
	if !now.Before(hc.notBefore) || hc.strikes == 0 {
		hc.strikes++
	}

	delay, source := parseRetryAfter(retryAfter, now)
	if source == "" {
		delay = rateLimitBaseDelay << (hc.strikes - 1)
		if delay > rateLimitMaxBackoff || delay <= 0 {
			delay = rateLimitMaxBackoff
		}
		// Up to +20% jitter so parallel requests do not all return at once.
		delay += time.Duration(rand.Int64N(int64(delay)/5 + 1))
		source = fmt.Sprintf("backoff, strike %d", hc.strikes)
	}

	until := now.Add(delay)
	if until.After(hc.notBefore) {
		hc.notBefore = until
		logger.Warn("ratelimit: HTTP 429 from %s — pausing all requests to it for %s (%s)", host, delay.Round(time.Millisecond), source)
		return
	}
	// Every 429 is logged, so the count in the log matches the retries: this
	// one arrived while a longer pause was already running, which it joins.
	logger.Warn("ratelimit: HTTP 429 from %s — already paused for another %s (%s)", host, hc.notBefore.Sub(now).Round(time.Millisecond), source)
}

// parseRetryAfter reads a Retry-After header in either of its forms (delay in
// seconds, or an HTTP-date). source is "" when the header is absent or unusable.
func parseRetryAfter(v string, now time.Time) (time.Duration, string) {
	if v == "" {
		return 0, ""
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(v); err == nil {
		d = at.Sub(now)
	} else {
		logger.Debug("ratelimit: ignoring unparseable Retry-After %q", v)
		return 0, ""
	}
	// "Retry-After: 0" would otherwise let the retry go out at once.
	if d < time.Second {
		d = time.Second
	}
	if d > rateLimitMaxRetryAfter {
		d = rateLimitMaxRetryAfter
	}
	return d, "Retry-After " + v
}

// loggablePath shortens path segments long enough to be tokens — the web
// download flow's /download/{signed key} and similar — so a retry log line
// never records a download credential.
func loggablePath(p string) string {
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		if len(seg) > 32 {
			segs[i] = seg[:6] + "…"
		}
	}
	return strings.Join(segs, "/")
}
