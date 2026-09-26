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
