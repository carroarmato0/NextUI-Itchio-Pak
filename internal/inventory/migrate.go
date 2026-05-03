package inventory

import (
	"fmt"
	"os"
	"strings"
)

// MigrateFormats carries the user's configured save and state format indices,
// read from /mnt/SDCARD/.userdata/shared/minuisettings.txt before calling MigrateFile.
type MigrateFormats struct {
	SaveFormat           int  // 0=MinUI, 1=Retroarch SRM compressed, 2=Generic, 3=Retroarch SRM uncompressed
	StateFormat          int  // 0=MinUI, 1/2=Retroarch-ish (legacy), 3/4=Retroarch
	UseExtractedFileName bool // mirrors useExtractedFileName from minuisettings.txt
}

// NXSettingsPath is the on-device path to NextUI's shared settings file.
const NXSettingsPath = "/mnt/SDCARD/.userdata/shared/minuisettings.txt"

// ReadMigrateFormats reads saveFormat, stateFormat, and useExtractedFileName
// from path. Missing or unreadable file returns all-zero (MinUI defaults).
func ReadMigrateFormats(path string) MigrateFormats {
	data, err := os.ReadFile(path)
	if err != nil {
		return MigrateFormats{}
	}
	var f MigrateFormats
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
