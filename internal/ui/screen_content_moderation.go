//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type contentModItem int

const (
	cmItemAdultContent contentModItem = iota
	cmItemQueerContent
	cmItemHeavyThemes
	cmItemSubstanceUse
	cmItemCount
)

// ContentModerationScreen lists the four content filter categories.
// Per-tag categories (Adult Content, Queer Content, Heavy Themes) open a
// TagFilterScreen sub-screen. Substance Use is a single toggle.
type ContentModerationScreen struct {
	cfg     *settings.Config
	cfgPath string
	cursor  contentModItem
	prev    Screen

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
}

func NewContentModerationScreen(cfg *settings.Config, cfgPath string, prev Screen) *ContentModerationScreen {
	return &ContentModerationScreen{cfg: cfg, cfgPath: cfgPath, prev: prev}
}

func (s *ContentModerationScreen) processAutoRepeat() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
		return
	}
	if s.heldDir > 0 && int(s.cursor) < int(cmItemCount)-1 {
		s.cursor++
	} else if s.heldDir < 0 && s.cursor > 0 {
		s.cursor--
	}
	s.lastRepeat = now
}

func (s *ContentModerationScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
func (s *ContentModerationScreen) HasPendingAnimation() bool { return false }

func (s *ContentModerationScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(72)
	footerH := int32(52)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Content Moderation", 20, textY, mt[0], mt[1], mt[2])

	f := s.cfg.Filter

	adultLabel := "Adult Content: Allowed >"
	if f.AdultContent.HasActiveTag(itchio.AdultContentTags) {
		adultLabel = "Adult Content: Filtered >"
	}

	queerLabel := "Queer Content: Allowed >"
	if f.QueerContent.HasActiveTag(itchio.QueerContentTags) {
		queerLabel = "Queer Content: Filtered >"
	}

	heavyLabel := "Heavy Themes: Allowed >"
	if f.HeavyThemes.HasActiveTag(itchio.HeavyThemesTags) {
		heavyLabel = "Heavy Themes: Filtered >"
	}

	substanceLabel := "Substance Use: Allowed"
	if f.SubstanceUse.Enabled {
		substanceLabel = "Substance Use: Blocked"
	}

	items := []string{
		adultLabel,
		queerLabel,
		heavyLabel,
		substanceLabel,
	}

	ac := r.Theme.Accent
	lt := r.Theme.ListText
	at := r.Theme.AccentText
	_, fontH := r.TextSize("Ag")
	rowH := fontH + 14
	for i, label := range items {
		y := headerH + 10 + int32(i)*rowH
		if contentModItem(i) == s.cursor {
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			r.DrawText(label, 20, y, at[0], at[1], at[2])
		} else {
			r.DrawText(label, 20, y, lt[0], lt[1], lt[2])
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *ContentModerationScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
	if dir > 0 && int(s.cursor) < int(cmItemCount)-1 {
		s.cursor++
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
	}
}

func (s *ContentModerationScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *ContentModerationScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.K_UP:
			if ev.Type == sdl.KEYDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(1)
			} else {
				s.stopHold(1)
			}
			return s
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(-1)
			} else {
				s.stopHold(-1)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case btnA:
			return s.activate()
		case btnB:
			return s.prev
		}
	}
	return s
}

func (s *ContentModerationScreen) activate() Screen {
	switch s.cursor {
	case cmItemAdultContent:
		return NewAdultContentFilterScreen(s.cfg, s.cfgPath, s)
	case cmItemQueerContent:
		return NewQueerContentFilterScreen(s.cfg, s.cfgPath, s)
	case cmItemHeavyThemes:
		return NewHeavyThemesFilterScreen(s.cfg, s.cfgPath, s)
	case cmItemSubstanceUse:
		s.cfg.Filter.SubstanceUse.Enabled = !s.cfg.Filter.SubstanceUse.Enabled
		s.cfg.Save(s.cfgPath)
	}
	return s
}
