//go:build !headless

package ui

import (
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

// newTestSettingsScreen builds a SettingsScreen with minimal fixtures, enough
// to exercise cursor movement without a renderer.
func newTestSettingsScreen(t *testing.T, onRefreshGames func(Screen) Screen) *SettingsScreen {
	t.Helper()
	dir := t.TempDir()
	inv, err := inventory.Load(filepath.Join(dir, "inventory.json"))
	if err != nil {
		t.Fatalf("inventory.Load: %v", err)
	}
	cfg := &settings.Config{ROMLocation: "auto"}
	client := itchio.NewClient()
	return NewSettingsScreen(client, cfg, filepath.Join(dir, "config.json"),
		inv, filepath.Join(dir, "inventory.json"), nil, nil,
		onRefreshGames, nil, theme.Theme{}, theme.Theme{}, true, "Dev Palette", func(bool) {}, nil)
}

// F2: Draw only appends "Refresh Game List" when onRefreshGames != nil, so
// rowHidden must agree, or the cursor stalls on the invisible row.
func TestMoveCursor_skipsHiddenRefreshCacheRow(t *testing.T) {
	s := newTestSettingsScreen(t, nil) // Settings opened from the game page: no refresh action
	if !s.rowHidden(sItemRefreshCache) {
		t.Fatalf("rowHidden(sItemRefreshCache) = false, want true when onRefreshGames is nil")
	}

	s.cursor = sItemClearCache
	s.moveCursor(1)
	if s.cursor != sItemUpdateInventory {
		t.Errorf("moving down from sItemClearCache landed on %v, want sItemUpdateInventory", s.cursor)
	}

	s.cursor = sItemUpdateInventory
	s.moveCursor(-1)
	if s.cursor != sItemClearCache {
		t.Errorf("moving up from sItemUpdateInventory landed on %v, want sItemClearCache", s.cursor)
	}
}

// With onRefreshGames set (Settings opened from the list page), the row must
// stay reachable.
func TestMoveCursor_reachesRefreshCacheRowWhenAvailable(t *testing.T) {
	s := newTestSettingsScreen(t, func(sc Screen) Screen { return sc })
	if s.rowHidden(sItemRefreshCache) {
		t.Fatalf("rowHidden(sItemRefreshCache) = true, want false when onRefreshGames is set")
	}

	s.cursor = sItemClearCache
	s.moveCursor(1)
	if s.cursor != sItemRefreshCache {
		t.Errorf("moving down from sItemClearCache landed on %v, want sItemRefreshCache", s.cursor)
	}
}
