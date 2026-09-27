//go:build !headless

package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// Offline the reduced page's image box reuses the covers' "Offline" wording;
// online no fetch is coming for it either, so it is simply "No Image".
func TestReducedCoverLabel(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()

	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
	if got := reducedCoverLabel(); got != "Offline" {
		t.Errorf("reducedCoverLabel() offline = %q, want %q", got, "Offline")
	}
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	if got := reducedCoverLabel(); got != "No Image" {
		t.Errorf("reducedCoverLabel() online = %q, want %q", got, "No Image")
	}
}
