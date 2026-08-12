package firmware

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// muOS exposes its configuration as a tree of single-value files, which is what
// its own shell helper GET_VAR reads. Reading them directly avoids shelling out.
const (
	muosMarkerDir  = "/opt/muos"
	muosDeviceConf = "/opt/muos/device/config"
	muosGlobalConf = "/opt/muos/config"

	// muosStoreDir is where muOS bind-mounts the MUOS/ subdirectories from
	// whichever card holds them. Applications are told to use this rather than
	// /mnt/mmc or /mnt/sdcard, which break on devices with a second card.
	muosStoreDir = "/run/muos/storage"

	// ROMs are the exception: they are not bind-mounted, and live in a ROMS
	// directory on each mounted card.
	muosROMSubdir = "ROMS"
)

// muosROMAliases lists the folder names accepted for each system, and the name
// created when none of them exist.
//
// muOS deliberately has no required ROM folder names — the documentation says
// folders "can be named whatever you want". The convention is the short key
// muOS maps to a display name in MUOS/info/name/folder.json, so the first entry
// is that key and the rest are names a user might plausibly have already.
// Matching is case-insensitive.
var muosROMAliases = map[string][]string{
	SysGB:      {"gb", "Nintendo Game Boy", "Game Boy", "gameboy"},
	SysGBC:     {"gbc", "Nintendo Game Boy Color", "Game Boy Color", "gameboycolor"},
	SysGBA:     {"gba", "Nintendo Game Boy Advance", "Game Boy Advance", "gameboyadvance"},
	SysNES:     {"nes", "fc", "Nintendo Entertainment System", "Nintendo Famicom", "Famicom"},
	SysGenesis: {"md", "genesis", "megadrive", "SEGA Mega Drive", "SEGA Genesis", "Mega Drive"},
	SysPico8:   {"pico8", "pico-8", "p8", "PICO-8"},
}

// muosDefaultDisplayNames mirrors the entries of muOS's own
// MUOS/info/name/folder.json that we care about. It is the fallback for naming
// the box-art catalogue directory when that file cannot be read.
var muosDefaultDisplayNames = map[string]string{
	"gb":      "Nintendo Game Boy",
	"gbc":     "Nintendo Game Boy Color",
	"gba":     "Nintendo Game Boy Advance",
	"nes":     "Nintendo Entertainment System",
	"fc":      "Nintendo Famicom",
	"md":      "SEGA Mega Drive",
	"genesis": "SEGA Genesis",
}

// muosDeviceLabels names the boards we have been able to verify. Anything else
// falls back to muOS's own board name, which is already meaningful — inventing
// a label for hardware we cannot check would be worse than showing "rg40xx-h".
var muosDeviceLabels = map[string]string{
	"tui-spoon":     "TrimUI Smart Pro",
	"tui-brick":     "TrimUI Brick",
	"tui-brick-pro": "TrimUI Brick Pro",
}

func newMuOS(prefix string) *Env {
	board := muosVar(prefix, muosDeviceConf, "board/name")
	label := muosDeviceLabels[board]
	if label == "" {
		label = board
	}

	// storage/rom/mount is SD1 and always present; storage/sdcard/mount is the
	// optional second card.
	romMount := muosVar(prefix, muosDeviceConf, "storage/rom/mount")
	if romMount == "" {
		romMount = "/mnt/mmc"
	}
	sdMount := muosVar(prefix, muosDeviceConf, "storage/sdcard/mount")

	roots := []string{filepath.Join(romMount, muosROMSubdir)}
	if sdMount != "" && isDir(filepath.Join(prefix, sdMount, muosROMSubdir)) {
		roots = append(roots, filepath.Join(sdMount, muosROMSubdir))
	}

	store := filepath.Join(prefix, muosStoreDir)
	data := dataDirFor()

	return &Env{
		kind:        KindMuOS,
		device:      board,
		deviceLabel: label,
		prefix:      prefix,

		root:    filepath.Join(prefix, romMount),
		romDirs: resolveMuOSROMDirs(prefix, roots),

		musicRoot:       filepath.Join(store, "music") + "/",
		browseRoot:      filepath.Join(prefix, romMount),
		musicBrowseRoot: filepath.Join(store, "music"),

		catalogueDir: filepath.Join(store, "info", "catalogue"),
		displayNames: loadMuOSDisplayNames(store),
		versionFile:  filepath.Join(prefix, muosGlobalConf, "system", "version"),

		dataDir: data,
		logPath: filepath.Join(data, "itchio.log"),

		// Everything here is a NextUI/MinUI concept with no muOS equivalent.
		// Save and state sync in particular must stay off: muOS assigns an
		// emulator core per folder, chosen by the user after the ROM is already
		// in place, so at download time there is nothing to derive a save path
		// from. Writing a guess would scatter files the user never finds.
		caps: Caps{},
	}
}

// resolveMuOSROMDirs decides where each system's ROMs belong.
//
// An existing folder always wins, so ROMs land beside the user's own library
// instead of in a second folder that only this app writes to. When nothing
// matches, the canonical short name is returned without being created — the
// download path already creates the directory it writes into, so creating
// folders here would litter the card on every launch.
func resolveMuOSROMDirs(prefix string, roots []string) map[string]string {
	type listing struct {
		root  string
		byLow map[string]string // lowercased name -> actual name on disk
	}

	listings := make([]listing, 0, len(roots))
	for _, r := range roots {
		byLow := map[string]string{}
		if entries, err := os.ReadDir(filepath.Join(prefix, r)); err == nil {
			for _, e := range entries {
				name := e.Name()
				// muOS hides folders prefixed with "." or "_"; a hidden folder
				// is not somewhere the user wants new downloads.
				if !e.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					continue
				}
				byLow[strings.ToLower(name)] = name
			}
		}
		listings = append(listings, listing{root: r, byLow: byLow})
	}

	out := make(map[string]string, len(muosROMAliases)+1)
	for key, aliases := range muosROMAliases {
		var found string
		for _, l := range listings {
			for _, alias := range aliases {
				if actual, ok := l.byLow[strings.ToLower(alias)]; ok {
					found = filepath.Join(prefix, l.root, actual) + "/"
					break
				}
			}
			if found != "" {
				break
			}
		}
		if found == "" && len(roots) > 0 {
			found = filepath.Join(prefix, roots[0], aliases[0]) + "/"
		}
		out[key] = found
	}

	// muOS runs Pico-8 through one core against one folder, so the two entries
	// NextUI keeps apart (its fakeo8 and pico8 folders) resolve to the same place.
	out[SysPico8Alt] = out[SysPico8]

	return out
}

// loadMuOSDisplayNames reads muOS's folder-name mapping, falling back to the
// defaults we ship. Keys are lowercased for case-insensitive lookup.
func loadMuOSDisplayNames(store string) map[string]string {
	names := make(map[string]string, len(muosDefaultDisplayNames))
	for k, v := range muosDefaultDisplayNames {
		names[k] = v
	}

	data, err := os.ReadFile(filepath.Join(store, "info", "name", "folder.json"))
	if err != nil {
		return names
	}
	var fromDevice map[string]string
	if err := json.Unmarshal(data, &fromDevice); err != nil {
		return names
	}
	for k, v := range fromDevice {
		if v != "" {
			names[strings.ToLower(k)] = v
		}
	}
	return names
}

// muosVar reads one of muOS's single-value config files, the same way its own
// GET_VAR helper does. Returns "" when absent.
func muosVar(prefix, base, key string) string {
	data, err := os.ReadFile(filepath.Join(prefix, base, key))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
