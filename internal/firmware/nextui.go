package firmware

import (
	"os"
	"path/filepath"
)

// nextUISDCard is the SD card mount point on every NextUI platform
// (workspace/tg5040/platform/platform.h:156, tg5050:151); RES_PATH and
// SHARED_USERDATA_PATH are derived from it in common/defines.h:19,21.
const nextUISDCard = "/mnt/SDCARD"

// nextUIROMFolders maps our system keys onto NextUI's ROM folder display names.
// The name is load-bearing beyond placement: NextUI derives the emulator and the
// save tag from it, and roms.RomCoreInfo matches on it in reverse.
var nextUIROMFolders = map[string]string{
	SysGB:       "Game Boy (GB)",
	SysGBC:      "Game Boy Color (GBC)",
	SysGBA:      "Game Boy Advance (GBA)",
	SysGBAAlt:   "Game Boy Advance (MGBA)",
	SysNES:      "Nintendo Entertainment System (FC)",
	SysGenesis:  "Sega Genesis (MD)",
	SysPico8:    "Pico-8 (P8)",
	SysPico8Alt: "Pico-8 (PICO)",
}

// nextUIDeviceLabels are the human-readable names for NextUI platform codes.
var nextUIDeviceLabels = map[string]string{
	"tg5040": "TrimUI Brick / Smart Pro",
	"tg5050": "TrimUI Smart Pro S",
	"my355":  "Miyoo Flip",
}

func newNextUI(prefix string) *Env {
	root := filepath.Join(prefix, nextUISDCard)

	romDirs := make(map[string]string, len(nextUIROMFolders))
	for key, folder := range nextUIROMFolders {
		romDirs[key] = filepath.Join(root, "Roms", folder) + "/"
	}

	platform := os.Getenv("PLATFORM")
	label := nextUIDeviceLabels[platform]
	if label == "" {
		label = "unknown device"
	}

	sharedUserdata := filepath.Join(root, ".userdata", "shared")

	// NextUI writes pak logs under the platform directory. Without PLATFORM we
	// are almost certainly not on a device, so keep the log beside app state.
	data := dataDirFor()
	logPath := filepath.Join(data, "itchio.log")
	if platform != "" {
		logPath = filepath.Join(root, ".userdata", platform, "logs", "itchio.log")
	}

	return &Env{
		kind:        KindNextUI,
		device:      platform,
		deviceLabel: label,
		prefix:      prefix,

		root:    root,
		romDirs: romDirs,

		musicRoot:       filepath.Join(root, "Music") + "/",
		browseRoot:      root,
		musicBrowseRoot: filepath.Join(root, "Music"),

		sharedUserdata:    sharedUserdata,
		settingsFile:      filepath.Join(sharedUserdata, "minuisettings.txt"),
		builtinPaletteDir: filepath.Join(root, ".system", "res", "palettes"),
		userPaletteDir:    filepath.Join(root, "Palettes"),
		versionFile:       filepath.Join(root, ".system", "version.txt"),

		dataDir: data,
		logPath: logPath,

		caps: Caps{
			NextUIPalette:     true,
			MinUISaveFormats:  true,
			SaveStateSync:     true,
			GBAEmulatorChoice: true,
			Pico8CoreChoice:   true,
		},
	}
}

// newHost is the environment on a developer machine or in CI. It deliberately
// exposes no device paths: code that would write to the SD card gets "" and
// must handle it, which is what surfaces missing guards in tests.
func newHost(prefix string) *Env {
	data := dataDirFor()
	return &Env{
		kind:        KindHost,
		device:      "",
		deviceLabel: "host",
		prefix:      prefix,

		romDirs: map[string]string{},

		dataDir: data,
		logPath: filepath.Join(data, "itchio.log"),

		caps: Caps{},
	}
}
