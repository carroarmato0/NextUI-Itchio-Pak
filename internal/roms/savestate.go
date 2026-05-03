package roms

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SaveStatePaths returns the set of save-state paths that could exist for a ROM.
// The caller should filter to only paths that exist on disk before prompting.
//
// stateFormat:
//   0 = MinUI      — <full>.st0 … .st9   (10 paths; .st9 = auto-resume)
//   1/2 = Retroarch-ish — <stem>.state.1 … .state.8 + .state.auto  (9 paths)
//   3/4 = Retroarch     — <stem>.state, .state1…8, .state.auto      (10 paths)
//
// innerFilename: same semantics as SaveGamePath — only affects format 0.
// coreTag / coreName: must match the NextUI core directory (e.g. "GB", "gambatte").
// Returns nil for empty coreTag/coreName or unrecognised ROM directories.
func SaveStatePaths(romDestPath string, stateFormat int, innerFilename, coreTag, coreName string) []string {
	if coreTag == "" || coreName == "" {
		return nil
	}
	statesDir := filepath.Join("/mnt/SDCARD/.userdata/shared", coreTag+"-"+coreName)
	baseName := filepath.Base(romDestPath)
	if innerFilename != "" && stateFormat == 0 {
		baseName = innerFilename
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)

	switch stateFormat {
	case 0:
		paths := make([]string, 10)
		for i := 0; i <= 9; i++ {
			paths[i] = filepath.Join(statesDir, fmt.Sprintf("%s.st%d", baseName, i))
		}
		return paths
	case 1, 2:
		paths := make([]string, 9)
		for i := 1; i <= 8; i++ {
			paths[i-1] = filepath.Join(statesDir, fmt.Sprintf("%s.state.%d", stem, i))
		}
		paths[8] = filepath.Join(statesDir, stem+".state.auto")
		return paths
	case 3, 4:
		paths := make([]string, 10)
		paths[0] = filepath.Join(statesDir, stem+".state")
		for i := 1; i <= 8; i++ {
			paths[i] = filepath.Join(statesDir, fmt.Sprintf("%s.state%d", stem, i))
		}
		paths[9] = filepath.Join(statesDir, stem+".state.auto")
		return paths
	default:
		return nil
	}
}

// RomCoreInfo returns the coreTag and coreName for a ROM path.
// Returns ("", "") for unrecognised directories.
func RomCoreInfo(romDestPath string) (coreTag, coreName string) {
	switch filepath.Base(filepath.Dir(romDestPath)) {
	case "Game Boy (GB)":
		return "GB", "gambatte"
	case "Game Boy Color (GBC)":
		return "GBC", "gambatte"
	case "Game Boy Advance (GBA)":
		return "GBA", "gpsp"
	default:
		return "", ""
	}
}
