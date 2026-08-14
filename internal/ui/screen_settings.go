//go:build !headless

package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
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
	sItemROMLocation
	sItemPico8Core // ← new
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

type SettingsScreen struct {
	client         *itchio.Client
	cfg            *settings.Config
	cfgPath        string
	inv            *inventory.Inventory
	invPath        string
	cache          *renderer.ImageCache
	cursor         settingsItem
	prev           Screen
	onRefreshGames func(Screen) Screen // nil if not available
	updateSvc      UpdateServicer

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	paletteName    string // active NextUI palette, "" == Custom
	onThemeToggle  func(bool)
	onOwnedReady   func([]itchio.OwnedGame)

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	// pendingKeyTest is set by the keyboard callback to trigger a KeyTestScreen
	// transition on the next event cycle (after the keyboard closes).
	pendingKeyTest bool
}

func NewSettingsScreen(
	client *itchio.Client,
	cfg *settings.Config,
	cfgPath string,
	inv *inventory.Inventory,
	invPath string,
	cache *renderer.ImageCache,
	prev Screen,
	onRefreshGames func(Screen) Screen,
	updateSvc UpdateServicer,
	nextUITheme theme.Theme,
	defaultTheme theme.Theme,
	themeAvailable bool,
	paletteName string,
	onThemeToggle func(bool),
	onOwnedReady func([]itchio.OwnedGame),
) *SettingsScreen {
	s := &SettingsScreen{
		client:         client,
		cfg:            cfg,
		cfgPath:        cfgPath,
		inv:            inv,
		invPath:        invPath,
		cache:          cache,
		prev:           prev,
		onRefreshGames: onRefreshGames,
		updateSvc:      updateSvc,
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		paletteName:    paletteName,
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
	// Step past rows that are not rendered, reversing at either end rather than
	// coming to rest on one. Looping (instead of one check per row) matters now
	// that whole rows can be hidden by firmware: two hidden rows can be
	// adjacent, and a single pass would leave the cursor on the second.
	for s.rowHidden(s.cursor) {
		if dir >= 0 {
			if int(s.cursor) < int(sItemCount)-1 {
				s.cursor++
			} else {
				dir = -1
				s.cursor--
			}
		} else {
			if s.cursor > 0 {
				s.cursor--
			} else {
				dir = 1
				s.cursor++
			}
		}
	}
}

// rowHidden reports whether a settings row is currently absent from the list.
// It must agree with the rows Draw actually appends.
func (s *SettingsScreen) rowHidden(item settingsItem) bool {
	switch item {
	case sItemPico8Core:
		return !firmware.Active().Caps().Pico8CoreChoice
	case sItemNextUITheme:
		return !s.themeAvailable
	case sItemMusicLocation:
		return s.cfg.MusicDownload == "off"
	default:
		return false
	}
}

func (s *SettingsScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
func (s *SettingsScreen) HasPendingAnimation() bool { return false }

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	mu := r.Theme.Muted()
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

	// Naming the active palette turns "On" into something checkable: if this
	// disagrees with NextUI's own Settings, the two are reading different files.
	nextUIThemeLabel := "Off"
	if s.cfg.NextUITheme {
		nextUIThemeLabel = "On (" + theme.PaletteLabel(s.paletteName) + ")"
	}

	type menuItem struct {
		id    settingsItem
		label string
	}
	var items []menuItem
	items = append(items, menuItem{sItemAPIKey, "API Key: "})
	items = append(items, menuItem{sItemROMLocation, "ROM Location: " + s.cfg.ROMLocation})
	// Only offered where the firmware keeps a separate folder per Pico-8
	// runtime. muOS has one Pico-8 folder and runs the official binary, so
	// there is nothing to choose and nothing to migrate between.
	if firmware.Active().Caps().Pico8CoreChoice {
		pico8CoreLabel := "FakeO8 (default)"
		if s.cfg.Pico8Core == "pico8" {
			pico8CoreLabel = "Pico-8 (official)"
		}
		items = append(items, menuItem{sItemPico8Core, "Pico-8 Core: " + pico8CoreLabel})
	}
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
	if s.onRefreshGames != nil {
		items = append(items, menuItem{sItemRefreshCache, "Refresh Game List"})
	}
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
					ok := r.Theme.Success()
					statusLabel, sR, sG, sB = "WORKING", ok[0], ok[1], ok[2]
				case itchio.APIKeyStatusRejected:
					bad := r.Theme.Error()
					statusLabel, sR, sG, sB = "REJECTED", bad[0], bad[1], bad[2]
				default:
					mu2 := r.Theme.Muted()
					statusLabel, sR, sG, sB = "PRESENT", mu2[0], mu2[1], mu2[2]
				}
				sw, sh := r.SmallTextSize(statusLabel)
				const sp = int32(8) // padding to match tag list
				pillW := sw + sp*2
				pillH := sh + 4
				pillX := 20 + labelW + 12
				pillY := y - 4 + (rowH-pillH)/2
				r.DrawPill(pillX, pillY, pillW, pillH, sR, sG, sB)
				stC := r.Theme.ContrastText([3]uint8{sR, sG, sB})
				r.DrawSmallTextCenteredInRect(statusLabel, pillX, pillY, pillW, pillH, stC[0], stC[1], stC[2])
			} else {
				// Muted is derived from the background, so on the selected row —
				// which is filled with an Accent pill — it was unreadable: #867D8C
				// on #D6559E is a contrast of 2 on Plum Magenta. De-emphasise
				// relative to whatever this text actually sits on.
				notSet := mu
				if isSelected {
					notSet = theme.Mix(r.Theme.Accent, r.Theme.AccentText, 65)
				}
				r.DrawText("(not set)", 20+labelW, y, notSet[0], notSet[1], notSet[2])
			}
		}

		// Update Inventory row: right-aligned timestamp/running annotation.
		if item.id == sItemUpdateInventory && s.updateSvc != nil {
			annotation := updateInventoryAnnotation(s.updateSvc)
			aw, _ := r.SmallTextSize(annotation)
			ax := r.W - aw - 20
			var aR, aG, aB uint8
			if s.updateSvc.IsRunning() {
				aR, aG, aB = rgb(r.Theme.Warning())
			} else {
				aR, aG, aB = rgb(r.Theme.Muted())
			}
			_, fh := r.TextSize("Ag")
			_, sh := r.SmallTextSize(annotation)
			r.DrawSmallText(annotation, ax, y+(fh-sh)/2, aR, aG, aB)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}
	if s.cursor == sItemAPIKey {
		if s.cfg.APIKey != "" {
			hints[0].Text = "Test"
			hints = append(hints, renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "Y", Text: "Edit key"})
		} else {
			hints[0].Text = "Enter key"
		}
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

// openKeyboardForAPIKey opens the virtual keyboard for entering or editing the
// API key. seed is pre-filled (use s.cfg.APIKey to edit an existing key, or ""
// to enter a new one). On confirm, the key is saved and a KeyTestScreen is
// shown so the user sees the validation result immediately.
func (s *SettingsScreen) openKeyboardForAPIKey() Screen {
	seed := s.cfg.APIKey
	return NewKeyboardScreen(s, seed, func(value string) {
		if value == "" || value == seed {
			return // no change
		}
		s.cfg.APIKey = value
		go s.cfg.Save(s.cfgPath)
		logger.Info("settings: API key updated via keyboard, len=%d", len(value))
		s.pendingKeyTest = true
	})
}

func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {
	// Keyboard confirmed a new API key — transition to KeyTestScreen now that
	// the keyboard has closed and we are back on the event loop.
	if s.pendingKeyTest {
		s.pendingKeyTest = false
		return NewKeyTestScreen(s.client, s.cfg, s, s.onOwnedReady)
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
		case sdl.K_y: // physical Y — edit API key when one is already set
			if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
				return s.openKeyboardForAPIKey()
			}
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
		case btnA:
			if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
				return NewKeyTestScreen(s.client, s.cfg, s, s.onOwnedReady)
			}
			return s.activate()
		case btnY: // physical Y — edit API key when one is already set
			if s.cursor == sItemAPIKey && s.cfg.APIKey != "" {
				return s.openKeyboardForAPIKey()
			}
		case btnB:
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
			return s.openKeyboardForAPIKey()
		}
	case sItemROMLocation:
		if s.cfg.ROMLocation == "auto" {
			s.cfg.ROMLocation = "ask"
		} else {
			s.cfg.ROMLocation = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case sItemPico8Core:
		oldCore := s.cfg.Pico8Core
		newCore := "fakeo8"
		if oldCore == "fakeo8" {
			newCore = "pico8"
		}
		return NewPico8CoreMigrateScreen(s.cfg, s.cfgPath, s.inv, s.invPath, oldCore, newCore, s)
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
