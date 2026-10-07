package appupdate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
)

// installChecker is a NextUI checker whose cached RC result is rel.
func installChecker(t *testing.T, rel *Release, pakDir string, failed *Pending) *Checker {
	t.Helper()
	c := NewChecker(Config{Firmware: firmware.KindNextUI, Running: "v1.1.0-rc4", Channel: RC,
		PakDir: pakDir, Failed: failed})
	c.mu.Lock()
	c.st.Latest[RC] = rel
	c.mu.Unlock()
	return c
}

func waitInstall(t *testing.T, c *Checker) InstallStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := c.InstallStatus(); s.State != InstallRunning {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("install did not finish")
	return InstallStatus{}
}

func TestChecker_viaInstallNeedsAPakDir(t *testing.T) {
	r := nuiRel("v1.1.0-rc5")
	if got := installChecker(t, r, "", nil).Verdict().Via; got != ViaReleasePage {
		t.Fatalf("no PakDir: via %s, want release-page", got)
	}
	if got := installChecker(t, r, "/x/Itch-io.pak", nil).Verdict().Via; got != ViaInstall {
		t.Fatalf("with PakDir: via %s, want install", got)
	}
}

func TestChecker_installStages(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	srv := serve(t, body)
	r := nuiReleaseFor(srv.URL, body, "v1.1.0-rc5")
	c := installChecker(t, &r, pak, nil)
	c.dl = srv.Client()
	c.StartInstall()
	if s := waitInstall(t, c); s.State != InstallStaged || s.Tag != "v1.1.0-rc5" {
		t.Fatalf("status = %+v", s)
	}
	if c.RestartRequested() {
		t.Fatal("restart is only on request")
	}
	c.RequestRestart()
	if !c.RestartRequested() {
		t.Fatal("RequestRestart not recorded")
	}
}

func TestChecker_installCancel(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	started := make(chan struct{})
	srv := serveSlow(t, body, started)
	r := nuiReleaseFor(srv.URL, body, "v1.1.0-rc5")
	c := installChecker(t, &r, pak, nil)
	c.dl = srv.Client()
	c.StartInstall()
	<-started
	c.CancelInstall()
	s := waitInstall(t, c)
	if s.State != InstallFailed || s.Err == nil || !errorsIsCanceled(s.Err) {
		t.Fatalf("status = %+v", s)
	}
	assertNothingStaged(t, pak)
}

func TestChecker_stagedFromAnEarlierRunIsReported(t *testing.T) {
	pak := stageSetup(t)
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte("v1.1.0-rc5\n"), 0644)
	c := installChecker(t, nuiRel("v1.1.0-rc5"), pak, nil)
	if s := c.InstallStatus(); s.State != InstallStaged || s.Tag != "v1.1.0-rc5" {
		t.Fatalf("status = %+v", s)
	}
}

func TestChecker_failedInstallSilencesNotice(t *testing.T) {
	failed := &Pending{From: "v1.1.0-rc4", To: "v1.1.0-rc5"}
	c := installChecker(t, nuiRel("v1.1.0-rc5"), "/x/Itch-io.pak", failed)
	v, due := c.PendingNotice()
	if due {
		t.Fatal("the notice must not return for a version that failed to start")
	}
	if v.FailedInstall != "v1.1.0-rc5" || v.FailedFrom != "v1.1.0-rc4" {
		t.Fatalf("verdict = %+v", v)
	}
	// A newer version is announced as usual.
	c.mu.Lock()
	c.st.Latest[RC] = nuiRel("v1.1.0-rc6")
	c.mu.Unlock()
	if v, due := c.PendingNotice(); !due || v.FailedInstall != "" {
		t.Fatalf("rc6: due=%v verdict=%+v", due, v)
	}
}

// TestChecker_failedInstallClearsWhenItNowRuns: the version recorded as a
// failed install is retried, and this time it runs. The next launch must
// stop silencing the notice for it, in memory and on disk.
func TestChecker_failedInstallClearsWhenItNowRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	// Boot where rc5 failed to start and launch.sh rolled back to rc4.
	NewChecker(Config{Firmware: firmware.KindNextUI, Running: "v1.1.0-rc4", Channel: RC, StatePath: path,
		Failed: &Pending{From: "v1.1.0-rc4", To: "v1.1.0-rc5"}})

	// The user retried, and this boot is rc5 running successfully.
	c := NewChecker(Config{Firmware: firmware.KindNextUI, Running: "v1.1.0-rc5", Channel: RC, StatePath: path})
	c.mu.Lock()
	c.st.Latest[RC] = nuiRel("v1.1.0-rc5")
	failedStill := c.st.FailedInstall
	c.mu.Unlock()
	if failedStill != "" {
		t.Fatalf("state still has FailedInstall = %q now that rc5 runs", failedStill)
	}
	if v := c.Verdict(); v.FailedInstall != "" {
		t.Fatalf("Verdict().FailedInstall = %q, want empty", v.FailedInstall)
	}
	if st := LoadState(path); st.FailedInstall != "" || st.FailedInstallFrom != "" {
		t.Fatalf("persisted state still has a failed-install record: %+v", st)
	}
}

// TestChecker_failedInstallRecordSurvivesARestart: the failed-install record
// written on an earlier boot (while the rolled-back version is still what's
// running) must still be there, with both ends of it, on the next boot.
func TestChecker_failedInstallRecordSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := &State{FailedInstall: "v1.1.0-rc5", FailedInstallFrom: "v1.1.0-rc4"}
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	c := NewChecker(Config{Firmware: firmware.KindNextUI, Running: "v1.1.0-rc4", Channel: RC, StatePath: path})
	c.mu.Lock()
	c.st.Latest[RC] = nuiRel("v1.1.0-rc5")
	c.mu.Unlock()

	v, due := c.PendingNotice()
	if due {
		t.Fatal("must not notify for the version that failed to start")
	}
	if v.FailedInstall != "v1.1.0-rc5" || v.FailedFrom != "v1.1.0-rc4" {
		t.Fatalf("verdict = %+v", v)
	}
}

// TestChecker_newCheckerIgnoresStagedOlderThanRunning: NewChecker must not
// report an install that was staged earlier but is no longer newer than what
// is actually running (the §StagedNewer guard against a silent downgrade).
func TestChecker_newCheckerIgnoresStagedOlderThanRunning(t *testing.T) {
	pak := stageSetup(t)
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte("v1.1.0-rc5\n"), 0644)

	// Running is already past what got staged.
	c := NewChecker(Config{Firmware: firmware.KindNextUI, Running: "v1.1.0-rc6", Channel: RC, PakDir: pak})
	if s := c.InstallStatus(); s.State == InstallStaged {
		t.Fatalf("a staged tag not newer than running must not be reported: status = %+v", s)
	}
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("the stale stage must be discarded")
	}
}

// TestChecker_startInstallNoopWhenAlreadyStagedSameTag: a second StartInstall
// for the same tag that is already staged must not touch it.
func TestChecker_startInstallNoopWhenAlreadyStagedSameTag(t *testing.T) {
	pak := stageSetup(t)
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte("v1.1.0-rc5\n"), 0644)
	c := installChecker(t, nuiRel("v1.1.0-rc5"), pak, nil)
	if s := c.InstallStatus(); s.State != InstallStaged || s.Tag != "v1.1.0-rc5" {
		t.Fatalf("precondition: status = %+v", s)
	}

	c.StartInstall()
	c.wg.Wait() // no-op: nothing was ever added to wg

	if s := c.InstallStatus(); s.State != InstallStaged || s.Tag != "v1.1.0-rc5" {
		t.Fatalf("status changed: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(StagedDir(pak), completeMarker)); err != nil {
		t.Fatal(".complete marker must survive a no-op StartInstall")
	}
}

// TestChecker_startInstallRestagesWhenNewerIsAvailable: a newer release than
// what is already staged must still be staged over it.
func TestChecker_startInstallRestagesWhenNewerIsAvailable(t *testing.T) {
	pak := stageSetup(t)
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte("v1.1.0-rc5\n"), 0644)

	body := pakZip(t, goodPak(t, "v1.1.0-rc6")...)
	srv := serve(t, body)
	r := nuiReleaseFor(srv.URL, body, "v1.1.0-rc6")
	c := installChecker(t, &r, pak, nil)
	c.dl = srv.Client()

	if s := c.InstallStatus(); s.State != InstallStaged || s.Tag != "v1.1.0-rc5" {
		t.Fatalf("precondition: status = %+v", s)
	}
	c.StartInstall()
	if s := waitInstall(t, c); s.State != InstallStaged || s.Tag != "v1.1.0-rc6" {
		t.Fatalf("status = %+v", s)
	}
}

// TestChecker_requestRestartNoopWhenNotStaged: nothing to restart into yet.
func TestChecker_requestRestartNoopWhenNotStaged(t *testing.T) {
	c := installChecker(t, nuiRel("v1.1.0-rc5"), "/x/Itch-io.pak", nil)
	if c.InstallStatus().State == InstallStaged {
		t.Fatal("precondition: must not be staged")
	}
	c.RequestRestart()
	if c.RestartRequested() {
		t.Fatal("RequestRestart must be a no-op when nothing is staged")
	}
}

func errorsIsCanceled(err error) bool { return errors.Is(err, context.Canceled) }

// serveSlow sends the first bytes, signals started, then stalls until the
// request is cancelled.
func serveSlow(t *testing.T, body []byte, started chan<- struct{}) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body[:10])
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}
