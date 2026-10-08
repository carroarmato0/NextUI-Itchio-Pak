# Connectivity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the app know when it is offline, say so in plain words, pause background network work, resume on its own when the connection returns, and never lose an installed ROM to a dropped download.

**Architecture:** A new headless package `internal/netstate` classifies request errors and holds one app-wide Online/Offline state, fed by a `RoundTripper` wrapper in the itch.io client's transport chain. While offline, a monitor goroutine checks the local default route and probes itch.io with back-off; on reconnect it runs registered callbacks. A new package `internal/partfile` gives every download a hidden, journalled partial file that is renamed into place only when complete, and cleans up leftovers at startup.

**Tech Stack:** Go 1.27, SDL2 via go-sdl2 (UI only), `net/http`, `httptest` for tests.

**Spec:** `docs/superpowers/specs/2026-09-26-connectivity-design.md`

## Global Constraints

- Work on branch `feature/1.1-connectivity`, created from `release/1.1.0` in worktree `.worktrees/1.1-connectivity`; it merges back into `release/1.1.0` before `feature/1.1-app-updates` starts.
- `internal/netstate` and `internal/partfile` are headless-safe: no SDL imports, no build tags.
- Every new or modified code path logs through `internal/logger` (transitions, probes, file removals, aborts). No `fmt.Println`/`log.Printf`.
- No colour literals in drawing code; colours come from `internal/theme` (`scripts/no-color-literals.sh` enforces this).
- Logged error detail must never contain a request URL: signed CDN URLs carry credentials. Strip `*url.Error` to its `Op` and `Err`.
- No request is made purely to test the connection while online. The only extra request is the reconnect probe, and only while offline.
- Tests use `httptest` and temp dirs; no live network.
- Partial files are named `.<name>.itchio-part` in the destination folder; the app deletes only files with that suffix.
- Run the full suite with `./scripts/test.sh` (containerised). CI does not run the UI tests; a green CI check is not evidence.
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.

## Review Focus

1. **Wi-Fi switched off while browsing** — every visible cover and the inventory run must stop making requests, not retry per frame or per game. Pinned by Task 8 (`TestUpdateService_abortsOnFirstOfflineFailure`) and Task 9 (`TestImageCache_noFetchWhileOffline`).
2. **Wi-Fi dropping during a game update** — the old ROM must survive byte-for-byte and no partial file may remain. Pinned by Task 7 (`TestStreamToFile_failureKeepsExistingFile`).
3. **Power lost mid-download** — the hidden partial file must be gone after the next launch, including one written to a folder the user chose by hand. Pinned by Task 6 (`TestRecover_deletesJournalledLeftovers`).
4. **User presses Back during a download or sign-in** — a cancelled request must not flip the app to Offline. Pinned by Task 1 (`TestClassify` "canceled" case) and Task 2 (`TestTracker_canceledLeavesState`).
5. **itch.io returns 503 or 404** — the network works, so the state must be Online, and the message must not say "No Wi-Fi". Pinned by Task 2 (`TestTracker_httpResponseIsOnline`) and Task 4 (`TestDescribe` "5xx" case).

---

### Task 0: Branch and worktree

**Files:** none.

- [ ] **Step 1: Create the worktree**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
git worktree add -b feature/1.1-connectivity .worktrees/1.1-connectivity release/1.1.0
cd .worktrees/1.1-connectivity
git checkout feature/1.1-app-updates -- docs/superpowers/specs/2026-09-26-connectivity-design.md docs/superpowers/plans/2026-09-26-connectivity.md
git commit -m "docs: connectivity spec and plan

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

All following paths are relative to `.worktrees/1.1-connectivity`.

---

### Task 1: Error classification

**Files:**
- Create: `internal/netstate/classify.go`
- Test: `internal/netstate/classify_test.go`

**Interfaces:**
- Produces:
  - `type Reason int` with constants `ReasonNone, ReasonNoNetwork, ReasonDNS, ReasonUnreachable, ReasonClock, ReasonIntercepted, ReasonCanceled, ReasonOther`; `func (Reason) String() string`; `func (Reason) Offline() bool`.
  - `func Classify(err error) Reason`
  - `type StatusError struct { What string; Code int }` with `Error() string`.
  - package vars `buildDate string` (set by `-ldflags`), `now = time.Now` (overridable in tests).
  - `func detailOf(err error) string` — error text with any request URL stripped.

- [ ] **Step 1: Write the failing test**

```go
package netstate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }

	expired := &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}
	cases := []struct {
		name string
		err  error
		want Reason
	}{
		{"nil", nil, ReasonNone},
		{"dns", &url.Error{Op: "Get", URL: "https://itch.io", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}, ReasonDNS},
		{"net unreachable", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}}, ReasonNoNetwork},
		{"host unreachable", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}, ReasonNoNetwork},
		{"refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, ReasonUnreachable},
		{"reset", &net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, ReasonUnreachable},
		{"eof", &url.Error{Op: "Get", Err: io.EOF}, ReasonUnreachable},
		{"unexpected eof", fmt.Errorf("read stream: %w", io.ErrUnexpectedEOF), ReasonUnreachable},
		{"deadline", fmt.Errorf("no data: %w", os.ErrDeadlineExceeded), ReasonUnreachable},
		{"expired, plausible clock", expired, ReasonIntercepted},
		{"unknown authority", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, ReasonIntercepted},
		{"hostname", x509.HostnameError{Host: "itch.io"}, ReasonIntercepted},
		{"canceled", &url.Error{Op: "Get", Err: context.Canceled}, ReasonCanceled},
		{"status", &StatusError{What: "feed", Code: 503}, ReasonOther},
		{"plain", errors.New("not a valid ZIP"), ReasonOther},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("%s: Classify = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassify_expiredWithClockBeforeBuildIsClock(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	err := &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}
	if got := Classify(err); got != ReasonClock {
		t.Fatalf("Classify = %v, want Clock", got)
	}
}

func TestClassify_emptyBuildDateUsesFloor(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = ""
	now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	err := x509.CertificateInvalidError{Reason: x509.Expired}
	if got := Classify(err); got != ReasonClock {
		t.Fatalf("Classify = %v, want Clock (before the built-in floor)", got)
	}
}

// Real errors from the standard library, not hand-built ones.
func TestClassify_realRefusedAndTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = http.Get("http://" + addr + "/")
	if got := Classify(err); got != ReasonUnreachable {
		t.Errorf("refused: Classify(%v) = %v, want Unreachable", err, got)
	}

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	c := &http.Client{Timeout: 50 * time.Millisecond}
	_, err = c.Get(srv.URL)
	if got := Classify(err); got != ReasonUnreachable {
		t.Errorf("timeout: Classify(%v) = %v, want Unreachable", err, got)
	}
}

func TestReasonOffline(t *testing.T) {
	for _, r := range []Reason{ReasonNoNetwork, ReasonDNS, ReasonUnreachable, ReasonClock, ReasonIntercepted} {
		if !r.Offline() {
			t.Errorf("%v.Offline() = false", r)
		}
	}
	for _, r := range []Reason{ReasonNone, ReasonCanceled, ReasonOther} {
		if r.Offline() {
			t.Errorf("%v.Offline() = true", r)
		}
	}
}

func TestDetailOf_stripsURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://cdn.example/f.gb?sig=SECRET", Err: io.ErrUnexpectedEOF}
	if got := detailOf(err); got != "Get: unexpected EOF" {
		t.Fatalf("detailOf = %q", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags headless ./internal/netstate/ -run 'TestClassify|TestReasonOffline|TestDetailOf' -v`
Expected: FAIL — package does not compile (`undefined: Classify`).

- [ ] **Step 3: Write the implementation**

```go
// Package netstate knows whether the app can reach the network. It classifies
// request errors and keeps one app-wide Online/Offline state that every HTTP
// client feeds. It has no SDL dependency, so it is fully unit-tested.
package netstate

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"time"
)

// Reason says why a request failed.
type Reason int

const (
	ReasonNone        Reason = iota // no error
	ReasonNoNetwork                 // no route: Wi-Fi off or not associated
	ReasonDNS                       // name lookup failed
	ReasonUnreachable               // refused, reset, timed out, cut off
	ReasonClock                     // certificate "expired" because the device clock is wrong
	ReasonIntercepted               // a network in the way: captive portal, proxy
	ReasonCanceled                  // the user gave up (Back); not a network problem
	ReasonOther                     // anything else, including HTTP statuses
)

func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "none"
	case ReasonNoNetwork:
		return "no-network"
	case ReasonDNS:
		return "dns"
	case ReasonUnreachable:
		return "unreachable"
	case ReasonClock:
		return "clock"
	case ReasonIntercepted:
		return "intercepted"
	case ReasonCanceled:
		return "canceled"
	default:
		return "other"
	}
}

// Offline reports whether r means the network, not the server, is the problem.
func (r Reason) Offline() bool {
	switch r {
	case ReasonNoNetwork, ReasonDNS, ReasonUnreachable, ReasonClock, ReasonIntercepted:
		return true
	}
	return false
}

// StatusError is an HTTP response the caller did not want. The network worked.
type StatusError struct {
	What string
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.What, e.Code) }

// buildDate is the commit date of this build, set at build time:
//
//	-X github.com/carroarmato0/nextui-itchio-pak/internal/netstate.buildDate=2026-09-26T10:00:00+02:00
var buildDate = ""

// clockFloor is used when buildDate is missing (dev builds). A device whose
// clock reads earlier than the build it is running has certainly lost the date.
var clockFloor = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

var now = time.Now

func earliestPlausible() time.Time {
	if t, err := time.Parse(time.RFC3339, buildDate); err == nil && t.After(clockFloor) {
		return t
	}
	return clockFloor
}

// Classify says why err happened. The order matters: a cancelled request can
// wrap a network error, and the user pressing Back must win.
func Classify(err error) Reason {
	if err == nil {
		return ReasonNone
	}
	if errors.Is(err, context.Canceled) {
		return ReasonCanceled
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		// x509 reports "not yet valid" as Expired too. Only blame the clock
		// when it provably is wrong; otherwise something is presenting a bad
		// certificate, which is what captive portals do.
		if now().Before(earliestPlausible()) {
			return ReasonClock
		}
		return ReasonIntercepted
	}
	var unknownCA x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &unknownCA) || errors.As(err, &hostname) {
		return ReasonIntercepted
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return ReasonDNS
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return ReasonNoNetwork
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ReasonUnreachable
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ReasonUnreachable
	}
	return ReasonOther
}

// detailOf is err's text for the log, without the request URL: the text of a
// failed request quotes the whole URL, and signed CDN URLs carry credentials.
func detailOf(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags headless ./internal/netstate/ -run 'TestClassify|TestReasonOffline|TestDetailOf' -v`
Expected: PASS for all.

- [ ] **Step 5: Commit**

```bash
git add internal/netstate/classify.go internal/netstate/classify_test.go
git commit -m "netstate: classify request errors by cause

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: App-wide state

**Files:**
- Create: `internal/netstate/state.go`
- Test: `internal/netstate/state_test.go`

**Interfaces:**
- Consumes: `Classify`, `Reason.Offline`, `detailOf`, `now` (Task 1).
- Produces:
  - `type Status int` (`StatusUnknown, StatusOnline, StatusOffline`), `type State struct { Status Status; Reason Reason; Since time.Time; Detail string }`.
  - `func Report(err error)` — nil means a response arrived (Online).
  - `func Current() State`, `func Offline() bool`.
  - `func SetNotify(fn func())` — called after every transition (UI wake-up).
  - `func OnReconnect(fn func())` — run once per Offline→Online transition, in registration order.
  - `func SetForTest(s State)` and `func ResetForTest()`.
  - unexported: `std *tracker`, `(*tracker).set(next State)`, `(*tracker).subscribe(fn func(State))`, `(*tracker).hasRoute func() bool`.

- [ ] **Step 1: Write the failing test**

```go
package netstate

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
)

func dnsErr() error { return &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true} }

func TestTracker_transitions(t *testing.T) {
	tr := newTracker()
	if tr.current().Status != StatusUnknown {
		t.Fatal("initial state not Unknown")
	}
	notified := 0
	tr.notify = func() { notified++ }
	tr.report(dnsErr())
	if st := tr.current(); st.Status != StatusOffline || st.Reason != ReasonDNS {
		t.Fatalf("after DNS error: %+v", st)
	}
	tr.report(dnsErr()) // same state: no second notification
	tr.report(nil)
	if tr.current().Status != StatusOnline {
		t.Fatal("a response did not bring it online")
	}
	if notified != 2 {
		t.Fatalf("notified %d times, want 2", notified)
	}
}

func TestTracker_httpResponseIsOnline(t *testing.T) {
	tr := newTracker()
	tr.report(dnsErr())
	tr.report(nil) // any HTTP response, 503 included, is reported as nil by the transport
	if tr.current().Status != StatusOnline {
		t.Fatal("HTTP response did not mean Online")
	}
	tr.report(&StatusError{What: "feed", Code: 503})
	if tr.current().Status != StatusOnline {
		t.Fatal("a StatusError changed the state")
	}
}

func TestTracker_canceledLeavesState(t *testing.T) {
	tr := newTracker()
	tr.report(nil)
	tr.report(context.Canceled)
	tr.report(errors.New("not a valid ZIP"))
	if tr.current().Status != StatusOnline {
		t.Fatalf("state changed to %+v", tr.current())
	}
}

func TestTracker_noRouteBecomesNoNetwork(t *testing.T) {
	tr := newTracker()
	tr.hasRoute = func() bool { return false }
	tr.report(dnsErr())
	if r := tr.current().Reason; r != ReasonNoNetwork {
		t.Fatalf("reason = %v, want NoNetwork", r)
	}
}

func TestTracker_reconnectRunsOnceInOrder(t *testing.T) {
	tr := newTracker()
	var order []int
	tr.onReconnect(func() { order = append(order, 1) })
	tr.onReconnect(func() { order = append(order, 2) })
	tr.report(nil) // Unknown -> Online is not a reconnect
	tr.report(dnsErr())
	tr.report(nil)
	tr.report(nil)
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("reconnect callbacks ran %v, want [1 2]", order)
	}
}

func TestTracker_detailHasNoURL(t *testing.T) {
	tr := newTracker()
	tr.report(&url.Error{Op: "Get", URL: "https://cdn/x?sig=S", Err: dnsErr()})
	if d := tr.current().Detail; d != "Get: lookup itch.io: no such host" {
		t.Fatalf("detail = %q", d)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags headless ./internal/netstate/ -run TestTracker -v`
Expected: FAIL — `undefined: newTracker`.

- [ ] **Step 3: Write the implementation**

```go
package netstate

import (
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Status is the app's view of the connection.
type Status int

const (
	StatusUnknown Status = iota // nothing has been tried yet
	StatusOnline
	StatusOffline
)

// State is the connection as last observed.
type State struct {
	Status Status
	Reason Reason    // why, when Offline
	Since  time.Time // when this state began
	Detail string    // the error that caused it, URL stripped; for the log
}

func (s State) label() string {
	switch s.Status {
	case StatusOnline:
		return "online"
	case StatusOffline:
		return "offline(" + s.Reason.String() + ")"
	default:
		return "unknown"
	}
}

type tracker struct {
	mu        sync.Mutex
	st        State
	notify    func()
	listeners []func(State)
	reconnect []func()
	// hasRoute turns an offline-class error into NoNetwork when the device
	// has no default route. The monitor installs the real check.
	hasRoute func() bool
}

func newTracker() *tracker { return &tracker{hasRoute: func() bool { return true }} }

var std = newTracker()

func (t *tracker) current() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.st
}

func (t *tracker) report(err error) {
	r := Classify(err)
	switch {
	case r == ReasonNone:
		t.set(State{Status: StatusOnline})
	case r.Offline():
		t.mu.Lock()
		hasRoute := t.hasRoute
		t.mu.Unlock()
		if !hasRoute() {
			r = ReasonNoNetwork
		}
		t.set(State{Status: StatusOffline, Reason: r, Detail: detailOf(err)})
	}
	// Canceled and Other say nothing about the network.
}

func (t *tracker) set(next State) {
	t.mu.Lock()
	prev := t.st
	if prev.Status == next.Status && prev.Reason == next.Reason {
		t.mu.Unlock()
		return
	}
	next.Since = now()
	t.st = next
	notify := t.notify
	listeners := append([]func(State){}, t.listeners...)
	var reconnect []func()
	if prev.Status == StatusOffline && next.Status == StatusOnline {
		reconnect = append(reconnect, t.reconnect...)
	}
	t.mu.Unlock()

	if next.Detail != "" {
		logger.Info("netstate: %s -> %s (%s)", prev.label(), next.label(), next.Detail)
	} else {
		logger.Info("netstate: %s -> %s", prev.label(), next.label())
	}
	for _, l := range listeners {
		l(next)
	}
	if len(reconnect) > 0 {
		logger.Info("netstate: reconnected, running %d deferred task(s)", len(reconnect))
	}
	for _, fn := range reconnect {
		fn()
	}
	if notify != nil {
		notify()
	}
}

func (t *tracker) subscribe(fn func(State)) {
	t.mu.Lock()
	t.listeners = append(t.listeners, fn)
	t.mu.Unlock()
}

func (t *tracker) onReconnect(fn func()) {
	t.mu.Lock()
	t.reconnect = append(t.reconnect, fn)
	t.mu.Unlock()
}

// Report records the outcome of a request: nil when a response arrived,
// whatever its status, or the error when none did.
func Report(err error) { std.report(err) }

// Current returns the connection state as last observed.
func Current() State { return std.current() }

// Offline reports whether the last observation was a network failure.
func Offline() bool { return std.current().Status == StatusOffline }

// SetNotify registers a callback run after every transition; the UI uses it to
// push an SDL wake-up event. Set it once at startup.
func SetNotify(fn func()) {
	std.mu.Lock()
	std.notify = fn
	std.mu.Unlock()
}

// OnReconnect registers work to run once each time the app comes back online.
// Callbacks run on the reporting goroutine, in registration order, and must
// not block: start a goroutine or set a flag.
func OnReconnect(fn func()) { std.onReconnect(fn) }

// SetForTest forces a state, for tests and offscreen scenes. No callbacks run.
func SetForTest(s State) {
	std.mu.Lock()
	std.st = s
	std.mu.Unlock()
}

// ResetForTest restores a fresh tracker.
func ResetForTest() { std = newTracker() }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags headless -race ./internal/netstate/ -v`
Expected: PASS for all tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/netstate/state.go internal/netstate/state_test.go
git commit -m "netstate: one app-wide online/offline state with reconnect callbacks

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Local route check and reconnect monitor

**Files:**
- Create: `internal/netstate/route.go`, `internal/netstate/monitor.go`
- Test: `internal/netstate/route_test.go`, `internal/netstate/monitor_test.go`

**Interfaces:**
- Consumes: `std`, `(*tracker).set`, `(*tracker).subscribe`, `State`, `Offline` (Task 2).
- Produces:
  - `func HasRoute(root string) bool` — `root` is `/` on a device, a temp dir in tests.
  - `func StartMonitor(root string, probe func() error)` — installs the route check into the tracker and, on every transition to Offline, runs the reconnect loop in a goroutine (one at a time).

- [ ] **Step 1: Write the failing tests**

`internal/netstate/route_test.go`:

```go
package netstate

import (
	"os"
	"path/filepath"
	"testing"
)

const routeHeader = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"

func fakeRoot(t *testing.T, route string, operstate map[string]string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "proc/net"), 0755)
	os.WriteFile(filepath.Join(root, "proc/net/route"), []byte(routeHeader+route), 0644)
	for iface, st := range operstate {
		d := filepath.Join(root, "sys/class/net", iface)
		os.MkdirAll(d, 0755)
		os.WriteFile(filepath.Join(d, "operstate"), []byte(st+"\n"), 0644)
	}
	return root
}

// Copied from the Brick with Wi-Fi on (2026-09-26).
const brickRoute = "wlan0\t00000000\t0132A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
	"wlan0\t0032A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n"

func TestHasRoute(t *testing.T) {
	if !HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})) {
		t.Error("default route on an up interface: want true")
	}
	if HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "down"})) {
		t.Error("interface down: want false")
	}
	if HasRoute(fakeRoot(t, "wlan0\t0032A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n", map[string]string{"wlan0": "up"})) {
		t.Error("no default route: want false")
	}
	if !HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "unknown"})) {
		t.Error("operstate unknown (some drivers): want true")
	}
	if !HasRoute(t.TempDir()) {
		t.Error("no /proc/net/route: cannot tell, must not claim offline")
	}
}
```

`internal/netstate/monitor_test.go`:

```go
package netstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitor_backoffAndRecovery(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})
	var elapsed time.Duration
	var probes []time.Duration
	m := &monitor{
		t: tr, root: root, poll: 5 * time.Second,
		backoffMin: 15 * time.Second, backoffMax: 5 * time.Minute,
		sleep: func(d time.Duration) { elapsed += d },
	}
	m.probe = func() error {
		probes = append(probes, elapsed)
		if len(probes) < 3 {
			err := dnsErr()
			tr.report(err)
			return err
		}
		tr.report(nil)
		return nil
	}
	tr.report(dnsErr())
	m.run()
	want := []time.Duration{15 * time.Second, 45 * time.Second, 105 * time.Second}
	if len(probes) != 3 {
		t.Fatalf("probes at %v, want %v", probes, want)
	}
	for i := range want {
		if probes[i] != want[i] {
			t.Fatalf("probes at %v, want %v", probes, want)
		}
	}
	if tr.current().Status != StatusOnline {
		t.Fatal("monitor stopped while still offline")
	}
}

func TestMonitor_noRouteMeansNoNetworkAndNoProbe(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "down"})
	operstate := filepath.Join(root, "sys/class/net/wlan0/operstate")
	polls := 0
	m := &monitor{
		t: tr, root: root, poll: 5 * time.Second,
		backoffMin: 15 * time.Second, backoffMax: 5 * time.Minute,
	}
	m.probe = func() error { tr.report(nil); return nil }
	m.sleep = func(time.Duration) {
		polls++
		if polls == 1 {
			return // first poll sees the interface down
		}
		if tr.current().Reason != ReasonNoNetwork {
			t.Errorf("after a poll with no route, reason = %v", tr.current().Reason)
		}
		os.WriteFile(operstate, []byte("up\n"), 0644) // Wi-Fi switched back on
	}
	tr.report(dnsErr())
	m.run()
	if polls != 2 {
		t.Fatalf("polls = %d: the route returning should trigger a probe on the next poll", polls)
	}
	if tr.current().Status != StatusOnline {
		t.Fatal("not online after the route came back")
	}
}

func TestMonitor_otherErrorKeepsTrying(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})
	n := 0
	m := &monitor{t: tr, root: root, poll: time.Second, backoffMin: time.Second, backoffMax: time.Second,
		sleep: func(time.Duration) {}}
	m.probe = func() error {
		n++
		if n == 1 {
			return errors.New("weird") // Other: state unchanged, keep going
		}
		tr.report(nil)
		return nil
	}
	tr.report(dnsErr())
	m.run()
	if n != 2 {
		t.Fatalf("probes = %d, want 2", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -tags headless ./internal/netstate/ -run 'TestHasRoute|TestMonitor' -v`
Expected: FAIL — `undefined: HasRoute`, `undefined: monitor`.

- [ ] **Step 3: Write the implementation**

`internal/netstate/route.go`:

```go
package netstate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HasRoute reports whether the device has a default route on an interface
// that is up. It reads two procfs/sysfs files and makes no request, so it can
// run every few seconds. When the files cannot be read it answers true: not
// knowing is no reason to tell the user they are offline.
func HasRoute(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "proc/net/route"))
	if err != nil {
		return true
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 4 || f[1] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x1 == 0 { // RTF_UP
			continue
		}
		st, err := os.ReadFile(filepath.Join(root, "sys/class/net", f[0], "operstate"))
		if err != nil {
			return true
		}
		switch strings.TrimSpace(string(st)) {
		case "up", "unknown":
			return true
		}
	}
	return false
}
```

`internal/netstate/monitor.go`:

```go
package netstate

import (
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

type monitor struct {
	t                      *tracker
	root                   string
	probe                  func() error
	poll                   time.Duration
	backoffMin, backoffMax time.Duration
	sleep                  func(time.Duration)
	running                atomic.Bool
}

// StartMonitor watches for the connection coming back. It is idle while
// online; each transition to Offline starts one reconnect loop. probe must
// make one cheap request through a client whose transport feeds this package.
func StartMonitor(root string, probe func() error) {
	m := &monitor{
		t: std, root: root, probe: probe,
		poll: 5 * time.Second, backoffMin: 15 * time.Second, backoffMax: 5 * time.Minute,
		sleep: time.Sleep,
	}
	std.mu.Lock()
	std.hasRoute = func() bool { return HasRoute(root) }
	std.mu.Unlock()
	std.subscribe(func(s State) {
		if s.Status == StatusOffline && m.running.CompareAndSwap(false, true) {
			go func() {
				defer m.running.Store(false)
				m.run()
			}()
		}
	})
	logger.Info("netstate: monitor ready (root=%s)", root)
}

// run polls the local route every m.poll while offline. With no route it
// records NoNetwork and waits. With a route it probes, first after
// backoffMin, then doubling up to backoffMax; a route that has just come
// back is probed at once. It returns as soon as the state is Online.
func (m *monitor) run() {
	logger.Info("netstate: reconnect loop started")
	backoff := m.backoffMin
	var waited time.Duration
	hadRoute := true
	for m.t.current().Status == StatusOffline {
		m.sleep(m.poll)
		if !HasRoute(m.root) {
			if m.t.current().Reason != ReasonNoNetwork {
				m.t.set(State{Status: StatusOffline, Reason: ReasonNoNetwork, Detail: "no default route"})
			}
			logger.Debug("netstate: no default route")
			hadRoute, backoff, waited = false, m.backoffMin, 0
			continue
		}
		if !hadRoute {
			logger.Info("netstate: default route is back, probing")
			hadRoute, waited = true, backoff
		}
		waited += m.poll
		if waited < backoff {
			continue
		}
		waited = 0
		err := m.probe()
		if err != nil {
			logger.Info("netstate: reconnect probe failed (%s), next in %v", detailOf(err), min(backoff*2, m.backoffMax))
		} else {
			logger.Info("netstate: reconnect probe answered")
		}
		backoff = min(backoff*2, m.backoffMax)
	}
	logger.Info("netstate: reconnect loop finished")
}
```

Note the probe-schedule arithmetic the test pins: `waited` accumulates 5 s per poll, so probes land at 15 s, then 15+30 = 45 s, then 45+60 = 105 s.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -tags headless -race ./internal/netstate/ -v`
Expected: PASS for all.

- [ ] **Step 5: Commit**

```bash
git add internal/netstate/route.go internal/netstate/monitor.go internal/netstate/route_test.go internal/netstate/monitor_test.go
git commit -m "netstate: detect the connection coming back without polling itch.io

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Plain-language messages

**Files:**
- Create: `internal/netstate/describe.go`
- Test: `internal/netstate/describe_test.go`

**Interfaces:**
- Consumes: `Classify`, `StatusError`, `Current` (Tasks 1–2).
- Produces: `type Message struct { Title, Hint string }`; `func Describe(err error, service string) (Message, bool)` — `false` when `err` is not a connection problem, so the caller keeps its own text.

- [ ] **Step 1: Write the failing test**

```go
package netstate

import (
	"errors"
	"testing"
	"time"
)

func TestDescribe(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }

	cases := []struct {
		name  string
		err   error
		title string
		ok    bool
	}{
		{"dns", dnsErr(), "Can't reach itch.io", true},
		{"5xx", &StatusError{What: "feed", Code: 503}, "itch.io is having problems", true},
		{"404", &StatusError{What: "feed", Code: 404}, "", false},
		{"own text", errors.New("not a valid ZIP"), "", false},
	}
	for _, c := range cases {
		m, ok := Describe(c.err, "itch.io")
		if ok != c.ok || m.Title != c.title {
			t.Errorf("%s: Describe = %+v, %v; want title %q, %v", c.name, m, ok, c.title, c.ok)
		}
		if ok && m.Hint == "" {
			t.Errorf("%s: empty hint", c.name)
		}
	}
}

func TestDescribe_noRouteSaysNoWiFi(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	SetForTest(State{Status: StatusOffline, Reason: ReasonNoNetwork})
	m, ok := Describe(dnsErr(), "itch.io")
	if !ok || m.Title != "No Wi-Fi connection" {
		t.Fatalf("Describe = %+v, %v", m, ok)
	}
}

func TestDescribe_serviceName(t *testing.T) {
	m, _ := Describe(dnsErr(), "GitHub")
	if m.Title != "Can't reach GitHub" {
		t.Fatalf("title = %q", m.Title)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags headless ./internal/netstate/ -run TestDescribe -v`
Expected: FAIL — `undefined: Describe`.

- [ ] **Step 3: Write the implementation**

```go
package netstate

import "errors"

// Message is what the user is told about a failed request.
type Message struct {
	Title string
	Hint  string
}

// Describe turns err into words for the screen. service names who could not be
// reached ("itch.io", "GitHub"). It returns false when err is not a connection
// problem: the caller's own text ("not a valid ZIP") is then the better message.
func Describe(err error, service string) (Message, bool) {
	r := Classify(err)
	if r.Offline() {
		// A DNS failure with no route is really "Wi-Fi is off"; the monitor
		// knows that, the error alone does not.
		if cur := Current(); cur.Status == StatusOffline && cur.Reason == ReasonNoNetwork {
			r = ReasonNoNetwork
		}
	}
	switch r {
	case ReasonNoNetwork:
		return Message{"No Wi-Fi connection", "Turn on Wi-Fi in the system settings."}, true
	case ReasonDNS, ReasonUnreachable:
		return Message{"Can't reach " + service, "Check your Wi-Fi connection, then try again."}, true
	case ReasonClock:
		return Message{"The date and time are wrong", "Secure connections fail until the clock is set."}, true
	case ReasonIntercepted:
		return Message{"This network is blocking the connection", "Public Wi-Fi may need you to sign in on a phone or computer first."}, true
	}
	var se *StatusError
	if errors.As(err, &se) && se.Code >= 500 {
		return Message{service + " is having problems", "Try again in a few minutes."}, true
	}
	return Message{}, false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags headless ./internal/netstate/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/netstate/describe.go internal/netstate/describe_test.go
git commit -m "netstate: plain-language messages for connection problems

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Feed the state from the itch.io client

**Files:**
- Create: `internal/netstate/transport.go`, `internal/netstate/transport_test.go`
- Modify: `internal/itchio/client.go` (`newHTTPClient`, new `Probe`), `internal/itchio/feed.go:183-194`, `internal/itchio/errors.go:5-7`
- Test: `internal/itchio/probe_test.go`

**Interfaces:**
- Consumes: `Report` (Task 2), `StatusError` (Task 1).
- Produces:
  - `func netstate.Transport(next http.RoundTripper) http.RoundTripper`.
  - `func (c *itchio.Client) Probe() error` — `HEAD <base>/`.
  - `fetchGamesFromURLOnce` returns `*netstate.StatusError` for non-200 statuses other than 403/404/410.

- [ ] **Step 1: Write the failing tests**

`internal/netstate/transport_test.go`:

```go
package netstate

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransport_feedsState(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &http.Client{Transport: Transport(http.DefaultTransport)}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	ln.Close()
	if _, err := c.Get("http://" + closed); err == nil {
		t.Fatal("expected an error from a closed port")
	}
	if !Offline() {
		t.Fatalf("after refused: %+v", Current())
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if Current().Status != StatusOnline {
		t.Fatalf("a 503 response should mean Online, got %+v", Current())
	}
}
```

`internal/itchio/probe_test.go`:

```go
package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func TestProbe_headsTheBaseURL(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
	}))
	defer srv.Close()
	if err := itchio.NewClientWithBase(srv.URL).Probe(); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodHead || path != "/" {
		t.Fatalf("probe sent %s %s, want HEAD /", method, path)
	}
	if netstate.Current().Status != netstate.StatusOnline {
		t.Fatal("the client's transport did not report to netstate")
	}
}

func TestFetchGames_serverErrorIsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := itchio.NewClientWithBase(srv.URL).FetchGames(1)
	m, ok := netstate.Describe(err, "itch.io")
	if !ok || m.Title != "itch.io is having problems" {
		t.Fatalf("Describe(%v) = %+v, %v", err, m, ok)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -tags headless ./internal/netstate/ -run TestTransport -v && go test -tags headless ./internal/itchio/ -run 'TestProbe|TestFetchGames_serverError' -v`
Expected: FAIL — `undefined: Transport`, `c.Probe undefined`.

If `TestFetchGames_serverErrorIsStatusError` hangs or reports a different error because `FetchGames` retries (see the loop above `fetchGamesFromURLOnce` in `feed.go:160-176`), keep the test and confirm the retry loop returns the last error unwrapped-compatible (`fmt.Errorf("…: %w", err)` or the error itself). Adjust only the test's expectation of timing, never the retry behaviour.

- [ ] **Step 3: Write the implementation**

`internal/netstate/transport.go`:

```go
package netstate

import "net/http"

type transport struct{ next http.RoundTripper }

// Transport wraps next so every request reports its outcome: any response,
// whatever its status, means the network works; an error is classified.
func Transport(next http.RoundTripper) http.RoundTripper { return transport{next} }

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	Report(err)
	return resp, err
}
```

`internal/itchio/client.go` — in `newHTTPClient`, wrap the rate-limit transport (errors surface before the 429 logic sees them, and a 429 is a response):

```go
		Transport: &uaTransport{
			wrapped: netstate.Transport(newRateLimitTransport(&h2FallbackTransport{
				h2:      h2t,
				h1:      h1t,
				h1hosts: make(map[string]struct{}),
			})),
		},
```

Add the import `"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"`, and after `HTTPClient()`:

```go
// Probe makes the cheapest request that proves itch.io answers: HEAD on the
// site root, no body. Only the reconnect monitor calls it, and only offline.
func (c *Client) Probe() error {
	req, err := http.NewRequest(http.MethodHead, c.base+"/", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return withoutURL(err)
	}
	resp.Body.Close()
	logger.Debug("probe: HEAD %s/ -> %d", c.base, resp.StatusCode)
	return nil
}
```

(add `"github.com/carroarmato0/nextui-itchio-pak/internal/logger"` to the imports if `client.go` does not already import it).

`internal/itchio/feed.go:191-194` — replace:

```go
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: HTTP %d from %s", resp.StatusCode, url)
		return nil, fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}
```

with:

```go
	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: HTTP %d from %s", resp.StatusCode, url)
		return nil, &netstate.StatusError{What: "fetch feed", Code: resp.StatusCode}
	}
```

and at `feed.go:183-186` change the log text to `"feed: HTTP 403 from %s"` (Cloudflare no longer blocks the app). In `internal/itchio/errors.go`, update the comment and text, keeping the variable name so `feed.go:169` and `feed_test.go:431` still compile:

```go
// ErrCloudflareBlocked is returned when itch.io refuses a feed request with
// HTTP 403. The name predates itch.io lifting its Cloudflare block on the app.
var ErrCloudflareBlocked = errors.New("itch.io refused the request (HTTP 403)")
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -tags headless -race ./internal/netstate/ ./internal/itchio/`
Expected: PASS, including the existing `internal/itchio` suite.

- [ ] **Step 5: Commit**

```bash
git add internal/netstate/transport.go internal/netstate/transport_test.go internal/itchio/client.go internal/itchio/feed.go internal/itchio/errors.go internal/itchio/probe_test.go
git commit -m "itchio: every request reports to netstate; HEAD probe for reconnects

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Journalled partial files

**Files:**
- Create: `internal/partfile/partfile.go`
- Test: `internal/partfile/partfile_test.go`

**Interfaces:**
- Produces:
  - `const Suffix = ".itchio-part"`; `func PathFor(dest string) string`.
  - `func SetJournal(path string)` — `""` disables the journal (default).
  - `func Create(dest string) (*File, error)`; `type File struct` embedding `*os.File`; `func (*File) Commit() error`; `func (*File) Abort()`.
  - `func Recover() int` — deletes journalled leftovers, returns how many.
  - `func Sweep(dirs []string) int` — deletes suffixed files not in use, returns how many.

- [ ] **Step 1: Write the failing test**

```go
package partfile

import (
	"os"
	"path/filepath"
	"testing"
)

func reset(t *testing.T) string {
	t.Helper()
	mu.Lock()
	active = map[string]bool{}
	journal = ""
	mu.Unlock()
	j := filepath.Join(t.TempDir(), "partials.json")
	SetJournal(j)
	t.Cleanup(func() { SetJournal("") })
	return j
}

func TestPathFor(t *testing.T) {
	if got := PathFor("/r/Game Boy (GB)/Tobu.gb"); got != "/r/Game Boy (GB)/.Tobu.gb.itchio-part" {
		t.Fatalf("PathFor = %q", got)
	}
}

func TestCommit_replacesDestAtomically(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	f, err := Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("NEW"))
	if b, _ := os.ReadFile(dest); string(b) != "OLD" {
		t.Fatal("dest changed before Commit")
	}
	if err := f.Commit(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW" {
		t.Fatalf("dest = %q after Commit", b)
	}
	if _, err := os.Stat(PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left after Commit")
	}
}

func TestAbort_keepsDestAndRemovesPart(t *testing.T) {
	reset(t)
	dest := filepath.Join(t.TempDir(), "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	f, _ := Create(dest)
	f.Write([]byte("PARTIAL"))
	f.Abort()
	f.Abort() // idempotent
	if b, _ := os.ReadFile(dest); string(b) != "OLD" {
		t.Fatal("Abort touched dest")
	}
	if _, err := os.Stat(PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left after Abort")
	}
}

// A crash between Create and Commit leaves the file and its journal entry.
func TestRecover_deletesJournalledLeftovers(t *testing.T) {
	reset(t)
	chosen := filepath.Join(t.TempDir(), "picked by hand")
	os.MkdirAll(chosen, 0755)
	f, _ := Create(filepath.Join(chosen, "g.gb"))
	f.Write([]byte("PARTIAL"))
	f.File.Close() // the process dies here: no Commit, no Abort

	// Next launch: the in-process set is empty again.
	mu.Lock()
	active = map[string]bool{}
	mu.Unlock()
	if n := Recover(); n != 1 {
		t.Fatalf("Recover removed %d, want 1", n)
	}
	if _, err := os.Stat(PathFor(filepath.Join(chosen, "g.gb"))); !os.IsNotExist(err) {
		t.Fatal("leftover still there")
	}
	if n := Recover(); n != 0 {
		t.Fatalf("journal not cleared: second Recover removed %d", n)
	}
}

func TestRecover_corruptJournalIsHarmless(t *testing.T) {
	j := reset(t)
	os.WriteFile(j, []byte("{not json"), 0644)
	if n := Recover(); n != 0 {
		t.Fatalf("Recover = %d", n)
	}
}

func TestSweep_onlyOurStaleFiles(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	stale := filepath.Join(dir, ".old.gb"+Suffix)
	os.WriteFile(stale, []byte("x"), 0644)
	other := filepath.Join(dir, ".other.part")
	os.WriteFile(other, []byte("x"), 0644)
	tmp := filepath.Join(dir, "config.json.tmp")
	os.WriteFile(tmp, []byte("x"), 0644)
	running, _ := Create(filepath.Join(dir, "now.gb"))
	defer running.Abort()

	if n := Sweep([]string{dir, filepath.Join(dir, "missing")}); n != 1 {
		t.Fatalf("Sweep removed %d, want 1", n)
	}
	for _, keep := range []string{other, tmp, PathFor(filepath.Join(dir, "now.gb"))} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("Sweep removed %s", keep)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale partial file survived")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags headless ./internal/partfile/ -v`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the implementation**

```go
// Package partfile writes downloads to a hidden partial file beside their
// destination and renames it into place only when complete, so a dropped
// connection never damages the file already installed. Every partial file is
// recorded in a journal first, so one left behind by a crash or a power cut is
// deleted on the next launch.
package partfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Suffix marks files this app created. Nothing without it is ever deleted.
const Suffix = ".itchio-part"

var (
	mu      sync.Mutex
	journal string          // partials.json; "" disables the journal
	active  = map[string]bool{} // partial files being written by this process
)

// PathFor is the partial file for dest: hidden (dot-prefixed, so neither
// launcher lists it) and in the same folder, so the final rename never
// crosses filesystems.
func PathFor(dest string) string {
	dir, base := filepath.Split(dest)
	return filepath.Join(dir, "."+base+Suffix)
}

// SetJournal sets where partial files are recorded. Call once at startup.
func SetJournal(path string) {
	mu.Lock()
	journal = path
	mu.Unlock()
}

// saveLocked writes the active set as the journal. Caller holds mu.
func saveLocked() error {
	if journal == "" {
		return nil
	}
	paths := make([]string, 0, len(active))
	for p := range active {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	data, _ := json.Marshal(paths)
	tmp := journal + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, journal)
}

func forget(part string) {
	mu.Lock()
	delete(active, part)
	err := saveLocked()
	mu.Unlock()
	if err != nil {
		logger.Warn("partfile: journal update failed: %v", err)
	}
}

// File is a partial download. Write to it, then Commit or Abort exactly once;
// Abort after Commit is a no-op, so `defer f.Abort()` is safe.
type File struct {
	*os.File
	dest, part string
	done       bool
}

// Create journals and opens the partial file for dest.
func Create(dest string) (*File, error) {
	part := PathFor(dest)
	mu.Lock()
	active[part] = true
	jerr := saveLocked()
	mu.Unlock()
	if jerr != nil {
		// The sweep still finds it in default folders; not worth failing for.
		logger.Warn("partfile: could not journal %s: %v", part, jerr)
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		forget(part)
		return nil, err
	}
	logger.Debug("partfile: writing %s", part)
	return &File{File: f, dest: dest, part: part}, nil
}

// Commit closes the partial file and renames it over dest in one step.
func (f *File) Commit() error {
	if f.done {
		return nil
	}
	f.done = true
	if err := f.File.Close(); err != nil {
		os.Remove(f.part)
		forget(f.part)
		logger.Error("partfile: close %s: %v", f.part, err)
		return err
	}
	if err := os.Rename(f.part, f.dest); err != nil {
		os.Remove(f.part)
		forget(f.part)
		logger.Error("partfile: rename %s -> %s: %v", f.part, f.dest, err)
		return err
	}
	forget(f.part)
	logger.Debug("partfile: committed %s", f.dest)
	return nil
}

// Abort closes and deletes the partial file, leaving dest untouched.
func (f *File) Abort() {
	if f.done {
		return
	}
	f.done = true
	f.File.Close()
	if err := os.Remove(f.part); err != nil && !os.IsNotExist(err) {
		logger.Warn("partfile: remove %s: %v", f.part, err)
	}
	forget(f.part)
	logger.Info("partfile: discarded %s", f.part)
}

func removeLeftover(path, how string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if err := os.Remove(path); err != nil {
		logger.Warn("partfile: could not remove leftover %s: %v", path, err)
		return false
	}
	logger.Info("partfile: removed leftover %s (%d bytes, found by %s)", path, info.Size(), how)
	return true
}

// Recover deletes every journalled partial file that is not being written now,
// then rewrites the journal. Call once at startup, before any download.
func Recover() int {
	mu.Lock()
	defer mu.Unlock()
	if journal == "" {
		return 0
	}
	var paths []string
	if data, err := os.ReadFile(journal); err == nil {
		if err := json.Unmarshal(data, &paths); err != nil {
			logger.Warn("partfile: journal %s unreadable, relying on the sweep: %v", journal, err)
			paths = nil
		}
	}
	n := 0
	for _, p := range paths {
		if strings.HasSuffix(p, Suffix) && !active[p] && removeLeftover(p, "journal") {
			n++
		}
	}
	if err := saveLocked(); err != nil {
		logger.Warn("partfile: journal rewrite failed: %v", err)
	}
	return n
}

// Sweep deletes, non-recursively, suffixed files in dirs that this process is
// not writing. It catches leftovers whose journal was lost.
func Sweep(dirs []string) int {
	n := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, Suffix) {
				continue
			}
			p := filepath.Join(dir, name)
			mu.Lock()
			busy := active[p]
			mu.Unlock()
			if !busy && removeLeftover(p, "sweep") {
				n++
			}
		}
	}
	logger.Debug("partfile: sweep of %d folder(s) removed %d file(s)", len(dirs), n)
	return n
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags headless -race ./internal/partfile/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/partfile/
git commit -m "partfile: journalled partial downloads, recovered after a crash

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Downloads that cannot break an installed game

**Files:**
- Modify: `internal/itchio/download.go:251-313` (`streamToFile`)
- Test: `internal/itchio/stream_test.go`

**Interfaces:**
- Consumes: `partfile.Create`, `(*File).Commit`, `(*File).Abort` (Task 6); `netstate.Report`, `netstate.StatusError` (Tasks 1–2).
- Produces: package var `streamIdleTimeout = 30 * time.Second` (tests lower it via `export_test.go`); `streamToFile` behaviour as specified. Public signatures (`DownloadURL`, `DownloadFree`, `DownloadAuthUpload`) are unchanged.

- [ ] **Step 1: Write the failing test**

Create `internal/itchio/export_stream_test.go` (package `itchio`) to expose the timeout:

```go
package itchio

import "time"

// SetStreamIdleTimeoutForTest shortens the download idle timeout.
func SetStreamIdleTimeoutForTest(d time.Duration) func() {
	old := streamIdleTimeout
	streamIdleTimeout = d
	return func() { streamIdleTimeout = old }
}
```

Create `internal/itchio/stream_test.go`:

```go
package itchio_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// truncatingServer answers every request with a response that promises
// `promise` bytes, sends `send`, then closes the socket: Wi-Fi dropping
// mid-download, at the TCP level, with no net/http server logic in the way.
func truncatingServer(t *testing.T, promise int, send []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				bufio.NewReader(c).ReadString('\n') // request line; the rest is ignored
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", promise)
				c.Write(send)
			}(c)
		}
	}()
	return "http://" + ln.Addr().String() + "/f.gb"
}

func TestStreamToFile_failureKeepsExistingFile(t *testing.T) {
	srvURL := truncatingServer(t, 1000, []byte("NEWDATA"))
	dest := filepath.Join(t.TempDir(), "game.gb")
	os.WriteFile(dest, []byte("WORKING ROM"), 0644)

	err := itchio.NewClient().DownloadURL(srvURL, dest, nil)
	if err == nil {
		t.Fatal("expected an error for a truncated download")
	}
	if b, _ := os.ReadFile(dest); string(b) != "WORKING ROM" {
		t.Fatalf("installed ROM changed to %q", b)
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind")
	}
}

func TestStreamToFile_shortBodyRejected(t *testing.T) {
	// Content-Length says 10, the handler writes 4 and returns cleanly: net/http
	// reports this as unexpected EOF; either way it must not be committed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.Write([]byte("abcd"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "g.gb")
	if err := itchio.NewClient().DownloadURL(srv.URL, dest, nil); err == nil {
		t.Fatal("short body accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("short download was committed")
	}
}

func TestStreamToFile_idleTimeout(t *testing.T) {
	defer itchio.SetStreamIdleTimeoutForTest(100 * time.Millisecond)()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	dest := filepath.Join(t.TempDir(), "g.gb")
	start := time.Now()
	err := itchio.NewClient().DownloadURL(srv.URL, dest, nil)
	if err == nil {
		t.Fatal("stalled download succeeded")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("idle timeout did not fire (took %v)", time.Since(start))
	}
}

func TestStreamToFile_successReplacesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NEW ROM"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	if err := itchio.NewClient().DownloadURL(srv.URL, dest, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW ROM" {
		t.Fatalf("dest = %q", b)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -tags headless ./internal/itchio/ -run TestStreamToFile -v`
Expected: `failureKeepsExistingFile` FAILS (installed ROM becomes `NEWDATA`), `idleTimeout` FAILS or hangs past 5 s, the build fails on `streamIdleTimeout` until Step 3.

- [ ] **Step 3: Write the implementation**

Replace `streamToFile` in `internal/itchio/download.go` (lines 251–313) with the following, and add imports `"context"`, `"os"` (if missing), `"sync/atomic"`, `"time"`, `".../internal/netstate"`, `".../internal/partfile"`:

```go
// streamIdleTimeout aborts a download that receives nothing for this long.
// There is deliberately no overall timeout: a large file on slow Wi-Fi is fine
// as long as bytes keep arriving.
var streamIdleTimeout = 30 * time.Second

// idleReader cancels the request when no bytes arrive for d.
type idleReader struct {
	r     io.Reader
	d     time.Duration
	timer *time.Timer
	fired atomic.Bool
}

func newIdleReader(r io.Reader, d time.Duration, cancel func()) *idleReader {
	ir := &idleReader{r: r, d: d}
	ir.timer = time.AfterFunc(d, func() {
		ir.fired.Store(true)
		cancel()
	})
	return ir
}

func (ir *idleReader) Read(p []byte) (int, error) {
	n, err := ir.r.Read(p)
	if n > 0 {
		ir.timer.Reset(ir.d)
	}
	return n, err
}

func (c *Client) streamToFile(srcURL, dest string, progress func(int64, int64)) error {
	// c.http has a 30-second Timeout that covers the entire response body read —
	// fine for API calls but fatal for large file downloads. Create a per-call
	// client with no overall timeout that shares the same transport, so UA
	// injection, h2/h1 fallback, dial timeouts and netstate reporting apply.
	dlClient := &http.Client{
		Transport:     c.http.Transport,
		Jar:           c.http.Jar,
		CheckRedirect: c.http.CheckRedirect,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return fmt.Errorf("fetch file: %w", withoutURL(err))
	}
	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch file: %w", withoutURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("stream: HTTP %d fetching file", resp.StatusCode)
		return &netstate.StatusError{What: "file download", Code: resp.StatusCode}
	}

	// Log the destination and size but not the CDN source URL (may contain tokens).
	if resp.ContentLength >= 0 {
		logger.Info("stream: → %s (%d bytes)", dest, resp.ContentLength)
	} else {
		logger.Info("stream: → %s (unknown size)", dest)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// The installed file, if any, is not touched until the new one is complete.
	f, err := partfile.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer f.Abort() // no-op after Commit

	body := newIdleReader(resp.Body, streamIdleTimeout, cancel)
	defer body.timer.Stop()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				logger.Error("stream: write error after %d bytes: %v", downloaded, werr)
				return fmt.Errorf("write: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if body.fired.Load() {
				rerr = fmt.Errorf("no data for %v: %w", streamIdleTimeout, os.ErrDeadlineExceeded)
			}
			logger.Error("stream: read error after %d bytes: %v", downloaded, withoutURL(rerr))
			netstate.Report(rerr)
			return fmt.Errorf("read stream: %w", withoutURL(rerr))
		}
	}
	if total >= 0 && downloaded != total {
		err := fmt.Errorf("short download: got %d of %d bytes: %w", downloaded, total, io.ErrUnexpectedEOF)
		logger.Error("stream: %v", err)
		netstate.Report(err)
		return err
	}
	if err := f.Commit(); err != nil {
		return fmt.Errorf("finish download: %w", err)
	}
	logger.Info("stream: done, wrote %d bytes", downloaded)
	return nil
}
```

Check `download.go`'s existing imports: `filepath` and `io` are already used by the old body; keep them.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -tags headless -race ./internal/itchio/ -v 2>&1 | tail -40`
Expected: all `TestStreamToFile_*` PASS and the existing download tests still PASS. If an existing test asserted the old error text `file download status %d`, update its expectation to match `*netstate.StatusError` (`errors.As`), not the other way round.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/download.go internal/itchio/stream_test.go internal/itchio/export_stream_test.go
git commit -m "Downloads go to a partial file: a dropped connection no longer destroys the installed ROM

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Inventory checks pause while offline

**Files:**
- Modify: `internal/inventory/updater.go` (struct `UpdateService`, `runCheck`, new `RetryIfOwed`)
- Test: `internal/inventory/updater_offline_test.go`

**Interfaces:**
- Consumes: `netstate.Classify`, `Reason.Offline`, `netstate.Offline`, `netstate.ResetForTest` (Tasks 1–2).
- Produces: `func (s *UpdateService) RetryIfOwed()` — queues a check if the last one was abandoned for being offline.

- [ ] **Step 1: Write the failing test**

```go
package inventory_test

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// resetListener accepts connections and closes them at once, counting them:
// every request through it fails with a network error.
func resetListener(t *testing.T) (addr string, accepts *int32) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var n int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&n, 1)
			c.Close()
		}
	}()
	return ln.Addr().String(), &n
}

func TestUpdateService_abortsOnFirstOfflineFailure(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	addr, accepts := resetListener(t)
	dir := t.TempDir()
	inv := newInv()
	for _, name := range []string{"a", "b", "c"} {
		rom := filepath.Join(dir, name+".gb")
		os.WriteFile(rom, []byte("ROM"), 0644)
		inv.Add("http://"+addr+"/"+name, inventory.Entry{Title: name, IsFree: true},
			inventory.DownloadedFile{Filename: name + ".gb", DestPath: rom, DownloadedAt: time.Now()})
	}
	before := inv.LatestCheckedAt()

	client := itchio.NewClientWithBaseAndButler("http://"+addr, "http://"+addr)
	svc := inventory.NewUpdateService(inv, filepath.Join(dir, "inventory.json"), client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	// data.json for the first game fails; nothing else should be attempted.
	// The HTTP client may retry a reset connection once, so allow up to 2.
	if n := atomic.LoadInt32(accepts); n > 2 {
		t.Fatalf("%d connections made; the run should stop after the first failure", n)
	}
	if !inv.LatestCheckedAt().Equal(before) {
		t.Fatal("checked_at changed during an offline run")
	}
	if !netstate.Offline() {
		t.Fatal("netstate not offline after the failures")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags headless ./internal/inventory/ -run TestUpdateService_abortsOnFirstOfflineFailure -v`
Expected: FAIL — about 3–6 connections (one per game, plus cover-art repair attempts).

- [ ] **Step 3: Write the implementation**

In `internal/inventory/updater.go`, add to `UpdateService`:

```go
	// owed is set when a check was abandoned because the network was down, so
	// the reconnect handler knows to run one.
	owed atomic.Bool
```

(`sync/atomic` is already imported for `running`; import `".../internal/netstate"`.)

At the top of `runCheck`, after `s.inv.VerifyAndClean(s.inventoryPath)`:

```go
	if netstate.Offline() {
		s.owed.Store(true)
		logger.Info("update-svc: offline, check postponed until the connection is back")
		return
	}
```

In pass 1, replace the `switch` with:

```go
		d, err := s.client.FetchGameData(gameURL)
		switch {
		case isGameRemoved(err):
			s.inv.MarkRemoved(gameURL)
			logger.Warn("update-svc: game removed (404) %s", gameURL)
			continue
		case netstate.Classify(err).Offline():
			s.owed.Store(true)
			logger.Warn("update-svc: network down (%v), abandoning this check", err)
			return
		case err != nil:
			logger.Warn("update-svc: transient error for %s: %v", gameURL, err)
			continue
		}
```

Move `s.repairCoverArt(gameURL)` to after the `switch` (so cover repair never runs for a game whose data could not be fetched), i.e. immediately before `games[gameURL] = d`.

In pass 3, at the top of the `for gameURL, d := range games` loop:

```go
		if netstate.Offline() {
			s.owed.Store(true)
			logger.Warn("update-svc: network went down mid-check, abandoning it")
			return
		}
```

Add:

```go
// RetryIfOwed queues a check if the last one was abandoned while offline.
// Registered with netstate.OnReconnect; it does not block.
func (s *UpdateService) RetryIfOwed() {
	if s.owed.Swap(false) {
		logger.Info("update-svc: connection back, running the postponed check")
		s.TriggerNow()
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -tags headless -race ./internal/inventory/ -v 2>&1 | tail -30`
Expected: the new test PASSES; every existing inventory test still PASSES (they run against live `httptest` servers, so `netstate` stays Online).

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/updater.go internal/inventory/updater_offline_test.go
git commit -m "Inventory check stops at the first network failure and runs again on reconnect

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Cover art waits for the connection

**Files:**
- Modify: `internal/renderer/image_cache.go` (`Get`, new `Resume`)
- Test: `internal/renderer/image_cache_offline_test.go`

**Interfaces:**
- Consumes: `netstate.Offline`, `netstate.SetForTest`, `netstate.ResetForTest`.
- Produces: `func (c *ImageCache) Resume()` — calls the notify callback so the screen redraws and re-requests covers.

`image_cache.go` is `//go:build !headless`; the test therefore runs in the non-headless pass (`go test ./internal/renderer/`), which `test.sh` must include. Step 4 checks that.

- [ ] **Step 1: Write the failing test**

```go
//go:build !headless

package renderer

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func TestImageCache_noFetchWhileOffline(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewImageCache(10, srv.Client())
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
	for i := 0; i < 5; i++ {
		if c.Get(nil, srv.URL+"/cover.png") != nil {
			t.Fatal("got a texture offline")
		}
	}
	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("%d requests made while offline", n)
	}

	notified := make(chan struct{}, 1)
	c.SetNotify(func() { notified <- struct{}{} })
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	c.Resume()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("Resume did not ask for a redraw")
	}
	c.Get(nil, srv.URL+"/cover.png")
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&hits) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("no fetch after coming back online")
	}
}
```

Before relying on `c.Get(nil, url)`, confirm in `image_cache.go` that `Get` does not dereference `r` on the path for an uncached URL (it only locks, checks maps and queues `fetchInBackground`). If it does, construct the test with the offscreen renderer used by `cmd/devshot` (`renderer.NewOffscreen`, see `internal/renderer/offscreen.go`) instead of `nil`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/renderer/ -run TestImageCache_noFetchWhileOffline -v`
Expected: FAIL — requests are made while offline (`hits` > 0), and `c.Resume undefined`.

- [ ] **Step 3: Write the implementation**

In `Get`, right after the `failed` check and before the `fetching` check:

```go
	if netstate.Offline() {
		// Every request would fail and be forgotten, and Get runs on every
		// redraw: offline, that was a doomed request per cover per frame.
		c.mu.Unlock()
		return nil
	}
```

Add the import `"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"` and:

```go
// Resume asks for a redraw once the connection is back, so the screen calls
// Get again and fetches the covers it skipped. Registered with
// netstate.OnReconnect.
func (c *ImageCache) Resume() {
	logger.Info("image cache: connection back, resuming cover fetches")
	if c.notify != nil {
		c.notify()
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/renderer/ -v 2>&1 | tail -20`
Expected: PASS. Then confirm `scripts/test.sh` runs the non-headless renderer tests: `grep -n "internal/renderer\|internal/ui" scripts/test.sh`. If only `./internal/ui/` has a non-headless pass (line ~118), add after it:

```sh
echo "==> go test ./internal/renderer (non-headless)"
go test ./internal/renderer/ || exit 1
```

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/image_cache.go internal/renderer/image_cache_offline_test.go scripts/test.sh
git commit -m "Cover art makes no requests while offline and resumes on reconnect

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Plain messages on every network screen

**Files:**
- Create: `internal/ui/net_problem.go` (no build tag), `internal/ui/net_problem_test.go`
- Modify: `internal/ui/screen_list.go:634-645`, `screen_detail.go:441-443`, `screen_download.go:215`, `screen_multi_download.go:252`, `screen_zip_download.go:1106`, `screen_cache_refresh.go:137-150`, `screen_fetch_uploads.go:255`, `screen_autodetect.go:177-182`, `screen_zip_inspect.go:190-194`

**Interfaces:**
- Consumes: `netstate.Describe` (Task 4).
- Produces: `func problemText(err error) string`.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestProblemText(t *testing.T) {
	dns := &url.Error{Op: "Get", URL: "https://itch.io/games", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
	got := problemText(dns)
	if got != "Can't reach itch.io. Check your Wi-Fi connection, then try again." {
		t.Fatalf("problemText(dns) = %q", got)
	}
	if strings.Contains(got, "https://") {
		t.Fatal("URL on screen")
	}
	own := errors.New("no downloadable files found for this game")
	if problemText(own) != own.Error() {
		t.Fatal("a non-network error lost its own text")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run TestProblemText -v`
Expected: FAIL — `undefined: problemText`.

- [ ] **Step 3: Write the helper and replace each site**

`internal/ui/net_problem.go`:

```go
package ui

import "github.com/carroarmato0/nextui-itchio-pak/internal/netstate"

// problemText is what a screen shows for err. Connection problems get plain
// words; anything else keeps its own text, which is usually already written
// for the user ("no downloadable files found for this game"). The raw error is
// logged where it happens, never drawn.
func problemText(err error) string {
	if m, ok := netstate.Describe(err, "itch.io"); ok {
		return m.Title + ". " + m.Hint
	}
	return err.Error()
}
```

Replace, keeping each screen's layout:

1. `screen_list.go:634-645` — replace the whole `if s.err != nil { … }` drawing block body (keep the footer hints below it) with:

```go
	if s.err != nil {
		_, fontH := r.TextSize("Ag")
		msg := problemText(s.err)
		lines := r.WrapText(msg, r.W-40)
		startY := r.H/2 - int32(len(lines))*(fontH+4)/2
		er := r.Theme.Error()
		r.DrawWrappedText(msg, 20, startY, r.W-40, fontH+4, er[0], er[1], er[2])
		ftrY := r.DrawFooterBar(52)
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgeCircle, Label: "A", Text: "Retry"},
			{Kind: renderer.BadgeCircle, Label: "B", Text: "Exit"},
		}, ftrY)
		r.Present()
		return
	}
```

(This removes the Cloudflare branch. Remove the `errors` import from `screen_list.go` only if nothing else in the file uses it — check with `grep -n "errors\." internal/ui/screen_list.go`.)

2. `screen_detail.go:441-443` —

```go
	if s.err != nil {
		er := r.Theme.Error()
		r.DrawWrappedText(problemText(s.err), margin, contentTop+20, r.W-2*margin, fontH+4, er[0], er[1], er[2])
```

(if `fontH` is not in scope at that point, add `_, fontH := r.TextSize("Ag")` as the first line of the block.)

3. `screen_download.go:215` — `msg := s.err.Error()` → `msg := problemText(s.err)`.
4. `screen_multi_download.go:252` and `screen_zip_download.go:1106` — `s.err.Error()` → `problemText(s.err)` in the `DrawWrappedText` call.
5. `screen_cache_refresh.go:137-150` — delete the `if errors.Is(s.err, itchio.ErrCloudflareBlocked) { … } else {` wrapper so only the generic branch remains, and in it use `msg := problemText(s.err)` for both the `WrapText` and the `DrawWrappedText` calls. Remove now-unused imports (`errors`, possibly `itchio`) — check with `go vet`.
6. `screen_fetch_uploads.go:255` — `msg := s.err.Error()` → `msg := problemText(s.err)` (the equality check at line 244 stays on `s.err.Error()`).
7. `screen_autodetect.go:177-182` and `screen_zip_inspect.go:190-194` — introduce `msg := problemText(s.err)` at the top of the case and use it in both the `WrapText` and `DrawWrappedText` calls.

- [ ] **Step 4: Run the tests and a non-headless compile**

Run: `go test ./internal/ui/ -run TestProblemText -v && go vet ./internal/ui/ && go build ./cmd/itchio-pak/`
Expected: PASS, no vet findings, the SDL build compiles.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/
git commit -m "Say what went wrong with the connection instead of printing Go errors

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Offline chip, reconnect retries, Settings annotation

**Files:**
- Modify: `internal/ui/screen_list.go` (struct fields, `Draw` header block near line 583, `buildCache`, new `RetryAfterReconnect`), `internal/ui/screen_settings.go:447-466` (`updateInventoryAnnotation`)
- Test: `internal/ui/update_label_test.go` (extend)

**Interfaces:**
- Consumes: `netstate.Current`, `netstate.Offline`, `netstate.SetForTest`.
- Produces: `func (s *ListScreen) RetryAfterReconnect()` — safe from any goroutine.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/update_label_test.go` (check its build tag first; keep the file's existing tag):

```go
type fakeSvc struct {
	running bool
	at      time.Time
}

func (f fakeSvc) TriggerNow()                {}
func (f fakeSvc) IsRunning() bool            { return f.running }
func (f fakeSvc) LatestCheckedAt() time.Time { return f.at }

func TestUpdateInventoryAnnotation_offline(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
	if got := updateInventoryAnnotation(fakeSvc{running: true}); got != "offline" {
		t.Fatalf("annotation = %q, want offline", got)
	}
}
```

If a type named `fakeSvc` (or a fake `UpdateServicer`) already exists in the `ui` test files, reuse it instead of adding one. Add imports `time` and `.../internal/netstate` as needed.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run TestUpdateInventoryAnnotation_offline -v`
Expected: FAIL — annotation is `checking…`.

- [ ] **Step 3: Implement**

`screen_settings.go`, first lines of `updateInventoryAnnotation`:

```go
	if netstate.Offline() {
		return "offline"
	}
```

and in `Draw`'s annotation colour choice, treat offline like running (Warning colour): change `if s.updateSvc.IsRunning() {` to `if s.updateSvc.IsRunning() || netstate.Offline() {`.

`screen_list.go` — add fields to `ListScreen`:

```go
	// reconnected is set from the netstate goroutine and consumed in Draw, the
	// only place that may touch cacheReady, err and the fetch goroutines' flags.
	reconnected      atomic.Bool
	cacheFetchFailed atomic.Bool
```

In `buildCache`, in the `FetchAllGames` error branch, before `return`: `s.cacheFetchFailed.Store(true)`; and in the success path, `s.cacheFetchFailed.Store(false)`.

Add:

```go
// RetryAfterReconnect is registered with netstate.OnReconnect. It runs on the
// netstate goroutine, so it only raises a flag and wakes the UI.
func (s *ListScreen) RetryAfterReconnect() {
	s.reconnected.Store(true)
	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
}
```

At the start of `Draw` (after existing channel draining, before any drawing):

```go
	if s.reconnected.Swap(false) {
		if s.err != nil && !s.loading.Load() {
			logger.Info("feed: connection back, retrying page 1")
			s.err = nil
			go s.loadPage(1)
		}
		if s.cacheFetchFailed.Load() && !s.cacheBuilding.Load() {
			logger.Info("cache: connection back, retrying the game-list refresh")
			go s.buildCache()
		}
	}
```

Header chip — inside the "Filter pills" block, after the platform pill is drawn (after `r.DrawTextCenteredInRect(platLabel, …)`):

```go
		// Offline chip, left of the platform pill. The cached list stays usable;
		// this only says why nothing new is arriving.
		if netstate.Offline() {
			offLabel := "Offline"
			ow, _ := r.TextSize(offLabel)
			offW := ow + 24
			offX := platPillX - offW - 6
			wr, wg, wb := rgb(r.Theme.WarningBG())
			r.DrawPill(offX, pillY, offW, pillH, wr, wg, wb)
			tc := r.Theme.ToneOn(r.Theme.Warning(), r.Theme.WarningBG())
			r.DrawTextCenteredInRect(offLabel, offX, pillY, offW, pillH, tc[0], tc[1], tc[2])
		}
```

Add imports `sync/atomic` (if not present) and `.../internal/netstate`.

- [ ] **Step 4: Run tests and compile**

Run: `go test ./internal/ui/ -v 2>&1 | tail -20 && go build ./cmd/itchio-pak/`
Expected: PASS; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_list.go internal/ui/screen_settings.go internal/ui/update_label_test.go
git commit -m "Offline chip on the game list; list and cache refresh retry on reconnect

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Wiring and build date

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go` (after `client := itchio.NewClient()` ~line 205, after `listScreen := …` ~line 232), `scripts/build.sh:78-118,155,250`
- Test: `scripts/build_date_test.sh` (new)

**Interfaces:**
- Consumes: `netstate.SetNotify`, `netstate.StartMonitor`, `netstate.OnReconnect`, `(*itchio.Client).Probe`, `partfile.SetJournal/Recover/Sweep`, `(*UpdateService).RetryIfOwed`, `(*ImageCache).Resume`, `(*ListScreen).RetryAfterReconnect`, `firmware.Env.ROMDirs/MusicRoot`.

- [ ] **Step 1: Write the failing check**

`build.sh` dispatches on its argument at the bottom, so it cannot be sourced by a test. Move `LDFLAGS_FOR` (currently `scripts/build.sh:113-116`) into a new file `scripts/lib/ldflags.sh`, unchanged for now, and replace it in `build.sh` with:

```sh
# shellcheck source=scripts/lib/ldflags.sh
. "$(dirname "$0")/lib/ldflags.sh"
```

(`build.sh` runs again inside the container with the repository mounted, so the relative path resolves there too.)

Create `scripts/build_date_test.sh`:

```sh
#!/bin/sh
# The wrong-clock check in internal/netstate needs the build date in the binary.
set -e
. "$(dirname "$0")/lib/ldflags.sh"
FLAGS="$(BUILD_DATE=2026-09-26T10:00:00+02:00 LDFLAGS_FOR v1 abc)"
echo "$FLAGS" | grep -q "internal/netstate.buildDate=2026-09-26T10:00:00+02:00" || {
    echo "FAIL: LDFLAGS_FOR does not set netstate.buildDate: $FLAGS" >&2; exit 1; }
echo "PASS: build date in ldflags"
```

Run: `chmod +x scripts/build_date_test.sh && sh scripts/build_date_test.sh`
Expected: `FAIL: LDFLAGS_FOR does not set netstate.buildDate`.

- [ ] **Step 2: Implement the build date**

In `scripts/lib/ldflags.sh`:

```sh
LDFLAGS_FOR() {
    printf -- "-X 'main.version=%s' -X 'main.gitCommit=%s' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=%s' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/netstate.buildDate=%s'" \
        "$1" "$2" "$1" "${BUILD_DATE:-}"
}
```

Next to where `GIT_COMMIT` is computed on the host (line ~88), add:

```sh
    # Commit date, not wall-clock time, so a rebuild of the same commit is
    # identical. netstate uses it to tell a wrong device clock from a bad
    # certificate.
    BUILD_DATE=$(git log -1 --format=%cI 2>/dev/null || echo "")
```

declare `BUILD_DATE="${BUILD_DATE:-}"` beside `GIT_COMMIT="${GIT_COMMIT:-}"` (line 78), and pass it into both containers next to `-e GIT_COMMIT="$GIT_COMMIT"` (lines 155 and 250): `-e BUILD_DATE="$BUILD_DATE" \`.

In `scripts/test.sh`, after the `launch_test.sh` lines (52-53), add:

```sh
    echo "==> build_date_test.sh"
    "$SCRIPT_DIR/build_date_test.sh" || exit 1
```

Run: `sh scripts/build_date_test.sh`
Expected: `PASS: build date in ldflags`.

- [ ] **Step 3: Wire the app**

In `cmd/itchio-pak/main_sdl.go`, immediately after `dataDir := env.DataDir()` and before `settings.Load`:

```go
	// Before any download can start: delete partial files a crash or power cut
	// left behind last time.
	partfile.SetJournal(filepath.Join(dataDir, "partials.json"))
	if n := partfile.Recover(); n > 0 {
		logger.Info("partfile: removed %d leftover partial download(s)", n)
	}
```

After `client.SetAuthToken(cfg.AuthToken)`:

```go
	netstate.SetNotify(func() {
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
	})
	netstate.StartMonitor("/", client.Probe)
```

After `updateSvc.Start(nil)` / `defer updateSvc.Stop()` and after `listScreen := …`, register in the spec's order (game list, inventory, covers; the app-update check is added by its own plan):

```go
	netstate.OnReconnect(listScreen.RetryAfterReconnect)
	netstate.OnReconnect(updateSvc.RetryIfOwed)
	netstate.OnReconnect(cache.Resume)

	go func() {
		dirs := []string{env.MusicRoot()}
		for _, d := range env.ROMDirs() {
			if d != "" {
				dirs = append(dirs, d)
			}
		}
		partfile.Sweep(dirs)
	}()
```

Add imports for `netstate` and `partfile`. `logRomDirs` in `main.go` shows `ROMDirs()` returns `map[string]string`, which the loop above matches.

- [ ] **Step 4: Build and run the full suite**

Run: `./scripts/test.sh 2>&1 | tail -30 && ./scripts/build.sh nextui/tg5040`
Expected: all tests PASS, including the palette audit and `build_date_test.sh`; the cross-compile succeeds.

- [ ] **Step 5: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go scripts/
git commit -m "Wire connectivity: monitor, reconnect tasks, partial-file cleanup, build date

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: Offscreen scenes and palette audit

**Files:**
- Create: `internal/ui/dev_scenes_offline.go`
- Modify: none else (the audit picks up registered scenes automatically; confirm with `scripts/palette-audit.sh --help` or by reading the script).

**Interfaces:**
- Consumes: `netstate.SetForTest`, `devList`, `DetailScreen`, `DownloadScreen` fields `err`, `state`.

- [ ] **Step 1: Add the scenes**

```go
//go:build !headless

package ui

import (
	"net"
	"net/url"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func offlineErr() error {
	return &url.Error{Op: "Get", URL: "https://itch.io/games/made-with-gb-studio", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
}

// Offline scenes: the Offline chip over a cached list, and the plain-language
// message where a screen has nothing cached to fall back on.
func init() {
	offline := func(reason netstate.Reason) {
		netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: reason})
	}
	devScenes = append(devScenes,
		Scene{Name: "list-offline", Desc: "Cached game list while offline (header chip)", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonDNS)
			return devList(d)
		}},
		Scene{Name: "list-offline-nocache", Desc: "No cached list and no Wi-Fi", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonNoNetwork)
			s := devList(d)
			s.cachedGames, s.viewGames, s.cacheReady = nil, nil, false
			s.err = offlineErr()
			return s
		}},
		Scene{Name: "detail-offline", Desc: "Game page that could not load", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonDNS)
			s := devDetail(d)
			s.err = offlineErr()
			return s
		}},
	)
}
```

Adjust the field names (`cachedGames`, `viewGames`, `cacheReady`, `err`, `loading`) to the real ones if any differ (`grep -n "cachedGames\|viewGames\|cacheReady" internal/ui/screen_list.go | head`). If `devDetail` returns a screen whose `err` is not drawn until a loading flag is cleared, clear that flag too (see the `Loading...` branch in `screen_detail.go:430-440`).

Scene state leaks between scenes because `netstate` is global. Add, at the top of the existing scene runner in `cmd/devshot/render.go` (the function that calls `Scene.Build`), `netstate.ResetForTest()` before each build, so every other scene renders Online.

- [ ] **Step 2: Render and inspect**

Run:
```bash
mkdir -p /tmp/itchio-screenshots
for g in 1024x768 640x480 720x480 720x720; do
  go run ./cmd/devshot --scene list-offline --size $g --out /tmp/itchio-screenshots/list-offline-$g.png
  go run ./cmd/devshot --scene list-offline-nocache --size $g --out /tmp/itchio-screenshots/list-offline-nocache-$g.png
done
```
(check `go run ./cmd/devshot --help` for the exact size/output flag names and adapt.)
Expected: at every geometry the Offline chip sits left of the platform pill without overlapping the title; the no-cache message is centred, wraps, and contains no URL. If the chip collides with the title at 640×480, hide the chip when `abbreviate(r.W)` and the search/title text would overlap, by measuring the title width and skipping the chip when `offX` < title end + 10.

- [ ] **Step 3: Run the palette audit**

Run: `./scripts/palette-audit.sh`
Expected: no low-contrast findings for the new scenes on any of the 18 palettes at any geometry.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/dev_scenes_offline.go cmd/devshot/render.go
git commit -m "devshot: offline scenes, included in the palette audit

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: Verify on both devices

**Files:** none (results go in the PR description / release notes draft).

Use the `itchio-pak-build` and `itchio-pak-adb-debug` skills. `release.sh` before `deploy.sh`, always; select devices with `scripts/adb.sh` by firmware.

- [ ] **Step 1: Build and deploy to both**

```bash
./scripts/release.sh
ADB_FIRMWARE=nextui ./scripts/deploy.sh
ADB_FIRMWARE=muos ./scripts/deploy.sh
```
(check `deploy.sh --help` for how it selects firmware.)

- [ ] **Step 2: Run each scenario and record the log lines**

For each device, with `./scripts/debug.sh logs` streaming:

1. Launch online; switch Wi-Fi off in the system menu while on the list. Expect `netstate: online -> offline(…)`, the Offline chip, no further `image cache: fetch` lines while scrolling, `update-svc: … abandoning`.
2. Switch Wi-Fi back on. Expect `netstate: default route is back, probing`, `reconnect probe answered`, `running 3 deferred task(s)`, covers filling in without a relaunch.
3. Delete `games_cache.json` from the data dir, switch Wi-Fi off, launch. Expect "No Wi-Fi connection. Turn on Wi-Fi in the system settings." and A Retry working once Wi-Fi is on.
4. With an installed game that has an update, start the update and switch Wi-Fi off mid-download. Expect "Download failed" with the plain message, the old ROM still present and launchable, no `.…itchio-part` file in the ROM folder.
5. Start a download and kill the app mid-transfer (`adb shell killall itchio`). Relaunch. Expect `partfile: removed leftover … (found by journal)`.
6. Set the device date to 2020 (`adb shell date -s 2020-01-01`), launch online. Expect "The date and time are wrong". Restore the date (`adb shell date -s "$(date -u '+%Y-%m-%d %H:%M:%S')"`).

- [ ] **Step 3: Report**

Write the outcome of each scenario per device (pass/fail + the relevant log line) into the PR description. Anything that failed goes back to the task that owns it; do not merge with a failing scenario.
