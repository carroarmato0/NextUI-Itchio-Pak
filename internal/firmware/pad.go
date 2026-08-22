package firmware

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Pad is what SDL reports about a joystick at startup.
//
// It carries no SDL types: this package is compiled into the headless build,
// where there is no SDL at all, and the mapping derivation is worth testing
// without a device or a display.
type Pad struct {
	// GUID is SDL's own identifier for the pad, as a hex string. SDL keys its
	// controller database on it.
	GUID string
	// Name is the device name SDL read from the kernel — the same string
	// /proc/bus/input/devices prints, which is how the two are matched up.
	Name string
	// Buttons and Hats are what SDL reports for the opened joystick.
	Buttons, Hats int
}

// Boundaries of SDL's two button-enumeration passes over the evdev key bitmap.
const (
	btnJoystick = 0x120 // BTN_JOYSTICK
	keyMax      = 0x2ff // KEY_MAX — SDL's loops stop below this, so it is excluded
)

// h700PadButtons is the H700 pad's SDL binding -> evdev code table.
//
// Measured on hardware: a tester captured /dev/input/event1 while pressing each
// button in a stated order. Only the codes are measured here — what index SDL
// gives each code is derived per device, because that depends on which *other*
// keys the device exposes, and the RG XX family does not agree about those.
//
// The x/y rows look transposed and are not: the shell's Y key emits 306 and its
// X key emits 307, while SDL's x/y are positional. Binding "x" to 306 is what
// makes SDL_CONTROLLER_BUTTON_X arrive from the button labelled Y, which is the
// transposition FaceABDirect describes on the other side. The two must agree.
var h700PadButtons = []struct {
	binding string
	code    int
}{
	{"a", 304}, {"b", 305}, {"x", 306}, {"y", 307},
	{"leftshoulder", 308}, {"rightshoulder", 309},
	{"back", 310}, {"start", 311}, {"guide", 312},
	{"lefttrigger", 314}, {"righttrigger", 315},
}

// h700DPadBindings binds the d-pad, which is a hat (ABS_HAT0X/ABS_HAT0Y) rather
// than buttons or axes. The bits are SDL's: up=1, right=2, down=4, left=8.
const h700DPadBindings = "dpup:h0.1,dpright:h0.2,dpdown:h0.4,dpleft:h0.8"

// h700MeasuredOffset is how many buttons SDL enumerates ahead of the pad's own,
// on the one H700 device anyone has reported button behaviour from.
//
// It is only reached when the derivation cannot run — an unreadable /proc, a
// pad under a name that does not appear there, a bitmap that decodes to a
// different button count than SDL reports. Three is a measurement from a single
// SKU and the family has eleven, so this is a fallback and not the mechanism:
// rc4 shipped these indices hardcoded at an offset of zero and every button in
// the app was three bindings out.
const h700MeasuredOffset = 3

// procInputDevices lists every input device the kernel has, with the key
// bitmap of each. Overridden in tests.
var procInputDevices = "/proc/bus/input/devices"

// ControllerMapping returns an SDL game-controller mapping line for this
// device's pad, and whether one is needed at all.
//
// pad comes from SDL at runtime rather than from constants: SDL keys its
// database on the GUID, and a mapping carrying the wrong one is ignored
// silently rather than rejected. The GUID is built from the pad's bus, vendor,
// product and version plus a hash of its name, with a different layout when
// vendor and product are both zero — reconstructing that here would be a guess,
// and eleven RG XX SKUs may not all produce the same value.
//
// Only H700 gets a mapping. Every other supported platform's SDL2 already
// classifies its pad as a game controller, and overriding a working mapping
// would swap face buttons on hardware that is fine today.
func (e *Env) ControllerMapping(pad Pad) (string, bool) {
	if e.kind == KindMuOS || e.device != "h700" {
		return "", false
	}
	if pad.Hats == 0 {
		// Worth saying out loud: the d-pad bindings below cannot fire, so if
		// the d-pad still works it is arriving by some other route.
		logger.Warn("input: pad %q reports no hats, its d-pad bindings will never fire", pad.Name)
	}
	// A mapping line is comma-separated, so a comma in the name would shift
	// every binding after it by one field.
	name := strings.ReplaceAll(pad.Name, ",", " ")
	return pad.GUID + "," + name + ",platform:Linux," + h700Bindings(pad), true
}

// h700Bindings is the button half of the mapping line, with each button's index
// derived from the pad in front of the app where that is possible.
func h700Bindings(pad Pad) string {
	index, derived := padButtonIndexes(pad)
	if !derived {
		index = make(map[int]int, len(h700PadButtons))
		for i, b := range h700PadButtons {
			index[b.code] = i + h700MeasuredOffset
		}
	}

	var out []string
	var missing []string
	for _, b := range h700PadButtons {
		i, ok := index[b.code]
		if !ok {
			missing = append(missing, fmt.Sprintf("%s(%d)", b.binding, b.code))
			continue
		}
		out = append(out, fmt.Sprintf("%s:b%d", b.binding, i))
	}
	if len(missing) > 0 {
		// Not fatal — a pad missing L2 is still worth driving — but it is the
		// first thing to look at if a tester says one button does nothing.
		logger.Warn("input: pad %q exposes no key for %s", pad.Name, strings.Join(missing, " "))
	}
	out = append(out, h700DPadBindings)

	how := "measured fallback"
	if derived {
		how = "derived from the pad's key bitmap"
	}
	logger.Info("input: h700 bindings (%s): %s", how, strings.Join(out, ","))
	return strings.Join(out, ",")
}

// padButtonIndexes works out which button index SDL will report for each of the
// pad's evdev key codes, by reading the same bitmap SDL read.
//
// The count SDL reports is the check that this read the right device and read
// it correctly: it is the number of keys in the bitmap, so a decode that
// produces a different number of them is wrong, and its indices are fiction.
func padButtonIndexes(pad Pad) (map[int]int, bool) {
	if pad.Name == "" || pad.Buttons == 0 {
		logger.Warn("input: pad reports name=%q buttons=%d, cannot derive its button indexes", pad.Name, pad.Buttons)
		return nil, false
	}
	data, err := os.ReadFile(procInputDevices)
	if err != nil {
		logger.Warn("input: reading %s: %v — falling back to measured button indexes", procInputDevices, err)
		return nil, false
	}
	bitmaps := keyBitmapsFor(string(data), pad.Name)
	if len(bitmaps) == 0 {
		logger.Warn("input: no device named %q in %s — falling back to measured button indexes", pad.Name, procInputDevices)
		return nil, false
	}
	// The kernel prints the bitmap in native words and does not say how wide
	// they are, so both widths are tried and the decode has to prove itself.
	// The button count alone cannot: a line read at the wrong width has the
	// same number of bits set, just in the wrong places. Finding the four face
	// buttons where they belong is what says the width was right.
	for _, bitmap := range bitmaps {
		for _, wordBits := range []int{64, 32} {
			codes, err := parseKeyBitmap(bitmap, wordBits)
			if err != nil {
				logger.Warn("input: parsing KEY bitmap %q: %v", bitmap, err)
				continue
			}
			index := sdlButtonIndexes(codes)
			if len(index) != pad.Buttons || !hasFaceButtons(index) {
				continue
			}
			logger.Info("input: pad %q key bitmap decoded at %d-bit words, %d buttons", pad.Name, wordBits, len(index))
			logger.Debug("input: pad %q evdev code -> SDL button: %s", pad.Name, formatIndexes(index))
			return index, true
		}
	}
	logger.Warn("input: no KEY bitmap for %q decodes to the %d buttons SDL reports — falling back to measured button indexes",
		pad.Name, pad.Buttons)
	return nil, false
}

// hasFaceButtons reports whether a decoded bitmap contains the four face
// buttons. They are the load-bearing four — confirm, cancel and the two the
// screens hang delete and clear-filters off — so a decode without them is not
// worth binding even if the rest of it looks plausible.
func hasFaceButtons(index map[int]int) bool {
	for _, b := range h700PadButtons[:4] {
		if _, ok := index[b.code]; !ok {
			return false
		}
	}
	return true
}

// sdlButtonIndexes numbers evdev key codes the way SDL's Linux joystick backend
// does: BTN_JOYSTICK..KEY_MAX in ascending order first, then everything below
// BTN_JOYSTICK. A key's index therefore depends on which side of BTN_JOYSTICK
// it falls, not on its value — a pad whose volume keys are KEY_VOLUMEDOWN and
// KEY_VOLUMEUP numbers its face buttons from zero, and one whose extra keys are
// in the BTN_ range does not.
//
// The order is not taken on trust. MinUI's rg35xxplus platform.h publishes the
// joystick index of all nineteen keys on this hardware, and this function
// reproduces every one of them from the device's bitmap — including the six
// keys the app does not bind, which is what makes it a check rather than a
// restatement.
func sdlButtonIndexes(codes []int) map[int]int {
	sorted := append([]int(nil), codes...)
	sort.Ints(sorted)

	index := make(map[int]int, len(sorted))
	n := 0
	for _, code := range sorted {
		if code >= btnJoystick && code < keyMax {
			index[code] = n
			n++
		}
	}
	for _, code := range sorted {
		if code < btnJoystick {
			index[code] = n
			n++
		}
	}
	return index
}

// keyBitmapsFor returns the KEY bitmap line of every device in
// /proc/bus/input/devices carrying the given name. Names are not unique — a
// handheld can present the same gpio-keys node twice — so the caller decides
// which one decodes correctly.
func keyBitmapsFor(devices, name string) []string {
	var out []string
	match := false
	for _, line := range strings.Split(devices, "\n") {
		switch {
		case strings.HasPrefix(line, "N: Name="):
			match = strings.Trim(strings.TrimPrefix(line, "N: Name="), `"`) == name
		case match && strings.HasPrefix(line, "B: KEY="):
			out = append(out, strings.TrimPrefix(line, "B: KEY="))
		}
	}
	return out
}

// parseKeyBitmap turns one KEY= line into the evdev codes it has set.
//
// The kernel prints the bitmap as native words in hex, most significant word
// first, dropping leading empty words — so a word's position in the line gives
// its index, and only the width of a word has to be worked out.
func parseKeyBitmap(bitmap string, wordBits int) ([]int, error) {
	words := strings.Fields(bitmap)
	if len(words) == 0 {
		return nil, fmt.Errorf("no words")
	}
	var codes []int
	for i, word := range words {
		value, err := strconv.ParseUint(word, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("word %q: %w", word, err)
		}
		if wordBits < 64 && value >= 1<<wordBits {
			return nil, fmt.Errorf("word %q does not fit %d bits", word, wordBits)
		}
		base := (len(words) - 1 - i) * wordBits
		for bit := 0; bit < wordBits; bit++ {
			if value&(1<<uint(bit)) != 0 {
				codes = append(codes, base+bit)
			}
		}
	}
	return codes, nil
}

// formatIndexes renders the derived table for the log, in index order — the
// order a tester pressing buttons produces, so a log and a report line up.
func formatIndexes(index map[int]int) string {
	byIndex := make([]string, len(index))
	for code, i := range index {
		if i < len(byIndex) {
			byIndex[i] = fmt.Sprintf("b%d=%d", i, code)
		}
	}
	return strings.Join(byIndex, " ")
}
