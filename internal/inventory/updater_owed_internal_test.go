package inventory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// The reconnect that runs a postponed check can fire at any moment on the
// netstate goroutine. If the check reads "offline" before it marks itself
// owed, a reconnect landing between the two finds nothing owed and the check
// is lost until something else triggers one. So owed must already be set
// whenever the updater looks at, or causes, the offline state. Each test
// records owed at the exact moment the network state is read or reported —
// the window a reconnect would land in — rather than racing a real one.

func withOfflineNow(t *testing.T, fn func() bool) {
	t.Helper()
	old := offlineNow
	offlineNow = fn
	t.Cleanup(func() { offlineNow = old })
}

func withReportNetwork(t *testing.T, fn func(error)) {
	t.Helper()
	old := reportNetwork
	reportNetwork = fn
	t.Cleanup(func() { reportNetwork = old })
}

func newOwedTestService(t *testing.T, base string) *UpdateService {
	t.Helper()
	dir := t.TempDir()
	inv := &Inventory{Entries: make(map[string]*Entry)}
	rom := filepath.Join(dir, "a.gb")
	os.WriteFile(rom, []byte("ROM"), 0644)
	inv.Add(base+"/a", Entry{Title: "a", IsFree: true},
		DownloadedFile{Filename: "a.gb", DestPath: rom, DownloadedAt: time.Now()})
	return NewUpdateService(inv, filepath.Join(dir, "inventory.json"),
		itchio.NewClientWithBaseAndButler(base, base), nil)
}

func TestUpdateService_owedSetBeforeOfflineIsRead(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	s := newOwedTestService(t, "http://127.0.0.1:1")
	var owedAtRead []bool
	withOfflineNow(t, func() bool {
		owedAtRead = append(owedAtRead, s.owed.Load())
		return true
	})
	s.runCheck()
	if len(owedAtRead) == 0 || !owedAtRead[0] {
		t.Fatalf("owed at the offline read = %v; want true, or a reconnect in between loses the check", owedAtRead)
	}
	if !s.owed.Load() {
		t.Fatal("check abandoned offline but not owed")
	}
}

func TestUpdateService_postponeIfOffline(t *testing.T) {
	s := newOwedTestService(t, "http://127.0.0.1:1")
	var owedAtRead bool
	withOfflineNow(t, func() bool { owedAtRead = s.owed.Load(); return true })
	if !s.postponeIfOffline() || !owedAtRead || !s.owed.Load() {
		t.Fatalf("offline: postponed/owed-at-read/owed = %v/%v/%v, want all true",
			true, owedAtRead, s.owed.Load())
	}
	// Online: the check goes ahead, so nothing is owed any more.
	withOfflineNow(t, func() bool { return false })
	if s.postponeIfOffline() || s.owed.Load() {
		t.Fatalf("online: postponed=true or still owed")
	}
}

// A request failing offline-class is reported to netstate, which may be the
// offline transition itself; owed must be set by then.
func TestUpdateService_owedSetBeforeReport(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	s := newOwedTestService(t, "http://127.0.0.1:1") // nothing listens: refused
	withOfflineNow(t, func() bool { return false })
	reported, owedAtReport := false, false
	withReportNetwork(t, func(err error) {
		reported, owedAtReport = true, s.owed.Load()
		netstate.Report(err)
	})
	s.runCheck()
	if !reported {
		t.Fatal("offline-class failure was not reported")
	}
	if !owedAtReport {
		t.Fatal("owed was not set when the failure was reported")
	}
}
