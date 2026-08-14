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
// MUOS/info/name/folder.json that we care about. These name a system in the
// content list; they are NOT catalogue directory names — see
// muosCatalogueNames, which is a different mapping with different strings.
var muosDefaultDisplayNames = map[string]string{
	"gb":      "Nintendo Game Boy",
	"gbc":     "Nintendo Game Boy Color",
	"gba":     "Nintendo Game Boy Advance",
	"nes":     "Nintendo Entertainment System",
	"fc":      "Nintendo Famicom",
	"md":      "SEGA Mega Drive",
	"genesis": "SEGA Genesis",
}

// muosCatalogueNames maps a system to the catalogue directory muOS files its
// box art under, most likely first.
//
// These are deliberately not the folder.json display names. muOS calls the NES
// "Nintendo Entertainment System" in a content list but files its artwork under
// "Nintendo NES - Famicom", and the same split applies to the Mega Drive and
// Pico-8. Writing art under the display name puts it in a directory muOS never
// reads, with nothing to show for it and no error. Verified against the
// catalogue directories a muOS 2601.0 device creates for itself.
var muosCatalogueNames = map[string][]string{
	SysGB:      {"Nintendo Game Boy"},
	SysGBC:     {"Nintendo Game Boy Color"},
	SysGBA:     {"Nintendo Game Boy Advance"},
	SysNES:     {"Nintendo NES - Famicom", "Nintendo Entertainment System", "Nintendo Famicom"},
	SysGenesis: {"Sega Mega Drive - Genesis", "SEGA Mega Drive", "Sega Genesis"},
	SysPico8:   {"PICO-8", "Pico-8"},
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
	romDirs := resolveMuOSROMDirs(prefix, roots)

	return &Env{
		kind:        KindMuOS,
		device:      board,
		deviceLabel: label,
		prefix:      prefix,

		root:    filepath.Join(prefix, romMount),
		romDirs: romDirs,

		musicRoot:       filepath.Join(store, "music") + "/",
		browseRoot:      filepath.Join(prefix, romMount),
		musicBrowseRoot: filepath.Join(store, "music"),

		catalogueDir: filepath.Join(store, "info", "catalogue"),
		displayNames: loadMuOSDisplayNames(store),
		catalogueByDir: resolveMuOSCatalogues(
			filepath.Join(store, "info", "catalogue"), romDirs),
		versionFile: filepath.Join(prefix, muosGlobalConf, "system", "version"),

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

// resolveMuOSCatalogues decides which catalogue directory each system's box art
// belongs in, keyed by the ROM directory it will be written beside.
//
// A candidate that already exists always wins: muOS pre-creates the catalogue
// for every system it knows, so the directory on disk is a better answer than
// anything compiled in, and it keeps working if a future release renames one.
func resolveMuOSCatalogues(catalogueDir string, romDirs map[string]string) map[string]string {
	out := make(map[string]string, len(romDirs))
	for key, romDir := range romDirs {
		candidates := muosCatalogueNames[key]
		if len(candidates) == 0 || romDir == "" {
			continue
		}
		chosen := candidates[0]
		for _, name := range candidates {
			if isDir(filepath.Join(catalogueDir, name)) {
				chosen = name
				break
			}
		}
		out[strings.TrimRight(romDir, "/")] = chosen
	}
	return out
}
