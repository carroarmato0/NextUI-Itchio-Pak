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
