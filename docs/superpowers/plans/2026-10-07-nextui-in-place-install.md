# NextUI In-Place Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On NextUI, let Settings → App updates download, verify, stage and install a newer Itch-io release in place, with an automatic rollback when the new version fails to start.

**Architecture:** `internal/appupdate` gains a staging step (download → digest → unpack → checks → `.complete`), a swap (`SwapStaged`: pending file, two renames) and start-up bookkeeping (confirm / failed report). The binary execs the new `launch.sh`; the new `launch.sh` waits for the binary while an update is pending and rolls back if it never confirms. A debug override points the release check at a host fixture server so the whole path can be tested on the Brick before two releases carry the feature.

**Tech Stack:** Go 1.27 (`archive/zip`, `debug/elf`, `syscall.Exec`), POSIX `sh` (busybox ash on device, dash in tests), SDL2 UI in `internal/ui`.

**Spec:** `docs/superpowers/specs/2026-10-07-nextui-in-place-install-design.md`

## Global Constraints

- Work in worktree `.worktrees/1.1-nextui-install`, branch `feature/1.1-nextui-install` (from `release/1.1.0`).
- Run tests with `./scripts/test.sh` (containerised). It runs `go test -race -tags headless ./...`, then non-headless `./internal/ui`, `launch_test.sh`, shellcheck and the palette audit. A single package: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless ./internal/appupdate/` (substitute `docker` if podman is absent).
- `internal/ui` files are `//go:build !headless`; their tests only run in the non-headless pass of `test.sh`.
- Every new code path logs through `internal/logger` (`logger.Info/Warn/Error/Debug`), never `fmt.Println`/`log.Printf`. Log file I/O, renames, HTTP and goroutine start/finish.
- No colour literals in drawing code; colours come from `r.Theme`.
- Never log `%v` of a download error directly — it can embed a signed URL. Use `netstate.Detail(err)`.
- Asset name: `Itch-io.NextUI.<tag>.pak.zip`. Sibling folders of the live pak: `.<name>.staged.zip`, `.<name>.staged/`, `.<name>.prev/`, `.<name>.failed/`, where `<name>` is the live folder's base name (normally `Itch-io.pak`).
- Data-dir files (NextUI `$HOME` = `env.DataDir()`): `update_pending.json`, `update_failed.json`, `update_pending.orphan.json`, `update_launcher.log`, `update_source_override`, `update_state.override.json`.
- Never install a version that is not newer than the running one; never write the Pak Store database.
- Commit after every task; end commit messages with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.

## Review Focus

1. **Second rename fails during the swap** → the live folder must be back under its own name and no pending file left; owned by Task 4 (`TestSwapStaged_secondRenameFails`).
2. **New binary dies before its first frame** → `launch.sh` rolls back exactly once and the restored version starts; owned by Task 6 (`rollback when the binary does not confirm`, `no loop after rollback`).
3. **A zip that passes the digest but is malicious or wrong** (`..`, absolute path, symlink, x86 binary, wrong version, bad `launch.sh`) → nothing staged, no `.staged*` left; owned by Task 2/3.
4. **Cancel or connection drop mid-download** → no `.staged.zip`, `.staged/` or partial left, Install row available again; owned by Task 3 (`TestStageNextUI_cancel`) and Task 7 (`TestChecker_installCancel`).
5. **Notice keeps reappearing for a version that failed to start** → suppressed for that tag only, a newer tag still announced; owned by Task 7 (`TestChecker_failedInstallSilencesNotice`).

---

## File map

| File | Responsibility |
|---|---|
| `internal/appupdate/state.go` | `Release` gains NextUI asset fields; `State.FailedInstall` |
| `internal/appupdate/source.go` | `NextUIAssetName`, fill NextUI asset in `toRelease`; override constructor |
| `internal/appupdate/decide.go` | `ViaInstall` |
| `internal/appupdate/install_paths.go` (new) | sibling-folder names, data-dir file names |
| `internal/appupdate/install_stage.go` (new) | `UnpackPak`, `CheckStaged`, `StageNextUI` |
| `internal/appupdate/install_apply.go` (new) | `Pending`, `SwapStaged`, `ExecLauncher`, `ConfirmStarted`, `TakeFailed`, `TakeLauncherLog`, `DiscardIncompleteStaged` |
| `internal/appupdate/override.go` (new) | `OverrideBase`, `NewOverrideSource` |
| `internal/appupdate/checker.go` | install state machine, restart request, failed-install handling |
| `internal/firmware/firmware.go` | `Env.PakDir()` |
| `launch.sh` | wait-and-roll-back while an update is pending |
| `scripts/launch_test.sh` | rollback cases |
| `internal/ui/app_updater.go`, `screen_updates.go`, `updates_status.go`, `dev_scenes_updates.go` | Install / Restart rows, status lines, scenes |
| `cmd/itchio-pak/main.go`, `main_sdl.go`, `main_headless.go` | start-up apply/confirm, restart exec, wiring |
| `scripts/update-fixture.sh` (new), `scripts/test.sh` | host fixture server, shellcheck |

`scripts/release-pak_test.sh` already asserts `launch.sh`, `itchio` and `pak.json` at the zip root (line 66); no change needed.

---

### Task 1: NextUI asset on `Release` and the `ViaInstall` route

**Files:**
- Modify: `internal/appupdate/state.go:14-20`
- Modify: `internal/appupdate/source.go:27-28,190-202`
- Modify: `internal/appupdate/decide.go:30-50,100-127`
- Test: `internal/appupdate/decide_test.go`, `internal/appupdate/source_test.go`

**Interfaces:**
- Produces: `Release.NextUIAsset string`, `Release.NextUIDigest string`, `Release.NextUISize int64`; `func NextUIAssetName(tag string) string`; `const ViaInstall Via`; `func (r *Release) HasNextUIAsset() bool`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/appupdate/decide_test.go`:

```go
func nuiRel(tag string) *Release {
	r := rel(tag)
	r.NextUIAsset = "https://example.invalid/" + NextUIAssetName(tag)
	r.NextUIDigest = "sha256:" + strings.Repeat("ab", 32)
	r.NextUISize = 14741523
	return r
}

func TestDecide_viaInstall(t *testing.T) {
	cases := []struct {
		name string
		in   Inputs
		via  Via
	}{
		{"rc to newer rc, Store-managed", Inputs{firmware.KindNextUI, v("v1.1.0-rc4"), RC, managed("v1.0.25"), nuiRel("v1.1.0-rc5"), rel("v1.0.25")}, ViaInstall},
		{"not managed", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, notStore, nuiRel("v1.0.26"), nil}, ViaInstall},
		{"Store has not caught up with the final", Inputs{firmware.KindNextUI, v("v1.1.0-rc4"), RC, managed("v1.0.25"), nuiRel("v1.1.0"), rel("v1.0.25")}, ViaInstall},
		{"Store will offer it: never install", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, managed("v1.0.25"), nuiRel("v1.0.26"), rel("v1.0.26")}, ViaPakStore},
		{"no NextUI asset", Inputs{firmware.KindNextUI, v("v1.1.0-rc4"), RC, notStore, rel("v1.1.0-rc5"), nil}, ViaReleasePage},
		{"muOS ignores the NextUI asset", Inputs{firmware.KindMuOS, v("v1.1.0-rc4"), RC, notStore, nuiRel("v1.1.0-rc5"), nil}, ViaReleasePage},
	}
	for _, c := range cases {
		if got := Decide(c.in); got.Kind != Available || got.Via != c.via {
			t.Errorf("%s: got %s via %s, want available via %s", c.name, got.Kind, got.Via, c.via)
		}
	}
}

func TestHasNextUIAsset(t *testing.T) {
	r := nuiRel("v1.1.0-rc5")
	if !r.HasNextUIAsset() {
		t.Fatal("complete asset not recognised")
	}
	r.NextUIDigest = "md5:00"
	if r.HasNextUIAsset() {
		t.Fatal("a non-sha256 digest must not count")
	}
}
```

Add `"strings"` to the test file's imports if absent.

Append to `internal/appupdate/source_test.go`:

```go
func TestToRelease_fillsNextUIAsset(t *testing.T) {
	r := toRelease(ghRelease{Tag: "v1.1.0-rc5", Assets: []ghAsset{
		{Name: "Itch-io.NextUI.v1.1.0-rc5.pak.zip", URL: "https://x/pak.zip", Size: 10, Digest: "sha256:aa"},
		{Name: "Itch-io.NextUI.v1.1.0-rc5.pakz", URL: "https://x/pakz", Size: 50, Digest: "sha256:bb"},
		{Name: "Itch-io.muOS.v1.1.0-rc5.muxapp", URL: "https://x/mux", Size: 9, Digest: "sha256:cc"},
	}})
	if r.NextUIAsset != "https://x/pak.zip" || r.NextUISize != 10 || r.NextUIDigest != "sha256:aa" {
		t.Fatalf("NextUI asset = %q %d %q", r.NextUIAsset, r.NextUISize, r.NextUIDigest)
	}
	if r.Asset != "https://x/mux" {
		t.Fatalf("muOS asset = %q", r.Asset)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless ./internal/appupdate/`
Expected: compile failure — `NextUIAsset`, `NextUIAssetName`, `ViaInstall`, `HasNextUIAsset` undefined.

- [ ] **Step 3: Implement**

`internal/appupdate/state.go`, replace the `Release` struct:

```go
// Release is one release as far as updates care.
type Release struct {
	Tag    string `json:"tag"`
	URL    string `json:"url"`              // the release page (html_url)
	Asset  string `json:"asset,omitempty"`  // muOS .muxapp download URL
	Digest string `json:"digest,omitempty"` // "sha256:<hex>"
	Size   int64  `json:"size,omitempty"`
	// The NextUI pak zip, installed in place (NextUI install spec §1).
	NextUIAsset  string `json:"nextui_asset,omitempty"`
	NextUIDigest string `json:"nextui_digest,omitempty"`
	NextUISize   int64  `json:"nextui_size,omitempty"`
}

// HasNextUIAsset: the release can be installed in place on NextUI.
func (r *Release) HasNextUIAsset() bool {
	return r != nil && r.NextUIAsset != "" && r.NextUISize > 0 && strings.HasPrefix(r.NextUIDigest, "sha256:")
}
```

Add `"strings"` to `state.go`'s imports.

`internal/appupdate/source.go`, under `AssetName`:

```go
// NextUIAssetName is the NextUI pak zip of a release: the files of
// Itch-io.pak at the zip's root.
func NextUIAssetName(tag string) string { return "Itch-io.NextUI." + tag + ".pak.zip" }
```

and in `toRelease` replace the asset loop:

```go
	want, wantNUI := AssetName(r.Tag), NextUIAssetName(r.Tag)
	for _, a := range r.Assets {
		switch a.Name {
		case want:
			rel.Asset, rel.Digest, rel.Size = a.URL, a.Digest, a.Size
		case wantNUI:
			rel.NextUIAsset, rel.NextUIDigest, rel.NextUISize = a.URL, a.Digest, a.Size
		}
	}
	return rel
```

`internal/appupdate/decide.go`: add to the `Via` constants after `ViaArchive`:

```go
	// ViaInstall: NextUI, no Store route, and the release has a pak zip
	// with a digest: install it in place.
	ViaInstall
```

add `case ViaInstall: return "install"` to `Via.String()`, and in `via()` replace the final `return ViaReleasePage` with:

```go
	if in.Firmware == firmware.KindNextUI && in.Latest.HasNextUIAsset() {
		return ViaInstall
	}
	return ViaReleasePage
```

(The NextUI `case` only `return`s `ViaReleasePage`/`ViaPakStore` early on the Store paths; make the early `return ViaReleasePage` inside the PakJSON check fall through to this tail instead: replace that inner `return ViaReleasePage` with `break`.)

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless ./internal/appupdate/`
Expected: PASS, including the existing `TestDecide` table (its `rel()` releases have no NextUI asset, so they keep `ViaReleasePage`) and `TestDecide_neverOffersADowngrade`.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/
git commit -m "appupdate: record the NextUI pak zip and route installable updates via ViaInstall"
```

---

### Task 2: Unpack and check a staged pak

**Files:**
- Create: `internal/appupdate/install_paths.go`
- Create: `internal/appupdate/install_stage.go`
- Test: `internal/appupdate/install_stage_test.go`

**Interfaces:**
- Produces:
  - `func StagedDir(pakDir string) string`, `StagedZip(pakDir) string`, `PrevDir(pakDir) string`, `FailedDir(pakDir) string` — siblings named `.<base>.staged`, `.<base>.staged.zip`, `.<base>.prev`, `.<base>.failed`.
  - `const completeMarker = ".complete"`.
  - `func UnpackPak(zipPath, dir string) (files int, bytes int64, err error)`
  - `func CheckStaged(dir, tag string) error`
  - `var ErrDamaged = errors.New("the update is damaged")` — every check failure wraps it (`fmt.Errorf("%w: %s", ErrDamaged, reason)`).

- [ ] **Step 1: Write the failing tests**

`internal/appupdate/install_stage_test.go`:

```go
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
		"dotdot":   {{name: "../evil", body: []byte("x")}},
		"absolute": {{name: "/etc/evil", body: []byte("x")}},
		"backslash": {{name: `a\b`, body: []byte("x")}},
		"symlink":  {{name: "link", body: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0777}},
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
```

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run 'InstallPaths|Unpack|CheckStaged' ./internal/appupdate/`
Expected: compile failure — `StagedDir`, `UnpackPak`, `CheckStaged`, `ErrDamaged` undefined.

- [ ] **Step 3: Implement**

`internal/appupdate/install_paths.go`:

```go
package appupdate

import "path/filepath"

// The update is staged, and the replaced version kept, beside the live pak
// folder on the same card, so the swap is two renames. A leading dot hides
// them from NextUI's menus (hide() in workspace/all/common/utils.c).
func sibling(pakDir, suffix string) string {
	return filepath.Join(filepath.Dir(pakDir), "."+filepath.Base(pakDir)+suffix)
}

func StagedDir(pakDir string) string { return sibling(pakDir, ".staged") }
func StagedZip(pakDir string) string { return sibling(pakDir, ".staged.zip") }
func PrevDir(pakDir string) string   { return sibling(pakDir, ".prev") }
func FailedDir(pakDir string) string { return sibling(pakDir, ".failed") }

// completeMarker is written last into the staged folder: without it the
// folder is half-made and is never applied.
const completeMarker = ".complete"

// Files in the data dir ($HOME on NextUI). launch.sh reads the first two by
// these exact names.
const (
	pendingFile     = "update_pending.json"
	failedFile      = "update_failed.json"
	orphanFile      = "update_pending.orphan.json"
	launcherLogFile = "update_launcher.log"
)
```

`internal/appupdate/install_stage.go`:

```go
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

// UnpackPak extracts a NextUI pak zip into dir, which must not exist yet.
// Entries are checked with the same rules as a .muxapp: no absolute paths,
// "..", backslashes, links or special files, and bounded counts and sizes.
func UnpackPak(zipPath, dir string) (int, int64, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, 0, damaged("not a zip: %v", err)
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
		case f.UncompressedSize64 > maxEntrySize:
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

	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, 0, err
	}
	files, written := 0, int64(0)
	for _, f := range zr.File {
		dest := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.Mode().IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return files, written, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return files, written, err
		}
		n, err := extractOne(f, dest)
		written += n
		if err != nil {
			logger.Error("appupdate: unpack %s: %v", f.Name, err)
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
	n, err := io.Copy(out, io.LimitReader(rc, maxEntrySize+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
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
```

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run 'InstallPaths|Unpack|CheckStaged' ./internal/appupdate/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/install_paths.go internal/appupdate/install_stage.go internal/appupdate/install_stage_test.go
git commit -m "appupdate: unpack and check a staged NextUI pak"
```

---

### Task 3: `StageNextUI` — download, verify, unpack, mark complete

**Files:**
- Modify: `internal/appupdate/install_stage.go`
- Test: `internal/appupdate/install_stage_test.go`

**Interfaces:**
- Consumes: `fetchAsset`, `freeBytes`, `ErrIntegrity`, `SpaceError` (archive.go); `UnpackPak`, `CheckStaged`, path helpers (Task 2).
- Produces: `func StageNextUI(ctx context.Context, hc *http.Client, userAgent string, rel Release, pakDir string, phase func(InstallPhase), progress func(done, total int64)) error`; `type InstallPhase int` with `PhaseDownloading`, `PhaseChecking`, `PhaseUnpacking`.

- [ ] **Step 1: Write the failing tests**

Append to `install_stage_test.go` (add imports `context`, `crypto/sha256`, `encoding/hex`, `net/http`, `net/http/httptest`, `time`, and `github.com/carroarmato0/nextui-itchio-pak/internal/partfile`):

```go
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
```

Add `"fmt"` and `"strings"` to the test imports. `serve` already exists in `archive_test.go` (same package).

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run StageNextUI ./internal/appupdate/`
Expected: compile failure — `StageNextUI`, `InstallPhase` undefined.

- [ ] **Step 3: Implement**

Append to `install_stage.go` (add imports `context`, `net/http`, `time`, `github.com/carroarmato0/nextui-itchio-pak/internal/netstate`, `github.com/carroarmato0/nextui-itchio-pak/internal/partfile`):

```go
// InstallPhase is what StageNextUI is doing, for the progress row.
type InstallPhase int

const (
	PhaseDownloading InstallPhase = iota
	PhaseChecking
	PhaseUnpacking
)

// unpackFactor: the unpacked pak is about 1.8× the zip; leave margin.
const unpackFactor = 5 // in halves: 2.5×

// StageNextUI downloads rel's NextUI pak zip next to pakDir, verifies it,
// unpacks it into StagedDir(pakDir) and checks it, then writes .complete.
// On any failure or cancel nothing is left behind.
func StageNextUI(ctx context.Context, hc *http.Client, userAgent string, rel Release, pakDir string,
	phase func(InstallPhase), progress func(done, total int64)) (err error) {
	if !rel.HasNextUIAsset() || pakDir == "" {
		return fmt.Errorf("%s has no installable NextUI asset", rel.Tag)
	}
	if phase == nil {
		phase = func(InstallPhase) {}
	}
	staged, zipPath := StagedDir(pakDir), StagedZip(pakDir)
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staged)
			_ = os.Remove(zipPath)
			logger.Info("appupdate: staging %s abandoned, removed %s", rel.Tag, staged)
		}
	}()
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	_ = os.Remove(zipPath)

	need := rel.NextUISize + rel.NextUISize*unpackFactor/2
	if free, ok := freeBytes(filepath.Dir(pakDir)); ok && free < need {
		logger.Warn("appupdate: not enough space beside %s: need %d, have %d", pakDir, need, free)
		return &SpaceError{Need: need, Have: free}
	}

	start := time.Now()
	phase(PhaseDownloading)
	logger.Info("appupdate: install: downloading %s (%d bytes) to %s", NextUIAssetName(rel.Tag), rel.NextUISize, zipPath)
	pf, err := partfile.Create(zipPath)
	if err != nil {
		logger.Error("appupdate: create partial for %s: %v", zipPath, err)
		return err
	}
	defer pf.Abort() // no-op after Commit
	sum, done, err := fetchAsset(ctx, hc, userAgent, rel.NextUIAsset, pf, rel.NextUISize, progress)
	if err != nil {
		logger.Error("appupdate: install download failed after %d bytes: %s", done, netstate.Detail(err))
		return err
	}

	phase(PhaseChecking)
	if done != rel.NextUISize {
		logger.Error("appupdate: %s is %d bytes, release says %d", zipPath, done, rel.NextUISize)
		return ErrIntegrity
	}
	if want := strings.TrimPrefix(rel.NextUIDigest, "sha256:"); !strings.EqualFold(sum, want) {
		logger.Error("appupdate: verify mismatch: got sha256:%s want sha256:%s", sum, want)
		return ErrIntegrity
	}
	logger.Info("appupdate: verify ok for %s", rel.Tag)
	if err := pf.Commit(); err != nil {
		return err
	}

	phase(PhaseUnpacking)
	if _, _, err := UnpackPak(zipPath, staged); err != nil {
		logger.Error("appupdate: unpack %s: %v", rel.Tag, err)
		return err
	}
	if err := CheckStaged(staged, rel.Tag); err != nil {
		logger.Error("appupdate: staged %s rejected: %v", rel.Tag, err)
		return err
	}
	if err := os.Remove(zipPath); err != nil {
		logger.Warn("appupdate: remove %s: %v", zipPath, err)
	}
	if err := os.WriteFile(filepath.Join(staged, completeMarker), []byte(rel.Tag+"\n"), 0644); err != nil {
		logger.Error("appupdate: write %s marker: %v", completeMarker, err)
		return err
	}
	logger.Info("appupdate: staged %s in %s in %v", rel.Tag, staged, time.Since(start).Round(time.Millisecond))
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -race -tags headless ./internal/appupdate/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/install_stage.go internal/appupdate/install_stage_test.go
git commit -m "appupdate: stage a NextUI update beside the live pak"
```

---

### Task 4: Swap, pending file, confirm and failed report

**Files:**
- Create: `internal/appupdate/install_apply.go`
- Test: `internal/appupdate/install_apply_test.go`

**Interfaces:**
- Consumes: path helpers and file-name constants (Task 2).
- Produces:
  - `type Pending struct { From, To string; At time.Time }` (JSON keys `from`, `to`, `at`)
  - `func StagedReady(pakDir string) (tag string, ok bool)` — reads `.complete`
  - `func DiscardIncompleteStaged(pakDir string)`
  - `func SwapStaged(pakDir, dataDir, from string) error`
  - `func ExecLauncher(pakDir string) error` (does not return on success)
  - `func ConfirmStarted(pakDir, dataDir string)` — deletes pending, removes `.prev`/`.failed` in the background
  - `func TakeFailed(dataDir string) (*Pending, bool)` — reads and deletes `update_failed.json`
  - `func TakeLauncherLog(dataDir string)` — copies `update_launcher.log` lines into the log, deletes it
  - `var rename = os.Rename` (test seam)

- [ ] **Step 1: Write the failing tests**

`internal/appupdate/install_apply_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run 'StagedReady|SwapStaged|ConfirmStarted|TakeFailed|TakeLauncherLog' ./internal/appupdate/`
Expected: compile failure.

- [ ] **Step 3: Implement**

`internal/appupdate/install_apply.go`:

```go
package appupdate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Pending is update_pending.json: an update that has been swapped in and
// has not yet proved that it starts. launch.sh rolls back while it exists.
type Pending struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

// rename is os.Rename; tests replace it to fail the second rename.
var rename = os.Rename

// StagedReady reports the tag of a complete staged update.
func StagedReady(pakDir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(StagedDir(pakDir), completeMarker))
	if err != nil {
		return "", false
	}
	tag := strings.TrimSpace(string(b))
	return tag, tag != ""
}

// DiscardIncompleteStaged removes a staged folder that never got its
// .complete marker (a crash or power cut while staging).
func DiscardIncompleteStaged(pakDir string) {
	dir := StagedDir(pakDir)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if _, ok := StagedReady(pakDir); ok {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("appupdate: remove incomplete %s: %v", dir, err)
		return
	}
	logger.Info("appupdate: removed incomplete staged update %s", dir)
}

// SwapStaged puts the staged update in place of the live pak folder. The
// pending file is written first, so the new version is never in place
// without rollback protection.
func SwapStaged(pakDir, dataDir, from string) error {
	to, ok := StagedReady(pakDir)
	if !ok {
		return errors.New("no complete staged update")
	}
	prev, staged := PrevDir(pakDir), StagedDir(pakDir)
	if err := os.RemoveAll(prev); err != nil {
		logger.Error("appupdate: apply: remove old %s: %v", prev, err)
		return err
	}
	pendingPath := filepath.Join(dataDir, pendingFile)
	data, _ := json.Marshal(Pending{From: from, To: to, At: time.Now().UTC()})
	if err := os.WriteFile(pendingPath, data, 0644); err != nil {
		logger.Error("appupdate: apply: write %s: %v", pendingPath, err)
		return err
	}
	logger.Info("appupdate: apply: rename %s → %s", pakDir, prev)
	if err := rename(pakDir, prev); err != nil {
		logger.Error("appupdate: apply: rename %s: %v", pakDir, err)
		_ = os.Remove(pendingPath)
		return err
	}
	logger.Info("appupdate: apply: rename %s → %s", staged, pakDir)
	if err := rename(staged, pakDir); err != nil {
		logger.Error("appupdate: apply: rename %s: %v; restoring %s", staged, err, prev)
		if rerr := rename(prev, pakDir); rerr != nil {
			logger.Error("appupdate: apply: restore %s failed: %v", pakDir, rerr)
		}
		_ = os.Remove(pendingPath)
		return err
	}
	logger.Info("appupdate: apply: %s → %s in place, pending confirmation", from, to)
	return nil
}

// ExecLauncher replaces this process with the pak's launch.sh. Same PID and
// environment, so NextUI sees the same pak still running. Returns only on
// failure.
func ExecLauncher(pakDir string) error {
	launcher := filepath.Join(pakDir, "launch.sh")
	logger.Info("appupdate: exec %s", launcher)
	err := syscall.Exec("/bin/sh", []string{"/bin/sh", launcher}, os.Environ())
	logger.Error("appupdate: exec %s: %v", launcher, err)
	return err
}

// ConfirmStarted is called once the first frame is up and input has been
// read. It clears the pending file and removes the kept versions in the
// background; the returned WaitGroup is for tests.
func ConfirmStarted(pakDir, dataDir string) *sync.WaitGroup {
	var wg sync.WaitGroup
	path := filepath.Join(dataDir, pendingFile)
	b, err := os.ReadFile(path)
	if err == nil {
		var p Pending
		_ = json.Unmarshal(b, &p)
		if err := os.Remove(path); err != nil {
			logger.Error("appupdate: confirm: remove %s: %v", path, err)
		} else {
			logger.Info("update: %s → %s confirmed", p.From, p.To)
		}
	}
	if pakDir == "" {
		return &wg
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, d := range []string{PrevDir(pakDir), FailedDir(pakDir)} {
			if _, err := os.Stat(d); err != nil {
				continue
			}
			if err := os.RemoveAll(d); err != nil {
				logger.Warn("appupdate: remove %s: %v", d, err)
				continue
			}
			logger.Info("appupdate: removed %s", d)
		}
	}()
	return &wg
}

// TakeFailed reads and deletes update_failed.json, written by launch.sh
// when it rolled an update back.
func TakeFailed(dataDir string) (*Pending, bool) {
	path := filepath.Join(dataDir, failedFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	_ = os.Remove(path)
	var p Pending
	if err := json.Unmarshal(b, &p); err != nil || p.To == "" {
		logger.Warn("appupdate: %s unreadable: %v", path, err)
		return nil, false
	}
	logger.Warn("update: %s did not start, rolled back to %s", p.To, p.From)
	return &p, true
}

// TakeLauncherLog copies launch.sh's own log into ours and deletes it.
func TakeLauncherLog(dataDir string) {
	path := filepath.Join(dataDir, launcherLogFile)
	f, err := os.Open(path)
	if err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		logger.Info("launch.sh: %s", sc.Text())
	}
	f.Close()
	if err := os.Remove(path); err != nil {
		logger.Warn("appupdate: remove %s: %v", path, err)
	}
}

// String is for log lines.
func (p Pending) String() string { return fmt.Sprintf("%s → %s", p.From, p.To) }
```

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -race -tags headless ./internal/appupdate/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/install_apply.go internal/appupdate/install_apply_test.go
git commit -m "appupdate: swap a staged update in, confirm it, report a rollback"
```

---

### Task 5: `firmware.Env.PakDir()`

**Files:**
- Modify: `internal/firmware/firmware.go` (near `ArchiveDir`, line ~293)
- Test: `internal/firmware/firmware_test.go`

**Interfaces:**
- Produces: `func (e *Env) PakDir() string` — on NextUI the directory of the running executable; `""` on muOS and host (any other kind). `var executable = os.Executable` (test seam).

- [ ] **Step 1: Write the failing test**

Append to `internal/firmware/firmware_test.go`:

```go
func TestPakDir(t *testing.T) {
	executable = func() (string, error) { return "/mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio", nil }
	t.Cleanup(func() { executable = os.Executable })
	t.Setenv("PLATFORM", "tg5040")
	if got := newNextUI("").PakDir(); got != "/mnt/SDCARD/Tools/tg5040/Itch-io.pak" {
		t.Errorf("NextUI PakDir = %q", got)
	}
	prefix := t.TempDir()
	if got := (&Env{kind: KindMuOS}).PakDir(); got != "" {
		t.Errorf("muOS PakDir = %q, want empty (prefix %s)", got, prefix)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run TestPakDir ./internal/firmware/`
Expected: compile failure — `executable`, `PakDir` undefined.

- [ ] **Step 3: Implement**

In `internal/firmware/firmware.go` after `ArchiveDir`:

```go
// executable is os.Executable; tests replace it.
var executable = os.Executable

// PakDir is the pak folder this binary runs from, which an in-place update
// replaces. NextUI only: "" elsewhere, and in-place install is then never
// offered.
func (e *Env) PakDir() string {
	if e.kind != KindNextUI {
		return ""
	}
	exe, err := executable()
	if err != nil {
		logger.Warn("firmware: cannot resolve the executable: %v", err)
		return ""
	}
	return filepath.Dir(exe)
}
```

(`logger`, `os` and `filepath` are already imported by `firmware.go`; check and add if not.)

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless ./internal/firmware/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/firmware/
git commit -m "firmware: PakDir, the folder an in-place update replaces"
```

---

### Task 6: `launch.sh` waits and rolls back while an update is pending

**Files:**
- Modify: `launch.sh` (the final `exec` block, from `PROFILE_FLAGS=""` to the end)
- Test: `scripts/launch_test.sh`

**Interfaces:**
- Consumes: data-dir names `update_pending.json`, `update_failed.json`, `update_pending.orphan.json`, `update_launcher.log`; sibling names `.<name>.prev`, `.<name>.failed` (Task 2).

- [ ] **Step 1: Write the failing tests**

Append to `scripts/launch_test.sh`, before its final summary block (the lines that print the PASS/FAIL totals and exit):

```sh
# --- In-place update rollback ------------------------------------------------
#
# run_update <case> sets up Tools/tg5040/Itch-io.pak (new) and .Itch-io.pak.prev
# (old), each with a stub itchio, and runs the new launch.sh. launch.sh derives
# $HOME as $SHARED_USERDATA_PATH/Itch-io, so the pending file is seeded there.
# The stubs record which version ran in $HOME/ran.
#   confirm - the new stub deletes the pending file (it started)
#   crash   - the new stub exits 1 without confirming
#   noprev  - like crash, but there is no .prev to restore
run_update() {
    _case="$1"
    _root="$TMP/upd-$_case"
    rm -rf "$_root"
    _tools="$_root/Tools/tg5040"
    _home="$_root/shared/Itch-io"
    mkdir -p "$_tools/Itch-io.pak/assets" "$_home"
    cp launch.sh "$_tools/Itch-io.pak/launch.sh"
    if [ "$_case" = confirm ]; then
        printf '#!/bin/sh\necho new >> "$HOME/ran"\nrm -f "$HOME/update_pending.json"\n' > "$_tools/Itch-io.pak/itchio"
    else
        printf '#!/bin/sh\necho new >> "$HOME/ran"\nexit 1\n' > "$_tools/Itch-io.pak/itchio"
    fi
    chmod +x "$_tools/Itch-io.pak/itchio"
    if [ "$_case" != noprev ]; then
        mkdir -p "$_tools/.Itch-io.pak.prev/assets"
        cp launch.sh "$_tools/.Itch-io.pak.prev/launch.sh"
        printf '#!/bin/sh\necho old >> "$HOME/ran"\n' > "$_tools/.Itch-io.pak.prev/itchio"
        chmod +x "$_tools/.Itch-io.pak.prev/itchio"
    fi
    printf '{"from":"v1.1.0-rc5","to":"v1.1.0-rc6"}' > "$_home/update_pending.json"
    SHARED_USERDATA_PATH="$_root/shared" PLATFORM=tg5040 \
        sh "$_tools/Itch-io.pak/launch.sh" >/dev/null 2>&1 || true
}

run_update confirm
H="$TMP/upd-confirm/shared/Itch-io"; T="$TMP/upd-confirm/Tools/tg5040"
[ "$(cat "$H/ran")" = "new" ] && ok "update confirmed: only the new version ran" || fail "update confirmed: ran '$(cat "$H/ran")'"
[ -d "$T/.Itch-io.pak.prev" ] && ok "update confirmed: .prev left for the app to remove" || fail "update confirmed: .prev vanished"
[ ! -f "$H/update_failed.json" ] && ok "update confirmed: no failure recorded" || fail "update confirmed: failure recorded"

run_update crash
H="$TMP/upd-crash/shared/Itch-io"; T="$TMP/upd-crash/Tools/tg5040"
[ "$(tr '\n' ' ' < "$H/ran")" = "new old " ] && ok "rollback when the binary does not confirm: new, then old ran" || fail "rollback: ran '$(tr '\n' ' ' < "$H/ran")'"
grep -q 'echo old' "$T/Itch-io.pak/itchio" && ok "rollback: the old version is live again" || fail "rollback: live folder is not the old version"
[ -d "$T/.Itch-io.pak.failed" ] && ok "rollback: the failed version is kept as .failed" || fail "rollback: no .failed"
[ ! -f "$H/update_pending.json" ] && [ -f "$H/update_failed.json" ] && ok "rollback: pending moved to update_failed.json" || fail "rollback: pending/failed files wrong"
grep -q rollback "$H/update_launcher.log" && ok "rollback: launcher logged it" || fail "rollback: no launcher log"

# The restored launcher must not roll back again: old exits 0 without a
# pending file, and nothing else runs.
[ "$(wc -l < "$H/ran" | tr -d ' ')" = 2 ] && ok "no loop after rollback" || fail "no loop after rollback: ran $(wc -l < "$H/ran") times"

run_update noprev
H="$TMP/upd-noprev/shared/Itch-io"
[ ! -f "$H/update_pending.json" ] && [ -f "$H/update_pending.orphan.json" ] && [ ! -f "$H/update_failed.json" ] \
    && ok "no .prev: pending set aside, nothing restored" || fail "no .prev: pending/orphan/failed files wrong"
```

- [ ] **Step 2: Run to verify they fail**

Run: `sh scripts/launch_test.sh`
Expected: the new `rollback…` and `no .prev…` cases FAIL (today's launch.sh execs and never rolls back); existing cases still pass.

- [ ] **Step 3: Implement**

In `launch.sh`, right after `cd "$PAK_DIR" || exit 1`, add:

```sh
# Absolute from here on: the rollback below renames folders by full path.
PAK_DIR="$(pwd)"
```

Replace the final two lines (`# shellcheck disable=SC2086` and `exec "$PAK_DIR/itchio" $PROFILE_FLAGS "$@"`) with:

```sh
# An in-place update is confirmed by the new binary deleting
# update_pending.json once its first frame is up. Until then this launcher —
# always the *new* version's — waits for the binary instead of exec'ing it,
# and puts the previous version back if the binary exits without confirming.
# See internal/appupdate/install_apply.go and the NextUI install spec §4.
PENDING="$HOME/update_pending.json"
if [ ! -f "$PENDING" ]; then
    # shellcheck disable=SC2086
    exec "$PAK_DIR/itchio" $PROFILE_FLAGS "$@"
fi

# shellcheck disable=SC2086
"$PAK_DIR/itchio" $PROFILE_FLAGS "$@"
STATUS=$?
[ -f "$PENDING" ] || exit "$STATUS"

ULOG="$HOME/update_launcher.log"
TOOLS_DIR="$(dirname "$PAK_DIR")"
LIVE="$(basename "$PAK_DIR")"
PREV="$TOOLS_DIR/.$LIVE.prev"
FAILED="$TOOLS_DIR/.$LIVE.failed"
if [ ! -d "$PREV" ]; then
    echo "$(date '+%F %T') update did not confirm (exit $STATUS) and there is no $PREV: nothing to restore" >> "$ULOG"
    mv -f "$PENDING" "$HOME/update_pending.orphan.json"
    exit "$STATUS"
fi
echo "$(date '+%F %T') update did not confirm (exit $STATUS): rollback to $PREV" >> "$ULOG"
rm -rf "$FAILED"
if ! mv "$PAK_DIR" "$FAILED"; then
    echo "$(date '+%F %T') rollback: rename $PAK_DIR failed" >> "$ULOG"
    mv -f "$PENDING" "$HOME/update_pending.orphan.json"
    exit "$STATUS"
fi
if ! mv "$PREV" "$PAK_DIR"; then
    echo "$(date '+%F %T') rollback: rename $PREV failed, putting the new version back" >> "$ULOG"
    mv "$FAILED" "$PAK_DIR"
    mv -f "$PENDING" "$HOME/update_pending.orphan.json"
    exit "$STATUS"
fi
mv -f "$PENDING" "$HOME/update_failed.json"
echo "$(date '+%F %T') rollback: done, starting the restored version" >> "$ULOG"
exec /bin/sh "$PAK_DIR/launch.sh" "$@"
```

- [ ] **Step 4: Run tests and shellcheck**

Run: `sh scripts/launch_test.sh && shellcheck -s sh launch.sh scripts/launch_test.sh`
Expected: every case `ok`, shellcheck clean. If shellcheck flags the test's `printf` lines for `$HOME` in single quotes, add `# shellcheck disable=SC2016` above each, as the existing stub does.

- [ ] **Step 5: Commit**

```bash
git add launch.sh scripts/launch_test.sh
git commit -m "launch.sh: roll an in-place update back when it never confirms"
```

---

### Task 7: Checker — install state, restart request, failed-install handling

**Files:**
- Modify: `internal/appupdate/checker.go`, `internal/appupdate/state.go`
- Test: `internal/appupdate/install_checker_test.go` (new)

**Interfaces:**
- Consumes: `StageNextUI`, `StagedReady`, `ViaInstall`, `Release.HasNextUIAsset`, `Pending`.
- Produces:
  - `Config.PakDir string`, `Config.Failed *Pending`
  - `State.FailedInstall string` (JSON `failed_install,omitempty`)
  - `Verdict.FailedInstall string` — the tag that failed to start, when it equals `Latest.Tag`; `Verdict.FailedFrom string`
  - `type InstallState int` — `InstallIdle`, `InstallRunning`, `InstallStaged`, `InstallFailed`
  - `type InstallStatus struct { State InstallState; Phase InstallPhase; Tag string; Done, Total int64; Err error }`
  - `func (c *Checker) StartInstall()`, `CancelInstall()`, `InstallStatus() InstallStatus`, `RequestRestart()`, `RestartRequested() bool`

- [ ] **Step 1: Write the failing tests**

`internal/appupdate/install_checker_test.go`:

```go
package appupdate

import (
	"context"
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
```

Add helpers to the same file:

```go
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
```

with imports `errors`, `fmt`, `net/http`, `net/http/httptest`.

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run 'TestChecker_' ./internal/appupdate/`
Expected: compile failure — `PakDir`, `Failed`, `StartInstall`, `InstallStatus`… undefined.

- [ ] **Step 3: Implement**

`state.go`, in `State` after `Notified`:

```go
	// FailedInstall is a version that was installed in place and did not
	// start; the notice stays quiet for it (NextUI install spec §4.3).
	FailedInstall string `json:"failed_install,omitempty"`
```

`decide.go`, in `Verdict` after `StoreOffers`:

```go
	// FailedInstall is Latest.Tag when that version was installed in place
	// and rolled back; FailedFrom is the version that was kept.
	FailedInstall string
	FailedFrom    string
```

`checker.go`:

Add to `Config`:

```go
	PakDir     string   // firmware.Env.PakDir(); "" disables in-place install
	Failed     *Pending // appupdate.TakeFailed at start-up, if a rollback happened
```

Add types after `ArchiveStatus`:

```go
// InstallState is where an in-place install is.
type InstallState int

const (
	InstallIdle InstallState = iota
	InstallRunning
	InstallStaged
	InstallFailed
)

// InstallStatus is a snapshot of the in-place install.
type InstallStatus struct {
	State       InstallState
	Phase       InstallPhase
	Tag         string
	Done, Total int64
	Err         error
}
```

Add fields to `Checker` (after `cancelArchive`):

```go
	install       InstallStatus
	cancelInstall context.CancelFunc
	failedFrom    string
	restart       atomic.Bool
```

In `NewChecker`, before the final `logger.Info`:

```go
	if cfg.Failed != nil {
		c.st.FailedInstall, c.failedFrom = cfg.Failed.To, cfg.Failed.From
		c.saveLocked()
	}
	if tag, ok := StagedReady(cfg.PakDir); cfg.PakDir != "" && ok {
		c.install = InstallStatus{State: InstallStaged, Tag: tag}
		logger.Info("appupdate: %s is staged and installs at the next launch", tag)
	}
```

(`saveLocked` only writes `c.st` to `cfg.StatePath`; calling it here without the lock is safe because no other goroutine has the checker yet.)

Replace the body of `verdictLocked` after the `Decide(...)` call:

```go
	v := Decide(Inputs{Firmware: c.cfg.Firmware, Running: c.running, Channel: c.channel,
		Store: c.store, Latest: latest, PakJSON: c.st.PakJSON})
	if v.Via == ViaInstall && c.cfg.PakDir == "" {
		v.Via = ViaReleasePage
	}
	if v.Latest != nil && c.st.FailedInstall != "" && v.Latest.Tag == c.st.FailedInstall {
		v.FailedInstall, v.FailedFrom = c.st.FailedInstall, c.failedFrom
	}
	return v
```

In `PendingNotice`, replace the return:

```go
	v := c.verdictLocked()
	if v.FailedInstall != "" {
		return v, false
	}
	return v, ShouldNotify(v, c.st.Notified[v.Channel])
```

Add methods (after `ArchiveStatus`):

```go
// StartInstall stages the current ViaInstall release beside the live pak.
func (c *Checker) StartInstall() {
	c.mu.Lock()
	v := c.verdictLocked()
	if v.Kind != Available || v.Via != ViaInstall || c.install.State == InstallRunning {
		c.mu.Unlock()
		logger.Debug("appupdate: install ignored: verdict=%s via=%s state=%d", v.Kind, v.Via, c.install.State)
		return
	}
	r := *v.Latest
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelInstall = cancel
	c.install = InstallStatus{State: InstallRunning, Phase: PhaseDownloading, Tag: r.Tag, Total: r.NextUISize}
	c.mu.Unlock()

	logger.Info("appupdate: install start %s → %s asset=%s size=%d", c.cfg.Running, r.Tag, NextUIAssetName(r.Tag), r.NextUISize)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		err := StageNextUI(ctx, c.dl, c.cfg.UserAgent, r, c.cfg.PakDir,
			func(p InstallPhase) {
				c.mu.Lock()
				c.install.Phase = p
				c.mu.Unlock()
				c.notify()
			},
			func(done, _ int64) {
				c.mu.Lock()
				c.install.Done = done
				c.mu.Unlock()
			})
		c.mu.Lock()
		if err != nil {
			c.install.State, c.install.Err = InstallFailed, err
		} else {
			c.install.State = InstallStaged
		}
		c.cancelInstall = nil
		c.mu.Unlock()
		switch {
		case err == nil:
			logger.Info("appupdate: install staged %s", r.Tag)
		case errors.Is(err, context.Canceled):
			logger.Info("appupdate: install cancelled for %s", r.Tag)
		default:
			logger.Warn("appupdate: install failed for %s: %s", r.Tag, netstate.Detail(err))
		}
		c.notify()
	}()
}

// CancelInstall stops a running install; nothing is left behind.
func (c *Checker) CancelInstall() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelInstall != nil {
		logger.Info("appupdate: install cancel requested")
		c.cancelInstall()
	}
}

func (c *Checker) InstallStatus() InstallStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.install
}

// RequestRestart asks main to swap the staged update in and exec it once the
// event loop has exited.
func (c *Checker) RequestRestart() {
	if c.InstallStatus().State != InstallStaged {
		return
	}
	logger.Info("appupdate: restart requested to finish installing %s", c.InstallStatus().Tag)
	c.restart.Store(true)
}

func (c *Checker) RestartRequested() bool { return c.restart.Load() }
```

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -race -tags headless ./internal/appupdate/`
Expected: PASS (new and existing checker tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/
git commit -m "appupdate: checker installs in place, records rollbacks, takes restart requests"
```

---

### Task 8: Update source override

**Files:**
- Create: `internal/appupdate/override.go`
- Modify: `internal/appupdate/checker.go` (`Config.Override string`, log in `startCheck`/`run`)
- Test: `internal/appupdate/override_test.go`

**Interfaces:**
- Produces: `const overrideFile = "update_source_override"`; `func OverrideBase(dataDir string) (string, bool)`; `func NewOverrideSource(userAgent, base string) *Source`; `Config.Override string`; `func (c *Checker) SourceOverride() string`.

- [ ] **Step 1: Write the failing tests**

`internal/appupdate/override_test.go`:

```go
package appupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOverrideBase(t *testing.T) {
	dir := t.TempDir()
	if _, ok := OverrideBase(dir); ok {
		t.Fatal("no file, no override")
	}
	os.WriteFile(filepath.Join(dir, overrideFile), []byte("http://127.0.0.1:8765\n# comment\n"), 0644)
	if b, ok := OverrideBase(dir); !ok || b != "http://127.0.0.1:8765/" {
		t.Fatalf("OverrideBase = %q %v", b, ok)
	}
	os.WriteFile(filepath.Join(dir, overrideFile), []byte("ftp://nope\n"), 0644)
	if _, ok := OverrideBase(dir); ok {
		t.Fatal("only http and https are accepted")
	}
}

func TestNewOverrideSource_readsTheFixture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ghRelease{{Tag: "v1.1.0-rc6", Prerelease: true,
			Assets: []ghAsset{{Name: NextUIAssetName("v1.1.0-rc6"), URL: "http://x/z", Size: 5, Digest: "sha256:aa"}}}})
	})
	mux.HandleFunc("/pak.json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"version":"v1.1.0-rc6"}`)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := NewOverrideSource("test", srv.URL+"/")
	res, err := s.Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC == nil || !res.RC.HasNextUIAsset() {
		t.Fatalf("RC = %+v", res.RC)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -tags headless -run 'Override' ./internal/appupdate/`
Expected: compile failure.

- [ ] **Step 3: Implement**

`internal/appupdate/override.go`:

```go
package appupdate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// overrideFile, in the data dir, points update checks at a test server
// instead of GitHub (NextUI install spec §6). Written by
// scripts/update-fixture.sh; never by the app.
const overrideFile = "update_source_override"

// OverrideBase returns the override's base URL, with a trailing slash.
func OverrideBase(dataDir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, overrideFile))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
		logger.Warn("appupdate: ignoring %s: %q is not an http(s) URL", overrideFile, line)
		return "", false
	}
	if !strings.HasSuffix(line, "/") {
		line += "/"
	}
	return line, true
}

// NewOverrideSource is NewSource against a fixture server.
func NewOverrideSource(userAgent, base string) *Source {
	s := NewSource(userAgent)
	s.ReleasesURL = base + "releases.json"
	s.PakJSONURL = base + "pak.json"
	return s
}
```

`checker.go`: add `Override string // test update source base URL, "" normally` to `Config`; add

```go
// SourceOverride is the test update source in use, "" normally.
func (c *Checker) SourceOverride() string { return c.cfg.Override }
```

and at the top of the function that starts a check run (`startCheck`), add:

```go
	if c.cfg.Override != "" {
		logger.Warn("appupdate: WARNING using update source override %s", c.cfg.Override)
	}
```

- [ ] **Step 4: Run tests**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test -race -tags headless ./internal/appupdate/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/
git commit -m "appupdate: debug override for the update source"
```

---

### Task 9: Settings → App updates — Install and Restart rows

**Files:**
- Modify: `internal/ui/app_updater.go` (interface), `internal/ui/screen_updates.go`, `internal/ui/updates_status.go`, `internal/ui/dev_scenes_updates.go`
- Test: `internal/ui/screen_updates_test.go`

**Interfaces:**
- Consumes: Task 7 checker methods and types; Task 8 `SourceOverride()`.
- Produces: `AppUpdater` gains `StartInstall()`, `CancelInstall()`, `InstallStatus() appupdate.InstallStatus`, `RequestRestart()`, `SourceOverride() string`; `updatesStatus(enabled, v, a, inst, offline, lastErr, rateUntil, override)`; row `uRowInstall`.

- [ ] **Step 1: Write the failing tests**

Read `internal/ui/screen_updates_test.go` first and follow its fake-updater pattern (it has a test double implementing `AppUpdater`; add the five new methods to it, storing `inst appupdate.InstallStatus`, `started, cancelled, restarted bool`, `override string`). Then append:

```go
func installVerdict() appupdate.Verdict {
	v, _ := appupdate.Parse("v1.1.0-rc4")
	return appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.RC, Running: v,
		Latest: &appupdate.Release{Tag: "v1.1.0-rc5", URL: "https://example.invalid/r"}, Via: appupdate.ViaInstall}
}

func TestUpdatesStatus_install(t *testing.T) {
	v := installVerdict()
	cases := []struct {
		name string
		v    appupdate.Verdict
		inst appupdate.InstallStatus
		want string
	}{
		{"available", v, appupdate.InstallStatus{}, "v1.1.0-rc5 is available."},
		{"downloading", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Tag: "v1.1.0-rc5"}, "Downloading v1.1.0-rc5…"},
		{"checking", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Phase: appupdate.PhaseChecking, Tag: "v1.1.0-rc5"}, "Checking v1.1.0-rc5…"},
		{"unpacking", v, appupdate.InstallStatus{State: appupdate.InstallRunning, Phase: appupdate.PhaseUnpacking, Tag: "v1.1.0-rc5"}, "Unpacking v1.1.0-rc5…"},
		{"staged", v, appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}, "v1.1.0-rc5 is ready. Restart to finish installing, or it installs the next time you open Itch-io."},
		{"space", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: &appupdate.SpaceError{Need: 34 << 20, Have: 12 << 20}}, "Not enough space on the SD card: needs 34 MB, 12 MB free."},
		{"damaged", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: fmt.Errorf("%w: pak.json says x", appupdate.ErrDamaged)}, "The update is damaged: pak.json says x."},
		{"integrity", v, appupdate.InstallStatus{State: appupdate.InstallFailed, Tag: "v1.1.0-rc5", Err: appupdate.ErrIntegrity}, "The download failed the integrity check."},
	}
	for _, c := range cases {
		view := updatesStatus(true, c.v, appupdate.ArchiveStatus{}, c.inst, false, nil, time.Time{}, "")
		if !hasLine(view, c.want) {
			t.Errorf("%s: lines %v lack %q", c.name, view.Lines, c.want)
		}
	}
	failed := v
	failed.FailedInstall, failed.FailedFrom = "v1.1.0-rc5", "v1.1.0-rc4"
	if view := updatesStatus(true, failed, appupdate.ArchiveStatus{}, appupdate.InstallStatus{}, false, nil, time.Time{}, ""); !hasLine(view, "v1.1.0-rc5 did not start, so v1.1.0-rc4 was kept.") {
		t.Errorf("failed: lines %v", view.Lines)
	}
	if view := updatesStatus(true, v, appupdate.ArchiveStatus{}, appupdate.InstallStatus{}, false, nil, time.Time{}, "http://127.0.0.1:8765/"); !hasLine(view, "Test update source: http://127.0.0.1:8765/") {
		t.Errorf("override: lines %v", view.Lines)
	}
}

func hasLine(v updatesStatusView, text string) bool {
	for _, l := range v.Lines {
		if l.Text == text {
			return true
		}
	}
	return false
}

func TestUpdatesScreen_installRows(t *testing.T) {
	u := &fakeUpdater{v: installVerdict()} // the test double from this file
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)
	rows := s.visibleRows()
	if rows[len(rows)-1] != uRowInstall || s.rowLabel(uRowInstall) != "Install v1.1.0-rc5" {
		t.Fatalf("rows %v, label %q", rows, s.rowLabel(uRowInstall))
	}
	s.cursor = uRowInstall
	s.activate()
	if !u.started {
		t.Fatal("A on Install must start the install")
	}
	u.inst = appupdate.InstallStatus{State: appupdate.InstallStaged, Tag: "v1.1.0-rc5"}
	if s.rowLabel(uRowInstall) != "Restart now" {
		t.Fatalf("staged label %q", s.rowLabel(uRowInstall))
	}
	if next := s.activate(); next != nil || !u.restarted {
		t.Fatal("Restart now must request a restart and leave the event loop (nil screen)")
	}
	u.v.FailedInstall = "v1.1.0-rc5"
	u.inst = appupdate.InstallStatus{}
	if s.rowLabel(uRowInstall) != "Retry v1.1.0-rc5" {
		t.Fatalf("failed label %q", s.rowLabel(uRowInstall))
	}
}

func TestUpdatesScreen_backCancelsAnInstall(t *testing.T) {
	u := &fakeUpdater{v: installVerdict(), inst: appupdate.InstallStatus{State: appupdate.InstallRunning}}
	s := NewUpdatesScreen(&settings.Config{}, "", u, nil)
	if next := s.cancelOrBack(); next != s || !u.cancelled {
		t.Fatal("B while installing must cancel and stay")
	}
	if !s.IsBusy() {
		t.Fatal("a running install keeps the app awake")
	}
}
```

If the existing test double has a different name than `fakeUpdater`, use that name throughout.

- [ ] **Step 2: Run to verify they fail**

Run: `podman run --rm -v "$(pwd):/workspace" -w /workspace itchio-dev go test ./internal/ui/`
Expected: compile failure.

- [ ] **Step 3: Implement**

`app_updater.go` — add to `AppUpdater`:

```go
	StartInstall()
	CancelInstall()
	InstallStatus() appupdate.InstallStatus
	RequestRestart()
	// SourceOverride is the test update source in use, "" normally.
	SourceOverride() string
```

`dev_scenes_updates.go` — add an `inst appupdate.InstallStatus` field to `devAppUpdater` and:

```go
func (d *devAppUpdater) StartInstall()                          {}
func (d *devAppUpdater) CancelInstall()                         {}
func (d *devAppUpdater) InstallStatus() appupdate.InstallStatus { return d.inst }
func (d *devAppUpdater) RequestRestart()                        {}
func (d *devAppUpdater) SourceOverride() string                 { return "" }
```

and three scenes beside `updates-available`, reusing `rcAvail` with `Via: appupdate.ViaInstall`:

```go
		Scene{Name: "updates-install", Desc: "Updates: NextUI, Install row", Build: func(d SceneDeps) Screen {
			v := rcAvail
			v.Via = appupdate.ViaInstall
			s := devUpdates(d, &devAppUpdater{at: time.Now(), v: v})
			s.(*UpdatesScreen).cursor = uRowInstall
			return s
		}},
		Scene{Name: "updates-installing", Desc: "Updates: NextUI, downloading the update", Build: func(d SceneDeps) Screen {
			v := rcAvail
			v.Via = appupdate.ViaInstall
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: v, inst: appupdate.InstallStatus{
				State: appupdate.InstallRunning, Tag: v.Latest.Tag, Done: 6 << 20, Total: 14 << 20}})
		}},
		Scene{Name: "updates-staged", Desc: "Updates: NextUI, ready to restart", Build: func(d SceneDeps) Screen {
			v := rcAvail
			v.Via = appupdate.ViaInstall
			s := devUpdates(d, &devAppUpdater{at: time.Now(), v: v, inst: appupdate.InstallStatus{
				State: appupdate.InstallStaged, Tag: v.Latest.Tag}})
			s.(*UpdatesScreen).cursor = uRowInstall
			return s
		}},
```

`screen_updates.go`:

- add `uRowInstall // NextUI and ViaInstall only` after `uRowArchive`;
- add

```go
func (s *UpdatesScreen) installRowShown() bool {
	v, st := s.up.Verdict(), s.up.InstallStatus()
	if st.State == appupdate.InstallStaged {
		return true
	}
	return v.Kind == appupdate.Available && v.Via == appupdate.ViaInstall && st.State != appupdate.InstallRunning
}
```

- in `visibleRows`, after the archive append: `if s.installRowShown() { rows = append(rows, uRowInstall) }`;
- in `clampCursor`, add `if s.cursor == uRowInstall && !s.installRowShown() { s.cursor = uRowCheck }`;
- `IsBusy` and `NeedsRedraw` also return true when `s.up.InstallStatus().State == appupdate.InstallRunning`;
- in `rowLabel` add:

```go
	case uRowInstall:
		v, st := s.up.Verdict(), s.up.InstallStatus()
		switch {
		case st.State == appupdate.InstallStaged:
			return "Restart now"
		case v.FailedInstall != "" || st.State == appupdate.InstallFailed:
			return "Retry " + v.Latest.Tag
		default:
			return "Install " + v.Latest.Tag
		}
```

- in `activate` add:

```go
	case uRowInstall:
		if s.up.InstallStatus().State == appupdate.InstallStaged {
			logger.Info("updates: Restart now requested")
			s.up.RequestRestart()
			return nil // leave the event loop; main applies the update
		}
		logger.Info("updates: Install requested")
		s.up.StartInstall()
```

- in `cancelOrBack`, before the archive check:

```go
	if s.up.InstallStatus().State == appupdate.InstallRunning {
		s.up.CancelInstall()
		return s
	}
```

- in `Draw`, read `inst := s.up.InstallStatus()`, pass it and `s.up.SourceOverride()` to `updatesStatus`, draw the progress bar when `a.State == appupdate.ArchiveRunning || (inst.State == appupdate.InstallRunning && inst.Phase == appupdate.PhaseDownloading)` using `inst.Done/inst.Total` for the install case, and set the footer `back.Text = "Cancel"` while installing, `"Later"` when `inst.State == appupdate.InstallStaged`.

`updates_status.go` — change the signature to
`func updatesStatus(enabled bool, v appupdate.Verdict, a appupdate.ArchiveStatus, inst appupdate.InstallStatus, offline bool, lastErr error, rateUntil time.Time, override string) updatesStatusView`
(update its only caller in `Draw`), and after the archive `switch` add:

```go
	installState := appupdate.InstallIdle
	if v.Latest != nil && inst.Tag == v.Latest.Tag {
		installState = inst.State
	}
	switch installState {
	case appupdate.InstallRunning:
		verb := map[appupdate.InstallPhase]string{
			appupdate.PhaseDownloading: "Downloading", appupdate.PhaseChecking: "Checking", appupdate.PhaseUnpacking: "Unpacking",
		}[inst.Phase]
		add(fmt.Sprintf("%s %s…", verb, inst.Tag), false)
	case appupdate.InstallStaged:
		add(fmt.Sprintf("%s is ready. Restart to finish installing, or it installs the next time you open Itch-io.", inst.Tag), false)
	case appupdate.InstallFailed:
		add(installFailure(inst.Err), true)
	}
	if v.FailedInstall != "" && installState == appupdate.InstallIdle {
		add(fmt.Sprintf("%s did not start, so %s was kept.", v.FailedInstall, v.FailedFrom), true)
	}
	if override != "" {
		add("Test update source: "+override, true)
	}
```

and below `archiveFailure`:

```go
func installFailure(err error) string {
	var space *appupdate.SpaceError
	switch {
	case errors.Is(err, context.Canceled):
		return "Install cancelled."
	case errors.As(err, &space):
		return fmt.Sprintf("Not enough space on the SD card: needs %d MB, %d MB free.", space.Need>>20, space.Have>>20)
	case errors.Is(err, appupdate.ErrIntegrity):
		return "The download failed the integrity check."
	case errors.Is(err, appupdate.ErrDamaged):
		return strings.TrimSuffix(err.Error(), ".") + "."
	default:
		if m, ok := netstate.Describe(err, "GitHub"); ok {
			return m.Title
		}
		return "The update could not be installed."
	}
}
```

(`ErrDamaged`'s message is "the update is damaged: <reason>"; capitalise the first letter: replace the `ErrDamaged` case's return with `s := err.Error(); return strings.ToUpper(s[:1]) + strings.TrimSuffix(s[1:], ".") + "."`.) Add `"strings"` to the imports.

- [ ] **Step 4: Run tests and the palette audit for the new scenes**

Run: `./scripts/test.sh`
Expected: PASS, including the non-headless `internal/ui` pass and `palette-audit.sh` over the three new scenes at all four geometries. Look at one render: `go run ./cmd/devshot --scene updates-staged --out-dir /tmp/itchio-screenshots` (inside the `itchio-dev` container) and check the row and status line fit at 640×480.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/
git commit -m "ui: Install, Restart now and rollback report on Settings → App updates"
```

---

### Task 10: Wire it into start-up, the event loop and exit

**Files:**
- Modify: `cmd/itchio-pak/main.go` (after the startup banner, ~line 92; after `runSDL()`, line 171)
- Modify: `cmd/itchio-pak/main_sdl.go` (`func runSDL()` signature; checker `Config` at line 252; `partfile.Sweep` dirs at line 293; event loop end)
- Modify: `cmd/itchio-pak/main_headless.go`

**Interfaces:**
- Consumes: `env.PakDir()`, `appupdate.DiscardIncompleteStaged`, `StagedReady`, `SwapStaged`, `ExecLauncher`, `ConfirmStarted`, `TakeFailed`, `TakeLauncherLog`, `OverrideBase`, `NewOverrideSource`, `Checker.RestartRequested`.
- Produces: `func runSDL() (restart bool)` in both build variants.

- [ ] **Step 1: Change the `runSDL` signature**

`main_headless.go`:

```go
// runSDL is a no-op stub used when building with -tags headless (CI / unit tests).
func runSDL() bool { return false }
```

`main_sdl.go`: `func runSDL() bool {`, and every bare `return` inside `runSDL` becomes `return false`. The two `os.Exit(1)` calls stay.

- [ ] **Step 2: Apply a staged update at start-up ("Later") and after "Restart now"**

In `main.go`, add a helper below `main`:

```go
// applyStagedUpdate swaps a staged update in and execs its launcher. It
// returns only if there was nothing to apply or applying failed, in which
// case the current version carries on.
func applyStagedUpdate(env *firmware.Env, why string) {
	pakDir := env.PakDir()
	if pakDir == "" {
		return
	}
	appupdate.DiscardIncompleteStaged(pakDir)
	tag, ok := appupdate.StagedReady(pakDir)
	if !ok {
		return
	}
	logger.Info("update: applying staged %s (%s)", tag, why)
	if err := appupdate.SwapStaged(pakDir, env.DataDir(), version); err != nil {
		logger.Error("update: could not apply %s, carrying on as %s: %v", tag, version, err)
		_ = os.RemoveAll(appupdate.StagedDir(pakDir))
		return
	}
	// logger writes straight to the file (no buffer), so nothing is lost
	// when exec replaces this process.
	if err := appupdate.ExecLauncher(pakDir); err != nil {
		// The new version is in place but did not start: its launcher was not
		// run, so nothing will roll back. Put the old one back by hand.
		logger.Error("update: exec failed, restoring %s", version)
		_ = os.Rename(pakDir, appupdate.FailedDir(pakDir))
		_ = os.Rename(appupdate.PrevDir(pakDir), pakDir)
		_ = os.Remove(filepath.Join(env.DataDir(), "update_pending.json"))
	}
}
```

Add the `appupdate` import to `main.go`.

In `main()`, immediately after `logRomDirs(env)`:

```go
	// An update staged and left for "the next time you open Itch-io".
	applyStagedUpdate(env, "staged earlier")
	appupdate.TakeLauncherLog(env.DataDir())
```

and replace the bare `runSDL()` call with:

```go
	if runSDL() {
		applyStagedUpdate(env, "restart requested")
	}
```

- [ ] **Step 3: Wire the checker**

In `main_sdl.go`, replace the `appUpd := appupdate.NewChecker(appupdate.Config{…})` block with:

```go
	statePath := filepath.Join(dataDir, "update_state.json")
	var src *appupdate.Source
	override, overridden := appupdate.OverrideBase(dataDir)
	ua := itchio.BuildUserAgent(itchio.UAInfoFromEnv(version, env), false)
	if overridden {
		logger.Warn("appupdate: test update source %s in use (remove %s/update_source_override to stop)", override, dataDir)
		src = appupdate.NewOverrideSource(ua, override)
		statePath = filepath.Join(dataDir, "update_state.override.json")
	}
	failed, _ := appupdate.TakeFailed(dataDir)
	appUpd := appupdate.NewChecker(appupdate.Config{
		Firmware:   env.Kind(),
		Running:    version,
		Channel:    channel,
		StatePath:  statePath,
		StoreDB:    env.PakStoreDB(),
		ArchiveDir: env.ArchiveDir(),
		PakDir:     env.PakDir(),
		Failed:     failed,
		Override:   override,
		UserAgent:  ua,
		Source:     src,
		Notify: func() {
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
		},
	})
```

In the `partfile.Sweep` goroutine, before the call:

```go
		if d := env.PakDir(); d != "" {
			dirs = append(dirs, filepath.Dir(d))
		}
```

- [ ] **Step 4: Confirm after the first frame, and return the restart request**

In `main_sdl.go`, declare `confirmed := false` just before `loop:`. At the very end of the `for current != nil { … }` body (after the screen has drawn and presented for this iteration), add:

```go
		// The first full iteration has drawn a frame and read input: an
		// update that got this far has started (NextUI install spec §4.1).
		if !confirmed {
			confirmed = true
			appupdate.ConfirmStarted(env.PakDir(), dataDir)
		}
```

At the end of `runSDL` (after the loop and its existing clean-up, where the function currently ends), add `return appUpd.RestartRequested()`. All `defer`s in `runSDL` run before `main` continues, so SDL has released the display and controller before the exec.

- [ ] **Step 5: Build every target and run the suite**

Run: `./scripts/test.sh && ./scripts/build.sh nextui/tg5040`
Expected: tests PASS; the cross-compile succeeds (this is the only check that `main_sdl.go` compiles for the device — `-tags headless` skips it).

- [ ] **Step 6: Commit**

```bash
git add cmd/itchio-pak/
git commit -m "main: apply staged updates, confirm the first frame, restart into an update"
```

---

### Task 11: `scripts/update-fixture.sh`

**Files:**
- Create: `scripts/update-fixture.sh`
- Modify: `scripts/test.sh:64` (add the script to the shellcheck line)

**Interfaces:**
- Consumes: `scripts/adb.sh` (`adb_use nextui`, `$ADB_DEVICE`, `$PLATFORM` as `deploy.sh` uses them); the override file name and fixture layout from Task 8.

- [ ] **Step 1: Write the script**

`scripts/update-fixture.sh`:

```sh
#!/bin/sh
# Serves a NextUI pak zip as a fake GitHub release, so the in-place install can
# be tested on a device before two real releases carry the feature
# (docs/superpowers/specs/2026-10-07-nextui-in-place-install-design.md §6).
#
#   update-fixture.sh serve <pak.zip>            serve it as the newest rc
#   update-fixture.sh serve --broken <pak.zip>   same, but itchio replaced by the
#                                                device's busybox (exits at once:
#                                                the rollback case)
#   update-fixture.sh off                        stop using the override
#
# The device reaches the server through `adb reverse` on 127.0.0.1:8765.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
. "$SCRIPT_DIR/adb.sh"

PORT=8765
BASE="http://127.0.0.1:$PORT/"
DATA_DIR="/mnt/SDCARD/.userdata/shared/Itch-io"

usage() { sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

off() {
    adb_use nextui
    adb shell "rm -f '$DATA_DIR/update_source_override'" </dev/null
    adb reverse --remove "tcp:$PORT" 2>/dev/null || true
    echo "Override removed from $ADB_DEVICE."
}

serve() {
    broken=0
    if [ "${1:-}" = "--broken" ]; then broken=1; shift; fi
    [ $# -eq 1 ] && [ -f "$1" ] || usage
    adb_use nextui

    dir="$(mktemp -d)"
    trap 'rm -rf "$dir"; adb reverse --remove "tcp:$PORT" 2>/dev/null || true' EXIT INT TERM
    version="$(unzip -p "$1" pak.json | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    [ -n "$version" ] || { echo "no version in $1's pak.json" >&2; exit 1; }
    asset="Itch-io.NextUI.$version.pak.zip"
    cp "$1" "$dir/$asset"
    if [ "$broken" = 1 ]; then
        adb pull /bin/busybox "$dir/itchio" >/dev/null
        (cd "$dir" && zip -q "$asset" itchio && rm itchio)
        echo "Serving a BROKEN $version: itchio is busybox and exits at once."
    fi
    size="$(wc -c < "$dir/$asset" | tr -d ' ')"
    sum="$(sha256sum "$dir/$asset" | cut -d' ' -f1)"
    cat > "$dir/releases.json" <<EOF
[{"tag_name":"$version","html_url":"${BASE}release","draft":false,"prerelease":true,
  "assets":[{"name":"$asset","browser_download_url":"$BASE$asset","size":$size,"digest":"sha256:$sum"}]}]
EOF
    printf '{"version":"%s"}\n' "$version" > "$dir/pak.json"

    adb reverse "tcp:$PORT" "tcp:$PORT" >/dev/null
    adb shell "mkdir -p '$DATA_DIR' && echo '$BASE' > '$DATA_DIR/update_source_override'" </dev/null
    echo "Serving $version ($size bytes) on $BASE for $ADB_DEVICE. Open Settings → App updates → Check now."
    echo "Ctrl-C stops serving; run '$0 off' to remove the override."
    cd "$dir" && python3 -m http.server "$PORT" --bind 127.0.0.1
}

case "${1:-}" in
    serve) shift; serve "$@" ;;
    off)   off ;;
    *)     usage ;;
esac
```

`chmod +x scripts/update-fixture.sh`.

- [ ] **Step 2: Lint**

In `scripts/test.sh` line 64, append `"$SCRIPT_DIR/update-fixture.sh"` to the shellcheck file list. Run: `shellcheck -s sh scripts/update-fixture.sh`
Expected: clean. (`adb.sh` is sourced; add `# shellcheck source=scripts/adb.sh` above the `.` line if shellcheck asks.)

- [ ] **Step 3: Smoke-test without a device**

Run: `sh scripts/update-fixture.sh` → prints usage and exits 2.

- [ ] **Step 4: Commit**

```bash
git add scripts/update-fixture.sh scripts/test.sh
git commit -m "scripts: update-fixture.sh serves a pak zip as a fake release for install testing"
```

---

### Task 12: Hardware verification on the Brick

**Files:** none changed unless a defect is found (then fix with a test in the owning task's files and commit separately).

- [ ] **Step 1: Build and deploy the "old" version**

Set `pak.json` `"version"` to `v1.1.0-rc5`, then:

```bash
./scripts/release.sh --allow-dirty
./scripts/deploy.sh
cp dist/nextui/Itch-io.NextUI.v1.1.0-rc5.pak.zip /tmp/claude-scratch-rc5.pak.zip
```

Verify on the device: `adb shell "md5sum /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio"` matches `md5sum dist/nextui/Itch-io.pak/itchio`.

- [ ] **Step 2: Build the "new" version and serve it**

Set `pak.json` `"version"` to `v1.1.0-rc6`, `./scripts/release.sh --allow-dirty`, then in a second terminal:
`./scripts/update-fixture.sh serve dist/nextui/Itch-io.NextUI.v1.1.0-rc6.pak.zip`

- [ ] **Step 3: Install with Restart now**

On the Brick: open Itch-io → Settings → App updates → Check now. Expect "Test update source: …", `Install v1.1.0-rc6`. Press A, watch Downloading/Checking/Unpacking, then `Restart now`. Expect the app to restart into rc6. Verify:

```bash
adb shell "grep -E 'appupdate: (install|apply|exec|verify|staged)|update: ' /mnt/SDCARD/.userdata/tg5040/logs/itchio.log | tail -20; ls -a /mnt/SDCARD/Tools/tg5040 | grep Itch; grep version /mnt/SDCARD/Tools/tg5040/Itch-io.pak/pak.json" </dev/null
```

Expected: `confirmed`, no `.Itch-io.pak.*` left after a few seconds, pak.json says rc6.

- [ ] **Step 4: Install with Later**

Redeploy rc5 (`./scripts/deploy.sh` with the rc5 dist — rebuild it per Step 1), install again, press B on `Restart now`, quit, relaunch from Tools. Expect rc6 to start directly, same log lines with `(staged earlier)`.

- [ ] **Step 5: Cancel mid-download**

Redeploy rc5; press A on Install and immediately B. Expect "Install cancelled.", no `.Itch-io.pak.staged*` in `Tools/tg5040`.

- [ ] **Step 6: Rollback**

Redeploy rc5; `./scripts/update-fixture.sh serve --broken dist/nextui/Itch-io.NextUI.v1.1.0-rc6.pak.zip`; install, Restart now. Expect: the screen goes back to rc5 by itself, App updates says "v1.1.0-rc6 did not start, so v1.1.0-rc5 was kept." with `Retry v1.1.0-rc6`, the log has `launch.sh: … rollback` lines, the corner notice does not appear for rc6, `.Itch-io.pak.failed` is gone after the restored start.

- [ ] **Step 7: Clean up**

`./scripts/update-fixture.sh off`; `git checkout pak.json`; redeploy the branch's normal build. Record any defect found as a new failing test in the owning task's test file, fix, commit.

- [ ] **Step 8: Hand H700 verification to testers**

Note in the next rc's changelog that H700 testers should try Install (pak folder `Tools/h700/`, no `lib/`).

---

## Self-review notes

- Spec §1 → Task 1 (+ PakDir guard in Task 7). §2 → Task 9. §3 → Tasks 2–3. §3.1 → Tasks 4, 10. §4.1 → Tasks 4, 10. §4.2 → Task 6. §4.3 → Tasks 4, 7, 9. §5 → Tasks 5, 7, 10. §6 → Tasks 8, 10, 11. §7 logging → in every task's code. §8 → Tasks 3, 4, 6, 7, 9. Testing → every task + Task 12.
- Deviation from spec §2: "Later" is the B button on the staged screen (footer reads "Later"), not a separate row; the status line says it installs at the next launch. Same behaviour, one row fewer.
- Deviation from spec §4.3: notice suppression for a failed version is in `Checker.PendingNotice` (it has the state), not in the pure `ShouldNotify`.
