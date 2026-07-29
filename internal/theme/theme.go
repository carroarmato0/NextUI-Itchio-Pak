package theme

import (
	"bufio"
	"os"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Theme holds the 7 UI colors read from minuisettings.txt, mapped to the roles
// NextUI itself gives them. All fields are [R,G,B] triples.
//
// The mapping is not a guess. NextUI draws a selected row with color1 as the
// pill fill and color5 as its text, and an unselected row with color3 as the
// surface and color4 as its text (nextui.c:2733-2739, and again at :2781-2787).
// The two pill helpers agree: GFX_blitPillDark fills with color1 and
// GFX_blitPillLight with color2 (api.c:1891-1898).
//
// Two consequences worth knowing:
//
//   - MainText is color4, not color1. NextUI has no dedicated body-text colour,
//     and color1 is a pill fill — on Catppuccin Mocha it is a bright mauve
//     (#CBA6F7), which is unreadable as running text on the background.
//   - HeaderBG holds color3 raw, but nothing should draw with it directly:
//     color3 equals or nearly equals color7 in every palette NextUI ships, so a
//     bar filled with it disappears. Use Surface() instead.
type Theme struct {
	Background [3]uint8 // color7 — clear color, panel fills
	HeaderBG   [3]uint8 // color3 — secondary accent; read it through Surface()
	Accent     [3]uint8 // color1 — selection pill, footer badges
	AccentText [3]uint8 // color5 — text on the Accent pill
	ListText   [3]uint8 // color4 — unselected row text
	HintText   [3]uint8 // color6 — footer hint labels
	MainText   [3]uint8 // color4 — body text: author, metadata, description
	TitlePill  [3]uint8 // color2 — title / status pill fill
}

// Load reads minuisettings.txt and returns a Theme and a boolean indicating
// if the file was found and at least one valid color field was loaded.
// Missing or unreadable files return defaults and false.
func Load(path string) (Theme, bool) {
	th := Defaults()
	f, err := os.Open(path)
	if err != nil {
		logger.Debug("theme: configuration not found at %s, using defaults", path)
		return th, false
	}
	defer f.Close()

	var foundAny bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		// Only parse recognized color fields to avoid log noise for other settings.
		var isColorField bool
		switch k {
		case "color1", "color2", "color3", "color4", "color5", "color6", "color7":
			isColorField = true
		}
		if !isColorField {
			continue
		}

		c, err := ParseColor(v)
		if err != nil {
			logger.Warn("theme: %s: bad value %q: %v", k, v, err)
			continue
		}
		// The renderer draws every primitive opaque (SDL_BLENDMODE_NONE), so
		// alpha is parsed only to be dropped. Log it so there is evidence if a
		// palette ever ships a translucent colour.
		if !c.Opaque() {
			logger.Warn("theme: %s: alpha 0x%02X ignored, renderer is opaque", k, c.A())
		}
		rgb := c.RGB()
		switch k {
		case "color1":
			th.Accent = rgb
			foundAny = true
		case "color2":
			th.TitlePill = rgb
			foundAny = true
		case "color3":
			th.HeaderBG = rgb
			foundAny = true
		case "color4":
			// NextUI has no separate body-text colour; color4 serves both.
			th.ListText = rgb
			th.MainText = rgb
			foundAny = true
		case "color5":
			th.AccentText = rgb
			foundAny = true
		case "color6":
			th.HintText = rgb
			foundAny = true
		case "color7":
			th.Background = rgb
			foundAny = true
		}
	}

	if err := scanner.Err(); err != nil {
		// A truncated read leaves us with whatever parsed so far; say so rather
		// than reporting a partial palette as a clean load.
		logger.Warn("theme: read %s: %v (colors parsed so far are kept)", path, err)
	}

	if foundAny {
		logger.Info("theme: loaded from %s", path)
		logger.Debug("theme: bg=#%02X%02X%02X accent=#%02X%02X%02X main=#%02X%02X%02X",
			th.Background[0], th.Background[1], th.Background[2],
			th.Accent[0], th.Accent[1], th.Accent[2],
			th.MainText[0], th.MainText[1], th.MainText[2])
	} else {
		// Loud on purpose. This is exactly how the PR #762 regression presented:
		// the file was there, every colour failed to parse, and the NextUI theme
		// toggle silently vanished from Settings because themeAvailable is this
		// return value. If upstream changes the colour format again, this is the
		// line that will say so.
		logger.Warn("theme: %s found but no color fields parsed, using defaults "+
			"(NextUI theme toggle will be hidden)", path)
	}

	return th, foundAny
}

// Defaults returns the static grayscale fallback values.
// These match the hardcoded colors currently in the renderer and screens so
// a missing minuisettings.txt produces no visible change.
func Defaults() Theme {
	return Theme{
		Background: [3]uint8{0x14, 0x14, 0x14}, // #141414
		HeaderBG:   [3]uint8{0x1E, 0x1E, 0x1E}, // #1E1E1E
		Accent:     [3]uint8{0x3C, 0x3C, 0x5C}, // #3C3C5C
		AccentText: [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
		ListText:   [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
		HintText:   [3]uint8{0x8C, 0x8C, 0x8C}, // #8C8C8C
		MainText:   [3]uint8{0xDC, 0xDC, 0xDC}, // #DCDCDC
		// #303040 — exactly what the sort pill's old `Accent/2+18` produced
		// from the default Accent, so that pill does not shift.
		TitlePill: [3]uint8{0x30, 0x30, 0x40},
	}
}
