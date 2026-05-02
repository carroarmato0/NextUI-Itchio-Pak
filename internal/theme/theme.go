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

// Load reads minuisettings.txt and returns a Theme.
// Missing or unreadable files return defaults silently.
// Malformed lines are skipped with a WARN log; partial themes are valid.
func Load(path string) Theme {
	th := defaults()
	f, err := os.Open(path)
	if err != nil {
		return th
	}
	defer f.Close()

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
		rgb, err := parseHex(v)
		if err != nil {
			logger.Warn("theme: %s: bad value %q: %v", k, v, err)
			continue
		}
		switch k {
		case "color1":
			th.MainText = rgb
		case "color2":
			th.Accent = rgb
		case "color3":
			th.HeaderBG = rgb
		case "color4":
			th.ListText = rgb
		case "color5":
			th.AccentText = rgb
		case "color6":
			th.HintText = rgb
		case "color7":
			th.Background = rgb
		}
	}
	return th
}

// defaults returns the static grayscale fallback values.
// These match the hardcoded colors currently in the renderer and screens so
// a missing minuisettings.txt produces no visible change.
func defaults() Theme {
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
