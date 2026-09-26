package partfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func reset(t *testing.T) string {
	t.Helper()
	mu.Lock()
	active = map[string]bool{}
	journal = ""
	mu.Unlock()
	j := filepath.Join(t.TempDir(), "partials.json")
	SetJournal(j)
	t.Cleanup(func() { SetJournal("") })
	return j
}

func TestPathFor(t *testing.T) {
	if got := PathFor("/r/Game Boy (GB)/Tobu.gb"); got != "/r/Game Boy (GB)/.Tobu.gb.itchio-part" {
		t.Fatalf("PathFor = %q", got)
	}
}

func TestCommit_replacesDestAtomically(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	f, err := Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("NEW"))
	if b, _ := os.ReadFile(dest); string(b) != "OLD" {
		t.Fatal("dest changed before Commit")
	}
	if err := f.Commit(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW" {
		t.Fatalf("dest = %q after Commit", b)
	}
	if _, err := os.Stat(PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left after Commit")
	}
}

// A Sync failure must not let Commit rename an unflushed partial over dest,
// even when the subsequent Close would have succeeded on its own — i.e. the
// fsync check has to run and be checked, not just piggyback on Close's
// existing error handling. Simulated by dup2'ing the file's fd onto a pipe's
// write end: fsync(2) on a pipe fails with EINVAL, but close(2) on it is
// perfectly fine, so this isolates a Sync-specific failure from a Close one.
func TestCommit_syncFailureLeavesDestUntouched(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	f, err := Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("NEW"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	fd := int(f.File.Fd())
	if err := syscall.Dup2(int(w.Fd()), fd); err != nil {
		t.Fatalf("dup2: %v", err)
	}
	w.Close() // fd now solely refers to the pipe's write end

	if err := f.Commit(); err == nil {
		t.Fatal("Commit returned nil despite Sync failure")
	}
	if b, _ := os.ReadFile(dest); string(b) != "OLD" {
		t.Fatalf("dest changed despite Sync failure: %q", b)
	}
	if _, err := os.Stat(PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left after failed Commit")
	}
	f.Abort() // no-op: done is already set, but confirms it doesn't panic
}

func TestAbort_keepsDestAndRemovesPart(t *testing.T) {
	reset(t)
	dest := filepath.Join(t.TempDir(), "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	f, _ := Create(dest)
	f.Write([]byte("PARTIAL"))
	f.Abort()
	f.Abort() // idempotent
	if b, _ := os.ReadFile(dest); string(b) != "OLD" {
		t.Fatal("Abort touched dest")
	}
	if _, err := os.Stat(PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left after Abort")
	}
}

// A crash between Create and Commit leaves the file and its journal entry.
func TestRecover_deletesJournalledLeftovers(t *testing.T) {
	reset(t)
	chosen := filepath.Join(t.TempDir(), "picked by hand")
	os.MkdirAll(chosen, 0755)
	f, _ := Create(filepath.Join(chosen, "g.gb"))
	f.Write([]byte("PARTIAL"))
	f.File.Close() // the process dies here: no Commit, no Abort

	// Next launch: the in-process set is empty again.
	mu.Lock()
	active = map[string]bool{}
	mu.Unlock()
	if n := Recover(); n != 1 {
		t.Fatalf("Recover removed %d, want 1", n)
	}
	if _, err := os.Stat(PathFor(filepath.Join(chosen, "g.gb"))); !os.IsNotExist(err) {
		t.Fatal("leftover still there")
	}
	if n := Recover(); n != 0 {
		t.Fatalf("journal not cleared: second Recover removed %d", n)
	}
}

func TestRecover_corruptJournalIsHarmless(t *testing.T) {
	j := reset(t)
	os.WriteFile(j, []byte("{not json"), 0644)
	if n := Recover(); n != 0 {
		t.Fatalf("Recover = %d", n)
	}
}

func TestSweep_onlyOurStaleFiles(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	stale := filepath.Join(dir, ".old.gb"+Suffix)
	os.WriteFile(stale, []byte("x"), 0644)
	other := filepath.Join(dir, ".other.part")
	os.WriteFile(other, []byte("x"), 0644)
	tmp := filepath.Join(dir, "config.json.tmp")
	os.WriteFile(tmp, []byte("x"), 0644)
	running, _ := Create(filepath.Join(dir, "now.gb"))
	defer running.Abort()

	if n := Sweep([]string{dir, filepath.Join(dir, "missing")}); n != 1 {
		t.Fatalf("Sweep removed %d, want 1", n)
	}
	for _, keep := range []string{other, tmp, PathFor(filepath.Join(dir, "now.gb"))} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("Sweep removed %s", keep)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale partial file survived")
	}
}
