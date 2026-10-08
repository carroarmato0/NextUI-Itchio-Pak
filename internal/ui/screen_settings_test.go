//go:build !headless

package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
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
	s.moveCursor(1, true)
	if s.cursor != sItemUpdateInventory {
		t.Errorf("moving down from sItemClearCache landed on %v, want sItemUpdateInventory", s.cursor)
	}

	s.cursor = sItemUpdateInventory
	s.moveCursor(-1, true)
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
	s.moveCursor(1, true)
	if s.cursor != sItemRefreshCache {
		t.Errorf("moving down from sItemClearCache landed on %v, want sItemRefreshCache", s.cursor)
	}
}

// A fresh press past either end wraps, so the last row is one press from the
// first. Holding the button stops at the end instead of spinning round.
func TestMoveCursor_pressWrapsAtEnds(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemAccount
	s.startHold(-1)
	if s.cursor != sItemAbout {
		t.Fatalf("up from the first row: cursor = %d, want sItemAbout (%d)", s.cursor, sItemAbout)
	}
	s.stopHold(-1)
	s.startHold(1)
	if s.cursor != sItemAccount {
		t.Fatalf("down from the last row: cursor = %d, want sItemAccount (%d)", s.cursor, sItemAccount)
	}
	s.stopHold(1)
}

func TestMoveCursor_repeatStopsAtEnds(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemAccount
	s.moveCursor(-1, false)
	if s.cursor != sItemAccount {
		t.Fatalf("held up at the first row: cursor = %d, want sItemAccount", s.cursor)
	}
	s.cursor = sItemAbout
	s.moveCursor(1, false)
	if s.cursor != sItemAbout {
		t.Fatalf("held down at the last row: cursor = %d, want sItemAbout", s.cursor)
	}
}

func TestUpdatesRow_hiddenWithoutUpdater(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemNextUITheme
	s.moveCursor(1, false)
	if s.cursor != sItemLogLevel {
		t.Fatalf("without an updater the cursor must skip Updates, got %d", s.cursor)
	}
}

func TestUpdatesRow_shownWithUpdater(t *testing.T) {
	SetAppUpdater(&stubUpdater{})
	t.Cleanup(func() { SetAppUpdater(nil) })
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemNextUITheme
	s.moveCursor(1, false)
	if s.cursor != sItemUpdates {
		t.Fatalf("cursor = %d, want sItemUpdates", s.cursor)
	}
}

func TestUpdatesRowAnnotation(t *testing.T) {
	if got := updatesRowAnnotation(appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}}); got != "v1.1.0 available" {
		t.Errorf("available: %q", got)
	}
	if got := updatesRowAnnotation(appupdate.Verdict{Kind: appupdate.UpToDate, Latest: &appupdate.Release{Tag: "v1.1.0"}}); got != "" {
		t.Errorf("up to date: %q, want nothing", got)
	}
}

// Every row belongs to exactly one section, and the sections run in the
// order the rows are declared — moveCursor walks that order, so a row out of
// place would be reached from the wrong section.
func TestSettingsSections_coverEveryRowInOrder(t *testing.T) {
	next := settingsItem(0)
	for _, sec := range settingsSections {
		if sec.first != next {
			t.Fatalf("section %q starts at %d, want %d", sec.title, sec.first, next)
		}
		if sec.last < sec.first {
			t.Fatalf("section %q ends before it starts", sec.title)
		}
		next = sec.last + 1
	}
	if next != sItemCount {
		t.Fatalf("sections end at %d, want sItemCount (%d)", next, sItemCount)
	}
}

func headings(rows []settingsRow) []string {
	var out []string
	for _, r := range rows {
		if r.heading != "" {
			out = append(out, r.heading)
		}
	}
	return out
}

func TestSettingsRows_headingPerVisibleSection(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	got := strings.Join(headings(s.rows()), ",")
	if got != "Account,Downloads,Library,Appearance,App" {
		t.Fatalf("headings = %s", got)
	}
	// A section whose every row is hidden gets no heading either.
	s.themeAvailable = false
	got = strings.Join(headings(s.rows()), ",")
	if got != "Account,Downloads,Library,App" {
		t.Fatalf("without a NextUI theme, headings = %s", got)
	}
	// Each heading is followed by at least one row, never by another heading.
	rows := s.rows()
	for i, r := range rows {
		if r.heading != "" && (i+1 == len(rows) || rows[i+1].heading != "") {
			t.Fatalf("heading %q is not followed by a row", r.heading)
		}
	}
}

func TestJumpSection(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemAccount
	s.jumpSection(1)
	if s.cursor != sItemROMLocation {
		t.Fatalf("R1 from Account: cursor = %d, want ROM location", s.cursor)
	}
	s.cursor = sItemMusicDownload // mid-section: L1 goes to the previous section
	s.jumpSection(-1)
	if s.cursor != sItemAccount {
		t.Fatalf("L1 from Downloads: cursor = %d, want Account", s.cursor)
	}
	s.cursor = sItemAbout
	s.jumpSection(1)
	if s.cursor != sItemAccount {
		t.Fatalf("R1 from the last section: cursor = %d, want to wrap to Account", s.cursor)
	}
	s.jumpSection(-1)
	if s.cursor != sItemLogLevel {
		t.Fatalf("L1 from the first section: cursor = %d, want the App section's first visible row (Log level)", s.cursor)
	}
	// A section with no visible rows is skipped.
	s.themeAvailable = false
	s.cursor = sItemUpdateInventory
	s.jumpSection(1)
	if s.cursor != sItemLogLevel {
		t.Fatalf("R1 past a hidden Appearance section: cursor = %d, want Log level", s.cursor)
	}
}
