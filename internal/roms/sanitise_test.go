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

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
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
	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
	if got != current {
		t.Errorf("got %q, want %q", got, current)
	}
	if renamed {
		t.Error("renamed should be false when name is already correct")
	}
}

func TestResolveUnifiedDest_Collision(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file at the target name
	existing := filepath.Join(dir, "Doomslinger Dungeon.gb")
	if err := os.WriteFile(existing, []byte("other"), 0644); err != nil {
		t.Fatal(err)
	}

	got, renamed := roms.ResolveUnifiedDest(current, "Doomslinger Dungeon")
	want := filepath.Join(dir, "Doomslinger Dungeon (2).gb")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !renamed {
		t.Error("renamed should be true")
	}
}

func TestResolveUnifiedDest_EmptyTitle_NoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "Game Boy ROM.gb")
	if err := os.WriteFile(current, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	got, renamed := roms.ResolveUnifiedDest(current, "")
	if got != current {
		t.Errorf("empty title: got %q, want %q", got, current)
	}
	if renamed {
		t.Error("empty title: renamed should be false")
	}
}
