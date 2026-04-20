//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// TagFilterScreen is a generic per-tag toggle screen used for content filter
// categories that support individual tag opt-out (LGBTQ+, Heavy Themes).
//
// Row 0: master "All: Filtered / Allowed" toggle.
// Rows 1..N: one row per tag, showing Blocked or Allowed.
//
// Controller mapping (TrimUI convention): B = toggle, A = back.
type TagFilterScreen struct {
	title       string
	tags        []string
	cfg         *settings.Config
	cfgPath     string
	getEnabled  func() bool
	setEnabled  func(bool)
	getDisabled func() []string
	setDisabled func([]string)
	cursor      int
	prev        Screen
}

// NewLGBTQFilterScreen returns a TagFilterScreen configured for the LGBTQ+
// content category.
func NewLGBTQFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "LGBTQ+ Content",
		tags:        itchio.LGBTQTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.LGBTQ.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.LGBTQ.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.LGBTQ.Disabled },
		setDisabled: func(v []string) { cfg.Filter.LGBTQ.Disabled = v },
		prev:        prev,
	}
}

// NewHeavyThemesFilterScreen returns a TagFilterScreen configured for the
// Heavy Themes content category.
func NewHeavyThemesFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "Heavy Themes",
		tags:        itchio.HeavyThemesTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.HeavyThemes.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.HeavyThemes.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.HeavyThemes.Disabled },
		setDisabled: func(v []string) { cfg.Filter.HeavyThemes.Disabled = v },
		prev:        prev,
	}
}

func (s *TagFilterScreen) rowCount() int {
	return 1 + len(s.tags)
}

func (s *TagFilterScreen) isTagEnabled(tag string) bool {
	for _, d := range s.getDisabled() {
		if d == tag {
			return false
		}
	}
	return true
}

func (s *TagFilterScreen) anyTagEnabled() bool {
	for _, tag := range s.tags {
		if s.isTagEnabled(tag) {
			return true
		}
	}
	return false
}

func (s *TagFilterScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText(s.title, 20, 20, colorText, colorText, colorText)

	// Row 0 — master toggle
	y := int32(80)
	if s.cursor == 0 {
		r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
	}
	allLabel := "All: Allowed"
	if s.getEnabled() && s.anyTagEnabled() {
		allLabel = "All: Filtered"
	}
	r.DrawText(allLabel, 20, y, colorText, colorText, colorText)

	// Individual tag rows
	for i, tag := range s.tags {
		y = int32(120 + i*40)
		if s.cursor == i+1 {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		state := "Allowed"
		if s.getEnabled() && s.isTagEnabled(tag) {
			state = "Blocked"
		}
		r.DrawText("  "+tag+": "+state, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("B toggle · A back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *TagFilterScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < s.rowCount()-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			s.toggle()
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < s.rowCount()-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			s.toggle()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *TagFilterScreen) toggle() {
	if s.cursor == 0 {
		s.setEnabled(!s.getEnabled())
		s.cfg.Save(s.cfgPath)
		return
	}
	tag := s.tags[s.cursor-1]
	if s.isTagEnabled(tag) {
		s.setDisabled(append(s.getDisabled(), tag))
	} else {
		var updated []string
		for _, d := range s.getDisabled() {
			if d != tag {
				updated = append(updated, d)
			}
		}
		s.setDisabled(updated)
	}
	s.cfg.Save(s.cfgPath)
}
