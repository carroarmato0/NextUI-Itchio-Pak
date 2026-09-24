package roms

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
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
	Filename    string
	URL         string
	UploadID    string // itch.io upload ID (API-based download)
	NeedsFormat bool   // true if user must choose the format (GB, GBC, or ZIP)
	// Session is set when the upload is fetched through the itch.io API
	// rather than the web download flow. Every upload from one listing
	// shares it, so all requests of one install count as one download.
	Session *DownloadSession
}

// ViaAPI reports whether the upload is downloaded through the itch.io API.
func (u Upload) ViaAPI() bool { return u.Session != nil }

// DownloadSession groups the requests of one install — range reads for ZIP
// inspection, the magic-byte probe, the download itself — so itch.io counts
// them as a single download. The itchio package creates the server-side
// session lazily, on the first request that needs it.
type DownloadSession struct {
	GameID string
	KeyID  string // download key ID; empty for free games

	mu      sync.Mutex
	uuid    string
	created bool // a create attempt was made, successful or not
}

// NewDownloadSession returns a session for one install of gameID.
func NewDownloadSession(gameID, keyID string) *DownloadSession {
	return &DownloadSession{GameID: gameID, KeyID: keyID}
}

// UUID returns the session ID, calling create on first use. A failed create
// is not retried: the download still works without a session, it just is
// not grouped.
func (s *DownloadSession) UUID(create func() (string, error)) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.created {
		return s.uuid, nil
	}
	s.created = true
	id, err := create()
	if err != nil {
		return "", err
	}
	s.uuid = id
	return id, nil
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

// The ROM destinations below used to be hardcoded NextUI paths. They now come
// from internal/firmware, because muOS puts ROMs somewhere else entirely and
// lets the user name the folders. The functions keep their original shape so
// the ~15 call sites in internal/ui did not have to change.

// GBADir is the primary GBA ROM directory (the firmware's default emulator).
func GBADir() string { return firmware.Active().ROMDirForSystem(firmware.SysGBA) }

// GBAMGBADir is the alternative GBA ROM directory, for firmware that ships a
// second GBA emulator in its own folder. Empty when there is no such folder —
// guard with firmware.Active().Caps().GBAEmulatorChoice before offering it.
func GBAMGBADir() string { return firmware.Active().ROMDirForSystem(firmware.SysGBAAlt) }

// NESDir is the NES/Famicom ROM directory.
func NESDir() string { return firmware.Active().ROMDirForSystem(firmware.SysNES) }

// GenesisDir is the Sega Genesis/Mega Drive ROM directory.
func GenesisDir() string { return firmware.Active().ROMDirForSystem(firmware.SysGenesis) }

// Pico8ROMDir returns the Pico-8 ROM directory for the given core.
// core: "fakeo8" | "pico8" — any other value falls back to "fakeo8".
func Pico8ROMDir(core string) string { return firmware.Active().Pico8Dir(core) }

// DestinationDir returns the directory a ROM with this extension belongs in,
// or "" for extensions we do not place.
func DestinationDir(ext, pico8Core string) string {
	return firmware.Active().ROMDir(ext, pico8Core)
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
func MusicBaseDir() string { return firmware.Active().MusicRoot() }

// MusicDestinationDir returns the target directory for a game's music files.
func MusicDestinationDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return MusicBaseDir() + safe + "/"
}

// Pico8GameSubDir returns the subdirectory for a Pico-8 game that ships with
// multiple files (.p8/.p8.png/.lua). All game files are extracted here.
func Pico8GameSubDir(core, gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8ROMDir(core) + safe + "/"
}
