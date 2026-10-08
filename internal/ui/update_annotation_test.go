//go:build !headless

package ui

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

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
