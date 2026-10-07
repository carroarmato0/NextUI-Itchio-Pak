package appupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func applySetup(t *testing.T) (pak, data string) {
	t.Helper()
	root := t.TempDir()
	pak = filepath.Join(root, "Tools", "tg5040", "Itch-io.pak")
	data = filepath.Join(root, "data")
	for _, d := range []string{pak, data, StagedDir(pak)} {
		os.MkdirAll(d, 0755)
	}
	os.WriteFile(filepath.Join(pak, "version"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(StagedDir(pak), "version"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte("v1.1.0-rc5\n"), 0644)
	return pak, data
}

func readVersion(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "version"))
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	return string(b)
}

func TestStagedReady(t *testing.T) {
	pak, _ := applySetup(t)
	if tag, ok := StagedReady(pak); !ok || tag != "v1.1.0-rc5" {
		t.Fatalf("StagedReady = %q %v", tag, ok)
	}
	os.Remove(filepath.Join(StagedDir(pak), completeMarker))
	if _, ok := StagedReady(pak); ok {
		t.Fatal("a staged folder without .complete is not ready")
	}
	DiscardIncompleteStaged(pak)
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("an incomplete staged folder must be deleted")
	}
}

func TestSwapStaged_ok(t *testing.T) {
	pak, data := applySetup(t)
	os.MkdirAll(PrevDir(pak), 0755) // left from an earlier update
	if err := SwapStaged(pak, data, "v1.1.0-rc4"); err != nil {
		t.Fatal(err)
	}
	if got := readVersion(t, pak); got != "new" {
		t.Fatalf("live = %q, want new", got)
	}
	if got := readVersion(t, PrevDir(pak)); got != "old" {
		t.Fatalf(".prev = %q, want old", got)
	}
	var p Pending
	b, err := os.ReadFile(filepath.Join(data, pendingFile))
	if err != nil || json.Unmarshal(b, &p) != nil {
		t.Fatalf("pending file: %v", err)
	}
	if p.From != "v1.1.0-rc4" || p.To != "v1.1.0-rc5" || p.At.IsZero() {
		t.Fatalf("pending = %+v", p)
	}
}

func TestSwapStaged_notReady(t *testing.T) {
	pak, data := applySetup(t)
	os.Remove(filepath.Join(StagedDir(pak), completeMarker))
	if err := SwapStaged(pak, data, "v1.1.0-rc4"); err == nil {
		t.Fatal("an incomplete stage must not be applied")
	}
	if got := readVersion(t, pak); got != "old" {
		t.Fatal("the live folder must be untouched")
	}
}

func TestSwapStaged_secondRenameFails(t *testing.T) {
	pak, data := applySetup(t)
	calls := 0
	rename = func(a, b string) error {
		calls++
		if calls == 2 {
			return errors.New("injected")
		}
		return os.Rename(a, b)
	}
	t.Cleanup(func() { rename = os.Rename })
	if err := SwapStaged(pak, data, "v1.1.0-rc4"); err == nil {
		t.Fatal("want the injected error")
	}
	if got := readVersion(t, pak); got != "old" {
		t.Fatalf("live = %q: the first rename must be undone", got)
	}
	if _, err := os.Stat(filepath.Join(data, pendingFile)); !os.IsNotExist(err) {
		t.Fatal("no pending file may survive a failed swap")
	}
}

func stagedNewerSetup(t *testing.T, tag string) string {
	t.Helper()
	pak := filepath.Join(t.TempDir(), "Tools", "tg5040", "Itch-io.pak")
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), completeMarker), []byte(tag+"\n"), 0644)
	return pak
}

func TestStagedNewer_newer(t *testing.T) {
	pak := stagedNewerSetup(t, "v1.1.0-rc5")
	tag, ok := StagedNewer(pak, "v1.1.0-rc4")
	if !ok || tag != "v1.1.0-rc5" {
		t.Fatalf("StagedNewer = %q %v, want v1.1.0-rc5 true", tag, ok)
	}
	if _, err := os.Stat(StagedDir(pak)); err != nil {
		t.Fatal("a genuinely newer stage must be left in place")
	}
}

func TestStagedNewer_equal(t *testing.T) {
	pak := stagedNewerSetup(t, "v1.1.0-rc5")
	if _, ok := StagedNewer(pak, "v1.1.0-rc5"); ok {
		t.Fatal("a staged version equal to running must not be applied")
	}
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("a non-newer stage must be discarded")
	}
}

func TestStagedNewer_older(t *testing.T) {
	// Simulates v1.1.0 staging v1.1.1, the user picking Later, and the Pak
	// Store (or a side-load) putting v1.1.2 in place before the restart.
	pak := stagedNewerSetup(t, "v1.1.1")
	if _, ok := StagedNewer(pak, "v1.1.2"); ok {
		t.Fatal("a staged version older than running must not be applied (would be a silent downgrade)")
	}
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("a stage older than running must be discarded")
	}
}

func TestStagedNewer_runningUnparseable(t *testing.T) {
	pak := stagedNewerSetup(t, "v1.1.0-rc5")
	if _, ok := StagedNewer(pak, "dev"); ok {
		t.Fatal("a dev build must never apply a staged update")
	}
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("the stage must still be discarded when running does not parse")
	}
}

func TestDiscardIncompleteStaged_removesLeftoverZip(t *testing.T) {
	pak := filepath.Join(t.TempDir(), "Tools", "tg5040", "Itch-io.pak")
	os.MkdirAll(filepath.Dir(pak), 0755)
	os.WriteFile(StagedZip(pak), []byte("partial zip"), 0644)
	DiscardIncompleteStaged(pak)
	if _, err := os.Stat(StagedZip(pak)); !os.IsNotExist(err) {
		t.Fatal("a zip left by a crash mid-unpack must be removed")
	}
}

func TestDiscardIncompleteStaged_removesZipAlongsideIncompleteDir(t *testing.T) {
	pak, _ := applySetup(t)
	os.Remove(filepath.Join(StagedDir(pak), completeMarker)) // incomplete
	os.WriteFile(StagedZip(pak), []byte("partial zip"), 0644)
	DiscardIncompleteStaged(pak)
	if _, err := os.Stat(StagedZip(pak)); !os.IsNotExist(err) {
		t.Fatal("the zip must be removed")
	}
	if _, err := os.Stat(StagedDir(pak)); !os.IsNotExist(err) {
		t.Fatal("the incomplete dir must still be removed")
	}
}

func TestConfirmStarted_pendingRemoveFails(t *testing.T) {
	pak, data := applySetup(t)
	if err := SwapStaged(pak, data, "v1.1.0-rc4"); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(FailedDir(pak), 0755)

	// Replace update_pending.json with a non-empty directory: both reading
	// and removing it fail, standing in for "removing it failed".
	pendingPath := filepath.Join(data, pendingFile)
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pendingPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pendingPath, "x"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	ConfirmStarted(pak, data).Wait()

	if _, err := os.Stat(PrevDir(pak)); err != nil {
		t.Fatal(".prev must survive when the pending file could not be cleared: a later rollback needs it")
	}
	if _, err := os.Stat(FailedDir(pak)); err != nil {
		t.Fatal(".failed must survive too")
	}
}

func TestConfirmStarted(t *testing.T) {
	pak, data := applySetup(t)
	SwapStaged(pak, data, "v1.1.0-rc4")
	os.MkdirAll(FailedDir(pak), 0755)
	ConfirmStarted(pak, data).Wait()
	for _, p := range []string{filepath.Join(data, pendingFile), PrevDir(pak), FailedDir(pak)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must be gone after a confirmed start", p)
		}
	}
}

func TestTakeFailed(t *testing.T) {
	_, data := applySetup(t)
	if _, ok := TakeFailed(data); ok {
		t.Fatal("nothing failed yet")
	}
	os.WriteFile(filepath.Join(data, failedFile), []byte(`{"from":"v1.1.0-rc4","to":"v1.1.0-rc5"}`), 0644)
	p, ok := TakeFailed(data)
	if !ok || p.From != "v1.1.0-rc4" || p.To != "v1.1.0-rc5" {
		t.Fatalf("TakeFailed = %+v %v", p, ok)
	}
	if _, err := os.Stat(filepath.Join(data, failedFile)); !os.IsNotExist(err) {
		t.Fatal("update_failed.json is read once")
	}
}

func TestTakeLauncherLog(t *testing.T) {
	_, data := applySetup(t)
	os.WriteFile(filepath.Join(data, launcherLogFile), []byte("rolled back\n"), 0644)
	TakeLauncherLog(data)
	if _, err := os.Stat(filepath.Join(data, launcherLogFile)); !os.IsNotExist(err) {
		t.Fatal("the launcher log is copied once")
	}
}
