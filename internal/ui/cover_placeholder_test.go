//go:build !headless

package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// F3: while offline, ImageCache.Get/Peek return nil and start no fetch, so a
// texture-less cover would sit under "Loading..." forever -- that fetch is
// never coming. It should say so instead of lying about a fetch in flight.
func TestCoverPlaceholderLabel_offline(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})

	if got := coverPlaceholderLabel(); got != "Offline" {
		t.Errorf("coverPlaceholderLabel() = %q, want %q", got, "Offline")
	}
}

func TestCoverPlaceholderLabel_online(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})

	if got := coverPlaceholderLabel(); got != "Loading..." {
		t.Errorf("coverPlaceholderLabel() = %q, want %q", got, "Loading...")
	}
}
