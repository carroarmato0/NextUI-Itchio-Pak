# App updates: notice, channels and staging — design

Date: 2026-09-26 · Target: v1.1.0-rc2 (branch `feature/1.1-app-updates`, from `release/1.1.0`)

Depends on `2026-09-26-connectivity-design.md` (`internal/netstate`), which lands first.

## Goal

Tell users when a newer Itch-io exists, without nagging them and without going
around the Pak Store, which stays the main way to install and update on NextUI
(it also gives the project its install statistics). muOS has no store, so there
the app also offers to download the update into `ARCHIVE/` for Archive Manager.
Testers can opt into release candidates.

In scope for rc2:

1. An update check against GitHub, with two channels (Stable, Release
   candidates) and an Off setting.
2. A small animated notice in the top-left corner, shown once per new version
   per channel.
3. A Settings → Updates screen: channel, check now, status, release-page QR.
4. On NextUI, detecting whether the Pak Store manages this install and, if so,
   pointing the user to the Store instead of offering anything itself.
5. On muOS, "Save to ARCHIVE": download the `.muxapp`, verify it, leave it for
   Archive Manager.
6. Settings list wraps around: Up on the first row goes to the last, Down on
   the last goes to the first.

Out of scope (rc3, see "Follow-up"): installing an update in place.

The About screen does not change. Its QR code keeps pointing at the project
page; the release-page QR code lives on the Updates screen.

## Research this design rests on

Verified 2026-09-26 against source and both attached devices.

- **GitHub releases.** `GET /repos/carroarmato0/NextUI-Itchio-Pak/releases?per_page=10`
  returns tags with `prerelease` and `draft`, and every asset carries a
  `digest` of the form `sha256:<hex>` (checked on v1.0.25-rc2, v1.0.25,
  v1.1.0-rc1). Unauthenticated limit: 60 requests/hour per IP.
- **Pak Store** (`LoveRetro/nextui-pak-store`):
  - Keeps installs in SQLite at
    `/mnt/SDCARD/.userdata/<PLATFORM>/nextui-pak-store/pak-store.db`, table
    `installed_paks(name, display_name, pak_id, repo_url, type, version,
    can_uninstall)`. Itch-io's storefront id is `3JM2zY4UKh`.
  - Reads a pak's latest version from **`pak.json` on the `main` branch**
    (`raw.githubusercontent.com/<repo>/refs/heads/main/pak.json`), not from
    GitHub releases. It therefore never offers a release candidate.
  - Offers an update when `compareVersions(installed, latest) == -1`, where
    `compareVersions` strips `v`, splits on `.`, and `Atoi`s each part —
    so `"0-rc1"` parses as 0 and **`v1.1.0-rc1` equals `v1.1.0`**.
  - Only writes a row when the Store itself installs, or when "discover
    existing installs" (on by default) finds a pak with no row. The row is not
    refreshed from disk afterwards. On the Brick the row says `v1.0.23` while
    `v1.1.0-rc1` is on disk (deployed by hand).
  - Installs by `RemoveAll(pakDir)` then unzip.
- **muOS Archive Manager** (`MustardOS/internal` `extract.sh`, `zip.sh`):
  extracts a `.muxapp` with `unzip -o` into `$MUOS_STORE_DIR/application`,
  never deleting the existing app folder, so `data/` survives. `SAFE_ARCHIVE`
  rejects absolute paths, `..`, backslashes, links and oversized entries. On
  the Smart Pro, `ARCHIVE` is `/mnt/mmc/ARCHIVE` (it still holds a v1.0.23-rc1
  `.muxapp`).
- **Filesystems** (both devices, kernel 4.9.191): NextUI SD is vfat mounted
  `sync`; muOS SD1 is exFAT. On both, `rename()` over a running executable
  works and the running process is unaffected; directory renames work.
- **Scrappy** (muOS) updates itself with a Python script that disables TLS
  verification, has no checksum, and extracts over the live app with no
  rollback. A user reported a blank screen after an in-app update. It is the
  pattern to avoid.

## Design

### 1. Package `internal/appupdate` (headless-safe)

No SDL, fully unit-tested. Parts:

**Versions (`version.go`).** Parses `vMAJOR.MINOR.PATCH[-rcN]`. Ordering is
`1.1.0-rc1 < 1.1.0-rc2 < 1.1.0 < 1.1.1-rc1`. Anything that does not parse
(including `dev`) is "unknown": no check runs for it. Also contains
`StoreCompare(a, b)`, a line-for-line port of the Pak Store's
`compareVersions`, used only to predict what the Store will show, and tested
against the Store's own behaviour (`v1.1.0-rc1` == `v1.1.0`).

**Channel (`channel.go`).** `Stable`, `RC`, `Off`. Stored in `config.json` as
`update_channel` (`"stable" | "rc" | "off"`). Empty means not chosen yet and
resolves to `rc` when the running build is a release candidate, `stable`
otherwise — so an rc tester keeps hearing about the next rc without touching
Settings, and a stable user never sees one.

- Stable considers releases with `prerelease=false`.
- RC considers every non-draft release, and takes the highest version, so an
  RC tester is also told about the final release.
- Off makes no request.

**Sources (`source.go`).** One request per launch, at most:

| Install | Channel | Request |
|---|---|---|
| NextUI, Pak Store manages it | Stable | `pak.json` on `main` (the Store's own source) |
| everything else | Stable or RC | GitHub releases list |

Both use the bundled CA file with normal verification, a 10 s timeout, and
the short User-Agent (`NextUI-Itchio-Pak/<ver> (+<url>)`). The device details
behind "Share device info" are for itch.io and are not sent to GitHub.
Conditional requests with the stored `ETag`. A 403/429 with rate-limit headers
records a "not before" time and skips checks until then. Failures are logged
and leave the last known result in place; they never show UI.

**Pak Store detection (`internal/pakstore`).** A read-only reader for the
SQLite file format, just enough for this: read the header, find
`installed_paks` in `sqlite_master`, walk its table b-tree (interior and leaf
pages), decode records. About 150 lines; tested against databases built with
`sqlite3` and committed under `testdata/pakstore/` (single page, multi-page
with interior nodes, empty table, table missing, truncated file).

It returns one of:

- `NotInstalled` — no Store database for this platform, or no row for
  Itch-io.
- `Managed(version)` — a row matching `pak_id = "3JM2zY4UKh"`, else
  `name = "Itch-io"`.
- `Unknown` — the file exists but cannot be read with confidence: bad header,
  overflow pages, a non-empty `-wal` file beside it, or a parse error.
  **Treated as Managed.** Guessing wrong in that direction only means we
  point the user at the Store; guessing wrong in the other would go around it.

Linking a SQLite library was rejected: several MB of binary to read one
12 KB file.

**Decision (`decide.go`).** A pure function from (firmware, running version,
channel, store status, latest release) to a `Verdict`:

- `UpToDate`
- `Available{Version, Via}` where `Via` is:
  - `ViaPakStore` — NextUI, Store-managed, and
    `StoreCompare(storeRow, latest) == -1`. The Store will show it.
  - `ViaArchive` — muOS, and the release has the `.muxapp` asset with a
    digest.
  - `ViaReleasePage` — anything else: NextUI not managed by the Store, an RC
    the Store will never offer, or the Store-row trap (row `v1.1.0-rc1`,
    latest `v1.1.0`, which the Store considers equal). rc2 shows the version
    and the release-page QR code; rc3 turns this into an in-place install.

"Available" requires `latest > running` by the real ordering, whatever the
Store row says: a side-loaded rc build must not be told to "update" to an
older stable the Store happens to consider newer than its stale row.

**State (`state.go`).** `update_state.json` in the data directory, separate
from `config.json` because it is not a user choice:

```json
{
  "checked_at": "2026-09-26T14:00:00Z",
  "not_before": "0001-01-01T00:00:00Z",
  "etag": { "releases": "W/\"…\"", "pakjson": "W/\"…\"" },
  "latest": { "stable": { "tag": "v1.0.25", "url": "…", "asset": "…", "digest": "sha256:…", "size": 12678524 },
              "rc":     { "tag": "v1.1.0-rc2", … } },
  "notified": { "stable": "v1.0.25", "rc": "v1.1.0-rc2" }
}
```

Written atomically (tmp + rename, like `settings.Save`). A missing or corrupt
file is treated as empty.

### 2. Notification rule

Show the notice when the verdict is `Available` **and** the version is higher
than `notified[channel]`. Record `notified[channel]` when the notice has
finished its animation, not when the check finds the version, so a crash or
quick exit does not swallow it.

Keeping one value per channel is what makes switching work. Examples:

- RC tester sees rc2 (`notified.rc = rc2`), switches to Stable without
  installing it. Stable's own value is still older, so v1.0.26 is announced.
- Back on RC: rc2 is not announced again.
- An update installed through any route raises the running version; the rule
  then compares against that, so nothing is announced for a version already
  installed.

Changing the channel in Settings uses the cached `latest` for that channel if
present and triggers a check otherwise. It never shows the notice while the
user is on the Updates screen (the screen already says it).

### 3. The notice (UI)

`internal/ui/update_toast.go`, drawn over whatever screen is current.

- **Hook.** `Renderer` gains an optional `Overlay func(*Renderer)` called
  inside `Present()` just before the swap. Every screen already ends its
  `Draw` with `Present()`, so no screen changes.
- **Wake-ups.** When a check completes, the goroutine pushes the SDL user event
  the image cache already uses to wake the loop. While the toast animates, the
  main loop treats it like `NeedsRedraw()` (16 ms timeout, redraw every
  frame); afterwards the loop is idle again.
- **Motion.** Slides down from above the top-left corner over 250 ms (ease
  out), holds 4 s, slides back up over 250 ms. No input is consumed; buttons
  keep working on the screen underneath.
- **Text by width** (`abbreviate(w)`):
  - wide: `Itch-io v1.1.0 available`, with a second smaller line —
    `Settings → Updates`, or `Update it in the Pak Store` for `ViaPakStore`;
  - narrow: `Update available` on one line.
- **Look.** A pill in theme colours (Accent background, AccentText), margins
  from `LayoutFor`/`compact()`. No colour literals.
- **When.** Only on the first launch where the rule fires. It waits until the
  app has left the loading screen, so it does not appear on top of the startup
  progress, the sign-in QR screen or the first-run account prompt.

### 4. Settings → Updates screen

New row `Updates >` just above `About`, with a right-aligned annotation in the
style of "Update Inventory": `v1.1.0 available` (Accent-derived, readable on
the selected pill the way the Account row's "not signed in" is), otherwise
nothing.

`internal/ui/screen_updates.go`:

| Row | A does |
|---|---|
| `Channel: Stable` / `Release candidates` / `Off` | cycle |
| `Check now` — annotation `last: 5m ago` / `checking…` / `failed` / `never` | check |
| `Save to ARCHIVE` — muOS and `ViaArchive` only | download (below) |

Below the rows, a status block:

- `You have the latest version (v1.1.0-rc2)`
- `v1.1.0 is available. Update it in the Pak Store.`
- `v1.1.0 is available.` with the release-page QR code and
  `Scan for the release notes`
- after a save: `Saved to ARCHIVE. Open Applications → Archive Manager to install.`

The QR code is laid out like the About screen's (size clamped, placed beside
the text on wide screens and below it where `abbreviate(w)`), and it points at
the release's `html_url`.

### 5. muOS: Save to ARCHIVE

- Destination: `ARCHIVE/` at the root of the card muOS calls SD1
  (`firmware.Env` gains `ArchiveDir()`; `""` on NextUI and host).
- Download `Itch-io.muOS.<tag>.muxapp` to `ARCHIVE/.Itch-io.muOS.<tag>.muxapp.itchio-part`
  with the same partial-file/idle-timeout/length rules as game downloads
  (connectivity spec §4, including its journal and startup cleanup), checking free space first (asset size + 10 %). Progress bar and B to cancel,
  reusing the download screen's drawing.
- Verify size and the SHA-256 against the asset `digest`. On mismatch, delete
  the partial file, log both hashes, show `Download failed the integrity check`.
  A release without a digest is not downloadable.
- Check the zip before accepting it: every entry under `Itch-io/`, and the
  `SAFE_ARCHIVE` rules (no absolute paths, `..`, `\`, links; entry count and
  size limits), so Archive Manager will not reject it.
- Rename to the final name. Then remove older `Itch-io.muOS.v*.muxapp` files
  from the same `ARCHIVE/` so the user cannot install the wrong one. Only files
  matching our release naming are touched.
- The rest is Archive Manager's job. The app does not exit or relaunch.

### 6. Settings wrap-around

`moveCursor(dir, wrap bool)`. A single press (`startHold`) wraps: Up on the
first visible row goes to the last visible row, Down on the last goes to the
first. Auto-repeat (`processAutoRepeat`) passes `wrap=false` and stops at the
ends, so holding a direction never loops. "Visible" uses `rowHidden`, so the
wrap lands on a rendered row even when the last or first rows are hidden by
firmware. The scroll offset follows the cursor as it does now, so jumping to
the last row scrolls to the bottom. Same behaviour on the new Updates screen.

### 7. Wiring

- `main_sdl.go` starts the check in a goroutine after the first screen is up,
  when: firmware is NextUI or muOS, the running version parses, the channel is
  not Off, `not_before` has passed, and `netstate` is not Offline. A check
  skipped for being offline runs when `netstate` reports the connection back.
- The checker's HTTP client uses the `netstate` transport wrapper, so its
  failures and successes feed the shared state.
- `SettingsScreen` receives an `UpdateChecker` interface (like
  `UpdateServicer`) exposing `Verdict()`, `CheckNow()`, `IsRunning()`,
  `CheckedAt()`, `SetChannel()` and the ARCHIVE download.

### 8. Logging

- `appupdate: check start channel=… source=releases|pakjson store=…`
- the HTTP status, bytes, ETag hit and duration;
- the verdict with running, latest and store versions;
- `notified channel=… version=…`;
- download start/progress at debug/verify ok|mismatch/rename/cleanup of older
  files;
- rate-limit back-off until …;
- pakstore: path, page size, result, and the reason for `Unknown`.

### 9. Error handling summary

| Failure | Behaviour |
|---|---|
| Offline (per `netstate`) | skip; run on reconnect; Updates screen shows the `netstate` message |
| 5xx | log, keep last result, no UI |
| 403/429 rate limit | set `not_before`, skip until then |
| Store DB unreadable | treat as Store-managed |
| No suitable asset / no digest | notice and QR only, no download |
| Not enough space | message on the Updates screen, nothing written |
| Digest or zip check fails | partial file deleted, message, logged |
| Cancelled download | partial file deleted |

## Testing

- `appupdate`: version ordering; `StoreCompare` parity cases; default channel
  from running version; decision table covering every row of the `Via` rules,
  including the Store-row trap and the side-loaded-rc case; notification rule
  across channel switches; state load/save with a corrupt file.
- Sources against `httptest.NewServer` with fixtures in `testdata/appupdate/`
  (releases list with drafts, prereleases, a missing asset, a missing digest;
  `pak.json`; 304 with ETag; 403 with rate-limit headers).
- `pakstore` against committed `sqlite3`-built fixtures, including a
  multi-page table and the cases that must return `Unknown`.
- ARCHIVE download: digest mismatch leaves nothing, cancel leaves nothing,
  older release files are removed and other files are not, unsafe zip
  rejected.
- `internal/ui/input_test.go`: wrap on single press, no wrap on repeat, hidden
  first/last rows.
- `devshot` scenes: `update-toast` (wide and narrow), `updates` in each status,
  muOS save in progress and done. Included in `scripts/palette-audit.sh` at all
  four geometries.
- On hardware: Brick (Store row present → Pak Store verdict; delete-row copy →
  release-page verdict) and Smart Pro (Save to ARCHIVE, then install via
  Archive Manager and confirm `data/` survives).

## Open points to confirm on hardware

- Whether Archive Manager also scans an `ARCHIVE/` on SD2, and where the app
  should save when it runs from SD2.

## Follow-up: in-place install (rc3)

Recorded here so the research is not lost; it gets its own spec.

Stage → verify → swap at next launch → confirm → roll back:

1. Download the firmware's asset over verified TLS, check the digest and free
   space.
2. Extract into a hidden staging folder on the same card as the install;
   validate paths, the version inside, and that the binary is aarch64 ELF.
3. The launcher (`launch.sh` / `mux_launch.sh`), before starting the binary,
   renames the live folder to `.prev` and the staged one into place (on muOS
   moving `data/` across first), then execs the new launcher. "Restart now" is
   the app exiting with a code that asks the launcher to do this immediately.
4. The new binary writes a started-OK marker after its first frame and input
   poll; a pending update without the marker on the next launch restores
   `.prev`. `.prev` is deleted after one good start.

Applies where the decision is `ViaReleasePage` on NextUI and as an
"Install now" alternative to Save to ARCHIVE on muOS. Never on a
Store-managed NextUI install whose update the Store will offer, and the
Store database is never written. Residual risk: power loss in the
milliseconds between the two renames leaves no app folder (the Pak Store's
own install leaves none for seconds). To verify first: whether NextUI hides
dot-prefixed folders in `Tools/<platform>/`.
