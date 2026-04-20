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
	if now.Sub(s.heldSince) < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < repeatInterval {
		return
	}
	if s.heldDir > 0 && int(s.cursor) < int(cmItemCount)-1 {
		s.cursor++
	} else if s.heldDir < 0 && s.cursor > 0 {
		s.cursor--
	}
	s.lastRepeat = now
}

func (s *ContentModerationScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	r.Clear(colorBG, colorBG, colorBG)

	headerH := int32(72)
	footerH := int32(40)
	textY := r.DrawHeaderBar(headerH)
	r.DrawText("Content Moderation", 20, textY, colorText, colorText, colorText)

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

	_, fontH := r.TextSize("Ag")
	rowH := fontH + 14
	for i, label := range items {
		y := headerH + 10 + int32(i)*rowH
		if contentModItem(i) == s.cursor {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(label, 20, y, colorText, colorText, colorText)
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("D-pad navigate · B select · A back", 10, ftrY, 140, 140, 140)
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
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
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
