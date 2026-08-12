package firmware

import (
	"os"
	"path/filepath"
	"testing"
)

// muosFixture builds the parts of a muOS filesystem this package reads. Values
// are the ones a TrimUI Smart Pro running muOS 2601.0 JACARANDA actually
// reports, so the fixture cannot drift into describing a device that does not
// exist.
func muosFixture(t *testing.T, romFolders ...string) string {
	t.Helper()
	prefix := t.TempDir()

	for path, content := range map[string]string{
		"opt/muos/device/config/board/name":           "tui-spoon",
		"opt/muos/device/config/board/home":           "/root",
		"opt/muos/device/config/storage/rom/mount":    "/mnt/mmc",
		"opt/muos/device/config/storage/sdcard/mount": "/mnt/sdcard",
		"opt/muos/device/config/mux/width":            "1280",
		"opt/muos/device/config/mux/height":           "720",
		"opt/muos/config/system/version":              "2601.0_JACARANDA",
	} {
		full := filepath.Join(prefix, path)
		mkdirAll(t, filepath.Dir(full))
		writeFile(t, full, content)
	}

	mkdirAll(t, filepath.Join(prefix, "mnt/mmc/ROMS"))
	for _, f := range romFolders {
		mkdirAll(t, filepath.Join(prefix, "mnt/mmc/ROMS", f))
	}
	return prefix
}

func TestMuOSDetectedByOptDirectory(t *testing.T) {
	t.Setenv("PLATFORM", "")
	e := DetectIn(muosFixture(t))

	if e.Kind() != KindMuOS {
		t.Fatalf("Kind() = %q, want %q", e.Kind(), KindMuOS)
	}
	if e.Device() != "tui-spoon" {
		t.Errorf("Device() = %q, want tui-spoon", e.Device())
	}
	if e.DeviceLabel() != "TrimUI Smart Pro" {
		t.Errorf("DeviceLabel() = %q", e.DeviceLabel())
	}
	if got, want := e.FirmwareVersion(), "2601.0_JACARANDA"; got != want {
		t.Errorf("FirmwareVersion() = %q, want %q", got, want)
	}
}

// muOS must win even with a NextUI layout present: a card reflashed from NextUI
// can still carry /mnt/SDCARD, and PLATFORM may be inherited from the shell.
func TestMuOSWinsOverLeftoverNextUILayout(t *testing.T) {
	prefix := muosFixture(t)
	mkdirAll(t, filepath.Join(prefix, "mnt/SDCARD/.system"))
	t.Setenv("PLATFORM", "tg5040")

	if got := DetectIn(prefix).Kind(); got != KindMuOS {
		t.Fatalf("Kind() = %q, want %q", got, KindMuOS)
	}
}

// The whole point of scanning: put ROMs in the folder the user already has,
// whatever they called it, rather than making a second one beside it.
func TestMuOSAdoptsExistingROMFolders(t *testing.T) {
	t.Setenv("PLATFORM", "")

	for _, tc := range []struct {
		name    string
		folders []string
		ext     string
		want    string
	}{
		{"short key", []string{"gb"}, ".gb", "mnt/mmc/ROMS/gb/"},
		{"display name from folder.json", []string{"Nintendo Game Boy"}, ".gb", "mnt/mmc/ROMS/Nintendo Game Boy/"},
		{"plain english name", []string{"Game Boy"}, ".gb", "mnt/mmc/ROMS/Game Boy/"},
		{"different case", []string{"GBC"}, ".gbc", "mnt/mmc/ROMS/GBC/"},
		{"famicom alias for nes", []string{"fc"}, ".nes", "mnt/mmc/ROMS/fc/"},
		{"genesis alias for mega drive", []string{"genesis"}, ".md", "mnt/mmc/ROMS/genesis/"},
		{"pico-8 with hyphen", []string{"pico-8"}, ".p8", "mnt/mmc/ROMS/pico-8/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := muosFixture(t, tc.folders...)
			// filepath.Join drops the trailing slash, which callers rely on.
			want := filepath.Join(prefix, tc.want) + "/"
			if got := DetectIn(prefix).ROMDir(tc.ext, ""); got != want {
				t.Errorf("ROMDir(%q) = %q, want %q", tc.ext, got, want)
			}
		})
	}
}

// With nothing to adopt, fall back to muOS's own short-key convention — and do
// not create it: the download path makes the directory it writes into, so
// creating folders at detection would litter the card on every launch.
func TestMuOSFallsBackToConventionWithoutCreatingIt(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t)
	e := DetectIn(prefix)

	for _, tc := range []struct{ ext, want string }{
		{".gb", "mnt/mmc/ROMS/gb/"},
		{".gbc", "mnt/mmc/ROMS/gbc/"},
		{".gba", "mnt/mmc/ROMS/gba/"},
		{".nes", "mnt/mmc/ROMS/nes/"},
		{".md", "mnt/mmc/ROMS/md/"},
		{".p8", "mnt/mmc/ROMS/pico8/"},
	} {
		want := filepath.Join(prefix, tc.want) + "/"
		if got := e.ROMDir(tc.ext, ""); got != want {
			t.Errorf("ROMDir(%q) = %q, want %q", tc.ext, got, want)
		}
	}

	if entries, err := os.ReadDir(filepath.Join(prefix, "mnt/mmc/ROMS")); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Errorf("detection created %d directories, want none", len(entries))
	}
}

// A second card is only considered when it actually holds a ROMS directory, and
// the primary card still wins when both have a match.
func TestMuOSSecondCard(t *testing.T) {
	t.Setenv("PLATFORM", "")

	t.Run("adopted when SD1 has no match", func(t *testing.T) {
		prefix := muosFixture(t)
		mkdirAll(t, filepath.Join(prefix, "mnt/sdcard/ROMS/gb"))

		want := filepath.Join(prefix, "mnt/sdcard/ROMS/gb") + "/"
		if got := DetectIn(prefix).ROMDir(".gb", ""); got != want {
			t.Errorf("ROMDir = %q, want %q", got, want)
		}
	})

	t.Run("primary card wins when both match", func(t *testing.T) {
		prefix := muosFixture(t, "gb")
		mkdirAll(t, filepath.Join(prefix, "mnt/sdcard/ROMS/gb"))

		want := filepath.Join(prefix, "mnt/mmc/ROMS/gb") + "/"
		if got := DetectIn(prefix).ROMDir(".gb", ""); got != want {
			t.Errorf("ROMDir = %q, want %q", got, want)
		}
	})
}

// muOS hides folders prefixed with "." or "_", so they are not a place the user
// wants new downloads landing.
func TestMuOSIgnoresHiddenFolders(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t, ".gb", "_gb")

	want := filepath.Join(prefix, "mnt/mmc/ROMS/gb") + "/"
	if got := DetectIn(prefix).ROMDir(".gb", ""); got != want {
		t.Errorf("ROMDir = %q, want %q (hidden folders must not be adopted)", got, want)
	}
}

// muOS runs Pico-8 through one core against one folder, unlike NextUI's split.
func TestMuOSHasOnePico8Folder(t *testing.T) {
	t.Setenv("PLATFORM", "")
	e := DetectIn(muosFixture(t, "pico8"))

	if a, b := e.Pico8Dir(""), e.Pico8Dir("pico8"); a != b {
		t.Errorf("Pico8Dir differs by core: %q vs %q", a, b)
	}
}

// There is no second GBA folder to choose, so the picker must not offer one.
func TestMuOSHasNoAlternateGBAFolder(t *testing.T) {
	t.Setenv("PLATFORM", "")
	e := DetectIn(muosFixture(t))

	if got := e.ROMDirForSystem(SysGBAAlt); got != "" {
		t.Errorf("alternate GBA dir = %q, want empty", got)
	}
	if e.Caps().GBAEmulatorChoice {
		t.Error("muOS must not advertise a GBA emulator choice")
	}
}

// Box art goes in muOS's catalogue tree, filed under the system's display name
// rather than the folder name — not in a .media/ directory beside the ROM.
func TestMuOSCoverArtUsesCatalogue(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t, "gb")
	e := DetectIn(prefix)

	rom := filepath.Join(prefix, "mnt/mmc/ROMS/gb/Tobu Tobu Girl.gb")
	want := filepath.Join(prefix, "run/muos/storage/info/catalogue/Nintendo Game Boy/box/Tobu Tobu Girl.png")
	if got := e.CoverArtPath(rom); got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}

// An unmapped folder name is used verbatim, which is what muOS itself does.
func TestMuOSCatalogueFallsBackToFolderName(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t, "weirdname")
	e := DetectIn(prefix)

	rom := filepath.Join(prefix, "mnt/mmc/ROMS/weirdname/Game.gb")
	want := filepath.Join(prefix, "run/muos/storage/info/catalogue/weirdname/box/Game.png")
	if got := e.CoverArtPath(rom); got != want {
		t.Errorf("CoverArtPath = %q, want %q", got, want)
	}
}

// The device's own folder.json overrides our built-in defaults, so a user who
// renamed a system in muOS gets art filed where muOS will look for it.
func TestMuOSReadsFolderJSONFromDevice(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t, "gb")
	nameDir := filepath.Join(prefix, "run/muos/storage/info/name")
	mkdirAll(t, nameDir)
	writeFile(t, filepath.Join(nameDir, "folder.json"), `{"gb":"Game Boy Classic","xyz":"Some System"}`)

	e := DetectIn(prefix)
	if got, want := e.DisplayNameForFolder("gb"), "Game Boy Classic"; got != want {
		t.Errorf("DisplayNameForFolder(gb) = %q, want %q", got, want)
	}
	if got, want := e.DisplayNameForFolder("XYZ"), "Some System"; got != want {
		t.Errorf("case-insensitive lookup = %q, want %q", got, want)
	}
}

// Every NextUI-only behaviour must be off. These are not cosmetic: acting on a
// wrong save path writes files somewhere the user will never find them.
func TestMuOSDisablesNextUIOnlyBehaviour(t *testing.T) {
	t.Setenv("PLATFORM", "")
	e := DetectIn(muosFixture(t))

	if c := e.Caps(); c.NextUIPalette || c.MinUISaveFormats || c.SaveStateSync || c.GBAEmulatorChoice {
		t.Errorf("muOS should advertise no NextUI capabilities, got %+v", c)
	}
	if got := e.SettingsFile(); got != "" {
		t.Errorf("SettingsFile() = %q, want empty", got)
	}
	if b, u := e.PaletteDirs(); b != "" || u != "" {
		t.Errorf("PaletteDirs() = (%q, %q), want empty", b, u)
	}
	if got := e.StatesDir("GB", "gambatte"); got != "" {
		t.Errorf("StatesDir() = %q, want empty", got)
	}
	if got := e.SuspendCmd(); got != "" {
		t.Errorf("SuspendCmd() = %q, want empty (muOS suspends the app itself)", got)
	}
}

// App state must not default to HOME on muOS: SETUP_APP points HOME at /root on
// the system partition, which a firmware update can replace.
func TestMuOSDataDirHonoursLauncherOverride(t *testing.T) {
	t.Setenv("PLATFORM", "")
	t.Setenv("HOME", "/root")
	t.Setenv("ITCHIO_DATA_DIR", "/mnt/mmc/MUOS/application/Itch-io/data")

	e := DetectIn(muosFixture(t))
	if got, want := e.DataDir(), "/mnt/mmc/MUOS/application/Itch-io/data"; got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
	if got, want := e.LogPath(), "/mnt/mmc/MUOS/application/Itch-io/data/itchio.log"; got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

func TestMuOSBrowseRoots(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t)
	e := DetectIn(prefix)

	if got, want := e.BrowseRoot(), filepath.Join(prefix, "mnt/mmc"); got != want {
		t.Errorf("BrowseRoot() = %q, want %q", got, want)
	}
	if got, want := e.MusicRoot(), filepath.Join(prefix, "run/muos/storage/music")+"/"; got != want {
		t.Errorf("MusicRoot() = %q, want %q", got, want)
	}
}

// An unknown board is shown by its muOS name rather than a label we invented.
func TestMuOSUnknownBoardUsesItsOwnName(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t)
	writeFile(t, filepath.Join(prefix, "opt/muos/device/config/board/name"), "rg40xx-h")

	if got := DetectIn(prefix).DeviceLabel(); got != "rg40xx-h" {
		t.Errorf("DeviceLabel() = %q, want the board name", got)
	}
}

// Which SDL button each labelled face button produces is a user preference on
// muOS, applied by pointing SDL at one of two controller databases. The
// exported filename is that setting already resolved.
//
// retro is muOS's default and maps directly — verified on a TrimUI Smart Pro,
// where an earlier guess that it matched NextUI turned out to be backwards.
func TestMuOSFaceMappingFromSDLConfigFile(t *testing.T) {
	t.Setenv("PLATFORM", "")
	prefix := muosFixture(t)

	for _, tc := range []struct {
		file string
		want FaceMapping
	}{
		{"/opt/muos/share/info/gamecontrollerdb/retro.txt", FaceDirect},
		{"/opt/muos/share/info/gamecontrollerdb/modern.txt", FaceSwapped},
	} {
		t.Setenv("SDL_GAMECONTROLLERCONFIG_FILE", tc.file)
		if got := DetectIn(prefix).FaceMapping(); got != tc.want {
			t.Errorf("%s -> %q, want %q", tc.file, got, tc.want)
		}
	}
}

// Launched outside muOS's launcher the variable is absent, so fall back to the
// stored preference — and to retro when even that is missing, which is the case
// on releases predating the setting (JACARANDA 2601.0 has no remap/layout).
func TestMuOSFaceMappingFallsBack(t *testing.T) {
	t.Setenv("PLATFORM", "")
	t.Setenv("SDL_GAMECONTROLLERCONFIG_FILE", "")

	prefix := muosFixture(t)
	if got := DetectIn(prefix).FaceMapping(); got != FaceDirect {
		t.Errorf("with no setting at all = %q, want %q", got, FaceDirect)
	}

	remap := filepath.Join(prefix, "opt/muos/config/settings/remap")
	mkdirAll(t, remap)
	writeFile(t, filepath.Join(remap, "layout"), "1")
	if got := DetectIn(prefix).FaceMapping(); got != FaceSwapped {
		t.Errorf("with remap/layout=1 = %q, want %q", got, FaceSwapped)
	}

	writeFile(t, filepath.Join(remap, "layout"), "0")
	if got := DetectIn(prefix).FaceMapping(); got != FaceDirect {
		t.Errorf("with remap/layout=0 = %q, want %q", got, FaceDirect)
	}
}

// NextUI presents one arrangement and has no setting for it. The muOS variable
// must not leak across and change NextUI's bindings, which are known good.
func TestNextUIFaceButtonsAlwaysSwapped(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	t.Setenv("SDL_GAMECONTROLLERCONFIG_FILE", "/somewhere/retro.txt")

	if got := newNextUI("").FaceMapping(); got != FaceSwapped {
		t.Errorf("NextUI face mapping = %q, want %q", got, FaceSwapped)
	}
}
