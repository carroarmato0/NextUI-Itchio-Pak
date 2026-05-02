//go:build !headless

package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
)

// UpdateServicer is satisfied by *inventory.UpdateService; defined here to avoid
// importing the inventory package from the UI layer.
type UpdateServicer interface {
	TriggerNow()
	IsRunning() bool
	LatestCheckedAt() time.Time
}

type settingsItem int

const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemNextUITheme
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory
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
	updateSvc      UpdateServicer

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
}

func NewSettingsScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	prev Screen,
	onRefreshGames func(Screen) Screen,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
) *SettingsScreen {
	s := &SettingsScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		prev:           prev,
		onRefreshGames: onRefreshGames,
		updateSvc:      updateSvc,
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
	}
	// Start a one-shot background validation the first time Settings is opened
	// this session. MarkAPIKeyCheckStarted is a CAS gate so subsequent opens
	// are a no-op.
	if cfg.APIKey != "" && client.MarkAPIKeyCheckStarted() {
		go func() {
			status := client.CheckAPIKey(cfg.APIKey)
			client.StoreAPIKeyStatus(status)
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
		}()
	}
	return s
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
	s.moveCursor(s.heldDir)
	s.lastRepeat = now
}

func (s *SettingsScreen) moveCursor(dir int) {
	if dir > 0 {
		if int(s.cursor) < int(sItemCount)-1 {
			s.cursor++
		}
	} else if dir < 0 {
		if s.cursor > 0 {
			s.cursor--
		}
	}
	// Skip NextUI Theme if not available.
	if s.cursor == sItemNextUITheme && !s.themeAvailable {
		if dir >= 0 { // moving down or neutral
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			} else {
				s.cursor-- // bounce back if at end
			}
		} else { // moving up
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.cursor++ // bounce back if at start
			}
		}
	}
}

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])

	headerH := int32(72)
	footerH := int32(52)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Settings", 20, textY, mt[0], mt[1], mt[2])

	_, fontH := r.TextSize("Ag")
	rowH := fontH + 14

	logLevelLabel := "Info"
	if s.cfg.LogLevel == "debug" {
		logLevelLabel = "Debug"
	}

	nextUIThemeLabel := "Off"
	if s.cfg.NextUITheme {
		nextUIThemeLabel = "On"
	}

	type menuItem struct {
		id    settingsItem
		label string
	}
	var items []menuItem
	items = append(items, menuItem{sItemAPIKey, "API Key: "})
	items = append(items, menuItem{sItemROMMode, "ROM Selection: " + s.cfg.ROMSelection})
	items = append(items, menuItem{sItemROMLocation, "ROM Location: " + s.cfg.ROMLocation})
	if s.themeAvailable {
		items = append(items, menuItem{sItemNextUITheme, "NextUI Theme: " + nextUIThemeLabel})
	}
	items = append(items, menuItem{sItemLogLevel, "Log Level: " + logLevelLabel})
	items = append(items, menuItem{sItemClearCache, "Clear Image Cache"})
	items = append(items, menuItem{sItemRefreshCache, "Refresh Game List"})
	items = append(items, menuItem{sItemUpdateInventory, "Update Inventory"})
	items = append(items, menuItem{sItemContentModeration, "Content Moderation >"})
	items = append(items, menuItem{sItemAbout, "About"})

	for i, item := range items {
		y := headerH + 10 + int32(i)*rowH
		isSelected := item.id == s.cursor
		if isSelected {
			ac := r.Theme.Accent
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		}
		var tr, tg, tb uint8
		if isSelected {
			c := r.Theme.AccentText
			tr, tg, tb = c[0], c[1], c[2]
		} else {
			c := r.Theme.ListText
			tr, tg, tb = c[0], c[1], c[2]
		}
		r.DrawText(item.label, 20, y, tr, tg, tb)

		// API Key row: append live validation status as a small pill badge.
		if item.id == sItemAPIKey {
			labelW, _ := r.TextSize(item.label)
			if s.cfg.APIKey != "" {
				var statusLabel string
				var sR, sG, sB uint8
				switch s.client.GetAPIKeyStatus() {
				case itchio.APIKeyStatusWorking:
					statusLabel, sR, sG, sB = "WORKING", 80, 200, 80
				case itchio.APIKeyStatusRejected:
					statusLabel, sR, sG, sB = "REJECTED", 200, 60, 60
				default:
					statusLabel, sR, sG, sB = "PRESENT", 140, 140, 140
				}
				sw, sh := r.SmallTextSize(statusLabel)
				const sp = int32(4)
				pillX := 20 + labelW + 4
				pillY := y - 4 + (rowH-sh-sp)/2
				r.DrawPill(pillX, pillY, sw+sp*2, sh+sp, sR, sG, sB)
				r.DrawSmallText(statusLabel, pillX+sp, pillY+sp/2, 20, 20, 20)
			} else {
				r.DrawText("(not set)", 20+labelW, y, 120, 120, 120)
			}
		}

		// Update Inventory row: right-aligned timestamp/running annotation.
		if item.id == sItemUpdateInventory && s.updateSvc != nil {
			annotation := updateInventoryAnnotation(s.updateSvc)
			aw, _ := r.SmallTextSize(annotation)
			ax := r.W - aw - 20
			var aR, aG, aB uint8
			if s.updateSvc.IsRunning() {
				aR, aG, aB = 240, 160, 40
			} else {
				aR, aG, aB = 100, 100, 100
			}
			_, fh := r.TextSize("Ag")
			_, sh := r.SmallTextSize(annotation)
			r.DrawSmallText(annotation, ax, y+(fh-sh)/2, aR, aG, aB)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
	}
	if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
		hints[1].Text = "Test API key"
	}
	r.DrawFooterHints(hints, ftrY)
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
	s.moveCursor(dir)
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
	}
	return s
}

// updateInventoryAnnotation returns a short right-aligned label for the
// "Update Inventory" settings row.
func updateInventoryAnnotation(svc UpdateServicer) string {
	if svc.IsRunning() {
		return "checking…"
	}
	t := svc.LatestCheckedAt()
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "last: just now"
	case d < time.Hour:
		return fmt.Sprintf("last: %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("last: %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("last: %dd ago", int(d.Hours()/24))
	}
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
	case sItemNextUITheme:
		s.cfg.NextUITheme = !s.cfg.NextUITheme
		s.cfg.Save(s.cfgPath)
		logger.Info("settings: NextUI theme changed to %v", s.cfg.NextUITheme)
		if s.onThemeToggle != nil {
			s.onThemeToggle(s.cfg.NextUITheme)
		}
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
	case sItemUpdateInventory:
		if s.updateSvc != nil {
			s.updateSvc.TriggerNow()
			logger.Info("settings: Update Inventory triggered manually")
		}
	case sItemContentModeration:
		return NewContentModerationScreen(s.cfg, s.cfgPath, s)
	case sItemAbout:
		return NewAboutScreen(s)
	}
	return s
}
