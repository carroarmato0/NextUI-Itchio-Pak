package theme

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Predefined NextUI colour palettes (upstream PR #787).
//
// A palette is a plain text file holding the seven UI colours plus a name and a
// format version. NextUI ships eighteen and users can drop their own alongside
// them. We do not apply palettes ourselves — the active one is already baked
// into minuisettings.txt by the time we read it — but knowing which one is
// selected lets the Settings row say so, and enumerating them tells us in the
// log what a user actually has installed.
//
// Mirrors workspace/all/common/palette.c at upstream ebac427e.
const (
	// PaletteVersionMax is the highest palette-file format version understood.
	// Files declaring a higher version are skipped whole, matching
	// PALETTE_VERSION_MAX in palette.h.
	PaletteVersionMax = 1
	// PaletteColorCount is the number of colours in a palette (color1..color7).
	PaletteColorCount = 7
)

// Device paths. SDCardPath is "/mnt/SDCARD" on both TrimUI platforms
// (workspace/tg5040/platform/platform.h:156, tg5050:151); RES_PATH and
// SHARED_USERDATA_PATH are derived from it in common/defines.h:19,21.
//
// The Miyoo Flip (my355) has no NextUI platform upstream and may have none of
// these — every reader here degrades to "absent" rather than failing.
const (
	SDCardPath        = "/mnt/SDCARD"
	BuiltinPaletteDir = SDCardPath + "/.system/res/palettes"
	UserPaletteDir    = SDCardPath + "/Palettes"
	SettingsPath      = SDCardPath + "/.userdata/shared/minuisettings.txt"
)

// NextUIDefaultColors are CFG_DEFAULT_COLOR1..7 from common/config.h. A palette
// file that omits a colour inherits the corresponding default, so a partial file
// still yields seven.
var NextUIDefaultColors = [PaletteColorCount]RGBA{
	0xffffffff, // color1 main
	0x9b2257ff, // color2 accent
	0x1e2329ff, // color3 accent2
	0xffffffff, // color4 list text
	0x000000ff, // color5 list text selected
	0xffffffff, // color6 hint
	0x000000ff, // color7 background
}

// Palette is one predefined colour palette found on disk.
type Palette struct {
	Version int
	Name    string
	Path    string
	Builtin bool
	Colors  [PaletteColorCount]RGBA // index 0 == color1
}

// PaletteLabel renders a palette name for display. NextUI stores an empty name
// to mean the user has edited individual colours; it shows that as "Custom".
func PaletteLabel(name string) string {
	if name == "" {
		return "Custom"
	}
	return name
}

// LoadPalette reads one palette file. The bool is false when the file cannot be
// read or declares a format version newer than PaletteVersionMax.
func LoadPalette(path string, builtin bool) (Palette, bool) {
	f, err := os.Open(path)
	if err != nil {
		logger.Debug("theme: palette %s: %v", path, err)
		return Palette{}, false
	}
	defer f.Close()

	pal := Palette{
		Version: 1, // assumed when version= is omitted
		Path:    path,
		Builtin: builtin,
		Colors:  NextUIDefaultColors,
	}

	var haveName bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// NextUI matches keys with strncmp against the raw line, so a line with
		// leading whitespace is ignored. Match that: being stricter than the
		// device is safe, being looser would disagree with it.
		line := strings.TrimRight(scanner.Text(), "\r\n")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "version":
			// sscanf("%i") accepts hex and octal, so 0x2 is version 2.
			if n, err := strconv.ParseInt(v, 0, 32); err == nil {
				pal.Version = int(n)
			}
		case "name":
			pal.Name = v
			haveName = v != ""
		default:
			idx, ok := paletteColorIndex(k)
			if !ok {
				continue
			}
			c, err := ParseColor(v)
			if err != nil {
				logger.Warn("theme: palette %s: %s: bad value %q: %v", path, k, v, err)
				continue
			}
			pal.Colors[idx] = c
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("theme: palette %s: read: %v", path, err)
	}

	if pal.Version > PaletteVersionMax {
		logger.Warn("theme: palette %s: version=%d exceeds %d, skipping",
			path, pal.Version, PaletteVersionMax)
		return Palette{}, false
	}
	if !haveName {
		pal.Name = paletteNameFromFilename(filepath.Base(path))
	}
	logger.Debug("theme: palette %q loaded from %s (builtin=%v)", pal.Name, path, builtin)
	return pal, true
}

// paletteColorIndex maps "color1".."color7" to 0..6.
func paletteColorIndex(key string) (int, bool) {
	if len(key) != len("color1") || !strings.HasPrefix(key, "color") {
		return 0, false
	}
	n := int(key[len(key)-1] - '0')
	if n < 1 || n > PaletteColorCount {
		return 0, false
	}
	return n - 1, true
}

// paletteNameFromFilename strips a case-insensitive .txt and turns underscores
// into spaces, matching palette_nameFromFilename.
func paletteNameFromFilename(filename string) string {
	if len(filename) > 4 && strings.EqualFold(filename[len(filename)-4:], ".txt") {
		filename = filename[:len(filename)-4]
	}
	return strings.ReplaceAll(filename, "_", " ")
}

// EnumeratePalettes lists the palettes installed on the device, built-ins first
// then user drop-ins, matching PALETTE_enumerate's scan order. A directory that
// does not exist contributes nothing — on the hardware this was verified against
// there is no user palette directory at all.
func EnumeratePalettes(builtinDir, userDir string) []Palette {
	var out []Palette
	for _, d := range []struct {
		path    string
		builtin bool
	}{{builtinDir, true}, {userDir, false}} {
		n := len(out)
		out = append(out, scanPaletteDir(d.path, d.builtin)...)
		logger.Debug("theme: %d palettes in %s (builtin=%v)", len(out)-n, d.path, d.builtin)
	}
	logger.Info("theme: %d palettes available", len(out))
	return out
}

func scanPaletteDir(dir string, builtin bool) []Palette {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Debug("theme: palette dir %s: %v", dir, err)
		return nil
	}
	var out []Palette
	for _, e := range entries {
		name := e.Name()
		// Skip dot-files; require a name longer than ".txt" itself.
		if strings.HasPrefix(name, ".") || len(name) < 5 {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".txt") {
			continue
		}
		if pal, ok := LoadPalette(filepath.Join(dir, name), builtin); ok {
			out = append(out, pal)
		}
	}
	return out
}
