package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The H700 pad's key bitmap as /proc/bus/input/devices prints it, for the
// button set NextUI's sibling firmware documents.
//
// Decoded, the KEY line is: 102, 103, 105, 106, 108 (power and the d-pad keys),
// 114, 115 (volume down/up), 304-312, 314, 315 (the pad itself) and 354
// (KEY_GOTO, which MENU emits alongside 312). Nineteen keys in total, which is
// what SDL reports as the joystick's button count.
//
// This set is not guessed. MinUI's rg35xxplus platform.h — the same H700
// hardware — publishes the SDL joystick index of every button, and this bitmap
// run through sdlButtonIndexes reproduces all of them: A=0, B=1, Y=2, X=3,
// L1=4, R1=5, SELECT=6, START=7, MENU=8, L2=9, R2=10, then UP=13, LEFT=14,
// RIGHT=15, DOWN=16, VOL-=17, VOL+=18. A six-point match on the keys we do not
// bind is what establishes the enumeration order used here.
const h700PadDevices = `I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="ANBERNIC-keys"
P: Phys=gpio-keys/input0
S: Sysfs=/devices/platform/gpio-keys/input/input2
U: Uniq=
H: Handlers=event1 js0
B: PROP=0
B: EV=100003
B: KEY=400000000 dff000000000000 0 0 c16c000000000 0
`

// The same pad with three extra keys ahead of BTN_SOUTH — 288, 289 and 290,
// which sit in SDL's first enumeration pass and so push every gamepad button
// three indices up. This is the shape the rc4 tester's device reports: their
// B acted as L1, X as Select, Y as R1 and L1 as Start, each exactly three
// bindings along, and their volume-down key acted as B, i.e. index 1.
const h700PadDevicesExtraButtons = `I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="ANBERNIC-keys"
P: Phys=gpio-keys/input0
S: Sysfs=/devices/platform/gpio-keys/input/input2
U: Uniq=
H: Handlers=event1 js0
B: PROP=0
B: EV=100003
B: KEY=400000000 dff000700000000 0 0 16c000000000 0
`

func h700Pad(buttons int) Pad {
	return Pad{
		GUID:    "030000000100000001000000abcd0000",
		Name:    "ANBERNIC-keys",
		Buttons: buttons,
		Hats:    1,
	}
}

// withInputDevices points the derivation at a fixture instead of the host's
// own /proc, so the tests describe a device rather than the machine they run on.
// An empty body stands for "the file is not readable", which is every non-Linux
// host and any device that does not mount procfs where it is expected.
func withInputDevices(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devices")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
	}
	old := procInputDevices
	procInputDevices = path
	t.Cleanup(func() { procInputDevices = old })
}

func bindings(t *testing.T, mapping string, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, field := range strings.Split(mapping, ",") {
		if k, v, ok := strings.Cut(field, ":"); ok {
			got[k] = v
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("binding %s = %q, want %q (mapping %q)", k, got[k], v, mapping)
		}
	}
}

// The whole point of deriving: the indices come from the device in front of
// the app, so a pad with no surprises gets the numbering its own firmware
// documents.
func TestControllerMappingH700DerivesIndexes(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, h700PadDevices)

	got, ok := newNextUI("").ControllerMapping(h700Pad(19))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{
		"a": "b0", "b": "b1", "x": "b2", "y": "b3",
		"leftshoulder": "b4", "rightshoulder": "b5",
		"back": "b6", "start": "b7", "guide": "b8",
		"lefttrigger": "b9", "righttrigger": "b10",
		"dpup": "h0.1", "dpright": "h0.2", "dpdown": "h0.4", "dpleft": "h0.8",
	})
}

// A pad carrying three keys SDL enumerates first. Hardcoded indices are wrong
// here by exactly three, which is what rc4 shipped and what the tester felt.
func TestControllerMappingH700DerivesShiftedIndexes(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, h700PadDevicesExtraButtons)

	got, ok := newNextUI("").ControllerMapping(h700Pad(20))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{
		"a": "b3", "b": "b4", "x": "b5", "y": "b6",
		"leftshoulder": "b7", "rightshoulder": "b8",
		"back": "b9", "start": "b10", "guide": "b11",
		"lefttrigger": "b12", "righttrigger": "b13",
	})
}

// The kernel prints the bitmap in native words, and an armhf kernel's are half
// as wide. Word width is not stated in the file, so it is inferred — wrongly
// inferred, and every code lands in the wrong place.
func TestControllerMappingH700Reads32BitBitmaps(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, strings.Replace(h700PadDevices,
		"B: KEY=400000000 dff000000000000 0 0 c16c000000000 0",
		"B: KEY=4 0 dff0000 0 0 0 0 0 c16c0 0 0 0", 1))

	got, ok := newNextUI("").ControllerMapping(h700Pad(19))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{"a": "b0", "b": "b1", "leftshoulder": "b4"})
}

// Without a readable /proc the app still has to bind something, and the only
// H700 measurement anyone has taken is the rc4 tester's: three buttons ahead
// of BTN_SOUTH.
func TestControllerMappingH700FallsBackToMeasuredIndexes(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, "")

	got, ok := newNextUI("").ControllerMapping(h700Pad(20))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	// The GUID must lead: SDL keys the whole database on it, so a mapping
	// carrying the wrong one is silently ignored rather than rejected.
	if !strings.HasPrefix(got, "030000000100000001000000abcd0000,ANBERNIC-keys,") {
		t.Errorf("mapping does not start with GUID and name: %q", got)
	}
	// SDL skips any mapping whose platform does not match the running one.
	if !strings.Contains(got, "platform:Linux") {
		t.Errorf("mapping missing platform:Linux: %q", got)
	}
	bindings(t, got, map[string]string{
		"a": "b3", "b": "b4", "x": "b5", "y": "b6",
		"leftshoulder": "b7", "rightshoulder": "b8",
		"back": "b9", "start": "b10", "guide": "b11",
		"lefttrigger": "b12", "righttrigger": "b13",
		"dpup": "h0.1", "dpright": "h0.2", "dpdown": "h0.4", "dpleft": "h0.8",
	})
}

// A bitmap that decodes to a different number of buttons than SDL reports was
// read from the wrong device, or read wrongly. Either way the indices it yields
// are fiction, and the measured fallback is the better guess.
func TestControllerMappingH700RejectsMismatchedButtonCount(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, h700PadDevices)

	got, ok := newNextUI("").ControllerMapping(h700Pad(19 + 1))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{"a": "b3", "b": "b4"})
}

// Handhelds expose several input devices — a power button, an accelerometer,
// the HDMI audio jack. The pad is picked by the name SDL reports for it.
func TestControllerMappingH700PicksThePadByName(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, `I: Bus=0019 Vendor=0000 Product=0001 Version=0000
N: Name="axp2202-pek"
P: Phys=m1kbd/input2
U: Uniq=
H: Handlers=kbd event0
B: PROP=0
B: EV=3
B: KEY=10000000000000 0

`+h700PadDevices)

	got, ok := newNextUI("").ControllerMapping(h700Pad(19))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{"a": "b0", "leftshoulder": "b4"})
}

// SDL enumerates BTN_JOYSTICK..KEY_MAX first and everything below it after, so
// a key's index depends on which side of 0x120 it falls, not on its value.
func TestSDLButtonIndexes(t *testing.T) {
	got := sdlButtonIndexes([]int{115, 304, 305, 102, 354, 767, 800})
	want := map[int]int{304: 0, 305: 1, 354: 2, 102: 3, 115: 4}
	for code, idx := range want {
		if got[code] != idx {
			t.Errorf("index of %d = %d, want %d", code, got[code], idx)
		}
	}
	// KEY_MAX and anything above it is outside both of SDL's loops.
	for _, code := range []int{767, 800} {
		if _, ok := got[code]; ok {
			t.Errorf("code %d was given an index, want none", code)
		}
	}
}
