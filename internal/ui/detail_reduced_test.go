package ui

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// The reduced page has no GameDetail. Offline nothing A starts can work; online
// only a free game's download does, because that path needs nothing but the
// game URL (a paid download needs the page's game_id).
func TestReducedActionOffered(t *testing.T) {
	for _, tc := range []struct {
		name    string
		a       detailAction
		free    bool
		offline bool
		want    bool
	}{
		{"offline, free download", actionDownload, true, true, false},
		{"offline, free download again", actionDownloadAgain, true, true, false},
		{"offline, sign in", actionSignIn, false, true, false},
		{"online, free download", actionDownload, true, false, true},
		{"online, free download again", actionDownloadAgain, true, false, true},
		{"online, paid owned download needs game_id", actionDownload, false, false, false},
		{"online, paid owned download again needs game_id", actionDownloadAgain, false, false, false},
		{"online, sign in leads to a download that needs game_id", actionSignIn, false, false, false},
		{"online, reauth update", actionReauthUpdate, false, false, false},
		{"online, buy (A does nothing anyway)", actionBuy, false, false, false},
		{"online, checking", actionChecking, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reducedActionOffered(tc.a, tc.free, tc.offline); got != tc.want {
				t.Errorf("reducedActionOffered(%v, free=%v, offline=%v) = %v, want %v",
					tc.a, tc.free, tc.offline, got, tc.want)
			}
		})
	}
}

// The page reloads by itself only after an offline-class failure, once the
// connection has been seen to go down and come back, and never while a fetch
// is already running.
func TestShouldReloadDetail(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   reloadInput
		want bool
	}{
		{"offline failure, seen offline, back online", reloadInput{FailedOffline: true, SawOffline: true}, true},
		{"still offline", reloadInput{FailedOffline: true, SawOffline: true, OfflineNow: true}, false},
		{"never seen offline: no transition to act on", reloadInput{FailedOffline: true}, false},
		{"server error: reconnecting changes nothing", reloadInput{SawOffline: true}, false},
		{"already loading", reloadInput{FailedOffline: true, SawOffline: true, Loading: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldReloadDetail(tc.in); got != tc.want {
				t.Errorf("shouldReloadDetail(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The reason logged when the reduced page is shown: a class, never a URL.
func TestReducedReason(t *testing.T) {
	dns := &url.Error{Op: "Get", URL: "https://itch.io/x?sig=secret",
		Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"dns", fmt.Errorf("fetch game page: %w", dns), "dns"},
		{"server error", &netstate.StatusError{What: "fetch game detail", Code: 503}, "http-503"},
		{"removed", fmt.Errorf("fetch game detail: %w", itchio.ErrGameRemoved), "removed"},
		{"other", errors.New("parse: unexpected markup"), "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reducedReason(tc.err); got != tc.want {
				t.Errorf("reducedReason(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
