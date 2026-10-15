//go:build !headless

package ui

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

// askConfig pins NextUI under a temp root and returns a config with ROM
// location set to "ask", saved to a file in that root.
func askConfig(t *testing.T) (*settings.Config, string) {
	t.Helper()
	prev := firmware.Active()
	t.Cleanup(func() { firmware.SetActive(prev) })
	root := t.TempDir()
	firmware.SetActive(firmware.ForTest(firmware.KindNextUI, root))
	return &settings.Config{ROMLocation: "ask"}, filepath.Join(root, "settings.json")
}

func romManifest(names ...string) roms.ZIPManifest {
	var m roms.ZIPManifest
	for _, n := range names {
		m.Entries = append(m.Entries, roms.ZIPEntry{Name: n, Kind: roms.KindROM})
	}
	return m
}

func TestPendingROMExts(t *testing.T) {
	plan := ZIPPlan{
		Manifest:     romManifest("a.gbc", "b.gb", "c.p8", "d.p8.png", "e.gba"),
		DownloadROMs: true,
		SelectedROMs: map[string]string{".p8": ""},         // suppressed
		ROMDirs:      map[string]string{".gba": "/x/GBA/"}, // already chosen
	}
	if got, want := pendingROMExts(plan), []string{".gb", ".gbc", ".p8.png"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pendingROMExts = %v, want %v", got, want)
	}
	plan.DownloadROMs = false
	if got := pendingROMExts(plan); got != nil {
		t.Errorf("pendingROMExts with DownloadROMs=false = %v, want nil", got)
	}
}

// Issue #6: with ROM location "ask", archives were extracted to the default
// folder without asking. Each ROM type in the archive now gets a picker, and
// the answers reach the plan and are remembered.
func TestPickROMDirsAsksOncePerType(t *testing.T) {
	cfg, cfgPath := askConfig(t)
	plan := ZIPPlan{Manifest: romManifest("game.gb", "game.gbc"), DownloadROMs: true}

	var got *ZIPPlan
	next := func(p ZIPPlan) Screen { got = &p; return nil }

	scr := pickROMDirs(cfg, cfgPath, itchio.Game{Title: "Game"}, plan, nil, next)
	first, ok := scr.(*LocationPickerScreen)
	if !ok || first.ext != ".gb" {
		t.Fatalf("first screen = %#v, want a picker for .gb", scr)
	}
	first.currentDir = "/sd/Roms/GB/1) Itch/"

	second, ok := first.confirm().(*LocationPickerScreen)
	if !ok || second.ext != ".gbc" {
		t.Fatalf("after .gb, want a picker for .gbc")
	}
	second.currentDir = "/sd/Roms/GBC/1) Itch/"

	if scr := second.confirm(); scr != nil || got == nil {
		t.Fatalf("after .gbc, next was not called")
	}
	want := map[string]string{".gb": "/sd/Roms/GB/1) Itch/", ".gbc": "/sd/Roms/GBC/1) Itch/"}
	if !reflect.DeepEqual(got.ROMDirs, want) {
		t.Errorf("plan.ROMDirs = %v, want %v", got.ROMDirs, want)
	}
	if !reflect.DeepEqual(cfg.LastROMDirs, want) {
		t.Errorf("cfg.LastROMDirs = %v, want %v", cfg.LastROMDirs, want)
	}
	if plan.ROMDirs != nil {
		t.Errorf("caller's plan was mutated: %v", plan.ROMDirs)
	}
}

func TestZIPInspectRouteAsksForLocation(t *testing.T) {
	tests := []struct {
		name    string
		upload  string
		entries []string
		wantExt string
	}{
		{"single ROM kept as zip", "game.zip", []string{"game.gb"}, ".gb"},
		{"single ROM in 7z", "game.7z", []string{"game.gbc"}, ".gbc"},
		{"several ROM types", "game.zip", []string{"game.gbc", "game.gb"}, ".gb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cfgPath := askConfig(t)
			s := &ZIPInspectScreen{
				cfg: cfg, cfgPath: cfgPath,
				upload: roms.Upload{Filename: tt.upload},
				plan:   ZIPPlan{Manifest: romManifest(tt.entries...)},
			}
			p, ok := s.route().(*LocationPickerScreen)
			if !ok {
				t.Fatalf("route() = %T, want *LocationPickerScreen", s.route())
			}
			if p.ext != tt.wantExt {
				t.Errorf("picker ext = %q, want %q", p.ext, tt.wantExt)
			}
		})
	}
}
