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
	".nes": true,
	".md": true, ".gen": true, ".smd": true,
	".p8": true, ".p8.png": true,
}

var musicExts = map[string]bool{
	".mp3": true, ".ogg": true, ".flac": true, ".wav": true, ".opus": true,
	".mod": true, ".xm": true, ".s3m": true, ".it": true,
}

func ClassifyEntry(name string) FileKind {
	// macOS resource fork stubs start with "._"; never treat them as playable files.
	if strings.HasPrefix(filepath.Base(name), "._") {
		return KindOther
	}
	ext := strings.ToLower(ROMExt(name))
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

// HasOtherFiles reports whether the manifest contains any entry that is neither
// a ROM nor a music file (e.g. images, videos, booklets, READMEs).
func (m ZIPManifest) HasOtherFiles() bool {
	for _, e := range m.Entries {
		if e.Kind == KindOther {
			return true
		}
	}
	return false
}

func (m ZIPManifest) ROMsByExt() map[string][]ZIPEntry {
	groups := make(map[string][]ZIPEntry)
	for _, e := range m.Entries {
		if e.Kind == KindROM {
			ext := strings.ToLower(ROMExt(e.Name))
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

// HasPico8ROMs reports whether the manifest contains at least one Pico-8
// cartridge file (.p8 or .p8.png).
func (m ZIPManifest) HasPico8ROMs() bool {
	return len(m.ROMsByExt()[".p8"])+len(m.ROMsByExt()[".p8.png"]) > 0
}

// IsPico8MultiFileGame reports whether the manifest requires multi-file
// extraction into a game subdirectory: either multiple Pico-8 carts, or a
// single .p8 text-format cart with .lua files (which may be referenced at
// runtime via #include).
//
// A single .p8.png with .lua files does NOT qualify: .p8.png is a compiled,
// self-contained cartridge — the bundled .lua files are source artifacts that
// the emulator never reads at runtime.
func (m ZIPManifest) IsPico8MultiFileGame() bool {
	byExt := m.ROMsByExt()
	p8Count := len(byExt[".p8"]) + len(byExt[".p8.png"])
	if p8Count > 1 {
		return true
	}
	// Single cart: only .p8 text-format carts may need Lua includes at runtime.
	return len(byExt[".p8"]) > 0 && m.HasLuaFiles()
}

// HasLuaFiles reports whether the manifest contains any .lua file.
// Pico-8 games sometimes ship with Lua support scripts that must live
// alongside the cartridges.
func (m ZIPManifest) HasLuaFiles() bool {
	for _, e := range m.Entries {
		if strings.HasSuffix(strings.ToLower(e.Name), ".lua") {
			return true
		}
	}
	return false
}
