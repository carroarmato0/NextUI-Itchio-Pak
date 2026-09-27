//go:build !headless

package ui

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

// The reduced page reloads itself exactly once when the connection comes
// back, through the constructor's fetch, and never starts a second fetch
// while one is running.
func TestDetailScreen_reloadsOnceAfterReconnect(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()

	var pageHits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/game" {
			pageHits.Add(1)
			<-release
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	// Runs before srv.Close, so a failing assertion never leaves a handler
	// blocked and the test hung.
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	client := itchio.NewClient()
	s := &DetailScreen{
		client: client,
		cfg:    &settings.Config{},
		cache:  renderer.NewImageCache(4, client.HTTPClient()),
		game:   itchio.Game{Title: "T", URL: srv.URL + "/game", IsFree: true},
	}

	// The first fetch failed offline; the reduced page is showing.
	s.err = offlineErr()
	s.failedOffline = true
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})

	s.reloadIfReconnected()
	if pageHits.Load() != 0 || s.fetching.Load() {
		t.Fatal("reloaded while still offline")
	}

	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	s.reloadIfReconnected()
	if !s.fetching.Load() || !s.loading || s.err != nil {
		t.Fatalf("no reload after reconnect: fetching=%v loading=%v err=%v", s.fetching.Load(), s.loading, s.err)
	}
	// Draw keeps running while the fetch is in flight.
	s.reloadIfReconnected()
	if s.fetchDetail() {
		t.Error("fetchDetail started a second fetch while one was running")
	}
	waitFor(t, func() bool { return pageHits.Load() >= 1 })
	unblock()
	waitFor(t, func() bool { return !s.fetching.Load() })

	if got := pageHits.Load(); got != 1 {
		t.Errorf("game page fetched %d times, want 1", got)
	}
	// A 503 is not offline-class: reconnecting again must not retry it.
	if s.err == nil || s.failedOffline {
		t.Fatalf("after 503: err=%v failedOffline=%v", s.err, s.failedOffline)
	}
	s.loading = false
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
	s.reloadIfReconnected()
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	s.reloadIfReconnected()
	if s.fetching.Load() {
		t.Error("reloaded a server error on reconnect")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
