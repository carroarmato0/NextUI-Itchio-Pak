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

	state        keyTestState
	username     string
	ownedCount   int
	err          error
	onOwnedReady func([]itchio.OwnedGame)
}

func NewKeyTestScreen(client *itchio.Client, cfg *settings.Config, prev Screen, onOwnedReady func([]itchio.OwnedGame)) *KeyTestScreen {
	s := &KeyTestScreen{client: client, cfg: cfg, prev: prev, state: keyTestRunning, onOwnedReady: onOwnedReady}
	go func() {
		username, owned, err := client.ValidateAPIKey(cfg.APIKey)
		if err != nil {
			s.err = err
			s.state = keyTestFail
			client.StoreAPIKeyStatus(itchio.APIKeyStatusRejected)
		} else {
			s.username = username
			s.ownedCount = len(owned)
			s.state = keyTestOK
			client.StoreAPIKeyStatus(itchio.APIKeyStatusWorking)
			if s.onOwnedReady != nil {
				s.onOwnedReady(owned)
			}
		}
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *KeyTestScreen) NeedsRedraw() bool         { return s.state == keyTestRunning }
func (s *KeyTestScreen) HasPendingAnimation() bool { return false }

func (s *KeyTestScreen) Draw(r *renderer.Renderer) {
	bad := r.Theme.Error()
	badTx := r.Theme.ErrorText()
	mu := r.Theme.Muted()
	ok := r.Theme.Success()
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
		r.DrawTextCentered("Testing API key", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)

	case keyTestOK:
		r.DrawTextCentered("API key valid", 0, mid-fontH-smallFH*2-12, r.W, ok[0], ok[1], ok[2])
		r.DrawSmallTextCentered(fmt.Sprintf("Authenticated as: %s", s.username), 0, mid-smallFH-4, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered(fmt.Sprintf("%d owned game(s) found", s.ownedCount), 0, mid+smallFH+4, r.W, ht[0], ht[1], ht[2])
		r.DrawSmallTextCentered("(full list written to debug log)", 0, mid+smallFH*2+8, r.W, mu[0], mu[1], mu[2])

	case keyTestFail:
		errLines := r.WrapText(s.err.Error(), r.W-40)
		errH := int32(len(errLines)) * (smallFH + 4)
		totalH := fontH + 10 + errH
		startY := mid - totalH/2
		r.DrawTextCentered("API key invalid", 0, startY, r.W, bad[0], bad[1], bad[2])
		r.DrawWrappedText(s.err.Error(), 20, startY+fontH+10, r.W-40, smallFH+4, badTx[0], badTx[1], badTx[2])
	}

	ftrY := r.DrawFooterBar(footerH)
	if s.state != keyTestRunning {
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
