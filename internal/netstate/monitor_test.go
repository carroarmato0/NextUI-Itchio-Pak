package netstate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// TestMonitor_offlineRightAfterRunStillLoops reproduces, deterministically,
// the gap between run() returning (because the state just became Online) and
// running being cleared: a report lands in that exact window and puts the
// state back Offline. Without the recheck in loop(), the app would be stuck
// Offline with no reconnect loop running, because the listener's CAS finds
// running still true. With it, this same goroutine notices the state is
// Offline again once it clears running, and keeps going.
func TestMonitor_offlineRightAfterRunStillLoops(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})
	m := &monitor{
		t: tr, root: root, poll: time.Millisecond,
		backoffMin: time.Millisecond, backoffMax: time.Millisecond,
		sleep: func(time.Duration) {},
	}
	calls := 0
	m.probe = func() error {
		calls++
		tr.report(nil) // this probe always succeeds
		return nil
	}
	injected := false
	m.afterRun = func() {
		if !injected {
			injected = true
			tr.report(dnsErr()) // land in the run()-just-returned gap
		}
	}
	tr.report(dnsErr())
	m.loop()
	if calls < 2 {
		t.Fatalf("probe called %d times, want the loop to restart and probe again", calls)
	}
	if tr.current().Status != StatusOnline {
		t.Fatalf("final state = %+v, want Online", tr.current())
	}
}

// TestStartMonitor_singleLoopUnderConcurrentReports drives Report from many
// goroutines at once, flapping the state, and checks two things under -race:
// the tracker itself survives concurrent access, and the monitor never runs
// two reconnect loops at the same time.
func TestStartMonitor_singleLoopUnderConcurrentReports(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})

	var active, maxActive int32
	probe := func() error {
		n := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if n <= old || atomic.CompareAndSwapInt32(&maxActive, old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond) // widen the window for a would-be second loop
		atomic.AddInt32(&active, -1)
		tr.report(nil)
		return nil
	}
	startMonitor(tr, root, probe, func(time.Duration) {})

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.report(dnsErr())
			tr.report(nil)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got > 1 {
		t.Fatalf("max concurrent reconnect probes = %d, want at most 1", got)
	}
	// Every goroutine's own last call was report(nil); the state must reflect
	// that regardless of anything the monitor's own probes did concurrently.
	if tr.current().Status != StatusOnline {
		t.Fatalf("final state = %+v, want Online", tr.current())
	}
}

// TestStartMonitor_installsRouteCheck confirms startMonitor wires t.hasRoute
// to the real HasRoute for root, so a DNS error reported with no route comes
// back as NoNetwork, not DNS. The interface is brought back up on the second
// poll so the background loop this starts resolves before the test returns,
// instead of spinning forever against an interface that never recovers.
func TestStartMonitor_installsRouteCheck(t *testing.T) {
	tr := newTracker()
	root := fakeRoot(t, brickRoute, map[string]string{"wlan0": "down"})
	operstate := filepath.Join(root, "sys/class/net/wlan0/operstate")
	polls := 0
	sleep := func(time.Duration) {
		polls++
		if polls > 1 {
			os.WriteFile(operstate, []byte("up\n"), 0644)
		}
	}
	probe := func() error { tr.report(nil); return nil }
	startMonitor(tr, root, probe, sleep)

	tr.report(dnsErr())
	if r := tr.current().Reason; r != ReasonNoNetwork {
		t.Fatalf("reason = %v, want NoNetwork", r)
	}

	deadline := time.Now().Add(2 * time.Second)
	for tr.current().Status != StatusOnline && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tr.current().Status != StatusOnline {
		t.Fatal("background loop never recovered once the route came back")
	}
}
