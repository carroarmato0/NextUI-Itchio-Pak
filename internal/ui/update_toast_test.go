//go:build !headless

package ui

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
)

func TestNoticeText(t *testing.T) {
	v := appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}}
	if ti, sub := noticeText(v, false); ti != "Itch-io v1.1.0 available" || sub != "Settings → Updates" {
		t.Errorf("wide = %q / %q", ti, sub)
	}
	v.Via = appupdate.ViaPakStore
	if _, sub := noticeText(v, false); sub != "Update it in the Pak Store" {
		t.Errorf("pak store sub = %q", sub)
	}
	if ti, sub := noticeText(v, true); ti != "Update available" || sub != "" {
		t.Errorf("narrow = %q / %q", ti, sub)
	}
}

func TestNoticeShown(t *testing.T) {
	cases := map[time.Duration]float64{
		-time.Millisecond:          0,
		0:                          0,
		noticeSlide:                1,
		noticeSlide + noticeHold/2: 1,
		noticeSlide + noticeHold:   1,
		noticeTotal:                0,
		noticeTotal + time.Second:  0,
	}
	for at, want := range cases {
		if got := noticeShown(at); got != want {
			t.Errorf("noticeShown(%v) = %v, want %v", at, got, want)
		}
	}
	if mid := noticeShown(noticeSlide / 2); mid <= 0.5 || mid >= 1 {
		t.Errorf("ease-out: halfway in = %v, want above 0.5", mid)
	}
}

func TestNoticeMargin(t *testing.T) {
	if noticeMargin(1024, 768) != 12 || noticeMargin(640, 480) != 6 {
		t.Fatal("margin must be 12 px, 6 px when compact")
	}
}

func TestNoticeAllowed(t *testing.T) {
	list := &ListScreen{}
	if !noticeAllowed(list) {
		t.Error("an idle game list allows the notice")
	}
	list.loading.Store(true)
	if noticeAllowed(list) {
		t.Error("not over the startup loading")
	}
	for _, s := range []Screen{&SignInScreen{}, &AccountPromptScreen{}, &UpdatesScreen{}, &CacheRefreshScreen{}, &MigrateFlowScreen{}} {
		if noticeAllowed(s) {
			t.Errorf("%T must hold the notice back", s)
		}
	}
	if !noticeAllowed(&AboutScreen{}) {
		t.Error("ordinary screens allow it")
	}
}

// noticeStubUpdater is a recording AppUpdater for Tick tests: PendingNotice
// stays true until MarkNotified is called, mirroring the real checker
// (checker.go: MarkNotified makes ShouldNotify false for that tag).
type noticeStubUpdater struct {
	v           appupdate.Verdict
	pending     bool
	notifiedN   int
	notifiedCh  appupdate.Channel
	notifiedTag string
}

func (u *noticeStubUpdater) Verdict() appupdate.Verdict               { return u.v }
func (u *noticeStubUpdater) Channel() appupdate.Channel               { return u.v.Channel }
func (u *noticeStubUpdater) SetChannel(appupdate.Channel)             {}
func (u *noticeStubUpdater) CheckNow()                                {}
func (u *noticeStubUpdater) IsRunning() bool                          { return false }
func (u *noticeStubUpdater) CheckedAt() time.Time                     { return time.Time{} }
func (u *noticeStubUpdater) LastError() error                         { return nil }
func (u *noticeStubUpdater) RateLimitedUntil() time.Time              { return time.Time{} }
func (u *noticeStubUpdater) PendingNotice() (appupdate.Verdict, bool) { return u.v, u.pending }
func (u *noticeStubUpdater) MarkNotified(ch appupdate.Channel, tag string) {
	u.notifiedN++
	u.notifiedCh = ch
	u.notifiedTag = tag
	u.pending = false
}
func (u *noticeStubUpdater) StartArchiveSave()                      {}
func (u *noticeStubUpdater) CancelArchiveSave()                     {}
func (u *noticeStubUpdater) ArchiveStatus() appupdate.ArchiveStatus { return appupdate.ArchiveStatus{} }

func TestNoticeTick_cutShortWhenScreenNoLongerAllowsIt(t *testing.T) {
	up := &noticeStubUpdater{
		v:       appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}},
		pending: true,
	}
	SetAppUpdater(up)
	t.Cleanup(func() { SetAppUpdater(nil) })

	r := &renderer.Renderer{}
	var n UpdateNotice
	t0 := time.Now()

	if !n.Tick(r, &AboutScreen{}, t0) {
		t.Fatal("expected a redraw when the notice starts")
	}
	if !n.Animating() || r.Overlay == nil {
		t.Fatal("notice should be active with an overlay set")
	}

	if !n.Tick(r, &UpdatesScreen{}, t0.Add(time.Second)) {
		t.Fatal("expected a redraw on the cut-short finish")
	}
	if n.Animating() || r.Overlay != nil {
		t.Fatal("notice must end immediately once the screen no longer allows it")
	}
	if up.notifiedN != 1 || up.notifiedTag != "v1.1.0" {
		t.Fatalf("MarkNotified called %d times, tag=%q, want 1 time with v1.1.0", up.notifiedN, up.notifiedTag)
	}
}

func TestNoticeTick_normalFinish(t *testing.T) {
	up := &noticeStubUpdater{
		v:       appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}},
		pending: true,
	}
	SetAppUpdater(up)
	t.Cleanup(func() { SetAppUpdater(nil) })

	r := &renderer.Renderer{}
	var n UpdateNotice
	t0 := time.Now()

	n.Tick(r, &AboutScreen{}, t0)

	if !n.Tick(r, &AboutScreen{}, t0.Add(noticeTotal)) {
		t.Fatal("expected a redraw on the normal finish")
	}
	if n.Animating() || r.Overlay != nil {
		t.Fatal("notice should have ended")
	}
	if up.notifiedN != 1 {
		t.Fatalf("MarkNotified called %d times, want 1", up.notifiedN)
	}

	if n.Tick(r, &AboutScreen{}, t0.Add(noticeTotal+time.Second)) {
		t.Fatal("nothing should start again once PendingNotice reports false")
	}
	if n.Animating() {
		t.Fatal("should stay inactive")
	}
}
