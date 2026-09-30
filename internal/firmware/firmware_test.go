package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The NextUI environment must reproduce the paths that were hardcoded before
// this package existed, character for character. These literals are copied from
// the pre-refactor source (internal/roms/roms.go, internal/theme/palette.go,
// internal/inventory/migrate.go, cmd/itchio-pak/main.go) on purpose: if a
// refactor changes where a ROM lands on a user's SD card, that is a data-loss
// bug, not a cosmetic one, and it should fail here rather than on a device.
func TestNextUIReproducesLegacyPaths(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	t.Setenv("HOME", "/mnt/SDCARD/.userdata/shared/Itch-io")
	t.Setenv("ITCHIO_DATA_DIR", "")

	e := newNextUI("")

	t.Run("rom destinations by extension", func(t *testing.T) {
		for _, tc := range []struct{ ext, core, want string }{
			{".gb", "", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
			{".gbc", "", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
			{".GBC", "", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
			{".gba", "", "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
			{".nes", "", "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
			{".md", "", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
			{".gen", "", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
			{".smd", "", "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
			{".p8", "", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
			{".p8", "fakeo8", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
			{".p8", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
			{".p8.png", "pico8", "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
			// .zip is a placeholder until the archive is inspected.
			{".zip", "", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
			{".exe", "", ""},
			{"", "", ""},
		} {
			if got := e.ROMDir(tc.ext, tc.core); got != tc.want {
				t.Errorf("ROMDir(%q, %q) = %q, want %q", tc.ext, tc.core, got, tc.want)
			}
		}
	})

	t.Run("named directories", func(t *testing.T) {
		for _, tc := range []struct{ name, got, want string }{
			{"GBA", e.ROMDirForSystem(SysGBA), "/mnt/SDCARD/Roms/Game Boy Advance (GBA)/"},
			{"GBA alt", e.ROMDirForSystem(SysGBAAlt), "/mnt/SDCARD/Roms/Game Boy Advance (MGBA)/"},
			{"NES", e.ROMDirForSystem(SysNES), "/mnt/SDCARD/Roms/Nintendo Entertainment System (FC)/"},
			{"Genesis", e.ROMDirForSystem(SysGenesis), "/mnt/SDCARD/Roms/Sega Genesis (MD)/"},
			{"Pico-8 default", e.Pico8Dir(""), "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
			{"Pico-8 pico8", e.Pico8Dir("pico8"), "/mnt/SDCARD/Roms/Pico-8 (PICO)/"},
			{"music root", e.MusicRoot(), "/mnt/SDCARD/Music/"},
			{"browse root", e.BrowseRoot(), "/mnt/SDCARD"},
			{"music browse root", e.MusicBrowseRoot(), "/mnt/SDCARD/Music"},
			{"settings", e.SettingsFile(), "/mnt/SDCARD/.userdata/shared/minuisettings.txt"},
			{"log", e.LogPath(), "/mnt/SDCARD/.userdata/tg5040/logs/itchio.log"},
			{"states", e.StatesDir("GB", "gambatte"), "/mnt/SDCARD/.userdata/shared/GB-gambatte"},
		} {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		}

		builtin, user := e.PaletteDirs()
		if builtin != "/mnt/SDCARD/.system/res/palettes" {
			t.Errorf("builtin palettes = %q", builtin)
		}
		if user != "/mnt/SDCARD/Palettes" {
			t.Errorf("user palettes = %q", user)
		}
	})

	t.Run("device identity", func(t *testing.T) {
		if e.Kind() != KindNextUI {
			t.Errorf("Kind() = %q", e.Kind())
		}
		if e.Device() != "tg5040" {
			t.Errorf("Device() = %q", e.Device())
		}
		if e.DeviceLabel() != "TrimUI Brick / Smart Pro" {
			t.Errorf("DeviceLabel() = %q", e.DeviceLabel())
		}
	})

	t.Run("all capabilities on", func(t *testing.T) {
		c := e.Caps()
		if !c.NextUIPalette || !c.MinUISaveFormats || !c.SaveStateSync || !c.GBAEmulatorChoice {
			t.Errorf("NextUI should have every capability, got %+v", c)
		}
	})
}

func TestNextUIDeviceLabels(t *testing.T) {
	for platform, want := range map[string]string{
		"tg5040":  "TrimUI Brick / Smart Pro",
		"tg5050":  "TrimUI Smart Pro S",
		"my355":   "Miyoo Flip",
		"nonsuch": "unknown device",
	} {
		t.Setenv("PLATFORM", platform)
		if got := newNextUI("").DeviceLabel(); got != want {
			t.Errorf("PLATFORM=%q label = %q, want %q", platform, got, want)
		}
	}
}

// Without PLATFORM there is no platform log directory to write into, so the log
// has to fall back beside the app's own state rather than to a path that does
// not exist.
func TestNextUILogFallsBackWhenPlatformUnset(t *testing.T) {
	t.Setenv("PLATFORM", "")
	t.Setenv("ITCHIO_DATA_DIR", "")
	t.Setenv("HOME", "/tmp/somewhere")

	if got, want := newNextUI("").LogPath(), "/tmp/somewhere/itchio.log"; got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

func TestCoverArtPathFollowsTheROM(t *testing.T) {
	e := newNextUI("")
	for _, tc := range []struct{ rom, want string }{
		{"/mnt/SDCARD/Roms/Game Boy (GB)/Game.gb", "/mnt/SDCARD/Roms/Game Boy (GB)/.media/Game.png"},
		{"/elsewhere/Game.gbc", "/elsewhere/.media/Game.png"},
	} {
		if got := e.CoverArtPath(tc.rom); got != tc.want {
			t.Errorf("CoverArtPath(%q) = %q, want %q", tc.rom, got, tc.want)
		}
	}
}

// ITCHIO_DATA_DIR exists so a launcher can place app state somewhere other than
// HOME. muOS needs it: SETUP_APP points HOME at /root on the system partition,
// which a firmware update can replace.
func TestDataDirPrefersExplicitOverride(t *testing.T) {
	t.Setenv("HOME", "/home/ignored")
	t.Setenv("ITCHIO_DATA_DIR", "/mnt/mmc/MUOS/application/Itch-io/data")

	if got, want := dataDirFor(), "/mnt/mmc/MUOS/application/Itch-io/data"; got != want {
		t.Errorf("dataDirFor() = %q, want %q", got, want)
	}

	t.Setenv("ITCHIO_DATA_DIR", "")
	if got, want := dataDirFor(), "/home/ignored"; got != want {
		t.Errorf("dataDirFor() without override = %q, want %q", got, want)
	}
}

func TestDetectRecognisesFirmwareFromFixtureTree(t *testing.T) {
	t.Setenv("PLATFORM", "")

	t.Run("nextui by .system directory", func(t *testing.T) {
		prefix := t.TempDir()
		mkdirAll(t, filepath.Join(prefix, "mnt/SDCARD/.system"))

		e := DetectIn(prefix)
		if e.Kind() != KindNextUI {
			t.Fatalf("Kind() = %q, want %q", e.Kind(), KindNextUI)
		}
		want := filepath.Join(prefix, "/mnt/SDCARD/Roms/Game Boy (GB)") + "/"
		if got := e.ROMDir(".gb", ""); got != want {
			t.Errorf("ROMDir = %q, want %q", got, want)
		}
	})

	t.Run("host when nothing matches", func(t *testing.T) {
		e := DetectIn(t.TempDir())
		if e.Kind() != KindHost {
			t.Fatalf("Kind() = %q, want %q", e.Kind(), KindHost)
		}
		if got := e.ROMDir(".gb", ""); got != "" {
			t.Errorf("host ROMDir = %q, want empty", got)
		}
		if e.Caps().SaveStateSync {
			t.Error("host should not claim save/state sync")
		}
	})
}

func TestFirmwareVersionReadsFirstNonEmptyLine(t *testing.T) {
	prefix := t.TempDir()
	sys := filepath.Join(prefix, "mnt/SDCARD/.system")
	mkdirAll(t, sys)
	writeFile(t, filepath.Join(sys, "version.txt"), "\n\n  NextUI-20260726  \nsecond line\n")

	t.Setenv("PLATFORM", "")
	if got, want := DetectIn(prefix).FirmwareVersion(), "NextUI-20260726"; got != want {
		t.Errorf("FirmwareVersion() = %q, want %q", got, want)
	}
}

func TestFirmwareVersionUnknownWhenAbsent(t *testing.T) {
	prefix := t.TempDir()
	mkdirAll(t, filepath.Join(prefix, "mnt/SDCARD/.system"))
	t.Setenv("PLATFORM", "")

	if got := DetectIn(prefix).FirmwareVersion(); got != "unknown" {
		t.Errorf("FirmwareVersion() = %q, want %q", got, "unknown")
	}
}

// Active must never return nil, even when main never ran.
func TestActiveDefaultsToHost(t *testing.T) {
	SetActive(nil)
	if e := Active(); e == nil || e.Kind() != KindHost {
		t.Fatalf("Active() = %+v, want a host Env", e)
	}
	SetActive(nil)
}

// H700 reports its face buttons differently from every other NextUI device.
// NextUI's own platform.h reads the shell's A as joystick button 0 there and as
// button 1 on tg5040, and SDL's controller index equals that JOY_ index (a
// four-for-four match on TrimUI hardware). So A and B land where their labels
// say and X and Y do not.
func TestFaceMappingPerPlatform(t *testing.T) {
	for _, tc := range []struct {
		platform string
		want     FaceMapping
	}{
		{"tg5040", FaceSwapped},
		{"tg5050", FaceSwapped},
		{"my355", FaceSwapped},
		{"h700", FaceABDirect},
	} {
		t.Setenv("PLATFORM", tc.platform)
		if got := newNextUI("").FaceMapping(); got != tc.want {
			t.Errorf("PLATFORM=%q FaceMapping() = %q, want %q", tc.platform, got, tc.want)
		}
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveDirAndPakStoreDB(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	nx := ForTest(KindNextUI, "")
	if got := nx.PakStoreDB(); got != "/mnt/SDCARD/.userdata/tg5040/nextui-pak-store/pak-store.db" {
		t.Errorf("NextUI PakStoreDB = %q", got)
	}
	if nx.ArchiveDir() != "" {
		t.Errorf("NextUI ArchiveDir = %q, want empty", nx.ArchiveDir())
	}

	t.Setenv("PLATFORM", "")
	if got := ForTest(KindNextUI, "").PakStoreDB(); got != "" {
		t.Errorf("NextUI without PLATFORM: PakStoreDB = %q, want empty", got)
	}

	prefix := t.TempDir()
	mu := ForTest(KindMuOS, prefix)
	if got, want := mu.ArchiveDir(), filepath.Join(prefix, "/mnt/mmc", "ARCHIVE"); got != want {
		t.Errorf("muOS ArchiveDir = %q, want %q", got, want)
	}
	if mu.PakStoreDB() != "" {
		t.Errorf("muOS PakStoreDB = %q, want empty", mu.PakStoreDB())
	}

	host := ForTest(KindHost, "")
	if host.ArchiveDir() != "" || host.PakStoreDB() != "" {
		t.Error("host must expose neither path")
	}
}

// h700 is a single PLATFORM across eleven SKUs, so the platform code cannot say
// which handheld this is — $DEVICE can. The fallbacks matter as much as the
// table: an unrecognised SKU still has to produce something a bug report can be
// filed against, because a new Anbernic model will appear before we hear of it.
func TestNextUIH700LabelsComeFromDevice(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	for device, want := range map[string]string{
		"rg40xxv":    "Anbernic RG40XX V",
		"rgcubexx":   "Anbernic RG Cube XX",
		"rg35xxplus": "Anbernic RG35XX Plus",
		"rg28xx":     "Anbernic RG28XX",
		"RG40XXV":    "Anbernic RG40XX V",
		"rg99xx":     "Anbernic H700 (rg99xx)",
		"":           "Anbernic H700",
	} {
		t.Setenv("DEVICE", device)
		if got := newNextUI("").DeviceLabel(); got != want {
			t.Errorf("DEVICE=%q label = %q, want %q", device, got, want)
		}
	}
}

// DEVICE is exported on other platforms too, and must not leak into their
// labels.
func TestNextUIDeviceIgnoredOffH700(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	t.Setenv("DEVICE", "brick")
	if got, want := newNextUI("").DeviceLabel(), "TrimUI Brick / Smart Pro"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

// The mapping is a workaround for one device's missing database entry. Every
// other platform's SDL2 already classifies its pad as a controller, and
// overriding that would swap face buttons on hardware that works today.
func TestControllerMappingOnlyH700(t *testing.T) {
	for _, platform := range []string{"tg5040", "tg5050", "my355"} {
		t.Setenv("PLATFORM", platform)
		if _, ok := newNextUI("").ControllerMapping(Pad{GUID: "guid", Name: "pad", Buttons: 15}); ok {
			t.Errorf("PLATFORM=%q returned a mapping, want none", platform)
		}
	}
}

// A mapping line is comma-separated, so a name containing a comma would shift
// every binding that follows it by one field.
func TestControllerMappingSanitisesName(t *testing.T) {
	t.Setenv("PLATFORM", "h700")

	got, ok := newNextUI("").ControllerMapping(Pad{GUID: "guid", Name: "Ann,Bernic,keys", Buttons: 15})
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	if !strings.HasPrefix(got, "guid,Ann Bernic keys,platform:Linux,") {
		t.Errorf("comma in name not sanitised: %q", got)
	}
}

// From NextUI h700-rc11, the firmware's SDL numbers the H700 pad the way TrimUI
// does and recognises it as a game controller with a positional mapping, under
// a new GUID. The shell's A then arrives where Xbox puts B, as on tg5040. Older
// firmware — and rc11 with SDL_JOYSTICK_H700_FIXED_LAYOUT=0 — keep the old GUID
// and the old arrangement.
func TestFaceMappingH700FollowsThePadLayout(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	fixed := Pad{GUID: H700FixedLayoutGUID, Name: "ANBERNIC-keys", Buttons: 15, Hats: 1}
	legacy := Pad{GUID: "19000000010000000100000000010000", Name: "ANBERNIC-keys", Buttons: 18, Hats: 1}

	for _, tc := range []struct {
		name string
		pads []Pad
		want FaceMapping
	}{
		{"rc11 fixed layout", []Pad{fixed}, FaceSwapped},
		{"rc10 and earlier", []Pad{legacy}, FaceABDirect},
		{"no pad reported", nil, FaceABDirect},
	} {
		if got := newNextUI("").FaceMapping(tc.pads...); got != tc.want {
			t.Errorf("%s: FaceMapping() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The fixed-layout GUID means nothing off H700: another device reporting it
// must keep its own arrangement.
func TestFaceMappingFixedLayoutGUIDIgnoredOffH700(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	if got := newNextUI("").FaceMapping(Pad{GUID: H700FixedLayoutGUID}); got != FaceSwapped {
		t.Errorf("FaceMapping() = %q, want %q", got, FaceSwapped)
	}
}

// rc11's SDL maps the fixed-layout pad itself, so the app normally never asks.
// If it does — an SDL that numbers the pad the new way but lacks the database
// entry — the mapping must be positional, to agree with FaceSwapped, and must
// not come from the old key-bitmap derivation, whose indices no longer apply.
func TestControllerMappingH700FixedLayoutIsPositional(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	procInputDevices = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { procInputDevices = "/proc/bus/input/devices" })

	got, ok := newNextUI("").ControllerMapping(Pad{GUID: H700FixedLayoutGUID, Name: "ANBERNIC-keys", Buttons: 15, Hats: 1})
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want the positional mapping")
	}
	for _, want := range []string{"a:b0", "b:b1", "x:b2", "y:b3", "back:b6", "start:b7", "guide:b8",
		"leftshoulder:b4", "rightshoulder:b5", "lefttrigger:a2", "righttrigger:a5", "dpup:h0.1"} {
		if !strings.Contains(","+got+",", ","+want+",") {
			t.Errorf("mapping lacks %q: %s", want, got)
		}
	}
}
