//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

// stubUpdater is a minimal AppUpdater; the Settings tests (Task 12) use it too.
type stubUpdater struct {
	disabled         bool
	v                appupdate.Verdict
	archive          appupdate.ArchiveStatus
	cancelled        int
	inst             appupdate.InstallStatus
	started          bool
	installCancelled bool
	restarted        bool
	override         string
}

func (s *stubUpdater) Enabled() bool                            { return !s.disabled }
func (s *stubUpdater) Verdict() appupdate.Verdict               { return s.v }
func (s *stubUpdater) Channel() appupdate.Channel               { return s.v.Channel }
func (s *stubUpdater) SetChannel(ch appupdate.Channel)          { s.v.Channel = ch }
func (s *stubUpdater) CheckNow()                                {}
func (s *stubUpdater) IsRunning() bool                          { return false }
func (s *stubUpdater) CheckedAt() time.Time                     { return time.Time{} }
func (s *stubUpdater) LastError() error                         { return nil }
func (s *stubUpdater) RateLimitedUntil() time.Time              { return time.Time{} }
func (s *stubUpdater) PendingNotice() (appupdate.Verdict, bool) { return s.v, false }
func (s *stubUpdater) MarkNotified(appupdate.Channel, string)   {}
func (s *stubUpdater) PendingRollbackNotice() (appupdate.Pending, bool) {
	return appupdate.Pending{}, false
}
func (s *stubUpdater) MarkRollbackNotified()                  {}
func (s *stubUpdater) StartArchiveSave()                      {}
func (s *stubUpdater) CancelArchiveSave()                     { s.cancelled++ }
func (s *stubUpdater) ArchiveStatus() appupdate.ArchiveStatus { return s.archive }
func (s *stubUpdater) StartInstall()                          { s.started = true }
func (s *stubUpdater) CancelInstall()                         { s.installCancelled = true }
func (s *stubUpdater) InstallStatus() appupdate.InstallStatus { return s.inst }
func (s *stubUpdater) RequestRestart()                        { s.restarted = true }
func (s *stubUpdater) SourceOverride() string                 { return s.override }

// stubScreen is a minimal Screen distinct from *UpdatesScreen, so a test can
// tell "stayed on Updates" apart from "left to prev" unambiguously.
type stubScreen struct{}

func (stubScreen) Draw(*renderer.Renderer)      {}
func (stubScreen) HandleEvent(sdl.Event) Screen { return stubScreen{} }
func (stubScreen) NeedsRedraw() bool            { return false }
func (stubScreen) HasPendingAnimation() bool    { return false }

func ver(s string) appupdate.Version { v, _ := appupdate.Parse(s); return v }

func TestLastCheckedLabel(t *testing.T) {
	if lastCheckedLabel(time.Time{}) != "never" || lastCheckedLabel(time.Now().Add(-5*time.Minute)) != "last: 5m ago" {
		t.Fatal("unexpected labels")
	}
}

func newTestUpdatesScreen(t *testing.T, v appupdate.Verdict) (*UpdatesScreen, *stubUpdater) {
	up := &stubUpdater{v: v}
	cfg := &settings.Config{}
	return NewUpdatesScreen(cfg, filepath.Join(t.TempDir(), "config.json"), up, nil), up
}

var archiveVerdict = appupdate.Verdict{Kind: appupdate.Available, Via: appupdate.ViaArchive,
	Channel: appupdate.RC, Running: ver("v1.1.0-rc2"), Latest: &appupdate.Release{Tag: "v1.1.0-rc3", URL: "https://example.invalid/v1.1.0-rc3"}}

func TestUpdatesCursor_pressWrapsAtEnds(t *testing.T) {
	s, _ := newTestUpdatesScreen(t, archiveVerdict)
	s.startHold(-1)
	if s.cursor != uRowArchive {
		t.Fatalf("up from the first row: %d, want the archive row", s.cursor)
	}
	s.stopHold(-1)
	s.startHold(1)
	if s.cursor != uRowChannel {
		t.Fatalf("down from the last row: %d, want Channel", s.cursor)
	}
}

func TestUpdatesCursor_repeatStopsAtEnds(t *testing.T) {
	s, _ := newTestUpdatesScreen(t, archiveVerdict)
	s.moveCursor(-1, false)
	if s.cursor != uRowChannel {
		t.Fatalf("held up at the first row moved to %d", s.cursor)
	}
}

func TestUpdatesCursor_hiddenArchiveRowClamps(t *testing.T) {
	s, up := newTestUpdatesScreen(t, archiveVerdict)
	s.cursor = uRowArchive
	up.v = appupdate.Verdict{Kind: appupdate.UpToDate, Channel: appupdate.RC, Latest: &appupdate.Release{Tag: "v1.1.0-rc3"}}
	s.moveCursor(1, true)
	if s.cursor != uRowChannel {
		t.Fatalf("down from a vanished last row: %d, want to wrap to Channel", s.cursor)
	}
	s.cursor = uRowArchive
	s.clampCursor()
	if s.cursor != uRowCheck {
		t.Fatalf("clamp: %d, want Check now", s.cursor)
	}
}

func TestUpdatesChannelCycleSaves(t *testing.T) {
	s, up := newTestUpdatesScreen(t, appupdate.Verdict{Channel: appupdate.Stable})
	s.cursor = uRowChannel
	s.activate()
	if up.v.Channel != appupdate.RC || s.cfg.UpdateChannel != "rc" {
		t.Fatalf("after A: updater=%s cfg=%q, want rc", up.v.Channel, s.cfg.UpdateChannel)
	}
	if loaded, _ := settings.Load(s.cfgPath); loaded.UpdateChannel != "rc" {
		t.Fatalf("saved update_channel = %q", loaded.UpdateChannel)
	}
}

func TestUpdatesScreen_IsBusyWhileArchiveRunning(t *testing.T) {
	s, up := newTestUpdatesScreen(t, archiveVerdict)
	if s.IsBusy() {
		t.Fatal("IsBusy true before any archive save started")
	}
	up.archive = appupdate.ArchiveStatus{State: appupdate.ArchiveRunning}
	if !s.IsBusy() {
		t.Fatal("IsBusy false while ArchiveRunning")
	}
	up.archive = appupdate.ArchiveStatus{State: appupdate.ArchiveDone}
	if s.IsBusy() {
		t.Fatal("IsBusy true after archive save finished")
	}
}

// TestUpdatesScreen_StartAndSCancelWhileDownloading guards against Start and
// keyboard S leaving the screen mid-download: once another screen is
// current, the main loop's BusyChecker gate no longer sees this screen, so
// sleep/shutdown stops waiting for the ARCHIVE save. Start and S must behave
// exactly like B/Escape: cancel and stay while downloading, leave otherwise.
func TestUpdatesScreen_StartAndSCancelWhileDownloading(t *testing.T) {
	prev := stubScreen{}
	up := &stubUpdater{v: archiveVerdict, archive: appupdate.ArchiveStatus{State: appupdate.ArchiveRunning}}
	cfg := &settings.Config{}
	s := NewUpdatesScreen(cfg, filepath.Join(t.TempDir(), "config.json"), up, prev)

	startEv := &sdl.ControllerButtonEvent{Type: sdl.CONTROLLERBUTTONDOWN, Button: sdl.CONTROLLER_BUTTON_START}
	sEv := &sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Sym: sdl.K_s}}

	if got := s.HandleEvent(startEv); got != Screen(s) {
		t.Fatalf("Start while downloading: got %v, want the Updates screen itself", got)
	}
	if up.cancelled != 1 {
		t.Fatalf("Start while downloading: cancelled = %d, want 1", up.cancelled)
	}
	if got := s.HandleEvent(sEv); got != Screen(s) {
		t.Fatalf("S while downloading: got %v, want the Updates screen itself", got)
	}
	if up.cancelled != 2 {
		t.Fatalf("S while downloading: cancelled = %d, want 2", up.cancelled)
	}

	up.archive = appupdate.ArchiveStatus{State: appupdate.ArchiveIdle}
	if got := s.HandleEvent(startEv); got != Screen(prev) {
		t.Fatalf("Start while idle: got %v, want prev", got)
	}
	if got := s.HandleEvent(sEv); got != Screen(prev) {
		t.Fatalf("S while idle: got %v, want prev", got)
	}
	if up.cancelled != 2 {
		t.Fatalf("cancelled changed while idle: %d, want unchanged at 2", up.cancelled)
	}
}

func TestUpdatesStatus(t *testing.T) {
	rel := func(tag string) *appupdate.Release {
		return &appupdate.Release{Tag: tag, URL: "https://example.invalid/" + tag}
	}
	cases := []struct {
		name    string
		v       appupdate.Verdict
		a       appupdate.ArchiveStatus
		offline bool
		err     error
		want    string // substring of the joined lines
		qr      bool
	}{
		{"off", appupdate.Verdict{Channel: appupdate.Off}, appupdate.ArchiveStatus{}, false, nil, "Update checks are off.", false},
		{"latest", appupdate.Verdict{Kind: appupdate.UpToDate, Running: ver("v1.1.0-rc3"), Latest: rel("v1.1.0-rc3")}, appupdate.ArchiveStatus{}, false, nil, "You have the latest version (v1.1.0-rc3).", false},
		{"ahead", appupdate.Verdict{Kind: appupdate.UpToDate, Ahead: true, Running: ver("v1.1.0-rc3"), Latest: rel("v1.0.25")}, appupdate.ArchiveStatus{}, false, nil,
			"You are on v1.1.0-rc3, a release candidate newer than the latest stable release (v1.0.25).", false},
		{"pak store", appupdate.Verdict{Kind: appupdate.Available, Via: appupdate.ViaPakStore, Latest: rel("v1.0.26")}, appupdate.ArchiveStatus{}, false, nil, "v1.0.26 is available. Update it in the Pak Store.", false},
		{"release page", appupdate.Verdict{Kind: appupdate.Available, Latest: rel("v1.1.0")}, appupdate.ArchiveStatus{}, false, nil, "v1.1.0 is available.", true},
		{"offline, nothing known", appupdate.Verdict{}, appupdate.ArchiveStatus{}, true, nil, "You're offline.", false},
		{"store downgrade", appupdate.Verdict{Kind: appupdate.UpToDate, Running: ver("v1.1.0-rc3"), Latest: rel("v1.1.0-rc3"), StoreOffers: "v1.0.25"}, appupdate.ArchiveStatus{}, false, nil,
			"The Pak Store may offer v1.0.25 as an update. It is older than this release candidate; installing it would replace it.", false},
		{"saved", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveDone, Tag: "v1.1.0-rc3"}, false, nil, "Saved to ARCHIVE. Open Applications → Archive Manager to install.", true},
		{"integrity", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Tag: "v1.1.0-rc3", Err: appupdate.ErrIntegrity}, false, nil, "Download failed the integrity check.", true},
		{"cancelled", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Tag: "v1.1.0-rc3", Err: context.Canceled}, false, nil, "Download cancelled.", true},
		{"check failed", appupdate.Verdict{}, appupdate.ArchiveStatus{}, false, errors.New("decode releases"), "The last check failed.", false},
		{"available, latest nil", appupdate.Verdict{Kind: appupdate.Available}, appupdate.ArchiveStatus{}, false, nil, "Not checked yet.", false},
		{"up to date, latest nil", appupdate.Verdict{Kind: appupdate.UpToDate, Running: ver("v1.1.0-rc3")}, appupdate.ArchiveStatus{}, false, nil, "You have the latest version (v1.1.0-rc3).", false},
		{"archive running", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveRunning, Tag: "v1.1.0-rc3"}, false, nil, "Downloading v1.1.0-rc3…", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			view := updatesStatus(true, c.v, c.a, appupdate.InstallStatus{}, c.offline, c.err, time.Time{}, "")
			var texts []string
			for _, l := range view.Lines {
				texts = append(texts, l.Text)
			}
			joined := strings.Join(texts, " | ")
			if !strings.Contains(joined, c.want) {
				t.Fatalf("lines = %q, want %q", joined, c.want)
			}
			if (view.QR != "") != c.qr {
				t.Fatalf("QR = %q, want present=%v", view.QR, c.qr)
			}
		})
	}
}

// An ARCHIVE outcome belongs to the version it was saved for: once Latest
// moves on (or is unknown), it is not shown under another version.
func TestUpdatesStatus_archiveLineOnlyForItsVersion(t *testing.T) {
	for _, st := range []appupdate.ArchiveState{appupdate.ArchiveRunning, appupdate.ArchiveDone, appupdate.ArchiveFailed} {
		for name, v := range map[string]appupdate.Verdict{
			"other version": archiveVerdict,
			"latest nil":    {Kind: appupdate.Available, Via: appupdate.ViaArchive},
		} {
			a := appupdate.ArchiveStatus{State: st, Tag: "v1.1.0-rc2", Err: appupdate.ErrIntegrity}
			for _, l := range updatesStatus(true, v, a, appupdate.InstallStatus{}, false, nil, time.Time{}, "").Lines {
				if strings.Contains(l.Text, "Download") || strings.Contains(l.Text, "ARCHIVE") {
					t.Fatalf("state %d, %s: line %q shown for an outcome saved for v1.1.0-rc2", st, name, l.Text)
				}
			}
		}
	}
}

// A dev or unparseable build never checks: say so, and nothing else.
func TestUpdatesStatus_disabledBuild(t *testing.T) {
	for _, v := range []appupdate.Verdict{
		{Channel: appupdate.Stable},
		{Channel: appupdate.Off},
		{Kind: appupdate.Available, Channel: appupdate.RC, Latest: &appupdate.Release{Tag: "v1.1.0", URL: "https://example.invalid/"}, StoreOffers: "v1.0.25"},
	} {
		view := updatesStatus(false, v, appupdate.ArchiveStatus{}, appupdate.InstallStatus{}, true, errors.New("boom"), time.Now().Add(time.Hour), "")
		if len(view.Lines) != 1 || view.Lines[0].Text != "Update checks need a release build." || view.QR != "" {
			t.Fatalf("verdict %+v: view = %+v, want exactly the release-build line", v, view)
		}
	}
}

func installVerdict() appupdate.Verdict {
	v, _ := appupdate.Parse("v1.1.0-rc4")
	return appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.RC, Running: v,
		Latest: &appupdate.Release{Tag: "v1.1.0-rc5", URL: "https://example.invalid/r"}, Via: appupdate.ViaInstall}
}

func TestUpdatesStatus_install(t *testing.T) {
	v := installVerdict()
	cases := []struct {
		name string
		v    appupdate.Verdict
		inst appupdate.InstallStatus
		want string
	}{
		{"available", v, appupdate.InstallStatus{}, "v1.1.0-rc5 is available."},
		{"downloading", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Tag: "v1.1.0-rc5"}, "Downloading v1.1.0-rc5…"},
		{"checking", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Phase: appupdate.PhaseChecking, Tag: "v1.1.0-rc5"}, "Checking v1.1.0-rc5…"},
		{"unpacking", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Phase: appupdate.PhaseUnpacking, Tag: "v1.1.0-rc5"}, "Unpacking v1.1.0-rc5…"},
		{"staged", v, appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}, "v1.1.0-rc5 is ready. Restart to finish installing, or it installs the next time you open Itch-io."},
		{"space", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: &appupdate.SpaceError{Need: 34 << 20, Have: 12 << 20}}, "Not enough space on the SD card: needs 34 MB, 12 MB free."},
		{"damaged", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: fmt.Errorf("%w: pak.json says x", appupdate.ErrDamaged)}, "The update is damaged: pak.json says x."},
		{"integrity", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: appupdate.ErrIntegrity}, "The download failed the integrity check."},
	}
	for _, c := range cases {
		view := updatesStatus(true, c.v, appupdate.ArchiveStatus{}, c.inst, false, nil, time.Time{}, "")
		if !hasLine(view, c.want) {
			t.Errorf("%s: lines %v lack %q", c.name, view.Lines, c.want)
		}
	}
	failed := v
	failed.FailedInstall, failed.FailedFrom = "v1.1.0-rc5", "v1.1.0-rc4"
	if view := updatesStatus(true, failed, appupdate.ArchiveStatus{}, appupdate.InstallStatus{}, false, nil, time.Time{}, ""); !hasLine(view, "v1.1.0-rc5 did not start, so v1.1.0-rc4 was kept.") {
		t.Errorf("failed: lines %v", view.Lines)
	}
	if view := updatesStatus(true, v, appupdate.ArchiveStatus{}, appupdate.InstallStatus{}, false, nil, time.Time{}, "http://127.0.0.1:8765/"); !hasLine(view, "Test update source: http://127.0.0.1:8765/") {
		t.Errorf("override: lines %v", view.Lines)
	}
}

func hasLine(v updatesStatusView, text string) bool {
	for _, l := range v.Lines {
		if l.Text == text {
			return true
		}
	}
	return false
}

func TestUpdatesScreen_installRows(t *testing.T) {
	u := &stubUpdater{v: installVerdict()}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)
	rows := s.visibleRows()
	if rows[len(rows)-1] != uRowInstall || s.rowLabel(uRowInstall) != "Install v1.1.0-rc5" {
		t.Fatalf("rows %v, label %q", rows, s.rowLabel(uRowInstall))
	}
	s.cursor = uRowInstall
	s.activate()
	if !u.started {
		t.Fatal("A on Install must start the install")
	}
	u.inst = appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}
	if s.rowLabel(uRowInstall) != "Restart now" {
		t.Fatalf("staged label %q", s.rowLabel(uRowInstall))
	}
	if next := s.activate(); next != nil || !u.restarted {
		t.Fatal("Restart now must request a restart and leave the event loop (nil screen)")
	}
	u.v.FailedInstall = "v1.1.0-rc5"
	u.inst = appupdate.InstallStatus{}
	if s.rowLabel(uRowInstall) != "Retry v1.1.0-rc5" {
		t.Fatalf("failed label %q", s.rowLabel(uRowInstall))
	}
}

func TestUpdatesScreen_backCancelsAnInstall(t *testing.T) {
	u := &stubUpdater{v: installVerdict(), inst: appupdate.InstallStatus{State: appupdate.InstallRunning}}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)
	if next := s.cancelOrBack(); next != s || !u.installCancelled {
		t.Fatal("B while installing must cancel and stay")
	}
	if !s.IsBusy() {
		t.Fatal("a running install keeps the app awake")
	}
}

// Fix round 1: a staged build for one tag must not be hidden behind, or
// silently conflict with, a later check that moves Latest on to a newer tag
// — the row must offer to install the newer release instead of restarting
// into the older staged one, and the status block must still say the staged
// build (by its own tag) is ready.
func TestUpdatesScreen_stagedTagOlderThanLatest(t *testing.T) {
	v := installVerdict() // Latest: v1.1.0-rc5
	v.Latest = &appupdate.Release{Tag: "v1.1.0-rc6", URL: "https://example.invalid/r6"}
	u := &stubUpdater{v: v, inst: appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)

	if got := s.rowLabel(uRowInstall); got != "Install v1.1.0-rc6" {
		t.Fatalf("label %q, want %q", got, "Install v1.1.0-rc6")
	}
	s.cursor = uRowInstall
	if next := s.activate(); next != s || !u.started || u.restarted {
		t.Fatalf("activate: next=%v started=%v restarted=%v, want stay on screen with StartInstall only", next, u.started, u.restarted)
	}
	view := updatesStatus(true, v, appupdate.ArchiveStatus{}, u.inst, false, nil, time.Time{}, "")
	if !hasLine(view, "v1.1.0-rc5 is ready. Restart to finish installing, or it installs the next time you open Itch-io.") {
		t.Fatalf("status lines %v lack the staged rc5 line", view.Lines)
	}
}

// A staged build with no Latest at all (e.g. the channel changed) still
// offers only to restart into it: there is nothing newer to install instead.
func TestUpdatesScreen_stagedTagWithNoLatest(t *testing.T) {
	v := installVerdict()
	v.Latest = nil
	u := &stubUpdater{v: v, inst: appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)

	if got := s.rowLabel(uRowInstall); got != "Restart now" {
		t.Fatalf("label %q, want %q", got, "Restart now")
	}
	view := updatesStatus(true, v, appupdate.ArchiveStatus{}, u.inst, false, nil, time.Time{}, "")
	if !hasLine(view, "v1.1.0-rc5 is ready. Restart to finish installing, or it installs the next time you open Itch-io.") {
		t.Fatalf("status lines %v lack the staged rc5 line", view.Lines)
	}
}

// A stale InstallFailed for an old tag, once Latest has moved past it and
// with no FailedInstall rollback recorded, is an ordinary Install of the new
// tag rather than a Retry of the old one.
func TestUpdatesScreen_failedTagOlderThanLatestIsNotRetry(t *testing.T) {
	v := installVerdict()
	v.Latest = &appupdate.Release{Tag: "v1.1.0-rc6", URL: "https://example.invalid/r6"}
	u := &stubUpdater{v: v, inst: appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: appupdate.ErrIntegrity}}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)

	if got := s.rowLabel(uRowInstall); got != "Install v1.1.0-rc6" {
		t.Fatalf("label %q, want %q", got, "Install v1.1.0-rc6")
	}
}
