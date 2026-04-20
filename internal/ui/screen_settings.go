//go:build !headless

package ui

import (
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type settingsItem int

const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemClearCache
	sItemAdultContent
	sItemQueerContent
	sItemHeavyThemes
	sItemSubstanceUse
	sItemAbout
	sItemCount
)

type SettingsScreen struct {
	cfg     *settings.Config
	cfgPath string
	cursor  settingsItem
	prev    Screen
}

func NewSettingsScreen(cfg *settings.Config, cfgPath string, prev Screen) *SettingsScreen {
	return &SettingsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev}
}

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Settings", 20, 20, colorText, colorText, colorText)

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
		"API Key: " + maskKey(s.cfg.APIKey),
		"ROM Selection: " + s.cfg.ROMSelection,
		"Clear Image Cache",
		adultLabel,
		queerLabel,
		heavyLabel,
		substanceLabel,
		"About",
	}

	for i, label := range items {
		y := int32(60 + i*36)
		if settingsItem(i) >= sItemAdultContent {
			y += 22 // shift filter items down past "Content Moderation" header
		}
		if settingsItem(i) == sItemAdultContent {
			// Draw section header between Clear Image Cache and Adult Content
			headerY := int32(60+2*36) + 10 // = 142
			r.DrawText("Content Moderation", 20, headerY, 100, 100, 100)
			r.DrawRect(0, headerY+16, r.W, 1, 50, 50, 50)
		}
		if settingsItem(i) == s.cursor {
			r.DrawRect(0, y-4, r.W, 32, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(label, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("D-pad navigate · B select · A back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			return s.prev
		case sdl.K_s:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.activate()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case sItemROMMode:
		if s.cfg.ROMSelection == "auto" {
			s.cfg.ROMSelection = "ask"
		} else {
			s.cfg.ROMSelection = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemClearCache:
		os.RemoveAll("/tmp/itchio-pak/cache/")
	case sItemAdultContent:
		return NewAdultContentFilterScreen(s.cfg, s.cfgPath, s)
	case sItemQueerContent:
		return NewQueerContentFilterScreen(s.cfg, s.cfgPath, s)
	case sItemHeavyThemes:
		return NewHeavyThemesFilterScreen(s.cfg, s.cfgPath, s)
	case sItemSubstanceUse:
		s.cfg.Filter.SubstanceUse.Enabled = !s.cfg.Filter.SubstanceUse.Enabled
		s.cfg.Save(s.cfgPath)
	}
	return s
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}
