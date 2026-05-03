package roms

import (
	"path/filepath"
	"strings"
)

// RomDirToSaveTag maps a ROM directory name to its NextUI save tag.
// Returns "" for unrecognised directories.
func RomDirToSaveTag(romDestPath string) string {
	switch filepath.Base(filepath.Dir(romDestPath)) {
	case "Game Boy (GB)":
		return "GB"
	case "Game Boy Color (GBC)":
		return "GBC"
	case "Game Boy Advance (GBA)":
		return "GBA"
	default:
		return ""
	}
}

// SaveGamePath derives the SRAM save file path for a downloaded ROM.
//
// saveFormat:
//   0 = MinUI (default)   — full ROM filename + ".sav"  (e.g. Game.gb.sav)
//   1 = Retroarch SRM     — extension stripped + ".srm" (e.g. Game.srm)
//   2 = Generic           — extension stripped + ".sav" (e.g. Game.sav)
//   3 = Retroarch SRM     — same as 1, uncompressed
//
// innerFilename: when the ROM is a .zip and NextUI's useExtractedFileName is
// enabled, pass the filename of the ROM inside the zip. Only affects format 0
// output; formats 1–3 produce the same result either way.
//
// Returns "" for unrecognised ROM directories.
func SaveGamePath(romDestPath string, saveFormat int, innerFilename string) string {
	tag := RomDirToSaveTag(romDestPath)
	if tag == "" {
		return ""
	}
	baseName := filepath.Base(romDestPath)
	if innerFilename != "" && saveFormat == 0 {
		baseName = innerFilename
	}
	ext := filepath.Ext(baseName)
	savesDir := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(romDestPath))), "Saves")
	switch saveFormat {
	case 0:
		return filepath.Join(savesDir, tag, baseName+".sav")
	case 1, 3:
		stem := strings.TrimSuffix(baseName, ext)
		return filepath.Join(savesDir, tag, stem+".srm")
	case 2:
		stem := strings.TrimSuffix(baseName, ext)
		return filepath.Join(savesDir, tag, stem+".sav")
	default:
		return ""
	}
}
