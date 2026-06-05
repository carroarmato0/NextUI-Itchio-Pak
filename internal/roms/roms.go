package roms

import (
	"path/filepath"
	"strings"
)

// ROMExt returns the effective ROM extension for filename.
// For Pico-8 cartridges with the compound extension .p8.png it returns ".p8.png"
// rather than the ".png" that filepath.Ext would return.
func ROMExt(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".p8.png") {
		return ".p8.png"
	}
	return filepath.Ext(filename)
}

type Upload struct {
	Filename      string
	URL           string
	UploadID      string // itch.io upload ID (API-based paid download)
	DownloadKeyID string // itch.io download key ID (API-based paid download)
	NeedsFormat   bool   // true if user must choose the format (GB, GBC, or ZIP)
}

func ScoreUpload(filename string) int {
	switch strings.ToLower(ROMExt(filename)) {
	case ".gbc", ".p8.png":
		return 2
	case ".gb", ".gba", ".nes", ".md", ".gen", ".smd", ".p8":
		return 1
	default:
		return 0
	}
}

// GBADir is the default NextUI GBA ROM directory (uses the built-in GBA emulator).
const GBADir = "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"

// GBAMGBADir is the alternative NextUI GBA ROM directory (uses the MGBA emulator).
const GBAMGBADir = "/mnt/SDCARD/Roms/Game Boy Advance (MGBA)/"

// NESDir is the NextUI NES/Famicom ROM directory.
const NESDir = "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"

// GenesisDir is the NextUI Sega Genesis/Mega Drive ROM directory.
const GenesisDir = "/mnt/SDCARD/Roms/Sega Genesis (MD)/"

// Pico8Dir is the NextUI Pico-8 ROM directory.
const Pico8Dir = "/mnt/SDCARD/Roms/Pico-8 (P8)/"

func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	case ".gba":
		return GBADir
	case ".nes":
		return NESDir
	case ".md", ".gen", ".smd":
		return GenesisDir
	case ".p8", ".p8.png":
		return Pico8Dir
	case ".zip":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	default:
		return ""
	}
}

func SelectBest(uploads []Upload) *Upload {
	var best *Upload
	bestScore := 0
	for i := range uploads {
		s := ScoreUpload(uploads[i].Filename)
		if s > bestScore {
			bestScore = s
			best = &uploads[i]
		}
	}
	return best
}

// MusicBaseDir is the root directory for all extracted game soundtracks.
const MusicBaseDir = "/mnt/SDCARD/Music/"

// MusicDestinationDir returns the target directory for a game's music files.
func MusicDestinationDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return MusicBaseDir + safe + "/"
}

// Pico8GameDir returns the subdirectory for a Pico-8 game that ships with
// multiple files (.p8/.p8.png/.lua). All game files are extracted here.
func Pico8GameDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8Dir + safe + "/"
}
