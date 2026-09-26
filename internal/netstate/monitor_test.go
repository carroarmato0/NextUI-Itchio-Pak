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
