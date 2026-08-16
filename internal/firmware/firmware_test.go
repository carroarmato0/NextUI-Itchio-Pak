package firmware

import (
	"os"
	"path/filepath"
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
