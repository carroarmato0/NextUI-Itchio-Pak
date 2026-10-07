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
