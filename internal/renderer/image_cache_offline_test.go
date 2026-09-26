//go:build !headless

package renderer

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func TestImageCache_noFetchWhileOffline(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewImageCache(10, srv.Client())
	netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
	for i := 0; i < 5; i++ {
		if c.Get(nil, srv.URL+"/cover.png") != nil {
			t.Fatal("got a texture offline")
		}
	}
	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("%d requests made while offline", n)
	}

	notified := make(chan struct{}, 1)
	c.SetNotify(func() { notified <- struct{}{} })
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	c.Resume()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("Resume did not ask for a redraw")
	}
	c.Get(nil, srv.URL+"/cover.png")
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&hits) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("no fetch after coming back online")
	}
}
