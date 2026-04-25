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
