//go:build !headless

package ui

import (
	"os"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type settingsItem int

const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemContentModeration
	sItemAbout
	sItemCount
)

type SettingsScreen struct {
	client         *itchio.Client
	cfg            *settings.Config
	cfgPath        string
	cursor         settingsItem
	prev           Screen
	onRefreshGames func(Screen) Screen // nil if not available

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
}

func NewSettingsScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, prev Screen, onRefreshGames func(Screen) Screen) *SettingsScreen {
	return &SettingsScreen{client: client, cfg: cfg, cfgPath: cfgPath, prev: prev, onRefreshGames: onRefreshGames}
}

func (s *SettingsScreen) processAutoRepeat() {
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
	if s.heldDir > 0 && int(s.cursor) < int(sItemCount)-1 {
		s.cursor++
	} else if s.heldDir < 0 && s.cursor > 0 {
		s.cursor--
	}
	s.lastRepeat = now
}

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	r.Clear(colorBG, colorBG, colorBG)

	headerH := int32(72)
	footerH := int32(40)
	textY := r.DrawHeaderBar(headerH)
	r.DrawText("Settings", 20, textY, colorText, colorText, colorText)

	_, fontH := r.TextSize("Ag")
	rowH := fontH + 14

	logLevelLabel := "Info"
	if s.cfg.LogLevel == "debug" {
		logLevelLabel = "Debug"
	}

	items := []string{
		"API Key: ",
		"ROM Selection: " + s.cfg.ROMSelection,
		"ROM Location: " + s.cfg.ROMLocation,
		"Log Level: " + logLevelLabel,
		"Clear Image Cache",
		"Refresh Game List",
		"Content Moderation >",
		"About",
	}

	for i, label := range items {
		y := headerH + 10 + int32(i)*rowH
		if settingsItem(i) == s.cursor {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(label, 20, y, colorText, colorText, colorText)

		// API Key row: append status in a distinct colour.
		if settingsItem(i) == sItemAPIKey {
			labelW, _ := r.TextSize(label)
			if s.cfg.APIKey != "" {
				r.DrawText("FOUND", 20+labelW, y, 80, 200, 80)
			} else {
				r.DrawText("(not set)", 20+labelW, y, 120, 120, 120)
			}
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	footerHint := "D-pad navigate · B back · A select"
	if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
		footerHint = "D-pad navigate · B back · A test API key"
	}
	r.DrawSmallText(footerHint, 10, ftrY, 140, 140, 140)
	r.Present()
}

func (s *SettingsScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
	// Move immediately on first press
	if dir > 0 && int(s.cursor) < int(sItemCount)-1 {
		s.cursor++
	} else if dir < 0 && s.cursor > 0 {
		s.cursor--
	}
}

func (s *SettingsScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {
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
			if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
				return NewKeyTestScreen(s.client, s.cfg, s)
			}
			return s.activate()
		case sdl.K_ESCAPE:
			return s.prev
		case sdl.K_s:
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
			if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
				return NewKeyTestScreen(s.client, s.cfg, s)
			}
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
	case sItemROMLocation:
		if s.cfg.ROMLocation == "auto" {
			s.cfg.ROMLocation = "ask"
		} else {
			s.cfg.ROMLocation = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemLogLevel:
		if s.cfg.LogLevel == "debug" {
			s.cfg.LogLevel = ""
		} else {
			s.cfg.LogLevel = "debug"
		}
		s.cfg.Save(s.cfgPath)
		// Apply immediately — no restart required.
		logger.SetLevel(logger.LevelFromString(s.cfg.LogLevel))
	case sItemClearCache:
		os.RemoveAll("/tmp/itchio-pak/cache/")
	case sItemRefreshCache:
		if s.onRefreshGames != nil {
			return s.onRefreshGames(s)
		}
	case sItemContentModeration:
		return NewContentModerationScreen(s.cfg, s.cfgPath, s)
	case sItemAbout:
		return NewAboutScreen(s)
	}
	return s
}
