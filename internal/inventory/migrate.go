package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// MigrateFormats carries the user's configured save and state format indices,
// read from the firmware's settings file before calling MigrateFile.
type MigrateFormats struct {
	SaveFormat           int  // 0=MinUI, 1=Retroarch SRM compressed, 2=Generic, 3=Retroarch SRM uncompressed
	StateFormat          int  // 0=MinUI, 1/2=Retroarch-ish (legacy), 3/4=Retroarch
	UseExtractedFileName bool // mirrors useExtractedFileName from minuisettings.txt
	// Known reports whether these values were actually read from a settings
	// file. The zero value of the fields above means "MinUI defaults", which is
	// a fair assumption on NextUI when the file is simply absent, but a wrong
	// one on firmware that has no such file at all. Consult
	// firmware.Caps().MinUISaveFormats before acting on an unknown result.
	Known bool
}

// SettingsPath returns the firmware's settings file, or "" when it has none.
func SettingsPath() string { return firmware.Active().SettingsFile() }

// ReadMigrateFormats reads saveFormat, stateFormat, and useExtractedFileName
// from path. A missing or unreadable file returns a zero value with Known
// false, so callers can tell "the user chose MinUI defaults" apart from "this
// firmware does not have save formats".
func ReadMigrateFormats(path string) MigrateFormats {
	if path == "" {
		return MigrateFormats{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return MigrateFormats{}
	}
	f := MigrateFormats{Known: true}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		var n int
		if _, err := fmt.Sscanf(line, "saveFormat=%d", &n); err == nil {
			f.SaveFormat = n
			continue
		}
		if _, err := fmt.Sscanf(line, "stateFormat=%d", &n); err == nil {
			f.StateFormat = n
			continue
		}
		if _, err := fmt.Sscanf(line, "useExtractedFileName=%d", &n); err == nil {
			f.UseExtractedFileName = n != 0
		}
	}
	return f
}

// SaveDataCallback surfaces save-game and save-state prompts to the caller.
// In production the UI screens implement this interface.
// In tests a stub struct returns predetermined answers.
type SaveDataCallback interface {
	AskRenameExistingSave(savePath string) bool
	AskOverwriteExistingSave(newSavePath string) bool
	AskRenameExistingStates(statePaths []string) bool
}

// MigrateResult reports what MigrateFile changed.
type MigrateResult struct {
	ROMRenamed      bool
	CoverArtRenamed bool
	SaveRenamed     bool
	SaveSkipped     bool
	StatesRenamed   []string
	StatesSkipped   []string
	NewDestPath     string
}

// MigrateFile renames a downloaded ROM (and optionally its save/state files)
// between the upstream filename and the game-title-based name.
//
// enable=true:  rename to game title (UnifiedName=true).
// enable=false: revert to upstream filename (UnifiedName=false).
func MigrateFile(
	inv *Inventory,
	invPath string,
	gameURL string,
	file DownloadedFile,
	gameTitle string,
	enable bool,
	formats MigrateFormats,
	cb SaveDataCallback,
) (MigrateResult, error) {
	var res MigrateResult

	entry, ok := inv.Lookup(gameURL)
	if !ok {
		return res, errors.New("migrate: game not found in inventory")
	}

	currentPath := file.DestPath

	// Determine target path.
	var targetPath string
	if enable {
		targetPath, _ = roms.ResolveUnifiedDest(currentPath, gameTitle, false)
	} else {
		if !file.UnifiedName {
			return res, nil // already disabled — no-op
		}
		targetPath = filepath.Join(filepath.Dir(currentPath), file.Filename)
	}

	// No-op when path is already correct.
	if targetPath == currentPath {
		file.UnifiedName = enable
		inv.UpdateFile(gameURL, currentPath, file)
		_ = inv.Save(invPath)
		return res, nil
	}

	// Resolve inner filename for zip+useExtractedFileName.
	var innerFilename string
	if formats.UseExtractedFileName && filepath.Ext(currentPath) == ".zip" {
		innerFilename = roms.ZipInnerFilename(currentPath)
	}

	// --- Save game handling ---
	oldSavePath := roms.SaveGamePath(currentPath, formats.SaveFormat, innerFilename)
	newSavePath := roms.SaveGamePath(targetPath, formats.SaveFormat, innerFilename)
	var renameThisSave, skipThisSave bool

	if oldSavePath != "" && oldSavePath != newSavePath {
		if _, err := os.Stat(oldSavePath); err == nil {
			// Save exists at old path. Check for overwrite conflict first.
			if _, err2 := os.Stat(newSavePath); err2 == nil {
				// Save exists at new path too.
				if !cb.AskRenameExistingSave(oldSavePath) {
					skipThisSave = true
				} else if !cb.AskOverwriteExistingSave(newSavePath) {
					return res, fmt.Errorf("migrate: cancelled by user (save overwrite declined)")
				} else {
					renameThisSave = true
				}
			} else {
				if cb.AskRenameExistingSave(oldSavePath) {
					renameThisSave = true
				} else {
					skipThisSave = true
				}
			}
		}
	}

	// --- Save state handling ---
	coreTag, coreName := roms.RomCoreInfo(currentPath)
	var existingStatePaths []string
	if coreTag != "" {
		allOldStates := roms.SaveStatePaths(currentPath, formats.StateFormat, innerFilename, coreTag, coreName)
		for _, sp := range allOldStates {
			if _, err := os.Stat(sp); err == nil {
				existingStatePaths = append(existingStatePaths, sp)
			}
		}
	}
	renameStates := len(existingStatePaths) > 0 && cb.AskRenameExistingStates(existingStatePaths)

	// --- Rename ROM ---
	if err := os.Rename(currentPath, targetPath); err != nil {
		return res, fmt.Errorf("migrate: rename ROM: %w", err)
	}
	res.ROMRenamed = true
	res.NewDestPath = targetPath

	// --- Rename cover art (non-fatal) ---
	oldCover := CoverArtPath(entry.CoverURL, currentPath)
	if oldCover != "" {
		newCover := CoverArtPath(entry.CoverURL, targetPath)
		if err := os.Rename(oldCover, newCover); err != nil {
			logger.Warn("migrate: cover art rename failed: %v", err)
		} else {
			res.CoverArtRenamed = true
		}
	}

	// --- Rename save ---
	if renameThisSave {
		if err := os.Rename(oldSavePath, newSavePath); err != nil {
			logger.Warn("migrate: save rename failed: %v", err)
			res.SaveSkipped = true
		} else {
			res.SaveRenamed = true
		}
	} else if skipThisSave {
		res.SaveSkipped = true
	}

	// --- Rename state files (non-fatal per file) ---
	if renameStates {
		allOldStates := roms.SaveStatePaths(currentPath, formats.StateFormat, innerFilename, coreTag, coreName)
		allNewStates := roms.SaveStatePaths(targetPath, formats.StateFormat, innerFilename, coreTag, coreName)
		for _, oldState := range existingStatePaths {
			idx := -1
			for j, p := range allOldStates {
				if p == oldState {
					idx = j
					break
				}
			}
			if idx < 0 || idx >= len(allNewStates) {
				res.StatesSkipped = append(res.StatesSkipped, oldState)
				continue
			}
			newState := allNewStates[idx]
			if err := os.Rename(oldState, newState); err != nil {
				logger.Warn("migrate: state rename %s: %v", oldState, err)
				res.StatesSkipped = append(res.StatesSkipped, oldState)
			} else {
				res.StatesRenamed = append(res.StatesRenamed, newState)
			}
		}
	}

	// --- Update inventory ---
	file.DestPath = targetPath
	file.UnifiedName = enable
	inv.UpdateFile(gameURL, currentPath, file)
	if err := inv.Save(invPath); err != nil {
		logger.Warn("migrate: save inventory: %v", err)
	}
	return res, nil
}
