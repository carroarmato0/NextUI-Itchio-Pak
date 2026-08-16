// Package firmware resolves everything that depends on which custom firmware
// Itch-io is running under.
//
// The app was written against NextUI, so paths like
// "/mnt/SDCARD/Roms/Game Boy (GB)/" and conventions like ".media/" cover art
// were scattered through the code as constants. Supporting a second firmware
// means those become answers to a question — "where do Game Boy ROMs go on
// *this* device?" — rather than facts.
//
// One Env is resolved at startup and published with SetActive. Callers reach it
// through Active(), so the ~15 UI call sites that ask for a ROM destination did
// not have to change shape. Every filesystem read goes through the prefix field
// so the whole package is testable against fixture trees, with no device.
package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Kind identifies a supported firmware.
type Kind string

const (
	// KindNextUI is NextUI (and MinUI-derived firmware) on /mnt/SDCARD.
	KindNextUI Kind = "nextui"
	// KindMuOS is MustardOS.
	KindMuOS Kind = "muos"
	// KindHost is a developer machine or CI — no device paths exist.
	KindHost Kind = "host"
)

// System keys used to look up a ROM destination. They are internal identifiers,
// not display names: each firmware maps them onto its own folder layout.
const (
	SysGB       = "gb"
	SysGBC      = "gbc"
	SysGBA      = "gba"
	SysGBAAlt   = "gba-alt" // NextUI's separate MGBA-emulator folder
	SysNES      = "nes"
	SysGenesis  = "genesis"
	SysPico8    = "pico8"     // default Pico-8 core
	SysPico8Alt = "pico8-alt" // "pico8" core variant
)

// Caps records which firmware-specific behaviours are available. Features are
// disabled rather than emulated when a firmware has no equivalent: guessing
// would write files to the wrong place, which is worse than not writing them.
type Caps struct {
	// NextUIPalette: the firmware exposes a NextUI/MinUI colour palette that
	// the app should follow, and a Settings row to show it.
	NextUIPalette bool
	// MinUISaveFormats: minuisettings.txt declares saveFormat/stateFormat, so
	// save-file migration can know which naming scheme the emulator uses.
	MinUISaveFormats bool
	// SaveStateSync: existing saves and save states can be located for a
	// downloaded ROM. Requires knowing which emulator core owns the ROM folder.
	SaveStateSync bool
	// GBAEmulatorChoice: the firmware ships two GBA folders for two emulators,
	// so the user picks one at download time.
	GBAEmulatorChoice bool
	// Pico8CoreChoice: the firmware ships two Pico-8 folders for two runtimes,
	// so the user picks one and existing carts can be migrated between them.
	Pico8CoreChoice bool
}

// Env is the resolved view of the firmware the app is running under.
// Construct it with Detect; treat it as immutable afterwards.
type Env struct {
	kind        Kind
	device      string
	deviceLabel string

	// prefix is prepended to every absolute device path. Empty in production;
	// set to a temp dir by tests so detection and path building can be
	// exercised without a device.
	prefix string

	root    string            // SD card root, no trailing slash
	romDirs map[string]string // system key -> absolute dir, trailing slash

	musicRoot       string // trailing slash
	browseRoot      string
	musicBrowseRoot string

	sharedUserdata    string // NextUI: <root>/.userdata/shared
	settingsFile      string // minuisettings.txt, "" when the firmware has none
	builtinPaletteDir string
	userPaletteDir    string
	versionFile       string // firmware version marker, "" when unknown

	// catalogueDir is muOS's box-art root. Empty on firmware that keeps art in
	// a .media/ directory beside the ROM instead.
	catalogueDir string
	// displayNames maps a ROM folder name to the system name the firmware shows
	// in its content list.
	displayNames map[string]string
	// catalogueByDir maps a resolved ROM directory to the catalogue directory
	// its box art belongs in. Separate from displayNames because muOS files
	// artwork under different names than it displays.
	catalogueByDir map[string]string

	dataDir string
	logPath string

	caps Caps
}

var (
	activeMu sync.RWMutex
	active   *Env
)

// Active returns the process-wide Env. It falls back to a host Env so that
// tests and tools which never call SetActive still get sane, path-free answers
// instead of a nil dereference.
func Active() *Env {
	activeMu.RLock()
	e := active
	activeMu.RUnlock()
	if e != nil {
		return e
	}
	activeMu.Lock()
	defer activeMu.Unlock()
	if active == nil {
		active = newHost("")
	}
	return active
}

// SetActive publishes env as the process-wide environment. Called once from
// main; tests use it to pin a specific firmware.
func SetActive(env *Env) {
	activeMu.Lock()
	active = env
	activeMu.Unlock()
}

// Detect resolves the firmware from the real filesystem.
func Detect() *Env { return DetectIn("") }

// DetectIn resolves the firmware, treating prefix as the filesystem root.
// Tests pass a temp dir holding a fixture tree; production passes "".
//
// muOS is checked first, and on an unambiguous marker: only muOS has /opt/muos.
// It has to come first because a muOS card can carry a leftover /mnt/SDCARD
// from a previous firmware. NextUI is then recognised by its .system directory,
// or by the PLATFORM variable it exports to paks — the latter matters on the
// Miyoo Flip, where the .system layout is not guaranteed.
func DetectIn(prefix string) *Env {
	if isDir(filepath.Join(prefix, muosMarkerDir)) {
		return newMuOS(prefix)
	}
	if isDir(filepath.Join(prefix, "/mnt/SDCARD/.system")) || os.Getenv("PLATFORM") != "" {
		return newNextUI(prefix)
	}
	return newHost(prefix)
}

// ForTest builds an Env for a specific firmware without probing the machine
// running the tests. Tests in other packages use it to pin the environment
// their assertions are written against:
//
//	func TestMain(m *testing.M) {
//	    firmware.SetActive(firmware.ForTest(firmware.KindNextUI, ""))
//	    os.Exit(m.Run())
//	}
//
// prefix behaves as in DetectIn: "" for bare device paths, or a temp dir when
// the test needs real files underneath them.
func ForTest(kind Kind, prefix string) *Env {
	switch kind {
	case KindNextUI:
		return newNextUI(prefix)
	case KindMuOS:
		return newMuOS(prefix)
	default:
		return newHost(prefix)
	}
}

// Kind reports which firmware this is.
func (e *Env) Kind() Kind { return e.kind }

// Device is the firmware's own code for this hardware: a NextUI platform code
// ("tg5040") or a muOS board name ("tui-spoon"). Empty when unknown.
func (e *Env) Device() string { return e.device }

// DeviceLabel is a human-readable device name for logs and the About screen.
func (e *Env) DeviceLabel() string { return e.deviceLabel }

// Caps reports which firmware-specific behaviours are available.
func (e *Env) Caps() Caps { return e.caps }

// Root is the SD card root, without a trailing slash. Empty off-device.
func (e *Env) Root() string { return e.root }

// ROMDirForSystem returns the absolute destination directory for a system key,
// with a trailing slash. Returns "" when this firmware has no such directory.
func (e *Env) ROMDirForSystem(key string) string { return e.romDirs[key] }

// ROMDirs returns a copy of every resolved ROM destination, keyed by system.
// Logged at startup: on firmware where these are discovered rather than fixed,
// "which folder did it pick?" is the first question any misplaced-ROM report
// needs answered.
func (e *Env) ROMDirs() map[string]string {
	out := make(map[string]string, len(e.romDirs))
	for k, v := range e.romDirs {
		out[k] = v
	}
	return out
}

// SystemForExt maps a ROM extension to a system key. pico8Core selects between
// the two Pico-8 variants. Returns "" for extensions we do not place.
//
// ".zip" resolves to GBC as a placeholder: the archive has not been inspected
// yet at the point this is called, and the picker corrects it afterwards.
func SystemForExt(ext, pico8Core string) string {
	switch strings.ToLower(ext) {
	case ".gbc", ".zip":
		return SysGBC
	case ".gb":
		return SysGB
	case ".gba":
		return SysGBA
	case ".nes":
		return SysNES
	case ".md", ".gen", ".smd":
		return SysGenesis
	case ".p8", ".p8.png":
		if pico8Core == "pico8" {
			return SysPico8Alt
		}
		return SysPico8
	default:
		return ""
	}
}

// ROMDir returns the destination directory for a ROM with this extension.
func (e *Env) ROMDir(ext, pico8Core string) string {
	key := SystemForExt(ext, pico8Core)
	if key == "" {
		return ""
	}
	return e.romDirs[key]
}

// Pico8Dir returns the Pico-8 directory for the given core.
func (e *Env) Pico8Dir(core string) string {
	if core == "pico8" {
		return e.romDirs[SysPico8Alt]
	}
	return e.romDirs[SysPico8]
}

// MusicRoot is the directory extracted soundtracks go under, trailing slash.
func (e *Env) MusicRoot() string { return e.musicRoot }

// BrowseRoot is the highest directory the ROM location picker may navigate to.
func (e *Env) BrowseRoot() string { return e.browseRoot }

// MusicBrowseRoot is the equivalent for the music location picker.
func (e *Env) MusicBrowseRoot() string { return e.musicBrowseRoot }

// SettingsFile is the firmware's own settings file (NextUI's minuisettings.txt),
// or "" when the firmware has none.
func (e *Env) SettingsFile() string { return e.settingsFile }

// PaletteDirs returns the builtin and user palette directories, or "" each when
// the firmware has no palette system.
func (e *Env) PaletteDirs() (builtin, user string) {
	return e.builtinPaletteDir, e.userPaletteDir
}

// DataDir is where the app keeps config.json, inventory.json and its caches.
func (e *Env) DataDir() string { return e.dataDir }

// LogPath is the full path of the runtime log file.
func (e *Env) LogPath() string { return e.logPath }

// StatesDir returns the directory holding save states for an emulator core, or
// "" when this firmware cannot locate them.
func (e *Env) StatesDir(coreTag, coreName string) string {
	if !e.caps.SaveStateSync || e.sharedUserdata == "" || coreTag == "" || coreName == "" {
		return ""
	}
	return filepath.Join(e.sharedUserdata, coreTag+"-"+coreName)
}

// CoverArtPath returns where cover art for a ROM belongs.
func (e *Env) CoverArtPath(romPath string) string {
	base := filepath.Base(romPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(e.CoverArtDirFor(filepath.Dir(romPath)), base+".png")
}

// CoverArtDirFor returns the directory cover art for ROMs in romDir belongs in.
//
// NextUI and MinUI read box art from a .media/ directory beside the ROM. muOS
// keeps a single catalogue tree instead, filed under the system's display name
// rather than the folder name, so this is not a path join everywhere.
func (e *Env) CoverArtDirFor(romDir string) string {
	if e.catalogueDir == "" {
		return filepath.Join(romDir, ".media")
	}
	return filepath.Join(e.catalogueDir, e.catalogueNameFor(romDir), "box")
}

// catalogueNameFor picks the catalogue directory for a ROM directory. Known
// systems use the name resolved at startup; a directory the user chose
// themselves falls back to the firmware's display name for it, and then to the
// folder name, which is what muOS does for folders it has no mapping for.
func (e *Env) catalogueNameFor(romDir string) string {
	trimmed := strings.TrimRight(romDir, "/")
	if name, ok := e.catalogueByDir[trimmed]; ok {
		return name
	}
	return e.DisplayNameForFolder(filepath.Base(trimmed))
}

// DisplayNameForFolder maps a ROM folder name to the system name the firmware
// displays. Falls back to the folder name itself, which is what muOS does for
// folders it has no mapping for.
func (e *Env) DisplayNameForFolder(folder string) string {
	if name, ok := e.displayNames[strings.ToLower(folder)]; ok {
		return name
	}
	return folder
}

// FirmwareVersion reads the firmware's own version string, or "unknown".
func (e *Env) FirmwareVersion() string {
	if e.versionFile == "" {
		return "unknown"
	}
	data, err := os.ReadFile(e.versionFile)
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "unknown"
}

// SuspendCmd is the executable to run to put the device to sleep, or "" when
// the firmware suspends the app itself and the app should just keep running.
func (e *Env) SuspendCmd() string {
	if e.kind != KindNextUI {
		return ""
	}
	sys := os.Getenv("SYSTEM_PATH")
	if sys == "" {
		return ""
	}
	return filepath.Join(sys, "bin", "suspend")
}

// dataDirFor picks where mutable app state lives. ITCHIO_DATA_DIR lets a
// launcher place it explicitly; otherwise it follows HOME, which is what the
// NextUI launch script sets to the pak's shared userdata directory.
func dataDirFor() string {
	if d := os.Getenv("ITCHIO_DATA_DIR"); d != "" {
		return d
	}
	return os.Getenv("HOME")
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// FaceMapping says how SDL's face-button names line up with the labels printed
// on the shell.
//
// SDL names face buttons by position using the Xbox arrangement, and these
// handhelds print Nintendo labels, so the two often disagree — but not always,
// and not consistently across firmware. It has to be resolved rather than
// derived: NextUI and muOS present the same TrimUI pad under different device
// names with the same controller-database line, yet report opposite face
// buttons for it, because the raw button order underneath differs.
type FaceMapping string

const (
	// FaceSwapped: the shell's A sits where Xbox puts B, so the button labelled
	// A arrives as CONTROLLER_BUTTON_B.
	FaceSwapped FaceMapping = "swapped"
	// FaceDirect: SDL's names match the labels on the shell.
	FaceDirect FaceMapping = "direct"
	// FaceABDirect: A and B match the labels on the shell, but X and Y are
	// swapped. This is the H700 family: NextUI's own platform.h reads the
	// shell's A as joystick button 0 (tg5040 reads it as 1), while X and Y keep
	// the transposed order every one of these handhelds uses.
	FaceABDirect FaceMapping = "ab-direct"
)

// FaceMapping reports how to read this firmware's face buttons.
//
// Measured on hardware, not inferred. NextUI presents the TrimUI pad as an
// "X360 Controller" and swaps them. muOS presents the same pad as a "TRIMUI
// Smart Pro Controller" with an identical mapping line, and does not — the raw
// button indices behind the identical line are ordered differently. muOS's
// modern controller database then swaps its face buttons back relative to its
// own default, so it lands where NextUI is.
func (e *Env) FaceMapping() FaceMapping {
	if e.kind != KindMuOS {
		// Derived from upstream source, not measured — nobody here owns the
		// hardware. logControllerButton records every press, so the first
		// tester log settles it.
		if e.device == "h700" {
			return FaceABDirect
		}
		return FaceSwapped
	}

	// muOS applies the user's choice by pointing SDL at one of two controller
	// databases, so the filename it exports is that setting already resolved.
	switch filepath.Base(os.Getenv("SDL_GAMECONTROLLERCONFIG_FILE")) {
	case "modern.txt":
		return FaceSwapped
	case "retro.txt":
		return FaceDirect
	}

	// Launched outside muOS's own launcher, fall back to the stored preference.
	// Absent on releases predating it, where the default behaviour is retro.
	if muosVar(e.prefix, muosGlobalConf, "settings/remap/layout") == "1" {
		return FaceSwapped
	}
	return FaceDirect
}
