package ui

// Layout of the QR sign-in screen, kept free of SDL so every geometry can be
// tested on the host. See docs/mockups/signin.html for the approved design.

const (
	signInHeaderH = int32(72)
	signInFooterH = int32(52)
	signInPad     = int32(20)
	// qrQuietZone is the white margin around a QR code, in modules; four is
	// the minimum the QR specification asks for.
	qrQuietZone = 4
)

// signInLayout positions the QR code and the text that goes with it.
type signInLayout struct {
	// Stacked puts the text under the code, for square screens; otherwise it
	// sits in a column to the right.
	Stacked bool
	// Module is the size of one QR module in whole pixels, which keeps the
	// edges sharp for a phone camera. QRSize includes the quiet zone.
	Module, QRSize int32
	QRX, QRY       int32
	TextX, TextY   int32
	TextW          int32
	// CodeSize is the pixel size of the font the user code is drawn in.
	CodeSize int
}

// layoutSignIn fits a QR code of modules modules (quiet zone included) into a
// w×h screen whose main and small fonts are fontH and smallFH tall.
func layoutSignIn(w, h int32, modules int, fontH, smallFH int32) signInLayout {
	top := signInHeaderH + signInPad
	bottom := h - signInFooterH - signInPad
	n := int32(modules)
	l := signInLayout{Stacked: h >= w, CodeSize: int(fontH * 2)}

	if l.Stacked {
		// Under the code: the hint, the user code (about two main lines),
		// the timer and the address, each separated by half a pad.
		block := 3*smallFH + 2*fontH + 4*(signInPad/2)
		side := min32(bottom-top-block, w-4*signInPad)
		l.Module = max32(side/n, 1)
		l.QRSize = l.Module * n
		l.QRX = (w - l.QRSize) / 2
		l.QRY = top
		l.TextX = 2 * signInPad
		l.TextY = l.QRY + l.QRSize + signInPad/2
		l.TextW = w - 4*signInPad
		return l
	}

	// Beside the code: at most half the width, so the code and the text next
	// to it both get room.
	side := min32(bottom-top, w/2-2*signInPad)
	l.Module = max32(side/n, 1)
	l.QRSize = l.Module * n
	l.QRX = 2 * signInPad
	l.QRY = top + (bottom-top-l.QRSize)/2
	l.TextX = l.QRX + l.QRSize + 2*signInPad
	l.TextY = l.QRY
	l.TextW = w - l.TextX - 2*signInPad
	return l
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
