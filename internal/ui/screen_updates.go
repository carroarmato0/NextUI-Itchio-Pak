//go:build !headless

package ui

import (
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type updatesRow int

const (
	uRowChannel updatesRow = iota
	uRowCheck
	uRowArchive // muOS and ViaArchive only
	uRowInstall // NextUI and ViaInstall only
)

// UpdatesScreen is Settings → App updates (spec §4).
type UpdatesScreen struct {
	cfg     *settings.Config
	cfgPath string
	up      AppUpdater
	prev    Screen

	cursor     updatesRow
	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	qrTex *sdl.Texture
	qrKey string
}

func NewUpdatesScreen(cfg *settings.Config, cfgPath string, up AppUpdater, prev Screen) *UpdatesScreen {
	logger.Info("updates: opened (channel=%s)", up.Channel())
	return &UpdatesScreen{cfg: cfg, cfgPath: cfgPath, up: up, prev: prev}
}

// IsBusy implements BusyChecker. Returns true while a Save to ARCHIVE
// download is in flight, so the main loop waits for it (or the user cancels)
// before sleep/shutdown, as it does for game downloads.
func (s *UpdatesScreen) IsBusy() bool {
	return s.up.ArchiveStatus().State == appupdate.ArchiveRunning ||
		s.up.InstallStatus().State == appupdate.InstallRunning
}

func (s *UpdatesScreen) archiveRowShown() bool {
	v := s.up.Verdict()
	return v.Kind == appupdate.Available && v.Via == appupdate.ViaArchive &&
		s.up.ArchiveStatus().State != appupdate.ArchiveRunning
}

func (s *UpdatesScreen) installRowShown() bool {
	v, st := s.up.Verdict(), s.up.InstallStatus()
	if st.State == appupdate.InstallStaged {
		return true
	}
	return v.Kind == appupdate.Available && v.Via == appupdate.ViaInstall && st.State != appupdate.InstallRunning
}

// installRowStartsOver reports whether the Install row's action is
// StartInstall rather than RequestRestart: either there is no staged build,
// or a later check found a release strictly newer than the one already
// staged (the checker re-stages in that case). rowLabel and activate both
// call this, so the row's label and what A actually does can never drift
// apart — the bug that let a stale "Restart now" install an old staged
// build while the status line talked about a newer one.
func (s *UpdatesScreen) installRowStartsOver() bool {
	v, st := s.up.Verdict(), s.up.InstallStatus()
	if st.State != appupdate.InstallStaged {
		return true
	}
	if v.Latest == nil {
		return false
	}
	staged, stagedOK := appupdate.Parse(st.Tag)
	latest, latestOK := appupdate.Parse(v.Latest.Tag)
	return stagedOK && latestOK && appupdate.Compare(latest, staged) > 0
}

func (s *UpdatesScreen) visibleRows() []updatesRow {
	rows := []updatesRow{uRowChannel, uRowCheck}
	if s.archiveRowShown() {
		rows = append(rows, uRowArchive)
	}
	if s.installRowShown() {
		rows = append(rows, uRowInstall)
	}
	return rows
}

// moveCursor steps through the visible rows. A press (wrap) goes round the
// ends; auto-repeat stops at them, like Settings.
func (s *UpdatesScreen) moveCursor(dir int, wrap bool) {
	rows := s.visibleRows()
	i := len(rows) - 1 // a row that just vanished was the last one
	for j, r := range rows {
		if r == s.cursor {
			i = j
		}
	}
	switch j := i + dir; {
	case j >= 0 && j < len(rows):
		s.cursor = rows[j]
	case wrap && j < 0:
		s.cursor = rows[len(rows)-1]
	case wrap:
		s.cursor = rows[0]
	default:
		s.cursor = rows[i]
	}
}

// clampCursor moves off a row that is no longer shown.
func (s *UpdatesScreen) clampCursor() {
	if s.cursor == uRowArchive && !s.archiveRowShown() {
		s.cursor = uRowCheck
	}
	if s.cursor == uRowInstall && !s.installRowShown() {
		s.cursor = uRowCheck
	}
}

func (s *UpdatesScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir, s.heldSince = dir, time.Now()
	s.lastRepeat = s.heldSince
	s.moveCursor(dir, true)
}

func (s *UpdatesScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *UpdatesScreen) processAutoRepeat() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay || now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
		return
	}
	s.moveCursor(s.heldDir, false)
	s.lastRepeat = now
}

func (s *UpdatesScreen) NeedsRedraw() bool {
	return s.heldDir != 0 || s.up.IsRunning() || s.up.ArchiveStatus().State == appupdate.ArchiveRunning ||
		s.up.InstallStatus().State == appupdate.InstallRunning
}

func (s *UpdatesScreen) HasPendingAnimation() bool { return false }

func (s *UpdatesScreen) rowLabel(row updatesRow) string {
	switch row {
	case uRowChannel:
		return "Channel: " + s.up.Channel().Label()
	case uRowCheck:
		return "Check now"
	case uRowInstall:
		v, st := s.up.Verdict(), s.up.InstallStatus()
		tag := ""
		if v.Latest != nil {
			tag = v.Latest.Tag
		}
		if st.State == appupdate.InstallStaged && !s.installRowStartsOver() {
			return "Restart now"
		}
		// A stale InstallFailed only counts as "Retry" while it is for the
		// version Latest still points at; once Latest has moved on it is an
		// ordinary Install of the new tag (the FailedInstall rollback report
		// line, not this row, says what happened to the old one).
		failed := v.FailedInstall != "" || (st.State == appupdate.InstallFailed && v.Latest != nil && st.Tag == v.Latest.Tag)
		switch {
		case failed && tag == "":
			return "Retry"
		case failed:
			return "Retry " + tag
		case tag == "":
			return "Install"
		default:
			return "Install " + tag
		}
	default:
		return "Save to ARCHIVE"
	}
}

func (s *UpdatesScreen) activate() Screen {
	switch s.cursor {
	case uRowChannel:
		next := s.up.Channel().Next()
		s.cfg.UpdateChannel = string(next)
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("updates: save channel: %v", err)
		}
		s.up.SetChannel(next)
		logger.Info("updates: channel changed to %s", next)
	case uRowCheck:
		s.up.CheckNow()
	case uRowArchive:
		logger.Info("updates: Save to ARCHIVE requested")
		s.up.StartArchiveSave()
	case uRowInstall:
		if s.up.InstallStatus().State == appupdate.InstallStaged && !s.installRowStartsOver() {
			logger.Info("updates: Restart now requested")
			s.up.RequestRestart()
			return nil // leave the event loop; main applies the update
		}
		logger.Info("updates: Install requested")
		s.up.StartInstall()
	}
	return s
}

func (s *UpdatesScreen) back() Screen {
	if s.qrTex != nil {
		s.qrTex.Destroy()
		s.qrTex = nil
	}
	return s.prev
}

// cancelOrBack is the one path shared by every "leave" input (B, Escape, S,
// Start): while a Save to ARCHIVE is running it cancels and stays, so the
// download is never orphaned on a screen nothing is watching; otherwise it
// leaves normally. Routing all four buttons through this single method is
// what keeps them from diverging again.
func (s *UpdatesScreen) cancelOrBack() Screen {
	if s.up.InstallStatus().State == appupdate.InstallRunning {
		s.up.CancelInstall()
		return s
	}
	if s.up.ArchiveStatus().State == appupdate.ArchiveRunning {
		s.up.CancelArchiveSave()
		return s
	}
	return s.back()
}

func (s *UpdatesScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		switch ev.Keysym.Sym {
		case sdl.K_DOWN, sdl.K_UP:
			dir := 1
			if ev.Keysym.Sym == sdl.K_UP {
				dir = -1
			}
			if ev.Type == sdl.KEYDOWN {
				s.startHold(dir)
			} else {
				s.stopHold(dir)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE, sdl.K_s:
			return s.cancelOrBack()
		}
	case *sdl.ControllerButtonEvent:
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN, sdl.CONTROLLER_BUTTON_DPAD_UP:
			dir := 1
			if ev.Button == sdl.CONTROLLER_BUTTON_DPAD_UP {
				dir = -1
			}
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(dir)
			} else {
				s.stopHold(dir)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case btnA:
			return s.activate()
		case btnB, sdl.CONTROLLER_BUTTON_START:
			return s.cancelOrBack()
		}
	}
	return s
}

func (s *UpdatesScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	s.clampCursor()
	v := s.up.Verdict()
	a := s.up.ArchiveStatus()
	inst := s.up.InstallStatus()
	// Seeing it here counts as being told (spec §2).
	if v.Kind == appupdate.Available && v.Latest != nil {
		s.up.MarkNotified(v.Channel, v.Latest.Tag)
	}

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])
	headerH, footerH := int32(72), int32(52)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("App updates", 20, textY, mt[0], mt[1], mt[2])

	_, fontH := r.TextSize("Ag")
	_, smallH := r.SmallTextSize("Ag")
	rowH := fontH + 14
	y := headerH + 10
	for _, row := range s.visibleRows() {
		sel := row == s.cursor
		tc := r.Theme.ListText
		if sel {
			ac := r.Theme.Accent
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			tc = r.Theme.AccentText
		}
		r.DrawText(s.rowLabel(row), 20, y, tc[0], tc[1], tc[2])
		if row == uRowCheck {
			warn := s.up.IsRunning() || netstate.Offline() || s.up.LastError() != nil
			drawRowAnnotation(r, appUpdateAnnotation(s.up), y, annotationColor(r, sel, r.Theme.Warning(), warn))
		}
		y += rowH
	}

	view := updatesStatus(s.up.Enabled(), v, a, inst, netstate.Offline(), s.up.LastError(), s.up.RateLimitedUntil(), s.up.SourceOverride())
	y += 10
	statusTop := y
	textW := r.W - 40
	side := view.QR != "" && !abbreviate(r.W)
	var qrSize int32
	if side {
		qrSize = min(r.W/3, r.H-footerH-statusTop-smallH-16)
		qrSize = max(80, min(qrSize, 400))
		textW = r.W - 60 - qrSize
	}
	lineH := fontH + 4
	for _, ln := range view.Lines {
		c := r.Theme.MainText
		if ln.Warn {
			c = r.Theme.Warning()
		}
		y += r.DrawWrappedText(ln.Text, 20, y, textW, lineH, c[0], c[1], c[2]) + 6
	}

	installDownloading := inst.State == appupdate.InstallRunning && inst.Phase == appupdate.PhaseDownloading
	if a.State == appupdate.ArchiveRunning || installDownloading {
		done, total := a.Done, a.Total
		if installDownloading {
			done, total = inst.Done, inst.Total
		}
		track, ok := r.Theme.ProgressTrack(), r.Theme.Success()
		r.DrawRect(20, y, textW, 20, track[0], track[1], track[2])
		if total > 0 {
			r.DrawRect(20, y, int32(float64(textW)*float64(done)/float64(total)), 20, ok[0], ok[1], ok[2])
			mu := r.Theme.Muted()
			r.DrawSmallText(fmt.Sprintf("%d%%  (%s / %s)", done*100/total, humanBytes(done), humanBytes(total)),
				20, y+26, mu[0], mu[1], mu[2])
		}
		y += 26 + smallH + 6
	}

	if view.QR != "" {
		var qx, qy int32
		if side {
			qx, qy = r.W-20-qrSize, statusTop
		} else {
			qrSize = min(r.H-footerH-y-smallH-16, 400)
			qx, qy = (r.W-qrSize)/2, y
		}
		if qrSize >= 80 {
			s.drawQR(r, view.QR, qx, qy, qrSize)
			mu := r.Theme.Muted()
			r.DrawSmallTextCentered("Scan for the release notes", qx, qy+qrSize+4, qrSize, mu[0], mu[1], mu[2])
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	back := renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"}
	switch {
	case a.State == appupdate.ArchiveRunning || inst.State == appupdate.InstallRunning:
		back.Text = "Cancel"
	case inst.State == appupdate.InstallStaged:
		back.Text = "Later"
	}
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"}, back,
	}, ftrY)
	r.Present()
}

// drawQR keeps one texture per URL and size instead of rebuilding it every
// frame.
func (s *UpdatesScreen) drawQR(r *renderer.Renderer, url string, x, y, size int32) {
	key := fmt.Sprintf("%s@%d", url, size)
	if s.qrTex == nil || s.qrKey != key {
		if s.qrTex != nil {
			s.qrTex.Destroy()
			s.qrTex = nil
		}
		tex, err := r.QRTexture(url, int(size))
		if err != nil {
			logger.Error("updates: QR texture: %v", err)
			return
		}
		s.qrTex, s.qrKey = tex, key
		logger.Debug("updates: QR %dpx for %s", size, url)
	}
	r.DrawTextureAt(s.qrTex, x, y, size, size)
}
