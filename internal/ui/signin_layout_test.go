package ui

import "testing"

// The sign-in QR code is the one thing on the screen that has to work from a
// phone camera. Every shipping geometry must give it whole-pixel modules of a
// scannable size, inside the content area, clear of the text.
func TestLayoutSignIn_allGeometries(t *testing.T) {
	const modules = 33 + 2*qrQuietZone // a version-4 code, as the sign-in URL encodes to
	for _, g := range []struct {
		w, h      int32
		stacked   bool
		minModule int32
	}{
		{1024, 768, false, 10},
		{640, 480, false, 5},
		{720, 480, false, 5},
		{720, 720, true, 7},
	} {
		fontH, smallFH := g.h/22, g.h/32
		l := layoutSignIn(g.w, g.h, modules, fontH, smallFH)
		name := func() string { return itoa(g.w) + "x" + itoa(g.h) }

		if l.Stacked != g.stacked {
			t.Errorf("%s: stacked = %v, want %v", name(), l.Stacked, g.stacked)
		}
		if l.Module < g.minModule {
			t.Errorf("%s: %dpx modules, want at least %d", name(), l.Module, g.minModule)
		}
		if l.QRSize != l.Module*int32(modules) {
			t.Errorf("%s: QR %dpx is not a whole number of %dpx modules", name(), l.QRSize, l.Module)
		}
		if l.QRY < signInHeaderH || l.QRY+l.QRSize > g.h-signInFooterH || l.QRX < 0 || l.QRX+l.QRSize > g.w {
			t.Errorf("%s: QR %+v leaves the content area", name(), l)
		}
		if l.Stacked {
			if l.TextY < l.QRY+l.QRSize {
				t.Errorf("%s: text starts at %d, inside the QR code", name(), l.TextY)
			}
		} else if l.TextX < l.QRX+l.QRSize || l.TextX+l.TextW > g.w {
			t.Errorf("%s: text column %d..%d overlaps the code or the edge", name(), l.TextX, l.TextX+l.TextW)
		}
		t.Logf("%s: %dpx modules, QR %dpx", name(), l.Module, l.QRSize)
	}
}

func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
