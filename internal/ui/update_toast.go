//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

const (
	noticeSlide = 250 * time.Millisecond
	noticeHold  = 4 * time.Second
	noticeTotal = 2*noticeSlide + noticeHold
)

// UpdateNotice is the "new version" notice, drawn top-right over whatever
// screen is current (spec §3). main_sdl.go owns one and calls Tick every loop
// iteration. It consumes no input.
type UpdateNotice struct {
	verdict appupdate.Verdict
	start   time.Time
	active  bool
}

// Animating reports whether the loop must redraw every 16 ms.
func (n *UpdateNotice) Animating() bool { return n.active }

// Tick starts the notice when one is due and the current screen allows it,
// and retires it when the animation ends or the screen stops allowing it
// (spec §2, §3 — e.g. the user opens Settings → Updates mid-notice). true
// means redraw now.
func (n *UpdateNotice) Tick(r *renderer.Renderer, current Screen, now time.Time) bool {
	up := appUpdater()
	if n.active {
		if now.Sub(n.start) < noticeTotal && noticeAllowed(current) {
			return true
		}
		n.finish(r, up, now.Sub(n.start) >= noticeTotal)
		return true // one more frame, without it
	}
	if up == nil || !noticeAllowed(current) {
		return false
	}
	v, ok := up.PendingNotice()
	if !ok {
		return false
	}
	n.verdict, n.start, n.active = v, now, true
	r.Overlay = func(r *renderer.Renderer) {
		drawNotice(r, n.verdict, noticeShown(time.Since(n.start)))
	}
	logger.Info("update notice: showing %s (channel=%s via=%s)", v.Latest.Tag, v.Channel, v.Via)
	return true
}

// finish ends the notice, whether it ran its full course or was cut short by
// leaving to a screen that must not show it. Either way the user has been
// told — the Updates screen that cuts it short says the same thing itself —
// so it is marked notified once (spec §2).
func (n *UpdateNotice) finish(r *renderer.Renderer, up AppUpdater, full bool) {
	n.active = false
	r.Overlay = nil
	if up != nil {
		up.MarkNotified(n.verdict.Channel, n.verdict.Latest.Tag)
	}
	if full {
		logger.Debug("update notice: finished for %s", n.verdict.Latest.Tag)
	} else {
		logger.Debug("update notice: cut short for %s", n.verdict.Latest.Tag)
	}
}

// noticeAllowed keeps the notice off startup and sign-in, and off the Updates
// screen, which already says it (spec §2, §3).
func noticeAllowed(s Screen) bool {
	switch sc := s.(type) {
	case *SignInScreen, *AccountPromptScreen, *UpdatesScreen, *CacheRefreshScreen, *MigrateFlowScreen:
		return false
	case *ListScreen:
		return !sc.loading.Load()
	}
	return true
}

// noticeText is the notice's wording; narrow (abbreviate) fits one line.
func noticeText(v appupdate.Verdict, narrow bool) (title, sub string) {
	if narrow {
		return "Update available", ""
	}
	title = "Itch-io " + v.Latest.Tag + " available"
	if v.Via == appupdate.ViaPakStore {
		return title, "Update it in the Pak Store"
	}
	return title, "Settings → Updates"
}

// noticeShown is how far the notice has slid in: 0 above the screen, 1 at
// rest. Ease-out on the way in, ease-in on the way out.
func noticeShown(elapsed time.Duration) float64 {
	switch {
	case elapsed <= 0 || elapsed >= noticeTotal:
		return 0
	case elapsed < noticeSlide:
		p := float64(elapsed) / float64(noticeSlide)
		return 1 - (1-p)*(1-p)
	case elapsed <= noticeSlide+noticeHold:
		return 1
	default:
		p := float64(elapsed-noticeSlide-noticeHold) / float64(noticeSlide)
		return 1 - p*p
	}
}

// noticeMargin insets the notice from the top and right edges.
func noticeMargin(w, h int32) int32 {
	if compact(w, h) {
		return 6
	}
	return 12
}

// drawNotice draws the notice shown (0..1) of the way in. Accent fill with a
// ModalBorder outline, so it does not read as one more header pill on the
// game list (those are TitlePill and Chip).
func drawNotice(r *renderer.Renderer, v appupdate.Verdict, shown float64) {
	if shown <= 0 || v.Latest == nil {
		return
	}
	title, sub := noticeText(v, abbreviate(r.W))
	m := noticeMargin(r.W, r.H)

	// Both lines at the small size: the title bold, the hint regular and
	// softer, so the notice reads as a notice rather than a second header.
	// Padding follows the text height, so every geometry gets the same
	// proportions (pixel constants were cramped at 480 and loose at 768).
	size := int(r.H / 32)
	tw, th := r.DisplayTextSize(title, size)
	var sw, sh int32
	if sub != "" {
		sw, sh = r.SmallTextSize(sub)
	}
	padX, padY, gap := th*3/4, th/4, int32(0)
	w := max(tw, sw) + 2*padX
	h := th + 2*padY
	if sub != "" {
		h += gap + sh
	}
	x := r.W - m - w
	// From fully above the top edge (y = -h) down to the margin.
	y := int32(float64(-h-2) + shown*float64(h+2+m))

	bd, ac, at := r.Theme.ModalBorder(), r.Theme.Accent, r.Theme.AccentText
	hint := theme.Mix(ac, at, 75)
	rad := th / 3
	r.DrawRoundedRect(x-1, y-1, w+2, h+2, rad+1, bd[0], bd[1], bd[2])
	r.DrawRoundedRect(x, y, w, h, rad, ac[0], ac[1], ac[2])
	r.DrawDisplayText(title, x+padX, y+padY, size, at[0], at[1], at[2])
	if sub != "" {
		r.DrawSmallText(sub, x+padX, y+padY+th+gap, hint[0], hint[1], hint[2])
	}
}
