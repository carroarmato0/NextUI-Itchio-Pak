package theme

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Theme holds the 7 UI colors read from minuisettings.txt.
// All fields are [R,G,B] triples.
type Theme struct {
	Background [3]uint8 // color7 — clear color, panel fills
	HeaderBG   [3]uint8 // color3 — header + footer bar background
	Accent     [3]uint8 // color2 — selection pill, badges, separator
	AccentText [3]uint8 // color5 — text inside selection pill
	ListText   [3]uint8 // color4 — unselected row text
	HintText   [3]uint8 // color6 — footer hint labels
	MainText   [3]uint8 // color1 — author, metadata, description
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

		rgb, err := parseHex(v)
		if err != nil {
			logger.Warn("theme: %s: bad value %q: %v", k, v, err)
			continue
		}
		switch k {
		case "color1":
			th.MainText = rgb
			foundAny = true
		case "color2":
			th.Accent = rgb
			foundAny = true
		case "color3":
			th.HeaderBG = rgb
			foundAny = true
		case "color4":
			th.ListText = rgb
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

	if foundAny {
		logger.Info("theme: loaded from %s", path)
		logger.Debug("theme: bg=#%02X%02X%02X accent=#%02X%02X%02X main=#%02X%02X%02X",
			th.Background[0], th.Background[1], th.Background[2],
			th.Accent[0], th.Accent[1], th.Accent[2],
			th.MainText[0], th.MainText[1], th.MainText[2])
	} else {
		logger.Debug("theme: %s found but no valid color fields, using defaults", path)
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
	}
}

// parseHex parses a "0xRRGGBB" string into an [R,G,B] triple.
func parseHex(s string) ([3]uint8, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return [3]uint8{}, fmt.Errorf("missing 0x prefix")
	}
	n, err := strconv.ParseUint(s[2:], 16, 32)
	if err != nil {
		return [3]uint8{}, err
	}
	if n > 0xFFFFFF {
		return [3]uint8{}, fmt.Errorf("value %s out of 24-bit range", s)
	}
	return [3]uint8{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}
