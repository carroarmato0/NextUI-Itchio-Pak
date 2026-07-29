package theme

import "testing"

// TestParseColor pins our parser against NextUI's CFG_parseHexColor
// (workspace/all/common/config.c:111). NextUI decides RGB vs RGBA by hex digit
// count, not by numeric value — the bug this replaces used a 0xFFFFFF range
// check, which rejects every colour NextUI writes today.
func TestParseColor(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantRGB [3]uint8
		wantA   uint8
		wantErr bool
	}{
		// 8 digits — the format NextUI writes since PR #762 ("color1=0x%08X").
		{"packed rgba white", "0xFFFFFFFF", [3]uint8{0xFF, 0xFF, 0xFF}, 0xFF, false},
		{"packed rgba black", "0x000000ff", [3]uint8{0x00, 0x00, 0x00}, 0xFF, false},
		{"packed rgba nextui default accent", "0x9b2257ff", [3]uint8{0x9B, 0x22, 0x57}, 0xFF, false},
		{"packed rgba half alpha", "0x1E1E2E80", [3]uint8{0x1E, 0x1E, 0x2E}, 0x80, false},

		// 6 digits — legacy RGB, still written by older firmware. Alpha is
		// implied opaque, matching NextUI's (value << 8) | 0xFF.
		{"legacy rgb prefixed", "0xffffff", [3]uint8{0xFF, 0xFF, 0xFF}, 0xFF, false},
		{"legacy rgb bare", "ffffff", [3]uint8{0xFF, 0xFF, 0xFF}, 0xFF, false},
		{"legacy rgb uppercase prefix", "0X141414", [3]uint8{0x14, 0x14, 0x14}, 0xFF, false},
		{"leading whitespace", "  0x141414", [3]uint8{0x14, 0x14, 0x14}, 0xFF, false},
		{"trailing carriage return", "141414\r", [3]uint8{0x14, 0x14, 0x14}, 0xFF, false},

		// Short values. NextUI does NOT CSS-expand these: strtoul yields
		// 0x00000FFF, which the <=6 branch shifts to 0x000FFFFF -> RGB 00 0F FF.
		// Expanding "fff" to #FFFFFF would silently disagree with the device.
		{"three digits are not css shorthand", "0xfff", [3]uint8{0x00, 0x0F, 0xFF}, 0xFF, false},
		{"single digit", "0x0", [3]uint8{0x00, 0x00, 0x00}, 0xFF, false},

		// 7 digits falls on the RGBA side of NextUI's `digits <= 6` branch.
		{"seven digits is rgba", "0x1234567", [3]uint8{0x01, 0x23, 0x45}, 0x67, false},

		// Rejected. NextUI renders these black; we error so Load keeps the
		// default and logs, rather than painting a black screen silently.
		{"not hex", "notahex", [3]uint8{}, 0, true},
		{"empty", "", [3]uint8{}, 0, true},
		{"prefix only", "0x", [3]uint8{}, 0, true},
		{"css hash prefix unsupported", "#141414", [3]uint8{}, 0, true},
		{"too many digits", "0xFFFFFFFFF", [3]uint8{}, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseColor(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseColor(%q) = %#08x, want error", tc.in, uint32(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseColor(%q): unexpected error: %v", tc.in, err)
			}
			if rgb := got.RGB(); rgb != tc.wantRGB {
				t.Errorf("ParseColor(%q).RGB() = %v, want %v", tc.in, rgb, tc.wantRGB)
			}
			if a := got.A(); a != tc.wantA {
				t.Errorf("ParseColor(%q).A() = %#02x, want %#02x", tc.in, a, tc.wantA)
			}
		})
	}
}

// TestParseColor_IgnoresTrailingGarbage mirrors strtoul, which stops at the
// first non-hex byte rather than failing.
func TestParseColor_IgnoresTrailingGarbage(t *testing.T) {
	got, err := ParseColor("0x141414 # background")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rgb := got.RGB(); rgb != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("RGB() = %v, want [20 20 20]", rgb)
	}
}

func TestLuma_AndIsLight(t *testing.T) {
	tests := []struct {
		name    string
		c       [3]uint8
		isLight bool
	}{
		{"black", [3]uint8{0x00, 0x00, 0x00}, false},
		{"white", [3]uint8{0xFF, 0xFF, 0xFF}, true},
		{"catppuccin latte bg", [3]uint8{0xEF, 0xF1, 0xF5}, true},
		{"catppuccin mocha bg", [3]uint8{0x1E, 0x1E, 0x2E}, false},
		{"app default bg", [3]uint8{0x14, 0x14, 0x14}, false},
		{"mid grey is light at the 128 boundary", [3]uint8{0x80, 0x80, 0x80}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLight(tc.c); got != tc.isLight {
				t.Errorf("IsLight(%v) = %v (luma %d), want %v", tc.c, got, Luma(tc.c), tc.isLight)
			}
		})
	}
}

// TestShade_DarkTheme pins the arithmetic the renderer's modal fill relies on:
// two steps from the default background must equal the old literal `bg+20`.
func TestShade_DarkTheme(t *testing.T) {
	th := Defaults()
	if got := th.ShadeBG(2); got != [3]uint8{0x28, 0x28, 0x28} {
		t.Errorf("ShadeBG(2) = %v, want [40 40 40] (the old bg+20)", got)
	}
	// Separator must land on the grey the screens hardcoded as 50,50,50.
	if got := th.Separator(); got != [3]uint8{0x32, 0x32, 0x32} {
		t.Errorf("Separator() = %v, want [50 50 50]", got)
	}
}

// TestShade_LightTheme is the Catppuccin Latte case: shading must move *down*,
// not up, or panels vanish into the background.
func TestShade_LightTheme(t *testing.T) {
	th := Defaults()
	th.Background = [3]uint8{0xEF, 0xF1, 0xF5} // Latte
	got := th.ShadeBG(2)
	for i := range got {
		if got[i] >= th.Background[i] {
			t.Errorf("ShadeBG(2)[%d] = %d, want < %d (light theme must darken)",
				i, got[i], th.Background[i])
		}
	}
}

// TestShade_NoWrap is the regression this whole helper exists for: the old
// renderer did `bg[0]+20` on a uint8, which wraps to near-black on a light theme.
func TestShade_NoWrap(t *testing.T) {
	for _, bg := range [][3]uint8{
		{0xFF, 0xFF, 0xFF},
		{0x00, 0x00, 0x00},
		{0xFA, 0x02, 0xFE},
	} {
		th := Defaults()
		th.Background = bg
		for steps := 1; steps <= 30; steps++ {
			got := th.ShadeBG(steps)
			for i := range got {
				if th.IsLightTheme() && got[i] > bg[i] {
					t.Fatalf("bg %v steps %d: channel %d rose to %d (wrapped)", bg, steps, i, got[i])
				}
				if !th.IsLightTheme() && got[i] < bg[i] {
					t.Fatalf("bg %v steps %d: channel %d fell to %d (wrapped)", bg, steps, i, got[i])
				}
			}
		}
	}
}

// TestSurface_UsesHeaderBGWhenDistinct — the app's own default gives color3 a
// real 10-per-channel separation from the background, so it is used as-is and
// the bars render exactly as they always have.
func TestSurface_UsesHeaderBGWhenDistinct(t *testing.T) {
	th := Defaults()
	if got := th.Surface(); got != th.HeaderBG {
		t.Errorf("Surface() = %v, want HeaderBG %v", got, th.HeaderBG)
	}
}

// TestSurface_FallsBackWhenHeaderBGMatchesBackground — in every palette NextUI
// ships, color3 is equal or near-equal to color7 (byte-identical in Mocha and
// Latte). Using it raw would make the header and footer bars invisible.
func TestSurface_FallsBackWhenHeaderBGMatchesBackground(t *testing.T) {
	tests := []struct {
		name             string
		headerBG, bg     [3]uint8
		wantDistinctFrom bool
	}{
		{"mocha: color3 == color7", [3]uint8{0x1E, 0x1E, 0x2E}, [3]uint8{0x1E, 0x1E, 0x2E}, true},
		{"latte: color3 == color7", [3]uint8{0xEF, 0xF1, 0xF5}, [3]uint8{0xEF, 0xF1, 0xF5}, true},
		{"teal powder: 3 apart, under the threshold", [3]uint8{0x1A, 0x1A, 0x1A}, [3]uint8{0x1D, 0x1D, 0x1D}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th := Defaults()
			th.HeaderBG, th.Background = tc.headerBG, tc.bg
			got := th.Surface()
			if got == th.Background {
				t.Errorf("Surface() = %v, same as Background — bars would be invisible", got)
			}
		})
	}
}

// TestSurface_NextUIDefaultPaletteKeepsColor3 — NextUI's own Default.txt has a
// genuinely distinct color3, which should be honoured rather than derived.
func TestSurface_NextUIDefaultPaletteKeepsColor3(t *testing.T) {
	th := Defaults()
	th.HeaderBG = [3]uint8{0x1E, 0x23, 0x29} // Default.txt color3
	th.Background = [3]uint8{0x00, 0x00, 0x00}
	if got := th.Surface(); got != th.HeaderBG {
		t.Errorf("Surface() = %v, want the palette's color3 %v", got, th.HeaderBG)
	}
}

func TestRGBA_Accessors(t *testing.T) {
	c := RGBA(0x11223344)
	if rgb := c.RGB(); rgb != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("RGB() = %v, want [17 34 51]", rgb)
	}
	if a := c.A(); a != 0x44 {
		t.Errorf("A() = %#02x, want 0x44", a)
	}
}
