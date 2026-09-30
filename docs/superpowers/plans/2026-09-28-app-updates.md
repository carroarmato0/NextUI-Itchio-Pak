# App updates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tell users when a newer Itch-io exists, with Stable / Release-candidate / Off channels, a top-right notice, a Settings → Updates screen, Pak Store awareness on NextUI, and "Save to ARCHIVE" on muOS.

**Architecture:** Two new headless-safe packages carry all logic: `internal/pakstore` (a read-only SQLite reader for the Pak Store's install database) and `internal/appupdate` (versions, channels, GitHub sources, the verdict, persisted state, the ARCHIVE download, and a `Checker` that runs it all in the background). The UI reaches the checker through one `ui.AppUpdater` interface installed at startup; the notice is drawn through a new `Renderer.Overlay` hook inside `Present()`, so no existing screen changes.

**Tech Stack:** Go (stdlib `net/http`, `archive/zip`, `crypto/sha256`, `encoding/binary`), SDL2 via go-sdl2, `httptest` for tests, `sqlite3` CLI only to generate committed fixtures.

**Spec:** `docs/superpowers/specs/2026-09-26-app-updates-design.md` (refreshed 2026-09-27, reviewed 2026-09-28). Read it alongside this plan; §-numbers below refer to it.

**Worktree:** `.worktrees/1.1-app-updates`, branch `feature/1.1-app-updates`. Every path below is relative to it.

## Global Constraints

- Module path: `github.com/carroarmato0/nextui-itchio-pak`.
- Repo for update checks: `carroarmato0/NextUI-Itchio-Pak`. Releases URL `https://api.github.com/repos/carroarmato0/NextUI-Itchio-Pak/releases?per_page=30`; pak.json URL `https://raw.githubusercontent.com/carroarmato0/NextUI-Itchio-Pak/refs/heads/main/pak.json`.
- muOS asset name: `Itch-io.muOS.<tag>.muxapp`. Asset `digest` format: `sha256:<hex>`.
- Pak Store: database `<SD>/.userdata/<PLATFORM>/nextui-pak-store/pak-store.db`, table `installed_paks(name, display_name, pak_id, repo_url, type, version, can_uninstall)`, Itch-io `pak_id = "3JM2zY4UKh"`, fallback `name = "Itch-io"`.
- `config.json` key `update_channel`: `"stable" | "rc" | "off"`, empty = not chosen. State file `update_state.json` in the data directory.
- Checker HTTP: 10 s timeout, bundled CA via `SSL_CERT_FILE` (Go's default roots honour it — do not disable verification), User-Agent `itchio.BuildUserAgent(info, false)` (no device details to GitHub), transport wrapped in `netstate.Transport`.
- ARCHIVE download: 30 s idle timeout that also covers the wait for headers, free space ≥ size + 10 %, `partfile.Create` → verify → `Commit()`; `Abort()` on any failure.
- **Never offer, save or announce a version lower than or equal to the running one** (§2a).
- Notice: top-right, inset by `noticeMargin(w,h)` = 12 px, 6 px when `compact(w,h)`; slide 250 ms ease-out, hold 4 s, slide 250 ms; Accent fill, AccentText, 1 px `ModalBorder()` outline.
- No colour literals in drawing code; every colour from `internal/theme` (`scripts/no-color-literals.sh` enforces).
- No `fmt.Println`/`log.Printf`; use `internal/logger` (`Debug`/`Info`/`Warn`/`Error`). Every goroutine start, HTTP call, file write and error path is logged (memory "Logging standards"). Never log a URL from `SaveToArchive`'s response — use `netstate.Detail(err)`.
- SDL code is `//go:build !headless`; `internal/appupdate` and `internal/pakstore` have no build tag and no SDL import.
- Tests use `httptest.NewServer` and fixtures under `testdata/`; no live network.
- Screenshots go to `/tmp/itchio-screenshots/`, never `docs/screenshots/`.
- Commit messages: `<area>: <sentence>` (e.g. `appupdate: parse release tags`), ending with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.

## Review Focus

1. **A check still running when the user switches channel** — no second concurrent request, and the running check's result (which fills both channels) is what the new channel shows. Test: `TestChecker_setChannelWhileBusyDoesNotStartSecond` (Task 9).
2. **The Save to ARCHIVE row disappearing under the cursor** (download starts, or the verdict changes) — the cursor lands on a visible row and a press still wraps. Test: `TestUpdatesCursor_hiddenArchiveRowClamps` (Task 10).
3. **An `update_state.json` truncated by a power cut** — loads as empty, the next save repairs it, and no notice is lost or repeated beyond "shown once more". Test: `TestLoadState_corruptIsEmpty` (Task 3).
4. **The notice firing on the sign-in, account-prompt, loading or Updates screen** — it waits and appears on the next allowed screen. Test: `TestNoticeAllowed` (Task 11).
5. **A crash mid-download leaving `ARCHIVE/.Itch-io.muOS.<tag>.muxapp.itchio-part`** — removed at next launch by `partfile.Sweep` with `ArchiveDir()`. Test: `TestSweepRemovesArchiveLeftover` (Task 8).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/appupdate/version.go` | `Version`, `Parse`, `Compare`, `StoreCompare` |
| `internal/appupdate/channel.go` | `Channel`, `ResolveChannel`, `Next`, `Label` |
| `internal/appupdate/state.go` | `Release`, `State`, `LoadState`, `Save` |
| `internal/appupdate/source.go` | GitHub releases + pak.json fetch, ETag, rate limit |
| `internal/appupdate/decide.go` | `Verdict`, `Decide`, `ShouldNotify` |
| `internal/appupdate/archive.go` | `SaveToArchive`, `ValidateMuxapp`, `CleanOtherArchives` |
| `internal/appupdate/freespace_linux.go`, `freespace_other.go` | `freeBytes(dir)` |
| `internal/appupdate/checker.go` | `Checker`: background checks, owed/reconnect, channel switch, notice bookkeeping, ARCHIVE job |
| `internal/pakstore/pakstore.go` | `Lookup(dbPath) Result` |
| `internal/pakstore/sqlite.go` | minimal SQLite file-format reader |
| `testdata/pakstore/make.sh` + `*.db` | fixtures built with `sqlite3` |
| `testdata/appupdate/*.json` | GitHub releases / pak.json fixtures |
| `internal/firmware/{firmware,nextui,muos}.go` | `ArchiveDir()`, `PakStoreDB()` |
| `internal/settings/settings.go` | `UpdateChannel` field |
| `internal/renderer/renderer.go` | `Overlay` hook in `Present()` |
| `internal/ui/app_updater.go` | `AppUpdater` interface, `SetAppUpdater` |
| `internal/ui/update_toast.go` | `UpdateNotice`, notice text/timing/drawing, `noticeAllowed` |
| `internal/ui/screen_updates.go` | Settings → Updates screen |
| `internal/ui/updates_status.go` | pure status-text builder for the Updates screen |
| `internal/ui/screen_settings.go` | "Updates >" row + annotation, shared annotation colour/time helpers |
| `internal/ui/dev_scenes_updates.go` | offscreen scenes + `devAppUpdater` |
| `cmd/itchio-pak/main_sdl.go` | wiring |

---

### Task 1: Versions and the Store's comparison

**Files:**
- Create: `internal/appupdate/version.go`
- Test: `internal/appupdate/version_test.go`

**Interfaces:**
- Produces: `type Version struct{ Major, Minor, Patch, RC int }`; `Parse(s string) (Version, bool)`; `Compare(a, b Version) int`; `(Version) IsRC() bool`; `(Version) String() string`; `StoreCompare(a, b string) int`.

- [ ] **Step 1: Write the failing test**

```go
package appupdate

import "testing"

func TestParse(t *testing.T) {
	good := map[string]Version{
		"v1.1.0":      {1, 1, 0, 0},
		"1.1.0-rc3":   {1, 1, 0, 3},
		" v1.0.25 ":   {1, 0, 25, 0},
		"v2.10.3-rc12": {2, 10, 3, 12},
	}
	for in, want := range good {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"dev", "", "v1.1", "v1.1.0.1", "v1.1.0-rc", "v1.1.0-rc0",
		"v1.1.0-beta1", "v+1.1.0", "v1.1.0-rc-1", "vv1.1.0", "v1.1.x"} {
		if v, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %+v, true; want not a version", in, v)
		}
	}
}

func TestCompareOrdering(t *testing.T) {
	ordered := []string{"v1.0.25", "v1.1.0-rc1", "v1.1.0-rc2", "v1.1.0-rc10", "v1.1.0",
		"v1.1.1-rc1", "v1.2.0", "v2.0.0-rc1"}
	for i := range ordered {
		for j := range ordered {
			a, _ := Parse(ordered[i])
			b, _ := Parse(ordered[j])
			got := Compare(a, b)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}

func TestVersionString(t *testing.T) {
	for _, s := range []string{"v1.1.0", "v1.1.0-rc3", "v0.0.1"} {
		v, _ := Parse(s)
		if v.String() != s {
			t.Errorf("String() = %q, want %q", v.String(), s)
		}
	}
}

// Parity with LoveRetro/nextui-pak-store state/helpers.go compareVersions.
func TestStoreCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.1.0-rc1", "v1.1.0", 0}, // the trap: "0-rc1" reads as 0
		{"v1.1.0-rc2", "v1.1.0", 0},
		{"v1.0.23", "v1.0.25", -1},
		{"v1.0.25", "v1.1.0-rc3", -1},
		{"1.2", "1.2.0", 0},
		{"v2.0.0", "v1.9.9", 1},
		{"", "v1.0.0", -1},
	}
	for _, c := range cases {
		if got := StoreCompare(c.a, c.b); got != c.want {
			t.Errorf("StoreCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run 'TestParse|TestCompare|TestVersionString|TestStoreCompare' -v`
Expected: FAIL — `undefined: Parse` (package does not exist yet).

- [ ] **Step 3: Write minimal implementation**

```go
// Package appupdate tells the user when a newer Itch-io exists: it checks
// GitHub, decides what to say, and on muOS saves the update for Archive
// Manager. It has no SDL dependency, so all of it is unit-tested headless.
package appupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a release tag: vMAJOR.MINOR.PATCH with an optional -rcN.
type Version struct {
	Major, Minor, Patch int
	RC                  int // 0 for a final release
}

// Parse reads a release tag; the leading "v" is optional. Anything else —
// "dev", "1.1", "v1.1.0-beta1" — is not a version this app compares, and no
// update check runs for a build carrying it.
func Parse(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, rc, hasRC := strings.Cut(s, "-rc")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var n [3]int
	for i, p := range parts {
		v, ok := number(p)
		if !ok {
			return Version{}, false
		}
		n[i] = v
	}
	out := Version{Major: n[0], Minor: n[1], Patch: n[2]}
	if hasRC {
		r, ok := number(rc)
		if !ok || r == 0 {
			return Version{}, false
		}
		out.RC = r
	}
	return out, true
}

// number accepts plain decimal digits only; Atoi alone would also take "+1".
func number(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// Compare orders versions: 1.1.0-rc1 < 1.1.0-rc2 < 1.1.0 < 1.1.1-rc1.
func Compare(a, b Version) int {
	for _, d := range [3][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.RC == b.RC:
		return 0
	case a.RC == 0: // a final release is newer than all of its candidates
		return 1
	case b.RC == 0:
		return -1
	case a.RC < b.RC:
		return -1
	default:
		return 1
	}
}

// IsRC reports whether v is a release candidate.
func (v Version) IsRC() bool { return v.RC > 0 }

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.RC > 0 {
		s += fmt.Sprintf("-rc%d", v.RC)
	}
	return s
}

// StoreCompare is the Pak Store's compareVersions (LoveRetro/nextui-pak-store,
// state/helpers.go), ported line for line. It is used only to predict what the
// Store will show. It reads "0-rc1" as 0, so v1.1.0-rc1 equals v1.1.0: the
// Store can never tell a release candidate from its final release.
func StoreCompare(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		var numA, numB int

		if i < len(partsA) {
			numA, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			numB, _ = strconv.Atoi(partsB[i])
		}

		if numA < numB {
			return -1
		}
		if numA > numB {
			return 1
		}
	}

	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/appupdate/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/version.go internal/appupdate/version_test.go
git commit -m "appupdate: parse and order release tags, port the Pak Store's comparison"
```

---

### Task 2: Channels and the config field

**Files:**
- Create: `internal/appupdate/channel.go`
- Test: `internal/appupdate/channel_test.go`
- Modify: `internal/settings/settings.go:50-75` (add field)
- Test: `internal/settings/settings_test.go`

**Interfaces:**
- Consumes: `Version`, `Parse` (Task 1).
- Produces: `type Channel string`; consts `Stable = "stable"`, `RC = "rc"`, `Off = "off"`; `ResolveChannel(stored string, running Version) (ch Channel, pin bool)`; `(Channel) Next() Channel`; `(Channel) Label() string`; `settings.Config.UpdateChannel string` (`json:"update_channel,omitempty"`).

- [ ] **Step 1: Write the failing tests**

`internal/appupdate/channel_test.go`:

```go
package appupdate

import "testing"

func TestResolveChannel(t *testing.T) {
	rc, _ := Parse("v1.1.0-rc3")
	stable, _ := Parse("v1.1.0")
	cases := []struct {
		stored  string
		running Version
		want    Channel
		pin     bool
	}{
		{"", rc, RC, true},          // an rc tester keeps hearing about rcs...
		{"", stable, Stable, false}, // ...a stable user never does
		{"", Version{}, Stable, false},
		{"stable", rc, Stable, false}, // an explicit choice always wins
		{"rc", stable, RC, false},     // installing the final release keeps RC
		{"off", rc, Off, false},
		{"beta", rc, RC, true}, // garbage reads as "not chosen"
	}
	for _, c := range cases {
		got, pin := ResolveChannel(c.stored, c.running)
		if got != c.want || pin != c.pin {
			t.Errorf("ResolveChannel(%q, %v) = %s, %v; want %s, %v", c.stored, c.running, got, pin, c.want, c.pin)
		}
	}
}

func TestChannelNextCycles(t *testing.T) {
	if Stable.Next() != RC || RC.Next() != Off || Off.Next() != Stable {
		t.Fatal("Next must cycle Stable → RC → Off → Stable")
	}
	if Stable.Label() != "Stable" || RC.Label() != "Release candidates" || Off.Label() != "Off" {
		t.Fatal("unexpected labels")
	}
}
```

Append to `internal/settings/settings_test.go`:

```go
func TestUpdateChannel_roundTripAndDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"rom_location":"auto"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpdateChannel != "" {
		t.Fatalf("old config: UpdateChannel = %q, want empty (not chosen)", cfg.UpdateChannel)
	}
	cfg.UpdateChannel = "rc"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	again, _ := Load(path)
	if again.UpdateChannel != "rc" {
		t.Fatalf("after save: UpdateChannel = %q, want rc", again.UpdateChannel)
	}
}
```

(Add `"os"` / `"path/filepath"` to the test file's imports if they are not there.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/appupdate/ ./internal/settings/ -run 'Channel' -v`
Expected: FAIL — `undefined: ResolveChannel`, `cfg.UpdateChannel undefined`.

- [ ] **Step 3: Implement**

`internal/appupdate/channel.go`:

```go
package appupdate

// Channel is which releases the user wants to hear about.
type Channel string

const (
	Stable Channel = "stable" // releases with prerelease=false
	RC     Channel = "rc"     // every non-draft release, final ones included
	Off    Channel = "off"    // no request is made
)

// ResolveChannel turns config.json's update_channel into the channel in use.
// Empty (or unrecognised) means not chosen yet, and follows the running build:
// rc for a release candidate, stable otherwise. pin reports that "rc" should
// now be written, so a tester who installs the final release is not silently
// dropped to Stable. Stable is never pinned: a stable user who side-loads an
// rc has shown they want rcs, and gets them the same way (spec §1).
func ResolveChannel(stored string, running Version) (ch Channel, pin bool) {
	switch c := Channel(stored); c {
	case Stable, RC, Off:
		return c, false
	}
	if running.IsRC() {
		return RC, true
	}
	return Stable, false
}

// Next is the channel A cycles to on the Updates screen.
func (c Channel) Next() Channel {
	switch c {
	case Stable:
		return RC
	case RC:
		return Off
	default:
		return Stable
	}
}

// Label is the channel's name on screen.
func (c Channel) Label() string {
	switch c {
	case RC:
		return "Release candidates"
	case Off:
		return "Off"
	default:
		return "Stable"
	}
}
```

In `internal/settings/settings.go`, inside `type Config struct`, after the `ShareDeviceInfo` field:

```go
	// UpdateChannel is "stable", "rc" or "off"; empty until chosen, when it
	// follows the running build (appupdate.ResolveChannel).
	UpdateChannel string `json:"update_channel,omitempty"`
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/appupdate/ ./internal/settings/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/channel.go internal/appupdate/channel_test.go internal/settings/settings.go internal/settings/settings_test.go
git commit -m "appupdate: update channels, pinned to rc for release-candidate builds"
```

---

### Task 3: Persisted update state

**Files:**
- Create: `internal/appupdate/state.go`
- Test: `internal/appupdate/state_test.go`

**Interfaces:**
- Consumes: `Channel` (Task 2).
- Produces:
  ```go
  type Release struct { Tag, URL, Asset, Digest string; Size int64 } // json: tag,url,asset,digest,size
  type State struct {
      CheckedAt time.Time; NotBefore time.Time
      ETag map[string]string            // keys etagReleases, etagPakJSON
      Latest map[Channel]*Release
      PakJSON *Release                  // what main's pak.json says
      Notified map[Channel]string
  }
  const etagReleases = "releases"; const etagPakJSON = "pakjson"
  func LoadState(path string) *State   // never nil, maps non-nil
  func (s *State) Save(path string) error
  ```

- [ ] **Step 1: Write the failing test**

```go
package appupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_missingIsEmpty(t *testing.T) {
	st := LoadState(filepath.Join(t.TempDir(), "update_state.json"))
	if st.Latest == nil || st.Notified == nil || st.ETag == nil {
		t.Fatal("maps must be non-nil so callers can assign into them")
	}
}

func TestLoadState_corruptIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_state.json")
	// What a power cut mid-write leaves behind.
	os.WriteFile(path, []byte(`{"checked_at":"2026-09-26T14:00:00Z","latest":{"stable":{"tag":"v1.`), 0644)
	st := LoadState(path)
	if !st.CheckedAt.IsZero() || len(st.Latest) != 0 {
		t.Fatalf("corrupt file must load as empty, got %+v", st)
	}
	st.Notified[RC] = "v1.1.0-rc3"
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	if LoadState(path).Notified[RC] != "v1.1.0-rc3" {
		t.Fatal("the next save must repair the file")
	}
}

func TestState_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_state.json")
	st := LoadState(path)
	st.CheckedAt = time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	st.ETag[etagReleases] = `W/"abc"`
	st.Latest[Stable] = &Release{Tag: "v1.0.25", URL: "u", Asset: "a", Digest: "sha256:00", Size: 12678524}
	st.PakJSON = &Release{Tag: "v1.0.25", URL: "u"}
	st.Notified[Stable] = "v1.0.25"
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("Save must not leave its temporary file behind")
	}
	got := LoadState(path)
	if !got.CheckedAt.Equal(st.CheckedAt) || got.ETag[etagReleases] != `W/"abc"` ||
		*got.Latest[Stable] != *st.Latest[Stable] || got.PakJSON.Tag != "v1.0.25" ||
		got.Notified[Stable] != "v1.0.25" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run State -v`
Expected: FAIL — `undefined: LoadState`.

- [ ] **Step 3: Implement**

```go
package appupdate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Release is one release as far as updates care.
type Release struct {
	Tag    string `json:"tag"`
	URL    string `json:"url"`              // the release page (html_url)
	Asset  string `json:"asset,omitempty"`  // muOS .muxapp download URL
	Digest string `json:"digest,omitempty"` // "sha256:<hex>"
	Size   int64  `json:"size,omitempty"`
}

// ETag keys in State.ETag, one per source.
const (
	etagReleases = "releases"
	etagPakJSON  = "pakjson"
)

// State is update_state.json. It is not a user choice, so it lives apart
// from config.json.
type State struct {
	CheckedAt time.Time            `json:"checked_at"`
	NotBefore time.Time            `json:"not_before"`
	ETag      map[string]string    `json:"etag,omitempty"`
	Latest    map[Channel]*Release `json:"latest,omitempty"`
	// PakJSON is what pak.json on main says: the Pak Store's own source.
	PakJSON  *Release          `json:"pakjson,omitempty"`
	Notified map[Channel]string `json:"notified,omitempty"`
}

// LoadState reads path. A missing or corrupt file is an empty state: the worst
// outcome is one extra check and one notice shown again.
func LoadState(path string) *State {
	st := &State{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		logger.Debug("appupdate: no %s yet", path)
	case err != nil:
		logger.Warn("appupdate: read %s: %v", path, err)
	default:
		if err := json.Unmarshal(data, st); err != nil {
			logger.Warn("appupdate: %s is corrupt, starting empty: %v", path, err)
			st = &State{}
		}
	}
	if st.ETag == nil {
		st.ETag = map[string]string{}
	}
	if st.Latest == nil {
		st.Latest = map[Channel]*Release{}
	}
	if st.Notified == nil {
		st.Notified = map[Channel]string{}
	}
	return st
}

// Save writes the state atomically (tmp + rename), like settings.Save.
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Error("appupdate: write %s: %v", tmp, err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		logger.Error("appupdate: rename %s → %s: %v", tmp, path, err)
		return err
	}
	logger.Debug("appupdate: state saved to %s", path)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/appupdate/ -run State -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/state.go internal/appupdate/state_test.go
git commit -m "appupdate: persist check results and notices in update_state.json"
```

---

### Task 4: GitHub sources

**Files:**
- Create: `internal/appupdate/source.go`
- Create: `testdata/appupdate/releases.json`, `testdata/appupdate/pak.json`
- Test: `internal/appupdate/source_test.go`

**Interfaces:**
- Consumes: `Parse`, `Compare`, `Release`.
- Produces:
  ```go
  func AssetName(tag string) string                        // "Itch-io.muOS.<tag>.muxapp"
  type Source struct { HTTP *http.Client; UserAgent, ReleasesURL, PakJSONURL string; Now func() time.Time }
  func NewSource(userAgent string) *Source
  func newClient(timeout time.Duration) *http.Client       // netstate-wrapped; 0 = no overall timeout
  type ReleasesResult struct { Stable, RC *Release; ETag string; NotModified bool }
  type RateLimitError struct { Until time.Time }            // Error()
  func (s *Source) Releases(ctx context.Context, etag string) (ReleasesResult, error)
  func (s *Source) PakJSON(ctx context.Context, etag string) (rel *Release, newETag string, notModified bool, err error)
  ```
  5xx and other unwanted statuses return `*netstate.StatusError`.

- [ ] **Step 1: Create the fixtures**

`testdata/appupdate/releases.json` — a draft (ignored), two prereleases, a stable with digest, an older stable without one, and a tag that is not a version:

```json
[
  {"tag_name": "v1.2.0", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.2.0", "draft": true, "prerelease": false,
   "assets": [{"name": "Itch-io.muOS.v1.2.0.muxapp", "browser_download_url": "https://example.invalid/v1.2.0.muxapp", "size": 1, "digest": "sha256:aa"}]},
  {"tag_name": "nightly", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/nightly", "draft": false, "prerelease": true, "assets": []},
  {"tag_name": "v1.1.0-rc3", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.1.0-rc3", "draft": false, "prerelease": true,
   "assets": [
     {"name": "Itch-io.pak.v1.1.0-rc3.zip", "browser_download_url": "https://example.invalid/pak.zip", "size": 13000000, "digest": "sha256:bb"},
     {"name": "Itch-io.muOS.v1.1.0-rc3.muxapp", "browser_download_url": "https://example.invalid/v1.1.0-rc3.muxapp", "size": 12819887, "digest": "sha256:cc"}]},
  {"tag_name": "v1.1.0-rc2", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.1.0-rc2", "draft": false, "prerelease": true,
   "assets": [{"name": "Itch-io.muOS.v1.1.0-rc2.muxapp", "browser_download_url": "https://example.invalid/v1.1.0-rc2.muxapp", "size": 12819887, "digest": "sha256:dd"}]},
  {"tag_name": "v1.0.25", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.0.25", "draft": false, "prerelease": false,
   "assets": [
     {"name": "Itch-io.pak.zip", "browser_download_url": "https://example.invalid/Itch-io.pak.zip", "size": 12678524, "digest": "sha256:ee"},
     {"name": "Itch-io.muOS.v1.0.25.muxapp", "browser_download_url": "https://example.invalid/v1.0.25.muxapp", "size": 12678524, "digest": "sha256:ff"}]},
  {"tag_name": "v1.0.24", "html_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.0.24", "draft": false, "prerelease": false,
   "assets": [{"name": "Itch-io.muOS.v1.0.24.muxapp", "browser_download_url": "https://example.invalid/v1.0.24.muxapp", "size": 12000000}]}
]
```

`testdata/appupdate/pak.json`:

```json
{"name":"Itch-io","version":"v1.0.25","type":"TOOL","repo_url":"https://github.com/carroarmato0/NextUI-Itchio-Pak","release_filename":"Itch-io.pak.zip"}
```

- [ ] **Step 2: Write the failing test**

```go
package appupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/appupdate/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testSource(url string) *Source {
	s := NewSource("NextUI-Itchio-Pak/1.1.0 (+https://github.com/carroarmato0/NextUI-Itchio-Pak)")
	s.ReleasesURL, s.PakJSONURL = url+"/releases", url+"/pak.json"
	return s
}

func TestReleases_picksPerChannel(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("ETag", `W/"r1"`)
		w.Write(fixture(t, "releases.json"))
	}))
	defer srv.Close()

	res, err := testSource(srv.URL).Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC == nil || res.RC.Tag != "v1.1.0-rc3" {
		t.Fatalf("RC = %+v, want v1.1.0-rc3 (the draft v1.2.0 must be ignored)", res.RC)
	}
	if res.RC.Asset != "https://example.invalid/v1.1.0-rc3.muxapp" || res.RC.Digest != "sha256:cc" || res.RC.Size != 12819887 {
		t.Fatalf("RC asset = %+v, want the .muxapp, not the pak zip", res.RC)
	}
	if res.Stable == nil || res.Stable.Tag != "v1.0.25" || res.Stable.Digest != "sha256:ff" {
		t.Fatalf("Stable = %+v, want v1.0.25", res.Stable)
	}
	if res.ETag != `W/"r1"` {
		t.Fatalf("ETag = %q", res.ETag)
	}
	if strings.Contains(gotUA, "tg5040") || !strings.HasPrefix(gotUA, "NextUI-Itchio-Pak/") {
		t.Fatalf("User-Agent = %q: must be the short form, no device details", gotUA)
	}
}

func TestReleases_finalBeatsItsCandidatesOnRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v1.1.0-rc4","prerelease":true},{"tag_name":"v1.1.0","prerelease":false}]`))
	}))
	defer srv.Close()
	res, err := testSource(srv.URL).Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC.Tag != "v1.1.0" || res.Stable.Tag != "v1.1.0" {
		t.Fatalf("RC=%s Stable=%s; an RC tester must also be told about the final release", res.RC.Tag, res.Stable.Tag)
	}
	if res.RC.URL != "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.1.0" {
		t.Fatalf("missing html_url must fall back to the tag page, got %q", res.RC.URL)
	}
}

func TestReleases_noStableOnPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v1.1.0-rc4","prerelease":true}]`))
	}))
	defer srv.Close()
	res, _ := testSource(srv.URL).Releases(context.Background(), "")
	if res.Stable != nil {
		t.Fatalf("Stable = %+v, want nil so the caller keeps its cached one", res.Stable)
	}
}

func TestReleases_notModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `W/"r1"` {
			t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	res, err := testSource(srv.URL).Releases(context.Background(), `W/"r1"`)
	if err != nil || !res.NotModified || res.ETag != `W/"r1"` {
		t.Fatalf("res=%+v err=%v; want NotModified with the old ETag kept", res, err)
	}
}

func TestReleases_rateLimit(t *testing.T) {
	reset := time.Now().Add(20 * time.Minute).Unix()
	cases := []struct {
		name   string
		status int
		hdr    map[string]string
		want   time.Duration // approx. from now; 0 = expect a StatusError
	}{
		{"403 remaining 0", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(reset, 10)}, 20 * time.Minute},
		{"429 retry-after", 429, map[string]string{"Retry-After": "120"}, 2 * time.Minute},
		{"429 bare", 429, nil, time.Hour},
		{"403 without headers", 403, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range c.hdr {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			_, err := testSource(srv.URL).Releases(context.Background(), "")
			var rl *RateLimitError
			var se *netstate.StatusError
			if c.want == 0 {
				if !errors.As(err, &se) || se.Code != c.status {
					t.Fatalf("err = %v, want StatusError %d", err, c.status)
				}
				return
			}
			if !errors.As(err, &rl) {
				t.Fatalf("err = %v, want RateLimitError", err)
			}
			if d := time.Until(rl.Until); d < c.want-time.Minute || d > c.want+time.Minute {
				t.Fatalf("Until in %v, want ≈%v", d, c.want)
			}
		})
	}
}

func TestReleases_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := testSource(srv.URL).Releases(context.Background(), "")
	var se *netstate.StatusError
	if !errors.As(err, &se) || se.Code != 502 {
		t.Fatalf("err = %v, want StatusError 502", err)
	}
}

func TestPakJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"p1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"p1"`)
		w.Write(fixture(t, "pak.json"))
	}))
	defer srv.Close()
	s := testSource(srv.URL)
	rel, etag, nm, err := s.PakJSON(context.Background(), "")
	if err != nil || nm || rel.Tag != "v1.0.25" || etag != `"p1"` ||
		rel.URL != "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.0.25" {
		t.Fatalf("rel=%+v etag=%q nm=%v err=%v", rel, etag, nm, err)
	}
	if _, _, nm, err := s.PakJSON(context.Background(), `"p1"`); !nm || err != nil {
		t.Fatalf("second fetch: nm=%v err=%v, want not modified", nm, err)
	}
}

func TestPakJSON_badVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":"latest"}`))
	}))
	defer srv.Close()
	if _, _, _, err := testSource(srv.URL).PakJSON(context.Background(), ""); err == nil {
		t.Fatal("want an error for a version that is not a release tag")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run 'Releases|PakJSON' -v`
Expected: FAIL — `undefined: NewSource`.

- [ ] **Step 4: Implement**

```go
package appupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

const (
	repo = "carroarmato0/NextUI-Itchio-Pak"
	// per_page=30: a run of release candidates must not push the newest
	// stable release off the page (spec §1).
	defaultReleasesURL = "https://api.github.com/repos/" + repo + "/releases?per_page=30"
	// The Pak Store's own source for the latest version.
	defaultPakJSONURL = "https://raw.githubusercontent.com/" + repo + "/refs/heads/main/pak.json"
	releasePage       = "https://github.com/" + repo + "/releases/tag/"
	maxBody           = 4 << 20
)

// AssetName is the muOS asset of a release.
func AssetName(tag string) string { return "Itch-io.muOS." + tag + ".muxapp" }

// Source fetches release information from GitHub.
type Source struct {
	HTTP        *http.Client
	UserAgent   string
	ReleasesURL string
	PakJSONURL  string
	Now         func() time.Time
}

// NewSource returns a Source for the real endpoints. userAgent must be the
// short form (itchio.BuildUserAgent(info, false)): device details are for
// itch.io and are not sent to GitHub.
func NewSource(userAgent string) *Source {
	return &Source{
		HTTP:        newClient(10 * time.Second),
		UserAgent:   userAgent,
		ReleasesURL: defaultReleasesURL,
		PakJSONURL:  defaultPakJSONURL,
		Now:         time.Now,
	}
}

// newClient verifies TLS normally (SSL_CERT_FILE points Go at the bundled CA
// file on devices) and feeds the shared online/offline state. timeout 0 means
// none, for the ARCHIVE download, which has its own idle timeout.
func newClient(timeout time.Duration) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	return &http.Client{Timeout: timeout, Transport: netstate.Transport(base)}
}

// RateLimitError is GitHub asking us to wait until Until.
type RateLimitError struct{ Until time.Time }

func (e *RateLimitError) Error() string {
	return "GitHub rate limit until " + e.Until.Format(time.RFC3339)
}

// ReleasesResult is the newest release per channel in one releases response.
// One response lists both kinds, so both channels are filled every time.
type ReleasesResult struct {
	Stable, RC  *Release // nil when the page has none of that kind
	ETag        string
	NotModified bool
}

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	HTMLURL    string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

func (s *Source) get(ctx context.Context, url, etag, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", accept)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return s.HTTP.Do(req)
}

// checkStatus turns an unwanted response into an error; 200 and 304 pass.
func (s *Source) checkStatus(resp *http.Response, what string) error {
	switch c := resp.StatusCode; c {
	case http.StatusOK, http.StatusNotModified:
		return nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		if until, ok := rateLimitUntil(resp, s.Now()); ok {
			return &RateLimitError{Until: until}
		}
	}
	return &netstate.StatusError{What: what, Code: resp.StatusCode}
}

func rateLimitUntil(resp *http.Response, now time.Time) (time.Time, bool) {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
			return now.Add(time.Duration(secs) * time.Second), true
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return time.Unix(reset, 0), true
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return now.Add(time.Hour), true
	}
	return time.Time{}, false // a 403 that is not a rate limit
}

// Releases fetches the releases list.
func (s *Source) Releases(ctx context.Context, etag string) (ReleasesResult, error) {
	start := s.Now()
	resp, err := s.get(ctx, s.ReleasesURL, etag, "application/vnd.github+json")
	if err != nil {
		return ReleasesResult{}, err
	}
	defer resp.Body.Close()
	if err := s.checkStatus(resp, "GitHub releases"); err != nil {
		return ReleasesResult{}, err
	}
	if resp.StatusCode == http.StatusNotModified {
		logger.Info("appupdate: releases HTTP 304 (ETag hit) in %v", s.Now().Sub(start).Round(time.Millisecond))
		return ReleasesResult{ETag: etag, NotModified: true}, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return ReleasesResult{}, err
	}
	var list []ghRelease
	if err := json.Unmarshal(body, &list); err != nil {
		return ReleasesResult{}, fmt.Errorf("decode releases: %w", err)
	}
	res := pickReleases(list)
	res.ETag = resp.Header.Get("ETag")
	logger.Info("appupdate: releases HTTP 200, %d bytes, %d releases, stable=%s rc=%s in %v",
		len(body), len(list), tagOf(res.Stable), tagOf(res.RC), s.Now().Sub(start).Round(time.Millisecond))
	return res, nil
}

func pickReleases(list []ghRelease) ReleasesResult {
	var res ReleasesResult
	var bestStable, bestRC Version
	for _, r := range list {
		if r.Draft {
			continue
		}
		v, ok := Parse(r.Tag)
		if !ok {
			logger.Debug("appupdate: skipping release %q: not a version tag", r.Tag)
			continue
		}
		rel := toRelease(r)
		if res.RC == nil || Compare(v, bestRC) > 0 {
			res.RC, bestRC = rel, v
		}
		if !r.Prerelease && (res.Stable == nil || Compare(v, bestStable) > 0) {
			res.Stable, bestStable = rel, v
		}
	}
	return res
}

func toRelease(r ghRelease) *Release {
	rel := &Release{Tag: r.Tag, URL: r.HTMLURL}
	if rel.URL == "" {
		rel.URL = releasePage + r.Tag
	}
	want := AssetName(r.Tag)
	for _, a := range r.Assets {
		if a.Name == want {
			rel.Asset, rel.Digest, rel.Size = a.URL, a.Digest, a.Size
		}
	}
	return rel
}

func tagOf(r *Release) string {
	if r == nil {
		return "none"
	}
	return r.Tag
}

// PakJSON fetches pak.json on main: what the Pak Store will offer.
func (s *Source) PakJSON(ctx context.Context, etag string) (*Release, string, bool, error) {
	start := s.Now()
	resp, err := s.get(ctx, s.PakJSONURL, etag, "application/json")
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()
	if err := s.checkStatus(resp, "pak.json"); err != nil {
		return nil, "", false, err
	}
	if resp.StatusCode == http.StatusNotModified {
		logger.Info("appupdate: pak.json HTTP 304 (ETag hit) in %v", s.Now().Sub(start).Round(time.Millisecond))
		return nil, etag, true, nil
	}
	var pj struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&pj); err != nil {
		return nil, "", false, fmt.Errorf("decode pak.json: %w", err)
	}
	if _, ok := Parse(pj.Version); !ok {
		return nil, "", false, fmt.Errorf("pak.json version %q is not a release tag", pj.Version)
	}
	logger.Info("appupdate: pak.json HTTP 200, version=%s in %v", pj.Version, s.Now().Sub(start).Round(time.Millisecond))
	return &Release{Tag: pj.Version, URL: releasePage + pj.Version}, resp.Header.Get("ETag"), false, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/appupdate/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/appupdate/source.go internal/appupdate/source_test.go testdata/appupdate/
git commit -m "appupdate: fetch GitHub releases and main's pak.json with ETags and rate-limit back-off"
```

---

### Task 5: Pak Store database reader

**Files:**
- Create: `internal/pakstore/sqlite.go`, `internal/pakstore/pakstore.go`
- Create: `testdata/pakstore/make.sh` and the `*.db` files it generates
- Test: `internal/pakstore/pakstore_test.go`

**Interfaces:**
- Produces:
  ```go
  type Status int  // NotInstalled, Managed, Unknown; String()
  type Result struct { Status Status; Version string; Reason string } // String()
  func Lookup(dbPath string) Result
  ```

The schema below was read from the Brick's real database on 2026-09-28: a plain rowid table with a `unique (name)` index, 4096-byte pages, `journal_mode=delete`.

- [ ] **Step 1: Create the fixture generator and run it**

`testdata/pakstore/make.sh`:

```sh
#!/bin/sh
# Regenerates the Pak Store fixtures. The schema is the one on a real device
# (tg5040, 2026-09-28). Needs the sqlite3 CLI; the .db files are committed so
# tests do not.
set -eu
cd "$(dirname "$0")"
rm -f ./*.db

schema='CREATE TABLE installed_paks
(
    name          text not null,
    display_name  text not null,
    pak_id        text,
    repo_url      text,
    type          text not null,
    version       text not null,
    can_uninstall int  not null,
    unique (name)
);'
itch="('Itch-io','Itch-io','3JM2zY4UKh','https://github.com/carroarmato0/NextUI-Itchio-Pak','TOOL'"

sqlite3 single.db "$schema" "INSERT INTO installed_paks VALUES
  ('Pak Store','Pak Store','xK9mR2vL4w','https://github.com/LoveRetro/nextui-pak-store','TOOL','v4.2.0',0),
  $itch,'v1.0.23',1),
  ('BMO','BMO','qYyqrS1Jfz','https://github.com/example/bmo','TOOL','v1.0.1',1);"

# Matched by name when pak_id is missing.
sqlite3 byname.db "$schema" "INSERT INTO installed_paks VALUES ('Itch-io','Itch-io',NULL,NULL,'TOOL','v1.1.0-rc2',1);"

# 512-byte pages and 400 rows: a table b-tree with interior pages, two levels deep.
sqlite3 multipage.db "PRAGMA page_size=512;" "$schema" \
  "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<400)
   INSERT INTO installed_paks SELECT printf('Pak %03d',i), printf('Pak number %03d',i),
     printf('id%08d',i), printf('https://github.com/example/pak-%03d',i), 'TOOL', 'v0.1.0', 1 FROM n;" \
  "INSERT INTO installed_paks VALUES $itch,'v1.0.25',1);"

sqlite3 empty.db "$schema"
sqlite3 notable.db "CREATE TABLE other (x text);"

# A row too large for its page spills into overflow pages, which the reader
# does not follow.
sqlite3 overflow.db "$schema" "$(printf "INSERT INTO installed_paks VALUES %s,'v1.0.23',1);" "$itch")" \
  "INSERT INTO installed_paks VALUES ('Big', substr(replace(hex(zeroblob(2500)),'0','x'),1,5000), NULL, NULL, 'TOOL', 'v1', 1);"

head -c 6000 single.db > truncated.db
printf 'not a database at all' > badheader.db

echo "fixtures written: $(ls ./*.db | tr '\n' ' ')"
```

Run: `chmod +x testdata/pakstore/make.sh && testdata/pakstore/make.sh && sqlite3 testdata/pakstore/multipage.db 'PRAGMA page_size; PRAGMA page_count;'`
Expected: `fixtures written: …` listing 8 files; `512` and a page count well above 50.

- [ ] **Step 2: Write the failing test**

```go
package pakstore

import (
	"os"
	"path/filepath"
	"testing"
)

const fixtures = "../../testdata/pakstore/"

func TestLookup(t *testing.T) {
	cases := []struct {
		file    string
		status  Status
		version string
	}{
		{"single.db", Managed, "v1.0.23"},
		{"byname.db", Managed, "v1.1.0-rc2"},
		{"multipage.db", Managed, "v1.0.25"},
		{"empty.db", NotInstalled, ""},
		{"notable.db", Unknown, ""},
		{"overflow.db", Unknown, ""},
		{"truncated.db", Unknown, ""},
		{"badheader.db", Unknown, ""},
		{"does-not-exist.db", NotInstalled, ""},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			got := Lookup(fixtures + c.file)
			if got.Status != c.status || got.Version != c.version {
				t.Fatalf("Lookup = %+v, want %s %q", got, c.status, c.version)
			}
			if got.Status == Unknown && got.Reason == "" {
				t.Fatal("Unknown must say why, for the log")
			}
		})
	}
}

func TestLookup_emptyPathIsNotInstalled(t *testing.T) {
	if got := Lookup(""); got.Status != NotInstalled {
		t.Fatalf("Lookup(\"\") = %+v", got)
	}
}

func TestLookup_nonEmptyWALIsUnknown(t *testing.T) {
	dir := t.TempDir()
	data, _ := os.ReadFile(fixtures + "single.db")
	db := filepath.Join(dir, "pak-store.db")
	os.WriteFile(db, data, 0644)
	os.WriteFile(db+"-wal", []byte("uncommitted pages"), 0644)
	if got := Lookup(db); got.Status != Unknown {
		t.Fatalf("with a non-empty -wal: %+v, want Unknown", got)
	}
	os.WriteFile(db+"-wal", nil, 0644)
	if got := Lookup(db); got.Status != Managed {
		t.Fatalf("with an empty -wal: %+v, want Managed", got)
	}
}

func TestColumnNames(t *testing.T) {
	sql := "CREATE TABLE installed_paks\n(\n name text not null,\n pak_id text,\n version text not null,\n unique (name)\n)"
	got := columnNames(sql)
	want := []string{"name", "pak_id", "version"}
	if len(got) != len(want) {
		t.Fatalf("columnNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columnNames = %v, want %v", got, want)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/pakstore/ -v`
Expected: FAIL — `undefined: Lookup`.

- [ ] **Step 4: Implement the SQLite reader**

`internal/pakstore/sqlite.go`:

```go
package pakstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// A reader for exactly as much of the SQLite file format as Lookup needs:
// the header, table b-trees (interior and leaf pages) and records. Overflow
// pages are not followed: a row that needs them makes the database Unknown.
// Linking a SQLite library was rejected: several MB of binary to read one
// 12 KB file. Format reference: https://www.sqlite.org/fileformat.html

var (
	errOverflow = errors.New("row uses overflow pages")
	errNoTable  = errors.New("no installed_paks table")
)

type db struct {
	data     []byte
	pageSize int
	usable   int
	pages    int
}

func open(data []byte) (*db, error) {
	if len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
		return nil, errors.New("bad header")
	}
	ps := int(binary.BigEndian.Uint16(data[16:18]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps&(ps-1) != 0 {
		return nil, fmt.Errorf("bad page size %d", ps)
	}
	if len(data)%ps != 0 {
		return nil, fmt.Errorf("truncated: %d bytes is not a whole number of %d-byte pages", len(data), ps)
	}
	usable := ps - int(data[20])
	if usable < 480 {
		return nil, fmt.Errorf("usable page size %d too small", usable)
	}
	if enc := binary.BigEndian.Uint32(data[56:60]); enc != 1 {
		return nil, fmt.Errorf("text encoding %d is not UTF-8", enc)
	}
	return &db{data: data, pageSize: ps, usable: usable, pages: len(data) / ps}, nil
}

// page returns page n (1-based) and the offset of its b-tree header, which is
// 100 on page 1 because the file header comes first.
func (d *db) page(n int) ([]byte, int, error) {
	if n < 1 || n > d.pages {
		return nil, 0, fmt.Errorf("page %d out of range 1..%d", n, d.pages)
	}
	p := d.data[(n-1)*d.pageSize : n*d.pageSize]
	if n == 1 {
		return p, 100, nil
	}
	return p, 0, nil
}

// walkTable calls fn with the payload of every row in the table b-tree rooted
// at root.
func (d *db) walkTable(root int, fn func(payload []byte) error) error {
	visited := map[int]bool{}
	var walk func(n, depth int) error
	walk = func(n, depth int) error {
		if depth > 20 || visited[n] {
			return fmt.Errorf("b-tree loop at page %d", n)
		}
		visited[n] = true
		p, h, err := d.page(n)
		if err != nil {
			return err
		}
		if h+12 > len(p) {
			return fmt.Errorf("page %d: short header", n)
		}
		typ := p[h]
		count := int(binary.BigEndian.Uint16(p[h+3:]))
		hdr := 8
		if typ == 0x05 {
			hdr = 12
		}
		if h+hdr+2*count > len(p) {
			return fmt.Errorf("page %d: cell pointers overrun", n)
		}
		for i := 0; i < count; i++ {
			off := int(binary.BigEndian.Uint16(p[h+hdr+2*i:]))
			if off >= len(p) {
				return fmt.Errorf("page %d: cell %d out of range", n, i)
			}
			switch typ {
			case 0x0D: // table leaf: payload size, rowid, payload
				size, k1 := varint(p[off:])
				_, k2 := varint(p[off+k1:])
				if k1 == 0 || k2 == 0 {
					return fmt.Errorf("page %d: bad cell %d", n, i)
				}
				if int(size) > d.usable-35 {
					return errOverflow
				}
				start := off + k1 + k2
				if start+int(size) > len(p) {
					return fmt.Errorf("page %d: cell %d overruns the page", n, i)
				}
				if err := fn(p[start : start+int(size)]); err != nil {
					return err
				}
			case 0x05: // table interior: left child, rowid
				if off+4 > len(p) {
					return fmt.Errorf("page %d: bad cell %d", n, i)
				}
				if err := walk(int(binary.BigEndian.Uint32(p[off:])), depth+1); err != nil {
					return err
				}
			default:
				return fmt.Errorf("page %d: type 0x%02x is not a table page", n, typ)
			}
		}
		if typ == 0x05 {
			return walk(int(binary.BigEndian.Uint32(p[h+8:])), depth+1)
		}
		return nil
	}
	return walk(root, 0)
}

// varint decodes a SQLite varint. n is 0 when b ends first.
func varint(b []byte) (v uint64, n int) {
	for i := 0; i < 9 && i < len(b); i++ {
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}

// value is one decoded column. Only what Lookup reads is kept.
type value struct {
	isNull bool
	isInt  bool
	i      int64
	s      string // text columns
}

func decodeRecord(p []byte) ([]value, error) {
	hdrLen, n := varint(p)
	if n == 0 || int(hdrLen) > len(p) || int(hdrLen) < n {
		return nil, errors.New("bad record header")
	}
	var types []uint64
	for pos := n; pos < int(hdrLen); {
		t, k := varint(p[pos:hdrLen])
		if k == 0 {
			return nil, errors.New("bad serial type")
		}
		types = append(types, t)
		pos += k
	}
	body := int(hdrLen)
	intSize := [...]int{0, 1, 2, 3, 4, 6, 8}
	out := make([]value, 0, len(types))
	for _, t := range types {
		var size int
		v := value{}
		switch {
		case t == 0:
			v.isNull = true
		case t >= 1 && t <= 6:
			size = intSize[t]
			v.isInt = true
		case t == 7:
			size = 8
		case t == 8, t == 9:
			v.isInt, v.i = true, int64(t-8)
		case t >= 12 && t%2 == 0:
			size = int(t-12) / 2
		case t >= 13:
			size = int(t-13) / 2
		default:
			return nil, fmt.Errorf("reserved serial type %d", t)
		}
		if body+size > len(p) {
			return nil, errors.New("record overruns its payload")
		}
		field := p[body : body+size]
		switch {
		case v.isInt && size > 0:
			var x int64
			for _, c := range field {
				x = x<<8 | int64(c)
			}
			shift := 64 - 8*uint(size) // sign-extend
			v.i = x << shift >> shift
		case t >= 13 && t%2 == 1:
			v.s = string(field)
		}
		out = append(out, v)
		body += size
	}
	return out, nil
}

// columnNames reads the column names from a CREATE TABLE statement, skipping
// table constraints. Enough for the Store's plain schema.
func columnNames(sql string) []string {
	lp, rp := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	if lp < 0 || rp <= lp {
		return nil
	}
	var names []string
	depth, start := 0, lp+1
	body := sql[:rp]
	for i := lp + 1; i <= len(body); i++ {
		if i < len(body) {
			switch body[i] {
			case '(':
				depth++
				continue
			case ')':
				depth--
				continue
			case ',':
				if depth > 0 {
					continue
				}
			default:
				continue
			}
		}
		fields := strings.Fields(body[start:i])
		start = i + 1
		if len(fields) == 0 {
			continue
		}
		name := strings.Trim(fields[0], "\"`[]")
		switch strings.ToLower(name) {
		case "unique", "primary", "constraint", "check", "foreign":
			continue
		}
		names = append(names, name)
	}
	return names
}
```

- [ ] **Step 5: Implement Lookup**

`internal/pakstore/pakstore.go`:

```go
// Package pakstore tells whether the NextUI Pak Store manages this install,
// by reading its SQLite database read-only.
package pakstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Status is what the database says about Itch-io.
type Status int

const (
	// NotInstalled: no Store database for this platform, or no row for Itch-io.
	NotInstalled Status = iota
	// Managed: the Store has a row for Itch-io; Version is what it records.
	Managed
	// Unknown: the file exists but cannot be read with confidence. Callers
	// treat it as Managed: guessing wrong that way only points the user at the
	// Store; guessing wrong the other way would go around it.
	Unknown
)

func (s Status) String() string {
	switch s {
	case Managed:
		return "managed"
	case Unknown:
		return "unknown"
	default:
		return "not-installed"
	}
}

const (
	itchioPakID = "3JM2zY4UKh"
	itchioName  = "Itch-io"
)

// Result is Lookup's answer. Reason explains it in the log.
type Result struct {
	Status  Status
	Version string
	Reason  string
}

func (r Result) String() string {
	if r.Status == Managed {
		return "managed(" + r.Version + ")"
	}
	return r.Status.String() + "(" + r.Reason + ")"
}

// Lookup reads dbPath. "" means the firmware has no Pak Store.
func Lookup(dbPath string) Result {
	res := lookup(dbPath)
	logger.Info("pakstore: %s → %s", dbPath, res)
	return res
}

func lookup(dbPath string) Result {
	if dbPath == "" {
		return Result{Status: NotInstalled, Reason: "no Pak Store on this firmware"}
	}
	data, err := os.ReadFile(dbPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{Status: NotInstalled, Reason: "no database"}
	}
	if err != nil {
		return Result{Status: Unknown, Reason: "read: " + err.Error()}
	}
	// Committed pages may still be in the write-ahead log, not the main file.
	if fi, err := os.Stat(dbPath + "-wal"); err == nil && fi.Size() > 0 {
		return Result{Status: Unknown, Reason: "write-ahead log is not empty"}
	}
	d, err := open(data)
	if err != nil {
		return Result{Status: Unknown, Reason: err.Error()}
	}
	logger.Debug("pakstore: page size %d, %d pages", d.pageSize, d.pages)
	version, found, err := findItchio(d)
	switch {
	case errors.Is(err, errNoTable):
		return Result{Status: Unknown, Reason: err.Error()}
	case err != nil:
		return Result{Status: Unknown, Reason: err.Error()}
	case !found:
		return Result{Status: NotInstalled, Reason: "no row for Itch-io"}
	}
	return Result{Status: Managed, Version: version}
}

func findItchio(d *db) (string, bool, error) {
	root, sql, err := findTable(d, "installed_paks")
	if err != nil {
		return "", false, err
	}
	cols := columnNames(sql)
	idx := map[string]int{}
	for i, c := range cols {
		idx[c] = i
	}
	iName, ok1 := idx["name"]
	iID, ok2 := idx["pak_id"]
	iVer, ok3 := idx["version"]
	if !ok1 || !ok2 || !ok3 {
		return "", false, fmt.Errorf("unexpected columns %v", cols)
	}
	var byID, byName string
	var haveID, haveName bool
	err = d.walkTable(root, func(payload []byte) error {
		row, err := decodeRecord(payload)
		if err != nil {
			return err
		}
		if len(row) <= iVer || len(row) <= iName || len(row) <= iID {
			return errors.New("row has fewer columns than the schema")
		}
		switch {
		case row[iID].s == itchioPakID:
			byID, haveID = row[iVer].s, true
		case row[iName].s == itchioName:
			byName, haveName = row[iVer].s, true
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if haveID {
		return byID, true, nil
	}
	return byName, haveName, nil
}

// findTable looks name up in sqlite_schema, the table rooted at page 1.
func findTable(d *db, name string) (root int, sql string, err error) {
	err = d.walkTable(1, func(payload []byte) error {
		row, err := decodeRecord(payload)
		if err != nil {
			return err
		}
		// type, name, tbl_name, rootpage, sql
		if len(row) >= 5 && row[0].s == "table" && row[1].s == name && row[3].isInt {
			root, sql = int(row[3].i), row[4].s
		}
		return nil
	})
	if err == nil && root == 0 {
		err = errNoTable
	}
	return root, sql, err
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/pakstore/ -v`
Expected: PASS (all subtests).

Then check the reader against a real Store database. Add this skip-unless-set test to `pakstore_test.go` (the device database is not committed: it lists the user's installed paks):

```go
// Run by hand against a database pulled from a device; skipped otherwise.
func TestLookup_realDevice(t *testing.T) {
	path := os.Getenv("PAKSTORE_REAL_DB")
	if path == "" {
		t.Skip("PAKSTORE_REAL_DB not set")
	}
	got := Lookup(path)
	t.Logf("real device: %+v", got)
	if got.Status != Managed {
		t.Fatalf("real Store database: %+v, want Managed", got)
	}
}
```

Pull the database into the scratchpad (the Brick is `4c000c820706472229d`; check `adb devices` first) and run the test against it:

Run: `adb -s 4c000c820706472229d pull /mnt/SDCARD/.userdata/tg5040/nextui-pak-store/pak-store.db "$SCRATCH/" && PAKSTORE_REAL_DB="$SCRATCH/pak-store.db" go test ./internal/pakstore/ -run TestLookup_realDevice -v`
(`$SCRATCH` = the session scratchpad directory.)
Expected: PASS, logging `real device: {Status:1 Version:v1.0.23 Reason:}`. If no device is attached, say so in the task report and move on.

- [ ] **Step 7: Commit**

```bash
git add internal/pakstore/ testdata/pakstore/
git commit -m "pakstore: read the Pak Store's install database without a SQLite library"
```

---

### Task 6: Firmware paths for ARCHIVE and the Store database

**Files:**
- Modify: `internal/firmware/firmware.go` (fields in `Env`, accessors after `LogPath()`)
- Modify: `internal/firmware/nextui.go` (`newNextUI`)
- Modify: `internal/firmware/muos.go` (`newMuOS`)
- Test: `internal/firmware/firmware_test.go`

**Interfaces:**
- Produces: `(*Env) ArchiveDir() string` — muOS: `<SD1 mount>/ARCHIVE`, else `""`; `(*Env) PakStoreDB() string` — NextUI with `$PLATFORM`: `/mnt/SDCARD/.userdata/<PLATFORM>/nextui-pak-store/pak-store.db`, else `""`.

- [ ] **Step 1: Write the failing test**

Append to `internal/firmware/firmware_test.go`:

```go
func TestArchiveDirAndPakStoreDB(t *testing.T) {
	t.Setenv("PLATFORM", "tg5040")
	nx := ForTest(KindNextUI, "")
	if got := nx.PakStoreDB(); got != "/mnt/SDCARD/.userdata/tg5040/nextui-pak-store/pak-store.db" {
		t.Errorf("NextUI PakStoreDB = %q", got)
	}
	if nx.ArchiveDir() != "" {
		t.Errorf("NextUI ArchiveDir = %q, want empty", nx.ArchiveDir())
	}

	t.Setenv("PLATFORM", "")
	if got := ForTest(KindNextUI, "").PakStoreDB(); got != "" {
		t.Errorf("NextUI without PLATFORM: PakStoreDB = %q, want empty", got)
	}

	prefix := t.TempDir()
	mu := ForTest(KindMuOS, prefix)
	if got, want := mu.ArchiveDir(), filepath.Join(prefix, "/mnt/mmc", "ARCHIVE"); got != want {
		t.Errorf("muOS ArchiveDir = %q, want %q", got, want)
	}
	if mu.PakStoreDB() != "" {
		t.Errorf("muOS PakStoreDB = %q, want empty", mu.PakStoreDB())
	}

	host := ForTest(KindHost, "")
	if host.ArchiveDir() != "" || host.PakStoreDB() != "" {
		t.Error("host must expose neither path")
	}
}
```

(Add `"path/filepath"` to the imports if missing.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/firmware/ -run TestArchiveDirAndPakStoreDB -v`
Expected: FAIL — `nx.PakStoreDB undefined`.

- [ ] **Step 3: Implement**

In `internal/firmware/firmware.go`, inside `type Env struct`, before `dataDir string`:

```go
	// archiveDir is where muOS's Archive Manager looks for .muxapp files.
	archiveDir string
	// pakStoreDB is the NextUI Pak Store's install database.
	pakStoreDB string
```

After `func (e *Env) LogPath()`:

```go
// ArchiveDir is ARCHIVE/ at the root of the card muOS calls SD1, where Archive
// Manager finds .muxapp files. "" where there is no Archive Manager.
func (e *Env) ArchiveDir() string { return e.archiveDir }

// PakStoreDB is the Pak Store's install database for this platform. "" off
// NextUI or without PLATFORM. The file need not exist.
func (e *Env) PakStoreDB() string { return e.pakStoreDB }
```

In `newNextUI` (`internal/firmware/nextui.go`), before the `return &Env{`:

```go
	pakStoreDB := ""
	if platform != "" {
		pakStoreDB = filepath.Join(root, ".userdata", platform, "nextui-pak-store", "pak-store.db")
	}
```

and in the literal, after `versionFile:`:

```go
		pakStoreDB: pakStoreDB,
```

In `newMuOS` (`internal/firmware/muos.go`), in the literal after `versionFile:`:

```go
		archiveDir: filepath.Join(prefix, romMount, "ARCHIVE"),
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/firmware/ -v`
Expected: PASS (whole package).

- [ ] **Step 5: Commit**

```bash
git add internal/firmware/
git commit -m "firmware: expose muOS's ARCHIVE folder and the Pak Store database path"
```

---

### Task 7: The verdict and the notification rule

**Files:**
- Create: `internal/appupdate/decide.go`
- Test: `internal/appupdate/decide_test.go`

**Interfaces:**
- Consumes: `Version`, `Compare`, `StoreCompare`, `Channel`, `Release`, `pakstore.Result`, `firmware.Kind`.
- Produces:
  ```go
  type Kind int   // Unknown, UpToDate, Available; String()
  type Via int    // ViaReleasePage, ViaPakStore, ViaArchive; String()
  type Verdict struct {
      Kind Kind; Channel Channel; Running Version
      Latest *Release      // the channel's newest known release; nil when unknown
      Via Via              // Available only
      Ahead bool           // UpToDate and the running build is newer than Latest
      StoreOffers string   // non-empty: the Pak Store may offer this OLDER version (§2a)
  }
  type Inputs struct {
      Firmware firmware.Kind; Running Version; Channel Channel
      Store pakstore.Result
      Latest *Release      // for Stable on a Store-managed install: pak.json's release
      PakJSON *Release     // main's pak.json, when known
  }
  func Decide(in Inputs) Verdict
  func ShouldNotify(v Verdict, notified string) bool
  ```

- [ ] **Step 1: Write the failing test**

```go
package appupdate

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

func v(s string) Version { x, _ := Parse(s); return x }

func rel(tag string) *Release { return &Release{Tag: tag, URL: releasePage + tag} }

func muxRel(tag string) *Release {
	r := rel(tag)
	r.Asset, r.Digest, r.Size = "https://example.invalid/"+AssetName(tag), "sha256:ab", 12819887
	return r
}

func managed(ver string) pakstore.Result { return pakstore.Result{Status: pakstore.Managed, Version: ver} }

var notStore = pakstore.Result{Status: pakstore.NotInstalled}

func TestDecide(t *testing.T) {
	cases := []struct {
		name   string
		in     Inputs
		kind   Kind
		via    Via
		ahead  bool
		offers string
	}{
		{"muOS rc with asset", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), RC, notStore, muxRel("v1.1.0-rc4"), nil}, Available, ViaArchive, false, ""},
		{"muOS without digest", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), RC, notStore, rel("v1.1.0-rc4"), nil}, Available, ViaReleasePage, false, ""},
		{"NextUI not managed", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, notStore, rel("v1.0.26"), nil}, Available, ViaReleasePage, false, ""},
		{"NextUI managed stable", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, managed("v1.0.25"), rel("v1.0.26"), rel("v1.0.26")}, Available, ViaPakStore, false, ""},
		{"Store never offers an rc", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), RC, managed("v1.0.25"), rel("v1.1.0-rc4"), rel("v1.0.25")}, Available, ViaReleasePage, false, ""},
		{"Store-row trap", Inputs{firmware.KindNextUI, v("v1.1.0-rc2"), Stable, managed("v1.1.0-rc2"), rel("v1.1.0"), rel("v1.1.0")}, Available, ViaReleasePage, false, ""},
		{"Store unreadable counts as managed", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, pakstore.Result{Status: pakstore.Unknown}, rel("v1.0.26"), rel("v1.0.26")}, Available, ViaPakStore, false, ""},
		{"side-loaded rc, Store offers a downgrade", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), Stable, managed("v1.0.23"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, "v1.0.25"},
		{"downgrade warning on RC too", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), RC, managed("v1.0.23"), rel("v1.1.0-rc3"), rel("v1.0.25")}, UpToDate, 0, false, "v1.0.25"},
		{"no warning once the row caught up", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), Stable, managed("v1.0.25"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, ""},
		{"no warning on muOS", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), Stable, managed("v1.0.23"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, ""},
		{"RC→Stable while ahead", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), Stable, notStore, muxRel("v1.0.25"), nil}, UpToDate, 0, true, ""},
		{"final announced after RC→Stable", Inputs{firmware.KindMuOS, v("v1.1.0-rc4"), Stable, notStore, muxRel("v1.1.0"), nil}, Available, ViaArchive, false, ""},
		{"same version", Inputs{firmware.KindNextUI, v("v1.1.0"), Stable, notStore, rel("v1.1.0"), nil}, UpToDate, 0, false, ""},
		{"off", Inputs{firmware.KindNextUI, v("v1.0.25"), Off, notStore, rel("v1.0.26"), nil}, Unknown, 0, false, ""},
		{"nothing known", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, notStore, nil, nil}, Unknown, 0, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.in)
			if got.Kind != c.kind || (c.kind == Available && got.Via != c.via) || got.Ahead != c.ahead || got.StoreOffers != c.offers {
				t.Fatalf("Decide = {Kind:%s Via:%s Ahead:%v StoreOffers:%q}, want {%s %s %v %q}",
					got.Kind, got.Via, got.Ahead, got.StoreOffers, c.kind, c.via, c.ahead, c.offers)
			}
		})
	}
}

// §2a: nothing lower than or equal to the running version is ever Available.
func TestDecide_neverOffersADowngrade(t *testing.T) {
	tags := []string{"v1.0.23", "v1.0.25", "v1.1.0-rc1", "v1.1.0-rc3", "v1.1.0", "v1.1.1-rc1"}
	for _, running := range tags {
		for _, latest := range tags {
			for _, fw := range []firmware.Kind{firmware.KindNextUI, firmware.KindMuOS} {
				for _, st := range []pakstore.Result{notStore, managed("v1.0.23"), managed("v1.1.0-rc1"), {Status: pakstore.Unknown}} {
					got := Decide(Inputs{fw, v(running), Stable, st, muxRel(latest), muxRel(latest)})
					if got.Kind == Available && Compare(v(latest), v(running)) <= 0 {
						t.Fatalf("running %s, latest %s, %s, store %v: offered a non-upgrade", running, latest, fw, st)
					}
				}
			}
		}
	}
}

func TestShouldNotify(t *testing.T) {
	avail := func(tag string) Verdict { return Verdict{Kind: Available, Latest: rel(tag)} }
	cases := []struct {
		v        Verdict
		notified string
		want     bool
	}{
		{avail("v1.1.0-rc4"), "", true},
		{avail("v1.1.0-rc4"), "v1.1.0-rc4", false},
		{avail("v1.1.0"), "v1.1.0-rc4", true},
		{avail("v1.1.0-rc4"), "v1.1.0", false},
		{avail("v1.1.0-rc4"), "garbage", true},
		{Verdict{Kind: UpToDate, Latest: rel("v1.1.0")}, "", false},
	}
	for _, c := range cases {
		if got := ShouldNotify(c.v, c.notified); got != c.want {
			t.Errorf("ShouldNotify(%s %s, %q) = %v, want %v", c.v.Kind, c.v.Latest.Tag, c.notified, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run 'Decide|ShouldNotify' -v`
Expected: FAIL — `undefined: Decide`.

- [ ] **Step 3: Implement**

```go
package appupdate

import (
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

// Kind is the outcome of a decision.
type Kind int

const (
	Unknown   Kind = iota // nothing known yet, or the channel is Off
	UpToDate              // nothing newer than the running build
	Available             // Latest is newer than the running build
)

func (k Kind) String() string {
	switch k {
	case UpToDate:
		return "up-to-date"
	case Available:
		return "available"
	default:
		return "unknown"
	}
}

// Via is the route an available update takes.
type Via int

const (
	// ViaReleasePage: show the version and the release-page QR code.
	ViaReleasePage Via = iota
	// ViaPakStore: NextUI, Store-managed, and the Store will show it.
	ViaPakStore
	// ViaArchive: muOS, and the release has a .muxapp with a digest.
	ViaArchive
)

func (v Via) String() string {
	switch v {
	case ViaPakStore:
		return "pak-store"
	case ViaArchive:
		return "archive"
	default:
		return "release-page"
	}
}

// Verdict is what the app tells the user.
type Verdict struct {
	Kind    Kind
	Channel Channel
	Running Version
	Latest  *Release
	Via     Via
	// Ahead: UpToDate, and the running build is newer than Latest (an rc
	// tester on the Stable channel).
	Ahead bool
	// StoreOffers is a version the Pak Store may offer as an "update" that is
	// older than the running build (spec §2a). Shown on the Updates screen
	// only, never as a notice.
	StoreOffers string
}

// Inputs is everything Decide looks at.
type Inputs struct {
	Firmware firmware.Kind
	Running  Version
	Channel  Channel
	Store    pakstore.Result
	Latest   *Release
	PakJSON  *Release
}

// Decide is a pure function from the inputs to a Verdict. "Available" needs
// latest > running by the real ordering, whatever the Store row says: a
// side-loaded rc must never be told to "update" to an older stable.
func Decide(in Inputs) Verdict {
	out := Verdict{Channel: in.Channel, Running: in.Running, Latest: in.Latest, StoreOffers: storeOffers(in)}
	if in.Channel == Off || in.Latest == nil {
		return out
	}
	latest, ok := Parse(in.Latest.Tag)
	if !ok {
		return out
	}
	switch c := Compare(latest, in.Running); {
	case c <= 0:
		out.Kind, out.Ahead = UpToDate, c < 0
	default:
		out.Kind, out.Via = Available, via(in, latest)
	}
	return out
}

func via(in Inputs, latest Version) Via {
	switch in.Firmware {
	case firmware.KindNextUI:
		// The Store never offers a release candidate, and offers a final
		// release only when its row compares lower (its comparison thinks
		// v1.1.0-rc2 == v1.1.0: the Store-row trap).
		if in.Store.Status != pakstore.NotInstalled && !latest.IsRC() {
			if in.Store.Status == pakstore.Unknown || StoreCompare(in.Store.Version, in.Latest.Tag) == -1 {
				return ViaPakStore
			}
		}
	case firmware.KindMuOS:
		if in.Latest.Asset != "" && in.Latest.Size > 0 && strings.HasPrefix(in.Latest.Digest, "sha256:") {
			return ViaArchive
		}
	}
	return ViaReleasePage
}

// storeOffers is the §2a downgrade trap: the Store compares its stale row with
// pak.json on main, not with what is on disk.
func storeOffers(in Inputs) string {
	if in.Firmware != firmware.KindNextUI || in.Store.Status != pakstore.Managed || in.PakJSON == nil {
		return ""
	}
	pj, ok := Parse(in.PakJSON.Tag)
	if !ok {
		return ""
	}
	if StoreCompare(in.Store.Version, in.PakJSON.Tag) == -1 && Compare(pj, in.Running) < 0 {
		return in.PakJSON.Tag
	}
	return ""
}

// ShouldNotify is the rule of spec §2: Available, and newer than the last
// version announced on this channel.
func ShouldNotify(v Verdict, notified string) bool {
	if v.Kind != Available || v.Latest == nil {
		return false
	}
	latest, ok := Parse(v.Latest.Tag)
	if !ok {
		return false
	}
	prev, ok := Parse(notified)
	return !ok || Compare(latest, prev) > 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/appupdate/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/decide.go internal/appupdate/decide_test.go
git commit -m "appupdate: decide what to tell the user, never offering a downgrade"
```

---

### Task 8: muOS — Save to ARCHIVE

**Files:**
- Create: `internal/appupdate/archive.go`, `internal/appupdate/freespace_linux.go`, `internal/appupdate/freespace_other.go`
- Test: `internal/appupdate/archive_test.go`

**Interfaces:**
- Consumes: `Release`, `AssetName`, `partfile.Create/PathFor/Sweep/SetJournal`, `netstate.StatusError`.
- Produces:
  ```go
  var ErrIntegrity error                 // "download failed the integrity check"
  type SpaceError struct{ Need, Have int64 }
  var idleTimeout = 30 * time.Second     // tests shorten it
  func SaveToArchive(ctx context.Context, hc *http.Client, userAgent string, rel Release, dir string, progress func(done, total int64)) (string, error)
  func ValidateMuxapp(path string) error
  func CleanOtherArchives(dir, keep string) []string
  ```

- [ ] **Step 1: Write the failing test**

```go
package appupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

func buildZip(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("#!/bin/sh\n"))
	}
	zw.Close()
	return buf.Bytes()
}

func goodMuxapp(t *testing.T) []byte {
	return buildZip(t, "Itch-io/", "Itch-io/mux_launch.sh", "Itch-io/itchio", "Itch-io/assets/font.ttf")
}

func releaseFor(url string, body []byte, tag string) Release {
	sum := sha256.Sum256(body)
	return Release{Tag: tag, Asset: url, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

func archiveSetup(t *testing.T) string {
	t.Helper()
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	return filepath.Join(t.TempDir(), "ARCHIVE")
}

func serve(t *testing.T, body []byte) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv
}

func assertNoPart(t *testing.T, dir, tag string) {
	t.Helper()
	if _, err := os.Stat(partfile.PathFor(filepath.Join(dir, AssetName(tag)))); !os.IsNotExist(err) {
		t.Fatal("the partial file must not survive")
	}
	if _, err := os.Stat(filepath.Join(dir, AssetName(tag))); !os.IsNotExist(err) {
		t.Fatal("nothing may be saved under the final name")
	}
}

func TestSaveToArchive_ok_andCleansOthers(t *testing.T) {
	dir := archiveSetup(t)
	os.MkdirAll(dir, 0755)
	for _, n := range []string{"Itch-io.muOS.v1.0.25.muxapp", "Itch-io.muOS.v1.2.0-rc1.muxapp", "Other.muOS.v1.0.0.muxapp", "Itch-io.muOS.notes.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644)
	}
	body := goodMuxapp(t)
	srv := serve(t, body)
	var last int64
	path, err := SaveToArchive(context.Background(), srv.Client(), "ua", releaseFor(srv.URL, body, "v1.1.1"), dir,
		func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "Itch-io.muOS.v1.1.1.muxapp") || last != int64(len(body)) {
		t.Fatalf("path=%q progress=%d", path, last)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := map[string]bool{"Itch-io.muOS.v1.1.1.muxapp": true, "Other.muOS.v1.0.0.muxapp": true, "Itch-io.muOS.notes.txt": true}
	if len(names) != len(want) {
		t.Fatalf("ARCHIVE holds %v; want exactly %v (the newer rc must go too)", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected %s left in ARCHIVE", n)
		}
	}
}

func TestSaveToArchive_digestMismatch(t *testing.T) {
	dir := archiveSetup(t)
	body := goodMuxapp(t)
	srv := serve(t, body)
	rel := releaseFor(srv.URL, body, "v1.1.1")
	rel.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
	_, err := SaveToArchive(context.Background(), srv.Client(), "ua", rel, dir, nil)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_lengthMismatch(t *testing.T) {
	dir := archiveSetup(t)
	body := goodMuxapp(t)
	srv := serve(t, body)
	rel := releaseFor(srv.URL, body, "v1.1.1")
	rel.Size++
	if _, err := SaveToArchive(context.Background(), srv.Client(), "ua", rel, dir, nil); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_unsafeZip(t *testing.T) {
	for name, entries := range map[string][]string{
		"outside the app folder": {"Itch-io/mux_launch.sh", "evil.sh"},
		"dot-dot":                {"Itch-io/mux_launch.sh", "Itch-io/../../etc/x"},
		"absolute":               {"Itch-io/mux_launch.sh", "/etc/x"},
		"backslash":              {"Itch-io/mux_launch.sh", "Itch-io\\x"},
		"no launcher":            {"Itch-io/itchio"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := archiveSetup(t)
			body := buildZip(t, entries...)
			srv := serve(t, body)
			if _, err := SaveToArchive(context.Background(), srv.Client(), "ua", releaseFor(srv.URL, body, "v1.1.1"), dir, nil); err == nil {
				t.Fatal("want the zip rejected")
			}
			assertNoPart(t, dir, "v1.1.1")
		})
	}
}

func TestSaveToArchive_cancel(t *testing.T) {
	dir := archiveSetup(t)
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(make([]byte, 1000))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err := SaveToArchive(ctx, srv.Client(), "ua", Release{Tag: "v1.1.1", Asset: srv.URL, Digest: "sha256:00", Size: 1000000}, dir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_idleTimeout(t *testing.T) {
	old := idleTimeout
	idleTimeout = 100 * time.Millisecond
	t.Cleanup(func() { idleTimeout = old })
	dir := archiveSetup(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never sends headers
	}))
	defer srv.Close()
	start := time.Now()
	_, err := SaveToArchive(context.Background(), srv.Client(), "ua", Release{Tag: "v1.1.1", Asset: srv.URL, Digest: "sha256:00", Size: 10}, dir, nil)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err = %v after %v; want an idle timeout covering the header wait", err, time.Since(start))
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_notDownloadable(t *testing.T) {
	dir := archiveSetup(t)
	for _, rel := range []Release{{Tag: "v1.1.1"}, {Tag: "v1.1.1", Asset: "http://x", Size: 1}} {
		if _, err := SaveToArchive(context.Background(), http.DefaultClient, "ua", rel, dir, nil); err == nil {
			t.Fatalf("%+v: want an error without an asset and digest", rel)
		}
	}
}

func TestSweepRemovesArchiveLeftover(t *testing.T) {
	dir := archiveSetup(t)
	os.MkdirAll(dir, 0755)
	left := partfile.PathFor(filepath.Join(dir, AssetName("v1.1.0")))
	os.WriteFile(left, []byte("half a download"), 0644)
	partfile.Sweep([]string{dir})
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Fatalf("%s must be swept", left)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run 'Archive|Sweep' -v`
Expected: FAIL — `undefined: SaveToArchive`.

- [ ] **Step 3: Implement free space**

`internal/appupdate/freespace_linux.go`:

```go
//go:build linux

package appupdate

import "syscall"

// freeBytes is the space available to this process on dir's filesystem.
func freeBytes(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}
```

`internal/appupdate/freespace_other.go`:

```go
//go:build !linux

package appupdate

// freeBytes is unknown off Linux; the download then relies on write errors.
func freeBytes(dir string) (int64, bool) { return 0, false }
```

- [ ] **Step 4: Implement the download**

`internal/appupdate/archive.go`:

```go
package appupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// ErrIntegrity: the download does not match the release's size or digest.
var ErrIntegrity = errors.New("download failed the integrity check")

var errIdle = errors.New("download stalled: no data for 30 s")

// SpaceError: the card has less room than the asset plus 10 %.
type SpaceError struct{ Need, Have int64 }

func (e *SpaceError) Error() string {
	return fmt.Sprintf("not enough space: need %d bytes, %d free", e.Need, e.Have)
}

// idleTimeout also covers the wait for response headers. A variable so
// tests can shorten it.
var idleTimeout = 30 * time.Second

// Archive Manager's SAFE_ARCHIVE rejects oversized archives; stay well
// inside what a real .muxapp (≈13 MB, a few hundred entries) needs.
const (
	maxEntries   = 10000
	maxEntrySize = 256 << 20
	maxTotalSize = 1 << 30
)

var ourArchive = regexp.MustCompile(`^Itch-io\.muOS\.v\d+\.\d+\.\d+(-rc\d+)?\.muxapp$`)

// SaveToArchive downloads rel's .muxapp into dir for muOS's Archive Manager,
// verifying size, digest and zip layout before it takes its final name. On
// any failure or cancel nothing is left behind. On success every other
// Itch-io .muxapp in dir is removed, so ARCHIVE holds exactly the one the
// user chose (spec §5).
func SaveToArchive(ctx context.Context, hc *http.Client, userAgent string, rel Release, dir string,
	progress func(done, total int64)) (string, error) {
	if dir == "" || rel.Asset == "" || rel.Size <= 0 || !strings.HasPrefix(rel.Digest, "sha256:") {
		return "", fmt.Errorf("%s has no downloadable muOS asset with a digest", rel.Tag)
	}
	name := AssetName(rel.Tag)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("appupdate: create %s: %v", dir, err)
		return "", err
	}
	need := rel.Size + rel.Size/10
	if free, ok := freeBytes(dir); ok && free < need {
		logger.Warn("appupdate: not enough space in %s: need %d, have %d", dir, need, free)
		return "", &SpaceError{Need: need, Have: free}
	}

	dest := filepath.Join(dir, name)
	pf, err := partfile.Create(dest)
	if err != nil {
		logger.Error("appupdate: create partial for %s: %v", dest, err)
		return "", err
	}
	defer pf.Abort() // no-op after Commit

	start := time.Now()
	logger.Info("appupdate: downloading %s (%d bytes) to %s", name, rel.Size, dir)
	sum, done, err := fetchAsset(ctx, hc, userAgent, rel.Asset, pf, rel.Size, progress)
	if err != nil {
		logger.Error("appupdate: download %s failed after %d bytes: %s", name, done, netstate.Detail(err))
		return "", err
	}
	if done != rel.Size {
		logger.Error("appupdate: %s is %d bytes, release says %d", name, done, rel.Size)
		return "", ErrIntegrity
	}
	want := strings.TrimPrefix(rel.Digest, "sha256:")
	if !strings.EqualFold(sum, want) {
		logger.Error("appupdate: verify mismatch for %s: got sha256:%s want sha256:%s", name, sum, want)
		return "", ErrIntegrity
	}
	logger.Info("appupdate: verify ok for %s", name)
	if err := ValidateMuxapp(partfile.PathFor(dest)); err != nil {
		logger.Error("appupdate: %s rejected: %v", name, err)
		return "", err
	}
	if err := pf.Commit(); err != nil {
		return "", err
	}
	logger.Info("appupdate: saved %s in %v", dest, time.Since(start).Round(time.Millisecond))
	CleanOtherArchives(dir, name)
	return dest, nil
}

// fetchAsset streams url into w, hashing as it goes. The idle timer is armed
// before the request, so a server that never answers is caught too.
func fetchAsset(ctx context.Context, hc *http.Client, userAgent, url string, w io.Writer, total int64,
	progress func(done, total int64)) (string, int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var idled atomic.Bool
	idle := time.AfterFunc(idleTimeout, func() { idled.Store(true); cancel() })
	defer idle.Stop()
	fail := func(err error) error {
		if idled.Load() {
			return errIdle
		}
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, &netstate.StatusError{What: "download update", Code: resp.StatusCode}
	}

	h := sha256.New()
	buf := make([]byte, 64<<10)
	var done, lastLogged int64
	for {
		idle.Reset(idleTimeout)
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return "", done, err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
			if done-lastLogged >= 1<<20 {
				logger.Debug("appupdate: downloaded %d/%d bytes", done, total)
				lastLogged = done
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", done, fail(rerr)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), done, nil
}

// ValidateMuxapp applies Archive Manager's SAFE_ARCHIVE rules, and ours:
// every entry under Itch-io/, and the launcher present.
func ValidateMuxapp(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a zip: %w", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > maxEntries {
		return fmt.Errorf("%d entries", len(zr.File))
	}
	var total uint64
	launcher := false
	for _, f := range zr.File {
		n := f.Name
		switch {
		case n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, "\\"):
			return fmt.Errorf("unsafe entry %q", n)
		case !strings.HasPrefix(n, "Itch-io/"):
			return fmt.Errorf("entry %q is outside Itch-io/", n)
		case f.UncompressedSize64 > maxEntrySize:
			return fmt.Errorf("entry %q is too large", n)
		}
		for _, seg := range strings.Split(n, "/") {
			if seg == ".." {
				return fmt.Errorf("unsafe entry %q", n)
			}
		}
		if m := f.Mode(); m&fs.ModeSymlink != 0 || !(m.IsRegular() || m.IsDir()) {
			return fmt.Errorf("entry %q is not a plain file or folder", n)
		}
		total += f.UncompressedSize64
		if n == "Itch-io/mux_launch.sh" {
			launcher = true
		}
	}
	if total > maxTotalSize {
		return fmt.Errorf("%d bytes uncompressed", total)
	}
	if !launcher {
		return errors.New("no Itch-io/mux_launch.sh")
	}
	return nil
}

// CleanOtherArchives removes every Itch-io .muxapp in dir except keep. Only
// names matching our release naming are touched.
func CleanOtherArchives(dir, keep string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("appupdate: list %s: %v", dir, err)
		return nil
	}
	var removed []string
	for _, e := range entries {
		n := e.Name()
		if n == keep || e.IsDir() || !ourArchive.MatchString(n) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			logger.Warn("appupdate: remove old %s: %v", n, err)
			continue
		}
		logger.Info("appupdate: removed %s from ARCHIVE", n)
		removed = append(removed, n)
	}
	return removed
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/appupdate/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/appupdate/archive.go internal/appupdate/archive_test.go internal/appupdate/freespace_*.go
git commit -m "appupdate: save a verified .muxapp to ARCHIVE for Archive Manager"
```

---

### Task 9: The Checker

**Files:**
- Create: `internal/appupdate/checker.go`
- Test: `internal/appupdate/checker_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces:
  ```go
  type Config struct {
      Firmware firmware.Kind; Running string; Channel Channel
      StatePath, StoreDB, ArchiveDir, UserAgent string
      Source *Source  // nil → NewSource(UserAgent)
      Notify func()   // wakes the UI; called from background goroutines
  }
  type ArchiveState int // ArchiveIdle, ArchiveRunning, ArchiveDone, ArchiveFailed
  type ArchiveStatus struct { State ArchiveState; Done, Total int64; Path string; Err error }
  func NewChecker(cfg Config) *Checker
  func (c *Checker) Enabled() bool
  func (c *Checker) Start()
  func (c *Checker) CheckNow()
  func (c *Checker) RetryAfterReconnect()
  func (c *Checker) IsRunning() bool
  func (c *Checker) CheckedAt() time.Time
  func (c *Checker) LastError() error
  func (c *Checker) RateLimitedUntil() time.Time   // zero when not limited
  func (c *Checker) Channel() Channel
  func (c *Checker) SetChannel(ch Channel)
  func (c *Checker) Verdict() Verdict
  func (c *Checker) PendingNotice() (Verdict, bool)
  func (c *Checker) MarkNotified(ch Channel, tag string)
  func (c *Checker) StartArchiveSave()
  func (c *Checker) CancelArchiveSave()
  func (c *Checker) ArchiveStatus() ArchiveStatus
  ```

- [ ] **Step 1: Write the failing test**

```go
package appupdate

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
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

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
```

(Add `"strconv"` to this test file's imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/appupdate/ -run Checker -v`
Expected: FAIL — `undefined: NewChecker`.

- [ ] **Step 3: Implement**

```go
package appupdate

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

// Config is what the checker needs from the app.
type Config struct {
	Firmware   firmware.Kind
	Running    string // this build's version, e.g. "v1.1.0-rc3"
	Channel    Channel
	StatePath  string // update_state.json; "" keeps state in memory only
	StoreDB    string // firmware.Env.PakStoreDB()
	ArchiveDir string // firmware.Env.ArchiveDir()
	UserAgent  string // itchio.BuildUserAgent(info, false)
	Source     *Source
	Notify     func()
}

// ArchiveState is where a Save to ARCHIVE is.
type ArchiveState int

const (
	ArchiveIdle ArchiveState = iota
	ArchiveRunning
	ArchiveDone
	ArchiveFailed
)

// ArchiveStatus is a snapshot of the ARCHIVE download.
type ArchiveStatus struct {
	State       ArchiveState
	Done, Total int64
	Path        string
	Err         error
}

// offlineNow is netstate's, replaceable in tests.
var offlineNow = netstate.Offline

// Checker runs update checks in the background and answers the UI. Every
// method is safe from any goroutine and none blocks on the network.
type Checker struct {
	cfg       Config
	running   Version
	runningOK bool
	src       *Source
	dl        *http.Client

	busy atomic.Bool
	owed atomic.Bool
	wg   sync.WaitGroup // background work; tests wait on it

	mu            sync.Mutex
	st            *State
	channel       Channel
	store         pakstore.Result
	lastErr       error
	archive       ArchiveStatus
	cancelArchive context.CancelFunc
}

// NewChecker loads the saved state and reads the Store database (one small
// local file); it makes no request.
func NewChecker(cfg Config) *Checker {
	c := &Checker{cfg: cfg, channel: cfg.Channel, src: cfg.Source, dl: newClient(0)}
	c.running, c.runningOK = Parse(cfg.Running)
	if c.src == nil {
		c.src = NewSource(cfg.UserAgent)
	}
	c.st = LoadState(cfg.StatePath)
	c.store = c.lookupStore()
	logger.Info("appupdate: running=%s parsed=%v firmware=%s channel=%s store=%s",
		cfg.Running, c.runningOK, cfg.Firmware, c.channel, c.store)
	return c
}

func (c *Checker) lookupStore() pakstore.Result {
	if c.cfg.Firmware != firmware.KindNextUI {
		return pakstore.Result{Status: pakstore.NotInstalled, Reason: "not NextUI"}
	}
	return pakstore.Lookup(c.cfg.StoreDB)
}

// Enabled: checks run only on a real firmware, for a build with a release tag.
func (c *Checker) Enabled() bool {
	return c.runningOK && (c.cfg.Firmware == firmware.KindNextUI || c.cfg.Firmware == firmware.KindMuOS)
}

// managedLocked: NextUI and the Store has (or may have) a row. Caller holds mu.
func (c *Checker) managedLocked() bool {
	return c.cfg.Firmware == firmware.KindNextUI && c.store.Status != pakstore.NotInstalled
}

// Start is the once-per-launch check.
func (c *Checker) Start() { c.maybeCheck("launch") }

// CheckNow is "Check now" on the Updates screen.
func (c *Checker) CheckNow() {
	logger.Info("appupdate: check requested from Settings")
	c.maybeCheck("manual")
}

// RetryAfterReconnect is registered with netstate.OnReconnect. It runs on the
// netstate goroutine, so it only starts a goroutine, and only when owed.
func (c *Checker) RetryAfterReconnect() {
	if c.owed.Swap(false) {
		logger.Info("appupdate: connection back, running the postponed check")
		c.maybeCheck("reconnect")
	}
}

func (c *Checker) maybeCheck(why string) {
	if !c.Enabled() {
		logger.Debug("appupdate: %s check skipped: firmware=%s running=%q", why, c.cfg.Firmware, c.cfg.Running)
		return
	}
	c.mu.Lock()
	ch, notBefore := c.channel, c.st.NotBefore
	c.mu.Unlock()
	if ch == Off {
		logger.Debug("appupdate: %s check skipped: channel is off", why)
		return
	}
	if time.Now().Before(notBefore) {
		logger.Info("appupdate: %s check skipped: rate-limited until %s", why, notBefore.Format(time.RFC3339))
		return
	}
	// Owed first, then read: the same order as inventory's postponeIfOffline,
	// so a reconnect landing between the two is not lost.
	c.owed.Store(true)
	if offlineNow() {
		logger.Info("appupdate: offline, %s check owed until the connection is back", why)
		return
	}
	c.owed.Store(false)
	if !c.busy.CompareAndSwap(false, true) {
		logger.Debug("appupdate: %s check skipped: one is already running", why)
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.notify()
		defer c.busy.Store(false)
		c.run(why)
	}()
}

func (c *Checker) run(why string) {
	start := time.Now()
	store := c.lookupStore()

	c.mu.Lock()
	c.store = store
	ch := c.channel
	managed := c.managedLocked()
	etagRel, etagPak := c.st.ETag[etagReleases], c.st.ETag[etagPakJSON]
	c.mu.Unlock()

	// Stable on a Store-managed install reads the Store's own source. An rc on
	// a Store-managed install also reads it, for the downgrade warning (§2a).
	useReleases := !(managed && ch == Stable)
	usePak := managed && (ch == Stable || c.running.IsRC())
	logger.Info("appupdate: check start why=%s channel=%s releases=%v pakjson=%v store=%s",
		why, ch, useReleases, usePak, store)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var rel ReleasesResult
	var relErr, pakErr error
	var pak *Release
	var pakETag string
	var pakNM bool
	if useReleases {
		rel, relErr = c.src.Releases(ctx, etagRel)
	}
	if usePak {
		pak, pakETag, pakNM, pakErr = c.src.PakJSON(ctx, etagPak)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	failed := relErr
	if useReleases && relErr == nil {
		c.st.ETag[etagReleases] = rel.ETag
		if !rel.NotModified {
			if rel.RC != nil {
				c.st.Latest[RC] = rel.RC
			}
			if rel.Stable != nil {
				c.st.Latest[Stable] = rel.Stable
			} else {
				logger.Warn("appupdate: no stable release on the page; keeping %s", tagOf(c.st.Latest[Stable]))
			}
		}
	}
	if usePak {
		if pakErr != nil {
			if failed == nil {
				failed = pakErr
			}
		} else {
			c.st.ETag[etagPakJSON] = pakETag
			if !pakNM {
				c.st.PakJSON = pak
			}
		}
	}
	var rl *RateLimitError
	switch {
	case errors.As(failed, &rl):
		c.st.NotBefore = rl.Until
		logger.Warn("appupdate: rate-limit back-off until %s", rl.Until.Format(time.RFC3339))
	case failed != nil:
		logger.Warn("appupdate: check failed after %v: %s", time.Since(start).Round(time.Millisecond), netstate.Detail(failed))
	default:
		c.st.CheckedAt = time.Now()
	}
	c.lastErr = failed
	c.saveLocked()
	v := c.verdictLocked()
	logger.Info("appupdate: verdict %s running=%s latest=%s store=%s via=%s store_offers=%q in %v",
		v.Kind, c.running, tagOf(v.Latest), c.store, v.Via, v.StoreOffers, time.Since(start).Round(time.Millisecond))
}

func (c *Checker) saveLocked() {
	if c.cfg.StatePath == "" {
		return
	}
	if err := c.st.Save(c.cfg.StatePath); err != nil {
		logger.Error("appupdate: save state: %v", err)
	}
}

func (c *Checker) verdictLocked() Verdict {
	if !c.runningOK {
		return Verdict{Channel: c.channel}
	}
	latest := c.st.Latest[c.channel]
	if c.managedLocked() && c.channel == Stable {
		latest = c.st.PakJSON
	}
	return Decide(Inputs{Firmware: c.cfg.Firmware, Running: c.running, Channel: c.channel,
		Store: c.store, Latest: latest, PakJSON: c.st.PakJSON})
}

// Verdict is the current answer, from the last successful check.
func (c *Checker) Verdict() Verdict {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verdictLocked()
}

func (c *Checker) IsRunning() bool { return c.busy.Load() }

func (c *Checker) CheckedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.CheckedAt
}

func (c *Checker) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// RateLimitedUntil is when GitHub allows the next check; zero when now.
func (c *Checker) RateLimitedUntil() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.st.NotBefore) {
		return c.st.NotBefore
	}
	return time.Time{}
}

func (c *Checker) Channel() Channel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.channel
}

// SetChannel switches channel. The cached result for it is used straight
// away; a check runs only when there is none (spec §2a).
func (c *Checker) SetChannel(ch Channel) {
	c.mu.Lock()
	prev := c.channel
	c.channel = ch
	cached := c.st.Latest[ch] != nil
	if c.managedLocked() && ch == Stable {
		cached = c.st.PakJSON != nil
	}
	c.mu.Unlock()
	logger.Info("appupdate: channel %s → %s (cached=%v)", prev, ch, cached)
	if ch != Off && !cached {
		c.maybeCheck("channel")
	}
}

// PendingNotice reports whether the notice is due (spec §2).
func (c *Checker) PendingNotice() (Verdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.verdictLocked()
	return v, ShouldNotify(v, c.st.Notified[v.Channel])
}

// MarkNotified records that the user has been told about tag on ch: when the
// notice finished its animation, or when the Updates screen showed it. It
// never lowers the recorded version.
func (c *Checker) MarkNotified(ch Channel, tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ShouldNotify(Verdict{Kind: Available, Latest: &Release{Tag: tag}}, c.st.Notified[ch]) {
		return
	}
	c.st.Notified[ch] = tag
	c.saveLocked()
	logger.Info("appupdate: notified channel=%s version=%s", ch, tag)
}

// StartArchiveSave downloads the current ViaArchive release into ARCHIVE/.
func (c *Checker) StartArchiveSave() {
	c.mu.Lock()
	v := c.verdictLocked()
	if v.Kind != Available || v.Via != ViaArchive || c.archive.State == ArchiveRunning || c.cfg.ArchiveDir == "" {
		c.mu.Unlock()
		logger.Debug("appupdate: Save to ARCHIVE ignored: verdict=%s via=%s state=%d", v.Kind, v.Via, c.archive.State)
		return
	}
	r := *v.Latest
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelArchive = cancel
	c.archive = ArchiveStatus{State: ArchiveRunning, Total: r.Size}
	c.mu.Unlock()

	logger.Info("appupdate: Save to ARCHIVE started for %s", r.Tag)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		path, err := SaveToArchive(ctx, c.dl, c.cfg.UserAgent, r, c.cfg.ArchiveDir, func(done, _ int64) {
			c.mu.Lock()
			c.archive.Done = done
			c.mu.Unlock()
		})
		c.mu.Lock()
		if err != nil {
			c.archive.State, c.archive.Err = ArchiveFailed, err
		} else {
			c.archive.State, c.archive.Path = ArchiveDone, path
		}
		c.cancelArchive = nil
		c.mu.Unlock()
		logger.Info("appupdate: Save to ARCHIVE finished for %s: err=%v", r.Tag, err)
		c.notify()
	}()
}

// CancelArchiveSave stops a running download; nothing is left behind.
func (c *Checker) CancelArchiveSave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelArchive != nil {
		logger.Info("appupdate: Save to ARCHIVE cancelled")
		c.cancelArchive()
	}
}

func (c *Checker) ArchiveStatus() ArchiveStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.archive
}

func (c *Checker) notify() {
	if c.cfg.Notify != nil {
		c.cfg.Notify()
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/appupdate/ -v`
Expected: PASS, no race reports.

- [ ] **Step 5: Commit**

```bash
git add internal/appupdate/checker.go internal/appupdate/checker_test.go
git commit -m "appupdate: background checker with owed checks, channel switching and ARCHIVE saves"
```

---

### Task 10: The Updates screen

**Files:**
- Create: `internal/ui/app_updater.go`
- Create: `internal/ui/screen_updates.go`
- Create: `internal/ui/updates_status.go`
- Test: `internal/ui/screen_updates_test.go`

**Interfaces:**
- Consumes: `appupdate.*` (Tasks 1–9); existing `humanBytes`, `repeatDelay`, `currentRepeatInterval`, `abbreviate`, `btnA`/`btnB`.
- Produces:
  ```go
  type AppUpdater interface { Verdict() appupdate.Verdict; Channel() appupdate.Channel; SetChannel(appupdate.Channel)
      CheckNow(); IsRunning() bool; CheckedAt() time.Time; LastError() error; RateLimitedUntil() time.Time
      PendingNotice() (appupdate.Verdict, bool); MarkNotified(appupdate.Channel, string)
      StartArchiveSave(); CancelArchiveSave(); ArchiveStatus() appupdate.ArchiveStatus }
  func SetAppUpdater(u AppUpdater); func appUpdater() AppUpdater
  func NewUpdatesScreen(cfg *settings.Config, cfgPath string, up AppUpdater, prev Screen) *UpdatesScreen
  type statusLine struct{ Text string; Warn bool }
  type updatesStatusView struct{ Lines []statusLine; QR string }
  func updatesStatus(v appupdate.Verdict, a appupdate.ArchiveStatus, offline bool, lastErr error, rateUntil time.Time) updatesStatusView
  func archiveFailure(err error) string
  func appUpdateAnnotation(up AppUpdater) string
  func lastCheckedLabel(t time.Time) string                 // shared with Settings (Task 12)
  func annotationColor(r *renderer.Renderer, selected bool, tone [3]uint8, emphasised bool) [3]uint8
  func drawRowAnnotation(r *renderer.Renderer, text string, y int32, c [3]uint8)
  // test helper, screen_updates_test.go, reused by Task 12:
  type stubUpdater struct{ v appupdate.Verdict }            // implements AppUpdater
  ```

- [ ] **Step 1: Add the updater interface**

`internal/ui/app_updater.go`:

```go
//go:build !headless

package ui

import (
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
)

// AppUpdater is satisfied by *appupdate.Checker; an interface so offscreen
// scenes and tests can use a stand-in.
type AppUpdater interface {
	Verdict() appupdate.Verdict
	Channel() appupdate.Channel
	SetChannel(appupdate.Channel)
	CheckNow()
	IsRunning() bool
	CheckedAt() time.Time
	LastError() error
	RateLimitedUntil() time.Time
	PendingNotice() (appupdate.Verdict, bool)
	MarkNotified(ch appupdate.Channel, tag string)
	StartArchiveSave()
	CancelArchiveSave()
	ArchiveStatus() appupdate.ArchiveStatus
}

var (
	appUpdMu sync.RWMutex
	appUpd   AppUpdater
)

// SetAppUpdater installs the app-update checker. main_sdl.go calls it once at
// startup. Unset (the host, tests) hides Settings → Updates and the notice.
func SetAppUpdater(u AppUpdater) {
	appUpdMu.Lock()
	appUpd = u
	appUpdMu.Unlock()
}

func appUpdater() AppUpdater {
	appUpdMu.RLock()
	defer appUpdMu.RUnlock()
	return appUpd
}
```

- [ ] **Step 2: Write the failing test**

`internal/ui/screen_updates_test.go`:

```go
//go:build !headless

package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

// stubUpdater is a minimal AppUpdater; the Settings tests (Task 12) use it too.
type stubUpdater struct{ v appupdate.Verdict }

func (s *stubUpdater) Verdict() appupdate.Verdict               { return s.v }
func (s *stubUpdater) Channel() appupdate.Channel               { return s.v.Channel }
func (s *stubUpdater) SetChannel(ch appupdate.Channel)          { s.v.Channel = ch }
func (s *stubUpdater) CheckNow()                                {}
func (s *stubUpdater) IsRunning() bool                          { return false }
func (s *stubUpdater) CheckedAt() time.Time                     { return time.Time{} }
func (s *stubUpdater) LastError() error                         { return nil }
func (s *stubUpdater) RateLimitedUntil() time.Time              { return time.Time{} }
func (s *stubUpdater) PendingNotice() (appupdate.Verdict, bool) { return s.v, false }
func (s *stubUpdater) MarkNotified(appupdate.Channel, string)   {}
func (s *stubUpdater) StartArchiveSave()                        {}
func (s *stubUpdater) CancelArchiveSave()                       {}
func (s *stubUpdater) ArchiveStatus() appupdate.ArchiveStatus   { return appupdate.ArchiveStatus{} }

func ver(s string) appupdate.Version { v, _ := appupdate.Parse(s); return v }

func TestLastCheckedLabel(t *testing.T) {
	if lastCheckedLabel(time.Time{}) != "never" || lastCheckedLabel(time.Now().Add(-5*time.Minute)) != "last: 5m ago" {
		t.Fatal("unexpected labels")
	}
}

func newTestUpdatesScreen(t *testing.T, v appupdate.Verdict) (*UpdatesScreen, *stubUpdater) {
	up := &stubUpdater{v: v}
	cfg := &settings.Config{}
	return NewUpdatesScreen(cfg, filepath.Join(t.TempDir(), "config.json"), up, nil), up
}

var archiveVerdict = appupdate.Verdict{Kind: appupdate.Available, Via: appupdate.ViaArchive,
	Channel: appupdate.RC, Running: ver("v1.1.0-rc2"), Latest: &appupdate.Release{Tag: "v1.1.0-rc3", URL: "https://example.invalid/v1.1.0-rc3"}}

func TestUpdatesCursor_pressWrapsAtEnds(t *testing.T) {
	s, _ := newTestUpdatesScreen(t, archiveVerdict)
	s.startHold(-1)
	if s.cursor != uRowArchive {
		t.Fatalf("up from the first row: %d, want the archive row", s.cursor)
	}
	s.stopHold(-1)
	s.startHold(1)
	if s.cursor != uRowChannel {
		t.Fatalf("down from the last row: %d, want Channel", s.cursor)
	}
}

func TestUpdatesCursor_repeatStopsAtEnds(t *testing.T) {
	s, _ := newTestUpdatesScreen(t, archiveVerdict)
	s.moveCursor(-1, false)
	if s.cursor != uRowChannel {
		t.Fatalf("held up at the first row moved to %d", s.cursor)
	}
}

func TestUpdatesCursor_hiddenArchiveRowClamps(t *testing.T) {
	s, up := newTestUpdatesScreen(t, archiveVerdict)
	s.cursor = uRowArchive
	up.v = appupdate.Verdict{Kind: appupdate.UpToDate, Channel: appupdate.RC, Latest: &appupdate.Release{Tag: "v1.1.0-rc3"}}
	s.moveCursor(1, true)
	if s.cursor != uRowChannel {
		t.Fatalf("down from a vanished last row: %d, want to wrap to Channel", s.cursor)
	}
	s.cursor = uRowArchive
	s.clampCursor()
	if s.cursor != uRowCheck {
		t.Fatalf("clamp: %d, want Check now", s.cursor)
	}
}

func TestUpdatesChannelCycleSaves(t *testing.T) {
	s, up := newTestUpdatesScreen(t, appupdate.Verdict{Channel: appupdate.Stable})
	s.cursor = uRowChannel
	s.activate()
	if up.v.Channel != appupdate.RC || s.cfg.UpdateChannel != "rc" {
		t.Fatalf("after A: updater=%s cfg=%q, want rc", up.v.Channel, s.cfg.UpdateChannel)
	}
	if loaded, _ := settings.Load(s.cfgPath); loaded.UpdateChannel != "rc" {
		t.Fatalf("saved update_channel = %q", loaded.UpdateChannel)
	}
}

func TestUpdatesStatus(t *testing.T) {
	rel := func(tag string) *appupdate.Release { return &appupdate.Release{Tag: tag, URL: "https://example.invalid/" + tag} }
	cases := []struct {
		name    string
		v       appupdate.Verdict
		a       appupdate.ArchiveStatus
		offline bool
		err     error
		want    string // substring of the joined lines
		qr      bool
	}{
		{"off", appupdate.Verdict{Channel: appupdate.Off}, appupdate.ArchiveStatus{}, false, nil, "Update checks are off.", false},
		{"latest", appupdate.Verdict{Kind: appupdate.UpToDate, Running: ver("v1.1.0-rc3"), Latest: rel("v1.1.0-rc3")}, appupdate.ArchiveStatus{}, false, nil, "You have the latest version (v1.1.0-rc3).", false},
		{"ahead", appupdate.Verdict{Kind: appupdate.UpToDate, Ahead: true, Running: ver("v1.1.0-rc3"), Latest: rel("v1.0.25")}, appupdate.ArchiveStatus{}, false, nil,
			"You are on v1.1.0-rc3, a release candidate newer than the latest stable release (v1.0.25).", false},
		{"pak store", appupdate.Verdict{Kind: appupdate.Available, Via: appupdate.ViaPakStore, Latest: rel("v1.0.26")}, appupdate.ArchiveStatus{}, false, nil, "v1.0.26 is available. Update it in the Pak Store.", false},
		{"release page", appupdate.Verdict{Kind: appupdate.Available, Latest: rel("v1.1.0")}, appupdate.ArchiveStatus{}, false, nil, "v1.1.0 is available.", true},
		{"offline, nothing known", appupdate.Verdict{}, appupdate.ArchiveStatus{}, true, nil, "You're offline.", false},
		{"store downgrade", appupdate.Verdict{Kind: appupdate.UpToDate, Running: ver("v1.1.0-rc3"), Latest: rel("v1.1.0-rc3"), StoreOffers: "v1.0.25"}, appupdate.ArchiveStatus{}, false, nil,
			"The Pak Store may offer v1.0.25 as an update. It is older than this release candidate; installing it would replace it.", false},
		{"saved", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveDone}, false, nil, "Saved to ARCHIVE. Open Applications → Archive Manager to install.", true},
		{"integrity", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Err: appupdate.ErrIntegrity}, false, nil, "Download failed the integrity check.", true},
		{"cancelled", archiveVerdict, appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Err: context.Canceled}, false, nil, "Download cancelled.", true},
		{"check failed", appupdate.Verdict{}, appupdate.ArchiveStatus{}, false, errors.New("decode releases"), "The last check failed.", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			view := updatesStatus(c.v, c.a, c.offline, c.err, time.Time{})
			var texts []string
			for _, l := range view.Lines {
				texts = append(texts, l.Text)
			}
			joined := strings.Join(texts, " | ")
			if !strings.Contains(joined, c.want) {
				t.Fatalf("lines = %q, want %q", joined, c.want)
			}
			if (view.QR != "") != c.qr {
				t.Fatalf("QR = %q, want present=%v", view.QR, c.qr)
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'Updates' -v`
Expected: FAIL — `undefined: NewUpdatesScreen`.

- [ ] **Step 4: Implement the status builder and shared row helpers**

`internal/ui/updates_status.go`:

```go
//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

// statusLine is one paragraph under the Updates rows.
type statusLine struct {
	Text string
	Warn bool
}

// updatesStatusView is what the Updates screen says, independent of drawing.
type updatesStatusView struct {
	Lines []statusLine
	QR    string // release page to encode; "" for none
}

// updatesStatus builds the status block (spec §4, §2a).
func updatesStatus(v appupdate.Verdict, a appupdate.ArchiveStatus, offline bool, lastErr error, rateUntil time.Time) updatesStatusView {
	var out updatesStatusView
	add := func(text string, warn bool) { out.Lines = append(out.Lines, statusLine{text, warn}) }

	switch {
	case v.Channel == appupdate.Off:
		add("Update checks are off.", false)
		return out
	case v.Kind == appupdate.UpToDate && !v.Ahead:
		add(fmt.Sprintf("You have the latest version (%s).", v.Running), false)
	case v.Kind == appupdate.UpToDate && v.Running.IsRC():
		add(fmt.Sprintf("You are on %s, a release candidate newer than the latest stable release (%s). "+
			"You will be told when a newer stable release is out.", v.Running, v.Latest.Tag), false)
	case v.Kind == appupdate.UpToDate:
		add(fmt.Sprintf("You are on %s, newer than the latest release (%s).", v.Running, v.Latest.Tag), false)
	case v.Kind == appupdate.Available && v.Via == appupdate.ViaPakStore:
		add(fmt.Sprintf("%s is available. Update it in the Pak Store.", v.Latest.Tag), false)
	case v.Kind == appupdate.Available:
		add(fmt.Sprintf("%s is available.", v.Latest.Tag), false)
		out.QR = v.Latest.URL
	case offline:
		add("You're offline. Itch-io checks for updates when the connection is back.", true)
	default:
		add("Not checked yet.", false)
	}

	switch {
	case !rateUntil.IsZero():
		add("GitHub is limiting update checks. Try again after "+rateUntil.Local().Format("15:04")+".", true)
	case lastErr != nil:
		if m, ok := netstate.Describe(lastErr, "GitHub"); ok {
			add(m.Title, true)
			if m.Hint != "" {
				add(m.Hint, false)
			}
		} else {
			add("The last check failed.", true)
		}
	}

	switch a.State {
	case appupdate.ArchiveRunning:
		add(fmt.Sprintf("Downloading %s…", v.Latest.Tag), false)
	case appupdate.ArchiveDone:
		add("Saved to ARCHIVE. Open Applications → Archive Manager to install.", false)
	case appupdate.ArchiveFailed:
		add(archiveFailure(a.Err), true)
	}

	if v.StoreOffers != "" {
		older := "this release candidate"
		if !v.Running.IsRC() {
			older = "the version you have (" + v.Running.String() + ")"
		}
		add(fmt.Sprintf("The Pak Store may offer %s as an update. It is older than %s; installing it would replace it.",
			v.StoreOffers, older), true)
	}
	return out
}

// archiveFailure words a failed Save to ARCHIVE.
func archiveFailure(err error) string {
	var se *appupdate.SpaceError
	switch {
	case errors.Is(err, context.Canceled):
		return "Download cancelled."
	case errors.Is(err, appupdate.ErrIntegrity):
		return "Download failed the integrity check."
	case errors.As(err, &se):
		return fmt.Sprintf("Not enough space on the SD card: %s needed, %s free.", humanBytes(se.Need), humanBytes(se.Have))
	}
	if m, ok := netstate.Describe(err, "GitHub"); ok {
		return m.Title
	}
	return "Download failed."
}

// appUpdateAnnotation is the "Check now" row's right-aligned label.
func appUpdateAnnotation(up AppUpdater) string {
	switch {
	case netstate.Offline():
		return "offline"
	case up.IsRunning():
		return "checking…"
	case up.LastError() != nil:
		return "failed"
	}
	return lastCheckedLabel(up.CheckedAt())
}

// lastCheckedLabel is "last: 5m ago", or "never" for the zero time. Shared
// with Settings' Update Inventory row (Task 12).
func lastCheckedLabel(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "last: just now"
	case d < time.Hour:
		return fmt.Sprintf("last: %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("last: %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("last: %dd ago", int(d.Hours()/24))
	}
}

// annotationColor is the colour of a right-aligned row annotation. Warning and
// Muted are toned against Background, so on the selected row — filled with an
// Accent pill — they can vanish (orange on orange). ToneOn keeps the hue and
// clears contrast against the pill; the Mix de-emphasises relative to the pill
// the way Settings' "not signed in" does. The same rule Settings has used for
// Update Inventory since the rc2 readability fix.
func annotationColor(r *renderer.Renderer, selected bool, tone [3]uint8, emphasised bool) [3]uint8 {
	switch {
	case selected && emphasised:
		return r.Theme.ToneOn(tone, r.Theme.Accent)
	case selected:
		return theme.Mix(r.Theme.Accent, r.Theme.AccentText, 65)
	case emphasised:
		return tone
	default:
		return r.Theme.Muted()
	}
}

// drawRowAnnotation right-aligns small text on a settings-style row at y.
func drawRowAnnotation(r *renderer.Renderer, text string, y int32, c [3]uint8) {
	aw, sh := r.SmallTextSize(text)
	_, fh := r.TextSize("Ag")
	r.DrawSmallText(text, r.W-aw-20, y+(fh-sh)/2, c[0], c[1], c[2])
}
```

- [ ] **Step 5: Implement the screen**

`internal/ui/screen_updates.go`:

```go
//go:build !headless

package ui

import (
	"fmt"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type updatesRow int

const (
	uRowChannel updatesRow = iota
	uRowCheck
	uRowArchive // muOS and ViaArchive only
)

// UpdatesScreen is Settings → Updates (spec §4).
type UpdatesScreen struct {
	cfg     *settings.Config
	cfgPath string
	up      AppUpdater
	prev    Screen

	cursor     updatesRow
	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time

	qrTex *sdl.Texture
	qrKey string
}

func NewUpdatesScreen(cfg *settings.Config, cfgPath string, up AppUpdater, prev Screen) *UpdatesScreen {
	logger.Info("updates: opened (channel=%s)", up.Channel())
	return &UpdatesScreen{cfg: cfg, cfgPath: cfgPath, up: up, prev: prev}
}

func (s *UpdatesScreen) archiveRowShown() bool {
	v := s.up.Verdict()
	return v.Kind == appupdate.Available && v.Via == appupdate.ViaArchive &&
		s.up.ArchiveStatus().State != appupdate.ArchiveRunning
}

func (s *UpdatesScreen) visibleRows() []updatesRow {
	rows := []updatesRow{uRowChannel, uRowCheck}
	if s.archiveRowShown() {
		rows = append(rows, uRowArchive)
	}
	return rows
}

// moveCursor steps through the visible rows. A press (wrap) goes round the
// ends; auto-repeat stops at them, like Settings.
func (s *UpdatesScreen) moveCursor(dir int, wrap bool) {
	rows := s.visibleRows()
	i := len(rows) - 1 // a row that just vanished was the last one
	for j, r := range rows {
		if r == s.cursor {
			i = j
		}
	}
	switch j := i + dir; {
	case j >= 0 && j < len(rows):
		s.cursor = rows[j]
	case wrap && j < 0:
		s.cursor = rows[len(rows)-1]
	case wrap:
		s.cursor = rows[0]
	default:
		s.cursor = rows[i]
	}
}

// clampCursor moves off a row that is no longer shown.
func (s *UpdatesScreen) clampCursor() {
	if s.cursor == uRowArchive && !s.archiveRowShown() {
		s.cursor = uRowCheck
	}
}

func (s *UpdatesScreen) startHold(dir int) {
	if s.heldDir == dir {
		return
	}
	s.heldDir, s.heldSince = dir, time.Now()
	s.lastRepeat = s.heldSince
	s.moveCursor(dir, true)
}

func (s *UpdatesScreen) stopHold(dir int) {
	if s.heldDir == dir {
		s.heldDir = 0
	}
}

func (s *UpdatesScreen) processAutoRepeat() {
	if s.heldDir == 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(s.heldSince)
	if elapsed < repeatDelay || now.Sub(s.lastRepeat) < currentRepeatInterval(elapsed-repeatDelay) {
		return
	}
	s.moveCursor(s.heldDir, false)
	s.lastRepeat = now
}

func (s *UpdatesScreen) NeedsRedraw() bool {
	return s.heldDir != 0 || s.up.IsRunning() || s.up.ArchiveStatus().State == appupdate.ArchiveRunning
}

func (s *UpdatesScreen) HasPendingAnimation() bool { return false }

func (s *UpdatesScreen) rowLabel(row updatesRow) string {
	switch row {
	case uRowChannel:
		return "Channel: " + s.up.Channel().Label()
	case uRowCheck:
		return "Check now"
	default:
		return "Save to ARCHIVE"
	}
}

func (s *UpdatesScreen) activate() Screen {
	switch s.cursor {
	case uRowChannel:
		next := s.up.Channel().Next()
		s.cfg.UpdateChannel = string(next)
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Warn("updates: save channel: %v", err)
		}
		s.up.SetChannel(next)
		logger.Info("updates: channel changed to %s", next)
	case uRowCheck:
		s.up.CheckNow()
	case uRowArchive:
		logger.Info("updates: Save to ARCHIVE requested")
		s.up.StartArchiveSave()
	}
	return s
}

func (s *UpdatesScreen) back() Screen {
	if s.qrTex != nil {
		s.qrTex.Destroy()
		s.qrTex = nil
	}
	return s.prev
}

func (s *UpdatesScreen) HandleEvent(e sdl.Event) Screen {
	downloading := s.up.ArchiveStatus().State == appupdate.ArchiveRunning
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		switch ev.Keysym.Sym {
		case sdl.K_DOWN, sdl.K_UP:
			dir := 1
			if ev.Keysym.Sym == sdl.K_UP {
				dir = -1
			}
			if ev.Type == sdl.KEYDOWN {
				s.startHold(dir)
			} else {
				s.stopHold(dir)
			}
			return s
		}
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_RETURN:
			return s.activate()
		case sdl.K_ESCAPE:
			if downloading {
				s.up.CancelArchiveSave()
				return s
			}
			return s.back()
		case sdl.K_s:
			return s.back()
		}
	case *sdl.ControllerButtonEvent:
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN, sdl.CONTROLLER_BUTTON_DPAD_UP:
			dir := 1
			if ev.Button == sdl.CONTROLLER_BUTTON_DPAD_UP {
				dir = -1
			}
			if ev.Type == sdl.CONTROLLERBUTTONDOWN {
				s.startHold(dir)
			} else {
				s.stopHold(dir)
			}
			return s
		}
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case btnA:
			return s.activate()
		case btnB:
			if downloading {
				s.up.CancelArchiveSave()
				return s
			}
			return s.back()
		case sdl.CONTROLLER_BUTTON_START:
			return s.back()
		}
	}
	return s
}

func (s *UpdatesScreen) Draw(r *renderer.Renderer) {
	s.processAutoRepeat()
	s.clampCursor()
	v := s.up.Verdict()
	a := s.up.ArchiveStatus()
	// Seeing it here counts as being told (spec §2).
	if v.Kind == appupdate.Available && v.Latest != nil {
		s.up.MarkNotified(v.Channel, v.Latest.Tag)
	}

	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])
	headerH, footerH := int32(72), int32(52)
	textY := r.DrawHeaderBar(headerH)
	mt := r.Theme.MainText
	r.DrawText("Updates", 20, textY, mt[0], mt[1], mt[2])

	_, fontH := r.TextSize("Ag")
	_, smallH := r.SmallTextSize("Ag")
	rowH := fontH + 14
	y := headerH + 10
	for _, row := range s.visibleRows() {
		sel := row == s.cursor
		tc := r.Theme.ListText
		if sel {
			ac := r.Theme.Accent
			r.DrawPill(4, y-4, r.W-8, rowH, ac[0], ac[1], ac[2])
			tc = r.Theme.AccentText
		}
		r.DrawText(s.rowLabel(row), 20, y, tc[0], tc[1], tc[2])
		if row == uRowCheck {
			warn := s.up.IsRunning() || netstate.Offline() || s.up.LastError() != nil
			drawRowAnnotation(r, appUpdateAnnotation(s.up), y, annotationColor(r, sel, r.Theme.Warning(), warn))
		}
		y += rowH
	}

	view := updatesStatus(v, a, netstate.Offline(), s.up.LastError(), s.up.RateLimitedUntil())
	y += 10
	statusTop := y
	textW := r.W - 40
	side := view.QR != "" && !abbreviate(r.W)
	var qrSize int32
	if side {
		qrSize = min(r.W/3, r.H-footerH-statusTop-smallH-16)
		qrSize = max(80, min(qrSize, 400))
		textW = r.W - 60 - qrSize
	}
	lineH := fontH + 4
	for _, ln := range view.Lines {
		c := r.Theme.MainText
		if ln.Warn {
			c = r.Theme.Warning()
		}
		y += r.DrawWrappedText(ln.Text, 20, y, textW, lineH, c[0], c[1], c[2]) + 6
	}

	if a.State == appupdate.ArchiveRunning {
		track, ok := r.Theme.ProgressTrack(), r.Theme.Success()
		r.DrawRect(20, y, textW, 20, track[0], track[1], track[2])
		if a.Total > 0 {
			r.DrawRect(20, y, int32(float64(textW)*float64(a.Done)/float64(a.Total)), 20, ok[0], ok[1], ok[2])
			mu := r.Theme.Muted()
			r.DrawSmallText(fmt.Sprintf("%d%%  (%s / %s)", a.Done*100/a.Total, humanBytes(a.Done), humanBytes(a.Total)),
				20, y+26, mu[0], mu[1], mu[2])
		}
		y += 26 + smallH + 6
	}

	if view.QR != "" {
		var qx, qy int32
		if side {
			qx, qy = r.W-20-qrSize, statusTop
		} else {
			qrSize = min(r.H-footerH-y-smallH-16, 400)
			qx, qy = (r.W-qrSize)/2, y
		}
		if qrSize >= 80 {
			s.drawQR(r, view.QR, qx, qy, qrSize)
			mu := r.Theme.Muted()
			r.DrawSmallTextCentered("Scan for the release notes", qx, qy+qrSize+4, qrSize, mu[0], mu[1], mu[2])
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	back := renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"}
	if a.State == appupdate.ArchiveRunning {
		back.Text = "Cancel"
	}
	r.DrawFooterHints([]renderer.FooterHint{
		{Kind: renderer.BadgeCircle, Label: "A", Text: "Select"}, back,
	}, ftrY)
	r.Present()
}

// drawQR keeps one texture per URL and size instead of rebuilding it every
// frame.
func (s *UpdatesScreen) drawQR(r *renderer.Renderer, url string, x, y, size int32) {
	key := fmt.Sprintf("%s@%d", url, size)
	if s.qrTex == nil || s.qrKey != key {
		if s.qrTex != nil {
			s.qrTex.Destroy()
			s.qrTex = nil
		}
		tex, err := r.QRTexture(url, int(size))
		if err != nil {
			logger.Error("updates: QR texture: %v", err)
			return
		}
		s.qrTex, s.qrKey = tex, key
		logger.Debug("updates: QR %dpx for %s", size, url)
	}
	r.DrawTextureAt(s.qrTex, x, y, size, size)
}
```

- [ ] **Step 6: Run tests and the colour check**

Run: `go test ./internal/ui/ -v 2>&1 | tail -20 && ./scripts/no-color-literals.sh`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app_updater.go internal/ui/screen_updates.go internal/ui/updates_status.go internal/ui/screen_updates_test.go
git commit -m "ui: Settings → Updates screen with channel, check, status, QR and Save to ARCHIVE"
```

---

### Task 11: Overlay hook and the notice

**Files:**
- Modify: `internal/renderer/renderer.go:47-67` (field), `:201-203` (`Present`)
- Create: `internal/ui/update_toast.go`
- Test: `internal/ui/update_toast_test.go`

**Interfaces:**
- Consumes: `appupdate.Verdict`, `ViaPakStore`, `Channel`; `AppUpdater`, `appUpdater()`, `UpdatesScreen` (Task 10); `compact`, `abbreviate`.
- Produces:
  ```go
  // renderer
  Overlay func(*Renderer) // called inside Present, before the swap
  // ui
  type UpdateNotice struct{...}
  func (n *UpdateNotice) Animating() bool
  func (n *UpdateNotice) Tick(r *renderer.Renderer, current Screen, now time.Time) bool
  func noticeText(v appupdate.Verdict, narrow bool) (title, sub string)
  func noticeShown(elapsed time.Duration) float64
  func noticeMargin(w, h int32) int32
  func noticeAllowed(s Screen) bool
  func drawNotice(r *renderer.Renderer, v appupdate.Verdict, shown float64)
  ```

- [ ] **Step 1: Write the failing test**

`internal/ui/update_toast_test.go`:

```go
//go:build !headless

package ui

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
)

func TestNoticeText(t *testing.T) {
	v := appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}}
	if ti, sub := noticeText(v, false); ti != "Itch-io v1.1.0 available" || sub != "Settings → Updates" {
		t.Errorf("wide = %q / %q", ti, sub)
	}
	v.Via = appupdate.ViaPakStore
	if _, sub := noticeText(v, false); sub != "Update it in the Pak Store" {
		t.Errorf("pak store sub = %q", sub)
	}
	if ti, sub := noticeText(v, true); ti != "Update available" || sub != "" {
		t.Errorf("narrow = %q / %q", ti, sub)
	}
}

func TestNoticeShown(t *testing.T) {
	cases := map[time.Duration]float64{
		-time.Millisecond:            0,
		0:                            0,
		noticeSlide:                  1,
		noticeSlide + noticeHold/2:   1,
		noticeSlide + noticeHold:     1,
		noticeTotal:                  0,
		noticeTotal + time.Second:    0,
	}
	for at, want := range cases {
		if got := noticeShown(at); got != want {
			t.Errorf("noticeShown(%v) = %v, want %v", at, got, want)
		}
	}
	if mid := noticeShown(noticeSlide / 2); mid <= 0.5 || mid >= 1 {
		t.Errorf("ease-out: halfway in = %v, want above 0.5", mid)
	}
}

func TestNoticeMargin(t *testing.T) {
	if noticeMargin(1024, 768) != 12 || noticeMargin(640, 480) != 6 {
		t.Fatal("margin must be 12 px, 6 px when compact")
	}
}

func TestNoticeAllowed(t *testing.T) {
	list := &ListScreen{}
	if !noticeAllowed(list) {
		t.Error("an idle game list allows the notice")
	}
	list.loading.Store(true)
	if noticeAllowed(list) {
		t.Error("not over the startup loading")
	}
	for _, s := range []Screen{&SignInScreen{}, &AccountPromptScreen{}, &UpdatesScreen{}, &CacheRefreshScreen{}, &MigrateFlowScreen{}} {
		if noticeAllowed(s) {
			t.Errorf("%T must hold the notice back", s)
		}
	}
	if !noticeAllowed(&AboutScreen{}) {
		t.Error("ordinary screens allow it")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run Notice -v`
Expected: FAIL — `undefined: noticeText`.

- [ ] **Step 3: Add the Overlay hook**

In `internal/renderer/renderer.go`, in `type Renderer struct` after `descCache`:

```go
	// Overlay, when set, draws over whatever the current screen drew, just
	// before the frame is shown. Used by the app-update notice so no screen
	// has to know about it. It must not call Present.
	Overlay func(*Renderer)
```

Replace `Present`:

```go
func (r *Renderer) Present() {
	if r.Overlay != nil {
		r.Overlay(r)
	}
	r.Renderer.Present()
}
```

- [ ] **Step 4: Implement the notice**

`internal/ui/update_toast.go`:

```go
//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
)

const (
	noticeSlide = 250 * time.Millisecond
	noticeHold  = 4 * time.Second
	noticeTotal = 2*noticeSlide + noticeHold
)

// UpdateNotice is the "new version" notice, drawn top-right over whatever
// screen is current (spec §3). main_sdl.go owns one and calls Tick every loop
// iteration. It consumes no input.
type UpdateNotice struct {
	verdict appupdate.Verdict
	start   time.Time
	active  bool
}

// Animating reports whether the loop must redraw every 16 ms.
func (n *UpdateNotice) Animating() bool { return n.active }

// Tick starts the notice when one is due and the current screen allows it,
// and retires it when the animation ends. true means redraw now.
func (n *UpdateNotice) Tick(r *renderer.Renderer, current Screen, now time.Time) bool {
	up := appUpdater()
	if n.active {
		if now.Sub(n.start) < noticeTotal {
			return true
		}
		n.active = false
		r.Overlay = nil
		// Recorded once the notice has been seen in full, so a crash or quick
		// exit does not swallow it (spec §2).
		if up != nil {
			up.MarkNotified(n.verdict.Channel, n.verdict.Latest.Tag)
		}
		logger.Debug("update notice: finished for %s", n.verdict.Latest.Tag)
		return true // one more frame, without it
	}
	if up == nil || !noticeAllowed(current) {
		return false
	}
	v, ok := up.PendingNotice()
	if !ok {
		return false
	}
	n.verdict, n.start, n.active = v, now, true
	r.Overlay = func(r *renderer.Renderer) {
		drawNotice(r, n.verdict, noticeShown(time.Since(n.start)))
	}
	logger.Info("update notice: showing %s (channel=%s via=%s)", v.Latest.Tag, v.Channel, v.Via)
	return true
}

// noticeAllowed keeps the notice off startup and sign-in, and off the Updates
// screen, which already says it (spec §2, §3).
func noticeAllowed(s Screen) bool {
	switch sc := s.(type) {
	case *SignInScreen, *AccountPromptScreen, *UpdatesScreen, *CacheRefreshScreen, *MigrateFlowScreen:
		return false
	case *ListScreen:
		return !sc.loading.Load()
	}
	return true
}

// noticeText is the notice's wording; narrow (abbreviate) fits one line.
func noticeText(v appupdate.Verdict, narrow bool) (title, sub string) {
	if narrow {
		return "Update available", ""
	}
	title = "Itch-io " + v.Latest.Tag + " available"
	if v.Via == appupdate.ViaPakStore {
		return title, "Update it in the Pak Store"
	}
	return title, "Settings → Updates"
}

// noticeShown is how far the notice has slid in: 0 above the screen, 1 at
// rest. Ease-out on the way in, ease-in on the way out.
func noticeShown(elapsed time.Duration) float64 {
	switch {
	case elapsed <= 0 || elapsed >= noticeTotal:
		return 0
	case elapsed < noticeSlide:
		p := float64(elapsed) / float64(noticeSlide)
		return 1 - (1-p)*(1-p)
	case elapsed <= noticeSlide+noticeHold:
		return 1
	default:
		p := float64(elapsed-noticeSlide-noticeHold) / float64(noticeSlide)
		return 1 - p*p
	}
}

// noticeMargin insets the notice from the top and right edges.
func noticeMargin(w, h int32) int32 {
	if compact(w, h) {
		return 6
	}
	return 12
}

// drawNotice draws the notice shown (0..1) of the way in. Accent fill with a
// ModalBorder outline, so it does not read as one more header pill on the
// game list (those are TitlePill and Chip).
func drawNotice(r *renderer.Renderer, v appupdate.Verdict, shown float64) {
	if shown <= 0 || v.Latest == nil {
		return
	}
	title, sub := noticeText(v, abbreviate(r.W))
	m := noticeMargin(r.W, r.H)
	padX, padY := int32(16), int32(8)
	if compact(r.W, r.H) {
		padX, padY = 10, 5
	}
	tw, th := r.TextSize(title)
	var sw, sh int32
	if sub != "" {
		sw, sh = r.SmallTextSize(sub)
	}
	w := max(tw, sw) + 2*padX
	h := th + 2*padY
	if sub != "" {
		h += sh + 2
	}
	x := r.W - m - w
	// From fully above the top edge (y = -h) down to the margin.
	y := int32(float64(-h-2) + shown*float64(h+2+m))

	bd, ac, at := r.Theme.ModalBorder(), r.Theme.Accent, r.Theme.AccentText
	r.DrawPill(x-1, y-1, w+2, h+2, bd[0], bd[1], bd[2])
	r.DrawPill(x, y, w, h, ac[0], ac[1], ac[2])
	r.DrawText(title, x+padX, y+padY, at[0], at[1], at[2])
	if sub != "" {
		r.DrawSmallText(sub, x+padX, y+padY+th+2, at[0], at[1], at[2])
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ui/ ./internal/renderer/ 2>&1 | tail -5`
Expected: `ok` for both packages.

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/renderer.go internal/ui/update_toast.go internal/ui/update_toast_test.go
git commit -m "ui: top-right update notice drawn through a Present overlay"
```

---

### Task 12: Settings → "Updates" row

**Files:**
- Modify: `internal/ui/screen_settings.go`
- Test: `internal/ui/screen_settings_test.go`

**Interfaces:**
- Consumes: `AppUpdater`, `appUpdater()`, `NewUpdatesScreen`, `annotationColor`, `drawRowAnnotation`, `lastCheckedLabel`, and the test helper `stubUpdater` (all Task 10).
- Produces: `sItemUpdates` (between `sItemContentModeration` and `sItemAbout`); `SettingsScreen.appUpd AppUpdater` captured in `NewSettingsScreen`; `updatesRowAnnotation(v appupdate.Verdict) string`.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/screen_settings_test.go`:

```go
func TestUpdatesRow_hiddenWithoutUpdater(t *testing.T) {
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemContentModeration
	s.moveCursor(1, false)
	if s.cursor != sItemAbout {
		t.Fatalf("without an updater the cursor must skip Updates, got %d", s.cursor)
	}
}

func TestUpdatesRow_shownWithUpdater(t *testing.T) {
	SetAppUpdater(&stubUpdater{})
	t.Cleanup(func() { SetAppUpdater(nil) })
	s := newTestSettingsScreen(t, nil)
	s.cursor = sItemContentModeration
	s.moveCursor(1, false)
	if s.cursor != sItemUpdates {
		t.Fatalf("cursor = %d, want sItemUpdates", s.cursor)
	}
}

func TestUpdatesRowAnnotation(t *testing.T) {
	if got := updatesRowAnnotation(appupdate.Verdict{Kind: appupdate.Available, Latest: &appupdate.Release{Tag: "v1.1.0"}}); got != "v1.1.0 available" {
		t.Errorf("available: %q", got)
	}
	if got := updatesRowAnnotation(appupdate.Verdict{Kind: appupdate.UpToDate, Latest: &appupdate.Release{Tag: "v1.1.0"}}); got != "" {
		t.Errorf("up to date: %q, want nothing", got)
	}
}

```

(Add `"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"` to the test imports. `stubUpdater` lives in `screen_updates_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'UpdatesRow' -v`
Expected: FAIL — `undefined: sItemUpdates`.

- [ ] **Step 3: Implement**

In `screen_settings.go`:

1. Enum: insert `sItemUpdates` between `sItemContentModeration` and `sItemAbout`.
2. Struct: add field `appUpd AppUpdater // nil hides the Updates row` and in `NewSettingsScreen` set `appUpd: appUpdater(),` in the literal.
3. `rowHidden`: add
   ```go
   	case sItemUpdates:
   		return s.appUpd == nil
   ```
4. `Draw`: before `items = append(items, menuItem{sItemAbout, "About"})`:
   ```go
   	if s.appUpd != nil {
   		items = append(items, menuItem{sItemUpdates, "Updates >"})
   	}
   ```
5. Replace the whole Update Inventory annotation block (the `if item.id == sItemUpdateInventory …` block with its colour `switch`) with the shared helpers from Task 10, and add the Updates annotation after it:
   ```go
   		if item.id == sItemUpdateInventory && s.updateSvc != nil {
   			annotation := updateInventoryAnnotation(s.updateSvc)
   			warn := s.updateSvc.IsRunning() || netstate.Offline()
   			drawRowAnnotation(r, annotation, y, annotationColor(r, isSelected, r.Theme.Warning(), warn))
   		}
   		if item.id == sItemUpdates && s.appUpd != nil {
   			if a := updatesRowAnnotation(s.appUpd.Verdict()); a != "" {
   				drawRowAnnotation(r, a, y, annotationColor(r, isSelected, r.Theme.SuccessAction(), true))
   			}
   		}
   ```
6. Below `updateInventoryAnnotation`, add:
   ```go
   // updatesRowAnnotation is "v1.1.0 available", or nothing.
   func updatesRowAnnotation(v appupdate.Verdict) string {
   	if v.Kind == appupdate.Available && v.Latest != nil {
   		return v.Latest.Tag + " available"
   	}
   	return ""
   }
   ```
   and reduce `updateInventoryAnnotation` to use Task 10's `lastCheckedLabel`:
   ```go
   func updateInventoryAnnotation(svc UpdateServicer) string {
   	if netstate.Offline() {
   		return "offline"
   	}
   	if svc.IsRunning() {
   		return "checking…"
   	}
   	return lastCheckedLabel(svc.LatestCheckedAt())
   }
   ```
7. `activate`: add
   ```go
   	case sItemUpdates:
   		return NewUpdatesScreen(s.cfg, s.cfgPath, s.appUpd, s)
   ```
8. Imports: add `"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"`, and remove `"fmt"` — its only use was the time formatting that moved to `lastCheckedLabel` (`go build` will say so if another use remains; keep it then).

- [ ] **Step 4: Run tests and the colour check**

Run: `go test ./internal/ui/ -v 2>&1 | tail -15 && ./scripts/no-color-literals.sh`
Expected: PASS; the colour-literal check passes.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_settings.go internal/ui/screen_settings_test.go
git commit -m "Settings: Updates row with an available-version annotation, opening Settings → Updates"
```

---

### Task 13: Offscreen scenes and the palette audit

**Files:**
- Create: `internal/ui/dev_scenes_updates.go`

**Interfaces:**
- Consumes: `devList`, `devScenes`, `SceneDeps`, `NewSettingsScreen`, `NewUpdatesScreen`, `drawNotice`, `SetAppUpdater`.
- Produces: scenes `update-toast`, `update-toast-pakstore`, `settings-updates-selected`, `updates-latest`, `updates-available`, `updates-pakstore`, `updates-ahead`, `updates-offline`, `updates-archive-progress`, `updates-archive-done`, `updates-archive-failed`.

- [ ] **Step 1: Write the scenes**

```go
//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
)

// devAppUpdater is a stand-in for *appupdate.Checker so scenes can show every
// Updates state without a network.
type devAppUpdater struct {
	v       appupdate.Verdict
	running bool
	at      time.Time
	err     error
	archive appupdate.ArchiveStatus
}

func (d *devAppUpdater) Verdict() appupdate.Verdict                { return d.v }
func (d *devAppUpdater) Channel() appupdate.Channel                { return d.v.Channel }
func (d *devAppUpdater) SetChannel(ch appupdate.Channel)           { d.v.Channel = ch }
func (d *devAppUpdater) CheckNow()                                 {}
func (d *devAppUpdater) IsRunning() bool                           { return d.running }
func (d *devAppUpdater) CheckedAt() time.Time                      { return d.at }
func (d *devAppUpdater) LastError() error                          { return d.err }
func (d *devAppUpdater) RateLimitedUntil() time.Time               { return time.Time{} }
func (d *devAppUpdater) PendingNotice() (appupdate.Verdict, bool)  { return d.v, false }
func (d *devAppUpdater) MarkNotified(appupdate.Channel, string)    {}
func (d *devAppUpdater) StartArchiveSave()                         {}
func (d *devAppUpdater) CancelArchiveSave()                        {}
func (d *devAppUpdater) ArchiveStatus() appupdate.ArchiveStatus    { return d.archive }

func devVer(s string) appupdate.Version { v, _ := appupdate.Parse(s); return v }

func devRelease(tag string) *appupdate.Release {
	return &appupdate.Release{Tag: tag, URL: "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/" + tag,
		Asset: "https://example.invalid/" + appupdate.AssetName(tag), Digest: "sha256:00", Size: 12819887}
}

// noticeScene draws a screen with the notice resting over it.
type noticeScene struct {
	Screen
	v appupdate.Verdict
}

func (s noticeScene) Draw(r *renderer.Renderer) {
	r.Overlay = func(r *renderer.Renderer) { drawNotice(r, s.v, 1) }
	defer func() { r.Overlay = nil }()
	s.Screen.Draw(r)
}

func devUpdates(d SceneDeps, u *devAppUpdater) Screen {
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	return NewUpdatesScreen(d.Cfg, d.CfgPath, u, devList(d))
}

func init() {
	rcAvail := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.Stable,
		Running: devVer("v1.1.0-rc4"), Latest: devRelease("v1.1.0"), Via: appupdate.ViaReleasePage}
	storeAvail := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.Stable,
		Running: devVer("v1.0.25"), Latest: devRelease("v1.0.26"), Via: appupdate.ViaPakStore}
	archive := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.RC,
		Running: devVer("v1.1.0-rc2"), Latest: devRelease("v1.1.0-rc3"), Via: appupdate.ViaArchive}

	devScenes = append(devScenes,
		// Over the game list, whose top-right header holds the sort, platform
		// and Offline pills: the audit checks the notice against them.
		Scene{Name: "update-toast", Desc: "Update notice resting top-right over the game list", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			return noticeScene{devList(d), rcAvail}
		}},
		Scene{Name: "update-toast-pakstore", Desc: "Update notice pointing at the Pak Store", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			return noticeScene{devList(d), storeAvail}
		}},
		Scene{Name: "settings-updates-selected", Desc: "Settings, cursor on Updates, version available (annotation on the Accent pill)", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			SetAppUpdater(&devAppUpdater{v: rcAvail})
			defer SetAppUpdater(nil)
			s := devSettings(d)
			s.cursor = sItemUpdates
			return s
		}},
		Scene{Name: "updates-latest", Desc: "Updates: up to date", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now().Add(-5 * time.Minute), v: appupdate.Verdict{
				Kind: appupdate.UpToDate, Channel: appupdate.RC, Running: devVer("v1.1.0-rc3"), Latest: devRelease("v1.1.0-rc3")}})
		}},
		Scene{Name: "updates-available", Desc: "Updates: available, release-page QR", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: rcAvail})
		}},
		Scene{Name: "updates-pakstore", Desc: "Updates: available through the Pak Store", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: storeAvail})
		}},
		Scene{Name: "updates-ahead", Desc: "Updates: rc ahead of Stable, with the Pak Store downgrade warning", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: appupdate.Verdict{Kind: appupdate.UpToDate, Ahead: true,
				Channel: appupdate.Stable, Running: devVer("v1.1.0-rc3"), Latest: devRelease("v1.0.25"), StoreOffers: "v1.0.25"}})
		}},
		Scene{Name: "updates-offline", Desc: "Updates: offline, never checked", Build: func(d SceneDeps) Screen {
			s := devUpdates(d, &devAppUpdater{v: appupdate.Verdict{Channel: appupdate.Stable}})
			netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
			return s
		}},
		Scene{Name: "updates-archive-progress", Desc: "Updates (muOS): Save to ARCHIVE in progress", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveRunning, Done: 5 << 20, Total: 12819887}})
		}},
		Scene{Name: "updates-archive-done", Desc: "Updates (muOS): saved to ARCHIVE", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveDone, Path: "/mnt/mmc/ARCHIVE/" + appupdate.AssetName("v1.1.0-rc3")}})
		}},
		Scene{Name: "updates-archive-failed", Desc: "Updates (muOS): integrity check failed", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Err: appupdate.ErrIntegrity}})
		}},
	)
}
```

- [ ] **Step 2: Render and look**

Run: `go run ./cmd/devshot --list | grep -i update` — expect the 11 scenes.
Run: `mkdir -p /tmp/itchio-screenshots && for s in update-toast update-toast-pakstore settings-updates-selected updates-available updates-ahead updates-archive-progress; do go run ./cmd/devshot --scene $s --out /tmp/itchio-screenshots/$s.png; done` (check `go run ./cmd/devshot --help` for the exact output flag name and use it).
Then Read each PNG. Expected: the notice sits top-right with a visible gap to the top and right edges; the QR sits beside the text at 1024×768; the downgrade warning is in the Warning colour; the progress bar is under the text.
Also render `updates-available` and `update-toast` at 640×480 (the `--size`/geometry flag from `--help`): the notice reads "Update available" on one line; the QR moves below the text.

- [ ] **Step 3: Run the palette audit**

Run: `./scripts/palette-audit.sh`
Expected: exits 0 across all 18 palettes and four geometries. If a new scene reports a low-contrast pair, fix the colour choice in the drawing code (never add a literal), re-run.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/dev_scenes_updates.go
git commit -m "devshot: scenes for the update notice, Updates screen and Settings row"
```

---

### Task 14: Wiring, full suite, cross-compile

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`
- Modify: `CLAUDE.md` (Key Directories)

**Interfaces:**
- Consumes: `appupdate.Parse`, `ResolveChannel`, `NewChecker`, `Config`; `firmware.Env.PakStoreDB()`, `ArchiveDir()`; `ui.SetAppUpdater`, `ui.UpdateNotice`; `itchio.BuildUserAgent`, `UAInfoFromEnv`.

- [ ] **Step 1: Wire the checker**

In `main_sdl.go`, add the import `"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"`.

After `netstate.StartMonitor("/", client.Probe)`:

```go
	// App updates. The channel is resolved before anything reads it; an rc
	// build with no choice yet pins "rc", so installing the final release does
	// not silently drop a tester to Stable (spec §1).
	running, _ := appupdate.Parse(version)
	channel, pin := appupdate.ResolveChannel(cfg.UpdateChannel, running)
	if pin {
		cfg.UpdateChannel = string(channel)
		if err := cfg.Save(cfgPath); err != nil {
			logger.Warn("appupdate: could not pin the rc channel: %v", err)
		}
		logger.Info("appupdate: release-candidate build, update channel pinned to rc")
	}
	appUpd := appupdate.NewChecker(appupdate.Config{
		Firmware:   env.Kind(),
		Running:    version,
		Channel:    channel,
		StatePath:  filepath.Join(dataDir, "update_state.json"),
		StoreDB:    env.PakStoreDB(),
		ArchiveDir: env.ArchiveDir(),
		UserAgent:  itchio.BuildUserAgent(itchio.UAInfoFromEnv(version, env), false),
		Notify: func() {
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
		},
	})
	ui.SetAppUpdater(appUpd)
```

After `netstate.OnReconnect(cache.Resume)`:

```go
	netstate.OnReconnect(appUpd.RetryAfterReconnect)
```

In the `partfile.Sweep` goroutine, before `partfile.Sweep(dirs)`:

```go
		if d := env.ArchiveDir(); d != "" {
			dirs = append(dirs, d)
		}
```

After the `current` screen is chosen (after the `if devScreen … else … current = listScreen` block):

```go
	// After the first screen is up; the notice itself waits for a screen
	// that allows it (ui.noticeAllowed).
	appUpd.Start()
	var notice ui.UpdateNotice
```

In the loop, change the first wait branch:

```go
		if current.NeedsRedraw() || notice.Animating() {
			e = sdl.WaitEventTimeout(16)
```

and the final draw branch:

```go
		} else if noticeFrame := notice.Tick(r, current, time.Now()); gotEvent || newImages || current.NeedsRedraw() || noticeFrame {
			current.Draw(r)
		}
```

(Add `"time"` to the imports if missing.)

- [ ] **Step 2: Build natively and cross-compile**

Run: `go vet ./... && go build ./... && ./scripts/build.sh nextui/tg5040`
Expected: no vet findings; the tg5040 build succeeds and `build.sh` reports the GLIBC ceiling check passing.

- [ ] **Step 3: Document**

In `CLAUDE.md`, under Key Directories, after the `internal/itchio/` line:

```
internal/appupdate/ App-update checks (GitHub), channels, verdict, muOS Save to ARCHIVE
internal/pakstore/ Read-only reader for the Pak Store's SQLite install database
```

- [ ] **Step 4: Run the full suite**

Run: `./scripts/test.sh`
Expected: every stage passes (shell tests, no-color-literals, palette audit, `go test -race -tags headless ./...`, non-headless `internal/ui` and `internal/renderer`). Paste the tail of the output in the task report.

- [ ] **Step 5: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go CLAUDE.md
git commit -m "main: start the app-update checker and show its notice"
```

- [ ] **Step 6: Hardware check (with the user)**

Use the `itchio-pak-build` flow: `./scripts/release.sh`, then `./scripts/deploy.sh` (Brick) / `./scripts/deploy.sh muos` (Smart Pro), then verify the md5 on the device. Check `adb devices` first — only the Brick was attached on 2026-09-28.

- Brick (Store row `v1.0.23`, rc build): log shows `pakstore: … → managed(v1.0.23)`, `appupdate: check start … releases=true pakjson=true`, and the Updates screen shows the Pak Store downgrade warning. Then, with a copy of the database with the Itch-io row deleted in place, the verdict becomes release-page.
- Smart Pro (muOS): Save to ARCHIVE, then install through Archive Manager and confirm `data/` survives. Settles the spec's open point: note whether Archive Manager also lists an `ARCHIVE/` on SD2.
- Both: the notice appears top-right, clear of the edges, once; a relaunch does not show it again.
