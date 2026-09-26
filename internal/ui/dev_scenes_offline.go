//go:build !headless

package ui

import (
	"net"
	"net/url"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func offlineErr() error {
	return &url.Error{Op: "Get", URL: "https://itch.io/games/made-with-gb-studio", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
}

// Offline scenes: the Offline chip over a cached list, and the plain-language
// message where a screen has nothing cached to fall back on.
func init() {
	offline := func(reason netstate.Reason) {
		netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: reason})
	}
	devScenes = append(devScenes,
		Scene{Name: "list-offline", Desc: "Cached game list while offline (header chip)", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonDNS)
			return devList(d)
		}},
		Scene{Name: "list-offline-nocache", Desc: "No cached list and no Wi-Fi", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonNoNetwork)
			s := devList(d)
			s.cachedGames, s.viewGames, s.cacheReady = nil, nil, false
			s.err = offlineErr()
			return s
		}},
		Scene{Name: "detail-offline", Desc: "Game page that could not load", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonDNS)
			s := devDetail(d)
			s.err = offlineErr()
			return s
		}},
	)
}
