//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// SensitiveTagsScreen lets a parent toggle individual Sensitive Topic tags.
// Row 0 is the master "All" toggle; rows 1..N are individual tags alphabetically.
type SensitiveTagsScreen struct {
	cfg     *settings.Config
	cfgPath string
	cursor  int
	prev    Screen
}

func NewSensitiveTagsScreen(cfg *settings.Config, cfgPath string, prev Screen) *SensitiveTagsScreen {
	return &SensitiveTagsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev}
}

func (s *SensitiveTagsScreen) rowCount() int {
	return 1 + len(itchio.SensitiveTags) // "All" row + one per tag
}

// isTagEnabled reports whether an individual sensitive tag is enabled
// (i.e. not in SensitiveDisabled).
func (s *SensitiveTagsScreen) isTagEnabled(tag string) bool {
	for _, d := range s.cfg.Parental.SensitiveDisabled {
		if d == tag {
			return false
		}
	}
	return true
}

func (s *SensitiveTagsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Sensitive Topics", 20, 20, colorText, colorText, colorText)

	// Row 0 — master toggle
	y := int32(80)
	if s.cursor == 0 {
		r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
	}
	allLabel := "All: OFF"
	if s.cfg.Parental.SensitiveEnabled {
		allLabel = "All: ON"
	}
	r.DrawText(allLabel, 20, y, colorText, colorText, colorText)

	// Individual tag rows
	for i, tag := range itchio.SensitiveTags {
		y = int32(120 + i*40)
		if s.cursor == i+1 {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		state := "OFF"
		if s.cfg.Parental.SensitiveEnabled && s.isTagEnabled(tag) {
			state = "ON"
		}
		r.DrawText("  "+tag+": "+state, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("B toggle · A back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *SensitiveTagsScreen) HandleEvent(e sdl.Event) Screen {
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

// toggle activates the currently selected row.
func (s *SensitiveTagsScreen) toggle() {
	if s.cursor == 0 {
		// Master toggle
		s.cfg.Parental.SensitiveEnabled = !s.cfg.Parental.SensitiveEnabled
		s.cfg.Save(s.cfgPath)
		return
	}
	tag := itchio.SensitiveTags[s.cursor-1]
	if s.isTagEnabled(tag) {
		// Disable: add to SensitiveDisabled
		s.cfg.Parental.SensitiveDisabled = append(s.cfg.Parental.SensitiveDisabled, tag)
	} else {
		// Enable: remove from SensitiveDisabled
		updated := s.cfg.Parental.SensitiveDisabled[:0]
		for _, d := range s.cfg.Parental.SensitiveDisabled {
			if d != tag {
				updated = append(updated, d)
			}
		}
		s.cfg.Parental.SensitiveDisabled = updated
	}
	s.cfg.Save(s.cfgPath)
}
