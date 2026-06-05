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
		core string
		want string
	}{
		{".gbc", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".GBC", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".gb", "fakeo8", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
		{".gba", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
		{".nes", "fakeo8", "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
		{".md", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".gen", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".smd", "fakeo8", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
		{".p8", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".P8", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".p8.png", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{".p8", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{".p8.png", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{".zip", "fakeo8", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".unknown", "fakeo8", ""},
	}
	for _, tt := range tests {
		got := roms.DestinationDir(tt.ext, tt.core)
		if got != tt.want {
			t.Errorf("DestinationDir(%q, %q) = %q, want %q", tt.ext, tt.core, got, tt.want)
		}
	}
}

func TestPico8ROMDir(t *testing.T) {
	cases := []struct {
		core string
		want string
	}{
		{"fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{"pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
		{"", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
		{"other", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
	}
	for _, tc := range cases {
		got := roms.Pico8ROMDir(tc.core)
		if got != tc.want {
			t.Errorf("Pico8ROMDir(%q) = %q, want %q", tc.core, got, tc.want)
		}
	}
}

func TestPico8GameSubDir(t *testing.T) {
	cases := []struct {
		core  string
		title string
		want  string
	}{
		{"fakeo8", "Poom", "/mnt/SDCARD/Roms/Pico-8 (P8)/Poom/"},
		{"pico8", "Poom", "/mnt/SDCARD/Roms/Pico-8 (PICO)/Poom/"},
		{"fakeo8", "Celeste", "/mnt/SDCARD/Roms/Pico-8 (P8)/Celeste/"},
	}
	for _, tc := range cases {
		got := roms.Pico8GameSubDir(tc.core, tc.title)
		if got != tc.want {
			t.Errorf("Pico8GameSubDir(%q, %q) = %q, want %q", tc.core, tc.title, got, tc.want)
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

func TestPico8GameSubDirLegacy(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Celeste Classic", "/mnt/SDCARD/Roms/Pico-8 (P8)/Celeste Classic/"},
		{"Game: Title?", "/mnt/SDCARD/Roms/Pico-8 (P8)/Game Title/"},
		{"", "/mnt/SDCARD/Roms/Pico-8 (P8)/Unknown/"},
	}
	for _, tt := range tests {
		got := roms.Pico8GameSubDir("fakeo8", tt.title)
		if got != tt.want {
			t.Errorf("Pico8GameSubDir(%q) = %q, want %q", tt.title, got, tt.want)
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
