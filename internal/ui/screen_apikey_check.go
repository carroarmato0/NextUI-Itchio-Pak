//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type keyTestState int

const (
	keyTestRunning keyTestState = iota
	keyTestOK
	keyTestFail
)

// KeyTestScreen validates the configured API key against itch.io and shows
// the authenticated username and number of owned games on success.
type KeyTestScreen struct {
	client *itchio.Client
	cfg    *settings.Config
	prev   Screen

	state      keyTestState
	username   string
	ownedCount int
	err        error
}

func NewKeyTestScreen(client *itchio.Client, cfg *settings.Config, prev Screen) *KeyTestScreen {
	s := &KeyTestScreen{client: client, cfg: cfg, prev: prev, state: keyTestRunning}
	go func() {
		username, owned, err := client.ValidateAPIKey(cfg.APIKey)
		if err != nil {
			s.err = err
			s.state = keyTestFail
		} else {
			s.username = username
			s.ownedCount = len(owned)
			s.state = keyTestOK
		}
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *KeyTestScreen) NeedsRedraw() bool { return false }

func (s *KeyTestScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	footerH := int32(52)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")

	headerH := int32(72)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Test API Key", 20, textY, mt[0], mt[1], mt[2])

	contentTop := headerH + 6
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	ht := r.Theme.HintText
	switch s.state {
	case keyTestRunning:
		r.DrawTextCentered("Testing API key...", 0, mid-fontH/2, r.W, mt[0], mt[1], mt[2])

	case keyTestOK:
		r.DrawTextCentered("API key valid", 0, mid-fontH-smallFH*2-12, r.W, 80, 200, 80)
		r.DrawSmallTextCentered(fmt.Sprintf("Authenticated as: %s", s.username), 0, mid-smallFH-4, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered(fmt.Sprintf("%d owned game(s) found", s.ownedCount), 0, mid+smallFH+4, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered("(full list written to debug log)", 0, mid+smallFH*2+8, r.W, 100, 100, 100)

	case keyTestFail:
		r.DrawTextCentered("API key invalid", 0, mid-fontH-smallFH-8, r.W, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid, r.W-40, smallFH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	if s.state == keyTestRunning {
		r.DrawSmallText("Please wait...", 10, ftrY, ht[0], ht[1], ht[2])
	} else {
		r.DrawFooterHints([]renderer.FooterHint{
			{Kind: renderer.BadgePill, Label: "A/B", Text: "Back"},
		}, ftrY)
	}
	r.Present()
}

func (s *KeyTestScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.UserEvent:
		_ = ev
		// test finished — stay on screen so the result is visible
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN && s.state != keyTestRunning {
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN && s.state != keyTestRunning {
			return s.prev
		}
	}
	return s
}
