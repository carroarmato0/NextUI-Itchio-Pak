package appupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
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
	pakGate      chan struct{} // when set, pak.json waits on it
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
			if f.pakGate != nil {
				<-f.pakGate
			}
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

// TestChecker_offlineMidCheckOwesUntilReconnect covers a check that starts
// (netstate's own status is unknown or online, so gateOK lets it through)
// but whose request fails because the network itself is down mid-flight —
// e.g. Wi-Fi not associated yet at boot. That failure must be treated like
// gateOK's own offline skip: owed, not recorded as a failed check, and run
// again — and only once — on reconnect.
func TestChecker_offlineMidCheckOwesUntilReconnect(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	bad.Close() // refuses every connection from here on

	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.src.ReleasesURL = bad.URL + "/releases"

	c.Start()
	c.wg.Wait()
	if gh.releases.Load() != 0 {
		t.Fatal("the bad address must be the one dialed, not the fake")
	}
	if v := c.Verdict(); v.Kind != Unknown {
		t.Fatalf("verdict = %+v, want nothing filled by an offline mid-check failure", v)
	}
	if err := c.LastError(); err != nil {
		t.Fatalf("lastErr = %v, want nil: an offline failure is owed, not recorded as a failed check", err)
	}

	// Reconnect: point the source at the working fake and let the owed
	// check run.
	c.src.ReleasesURL = gh.srv.URL + "/releases"
	c.RetryAfterReconnect()
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("%d requests to the fake, want exactly 1 after reconnect", n)
	}
	if v := c.Verdict(); v.Kind != Available || v.Latest == nil || v.Latest.Tag != "v1.1.0-rc3" {
		t.Fatalf("verdict = %+v after reconnect", v)
	}

	c.RetryAfterReconnect()
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("%d requests, want still 1: a second RetryAfterReconnect must do nothing", n)
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

// TestChecker_setChannelRerunsCheckWhenNewChannelUncached covers the case
// TestChecker_setChannelWhileBusyDoesNotStartSecond does not: a running
// check that cannot itself fill the channel switched to (here, a
// Store-managed Stable check fetches pak.json only, never Releases, so a
// switch to RC mid-check finds nothing cached when the check finishes and
// must trigger an actual second run — not just consume the flag).
func TestChecker_setChannelRerunsCheckWhenNewChannelUncached(t *testing.T) {
	gh := newFakeGitHub(t, releasesRC3, `{"version":"v1.0.25"}`)
	gh.pakGate = make(chan struct{})
	c := newTestChecker(t, firmware.KindNextUI, "v1.0.25", Stable, gh, "../../testdata/pakstore/single.db")
	c.Start()
	for gh.pak.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	c.SetChannel(RC) // nothing cached for RC; the running check only touches pak.json
	close(gh.pakGate)
	c.wg.Wait()
	if n := gh.releases.Load(); n != 1 {
		t.Fatalf("releases requests = %d, want 1 (the queued rerun, for RC)", n)
	}
	if n := gh.pak.Load(); n != 1 {
		t.Fatalf("pak.json requests = %d, want 1 (only the original Stable check)", n)
	}
	if v := c.Verdict(); v.Channel != RC || v.Latest == nil || v.Latest.Tag != "v1.1.0-rc3" {
		t.Fatalf("verdict = %+v, want RC filled by the rerun", v)
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

// TestChecker_archiveSaveCancelled covers the cancelled-download logging
// path added for the ARCHIVE save: ArchiveStatus still reports ArchiveFailed
// with Err = context.Canceled (the UI words that as "Download cancelled"),
// logged at Info rather than as a Warn "failed", since a user-initiated
// cancel is not a failure.
func TestChecker_archiveSaveCancelled(t *testing.T) {
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	body := goodMuxapp(t)
	started := make(chan struct{})
	block := make(chan struct{})
	asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
		w.Write(body)
	}))
	t.Cleanup(asset.Close)
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
	<-started
	c.CancelArchiveSave()
	close(block)
	c.wg.Wait()
	st := c.ArchiveStatus()
	if st.State != ArchiveFailed || !errors.Is(st.Err, context.Canceled) {
		t.Fatalf("archive status = %+v, want ArchiveFailed/context.Canceled", st)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// refusing is an address that refuses every connection.
func refusing(t *testing.T) string {
	t.Helper()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	bad.Close()
	return bad.URL
}

// TestChecker_githubFailureLeavesNetstateAlone: GitHub traffic never moves
// the app-wide online state. netstate's monitor probes itch.io, so a
// GitHub-only failure reported there would flip the app Offline, be flipped
// back Online by the probe, rerun the owed check, and fail again — forever.
func TestChecker_githubFailureLeavesNetstateAlone(t *testing.T) {
	netstate.ResetForTest()
	t.Cleanup(netstate.ResetForTest)
	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.src.ReleasesURL = refusing(t) + "/releases"
	c.Start()
	c.wg.Wait()
	if st := netstate.Current(); st.Status != netstate.StatusUnknown {
		t.Fatalf("netstate = %+v after a refused GitHub check, want untouched (unknown)", st)
	}

	// The ARCHIVE download client too.
	dir := archiveSetup(t)
	_, err := SaveToArchive(context.Background(), c.dl, "ua",
		Release{Tag: "v1.1.1", Asset: refusing(t), Digest: "sha256:00", Size: 10}, dir, nil)
	if err == nil {
		t.Fatal("want the refused download to fail")
	}
	if st := netstate.Current(); st.Status != netstate.StatusUnknown {
		t.Fatalf("netstate = %+v after a refused ARCHIVE download, want untouched (unknown)", st)
	}
}

// droppingGitHub hangs up on every request before answering: an
// offline-classified failure (EOF before any response) that can be counted.
func droppingGitHub(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &n
}

// TestChecker_reconnectRetryBudget: a check that fails offline is owed, but
// only one reconnect retry per launch; after that retry has itself failed
// offline nothing is owed. Check now still runs.
func TestChecker_reconnectRetryBudget(t *testing.T) {
	url, hits := droppingGitHub(t)
	gh := newFakeGitHub(t, releasesRC3, "")
	c := newTestChecker(t, firmware.KindMuOS, "v1.1.0-rc2", RC, gh, "")
	c.src.ReleasesURL = url + "/releases"

	c.Start()
	c.wg.Wait()
	afterLaunch := hits.Load()
	if afterLaunch == 0 {
		t.Fatal("the launch check must have reached the dropping server")
	}
	if netstate.Classify(c.LastError()).Offline() {
		t.Fatal("an owed failure must not be recorded")
	}

	c.RetryAfterReconnect()
	c.wg.Wait()
	afterRetry := hits.Load()
	if afterRetry == afterLaunch {
		t.Fatal("the first reconnect must run the owed check")
	}

	c.RetryAfterReconnect()
	c.wg.Wait()
	if n := hits.Load(); n != afterRetry {
		t.Fatalf("second reconnect made %d more request(s), want none: the retry budget is spent", n-afterRetry)
	}
	if err := c.LastError(); err == nil || !netstate.Classify(err).Offline() {
		t.Fatalf("LastError = %v, want the offline failure recorded once nothing is owed", err)
	}

	c.CheckNow()
	c.wg.Wait()
	if hits.Load() == afterRetry {
		t.Fatal("Check now must still run after the reconnect budget is spent")
	}
}

// The ARCHIVE status names the version it was started for, so the UI never
// shows one version's outcome under another.
func TestChecker_archiveStatusRecordsTag(t *testing.T) {
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	body := goodMuxapp(t)
	asset := serve(t, body)
	r := releaseFor(asset.URL, body, "v1.1.0-rc3")
	gh := newFakeGitHub(t, `[{"tag_name":"v1.1.0-rc3","prerelease":true,"assets":[{"name":"Itch-io.muOS.v1.1.0-rc3.muxapp","browser_download_url":"`+
		r.Asset+`","size":`+itoa(r.Size)+`,"digest":"`+r.Digest+`"}]}]`, "")
	var off atomic.Bool
	onlineForTest(t, &off)
	c := NewChecker(Config{Firmware: firmware.KindMuOS, Running: "v1.1.0-rc2", Channel: RC,
		StatePath: filepath.Join(t.TempDir(), "s.json"), ArchiveDir: filepath.Join(t.TempDir(), "ARCHIVE"),
		UserAgent: "ua", Source: testSource(gh.srv.URL)})
	c.Start()
	c.wg.Wait()
	c.StartArchiveSave()
	if st := c.ArchiveStatus(); st.Tag != "v1.1.0-rc3" {
		t.Fatalf("running status Tag = %q, want v1.1.0-rc3", st.Tag)
	}
	c.wg.Wait()
	if st := c.ArchiveStatus(); st.State != ArchiveDone || st.Tag != "v1.1.0-rc3" {
		t.Fatalf("finished status = %+v, want Done with Tag v1.1.0-rc3", st)
	}
}
