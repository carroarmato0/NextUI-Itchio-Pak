package roms

import (
	"path/filepath"
	"strings"
)

type FileKind int

const (
	KindOther FileKind = iota
	KindROM            // .gb .gbc .gba
	KindMusic          // .mp3 .ogg .flac .wav .opus .mod .xm .s3m .it
)

// ZIPEntry is one file from a ZIP's central directory.
type ZIPEntry struct {
	Name string
	Kind FileKind
	Size uint64 // uncompressed bytes
}

// ZIPManifest is the classified contents of a ZIP file.
type ZIPManifest struct {
	Entries []ZIPEntry
}

var romExts = map[string]bool{
	".gb": true, ".gbc": true, ".gba": true,
}

var musicExts = map[string]bool{
	".mp3": true, ".ogg": true, ".flac": true, ".wav": true, ".opus": true,
	".mod": true, ".xm": true, ".s3m": true, ".it": true,
}

func ClassifyEntry(name string) FileKind {
	ext := strings.ToLower(filepath.Ext(name))
	if romExts[ext] {
		return KindROM
	}
	if musicExts[ext] {
		return KindMusic
	}
	return KindOther
}

func (m ZIPManifest) HasROMs() bool {
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			return true
		}
	}
	return false
}

func (m ZIPManifest) HasMusic() bool {
	for _, e := range m.Entries {
		if e.Kind == KindMusic {
			return true
		}
	}
	return false
}

func (m ZIPManifest) ROMCount() int {
	n := 0
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			n++
		}
	}
	return n
}

func (m ZIPManifest) MusicCount() int {
	n := 0
	for _, e := range m.Entries {
		if e.Kind == KindMusic {
			n++
		}
	}
	return n
}

// IsSingleROMOnly reports whether the manifest contains exactly one ROM and no music.
func (m ZIPManifest) IsSingleROMOnly() bool {
	return m.ROMCount() == 1 && !m.HasMusic()
}

func (m ZIPManifest) ROMsByExt() map[string][]ZIPEntry {
	groups := make(map[string][]ZIPEntry)
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			ext := strings.ToLower(filepath.Ext(e.Name))
			groups[ext] = append(groups[ext], e)
		}
	}
	return groups
}

func (m ZIPManifest) HasDuplicateROMExt() bool {
	for _, entries := range m.ROMsByExt() {
		if len(entries) > 1 {
			return true
		}
	}
	return false
}
