package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestClassifyEntry(t *testing.T) {
	tests := []struct {
		name string
		want roms.FileKind
	}{
		{"game.gb", roms.KindROM},
		{"game.GB", roms.KindROM},
		{"game.gbc", roms.KindROM},
		{"game.GBC", roms.KindROM},
		{"game.gba", roms.KindROM},
		{"track01.mp3", roms.KindMusic},
		{"track01.MP3", roms.KindMusic},
		{"track01.ogg", roms.KindMusic},
		{"track01.flac", roms.KindMusic},
		{"track01.wav", roms.KindMusic},
		{"track01.opus", roms.KindMusic},
		{"track01.mod", roms.KindMusic},
		{"track01.xm", roms.KindMusic},
		{"track01.s3m", roms.KindMusic},
		{"track01.it", roms.KindMusic},
		{"readme.txt", roms.KindOther},
		{"cover.png", roms.KindOther},
		{"manual.pdf", roms.KindOther},
		{"noext", roms.KindOther},
	}
	for _, tt := range tests {
		got := roms.ClassifyEntry(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyEntry(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestZIPManifestHelpers(t *testing.T) {
	m := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game-v1.gbc", Kind: roms.KindROM},
		{Name: "game-v2.gbc", Kind: roms.KindROM},
		{Name: "game.gb", Kind: roms.KindROM},
		{Name: "track.mp3", Kind: roms.KindMusic},
	}}

	if !m.HasROMs() {
		t.Error("HasROMs() = false, want true")
	}
	if !m.HasMusic() {
		t.Error("HasMusic() = false, want true")
	}
	if m.ROMCount() != 3 {
		t.Errorf("ROMCount() = %d, want 3", m.ROMCount())
	}
	if m.MusicCount() != 1 {
		t.Errorf("MusicCount() = %d, want 1", m.MusicCount())
	}
	if m.IsSingleROMOnly() {
		t.Error("IsSingleROMOnly() = true, want false (has music + multiple ROMs)")
	}
	if !m.HasDuplicateROMExt() {
		t.Error("HasDuplicateROMExt() = false, want true (two .gbc entries)")
	}

	byExt := m.ROMsByExt()
	if len(byExt[".gbc"]) != 2 {
		t.Errorf("ROMsByExt()[.gbc] len = %d, want 2", len(byExt[".gbc"]))
	}
	if len(byExt[".gb"]) != 1 {
		t.Errorf("ROMsByExt()[.gb] len = %d, want 1", len(byExt[".gb"]))
	}
}

func TestIsSingleROMOnly(t *testing.T) {
	m := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gbc", Kind: roms.KindROM},
		{Name: "readme.txt", Kind: roms.KindOther},
	}}
	if !m.IsSingleROMOnly() {
		t.Error("IsSingleROMOnly() = false, want true (1 ROM, no music)")
	}
}

func TestEmptyManifest(t *testing.T) {
	m := roms.ZIPManifest{}
	if m.HasROMs() || m.HasMusic() || m.ROMCount() != 0 || m.MusicCount() != 0 {
		t.Error("empty manifest should have no ROMs or music")
	}
	if m.IsSingleROMOnly() {
		t.Error("empty manifest IsSingleROMOnly() should be false")
	}
}

func TestHasOtherFiles(t *testing.T) {
	bare := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gbc", Kind: roms.KindROM},
	}}
	if bare.HasOtherFiles() {
		t.Error("HasOtherFiles() = true for bare ROM ZIP, want false")
	}

	withExtras := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gbc", Kind: roms.KindROM},
		{Name: "cover.png", Kind: roms.KindOther},
		{Name: "trailer.mp4", Kind: roms.KindOther},
	}}
	if !withExtras.HasOtherFiles() {
		t.Error("HasOtherFiles() = false for ZIP with images/videos, want true")
	}

	withMusic := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gbc", Kind: roms.KindROM},
		{Name: "track.mp3", Kind: roms.KindMusic},
	}}
	if withMusic.HasOtherFiles() {
		t.Error("HasOtherFiles() = true for ROM+music ZIP (no KindOther), want false")
	}
}

func TestHasNoDuplicateROMExt(t *testing.T) {
	m := roms.ZIPManifest{Entries: []roms.ZIPEntry{
		{Name: "game.gb", Kind: roms.KindROM},
		{Name: "game.gbc", Kind: roms.KindROM},
		{Name: "readme.txt", Kind: roms.KindOther},
	}}
	if m.HasDuplicateROMExt() {
		t.Error("HasDuplicateROMExt() = true, want false (each extension appears once)")
	}
}
