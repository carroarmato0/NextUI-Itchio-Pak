package ui

import (
	"errors"
	"strconv"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// The reduced game page is what DetailScreen shows when the full page could
// not be fetched: what the list already knew about the game (title, author,
// tags, cover if cached), the QR code, the local files, and why the rest is
// missing. It has no *itchio.GameDetail, so everything here decides from the
// list's itchio.Game and the connection state alone.

// reducedActionOffered reports whether the reduced page offers a on the A
// button. Offline nothing A starts can work. Online, only a free game's
// download can: FetchUploadsScreen's free path needs nothing but the game URL,
// whereas a paid download needs the page's game_id — and signing in on this
// page would only lead to that paid download.
func reducedActionOffered(a detailAction, free, offline bool) bool {
	if offline {
		return false
	}
	return free && a.onA() == "download"
}

// reloadInput is what the reduced page knows when deciding whether to fetch
// the full page again by itself.
type reloadInput struct {
	FailedOffline bool // the fetch failed with an offline-class error
	SawOffline    bool // the connection has been seen down since that failure
	OfflineNow    bool // netstate.Offline() right now
	Loading       bool // a fetch is already running
}

// shouldReloadDetail reports whether the reduced page should reload itself:
// only after an offline-class failure, once the connection has been seen to
// go down and come back. Requiring the transition means a failure the tracker
// never saw as offline cannot retry on every frame. A server error or a
// removed game does not get better when the Wi-Fi does.
func shouldReloadDetail(in reloadInput) bool {
	return in.FailedOffline && in.SawOffline && !in.OfflineNow && !in.Loading
}

// reducedReason is the class of err for the log line that says the reduced
// page is showing: netstate's reason, "http-<code>" for a server status,
// "removed" for a 404/410. Never the error text, which can quote a URL.
func reducedReason(err error) string {
	if errors.Is(err, itchio.ErrGameRemoved) {
		return "removed"
	}
	var se *netstate.StatusError
	if errors.As(err, &se) {
		return "http-" + strconv.Itoa(se.Code)
	}
	return netstate.Classify(err).String()
}
