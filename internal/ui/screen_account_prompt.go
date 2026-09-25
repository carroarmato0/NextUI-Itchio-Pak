//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// AccountPromptScreen asks, once, whether to sign in: on the first run, and
// after an upgrade removed a 1.0.x API key. See docs/mockups/account-prompt.html.
type AccountPromptScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	next    *ListScreen
	legacy  bool
}

// NeedsAccountPrompt reports whether the prompt should be shown at startup.
func NeedsAccountPrompt(cfg *settings.Config) bool {
	return !cfg.SignedIn() && (!cfg.OnboardingSeen || cfg.LegacyKeyRemoved)
}

// NewAccountPromptScreen shows the prompt in front of the game list.
func NewAccountPromptScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, next *ListScreen) *AccountPromptScreen {
	s := &AccountPromptScreen{client: client, cfg: cfg, cfgPath: cfgPath, next: next, legacy: cfg.LegacyKeyRemoved}
	logger.Info("account-prompt: shown (after API key removal: %v)", s.legacy)
	return s
}

func (s *AccountPromptScreen) NeedsRedraw() bool         { return false }
func (s *AccountPromptScreen) HasPendingAnimation() bool { return false }

func (s *AccountPromptScreen) Draw(r *renderer.Renderer) {
	scrim := r.Theme.ModalScrim()
	r.Clear(scrim[0], scrim[1], scrim[2])
	title, body, later := "Do you have an itch.io account?",
		"Sign in with your phone to download the games you own. Free games download without an account.",
		"Not now"
	if s.legacy {
		title, body, later = "Sign in again to download paid games",
			"API keys are no longer used. Sign in once with your phone. Your installed games stay where they are.",
			"Later"
	}
	r.DrawModal(title, body, []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Sign in"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: later},
	})
	r.Present()
}

// answered records that the prompt was dealt with, so it is not shown again.
func (s *AccountPromptScreen) answered(signIn bool) {
	s.cfg.OnboardingSeen = true
	s.cfg.LegacyKeyRemoved = false
	if err := s.cfg.Save(s.cfgPath); err != nil {
		logger.Warn("account-prompt: save failed: %v", err)
	}
	logger.Info("account-prompt: answered, sign in=%v", signIn)
}

func (s *AccountPromptScreen) HandleEvent(e sdl.Event) Screen {
	var a, b bool
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN {
			a, b = ev.Keysym.Sym == sdl.K_RETURN, ev.Keysym.Sym == sdl.K_ESCAPE
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN {
			a, b = ev.Button == btnA, ev.Button == btnB
		}
	}
	switch {
	case a:
		s.answered(true)
		return NewSignInScreen(s.client, s.cfg, s.cfgPath, s.next, s.next.OwnedReady())
	case b:
		s.answered(false)
		return s.next
	}
	return s
}
