//go:build !headless

package ui

import (
	"os"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
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

// Rows in display order, grouped by section (settingsSections). moveCursor
// walks this order, so a new row goes inside its section's range.
const (
	// Account
	sItemAccount settingsItem = iota
	sItemShareDeviceInfo
	// Downloads
	sItemROMLocation
	sItemUnifiedNaming
	sItemPico8Core
	sItemMusicDownload
	sItemMusicLocation
	// Library
	sItemContentModeration
	sItemClearCache
	sItemRefreshCache
	sItemUpdateInventory
	// Appearance
	sItemNextUITheme
	// App
	sItemUpdates
	sItemLogLevel
	sItemAbout
	sItemCount
)

// settingsSection is a heading and the contiguous run of rows under it.
type settingsSection struct {
	title       string
	first, last settingsItem
}

var settingsSections = []settingsSection{
	{"Account", sItemAccount, sItemShareDeviceInfo},
	{"Downloads", sItemROMLocation, sItemMusicLocation},
	{"Library", sItemContentModeration, sItemUpdateInventory},
	{"Appearance", sItemNextUITheme, sItemNextUITheme},
	{"App", sItemUpdates, sItemAbout},
}

// sectionIndex is the section item belongs to.
func sectionIndex(item settingsItem) int {
	for i, sec := range settingsSections {
		if item >= sec.first && item <= sec.last {
			return i
		}
	}
	return 0
}

// settingsRow is one line of the rendered list: a section heading, or a row
// the cursor can rest on.
type settingsRow struct {
	heading string       // non-empty for a heading
	id      settingsItem // a row's item; unused for a heading
}

// rows is the list as drawn: each section with at least one visible row, its
// heading first. It must agree with rowHidden, which moveCursor uses.
func (s *SettingsScreen) rows() []settingsRow {
	var out []settingsRow
	for _, sec := range settingsSections {
		var items []settingsRow
		for id := sec.first; id <= sec.last; id++ {
			if !s.rowHidden(id) {
				items = append(items, settingsRow{id: id})
			}
		}
		if len(items) > 0 {
			out = append(out, settingsRow{heading: sec.title})
			out = append(out, items...)
		}
	}
	return out
}

// jumpSection moves the cursor to the first visible row of the next (dir>0)
// or previous section, wrapping at either end. Sections with no visible row
// are skipped. Bound to L1/R1.
func (s *SettingsScreen) jumpSection(dir int) {
	n := len(settingsSections)
	cur := sectionIndex(s.cursor)
	for step := 1; step <= n; step++ {
		sec := settingsSections[((cur+dir*step)%n+n)%n]
		for id := sec.first; id <= sec.last; id++ {
			if !s.rowHidden(id) {
				s.cursor = id
				logger.Debug("settings: jumped to section %q", sec.title)
				return
			}
		}
	}
}

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
	appUpd         AppUpdater // nil hides the Updates row

	nextUITheme    theme.Theme
	defaultTheme   theme.Theme
	themeAvailable bool
	paletteName    string // active NextUI palette, "" == Custom
	onThemeToggle  func(bool)
	onOwnedReady   func([]itchio.OwnedGame)

	scrollY int32 // list scroll offset in pixels; kept between frames

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
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
		appUpd:         appUpdater(),
		nextUITheme:    nextUITheme,
		defaultTheme:   defaultTheme,
		themeAvailable: themeAvailable,
		paletteName:    paletteName,
		onThemeToggle:  onThemeToggle,
		onOwnedReady:   onOwnedReady,
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
	s.moveCursor(s.heldDir, false)
	s.lastRepeat = now
}

// moveCursor steps one row in dir. With wrap, stepping past either end lands
// on the other, so the last row is one press away from the first; auto-repeat
// passes false so a held button stops at the end instead of spinning round.
func (s *SettingsScreen) moveCursor(dir int, wrap bool) {
	last := sItemCount - 1
	if dir > 0 {
		if s.cursor < last {
			s.cursor++
		} else if wrap {
			s.cursor = 0
			logger.Debug("settings: cursor wrapped to the first row")
		}
	} else if dir < 0 {
		if s.cursor > 0 {
			s.cursor--
		} else if wrap {
			s.cursor = last
			logger.Debug("settings: cursor wrapped to the last row")
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
	case sItemRefreshCache:
		return s.onRefreshGames == nil
	case sItemUpdates:
		return s.appUpd == nil
	default:
		return false
	}
}

func (s *SettingsScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
func (s *SettingsScreen) HasPendingAnimation() bool { return false }

// rowText is a row's name and, right-aligned, its current value. An empty
// value draws nothing on the right (the row is an action, or it has its own
// annotation).
func (s *SettingsScreen) rowText(id settingsItem) (label, value string) {
	onOff := func(b bool) string {
		if b {
			return "On"
		}
		return "Off"
	}
	autoAsk := func(v string) string {
		if v == "ask" {
			return "Ask"
		}
		return "Auto"
	}
	switch id {
	case sItemAccount:
		if s.cfg.SignedIn() {
			return "Account", s.cfg.AuthUser
		}
		return "Account", "not signed in"
	case sItemShareDeviceInfo:
		return "Share device info with itch.io", onOff(s.cfg.ShareDeviceInfo)
	case sItemROMLocation:
		return "ROM location", autoAsk(s.cfg.ROMLocation)
	case sItemUnifiedNaming:
		return "Game title as file name", onOff(s.cfg.UnifiedNaming)
	case sItemPico8Core:
		if s.cfg.Pico8Core == "pico8" {
			return "Pico-8 core", "Pico-8 (official)"
		}
		return "Pico-8 core", "FakeO8 (default)"
	case sItemMusicDownload:
		switch musicDownloadLabel(s.cfg.MusicDownload) {
		case "auto":
			return "Music downloads", "Auto"
		case "ask":
			return "Music downloads", "Ask"
		}
		return "Music downloads", "Off"
	case sItemMusicLocation:
		return "Music location", autoAsk(s.cfg.MusicLocation)
	case sItemContentModeration:
		return "Content moderation", ">"
	case sItemClearCache:
		return "Clear image cache", ""
	case sItemRefreshCache:
		return "Refresh game list", ""
	case sItemUpdateInventory:
		return "Update inventory", ""
	case sItemNextUITheme:
		// Naming the active palette turns "On" into something checkable: if this
		// disagrees with NextUI's own Settings, the two are reading different files.
		if s.cfg.NextUITheme {
			return "NextUI theme", "On (" + theme.PaletteLabel(s.paletteName) + ")"
		}
		return "NextUI theme", "Off"
	case sItemUpdates:
		return "App updates", ">"
	case sItemLogLevel:
		if s.cfg.LogLevel == "debug" {
			return "Log level", "Debug"
		}
		return "Log level", "Info"
	case sItemAbout:
		return "About", ">"
	}
	return "", ""
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
	_, smallH := r.SmallTextSize("Ag")
	rowH := fontH + 14
	headingH := smallH + 14
	if compact(r.W, r.H) {
		headingH = smallH + 8
	}

	// Lay the list out in content coordinates: headings are shorter than rows.
	rows := s.rows()
	tops := make([]int32, len(rows))
	var total, cursorTop, sectionTop int32
	for i, row := range rows {
		tops[i] = total
		if row.heading != "" {
			total += headingH
			continue
		}
		if row.id == s.cursor {
			cursorTop = tops[i]
			sectionTop = cursorTop
			if i > 0 && rows[i-1].heading != "" {
				sectionTop = tops[i-1] // bring a section's heading in with its first row
			}
		}
		total += rowH
	}

	viewTop := headerH + 6
	viewH := r.H - footerH - viewTop
	if sectionTop < s.scrollY {
		s.scrollY = sectionTop
	}
	if cursorTop+rowH > s.scrollY+viewH {
		s.scrollY = cursorTop + rowH - viewH
	}
	s.scrollY = max(0, min(s.scrollY, total-viewH))

	// Rows cut by the edges are clipped rather than skipped, so scrolling moves
	// the list smoothly instead of leaving a gap under the header.
	r.SetClipRect(0, viewTop, r.W, viewH)
	for i, row := range rows {
		top := viewTop + tops[i] - s.scrollY
		h := rowH
		if row.heading != "" {
			h = headingH
		}
		if top+h <= viewTop || top >= viewTop+viewH {
			continue
		}
		if row.heading != "" {
			mu := r.Theme.Muted()
			r.DrawSmallText(strings.ToUpper(row.heading), 20, top+h-smallH-3, mu[0], mu[1], mu[2])
			continue
		}
		s.drawRow(r, row.id, top+4, rowH)
	}
	r.ClearClipRect()

	ftrY := r.DrawFooterBar(footerH)
	hints := []renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"},
		{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"},
	}
	if s.cursor == sItemAccount {
		if s.cfg.SignedIn() {
			hints[0].Text = "Sign in again"
			hints = []renderer.FooterHint{hints[0],
				{Kind: renderer.BadgeCircle, Label: "Y", Text: "Sign out"}, hints[1]}
		} else {
			hints[0].Text = "Sign in"
		}
	}
	shoulder := renderer.FooterHint{Kind: renderer.BadgePill, Label: "L1R1", Text: "Section"}
	if abbreviate(r.W) {
		shoulder.Label = "LR"
	}
	hints = append(hints, shoulder)
	r.DrawFooterHints(hints, ftrY)
	r.Present()
}

// drawRow draws one selectable row whose text line starts at y.
func (s *SettingsScreen) drawRow(r *renderer.Renderer, id settingsItem, y, rowH int32) {
	isSelected := id == s.cursor
	tc := r.Theme.ListText
	if isSelected {
		ac := r.Theme.Accent
		r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
		tc = r.Theme.AccentText
	}
	label, value := s.rowText(id)

	right := r.W - 20
	if value != "" {
		vc := tc
		if id == sItemAccount && !s.cfg.SignedIn() || value == ">" {
			// De-emphasised relative to whatever the row sits on: Muted is
			// derived from the background, so on the Accent pill it vanished.
			vc = r.Theme.Muted()
			if isSelected {
				vc = theme.Mix(r.Theme.Accent, r.Theme.AccentText, 65)
			}
		}
		vw, _ := r.TextSize(value)
		r.DrawText(value, right-vw, y, vc[0], vc[1], vc[2])
		right -= vw + 16
	}
	r.DrawText(truncateToWidth(r, label, right-20), 20, y, tc[0], tc[1], tc[2])

	// Right-aligned status, left of the value (if any).
	switch {
	case id == sItemUpdateInventory && s.updateSvc != nil:
		warn := s.updateSvc.IsRunning() || netstate.Offline()
		drawRowAnnotationAt(r, updateInventoryAnnotation(s.updateSvc), right, y,
			annotationColor(r, isSelected, r.Theme.Warning(), warn))
	case id == sItemUpdates && s.appUpd != nil:
		if a := updatesRowAnnotation(s.appUpd.Verdict()); a != "" {
			drawRowAnnotationAt(r, a, right, y, annotationColor(r, isSelected, r.Theme.SuccessAction(), true))
		}
	}
}

func (s *SettingsScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir = dir
	s.heldSince = time.Now()
	s.lastRepeat = s.heldSince
	// Move immediately on first press; only a fresh press wraps round.
	s.moveCursor(dir, true)
}

func (s *SettingsScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

// signOut forgets the itch.io sign-in. Installed games, their files and the
// owned-games cache are left alone: only downloading needs to be signed in.
func (s *SettingsScreen) signOut() Screen {
	if !s.cfg.SignedIn() {
		return s
	}
	s.cfg.SignOut()
	s.client.SetAuthToken("")
	if err := s.cfg.Save(s.cfgPath); err != nil {
		logger.Warn("settings: save after sign-out failed: %v", err)
	}
	logger.Info("settings: signed out of itch.io")
	return s
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
			return s.activate()
		case sdl.K_y: // physical Y — sign out
			if s.cursor == sItemAccount {
				return s.signOut()
			}
		case sdl.K_PAGEDOWN: // R shoulder
			s.jumpSection(1)
		case sdl.K_PAGEUP: // L shoulder
			s.jumpSection(-1)
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
			return s.activate()
		case btnY: // physical Y — sign out
			if s.cursor == sItemAccount {
				return s.signOut()
			}
		case btnB:
			return s.prev
		case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
			s.jumpSection(1)
		case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
			s.jumpSection(-1)
		case sdl.CONTROLLER_BUTTON_START:
			return s.prev
		}
	}
	return s
}

// updateInventoryAnnotation returns a short right-aligned label for the
// "Update Inventory" settings row.
func updateInventoryAnnotation(svc UpdateServicer) string {
	if netstate.Offline() {
		return "offline"
	}
	if svc.IsRunning() {
		return "checking…"
	}
	return lastCheckedLabel(svc.LatestCheckedAt())
}

// updatesRowAnnotation is "v1.1.0 available", or nothing.
func updatesRowAnnotation(v appupdate.Verdict) string {
	if v.Kind == appupdate.Available && v.Latest != nil {
		return v.Latest.Tag + " available"
	}
	return ""
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
	case sItemAccount:
		// Signed in or not, A starts a fresh QR sign-in.
		return NewSignInScreen(s.client, s.cfg, s.cfgPath, s, s.onOwnedReady)
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
	case sItemShareDeviceInfo:
		s.cfg.ShareDeviceInfo = !s.cfg.ShareDeviceInfo
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("settings: save failed: %v", err)
		}
		// Applies to the next request; the UA itself is logged by itchio.
		itchio.SetShareDeviceInfo(s.cfg.ShareDeviceInfo)
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
	case sItemUpdates:
		return NewUpdatesScreen(s.cfg, s.cfgPath, s.appUpd, s)
	case sItemAbout:
		return NewAboutScreen(s)
	}
	return s
}
