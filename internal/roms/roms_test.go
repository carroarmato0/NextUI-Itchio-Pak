package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestScoreUpload(t *testing.T) {
	tests := []struct {
		filename string
		want     int
	}{
		{"game.gbc", 2},
		{"game.GBC", 2},
		{"game.gb", 1},
		{"game.GB", 1},
		{"game.gba", 1},
		{"game.nes", 1},
		{"game.NES", 1},
		{"game.md", 1},
		{"game.gen", 1},
		{"game.smd", 1},
		{"game.p8.png", 2},
		{"game.P8.PNG", 2},
		{"game.p8", 1},
		{"game.P8", 1},
		{"game.zip", 0},
		{"game.pocket", 0},
		{"game.pdf", 0},
	}
	for _, tt := range tests {
		got := roms.ScoreUpload(tt.filename)
		if got != tt.want {
			t.Errorf("ScoreUpload(%q) = %d, want %d", tt.filename, got, tt.want)
		}
	}
}

func TestDestinationDir(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".gbc", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".GBC", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".gb", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
		{".GB", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
		{".gba", "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
		{".GBA", "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
		{".nes", "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
		{".NES", "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
		{".md", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".MD", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".gen", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".smd", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".p8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".P8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".p8.png", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".zip", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".ZIP", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
	}
	for _, tt := range tests {
		got := roms.DestinationDir(tt.ext)
		if got != tt.want {
			t.Errorf("DestinationDir(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestSelectBest(t *testing.T) {
	uploads := []roms.Upload{
		{Filename: "game.pdf", URL: "u1"},
		{Filename: "game.gb", URL: "u2"},
		{Filename: "game.gbc", URL: "u3"},
	}
	got := roms.SelectBest(uploads)
	if got == nil || got.URL != "u3" {
		t.Errorf("SelectBest: expected gbc upload (u3), got %v", got)
	}
}

func TestSelectBestNoROMs(t *testing.T) {
	uploads := []roms.Upload{
		{Filename: "manual.pdf", URL: "u1"},
	}
	got := roms.SelectBest(uploads)
	if got != nil {
		t.Errorf("SelectBest with no ROMs: expected nil, got %v", got)
	}
}

func TestMusicDestinationDir(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Solastra", "/mnt/SDCARD/Music/Solastra/"},
		{"Game: Title?", "/mnt/SDCARD/Music/Game Title/"},
		{"", "/mnt/SDCARD/Music/Unknown/"},
	}
	for _, tt := range tests {
		got := roms.MusicDestinationDir(tt.title)
		if got != tt.want {
			t.Errorf("MusicDestinationDir(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestPico8GameDir(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Celeste Classic", "/mnt/SDCARD/Roms/Pico-8 (P8)/Celeste Classic/"},
		{"Game: Title?", "/mnt/SDCARD/Roms/Pico-8 (P8)/Game Title/"},
		{"", "/mnt/SDCARD/Roms/Pico-8 (P8)/Unknown/"},
	}
	for _, tt := range tests {
		got := roms.Pico8GameDir(tt.title)
		if got != tt.want {
			t.Errorf("Pico8GameDir(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestROMExt(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"game.p8.png", ".p8.png"},
		{"game.P8.PNG", ".p8.png"},
		{"GAME.P8.PNG", ".p8.png"},
		{"game.p8", ".p8"},
		{"game.gbc", ".gbc"},
		{"game.png", ".png"},
		{"game", ""},
	}
	for _, tt := range tests {
		got := roms.ROMExt(tt.filename)
		if got != tt.want {
			t.Errorf("ROMExt(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}
