package theme

import "testing"

// shippedPalettes is every palette NextUI ships, transcribed from
// skeleton/SYSTEM/res/palettes/*.txt (upstream ebac427e). Values are inlined
// rather than read from disk so these tests run anywhere, including CI.
//
// Six of the eighteen are light themes — Latte, Deep Violet, Mossy Sage,
// Mustard Butter, Teal Powder, Terracotta Cream, Brick Blush — which is the
// reason every derived colour has to be background-aware.
var shippedPalettes = []struct {
	name   string
	colors [7]string // color1..color7
}{
	{"Brick Blush", [7]string{"0xB5442EFF", "0xF6DFD9FF", "0xFBEAE6FF", "0x33201DFF", "0xFBEAE6FF", "0x8A6259FF", "0xFBECE8FF"}},
	{"Catppuccin Frappe", [7]string{"0xA6D189FF", "0x292C3CFF", "0x232634FF", "0xC6D0F5FF", "0x232634FF", "0xA5ADCEFF", "0x303446FF"}},
	{"Catppuccin Latte", [7]string{"0x8839EFFF", "0xE6E9EFFF", "0xEFF1F5FF", "0x4C4F69FF", "0xEFF1F5FF", "0x6C6F85FF", "0xEFF1F5FF"}},
	{"Catppuccin Macchiato", [7]string{"0xF5A97FFF", "0x1E2030FF", "0x24273AFF", "0xCAD3F5FF", "0x24273AFF", "0xA5ADCBFF", "0x24273AFF"}},
	{"Catppuccin Mocha", [7]string{"0xCBA6F7FF", "0x181825FF", "0x1E1E2EFF", "0xCDD6F4FF", "0x1E1E2EFF", "0xA6ADC8FF", "0x1E1E2EFF"}},
	{"Charcoal Coral", [7]string{"0xFF6B5BFF", "0x161416FF", "0x2A0D06FF", "0xF2F0ECFF", "0x2A0D06FF", "0x948F8CFF", "0x1D1B1EFF"}},
	{"Deep Violet", [7]string{"0x6C4BC9FF", "0xE7DCF7FF", "0xF4EFFDFF", "0x241B3DFF", "0xF4EFFDFF", "0x6E6389FF", "0xF1EAFAFF"}},
	{"Default", [7]string{"0xffffffff", "0x9b2257ff", "0x1e2329ff", "0xffffffff", "0x000000ff", "0xffffffff", "0x000000ff"}},
	{"Forest Lime", [7]string{"0xB7DD5BFF", "0x0A1712FF", "0x1B2708FF", "0xE7EFE7FF", "0x1B2708FF", "0x7F998AFF", "0x0F1F18FF"}},
	{"Ink & Gold", [7]string{"0xF2A93BFF", "0x0C0E17FF", "0x241A05FF", "0xE7E6F2FF", "0x241A05FF", "0x8A87A3FF", "0x12141FFF"}},
	{"Maroon Rose", [7]string{"0xE9A6A0FF", "0x1C0B0EFF", "0x2E100CFF", "0xF3E6E4FF", "0x2E100CFF", "0x9C7B78FF", "0x271014FF"}},
	{"MinUI", [7]string{"0xffffffff", "0x262626ff", "0x999999ff", "0xffffffff", "0x000000ff", "0xffffffff", "0x000000ff"}},
	{"Mossy Sage", [7]string{"0x4B6B3FFF", "0xE4EBD9FF", "0xEBF3E4FF", "0x1F2A1BFF", "0xEBF3E4FF", "0x647459FF", "0xEEF2E6FF"}},
	{"Mustard Butter", [7]string{"0xB08117FF", "0xF5E9C4FF", "0xFDF3DAFF", "0x2E2610FF", "0xFDF3DAFF", "0x8A7A45FF", "0xFBF3DCFF"}},
	{"Plum Magenta", [7]string{"0xD6559EFF", "0x170F1DFF", "0x2E0A1FFF", "0xEEE6F2FF", "0x2E0A1FFF", "0x93849EFF", "0x1F1526FF"}},
	{"Slate Cyan", [7]string{"0x45CFC3FF", "0x111A28FF", "0x0A2320FF", "0xE6EDF3FF", "0x0A2320FF", "0x7E8FA3FF", "0x182233FF"}},
	{"Teal Powder", [7]string{"0x1E6E76FF", "0xDCEBEFFF", "0xE7F5F5FF", "0x17232EFF", "0xE7F5F5FF", "0x5B7480FF", "0xE9F2F5FF"}},
	{"Terracotta Cream", [7]string{"0xC1602EFF", "0xF1E8D9FF", "0xFCEEE4FF", "0x2B2118FF", "0xFCEEE4FF", "0x7A6E5CFF", "0xF7F1E7FF"}},
}

// themeFor builds a Theme from a shipped palette using the same key-to-field
// mapping Load uses.
func themeFor(t *testing.T, colors [7]string) Theme {
	t.Helper()
	th := Defaults()
	rgb := func(i int) [3]uint8 {
		c, err := ParseColor(colors[i])
		if err != nil {
			t.Fatalf("ParseColor(%q): %v", colors[i], err)
		}
		return c.RGB()
	}
	th.Accent = rgb(0)     // color1
	th.TitlePill = rgb(1)  // color2
	th.HeaderBG = rgb(2)   // color3
	th.ListText = rgb(3)   // color4
	th.MainText = rgb(3)   // color4
	th.AccentText = rgb(4) // color5
	th.HintText = rgb(5)   // color6
	th.Background = rgb(6) // color7
	return th
}

// minReadable is the luma gap below which text on a fill is a strain to read.
const minReadable = 60

// TestShippedPalettes_TitlePillIsReadable is the test that should have existed
// before this shipped. Pairing the color2 pill with color5 text looked fine in
// unit tests and was invisible on the device: on Catppuccin Macchiato it draws
// #24273A on #1E2030, a luma gap of 4.
//
// NextUI pairs the color2 pill with color6 (nextui.c:2849), which is what
// TitlePillText returns.
func TestShippedPalettes_TitlePillIsReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := Contrast(th.TitlePill, th.TitlePillText()); got < minReadable {
				t.Errorf("TitlePill %v vs text %v: contrast %d, want >= %d",
					th.TitlePill, th.TitlePillText(), got, minReadable)
			}
		})
	}
}

// TestShippedPalettes_AccentPillIsReadable checks the other pairing NextUI
// defines: color5 text on the color1 selection pill.
func TestShippedPalettes_AccentPillIsReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := Contrast(th.Accent, th.AccentText); got < minReadable {
				t.Errorf("Accent %v vs AccentText %v: contrast %d, want >= %d",
					th.Accent, th.AccentText, got, minReadable)
			}
		})
	}
}

// TestShippedPalettes_BodyTextIsReadable confirms MainText taking color4 rather
// than color1 is right: color1 is a pill fill and is frequently low-contrast
// against the background.
func TestShippedPalettes_BodyTextIsReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := Contrast(th.Background, th.MainText); got < minReadable {
				t.Errorf("Background %v vs MainText %v: contrast %d, want >= %d",
					th.Background, th.MainText, got, minReadable)
			}
		})
	}
}

// TestShippedPalettes_SurfaceIsVisible pins why Surface exists: color3 equals or
// nearly equals color7 in most shipped palettes, so a bar drawn with color3 raw
// would vanish into the background.
func TestShippedPalettes_SurfaceIsVisible(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := th.Surface(); got == th.Background {
				t.Errorf("Surface() == Background %v — the bars would be invisible", got)
			}
		})
	}
}

// TestShippedPalettes_SeparatorIsVisible guards the header/footer rule.
func TestShippedPalettes_SeparatorIsVisible(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := th.Separator(); got == th.Background {
				t.Errorf("Separator() == Background %v", got)
			}
		})
	}
}

// TestShippedPalettes_ModalPanelIsVisible is the light-theme regression: the
// renderer used to fill the modal with a raw `bg[0]+20` on a uint8. On
// Catppuccin Latte, background #EFF1F5, that wraps to #030509 — a near-black
// panel with near-black text on it.
func TestShippedPalettes_ModalPanelIsVisible(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			panel := th.ModalPanel()
			if panel == th.Background {
				t.Errorf("ModalPanel() == Background %v — the panel would not read", panel)
			}
			// The wrap always produced a huge swing; a correct shade never does.
			if got := Contrast(panel, th.Background); got > 40 {
				t.Errorf("ModalPanel() %v is %d luma from Background %v — looks like a wrap",
					panel, got, th.Background)
			}
			if got := Contrast(panel, th.MainText); got < minReadable {
				t.Errorf("ModalPanel() %v vs MainText %v: contrast %d, want >= %d",
					panel, th.MainText, got, minReadable)
			}
		})
	}
}

// TestShippedPalettes_ModalBorderIsVisible — a border that matches its own panel
// is not a border.
func TestShippedPalettes_ModalBorderIsVisible(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if got := th.ModalBorder(); got == th.ModalPanel() {
				t.Errorf("ModalBorder() == ModalPanel() %v", got)
			}
		})
	}
}

// TestShippedPalettes_ChipIsVisible covers the tag pills, which used
// `(Accent+35)/2` — arithmetic with no meaning once Accent became color1.
func TestShippedPalettes_ChipIsVisible(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			chip := th.Chip()
			if chip == th.Background {
				t.Errorf("Chip() == Background %v", chip)
			}
			// Chips sit on Surface, so they have to read against it too.
			if chip == th.Surface() {
				t.Errorf("Chip() == Surface() %v — the pill shape would not read", chip)
			}
			// The label goes through ContrastText, which is what the call site uses.
			if got := Contrast(chip, th.ContrastText(chip)); got < minReadable {
				t.Errorf("Chip() %v vs its text %v: contrast %d, want >= %d",
					chip, th.ContrastText(chip), got, minReadable)
			}
		})
	}
}

// TestShippedPalettes_SemanticColorsAreReadable checks every status hue against
// every shipped background. Fixed literals fail this badly: raw amber
// #F0B43C has luma 184 against Catppuccin Latte's 240, a gap of 56.
func TestShippedPalettes_SemanticColorsAreReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			for _, c := range []struct {
				name string
				got  [3]uint8
			}{
				{"Success", th.Success()},
				{"Error", th.Error()},
				{"Warning", th.Warning()},
				{"Info", th.Info()},
				{"Muted", th.Muted()},
			} {
				if got := Contrast(c.got, th.Background); got < minReadable {
					t.Errorf("%s %v on Background %v: contrast %d, want >= %d",
						c.name, c.got, th.Background, got, minReadable)
				}
			}
		})
	}
}

// TestShippedPalettes_SemanticColorsStayDistinct — the whole point of fixing the
// hues rather than deriving them is that success never becomes error.
func TestShippedPalettes_SemanticColorsStayDistinct(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			if th.Success() == th.Error() {
				t.Errorf("Success() == Error() == %v", th.Success())
			}
			if th.Warning() == th.Error() {
				t.Errorf("Warning() == Error() == %v", th.Warning())
			}
		})
	}
}

// TestShippedPalettes_StatusBackingsAreReadable covers the tinted row backings.
func TestShippedPalettes_StatusBackingsAreReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			for _, c := range []struct {
				name string
				got  [3]uint8
			}{
				{"SuccessBG", th.SuccessBG()},
				{"ErrorBG", th.ErrorBG()},
				{"WarningBG", th.WarningBG()},
				{"ProgressTrack", th.ProgressTrack()},
			} {
				if got := Contrast(c.got, th.ContrastText(c.got)); got < minReadable {
					t.Errorf("%s %v vs its text: contrast %d, want >= %d",
						c.name, c.got, got, minReadable)
				}
			}
			if th.ProgressTrack() == th.ProgressFill() {
				t.Errorf("ProgressTrack() == ProgressFill() — no progress visible")
			}
		})
	}
}

// TestSemantic_DarkThemeIsIdentity pins that the app's shipping look does not
// move: on the default dark theme every status hue is used verbatim.
func TestSemantic_DarkThemeIsIdentity(t *testing.T) {
	d := Defaults()
	tests := []struct {
		name string
		got  [3]uint8
		want [3]uint8
	}{
		{"Success", d.Success(), [3]uint8{80, 200, 80}},
		{"Error", d.Error(), [3]uint8{200, 60, 60}},
		{"Warning", d.Warning(), [3]uint8{240, 180, 60}},
		{"Info", d.Info(), [3]uint8{80, 200, 220}},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v (dark themes must be untouched)", tc.name, tc.got, tc.want)
		}
	}
}

// TestTone_Terminates sweeps every possible grey background, including the
// mid-greys where no amount of toning can reach minContrast, and asserts the
// loop always ends and always moves in the correct direction.
func TestTone_Terminates(t *testing.T) {
	for v := 0; v <= 255; v++ {
		bg := [3]uint8{uint8(v), uint8(v), uint8(v)}
		th := Defaults()
		th.Background = bg
		for _, base := range [][3]uint8{baseSuccess, baseError, baseWarning, baseInfo} {
			got := th.Tone(base)
			if got == base {
				continue // already readable; nothing to check
			}
			// Toning must push away from the background, never toward it.
			if th.IsLightTheme() && Luma(got) > Luma(base) {
				t.Fatalf("bg %v: light theme toned %v lighter to %v", bg, base, got)
			}
			if !th.IsLightTheme() && Luma(got) < Luma(base) {
				t.Fatalf("bg %v: dark theme toned %v darker to %v", bg, base, got)
			}
			// And it must never make contrast worse than leaving it alone.
			if Contrast(got, bg) < Contrast(base, bg) {
				t.Fatalf("bg %v: toning %v -> %v reduced contrast %d -> %d",
					bg, base, got, Contrast(base, bg), Contrast(got, bg))
			}
		}
	}
}

// TestShippedPalettes_ContrastTextAlwaysReadable is the safety net: whatever
// fill a screen invents, ContrastText must return something legible on it.
func TestShippedPalettes_ContrastTextAlwaysReadable(t *testing.T) {
	for _, p := range shippedPalettes {
		t.Run(p.name, func(t *testing.T) {
			th := themeFor(t, p.colors)
			for _, fill := range [][3]uint8{
				th.Background, th.Surface(), th.Accent, th.TitlePill,
				th.ModalPanel(), th.Chip(),
				{0x00, 0x00, 0x00}, {0xFF, 0xFF, 0xFF}, {0x80, 0x80, 0x80},
			} {
				if got := Contrast(fill, th.ContrastText(fill)); got < minReadable {
					t.Errorf("fill %v -> text %v: contrast %d, want >= %d",
						fill, th.ContrastText(fill), got, minReadable)
				}
			}
		})
	}
}
