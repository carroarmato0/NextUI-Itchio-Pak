package appupdate

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

type fakeGitHub struct {
	srv          *httptest.Server
	releases     atomic.Int32
	pak          atomic.Int32
	releasesBody string
	pakBody      string
	status       int
	hdr          map[string]string
	gate         chan struct{} // when set, releases waits on it
}

func newFakeGitHub(t *testing.T, releases, pak string) *fakeGitHub {
	f := &fakeGitHub{releasesBody: releases, pakBody: pak, status: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			f.releases.Add(1)
			if f.gate != nil {
				<-f.gate
			}
			for k, v := range f.hdr {
				w.Header().Set(k, v)
			}
			w.WriteHeader(f.status)
			w.Write([]byte(f.releasesBody))
		case "/pak.json":
			f.pak.Add(1)
			w.Write([]byte(f.pakBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

const releasesRC3 = `[{"tag_name":"v1.1.0-rc3","prerelease":true},{"tag_name":"v1.0.25","prerelease":false}]`

func onlineForTest(t *testing.T, offline *atomic.Bool) {
	old := offlineNow
	offlineNow = offline.Load
	t.Cleanup(func() { offlineNow = old })
}

func newTestChecker(t *testing.T, fw firmware.Kind, running string, ch Channel, gh *fakeGitHub, storeDB string) *Checker {
	t.Helper()
	var off atomic.Bool
	onlineForTest(t, &off)
	return NewChecker(Config{
		Firmware: fw, Running: running, Channel: ch,
		StatePath: filepath.Join(t.TempDir(), "update_state.json"),
		StoreDB:   storeDB, UserAgent: "ua", Source: testSource(gh.srv.URL),
	})
}

func TestChecker_releasesFillBothChannels_switchUsesCache(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.Start()
	c.wg.Wait()
	if v := c.Verdict(); v.Kind != Available || v.Latest.Tag != "v1.1.0-rc3" {
		t.Fatalf("RC verdict = %+v", v)
	}
	c.SetChannel(Stable)
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("switching to Stable made %d requests in total, want 1 (cached)", n)
	}
	if v := c.Verdict(); v.Kind != UpToDate || !v.Ahead || v.Latest.Tag != "v1.0.25" {
		t.Fatalf("Stable verdict = %+v, want UpToDate+Ahead against v1.0.25", v)
	}
}

func TestChecker_offlineOwedRunsOnceOnReconnect(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	var off atomic.Bool
	off.Store(true)
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	onlineForTest(t, &off)
	c.Start()
	c.wg.Wait()
	if gh.releases.Load() != 0 {
		t.Fatal("no request while offline")
	}
	off.Store(false)
	c.RetryAfterReconnect()
	c.wg.Wait()
	c.RetryAfterReconnect()
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("%d requests, want exactly 1 after reconnect", n)
	}
}

func TestChecker_reconnectWithNothingOwed(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.RetryAfterReconnect()
	c.wg.Wait()
	if gh.releases.Load() != 0 {
		t.Fatal("reconnect with nothing owed must not check")
	}
}

func TestChecker_storeManagedStableUsesPakJSONOnly(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, `{"version":"v1.0.26"}`)
	c := newTestChecker(t, firmware.KindNextUI, "v1.0.25", Stable, gh, "../../testdata/pakstore/single.db")
	c.Start()
	c.wg.Wait()
	if gh.releases.Load() != 0 || gh.pak.Load() != 1 {
		t.Fatalf("releases=%d pak=%d, want 0 and 1", gh.releases.Load(), gh.pak.Load())
	}
	if v := c.Verdict(); v.Kind != Available || v.Via != ViaPakStore || v.Latest.Tag != "v1.0.26" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestChecker_rcOnStoreManagedFetchesBoth_andWarns(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, `{"version":"v1.0.25"}`)
	c := newTestChecker(t, firmware.KindNextUI, "v1.1.0-rc3", RC, gh, "../../testdata/pakstore/single.db")
	c.Start()
	c.wg.Wait()
	if gh.releases.Load() != 1 || gh.pak.Load() != 1 {
		t.Fatalf("releases=%d pak=%d, want 1 and 1", gh.releases.Load(), gh.pak.Load())
	}
	if v := c.Verdict(); v.StoreOffers != "v1.0.25" {
		t.Fatalf("StoreOffers = %q, want the downgrade warning (row v1.0.23)", v.StoreOffers)
	}
}

func TestChecker_rateLimitBacksOff(t *testing.T) {
	gh := newFakeGitHub(t, "", "")
	gh.status = 403
	gh.hdr = map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "4102444800"} // 2100
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.Start()
	c.wg.Wait()
	if c.RateLimitedUntil().IsZero() {
		t.Fatal("RateLimitedUntil must be set")
	}
	c.CheckNow()
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("%d requests, want no retry before not_before", n)
	}
}

func TestChecker_devBuildAndOffNeverCheck(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	for _, c := range []*Checker{
		newTestChecker(t, firmware.KindMuOS, "dev", RC, gh, ""),
		newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", Off, gh, ""),
		newTestChecker(t, firmware.KindHost, "v1.1.0-rc2", RC, gh, ""),
	} {
		c.Start()
		c.CheckNow()
		c.wg.Wait()
	}
	if gh.releases.Load() != 0 {
		t.Fatalf("%d requests, want none", gh.releases.Load())
	}
}

func TestChecker_setChannelWhileBusyDoesNotStartSecond(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	gh.gate = make(chan struct{})
	c := newTestChecker(t, firmware.KindMuOS, "v1.0.25", Stable, gh, "")
	c.Start()
	for gh.releases.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	c.SetChannel(RC) // nothing cached for RC yet, but a check is running
	close(gh.gate)
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("%d requests, want 1", n)
	}
	if v := c.Verdict(); v.Channel != RC || v.Latest.Tag != "v1.1.0-rc3" {
		t.Fatalf("verdict = %+v, want the running check's RC result", v)
	}
}

func TestChecker_notificationRulePersists(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.Start()
	c.wg.Wait()
	v, ok := c.PendingNotice()
	if !ok || v.Latest.Tag != "v1.1.0-rc3" {
		t.Fatalf("PendingNotice = %+v, %v", v, ok)
	}
	c.MarkNotified(RC, "v1.1.0-rc3")
	c.MarkNotified(RC, "v1.1.0-rc1") // never lowers
	if _, ok := c.PendingNotice(); ok {
		t.Fatal("announced twice")
	}
	again := NewChecker(Config{Firmware: firmware.KindMuOS, Running: "v1.1.0-rc2", Channel: RC, StatePath: c.cfg.StatePath})
	if _, ok := again.PendingNotice(); ok {
		t.Fatal("notified must survive a restart")
	}
	// Switching channel: Stable's own value is separate (spec §2).
	if again.st.Notified[Stable] != "" {
		t.Fatal("RC's notice must not mark Stable")
	}
}

func TestChecker_archiveSave(t *testing.T) {
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	body := goodMuxapp(t)
	asset := serve(t, body)
	r := releaseFor(asset.URL, body, "v1.1.0-rc3")
	gh := newFakeGitHub(t, `[{"tag_name":"v1.1.0-rc3","prerelease":true,"assets":[{"name":"Itch-io.muOS.v1.1.0-rc3.muxapp","browser_download_url":"`+
		r.Asset+`","size":`+itoa(r.Size)+`,"digest":"`+r.Digest+`"}]}]`, "")
	var off atomic.Bool
	onlineForTest(t, &off)
	dir := filepath.Join(t.TempDir(), "ARCHIVE")
	c := NewChecker(Config{Firmware: firmware.KindMuOS, Running: "v1.1.0-rc2", Channel: RC,
		StatePath: filepath.Join(t.TempDir(), "s.json"), ArchiveDir: dir, UserAgent: "ua", Source: testSource(gh.srv.URL)})
	c.Start()
	c.wg.Wait()
	if v := c.Verdict(); v.Via != ViaArchive {
		t.Fatalf("verdict = %+v, want ViaArchive", v)
	}
	c.StartArchiveSave()
	c.wg.Wait()
	st := c.ArchiveStatus()
	if st.State != ArchiveDone || st.Err != nil {
		t.Fatalf("archive status = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, "Itch-io.muOS.v1.1.0-rc3.muxapp")); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
