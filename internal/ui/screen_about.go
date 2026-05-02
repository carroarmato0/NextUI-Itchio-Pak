//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

// appVersion is set at build time via -ldflags:
//
//	-X github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=vX.Y.Z
var appVersion = "dev"

const (
	appRepoURL   = "https://github.com/carroarmato0/NextUI-Itchio-Pak"
	appDescLine1 = "Browse and download GB/GBC games"
	appDescLine2 = "from Itch.io's GB Studio collection."
	appNote      = "Unofficial community Pak — not affiliated with Itch.io."
)

type AboutScreen struct {
	prev Screen
}

func NewAboutScreen(prev Screen) *AboutScreen {
	return &AboutScreen{prev: prev}
}

func (s *AboutScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(72)
	footerH := int32(52)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("About", 20, textY, mt[0], mt[1], mt[2])

	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	// ── Text block ──────────────────────────────────────────────
	y := headerH + 20
	r.DrawTextCentered(appDescLine1, 0, y, r.W, mt[0], mt[1], mt[2])
	y += fontH + 6
	r.DrawTextCentered(appDescLine2, 0, y, r.W, mt[0], mt[1], mt[2])
	y += fontH + 16
	ht := r.Theme.HintText
	r.DrawSmallTextCentered("Version "+appVersion, 0, y, r.W, ht[0], ht[1], ht[2])
	y += smallFH + 10
	r.DrawSmallTextCentered(appNote, 0, y, r.W, 100, 100, 100)

	// ── QR code centered in the remaining space ─────────────────
	contentBottom := r.H - footerH - 4
	qrAreaTop := y + smallFH + 14
	qrAreaH := contentBottom - qrAreaTop

	qrSize := qrAreaH - 2*smallFH - 12 // leave room for "Scan to open" label
	if qrSize > r.W/3 {
		qrSize = r.W / 3
	}
	if qrSize < 80 {
		qrSize = 80
	}
	if qrSize > 400 {
		qrSize = 400
	}

	tex, err := r.QRTexture(appRepoURL, int(qrSize))
	if err == nil && tex != nil {
		qrX := (r.W - qrSize) / 2
		qrY := qrAreaTop + (qrAreaH-qrSize-smallFH-6)/2
		r.DrawTextureAt(tex, qrX, qrY, qrSize, qrSize)
		tex.Destroy()
		r.DrawSmallTextCentered("Scan to open project page", 0, qrY+qrSize+4, r.W, 120, 120, 120)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *AboutScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_ESCAPE, sdl.K_RETURN, sdl.K_s:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	}
	return s
}
