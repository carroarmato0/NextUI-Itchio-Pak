package appupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// withEntryLimit lowers unpackEntryLimit for the duration of a test, so an
// oversized entry can be exercised without a multi-hundred-MB zip.
func withEntryLimit(t *testing.T, n uint64) {
	t.Helper()
	old := unpackEntryLimit
	unpackEntryLimit = n
	t.Cleanup(func() { unpackEntryLimit = old })
}

// fakeELF is a 64-byte ELF header for machine, enough for debug/elf.
func fakeELF(t *testing.T, machine elf.Machine) []byte {
	t.Helper()
	var h elf.Header64
	copy(h.Ident[:], elf.ELFMAG)
	h.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	h.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	h.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	h.Type = uint16(elf.ET_EXEC)
	h.Machine = uint16(machine)
	h.Version = uint32(elf.EV_CURRENT)
	h.Ehsize = 64
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, &h); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zent struct {
	name string
	body []byte
	mode fs.FileMode
}

func pakZip(t *testing.T, ents ...zent) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ents {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if mode == 0 {
			mode = 0644
		}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(e.body)
	}
	zw.Close()
	return buf.Bytes()
}

func goodPak(t *testing.T, version string) []zent {
	return []zent{
		{name: "itchio", body: fakeELF(t, elf.EM_AARCH64), mode: 0755},
		{name: "launch.sh", body: []byte("#!/bin/sh\nexec ./itchio\n"), mode: 0755},
		{name: "pak.json", body: []byte(`{"name":"Itch-io","version":"` + version + `"}`)},
		{name: "assets/font.ttf", body: []byte("font")},
		{name: "lib/tg5040/libSDL2-2.0.so.0", body: []byte("lib")},
	}
}

func writeZip(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.zip")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallPaths(t *testing.T) {
	pak := "/mnt/SDCARD/Tools/tg5040/Itch-io.pak"
	for got, want := range map[string]string{
		StagedDir(pak): "/mnt/SDCARD/Tools/tg5040/.Itch-io.pak.staged",
		StagedZip(pak): "/mnt/SDCARD/Tools/tg5040/.Itch-io.pak.staged.zip",
		PrevDir(pak):   "/mnt/SDCARD/Tools/tg5040/.Itch-io.pak.prev",
		FailedDir(pak): "/mnt/SDCARD/Tools/tg5040/.Itch-io.pak.failed",
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

func TestUnpackAndCheck_ok(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "staged")
	n, size, err := UnpackPak(writeZip(t, pakZip(t, goodPak(t, "v1.1.0-rc5")...)), dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 || size <= 0 {
		t.Fatalf("unpacked %d files, %d bytes", n, size)
	}
	if fi, err := os.Stat(filepath.Join(dir, "itchio")); err != nil || fi.Mode().Perm()&0100 == 0 {
		t.Fatalf("itchio not executable: %v %v", fi, err)
	}
	if err := CheckStaged(dir, "v1.1.0-rc5"); err != nil {
		t.Fatal(err)
	}
}

func TestUnpack_rejectsUnsafeEntries(t *testing.T) {
	for name, ents := range map[string][]zent{
		"dotdot":    {{name: "../evil", body: []byte("x")}},
		"absolute":  {{name: "/etc/evil", body: []byte("x")}},
		"backslash": {{name: `a\b`, body: []byte("x")}},
		"symlink":   {{name: "link", body: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0777}},
	} {
		dir := filepath.Join(t.TempDir(), "staged")
		_, _, err := UnpackPak(writeZip(t, pakZip(t, ents...)), dir)
		if !errors.Is(err, ErrDamaged) {
			t.Errorf("%s: err = %v, want ErrDamaged", name, err)
		}
	}
}

func TestCheckStaged_rejects(t *testing.T) {
	type mutate func([]zent) []zent
	drop := func(n string) mutate {
		return func(es []zent) []zent {
			var out []zent
			for _, e := range es {
				if e.name != n {
					out = append(out, e)
				}
			}
			return out
		}
	}
	replace := func(n string, body []byte) mutate {
		return func(es []zent) []zent {
			for i := range es {
				if es[i].name == n {
					es[i].body = body
				}
			}
			return es
		}
	}
	cases := map[string]mutate{
		"no itchio":        drop("itchio"),
		"no launch.sh":     drop("launch.sh"),
		"no pak.json":      drop("pak.json"),
		"wrong version":    replace("pak.json", []byte(`{"version":"v1.1.0-rc4"}`)),
		"x86-64 binary":    replace("itchio", fakeELF(t, elf.EM_X86_64)),
		"not an ELF":       replace("itchio", []byte("#!/bin/sh\n")),
		"launch.sh syntax": replace("launch.sh", []byte("#!/bin/sh\nif then fi fi\n")),
	}
	for name, m := range cases {
		dir := filepath.Join(t.TempDir(), "staged")
		if _, _, err := UnpackPak(writeZip(t, pakZip(t, m(goodPak(t, "v1.1.0-rc5"))...)), dir); err != nil {
			t.Fatalf("%s: unpack: %v", name, err)
		}
		if err := CheckStaged(dir, "v1.1.0-rc5"); !errors.Is(err, ErrDamaged) {
			t.Errorf("%s: err = %v, want ErrDamaged", name, err)
		}
	}
}

// TestExtractOne_rejectsOversizedActualContent exercises extractOne in
// isolation: a real zip entry whose actual decompressed body (11 bytes)
// exceeds a lowered unpackEntryLimit (4). Before the fix, io.Copy would
// stop silently at the LimitReader's bound and extractOne would report
// success having written a truncated file; it must now report ErrDamaged
// instead.
func TestExtractOne_rejectsOversizedActualContent(t *testing.T) {
	withEntryLimit(t, 4)

	zr, err := zip.OpenReader(writeZip(t, pakZip(t, zent{name: "big", body: []byte("hello world")})))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	dest := filepath.Join(t.TempDir(), "big")
	n, err := extractOne(zr.File[0], dest)
	if !errors.Is(err, ErrDamaged) {
		t.Fatalf("err = %v, want ErrDamaged", err)
	}
	if uint64(n) <= unpackEntryLimit {
		t.Fatalf("n = %d, want more than %d to prove the limit doesn't just silently truncate", n, unpackEntryLimit)
	}
}

// TestUnpack_rejectsOversizedEntry runs the same oversized entry through the
// full UnpackPak pipeline (caught here by the declared-size pre-pass, since
// a zip built by the standard library always declares the real size) and
// checks that the staged dir is not left behind.
func TestUnpack_rejectsOversizedEntry(t *testing.T) {
	withEntryLimit(t, 4)

	dir := filepath.Join(t.TempDir(), "staged")
	_, _, err := UnpackPak(writeZip(t, pakZip(t, zent{name: "big", body: []byte("hello world")})), dir)
	if !errors.Is(err, ErrDamaged) {
		t.Fatalf("err = %v, want ErrDamaged", err)
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("dir %s should not exist after a rejected oversized entry, stat err = %v", dir, statErr)
	}
}

// TestUnpack_rejectsExistingDir covers the "dir must not exist yet"
// precondition: UnpackPak must refuse to run at all, and must not touch
// whatever is already there.
func TestUnpack_rejectsExistingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "staged")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "keepme")
	if err := os.WriteFile(marker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := UnpackPak(writeZip(t, pakZip(t, goodPak(t, "v1.1.0-rc5")...)), dir)
	if err == nil || errors.Is(err, ErrDamaged) {
		t.Fatalf("err = %v, want a plain already-exists error, not ErrDamaged", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("existing dir's contents were touched: %v", statErr)
	}
}

// TestUnpack_cleansUpOnFailure forces a failure partway through extraction
// (the second entry's parent path is already a plain file, so MkdirAll must
// fail) and checks that the partially-unpacked dir is removed, not left
// behind half-made.
func TestUnpack_cleansUpOnFailure(t *testing.T) {
	ents := []zent{
		{name: "a", body: []byte("file")},
		{name: "a/b", body: []byte("x")},
	}
	dir := filepath.Join(t.TempDir(), "staged")
	_, _, err := UnpackPak(writeZip(t, pakZip(t, ents...)), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, statErr := os.Stat(dir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("dir %s should not exist after a failed unpack, stat err = %v", dir, statErr)
	}
}

func stageSetup(t *testing.T) string {
	t.Helper()
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	pak := filepath.Join(t.TempDir(), "Tools", "tg5040", "Itch-io.pak")
	if err := os.MkdirAll(pak, 0755); err != nil {
		t.Fatal(err)
	}
	return pak
}

func nuiReleaseFor(url string, body []byte, tag string) Release {
	sum := sha256.Sum256(body)
	return Release{Tag: tag, NextUIAsset: url, NextUIDigest: "sha256:" + hex.EncodeToString(sum[:]), NextUISize: int64(len(body))}
}

func assertNothingStaged(t *testing.T, pak string) {
	t.Helper()
	for _, p := range []string{StagedDir(pak), StagedZip(pak), partfile.PathFor(StagedZip(pak))} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must not survive", p)
		}
	}
}

func TestStageNextUI_ok(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	srv := serve(t, body)
	var phases []InstallPhase
	err := StageNextUI(context.Background(), srv.Client(), "test", nuiReleaseFor(srv.URL, body, "v1.1.0-rc5"), pak,
		func(p InstallPhase) { phases = append(phases, p) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(StagedDir(pak), completeMarker)); err != nil {
		t.Fatal("no .complete marker")
	}
	if _, err := os.Stat(StagedZip(pak)); !os.IsNotExist(err) {
		t.Fatal("the zip must be deleted after unpacking")
	}
	if len(phases) != 3 || phases[0] != PhaseDownloading || phases[2] != PhaseUnpacking {
		t.Fatalf("phases = %v", phases)
	}
}

func TestStageNextUI_replacesAnOldStagedFolder(t *testing.T) {
	pak := stageSetup(t)
	os.MkdirAll(StagedDir(pak), 0755)
	os.WriteFile(filepath.Join(StagedDir(pak), "stale"), []byte("x"), 0644)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	srv := serve(t, body)
	if err := StageNextUI(context.Background(), srv.Client(), "test", nuiReleaseFor(srv.URL, body, "v1.1.0-rc5"), pak, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(StagedDir(pak), "stale")); !os.IsNotExist(err) {
		t.Fatal("a leftover staged folder must be replaced, not merged into")
	}
}

func TestStageNextUI_digestMismatch(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	srv := serve(t, body)
	rel := nuiReleaseFor(srv.URL, body, "v1.1.0-rc5")
	rel.NextUIDigest = "sha256:" + strings.Repeat("00", 32)
	if err := StageNextUI(context.Background(), srv.Client(), "test", rel, pak, nil, nil); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	assertNothingStaged(t, pak)
}

func TestStageNextUI_damagedPakLeavesNothing(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc4")...) // version disagrees with the tag
	srv := serve(t, body)
	if err := StageNextUI(context.Background(), srv.Client(), "test", nuiReleaseFor(srv.URL, body, "v1.1.0-rc5"), pak, nil, nil); !errors.Is(err, ErrDamaged) {
		t.Fatalf("err = %v, want ErrDamaged", err)
	}
	assertNothingStaged(t, pak)
}

func TestStageNextUI_cancel(t *testing.T) {
	pak := stageSetup(t)
	body := pakZip(t, goodPak(t, "v1.1.0-rc5")...)
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body[:10])
		w.(http.Flusher).Flush()
		cancel()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	if err := StageNextUI(ctx, srv.Client(), "test", nuiReleaseFor(srv.URL, body, "v1.1.0-rc5"), pak, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertNothingStaged(t, pak)
}

func TestStageNextUI_notInstallable(t *testing.T) {
	pak := stageSetup(t)
	if err := StageNextUI(context.Background(), http.DefaultClient, "test", Release{Tag: "v1.1.0-rc5"}, pak, nil, nil); err == nil {
		t.Fatal("a release without a NextUI asset must be refused")
	}
}
