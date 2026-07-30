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

// TestUnifiedCollisions_RealCase reproduces the Capybara Village report: two
// ROM uploads for one game, both renamed to the game title, the second
// overwriting the first and leaving the older build installed.
func TestUnifiedCollisions_RealCase(t *testing.T) {
	dir := "/mnt/SDCARD/Roms/Game Boy (GB)"
	paths := []string{
		filepath.Join(dir, "Capybara-Village-Update1.gb"),
		filepath.Join(dir, "Game-Jam-Submission-Version.gb"),
	}
	got := roms.UnifiedCollisions(paths, "Capybara Village")
	want := filepath.Join(dir, "Capybara Village.gb")
	if !got[want] {
		t.Errorf("UnifiedCollisions = %v, want %q flagged", got, want)
	}
}

// A single upload is the normal case and must still be unified.
func TestUnifiedCollisions_SingleFile(t *testing.T) {
	got := roms.UnifiedCollisions([]string{"/roms/GB/whatever.gb"}, "Some Game")
	if len(got) != 0 {
		t.Errorf("UnifiedCollisions = %v, want none for a single file", got)
	}
}

// Uploads that differ by extension resolve to different names, so they are not
// a collision and keep their unified names.
func TestUnifiedCollisions_DifferentExtensions(t *testing.T) {
	got := roms.UnifiedCollisions([]string{"/roms/x/a.gb", "/roms/x/b.gbc"}, "Some Game")
	if len(got) != 0 {
		t.Errorf("UnifiedCollisions = %v, want none across extensions", got)
	}
}

// Same name, different destination folders is also not a collision.
func TestUnifiedCollisions_DifferentDirs(t *testing.T) {
	got := roms.UnifiedCollisions([]string{"/roms/GB/a.gb", "/roms/GBC/b.gb"}, "Some Game")
	if len(got) != 0 {
		t.Errorf("UnifiedCollisions = %v, want none across directories", got)
	}
}

func TestUnifiedCollisions_EmptyTitle(t *testing.T) {
	got := roms.UnifiedCollisions([]string{"/roms/x/a.gb", "/roms/x/b.gb"}, "")
	if len(got) != 0 {
		t.Errorf("UnifiedCollisions = %v, want none when the title is empty", got)
	}
}
