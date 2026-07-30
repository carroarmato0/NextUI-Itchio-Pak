package theme

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSettings writes a minuisettings.txt into a temp dir and returns its path.
func writeSettings(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "minuisettings.txt")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSemanticMapping is the whole Stage-2 contract in one test: which
// minuisettings.txt key feeds which Theme field. Each colour gets a distinct
// value so a swapped pair cannot pass.
//
// The mapping follows NextUI's own use of these colours — see the Theme doc
// comment for the source references. color4 deliberately feeds two fields:
// NextUI has no separate body-text colour, and color1 is a pill fill, not text.
func TestSemanticMapping(t *testing.T) {
	p := writeSettings(t, `color1=0x010101FF
color2=0x020202FF
color3=0x030303FF
color4=0x040404FF
color5=0x050505FF
color6=0x060606FF
color7=0x070707FF
`)
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true")
	}

	tests := []struct {
		field string
		got   [3]uint8
		want  uint8 // all channels equal, so one byte says which colorN it came from
	}{
		{"Accent (color1)", th.Accent, 0x01},
		{"TitlePill (color2)", th.TitlePill, 0x02},
		{"HeaderBG (color3)", th.HeaderBG, 0x03},
		{"ListText (color4)", th.ListText, 0x04},
		{"MainText (color4)", th.MainText, 0x04},
		{"AccentText (color5)", th.AccentText, 0x05},
		{"HintText (color6)", th.HintText, 0x06},
		{"Background (color7)", th.Background, 0x07},
	}
	for _, tc := range tests {
		want := [3]uint8{tc.want, tc.want, tc.want}
		if tc.got != want {
			t.Errorf("%s = %v, want %v", tc.field, tc.got, want)
		}
	}
}

// TestDefaults_Unchanged is the zero-visual-change guard. With the NextUI theme
// toggle off, or on a device with no minuisettings.txt, the app renders from
// these values — they must not drift when the key mapping changes.
func TestDefaults_Unchanged(t *testing.T) {
	want := Theme{
		Background: [3]uint8{0x14, 0x14, 0x14},
		HeaderBG:   [3]uint8{0x1E, 0x1E, 0x1E},
		Accent:     [3]uint8{0x3C, 0x3C, 0x5C},
		AccentText: [3]uint8{0xDC, 0xDC, 0xDC},
		ListText:   [3]uint8{0xDC, 0xDC, 0xDC},
		HintText:   [3]uint8{0x8C, 0x8C, 0x8C},
		MainText:   [3]uint8{0xDC, 0xDC, 0xDC},
		// Matches what the old `Accent/2+18` arithmetic produced from the
		// default Accent, so the sort pill does not shift.
		TitlePill: [3]uint8{0x30, 0x30, 0x40},
	}
	if got := Defaults(); got != want {
		t.Errorf("Defaults() drifted:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoad_AllFields(t *testing.T) {
	p := writeSettings(t, `color1=0xFF0000
color2=0x00FF00
color3=0x0000FF
color4=0x112233
color5=0xAABBCC
color6=0xDDEEFF
color7=0x010203
`)
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for valid theme file")
	}
	if th.Accent != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.TitlePill != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("TitlePill: got %v", th.TitlePill)
	}
	if th.HeaderBG != [3]uint8{0x00, 0x00, 0xFF} {
		t.Errorf("HeaderBG: got %v", th.HeaderBG)
	}
	if th.ListText != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("ListText: got %v", th.ListText)
	}
	if th.MainText != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("MainText: got %v", th.MainText)
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
	p := writeSettings(t, "color2=0x9B2257\n")
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for subset of fields")
	}
	if th.TitlePill != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("TitlePill: got %v", th.TitlePill)
	}
	if th.Background != Defaults().Background {
		t.Errorf("Background default: got %v", th.Background)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	th, ok := Load("/nonexistent/path/minuisettings.txt")
	if ok {
		t.Error("expected ok=false for missing file")
	}
	if def := Defaults(); th != def {
		t.Errorf("expected defaults for missing file, got %+v", th)
	}
}

func TestLoad_MalformedLine(t *testing.T) {
	p := writeSettings(t, "color2=notahex\ncolor7=0x141414\n")
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true — color7 is valid")
	}
	if th.TitlePill != Defaults().TitlePill {
		t.Errorf("TitlePill should be default after bad hex, got %v", th.TitlePill)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_IgnoreUnrecognizedKeys(t *testing.T) {
	p := writeSettings(t, "color2=0x00FF00\nradius=20\nshowclock=1\n")
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for partial theme file")
	}
	if th.TitlePill != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("TitlePill: got %v", th.TitlePill)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	p := writeSettings(t, "")
	th, ok := Load(p)
	if ok {
		t.Error("expected ok=false for empty file (no fields found)")
	}
	if def := Defaults(); th != def {
		t.Errorf("expected defaults for empty file, got %+v", th)
	}
}

// TestLoad_PackedRGBA feeds Load a file written exactly the way NextUI writes
// one today (config.c:1607-1614). Before the digit-count parser this failed on
// all seven colours, ok came back false, and the Settings toggle disappeared.
func TestLoad_PackedRGBA(t *testing.T) {
	// Catppuccin Mocha, byte-for-byte from skeleton/SYSTEM/res/palettes/.
	p := writeSettings(t, `font=1
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
`)
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true for a NextUI-written settings file")
	}
	if th.Accent != [3]uint8{0xCB, 0xA6, 0xF7} {
		t.Errorf("Accent: got %v, want [203 166 247]", th.Accent)
	}
	if th.TitlePill != [3]uint8{0x18, 0x18, 0x25} {
		t.Errorf("TitlePill: got %v, want [24 24 37]", th.TitlePill)
	}
	if th.MainText != [3]uint8{0xCD, 0xD6, 0xF4} {
		t.Errorf("MainText: got %v, want [205 214 244]", th.MainText)
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
	p := writeSettings(t, "color1=0x9B2257\ncolor7=0x000000\n")
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
	p := writeSettings(t, "color1=0xFF0000\ncolor2=0x00FF00FF\n")
	th, ok := Load(p)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if th.Accent != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("Accent: got %v, want [255 0 0]", th.Accent)
	}
	if th.TitlePill != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("TitlePill: got %v, want [0 255 0]", th.TitlePill)
	}
}

func TestLoad_NoPrefix(t *testing.T) {
	p := writeSettings(t, "color7=141414\n")
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
	p := writeSettings(t, "color1=0x9B225780\n")
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
	p := writeSettings(t, "color3=\ncolor4=#112233\ncolor7=0x141414\n")
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
	if th.MainText != def.MainText {
		t.Errorf("MainText should stay default after a '#' value, got %v", th.MainText)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}
