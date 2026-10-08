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
