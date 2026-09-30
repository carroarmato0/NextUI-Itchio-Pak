//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

// statusLine is one paragraph under the Updates rows.
type statusLine struct {
	Text string
	Warn bool
}

// updatesStatusView is what the Updates screen says, independent of drawing.
type updatesStatusView struct {
	Lines []statusLine
	QR    string // release page to encode; "" for none
}

// updatesStatus builds the status block (spec §4, §2a). enabled is the
// checker's Enabled(): a dev or unparseable build never checks, so it says
// that and nothing else rather than "Not checked yet" forever.
func updatesStatus(enabled bool, v appupdate.Verdict, a appupdate.ArchiveStatus, offline bool, lastErr error, rateUntil time.Time) updatesStatusView {
	var out updatesStatusView
	add := func(text string, warn bool) { out.Lines = append(out.Lines, statusLine{text, warn}) }

	if !enabled {
		add("Update checks need a release build.", false)
		return out
	}

	switch {
	case v.Channel == appupdate.Off:
		add("Update checks are off.", false)
		return out
	case v.Kind == appupdate.UpToDate && v.Latest == nil:
		// Latest is nil-guarded here too: UpToDate is only ever set alongside
		// a Latest release, but a dereference below must never be the thing
		// that turns a decide.go bug into a crash on this screen.
		add(fmt.Sprintf("You have the latest version (%s).", v.Running), false)
	case v.Kind == appupdate.UpToDate && !v.Ahead:
		add(fmt.Sprintf("You have the latest version (%s).", v.Running), false)
	case v.Kind == appupdate.UpToDate && v.Running.IsRC():
		add(fmt.Sprintf("You are on %s, a release candidate newer than the latest stable release (%s). "+
			"You will be told when a newer stable release is out.", v.Running, v.Latest.Tag), false)
	case v.Kind == appupdate.UpToDate:
		add(fmt.Sprintf("You are on %s, newer than the latest release (%s).", v.Running, v.Latest.Tag), false)
	case v.Kind == appupdate.Available && v.Latest == nil:
		add("Not checked yet.", false)
	case v.Kind == appupdate.Available && v.Via == appupdate.ViaPakStore:
		add(fmt.Sprintf("%s is available. Update it in the Pak Store.", v.Latest.Tag), false)
	case v.Kind == appupdate.Available:
		add(fmt.Sprintf("%s is available.", v.Latest.Tag), false)
		out.QR = v.Latest.URL
	case offline:
		add("You're offline. Itch-io checks for updates when the connection is back.", true)
	default:
		add("Not checked yet.", false)
	}

	switch {
	case !rateUntil.IsZero():
		add("GitHub is limiting update checks. Try again after "+rateUntil.Local().Format("15:04")+".", true)
	case lastErr != nil:
		if m, ok := netstate.Describe(lastErr, "GitHub"); ok {
			add(m.Title, true)
			if m.Hint != "" {
				add(m.Hint, false)
			}
		} else {
			add("The last check failed.", true)
		}
	}

	// An ARCHIVE outcome is shown only under the version it was saved for: a
	// later check can move Latest on, and "Saved to ARCHIVE" must not then
	// appear to be about the newer release.
	archiveState := appupdate.ArchiveIdle
	if v.Latest != nil && a.Tag == v.Latest.Tag {
		archiveState = a.State
	}
	switch archiveState {
	case appupdate.ArchiveRunning:
		add(fmt.Sprintf("Downloading %s…", v.Latest.Tag), false)
	case appupdate.ArchiveDone:
		add("Saved to ARCHIVE. Open Applications → Archive Manager to install.", false)
	case appupdate.ArchiveFailed:
		add(archiveFailure(a.Err), true)
	}

	if v.StoreOffers != "" {
		older := "this release candidate"
		if !v.Running.IsRC() {
			older = "the version you have (" + v.Running.String() + ")"
		}
		add(fmt.Sprintf("The Pak Store may offer %s as an update. It is older than %s; installing it would replace it.",
			v.StoreOffers, older), true)
	}
	return out
}

// archiveFailure words a failed Save to ARCHIVE.
func archiveFailure(err error) string {
	var se *appupdate.SpaceError
	switch {
	case errors.Is(err, context.Canceled):
		return "Download cancelled."
	case errors.Is(err, appupdate.ErrIntegrity):
		return "Download failed the integrity check."
	case errors.As(err, &se):
		return fmt.Sprintf("Not enough space on the SD card: %s needed, %s free.", humanBytes(se.Need), humanBytes(se.Have))
	}
	if m, ok := netstate.Describe(err, "GitHub"); ok {
		return m.Title
	}
	return "Download failed."
}

// appUpdateAnnotation is the "Check now" row's right-aligned label.
func appUpdateAnnotation(up AppUpdater) string {
	switch {
	case netstate.Offline():
		return "offline"
	case up.IsRunning():
		return "checking…"
	case up.LastError() != nil:
		return "failed"
	}
	return lastCheckedLabel(up.CheckedAt())
}

// lastCheckedLabel is "last: 5m ago", or "never" for the zero time. Shared
// with Settings' Update Inventory row (Task 12).
func lastCheckedLabel(t time.Time) string {
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

// annotationColor is the colour of a right-aligned row annotation. Warning and
// Muted are toned against Background, so on the selected row — filled with an
// Accent pill — they can vanish (orange on orange). ToneOn keeps the hue and
// clears contrast against the pill; the Mix de-emphasises relative to the pill
// the way Settings' "not signed in" does. The same rule Settings has used for
// Update Inventory since the rc2 readability fix.
func annotationColor(r *renderer.Renderer, selected bool, tone [3]uint8, emphasised bool) [3]uint8 {
	switch {
	case selected && emphasised:
		return r.Theme.ToneOn(tone, r.Theme.Accent)
	case selected:
		return theme.Mix(r.Theme.Accent, r.Theme.AccentText, 65)
	case emphasised:
		return tone
	default:
		return r.Theme.Muted()
	}
}

// drawRowAnnotation right-aligns small text on a settings-style row at y.
func drawRowAnnotation(r *renderer.Renderer, text string, y int32, c [3]uint8) {
	aw, sh := r.SmallTextSize(text)
	_, fh := r.TextSize("Ag")
	r.DrawSmallText(text, r.W-aw-20, y+(fh-sh)/2, c[0], c[1], c[2])
}
