# App updates: notice, channels and staging — design

Date: 2026-09-26, refreshed 2026-09-27 · Target: **v1.1.0-rc3** (branch
`feature/1.1-app-updates`, brought up to date with `release/1.1.0` at
`v1.1.0-rc2`)

Builds on `2026-09-26-connectivity-design.md`, which has landed: it shipped in
v1.1.0-rc2 together with the H700 rc11 button fix. The names this design uses
from it are now real code, listed under "Building blocks" below.

**Changes in the 2026-09-27 refresh:**

- Target moved from rc2 to rc3. rc2 shipped with connectivity and the H700 fix
  instead, and in-place install moves from rc3 to rc4.
- Settings wrap-around is done (commit `633e0cf`, in rc2) and dropped from
  scope. The Updates screen uses the same `moveCursor(dir, wrap)`.
- Abstract references to connectivity replaced with the real API; see
  "Building blocks".
- The ARCHIVE download gets its own small streaming loop on top of
  `partfile`, not a refactor of the itch.io download path.
- The "Updates" row annotation uses the selected-row colour rule Settings has
  had since the rc2 readability fix.
- The app-update check is the fourth reconnect callback, after the game list,
  inventory and covers.
- `partfile.Sweep` also covers `ARCHIVE/`.
- Research re-checked against rc2's live release: its assets carry digests,
  and `main`'s `pak.json` still says `v1.0.25`.

**Changes in the 2026-09-28 review:**

- The notice moves to the **top-right** corner, inset from the top and right
  edges by a margin (§3).
- New §2a "Switching channels": what happens when you switch Stable → RC and
  RC → Stable, including the case where the Pak Store offers a downgrade. It
  changes §1 (one releases request fills both channels, `per_page=30`, the
  default channel is pinned once you run an rc), §2 (seeing a version on the
  Updates screen counts as being told), §4 (status wording when you are ahead
  of the channel) and §5 (clean-up removes every other Itch-io `.muxapp`, not
  only older ones).

## Goal

Tell users when a newer Itch-io exists, without nagging them and without going
around the Pak Store, which stays the main way to install and update on NextUI
(it also gives the project its install statistics). muOS has no store, so there
the app also offers to download the update into `ARCHIVE/` for Archive Manager.
Testers can opt into release candidates.

In scope for rc3:

1. An update check against GitHub, with two channels (Stable, Release
   candidates) and an Off setting.
2. A small animated notice in the top-left corner, shown once per new version
   per channel.
3. A Settings → Updates screen: channel, check now, status, release-page QR.
4. On NextUI, detecting whether the Pak Store manages this install and, if so,
   pointing the user to the Store instead of offering anything itself.
5. On muOS, "Save to ARCHIVE": download the `.muxapp`, verify it, leave it for
   Archive Manager.

Out of scope: installing an update in place (rc4, see "Follow-up").

The About screen does not change. Its QR code keeps pointing at the project
page; the release-page QR code lives on the Updates screen.

## Research this design rests on

Verified 2026-09-26 against source and both attached devices, and re-checked
2026-09-27.

- **GitHub releases.** `GET /repos/carroarmato0/NextUI-Itchio-Pak/releases?per_page=10`
  returns tags with `prerelease` and `draft`, and every asset carries a
  `digest` of the form `sha256:<hex>`. This was checked on v1.0.25-rc1,
  v1.0.25-rc2, v1.0.25, v1.1.0-rc1 and v1.1.0-rc2. The current state: the
  newest release is `v1.1.0-rc2` (`prerelease: true`), with
  `Itch-io.muOS.v1.1.0-rc2.muxapp` at 12,819,887 bytes; the newest stable
  release is `v1.0.25`, the only one to also carry the bare `Itch-io.pak.zip`.
  The unauthenticated rate limit is 60 requests per hour per IP.
- **`pak.json` on `main`** says `v1.0.25`. Release candidates bump `pak.json`
  on `release/1.1.0` only, which is why the Store never sees them.
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

## Building blocks already in `release/1.1.0`

These exist at v1.1.0-rc2. They are the interfaces this design is written
against, not proposals.

| Need | Use | Notes |
|---|---|---|
| Feed the shared online/offline state | `netstate.Transport(next http.RoundTripper)` | Wrap the checker's HTTP client transport with it. A cancelled request never reads as offline. |
| Skip while offline | `netstate.Offline()` | |
| Run again when the connection is back | `netstate.OnReconnect(fn)` | Registrations are permanent, and callbacks run on the netstate goroutine: raise a flag and wake the UI, never touch UI state. Registered in `main_sdl.go` after the game list, inventory and covers. |
| Words for a failure | `netstate.Describe(err, "GitHub")` | Returns a `Message{Title, Hint}`, or ok=false for errors that are not connection problems. |
| URL-free log detail | `netstate.Detail(err)` | Signed GitHub asset redirects carry tokens in the URL. |
| HTTP 5xx | `&netstate.StatusError{What, Code}` | `Describe` maps it to "… is having problems". |
| Partial files | `partfile.Create(dest)` → `(*File).Commit()` / `Abort()` | Writes go to `.<name>.itchio-part` beside `dest` and are journalled. Commit fsyncs, then renames. |
| Leftovers after a crash | `partfile.Recover()` at startup, `partfile.Sweep(dirs)` | `ARCHIVE/` must be added to the directories passed to `Sweep` in `main_sdl.go`. |
| Short User-Agent | `itchio.BuildUserAgent(info, false)` | The opt-out form: `NextUI-Itchio-Pak/<ver> (+<url>)`, with no device details. |
| List wrap-around | `SettingsScreen.moveCursor(dir, wrap)` | A single press wraps; auto-repeat stops at the ends. |
| Selected-row annotation colour | The `isSelected && warn` switch in `screen_settings.go` | Readable on the Accent pill for every palette, and pinned by `palette-audit.sh` scenes. |
| Wake the main loop | `sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})` | The code the image cache and netstate already use. |

The itch.io download path (`(*itchio.Client).streamToFile`) is deliberately
not reused. It is bound to the itch.io client, its rate limiter and its
User-Agent, and it was reviewed as part of connectivity. The ARCHIVE download
(§5) has its own small loop on top of `partfile` with the same rules: 30 s
idle timeout, length check, and fsync by `Commit`.

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

The first time an empty channel resolves to `rc`, `"rc"` is written to
`config.json`. Without that, a tester who installs the final release
(`v1.1.0-rc4` → `v1.1.0`) would drop silently to Stable and never hear
about `v1.1.1-rc1`. `"stable"` is never written automatically: a stable user
who side-loads an rc has shown they want rcs, and gets them the same way.

- Stable considers releases with `prerelease=false`.
- RC considers every non-draft release, and takes the highest version, so an
  RC tester is also told about the final release.
- Off makes no request.

**Sources (`source.go`).** One request per launch, at most (the single exception is in §2a):

| Install | Channel | Request |
|---|---|---|
| NextUI, Pak Store manages it | Stable | `pak.json` on `main` (the Store's own source) |
| everything else | Stable or RC | GitHub releases list |

One releases response lists both prereleases and stable releases, so every
releases check fills **both** `latest.stable` and `latest.rc`, whatever the
channel. Switching channels then costs no request, except switching to
Stable on a Store-managed install, whose source is `pak.json`. The list is
fetched with `per_page=30`, so a run of release candidates cannot push the
newest stable off the page. If the page still contains no stable release,
the cached `latest.stable` is kept rather than cleared.

Both use the bundled CA file with normal verification, a 10 s timeout, the
short User-Agent (`itchio.BuildUserAgent(info, false)`), and a transport
wrapped in `netstate.Transport`. The device details behind "Share device info"
are for itch.io and are not sent to GitHub. A 5xx is returned as a
`netstate.StatusError`.
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
    the Store will never offer, or the Store-row trap (row `v1.1.0-rc2`,
    latest `v1.1.0`, which the Store considers equal). rc3 shows the version
    and the release-page QR code; rc4 turns this into an in-place install.

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
              "rc":     { "tag": "v1.1.0-rc3", … } },
  "notified": { "stable": "v1.0.25", "rc": "v1.1.0-rc3" }
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

- RC tester sees rc4 (`notified.rc = rc4`), switches to Stable without
  installing it. Stable's own value is still older, so v1.1.0 is announced.
- Back on RC: rc4 is not announced again.
- An update installed through any route raises the running version; the rule
  then compares against that, so nothing is announced for a version already
  installed.

Changing the channel in Settings uses the cached `latest` for that channel if
present and triggers a check otherwise. It never shows the notice while the
user is on the Updates screen (the screen already says it). When the Updates
screen shows an `Available` verdict, that counts as being told:
`notified[channel]` is recorded then, so the next launch does not announce
a version the user has already seen there.

### 2a. Switching channels

Both directions were worked through against the Store's behaviour and the
real version ordering. **The app never offers a downgrade**, on either
channel and through any route (notice, Updates screen, ARCHIVE, and rc4's
in-place install). Going back is not safe: 1.1 replaces the API key with
OAuth and moves settings forward, and 1.0.x does not know how to read them,
so a downgrade would lose sign-in at the very least.

**Stable → RC.**

- Takes effect straight away from the cached `latest.rc` (§1); no request.
- The newest rc is announced if it is newer than the running version and
  than `notified.rc`.
- On NextUI the verdict is always `ViaReleasePage`: the Store never offers an
  rc. On muOS it is `ViaArchive`.
- The Store row does not change when an rc is installed by hand. That is the
  source of the hazard below.

**RC → Stable.**

- If the running rc is newer than the latest stable (`v1.1.0-rc3` against
  `v1.0.25`), the verdict is `UpToDate`, and nothing is offered. The status
  line says so plainly instead of "You have the latest version":
  `You are on v1.1.0-rc3, a release candidate newer than the latest stable
  release (v1.0.25). You will be told when a newer stable release is out.`
- The final release of the same line is newer than all its rcs
  (`v1.1.0 > v1.1.0-rc4`), so it is announced when it ships. From then on the
  user is back on the stable line.
- `notified.stable` is separate from `notified.rc`, so the switch does not
  bring back an old stable announcement.

**The Pak Store downgrade trap (NextUI, Store-managed, running an rc).** This
happens whatever channel is selected. The Store compares its stale row with
`pak.json` on `main`, not with what is on disk. On the Brick the row says
`v1.0.23` and the disk holds an rc. The Store therefore offers "v1.0.25" as
an update, and installing it would downgrade the app. The Store database is
never written, so the app cannot stop this. It can warn:

- Condition: Store-managed, and `StoreCompare(row, pakjson) == -1`, and
  `pakjson < running` by the real ordering.
- Status on the Updates screen: `The Pak Store may offer v1.0.25 as an
  update. It is older than this release candidate; installing it would
  replace it.`
- This is shown on the Updates screen only, never as a notice. It needs
  `pak.json`, so while running an rc on a Store-managed install, the check
  also fetches `pak.json`. That is the one case with two requests per launch.

**Off.** No request is made and the cached results are left alone. Switching
back from Off uses them straight away and checks if they are missing.

### 3. The notice (UI)

`internal/ui/update_toast.go`, drawn over whatever screen is current.

- **Hook.** `Renderer` gains an optional `Overlay func(*Renderer)` called
  inside `Present()` just before the swap. Every screen already ends its
  `Draw` with `Present()`, so no screen changes.
- **Wake-ups.** When a check completes, the goroutine pushes the SDL user event
  the image cache already uses to wake the loop. While the toast animates, the
  main loop treats it like `NeedsRedraw()` (16 ms timeout, redraw every
  frame); afterwards the loop is idle again.
- **Position.** Top-right, right-aligned. It sits in from the right edge and
  down from the top edge by the same margin: `toastMargin(w, h)`, 12 px, or
  6 px when `compact(w, h)`. It never touches the screen edge.
- **Motion.** Slides down from above the top edge to its resting place over
  250 ms (ease out), holds 4 s, and slides back up over 250 ms. No input is
  consumed; buttons keep working on the screen underneath.
- **Overlap.** On the game list, the top-right corner holds the sort,
  platform and Offline pills (`screen_list.go`). The notice covers them while
  it is showing. That is acceptable for 4.5 s, but the notice must not read as
  another header pill. It uses the Accent fill (the header pills use TitlePill
  and Chip), with a 1 px `ModalBorder()` outline so it separates from them
  on every palette. The `update-toast` devshot scene is rendered over the
  list screen so `palette-audit.sh` checks it against those pills.
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
style of "Update Inventory": `v1.1.0 available`, otherwise nothing. Its colour
follows the same selected/unselected switch as the Update Inventory
annotation, so it stays readable on the Accent pill. A
`settings-updates-selected` devshot scene puts it under `palette-audit.sh`, as
the rc2 fix did for Update Inventory.

`internal/ui/screen_updates.go`:

| Row | A does |
|---|---|
| `Channel: Stable` / `Release candidates` / `Off` | cycle |
| `Check now` — annotation `last: 5m ago` / `checking…` / `failed` / `never` | check |
| `Save to ARCHIVE` — muOS and `ViaArchive` only | download (below) |

Below the rows, a status block:

- `You have the latest version (v1.1.0-rc3)`
- `v1.1.0 is available. Update it in the Pak Store.`
- `v1.1.0 is available.` with the release-page QR code and
  `Scan for the release notes`
- after a save: `Saved to ARCHIVE. Open Applications → Archive Manager to install.`
- ahead of the channel (RC → Stable, §2a): `You are on v1.1.0-rc3, a release
  candidate newer than the latest stable release (v1.0.25). …`
- the Store downgrade warning (§2a), under whichever of the above applies

The QR code is laid out like the About screen's (size clamped, placed beside
the text on wide screens and below it where `abbreviate(w)`), and it points at
the release's `html_url`. The rows wrap with `moveCursor(dir, wrap)`, as
Settings does.

### 5. muOS: Save to ARCHIVE

- Destination: `ARCHIVE/` at the root of the card muOS calls SD1
  (`firmware.Env` gains `ArchiveDir()`; `""` on NextUI and host).
- Download `Itch-io.muOS.<tag>.muxapp` through
  `partfile.Create(ARCHIVE/Itch-io.muOS.<tag>.muxapp)`, which writes
  `ARCHIVE/.Itch-io.muOS.<tag>.muxapp.itchio-part` and journals it. A small
  loop in `appupdate` applies the game-download rules: a 30 s idle timeout
  that also covers the wait for headers, a length check against the asset
  size, `Abort()` on any failure or cancel, and `Commit()` only after
  verification. Check free space first (asset size + 10 %). Progress bar and
  B to cancel, reusing the download screen's progress drawing. Add
  `env.ArchiveDir()` to the directories `main_sdl.go` passes to
  `partfile.Sweep`, so a leftover from a crash is cleaned up at the next
  launch.
- Verify size and the SHA-256 against the asset `digest` before `Commit()`.
  On mismatch, `Abort()`, log both hashes, and show `Download failed the
  integrity check`.
  A release without a digest is not downloadable.
- Check the zip before accepting it: every entry under `Itch-io/`, and the
  `SAFE_ARCHIVE` rules (no absolute paths, `..`, `\`, links; entry count and
  size limits), so Archive Manager will not reject it.
- Rename to the final name. Then remove every **other**
  `Itch-io.muOS.v*.muxapp` from the same `ARCHIVE/`, so it holds exactly the
  one the user just saved. Removing only the older ones is not enough once
  channels switch: save `v1.2.0-rc1` on RC, switch to Stable, save `v1.1.1`,
  and the rc is the newer file, yet not the one the user chose. Only files
  matching our release naming are touched, and each removal is logged.
- The rest is Archive Manager's job. The app does not exit or relaunch.

### 6. Wiring

- `main_sdl.go` starts the check in a goroutine after the first screen is up,
  when: firmware is NextUI or muOS, the running version parses, the channel is
  not Off, `not_before` has passed, and `netstate.Offline()` is false. A check
  skipped for being offline is owed. The checker exposes
  `RetryAfterReconnect()`, registered with `netstate.OnReconnect` after the
  game list, inventory and covers. If a check is owed, it starts it on its own
  goroutine; otherwise it does nothing.
- The checker's HTTP client transport is wrapped in `netstate.Transport`, so
  its failures and successes feed the shared state.
- `SettingsScreen` receives an `UpdateChecker` interface (like
  `UpdateServicer`) exposing `Verdict()`, `CheckNow()`, `IsRunning()`,
  `CheckedAt()`, `SetChannel()` and the ARCHIVE download.

### 7. Logging

- `appupdate: check start channel=… source=releases|pakjson store=…`
- the HTTP status, bytes, ETag hit and duration;
- the verdict with running, latest and store versions;
- `notified channel=… version=…`;
- download start/progress at debug/verify ok|mismatch/rename/cleanup of older
  files;
- rate-limit back-off until …;
- pakstore: path, page size, result, and the reason for `Unknown`.

### 8. Error handling summary

| Failure | Behaviour |
|---|---|
| Offline (`netstate.Offline()`) | skip and mark owed; run on reconnect; the Updates screen shows `netstate.Describe(err, "GitHub")` |
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
- Channel switching (§2a), as a table test: Stable → RC uses the cached
  `latest.rc` with no request; RC → Stable while ahead gives `UpToDate` with
  the ahead-of-channel status; the final release is announced after RC →
  Stable; no verdict is ever lower than the running version; an empty channel
  pins `"rc"` when running an rc and never pins `"stable"`; the Store
  downgrade warning fires exactly when its three conditions hold; one releases
  response fills both channels; a page with no stable release keeps the
  cached one; seeing a version on the Updates screen suppresses its notice.
- Sources against `httptest.NewServer` with fixtures in `testdata/appupdate/`
  (releases list with drafts, prereleases, a missing asset, a missing digest;
  `pak.json`; 304 with ETag; 403 with rate-limit headers).
- `pakstore` against committed `sqlite3`-built fixtures, including a
  multi-page table and the cases that must return `Unknown`.
- ARCHIVE download: digest mismatch leaves nothing, cancel leaves nothing,
  every other Itch-io release file is removed (including a newer one) and
  unrelated files are not, unsafe zip rejected.
- Updates screen: wrap on a single press and no wrap on repeat, like the
  existing `TestMoveCursor_pressWrapsAtEnds` and
  `TestMoveCursor_repeatStopsAtEnds` for Settings.
- Offline: a check skipped while `netstate` is Offline runs once on
  `RetryAfterReconnect`, and not at all when nothing is owed.
- ARCHIVE: a crash leftover (`.Itch-io.muOS.<tag>.muxapp.itchio-part`) is
  removed by `partfile.Sweep` when `ArchiveDir()` is passed in.
- `devshot` scenes: `update-toast` (wide and narrow, top-right over the list
  header pills), `updates` in each status (including ahead-of-channel and the
  Store downgrade warning),
  `settings-updates-selected` (the annotation on the Accent pill), and muOS
  save in progress and done. Included in `scripts/palette-audit.sh` at all
  four geometries.
- On hardware: Brick (Store row present → Pak Store verdict; delete-row copy →
  release-page verdict) and Smart Pro (Save to ARCHIVE, then install via
  Archive Manager and confirm `data/` survives).

## Open points to confirm on hardware

- Whether Archive Manager also scans an `ARCHIVE/` on SD2, and where the app
  should save when it runs from SD2.

## Follow-up: in-place install (rc4)

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
Store database is never written. Like everything else here, it never
installs a version lower than the running one (§2a). Residual risk: power loss in the
milliseconds between the two renames leaves no app folder (the Pak Store's
own install leaves none for seconds). To verify first: whether NextUI hides
dot-prefixed folders in `Tools/<platform>/`.
