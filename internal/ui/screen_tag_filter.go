//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const tagPageJump = 5  // rows to skip with L1/R1
const tagAreaTop = 120 // y where the scrollable tag list begins

// TagFilterScreen is a generic per-tag toggle screen used for content filter
// categories that support individual tag opt-out.
//
// Fixed header: title, caveat note, master "All" toggle.
// Scrollable body: individual tag rows, clipped to the area below the header.
//
// Controller mapping (TrimUI): B = toggle, A = back, L1/R1 = skip 5 rows.
type TagFilterScreen struct {
	title       string
	tags        []string
	cfg         *settings.Config
	cfgPath     string
	getEnabled  func() bool
	setEnabled  func(bool)
	getDisabled func() []string
	setDisabled func([]string)
	cursor   int
	scrollY  int32
	tagAreaH int32 // set during Draw; used for scroll clamping in HandleEvent
	prev     Screen
}

// NewAdultContentFilterScreen returns a TagFilterScreen for Adult Content.
func NewAdultContentFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "Adult Content",
		tags:        itchio.AdultContentTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.AdultContent.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.AdultContent.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.AdultContent.Disabled },
		setDisabled: func(v []string) { cfg.Filter.AdultContent.Disabled = v },
		prev:        prev,
	}
}

// NewQueerContentFilterScreen returns a TagFilterScreen for Queer Content.
func NewQueerContentFilterScreen(cfg *settings.Config, cfgPath string, prev Screen) *TagFilterScreen {
	return &TagFilterScreen{
		title:       "Queer Content",
		tags:        itchio.QueerContentTags,
		cfg:         cfg,
		cfgPath:     cfgPath,
		getEnabled:  func() bool { return cfg.Filter.QueerContent.Enabled },
		setEnabled:  func(v bool) { cfg.Filter.QueerContent.Enabled = v },
		getDisabled: func() []string { return cfg.Filter.QueerContent.Disabled },
		setDisabled: func(v []string) { cfg.Filter.QueerContent.Disabled = v },
		prev:        prev,
	}
}

// NewHeavyThemesFilterScreen returns a TagFilterScreen for Heavy Themes.
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
	return 1 + len(s.tags) // master row + individual tags
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

func (s *TagFilterScreen) clampScroll() {
	if s.tagAreaH == 0 {
		return
	}
	maxScroll := int32(len(s.tags))*40 - s.tagAreaH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if s.scrollY < 0 {
		s.scrollY = 0
	}
	if s.scrollY > maxScroll {
		s.scrollY = maxScroll
	}
}

// ensureCursorVisible scrolls so that the currently selected tag row is visible.
// The master toggle (cursor==0) lives in the fixed header and is always visible.
func (s *TagFilterScreen) ensureCursorVisible() {
	if s.tagAreaH == 0 || s.cursor == 0 {
		return
	}
	tagIdx := s.cursor - 1
	rowTop := int32(tagIdx) * 40
	rowBottom := rowTop + 40
	if rowTop < s.scrollY {
		s.scrollY = rowTop
	}
	if rowBottom > s.scrollY+s.tagAreaH {
		s.scrollY = rowBottom - s.tagAreaH
	}
	s.clampScroll()
}

func (s *TagFilterScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	s.tagAreaH = r.H - int32(tagAreaTop) - 28
	s.clampScroll()

	// ── Fixed header ──────────────────────────────────────────────────────────
	r.DrawText(s.title, 20, 20, colorText, colorText, colorText)
	r.DrawText("Note: coverage depends on creators' tagging.", 20, 44, 110, 110, 110)

	// Master toggle (always visible — not part of scrollable area)
	masterY := int32(80)
	if s.cursor == 0 {
		r.DrawRect(0, masterY-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
	}
	allLabel := "All: Allowed"
	if s.getEnabled() && s.anyTagEnabled() {
		allLabel = "All: Filtered"
	}
	r.DrawText(allLabel, 20, masterY, colorText, colorText, colorText)

	// ── Scrollable tag list ───────────────────────────────────────────────────
	r.SetClipRect(0, int32(tagAreaTop), r.W, s.tagAreaH)
	for i, tag := range s.tags {
		rowY := int32(tagAreaTop) + int32(i)*40 - s.scrollY
		if s.cursor == i+1 {
			r.DrawRect(0, rowY-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		state := "Allowed"
		if s.getEnabled() && s.isTagEnabled(tag) {
			state = "Blocked"
		}
		r.DrawText("  "+tag+": "+state, 20, rowY, colorText, colorText, colorText)
	}
	r.ClearClipRect()

	// ── Footer ────────────────────────────────────────────────────────────────
	r.DrawText("L/R skip · B toggle · A back", 10, r.H-24, 140, 140, 140)
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
				s.ensureCursorVisible()
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
				s.ensureCursorVisible()
			}
		case sdl.K_RIGHT:
			s.cursor += tagPageJump
			if s.cursor >= s.rowCount() {
				s.cursor = s.rowCount() - 1
			}
			s.ensureCursorVisible()
		case sdl.K_LEFT:
			s.cursor -= tagPageJump
			if s.cursor < 0 {
				s.cursor = 0
			}
			s.ensureCursorVisible()
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
				s.ensureCursorVisible()
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
				s.ensureCursorVisible()
			}
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			s.cursor -= tagPageJump
			if s.cursor < 0 {
				s.cursor = 0
			}
			s.ensureCursorVisible()
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			s.cursor += tagPageJump
			if s.cursor >= s.rowCount() {
				s.cursor = s.rowCount() - 1
			}
			s.ensureCursorVisible()
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
