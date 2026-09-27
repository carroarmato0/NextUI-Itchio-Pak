//go:build !headless

package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// A manual "Refresh Game List" that succeeds has fetched the whole list, so
// an earlier failed background fetch is no longer owed: the next reconnect
// must not run another full fetch.
func TestManualRefreshSuccessClearsCacheFetchFailed(t *testing.T) {
	s := &ListScreen{cacheUpdateCh: make(chan []itchio.Game, 1)}
	s.cacheFetchFailed.Store(true)
	s.manualRefreshDone([]itchio.Game{{Title: "a"}})
	if s.cacheFetchFailed.Load() {
		t.Fatal("cacheFetchFailed still set after a successful manual refresh")
	}
	select {
	case got := <-s.cacheUpdateCh:
		if len(got) != 1 {
			t.Fatalf("installed %d games, want 1", len(got))
		}
	default:
		t.Fatal("the refreshed list was not handed to the list screen")
	}
}
