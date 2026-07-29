package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_AllFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color1=0xFF0000
color2=0x00FF00
color3=0x0000FF
color4=0x112233
color5=0xAABBCC
color6=0xDDEEFF
color7=0x010203
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Error("expected ok=true for valid theme file")
	}
	if th.MainText != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("MainText: got %v", th.MainText)
	}
	if th.Accent != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.HeaderBG != [3]uint8{0x00, 0x00, 0xFF} {
		t.Errorf("HeaderBG: got %v", th.HeaderBG)
	}
	if th.ListText != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("ListText: got %v", th.ListText)
	}
	if th.AccentText != [3]uint8{0xAA, 0xBB, 0xCC} {
		t.Errorf("AccentText: got %v", th.AccentText)
	}
	if th.HintText != [3]uint8{0xDD, 0xEE, 0xFF} {
		t.Errorf("HintText: got %v", th.HintText)
	}
	if th.Background != [3]uint8{0x01, 0x02, 0x03} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_SubsetOfFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color2=0x9B2257\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Error("expected ok=true for subset of fields")
	}
	if th.Accent != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background default: got %v", th.Background)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	th, ok := Load("/nonexistent/path/minuisettings.txt")
	if ok {
		t.Error("expected ok=false for missing file")
	}
	def := Defaults()
	if th != def {
		t.Errorf("expected defaults for missing file, got %+v", th)
	}
}

func TestLoad_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color2=notahex
color7=0x141414
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Error("expected ok=true for valid theme file")
	}
	if th.Accent != Defaults().Accent {

		t.Errorf("Accent should be default after bad hex, got %v", th.Accent)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_IgnoreUnrecognizedKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color2=0x00FF00
radius=20
showclock=1
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Error("expected ok=true for partial theme file")
	}
	if th.Accent != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("Accent: got %v", th.Accent)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if ok {
		t.Error("expected ok=false for empty file (no fields found)")
	}
	def := Defaults()
	if th != def {
		t.Errorf("expected defaults for empty file, got %+v", th)
	}
}

// TestLoad_PackedRGBA feeds Load a file written exactly the way NextUI writes
// one today (config.c:1607-1614). Before the digit-count parser this failed on
// all seven colours, ok came back false, and the Settings toggle disappeared.
func TestLoad_PackedRGBA(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `font=1
palette=Catppuccin Mocha
color1=0xCBA6F7FF
color2=0x181825FF
color3=0x1E1E2EFF
color4=0xCDD6F4FF
color5=0x1E1E2EFF
color6=0xA6ADC8FF
color7=0x1E1E2EFF
radius=20
showclock=0
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for a NextUI-written settings file")
	}
	if th.MainText != [3]uint8{0xCB, 0xA6, 0xF7} {
		t.Errorf("MainText: got %v, want [203 166 247]", th.MainText)
	}
	if th.Accent != [3]uint8{0x18, 0x18, 0x25} {
		t.Errorf("Accent: got %v, want [24 24 37]", th.Accent)
	}
	if th.Background != [3]uint8{0x1E, 0x1E, 0x2E} {
		t.Errorf("Background: got %v, want [30 30 46]", th.Background)
	}
	if th.HintText != [3]uint8{0xA6, 0xAD, 0xC8} {
		t.Errorf("HintText: got %v, want [166 173 200]", th.HintText)
	}
}

// TestLoad_LegacyRGBStillWorks guards the older-firmware path: a 6-digit file
// must keep loading exactly as it did before.
func TestLoad_LegacyRGBStillWorks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color2=0x9B2257\ncolor7=0x000000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for a legacy RGB settings file")
	}
	if th.Accent != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("Accent: got %v, want [155 34 87]", th.Accent)
	}
	if th.Background != [3]uint8{0x00, 0x00, 0x00} {
		t.Errorf("Background: got %v, want [0 0 0]", th.Background)
	}
}

// TestLoad_MixedDigitCounts covers a file mid-migration, where some keys have
// been rewritten with alpha and others have not.
func TestLoad_MixedDigitCounts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color1=0xFF0000\ncolor2=0x00FF00FF\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if th.MainText != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("MainText: got %v, want [255 0 0]", th.MainText)
	}
	if th.Accent != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("Accent: got %v, want [0 255 0]", th.Accent)
	}
}

func TestLoad_NoPrefix(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color7=141414\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for an unprefixed hex value")
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v, want [20 20 20]", th.Background)
	}
}

// TestLoad_AlphaIsDropped documents that the renderer is opaque: alpha is
// parsed so we can log it, then discarded.
func TestLoad_AlphaIsDropped(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color2=0x9B225780\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if th.Accent != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("Accent: got %v, want [155 34 87]", th.Accent)
	}
}

// TestLoad_MalformedKeepsDefault extends the malformed-line coverage to the two
// shapes NextUI itself would render black.
func TestLoad_MalformedKeepsDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color3=
color4=#112233
color7=0x141414
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true — color7 is valid")
	}
	def := Defaults()
	if th.HeaderBG != def.HeaderBG {
		t.Errorf("HeaderBG should stay default after a blank value, got %v", th.HeaderBG)
	}
	if th.ListText != def.ListText {
		t.Errorf("ListText should stay default after a '#' value, got %v", th.ListText)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}
