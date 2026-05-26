package roms_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestSanitiseFilename(t *testing.T) {
	cases := []struct {
		title string
		ext   string
		want  string
	}{
		{"Doomslinger Dungeon", ".gb", "Doomslinger Dungeon.gb"},
		{"Solastra", ".gbc", "Solastra.gbc"},
		{"My Game: Subtitle", ".gb", "My Game Subtitle.gb"},
		{"Game/Title", ".gb", "GameTitle.gb"},
		{"  Spaced  Title  ", ".gb", "Spaced Title.gb"},
		{"Game * Name", ".gb", "Game Name.gb"},
		{"Game Boy ROM", ".gb", "Game Boy ROM.gb"},
		{"", ".gb", ""},
		{"🎮 Adventure Quest", ".gb", "Adventure Quest.gb"},
		{"Night 🌙 Crawler", ".gb", "Night Crawler.gb"},
		{"⚔️Dungeon", ".gbc", "Dungeon.gbc"},
		{"🎮🌙", ".gb", ""},
	}
	for _, c := range cases {
		got := roms.SanitiseFilename(c.title, c.ext)
		if got != c.want {
			t.Errorf("SanitiseFilename(%q, %q) = %q, want %q", c.title, c.ext, got, c.want)
		}
	}
}

func TestResolveUnifiedDest_NoCollision(t *testing.T) {
	dir := t.TempDir()
	// Create the ROM at its upstream name
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon", false)
	want := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true")
	}
}

func TestResolveUnifiedDest_SameNameNoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon", false)
	if got != current {
		t.Errorf("got %q, want %q", got, current)
	}
	if renamed {
		t.Error("renamed should be false when name is already correct")
	}
}

// TestResolveUnifiedDest_Collision_NoOverwrite tests migration context
// (allowOverwrite=false): a pre-existing file at the target name must not be
// overwritten — the result is bumped to the next free numbered slot.
func TestResolveUnifiedDest_Collision_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file at the target name (a different game).
	if err := os.WriteFile(filepath.Join(dir, "Doomslinger Dungeon.gb"), []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon", false)
	want := filepath.Join(dir, "Doomslinger Dungeon (2).gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true")
	}
}

// TestResolveUnifiedDest_Collision_AllowOverwrite tests download context
// (allowOverwrite=true): when the target exists and currentPath is not already a
// numbered slot, the target is returned directly so os.Rename can replace it.
func TestResolveUnifiedDest_Collision_AllowOverwrite(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "game-upload.gb")
	if err := os.WriteFile(current, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	// Existing file at the unified name (e.g. previous download of the same game).
	if err := os.WriteFile(filepath.Join(dir, "Doomslinger Dungeon.gb"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon", true)
	want := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true — caller will overwrite with os.Rename")
	}
}

// TestResolveUnifiedDest_CollisionCurrentPathIsSlot tests that a re-download
// of a game already assigned to a numbered slot (allowOverwrite=true) keeps that
// slot rather than overwriting the primary name held by a different game.
func TestResolveUnifiedDest_CollisionCurrentPathIsSlot(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Doomslinger Dungeon (2).gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	// "Doomslinger Dungeon.gb" is held by a different game.
	if err := os.WriteFile(filepath.Join(dir, "Doomslinger Dungeon.gb"), []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon", true)
	if got != current {
		t.Errorf("got %q, want %q (currentPath)", got, current)
	}
	if renamed {
		t.Error("renamed should be false — file is already at best available slot")
	}
}

func TestResolveUnifiedDest_EmptyTitle_NoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	got, renamed := roms.ResolveUnifiedDest(current, "", false)
	if got != current {
		t.Errorf("empty title: got %q, want %q", got, current)
	}
	if renamed {
		t.Error("empty title: renamed should be false")
	}
}
