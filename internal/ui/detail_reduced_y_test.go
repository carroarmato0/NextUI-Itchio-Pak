//go:build !headless

package ui

import (
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// Y (unified naming) is not drawn on the reduced page, and it renames files:
// it must do nothing there, on the keyboard and on the pad.
func TestDetailScreen_reducedPageIgnoresY(t *testing.T) {
	for _, tc := range []struct {
		name string
		ev   sdl.Event
	}{
		{"keyboard", &sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Sym: sdl.K_y}}},
		{"pad", &sdl.ControllerButtonEvent{Type: sdl.CONTROLLERBUTTONDOWN, Button: btnY}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			invPath := filepath.Join(dir, "inventory.json")
			inv, _ := inventory.Load(invPath)
			game := itchio.Game{Title: "T", URL: "https://dev.itch.io/t", IsFree: true}
			inv.Add(game.URL, inventory.Entry{GameURL: game.URL, Title: game.Title},
				inventory.DownloadedFile{Filename: "T.gb", DestPath: filepath.Join(dir, "T.gb")})
			s := &DetailScreen{
				cfg: &settings.Config{UnifiedNaming: true}, game: game,
				inv: inv, inventoryPath: invPath,
				err: offlineErr(), // the fetch failed: detail stays nil
			}

			if got := s.HandleEvent(tc.ev); got != Screen(s) {
				t.Errorf("Y on the reduced page returned %T, want the same screen", got)
			}
			if e, _ := inv.Lookup(game.URL); e.UnifiedNamingDisabled {
				t.Error("Y on the reduced page changed the unified-naming flag")
			}
		})
	}
}
