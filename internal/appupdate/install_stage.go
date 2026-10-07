package appupdate

import (
	"archive/zip"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// ErrDamaged: the downloaded pak is not something that may be installed.
var ErrDamaged = errors.New("the update is damaged")

func damaged(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDamaged, fmt.Sprintf(format, args...))
}

// unpackEntryLimit bounds any single extracted file, both by its declared
// size (the pre-pass below) and by what actually comes out of the
// decompressor (extractOne): a zip's declared UncompressedSize64 is just a
// header field and need not match the real stream, so only checking the
// declared size would let a mismatched entry extract past the limit
// unnoticed. It is a var, not the maxEntrySize const, so a test can lower it
// instead of generating a multi-hundred-MB entry.
var unpackEntryLimit uint64 = maxEntrySize

// UnpackPak extracts a NextUI pak zip into dir, which must not exist yet.
// Entries are checked with the same rules as a .muxapp: no absolute paths,
// "..", backslashes, links or special files, and bounded counts and sizes.
// If dir already exists, or anything below fails after dir was created,
// UnpackPak removes dir again so a failed unpack never leaves a partial
// staged folder behind.
func UnpackPak(zipPath, dir string) (files int, written int64, err error) {
	if _, statErr := os.Stat(dir); statErr == nil {
		return 0, 0, fmt.Errorf("%s already exists", dir)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return 0, 0, statErr
	}

	zr, zerr := zip.OpenReader(zipPath)
	if zerr != nil {
		return 0, 0, damaged("not a zip: %v", zerr)
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > maxEntries {
		return 0, 0, damaged("%d entries", len(zr.File))
	}
	var total uint64
	for _, f := range zr.File {
		n := f.Name
		switch {
		case n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, "\\"):
			return 0, 0, damaged("unsafe entry %q", n)
		case f.UncompressedSize64 > unpackEntryLimit:
			return 0, 0, damaged("entry %q is too large", n)
		}
		for _, seg := range strings.Split(n, "/") {
			if seg == ".." {
				return 0, 0, damaged("unsafe entry %q", n)
			}
		}
		if m := f.Mode(); m&fs.ModeSymlink != 0 || !(m.IsRegular() || m.IsDir()) {
			return 0, 0, damaged("entry %q is not a plain file or folder", n)
		}
		total += f.UncompressedSize64
	}
	if total > maxTotalSize {
		return 0, 0, damaged("%d bytes uncompressed", total)
	}

	if err = os.MkdirAll(dir, 0755); err != nil {
		return 0, 0, err
	}
	// From here dir exists and is ours: on any failure below, remove it
	// again rather than leave a half-unpacked folder behind.
	defer func() {
		if err == nil {
			return
		}
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			logger.Error("appupdate: clean up %s after failed unpack: %v", dir, rmErr)
			return
		}
		logger.Warn("appupdate: removed %s after a failed unpack: %v", dir, err)
	}()

	for _, f := range zr.File {
		dest := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.Mode().IsDir() {
			if mkErr := os.MkdirAll(dest, 0755); mkErr != nil {
				err = mkErr
				return files, written, err
			}
			continue
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			err = mkErr
			return files, written, err
		}
		n, exErr := extractOne(f, dest)
		written += n
		if exErr != nil {
			logger.Error("appupdate: unpack %s: %v", f.Name, exErr)
			err = exErr
			return files, written, err
		}
		files++
	}
	logger.Info("appupdate: unpacked %d files, %d bytes into %s", files, written, dir)
	return files, written, nil
}

func extractOne(f *zip.File, dest string) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, damaged("open %q: %v", f.Name, err)
	}
	defer rc.Close()
	mode := os.FileMode(0644)
	if f.Mode().Perm()&0111 != 0 || f.Name == "itchio" || f.Name == "launch.sh" {
		mode = 0755
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, int64(unpackEntryLimit)+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
	}
	// io.Copy stops silently, with a nil error, once the LimitReader above
	// is exhausted — so a stream that decompresses to more than declared
	// would otherwise extract truncated and still get reported as success.
	if uint64(n) > unpackEntryLimit {
		return n, damaged("entry %q decompresses past its declared size", f.Name)
	}
	// vfat ignores modes; other filesystems honour the umask, so set it.
	return n, os.Chmod(dest, mode)
}

// CheckStaged decides whether an unpacked pak may replace the live one.
func CheckStaged(dir, tag string) error {
	for _, f := range []string{"itchio", "launch.sh", "pak.json"} {
		if fi, err := os.Stat(filepath.Join(dir, f)); err != nil || !fi.Mode().IsRegular() {
			return damaged("%s is missing", f)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "pak.json"))
	if err != nil {
		return damaged("read pak.json: %v", err)
	}
	var pj struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return damaged("pak.json does not decode: %v", err)
	}
	if pj.Version != tag {
		return damaged("pak.json says %q, the release is %q", pj.Version, tag)
	}
	ef, err := elf.Open(filepath.Join(dir, "itchio"))
	if err != nil {
		return damaged("itchio is not an ELF binary: %v", err)
	}
	machine := ef.Machine
	ef.Close()
	if machine != elf.EM_AARCH64 {
		return damaged("itchio is built for %s, not aarch64", machine)
	}
	// The new launcher is what rolls a failed update back, so a launcher
	// that cannot even be parsed must never be installed.
	if out, err := exec.Command("/bin/sh", "-n", filepath.Join(dir, "launch.sh")).CombinedOutput(); err != nil {
		return damaged("launch.sh does not parse: %s", strings.TrimSpace(string(out)))
	}
	logger.Info("appupdate: staged pak %s passed every check", tag)
	return nil
}
