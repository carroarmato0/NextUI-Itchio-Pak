//go:build !headless

package ui

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
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
