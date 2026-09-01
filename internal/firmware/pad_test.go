package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// h700PadDevices is /proc/bus/input/devices as an Anbernic RG XX prints it,
// captured verbatim from the tester who reported rc4's buttons.
//
// The pad's KEY line decodes to fifteen keys: 1 (KEY_ESC), 114 and 115 (volume
// down and up), 304-312 and 314, 315 (the pad itself) and 354 (KEY_GOTO, which
// MENU emits alongside 312). Numbered in ascending order that puts A at b3, and
// every one of the tester's nine observations follows from it: their B produced
// L1 (305 -> b4, which rc4 had bound to leftshoulder), their X produced Select,
// their volume-down key produced B.
const h700PadDevices = `I: Bus=0000 Vendor=0000 Product=0000 Version=0000
N: Name="axp2202-pek"
P: Phys=m1kbd/input2
S: Sysfs=/devices/platform/soc/twi5/i2c-5/5-0034/axp2101-pek.0/input/input0
U: Uniq=
H: Handlers=kbd event0
B: PROP=0
B: EV=100003
B: KEY=12c00000000000 0

I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="ANBERNIC-keys"
P: Phys=gpio-keys-polled/input0
S: Sysfs=/devices/platform/soc/soc@03000000:gpio_keys/input/input1
U: Uniq=
H: Handlers=kbd js0 event1
B: PROP=0
B: EV=20000b
B: KEY=400000000 dff000000000000 0 0 c000000000000 2
B: ABS=30038
B: FF=107030000 0

I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="dierct-keys-polled"
P: Phys=dierct-keys-polled/input0
S: Sysfs=/devices/platform/dierct-keys-polled/input/input2
U: Uniq=
H: Handlers=kbd event2
B: PROP=0
B: EV=3
B: KEY=c378000000000 c042e2100000
`

// h700PadDevicesNoLowKeys is a pad carrying nothing but its own buttons — no
// power key, no volume keys. Its twelve keys number from zero, so its A is b0.
//
// The family has eleven SKUs and one dump between them, so this stands for the
// ones nobody has seen: it is the case a fixed offset gets wrong.
const h700PadDevicesNoLowKeys = `I: Bus=0019 Vendor=0001 Product=0001 Version=0100
N: Name="other-keys"
P: Phys=gpio-keys-polled/input0
S: Sysfs=/devices/platform/other/input/input3
U: Uniq=
H: Handlers=kbd js0 event3
B: PROP=0
B: EV=20000b
B: KEY=400000000 dff000000000000 0 0 0 0
B: ABS=30038
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

// The device the rc4 report came from. These are the indices that report
// implies, read out of the pad's own key bitmap rather than assumed.
func TestControllerMappingH700DerivesIndexes(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, h700PadDevices)

	got, ok := newNextUI("").ControllerMapping(h700Pad(15))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{
		"a": "b3", "b": "b4", "x": "b5", "y": "b6",
		"leftshoulder": "b7", "rightshoulder": "b8",
		"back": "b9", "start": "b10", "guide": "b11",
		"lefttrigger": "b12", "righttrigger": "b13",
		"dpup": "h0.1", "dpright": "h0.2", "dpdown": "h0.4", "dpleft": "h0.8",
	})
}

// The offset is not the fix — reading the device is. A pad without the three
// low keys numbers its buttons from zero, and gets bound that way even though
// the only H700 anyone has measured needs three.
//
// It also has to be found by name: the fixture holds the rc4 tester's pad too,
// and binding that one's fifteen keys to this one's twelve buttons would fail
// the count check and fall back to the offset this asserts against.
func TestControllerMappingH700DerivesFromTheDeviceNotAnOffset(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, h700PadDevices+"\n"+h700PadDevicesNoLowKeys)

	pad := h700Pad(12)
	pad.Name = "other-keys"
	got, ok := newNextUI("").ControllerMapping(pad)
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{
		"a": "b0", "b": "b1", "x": "b2", "y": "b3",
		"leftshoulder": "b4", "rightshoulder": "b5",
		"back": "b6", "start": "b7", "guide": "b8",
		"lefttrigger": "b9", "righttrigger": "b10",
	})
}

// The kernel prints the bitmap in native words, and a 32-bit kernel's are half
// as wide. The file does not say which, and a line read at the wrong width has
// the same number of bits set — just in the wrong places — so the button count
// cannot catch it and finding the face buttons is what does.
func TestControllerMappingH700Reads32BitBitmaps(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, strings.Replace(h700PadDevices,
		"B: KEY=400000000 dff000000000000 0 0 c000000000000 2",
		"B: KEY=4 0 dff0000 0 0 0 0 0 c0000 0 0 2", 1))

	got, ok := newNextUI("").ControllerMapping(h700Pad(15))
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	bindings(t, got, map[string]string{"a": "b3", "b": "b4", "leftshoulder": "b7"})
}

// Without a readable /proc the app still has to bind something, and the only
// H700 anyone has dumped puts three keys ahead of the pad's own.
func TestControllerMappingH700FallsBackToMeasuredIndexes(t *testing.T) {
	t.Setenv("PLATFORM", "h700")
	withInputDevices(t, "")

	got, ok := newNextUI("").ControllerMapping(h700Pad(15))
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
	withInputDevices(t, h700PadDevicesNoLowKeys)

	pad := h700Pad(12 + 1)
	pad.Name = "other-keys"
	got, ok := newNextUI("").ControllerMapping(pad)
	if !ok {
		t.Fatal("ControllerMapping() ok = false, want a mapping for h700")
	}
	// Its own bitmap would have said b0. Twelve keys is not thirteen buttons,
	// so the read is discarded and the fallback binds instead.
	bindings(t, got, map[string]string{"a": "b3", "b": "b4"})
}

// Every key set in the bitmap takes an index, in ascending order, whether or
// not a pad button produces it — which is why the power and volume keys move
// the gamepad's own buttons along.
func TestSDLButtonIndexes(t *testing.T) {
	got := sdlButtonIndexes([]int{115, 304, 305, 102, 354, 767, 800})
	want := map[int]int{102: 0, 115: 1, 304: 2, 305: 3, 354: 4}
	for code, idx := range want {
		if got[code] != idx {
			t.Errorf("index of %d = %d, want %d", code, got[code], idx)
		}
	}
	// KEY_MAX and anything above it is outside SDL's enumeration.
	for _, code := range []int{767, 800} {
		if _, ok := got[code]; ok {
			t.Errorf("code %d was given an index, want none", code)
		}
	}
}
