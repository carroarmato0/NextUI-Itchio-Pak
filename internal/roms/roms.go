package roms

import (
	"path/filepath"
	"strings"
)

type Upload struct {
	Filename      string
	URL           string
	UploadID      string // itch.io upload ID (API-based paid download)
	DownloadKeyID string // itch.io download key ID (API-based paid download)
	NeedsFormat   bool   // true if user must choose the format (GB, GBC, or ZIP)
}

func ScoreUpload(filename string) int {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".gbc":
		return 2
	case ".gb":
		return 1
	default:
		return 0
	}
}

// GBADir is the default NextUI GBA ROM directory (uses the built-in GBA emulator).
const GBADir = "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"

// GBAMGBADir is the alternative NextUI GBA ROM directory (uses the MGBA emulator).
const GBAMGBADir = "/mnt/SDCARD/Roms/Game Boy Advance (MGBA)/"

func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	case ".gba":
		return GBADir
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
