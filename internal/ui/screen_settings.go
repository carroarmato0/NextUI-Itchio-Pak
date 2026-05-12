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
	sItemMusicDownload
	sItemMusicLocation
	sItemUnifiedNaming
	sItemNextUITheme
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory
	sItemContentModeration
	sItemAbout
	sItemCount
)

const (
	apiKeySetupURL  = "https://github.com/carroarmato0/NextUI-Itchio-Pak#adding-the-key-to-the-pak"
	apiKeySetupBody = "Paid games require an Itch.io API key. Scan the QR code below for instructions on how to add it."
)

type SettingsScreen struct {
	client         *itchio.Client
	cfg            *settings.Config
	cfgPath        string
	cache          *renderer.ImageCache
	cursor         settingsItem
	prev           Screen
	onRefreshGames func(Screen) Screen // nil if not available
	updateSvc      UpdateServicer

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	onThemeToggle  func(bool)
	onOwnedReady   func([]itchio.OwnedGame)

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	showAPIKeyHelp bool
	apiKeyHelpQR   *sdl.Texture
}

func NewSettingsScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	cache *renderer.ImageCache,
	prev Screen,
	onRefreshGames func(Screen) Screen,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	onThemeToggle func(bool),
	onOwnedReady func([]itchio.OwnedGame),
) *SettingsScreen {
	s := &SettingsScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		cache:          cache,
		prev:           prev,
		onRefreshGames: onRefreshGames,
		updateSvc:      updateSvc,
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		onThemeToggle:  onThemeToggle,
		onOwnedReady:   onOwnedReady,
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
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay {
		return
	}
	if now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
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
	// Skip Music Location if music download is disabled.
	if s.cursor == sItemMusicLocation && s.cfg.MusicDownload == "off" {
		if dir >= 0 {
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			} else {
				s.cursor--
			}
		} else {
			if s.cursor > 0 {
				s.cursor--
			} else {
				s.cursor++
			}
		}
	}
}

func (s *SettingsScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
func (s *SettingsScreen) HasPendingAnimation() bool { return false }

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
	items = append(items, menuItem{sItemMusicDownload, "Music Download: " + musicDownloadLabel(s.cfg.MusicDownload)})
	if s.cfg.MusicDownload != "off" {
		items = append(items, menuItem{sItemMusicLocation, "Music Location: " + s.cfg.MusicLocation})
	}
	unifiedNamingVal := "OFF"
	if s.cfg.UnifiedNaming {
		unifiedNamingVal = "ON"
	}
	items = append(items, menuItem{sItemUnifiedNaming, "Use game title as filename: " + unifiedNamingVal})
	if s.themeAvailable {
		items = append(items, menuItem{sItemNextUITheme, "NextUI Theme: " + nextUIThemeLabel})
	}
	items = append(items, menuItem{sItemLogLevel, "Log Level: " + logLevelLabel})
	items = append(items, menuItem{sItemClearCache, "Clear Image Cache"})
	items = append(items, menuItem{sItemRefreshCache, "Refresh Game List"})
	items = append(items, menuItem{sItemUpdateInventory, "Update Inventory"})
	items = append(items, menuItem{sItemContentModeration, "Content Moderation >"})
	items = append(items, menuItem{sItemAbout, "About"})

	// Find where the cursor sits in the rendered items slice (theme row may be absent).
	cursorIdx := 0
	for j, item := range items {
		if item.id == s.cursor {
			cursorIdx = j
			break
		}
	}

	visibleRows := int((r.H - headerH - 10 - footerH) / rowH)
	if visibleRows < 1 {
		visibleRows = 1
	}
	scrollOffset := 0
	if cursorIdx >= visibleRows {
		scrollOffset = cursorIdx - visibleRows + 1
	}

	for i, item := range items {
		if i < scrollOffset {
			continue
		}
		y := headerH + 10 + int32(i-scrollOffset)*rowH
		if y >= r.H-footerH {
			break
		}
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
				const sp = int32(8) // padding to match tag list
				pillW := sw + sp*2
				pillH := sh + 4
				pillX := 20 + labelW + 12
				pillY := y - 4 + (rowH-pillH)/2
				r.DrawPill(pillX, pillY, pillW, pillH, sR, sG, sB)
				r.DrawSmallTextCenteredInRect(statusLabel, pillX, pillY, pillW, pillH, 20, 20, 20)
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
	if s.cursor == sItemAPIKey {
		if s.cfg.APIKey != "" {
			hints[1].Text = "Test API key"
		} else {
			hints[1].Text = "Setup guide"
		}
	}
	r.DrawFooterHints(hints, ftrY)
	if s.showAPIKeyHelp {
		s.drawAPIKeyHelpOverlay(r)
	}
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
	if s.showAPIKeyHelp {
		dismiss := false
		switch ev := e.(type) {
		case *sdl.KeyboardEvent:
			dismiss = ev.Type == sdl.KEYDOWN
		case *sdl.ControllerButtonEvent:
			dismiss = ev.Type == sdl.CONTROLLERBUTTONDOWN
		}
		if dismiss {
			s.showAPIKeyHelp = false
			if s.apiKeyHelpQR != nil {
				s.apiKeyHelpQR.Destroy()
				s.apiKeyHelpQR = nil
			}
			logger.Debug("settings: API key help overlay dismissed")
		}
		return s
	}
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
				return NewKeyTestScreen(s.client, s.cfg, s, s.onOwnedReady)
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
				return NewKeyTestScreen(s.client, s.cfg, s, s.onOwnedReady)
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

func musicDownloadLabel(v string) string {
	switch v {
	case "auto":
		return "auto"
	case "ask":
		return "ask"
	default:
		return "off"
	}
}

func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case sItemAPIKey:
		if s.cfg.APIKey == "" {
			s.showAPIKeyHelp = true
			logger.Info("settings: API key help overlay shown")
		}
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
	case sItemMusicDownload:
		switch s.cfg.MusicDownload {
		case "off":
			s.cfg.MusicDownload = "auto"
		case "auto":
			s.cfg.MusicDownload = "ask"
		default:
			s.cfg.MusicDownload = "off"
		}
		s.cfg.Save(s.cfgPath)
		logger.Info("settings: music download changed to %s", s.cfg.MusicDownload)
	case sItemMusicLocation:
		if s.cfg.MusicLocation == "auto" {
			s.cfg.MusicLocation = "ask"
		} else {
			s.cfg.MusicLocation = "auto"
		}
		s.cfg.Save(s.cfgPath)
		logger.Info("settings: music location changed to %s", s.cfg.MusicLocation)
	case sItemUnifiedNaming:
		s.cfg.UnifiedNaming = !s.cfg.UnifiedNaming
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("settings: save failed: %v", err)
		}
		logger.Info("settings: unified naming changed to %v", s.cfg.UnifiedNaming)
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
		if s.cache != nil {
			s.cache.Clear()
		}
		logger.Info("settings: image cache cleared")
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

func (s *SettingsScreen) drawAPIKeyHelpOverlay(r *renderer.Renderer) {
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	lineH := fontH + 4

	pad := int32(20)
	panelW := r.W * 6 / 10
	bodyMaxW := panelW - pad*2

	// Pre-measure body text to correctly size the panel.
	bodyLines := r.WrapText(apiKeySetupBody, bodyMaxW)
	bodyH := int32(len(bodyLines)) * lineH

	// QR size: 40% of panel width, clamped to [80, 160].
	qrSize := panelW * 4 / 10
	if qrSize > 160 {
		qrSize = 160
	}
	if qrSize < 80 {
		qrSize = 80
	}

	panelH := pad + fontH + 8 + bodyH + 12 + qrSize + 6 + smallFH + pad
	panelX := (r.W - panelW) / 2
	panelY := (r.H - panelH) / 2

	// 2px accent-coloured border drawn as a slightly larger rect behind the panel.
	ac := r.Theme.Accent
	r.DrawRect(panelX-2, panelY-2, panelW+4, panelH+4, ac[0], ac[1], ac[2])

	// Solid panel background.
	bg := r.Theme.Background
	r.DrawRect(panelX, panelY, panelW, panelH, bg[0], bg[1], bg[2])

	// Title.
	mt := r.Theme.MainText
	y := panelY + pad
	r.DrawTextCentered("API Key Setup", panelX, y, panelW, mt[0], mt[1], mt[2])
	y += fontH + 8

	// Body text.
	ht := r.Theme.HintText
	r.DrawWrappedText(apiKeySetupBody, panelX+pad, y, bodyMaxW, lineH, ht[0], ht[1], ht[2])
	y += bodyH + 12

	// QR code — lazily generated and cached for the lifetime of the overlay.
	if s.apiKeyHelpQR == nil {
		tex, err := r.QRTexture(apiKeySetupURL, int(qrSize))
		if err != nil {
			logger.Warn("settings: API key help QR generation failed: %v", err)
		} else {
			s.apiKeyHelpQR = tex
			logger.Debug("settings: API key help QR texture generated")
		}
	}
	if s.apiKeyHelpQR != nil {
		qrX := panelX + (panelW-qrSize)/2
		r.DrawTextureAt(s.apiKeyHelpQR, qrX, y, qrSize, qrSize)
	}
	y += qrSize + 6

	// Caption.
	r.DrawSmallTextCentered("Scan to open setup guide", panelX, y, panelW, 120, 120, 120)
}
