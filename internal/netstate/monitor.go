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
	// afterRun, when set, runs after run() returns and before running is
	// cleared. Tests only: it lets a test land a state change deterministically
	// in the window the loop below has to survive.
	afterRun func()
}

// StartMonitor watches for the connection coming back. It is idle while
// online; each transition to Offline starts one reconnect loop. probe must
// make one cheap request through a client whose transport feeds this package.
func StartMonitor(root string, probe func() error) {
	startMonitor(getStd(), root, probe, time.Sleep)
}

// startMonitor is StartMonitor with the tracker and the poll sleep injectable,
// so tests can drive it against a private tracker instead of std and without
// real waits.
func startMonitor(t *tracker, root string, probe func() error, sleep func(time.Duration)) *monitor {
	m := &monitor{
		t: t, root: root, probe: probe,
		poll: 5 * time.Second, backoffMin: 15 * time.Second, backoffMax: 5 * time.Minute,
		sleep: sleep,
	}
	t.mu.Lock()
	t.hasRoute = func() bool { return HasRoute(root) }
	t.mu.Unlock()
	t.subscribe(func(s State) {
		if s.Status == StatusOffline && m.running.CompareAndSwap(false, true) {
			go m.loop()
		}
	})
	logger.Info("netstate: monitor ready (root=%s)", root)
	return m
}

// loop runs the reconnect loop and keeps running it as long as the state is
// still Offline when it finishes. run() only returns once the state is no
// longer Offline, but the state can flip back to Offline in the gap between
// that return and running being cleared below; a Report arriving in exactly
// that gap finds running still true, so its subscriber cannot start a second
// loop. Rechecking here — from the one goroutine that is allowed to run at a
// time — closes that gap instead of leaving the app Offline with nothing
// polling for a route. run() itself logs each poll attempt; loop only logs
// the session boundary and a restart, so the two nest cleanly in the log.
func (m *monitor) loop() {
	logger.Info("netstate: reconnect loop started")
	for {
		m.run()
		if m.afterRun != nil {
			m.afterRun()
		}
		m.running.Store(false)
		if m.t.current().Status != StatusOffline {
			logger.Info("netstate: reconnect loop finished")
			return
		}
		if !m.running.CompareAndSwap(false, true) {
			// Another Report already won the race and started its own loop;
			// let it own the retry.
			logger.Info("netstate: reconnect loop handed off to a new one, stopping")
			return
		}
		logger.Info("netstate: went offline again before the loop cleared, continuing")
	}
}

// run polls the local route every m.poll while offline. With no route it
// records NoNetwork and waits. With a route it probes, first after
// backoffMin, then doubling up to backoffMax; a route that has just come
// back is probed at once. It returns as soon as the state is Online. Callers
// only ever see one pass end before the next begins: loop, not run, decides
// whether to call it again.
func (m *monitor) run() {
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
}
