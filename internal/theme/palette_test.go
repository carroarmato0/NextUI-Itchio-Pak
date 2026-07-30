package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// mochaFile is byte-for-byte skeleton/SYSTEM/res/palettes/Catppuccin Mocha.txt.
const mochaFile = `version=1
name=Catppuccin Mocha
color1=0xCBA6F7FF
color2=0x181825FF
color3=0x1E1E2EFF
color4=0xCDD6F4FF
color5=0x1E1E2EFF
color6=0xA6ADC8FF
color7=0x1E1E2EFF
`

func TestLoadPalette_Full(t *testing.T) {
	p := writeFile(t, t.TempDir(), "Catppuccin Mocha.txt", mochaFile)
	pal, ok := LoadPalette(p, true)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pal.Name != "Catppuccin Mocha" {
		t.Errorf("Name = %q", pal.Name)
	}
	if pal.Version != 1 {
		t.Errorf("Version = %d, want 1", pal.Version)
	}
	if !pal.Builtin {
		t.Error("Builtin = false, want true")
	}
	if got := pal.Colors[0].RGB(); got != [3]uint8{0xCB, 0xA6, 0xF7} {
		t.Errorf("color1 = %v", got)
	}
	if got := pal.Colors[6].RGB(); got != [3]uint8{0x1E, 0x1E, 0x2E} {
		t.Errorf("color7 = %v", got)
	}
}

// A palette authored for a newer format is skipped entirely, not partially
// applied — palette.c:80 checks the version after reading the whole file and
// returns false.
func TestLoadPalette_VersionTooNew(t *testing.T) {
	p := writeFile(t, t.TempDir(), "future.txt", "version=2\nname=Future\ncolor1=0xFF0000FF\n")
	if _, ok := LoadPalette(p, false); ok {
		t.Error("expected ok=false for a version newer than we support")
	}
}

func TestLoadPalette_VersionAbsentDefaultsTo1(t *testing.T) {
	p := writeFile(t, t.TempDir(), "noversion.txt", "name=No Version\ncolor1=0xFF0000FF\n")
	pal, ok := LoadPalette(p, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pal.Version != 1 {
		t.Errorf("Version = %d, want 1", pal.Version)
	}
}

// NextUI parses version with sscanf("%i"), which accepts hex and octal.
// A "0x2" here means 2, and must be skipped like any other future version.
func TestLoadPalette_VersionAcceptsHex(t *testing.T) {
	p := writeFile(t, t.TempDir(), "hexver.txt", "version=0x2\nname=Hex\n")
	if _, ok := LoadPalette(p, false); ok {
		t.Error("expected ok=false: 0x2 is version 2")
	}
}

func TestLoadPalette_NameFromFilename(t *testing.T) {
	p := writeFile(t, t.TempDir(), "Deep_Violet.txt", "version=1\ncolor1=0xFF0000FF\n")
	pal, ok := LoadPalette(p, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pal.Name != "Deep Violet" {
		t.Errorf("Name = %q, want %q", pal.Name, "Deep Violet")
	}
}

// name= present but empty is treated as absent (palette.c:60 sets haveName only
// when the value is non-empty).
func TestLoadPalette_EmptyNameFallsBack(t *testing.T) {
	p := writeFile(t, t.TempDir(), "My_Theme.txt", "version=1\nname=\ncolor1=0xFF0000FF\n")
	pal, ok := LoadPalette(p, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pal.Name != "My Theme" {
		t.Errorf("Name = %q, want %q", pal.Name, "My Theme")
	}
}

// Missing colours fall back to NextUI's defaults, not to zero (palette.c:42).
func TestLoadPalette_MissingColorsUseNextUIDefaults(t *testing.T) {
	p := writeFile(t, t.TempDir(), "partial.txt", "version=1\nname=Partial\ncolor1=0x112233FF\n")
	pal, ok := LoadPalette(p, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := pal.Colors[0]; got != RGBA(0x112233FF) {
		t.Errorf("color1 = %#08x", uint32(got))
	}
	for i := 1; i < PaletteColorCount; i++ {
		if pal.Colors[i] != NextUIDefaultColors[i] {
			t.Errorf("color%d = %#08x, want NextUI default %#08x",
				i+1, uint32(pal.Colors[i]), uint32(NextUIDefaultColors[i]))
		}
	}
}

func TestLoadPalette_MissingFile(t *testing.T) {
	if _, ok := LoadPalette("/nonexistent/nope.txt", false); ok {
		t.Error("expected ok=false for a missing file")
	}
}

func TestEnumeratePalettes_Order(t *testing.T) {
	bdir, udir := t.TempDir(), t.TempDir()
	writeFile(t, bdir, "Default.txt", "version=1\nname=Default\n")
	writeFile(t, bdir, "MinUI.txt", "version=1\nname=MinUI\n")
	writeFile(t, udir, "Mine.txt", "version=1\nname=Mine\n")
	// Skipped: dot-prefixed, wrong extension, too short a name.
	writeFile(t, udir, ".hidden.txt", "version=1\nname=Hidden\n")
	writeFile(t, udir, "notes.md", "version=1\nname=Notes\n")
	// Accepted: extension match is case-insensitive.
	writeFile(t, udir, "Loud.TXT", "version=1\nname=Loud\n")

	got := EnumeratePalettes(bdir, udir)
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if len(got) != 4 {
		t.Fatalf("got %d palettes %v, want 4", len(got), names)
	}
	// Built-ins first, matching PALETTE_enumerate's scan order.
	if !got[0].Builtin || !got[1].Builtin {
		t.Errorf("expected built-ins first, got %v", names)
	}
	if got[2].Builtin || got[3].Builtin {
		t.Errorf("expected user palettes last, got %v", names)
	}
	for _, n := range names {
		if n == "Hidden" || n == "Notes" {
			t.Errorf("%q should have been skipped; got %v", n, names)
		}
	}
}

// The device this was verified on has no /mnt/SDCARD/Palettes at all, and
// my355 has no NextUI platform, so both directories can be absent.
func TestEnumeratePalettes_MissingDirs(t *testing.T) {
	got := EnumeratePalettes("/nonexistent/builtin", "/nonexistent/user")
	if len(got) != 0 {
		t.Errorf("got %d palettes, want 0", len(got))
	}
}

func TestEnumeratePalettes_SkipsFutureVersions(t *testing.T) {
	udir := t.TempDir()
	writeFile(t, udir, "ok.txt", "version=1\nname=Fine\n")
	writeFile(t, udir, "future.txt", "version=99\nname=Future\n")
	got := EnumeratePalettes("/nonexistent", udir)
	if len(got) != 1 || got[0].Name != "Fine" {
		t.Errorf("got %+v, want only the supported palette", got)
	}
}

func TestLoadSettings_PaletteName(t *testing.T) {
	p := writeSettings(t, "palette=Catppuccin Macchiato\ncolor7=0x24273AFF\n")
	th, name, ok := LoadSettings(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Catppuccin Macchiato" {
		t.Errorf("palette name = %q", name)
	}
	if th.Background != [3]uint8{0x24, 0x27, 0x3A} {
		t.Errorf("Background = %v", th.Background)
	}
}

// An empty palette= means the user edited individual colours; NextUI calls that
// "Custom" and so do we.
func TestLoadSettings_EmptyPaletteMeansCustom(t *testing.T) {
	p := writeSettings(t, "palette=\ncolor7=0x000000FF\n")
	_, name, ok := LoadSettings(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "" {
		t.Errorf("palette name = %q, want empty (Custom)", name)
	}
}

// Firmware older than PR #787 writes no palette= line at all.
func TestLoadSettings_NoPaletteLine(t *testing.T) {
	p := writeSettings(t, "color7=0x000000FF\n")
	_, name, ok := LoadSettings(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "" {
		t.Errorf("palette name = %q, want empty", name)
	}
}

// PaletteLabel is what the Settings row shows next to the toggle.
func TestPaletteLabel(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Catppuccin Macchiato", "Catppuccin Macchiato"},
		{"", "Custom"},
	}
	for _, tc := range tests {
		if got := PaletteLabel(tc.name); got != tc.want {
			t.Errorf("PaletteLabel(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
