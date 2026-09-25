//go:build !headless

package ui

import (
	"path/filepath"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// Paid-game scenes: the game page for a paid game in each sign-in and
// ownership state (docs/mockups/detail-paid-gating.html).
func init() {
	type paidState struct {
		name, desc                                string
		signedIn, owned, installed, updatePending bool
		checking                                  bool
	}
	states := []paidState{
		{"detail-paid-signed-out", "Paid game, signed out", false, false, false, false, false},
		{"detail-paid-not-owned", "Paid game, signed in, not owned", true, false, false, false, false},
		{"detail-paid-checking", "Paid game, checking ownership", true, false, false, false, true},
		{"detail-paid-owned", "Paid game, signed in, owned", true, true, false, false, false},
		{"detail-paid-installed-update", "Installed paid game, signed out, update waiting", false, false, true, true, false},
		{"detail-paid-installed", "Installed paid game, signed out", false, false, true, false, false},
	}
	for _, st := range states {
		st := st
		devScenes = append(devScenes, Scene{Name: st.name, Desc: st.desc, Build: func(d SceneDeps) Screen {
			if st.signedIn {
				d.Cfg.SetSignedIn("dev-token", "carroarmato0", time.Now())
			}
			game := d.Games[0]
			game.IsFree, game.Price = false, 2.75
			if st.installed {
				devInstallPaid(d, game, st.updatePending)
			}
			s := devDetailFor(d, game)
			s.owned.Store(st.owned)
			s.checkingOwnership.Store(st.checking)
			return s
		}})
	}
}

// devInstallPaid records game as installed, optionally with a newer file
// upstream so an update is pending.
func devInstallPaid(d SceneDeps, game itchio.Game, updatePending bool) {
	file := inventory.DownloadedFile{
		Filename: "Tobu Tobu Girl Deluxe.gbc",
		DestPath: filepath.Join(firmware.Active().ROMDirForSystem(firmware.SysGBC), "Tobu Tobu Girl Deluxe.gbc"),
	}
	d.Inv.Add(game.URL, inventory.Entry{GameURL: game.URL, Title: game.Title, Author: game.Author}, file)
	if updatePending {
		d.Inv.SetUpstreamFiles(game.URL, []inventory.UpstreamFile{{Filename: "tobudx.gbc", UploadID: "1"}})
		d.Inv.SetUpstreamFiles(game.URL, []inventory.UpstreamFile{
			{Filename: "tobudx.gbc", UploadID: "1"}, {Filename: "tobudx-v2.gbc", UploadID: "2"},
		})
	}
}
