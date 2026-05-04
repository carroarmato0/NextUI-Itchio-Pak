//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const tagPageJump = 5 // rows to skip with L1/R1

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
	cursor     int
	scrollY    int32
	tagAreaTop int32 // set during Draw; used in HandleEvent
	tagAreaH   int32 // set during Draw; used for scroll clamping in HandleEvent
	rowH       int32 // set during Draw; used in ensureCursorVisible
	prev       Screen

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
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
	if s.tagAreaH == 0 || s.rowH == 0 {
		return
	}
	maxScroll := int32(len(s.tags))*s.rowH - s.tagAreaH
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
	if s.tagAreaH == 0 || s.rowH == 0 || s.cursor == 0 {
		return
	}
	tagIdx := s.cursor - 1
	rowTop := int32(tagIdx) * s.rowH
	rowBottom := rowTop + s.rowH
	if rowTop < s.scrollY {
		s.scrollY = rowTop
	}
	if rowBottom > s.scrollY+s.tagAreaH {
		s.scrollY = rowBottom - s.tagAreaH
	}
	s.clampScroll()
}

func (s *TagFilterScreen) processAutoRepeat() {
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
	if s.heldDir > 0 && s.cursor < s.rowCount()-1 {
		s.cursor++
		s.ensureCursorVisible()
	} else if s.heldDir < 0 && s.cursor > 0 {
		s.cursor--
		s.ensureCursorVisible()
	}
	s.lastRepeat = now
}

func (s *TagFilterScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
	if dir > 0 && s.cursor < s.rowCount()-1 {
		s.cursor++
		s.ensureCursorVisible()
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
		s.ensureCursorVisible()
	}
}

func (s *TagFilterScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *TagFilterScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
func (s *TagFilterScreen) HasPendingAnimation() bool { return false }

func (s *TagFilterScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(72)
	footerH := int32(52)

	// ── Header bar ────────────────────────────────────────────────────────────
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText(s.title, 20, textY, mt[0], mt[1], mt[2])

	// Warning note (small font) just below header
	_, smallFH := r.SmallTextSize("Ag")
	noteY := headerH + 8
	r.DrawSmallText("Note: coverage depends on creators' tagging.", 20, noteY, 110, 110, 110)

	// Master toggle row
	ac := r.Theme.Accent
	lt := r.Theme.ListText
	at := r.Theme.AccentText
	_, fontH := r.TextSize("Ag")
	rowH := fontH + 14
	s.rowH = rowH
	masterY := noteY + smallFH + 10
	if s.cursor == 0 {
		r.DrawPill(4, masterY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
	}
	allLabel := "All: Allowed"
	if s.getEnabled() && s.anyTagEnabled() {
		allLabel = "All: Filtered"
	}
	if s.cursor == 0 {
		r.DrawText(allLabel, 20, masterY, at[0], at[1], at[2])
	} else {
		r.DrawText(allLabel, 20, masterY, lt[0], lt[1], lt[2])
	}

	// ── Scrollable tag list ───────────────────────────────────────────────────
	tagAreaTop := masterY + rowH + 6
	s.tagAreaTop = tagAreaTop
	s.tagAreaH = r.H - tagAreaTop - footerH
	s.clampScroll()

	r.SetClipRect(0, tagAreaTop, r.W, s.tagAreaH)
	for i, tag := range s.tags {
		rowY := tagAreaTop + int32(i)*rowH - s.scrollY
		selected := s.cursor == i+1
		if selected {
			r.DrawPill(4, rowY-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		state := "Allowed"
		if s.getEnabled() && s.isTagEnabled(tag) {
			state = "Blocked"
		}
		if selected {
			r.DrawText("  "+tag+": "+state, 20, rowY, at[0], at[1], at[2])
		} else {
			r.DrawText("  "+tag+": "+state, 20, rowY, lt[0], lt[1], lt[2])
		}
	}
	r.ClearClipRect()

	// ── Footer ────────────────────────────────────────────────────────────────
	ftrY := r.DrawFooterBar(footerH)
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgePill, Label: "L/R", Text: "Skip"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Toggle"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Back"},
	}, ftrY)
	r.Present()
}

func (s *TagFilterScreen) HandleEvent(e sdl.Event) Screen {
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
