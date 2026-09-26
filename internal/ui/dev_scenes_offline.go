//go:build !headless

package ui

import (
	"net"
	"net/url"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func offlineErr() error {
	return &url.Error{Op: "Get", URL: "https://itch.io/games/made-with-gb-studio", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
}

// devUpdateSvc is a lightweight stand-in for *inventory.UpdateService, used
// only so scenes can render the Update Inventory annotation states without
// owning a live game list.
type devUpdateSvc struct {
	running bool
	at      time.Time
}

func (d devUpdateSvc) TriggerNow()                {}
func (d devUpdateSvc) IsRunning() bool            { return d.running }
func (d devUpdateSvc) LatestCheckedAt() time.Time { return d.at }

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
		// F1: the Update Inventory row's right-aligned annotation is Warning
		// (running/offline) or Muted (idle) regardless of selection, and the
		// selected row is filled with an Accent pill — on some palettes that is
		// invisible. These two put the cursor on the row in each state so the
		// audit checks the annotation over the Accent fill, not just the
		// unselected row.
		Scene{Name: "settings-update-offline", Desc: "Settings, cursor on Update Inventory, offline (annotation on the Accent pill)", Build: func(d SceneDeps) Screen {
			offline(netstate.ReasonDNS)
			s := NewSettingsScreen(d.Client, d.Cfg, d.CfgPath, d.Inv, d.InvPath, d.Cache,
				devList(d), nil, devUpdateSvc{at: time.Now()},
				d.Theme, d.Theme, true, "Dev Palette", func(bool) {}, nil)
			s.cursor = sItemUpdateInventory
			return s
		}},
		Scene{Name: "settings-update-idle", Desc: "Settings, cursor on Update Inventory, online and idle (annotation on the Accent pill)", Build: func(d SceneDeps) Screen {
			s := NewSettingsScreen(d.Client, d.Cfg, d.CfgPath, d.Inv, d.InvPath, d.Cache,
				devList(d), nil, devUpdateSvc{at: time.Now().Add(-5 * time.Minute)},
				d.Theme, d.Theme, true, "Dev Palette", func(bool) {}, nil)
			s.cursor = sItemUpdateInventory
			return s
		}},
	)
}
