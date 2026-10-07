package appupdate

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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
