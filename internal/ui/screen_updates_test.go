//go:build !headless

package ui

import (
	"context"
	"errors"
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
	disabled  bool
	v         appupdate.Verdict
	archive   appupdate.ArchiveStatus
	cancelled int
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
func (s *stubUpdater) StartArchiveSave()                        {}
func (s *stubUpdater) CancelArchiveSave()                       { s.cancelled++ }
func (s *stubUpdater) ArchiveStatus() appupdate.ArchiveStatus   { return s.archive }

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
			view := updatesStatus(true, c.v, c.a, c.offline, c.err, time.Time{})
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
			for _, l := range updatesStatus(true, v, a, false, nil, time.Time{}).Lines {
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
		view := updatesStatus(false, v, appupdate.ArchiveStatus{}, true, errors.New("boom"), time.Now().Add(time.Hour))
		if len(view.Lines) != 1 || view.Lines[0].Text != "Update checks need a release build." || view.QR != "" {
			t.Fatalf("verdict %+v: view = %+v, want exactly the release-build line", v, view)
		}
	}
}
