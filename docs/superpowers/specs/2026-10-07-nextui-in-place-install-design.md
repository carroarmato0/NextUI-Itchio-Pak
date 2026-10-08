# NextUI: install app updates in place — design

Date: 2026-10-07 · Target: the next 1.1.0 release candidate (branch
`feature/1.1-nextui-install`, from `release/1.1.0` at `015f847`, i.e. rc4)

Builds on `2026-09-26-app-updates-design.md` (shipped in v1.1.0-rc3), whose
"Follow-up: in-place install" section this replaces for NextUI. muOS keeps
Save to ARCHIVE and is out of scope here.

## Goal

When an update is available on NextUI and the Pak Store will not deliver it,
let the user install it from Settings → App updates instead of scanning a QR
code and copying files by hand. The update is checked before it replaces
anything, and a version that fails to start is rolled back automatically.

The Pak Store stays the route whenever it will deliver the update. Its
database is never written.

## Scope

**Offered** wherever rc3 decides `ViaReleasePage` on NextUI and the release
has an `Itch-io.NextUI.<tag>.pak.zip` asset with a sha256 digest:

- a newer release candidate (the Store never offers one);
- any update on an install the Store does not manage;
- a final release the Store will not offer yet, because `pak.json` on `main`
  still names an older version.

**Never offered** when the decision is `ViaPakStore`, nor for a version that
is not newer than the running one (`Decide` already enforces this, §2a of the
app-updates spec).

**Not in scope:** muOS ("Install now" as an alternative to ARCHIVE), the
`.pakz` bundle, writing the Pak Store database.

**The first build with this feature can only update to later builds.** rc3 and
rc4 have no installer, and the rollback lives in the *new* version's launcher
(§4), so the end-to-end path needs two releases that both carry the feature.
The debug override (§6) makes it testable before then.

## Research this design rests on

Verified 2026-10-07 on the TrimUI Brick (NextUI, `tg5040`), firmware kernel
4.9.191.

- `/mnt/SDCARD` is **vfat**, mounted `rw,sync`.
- With a binary running from `Tools/tg5040/.rt-live/`, renaming that folder to
  `.rt-prev` and a staged folder into its place both succeed. The running
  process keeps running, `/proc/<pid>/exe` follows it to `.rt-prev`, and a
  binary in the renamed folder still executes. The staged launcher, exec'd at
  the old path, runs from the new folder; renaming back restores the old one.
- NextUI hides every name starting with `.` (and ending in `.disabled`) from
  its menus: `hide()` in `workspace/all/common/utils.c`. `Tools/tg5040/.media`
  is an existing example. Staging and backup folders named `.Itch-io.pak.*`
  are therefore invisible.
- Settings and app data live in `$SHARED_USERDATA_PATH/<pak name>`
  (`.userdata/shared/Itch-io`), outside the pak folder, so replacing the
  folder cannot touch them. `launch.sh` derives `<pak name>` from the folder
  name, so the folder must keep its name.
- The NextUI release zip holds the pak's files at its root (`itchio`,
  `launch.sh`, `pak.json`, `assets/`, `lib/<platform>/`), with no enclosing
  folder. GitHub reports its size and a `sha256:` digest (checked on
  v1.1.0-rc4: 14 741 523 bytes).
- On the Brick, the Pak Store manages this install with a row saying
  `v1.0.25` while rc3 runs. The rc3 verdict for rc4 is `via=release-page`, the
  case this design turns into an install.
- The running version is stamped from `pak.json` by `build.sh`
  (`-X main.version`).

## Design

### 1. Deciding to offer it (`internal/appupdate`)

- `Release` gains the NextUI asset beside the muOS one: `NextUIAsset`,
  `NextUIDigest`, `NextUISize`, filled by `toRelease` from
  `Itch-io.NextUI.<tag>.pak.zip` (`NextUIAssetName(tag)`).
- New `Via` value `ViaInstall`. In `via()`, on NextUI, every path that today
  returns `ViaReleasePage` returns `ViaInstall` instead when the release has a
  NextUI asset with a positive size and a `sha256:` digest. `ViaPakStore` and
  the version rules are unchanged.
- `Verdict` carries the failed-install record (§4.3) so the screen can say
  "Retry" and `ShouldNotify` can stay quiet for that version.

### 2. Settings → App updates

A new row, NextUI and `ViaInstall` only, in the place `Save to ARCHIVE` takes
on muOS:

| State | Row | A | B |
|---|---|---|---|
| available | `Install v1.1.0-rc5` | start | — |
| downloading / checking / unpacking | progress bar, `Downloading…` / `Checking…` / `Unpacking…` | — | cancel |
| staged | `Restart now` | restart (§3) | `Later` |
| staged, Later chosen | `Installs at next launch` (annotation) | restart now | — |
| failed before (§4.3) | `Retry v1.1.0-rc5` | start | — |

Status lines below the rows, beside the existing release-page QR code (kept as
an alternative):

- `v1.1.0-rc5 is ready. Restart to finish installing.`
- `Not enough space on the SD card: needs 34 MB, 12 MB free.`
- `The download failed the integrity check.` / `The update is damaged: <reason>.`
- `v1.1.0-rc5 did not start, so v1.1.0-rc4 was kept.`

The corner notice is unchanged: it announces, App updates installs.

### 3. Download, check and stage (`appupdate.StageNextUI`)

Input: the release, the live pak folder (the directory of `os.Executable()`,
e.g. `/mnt/SDCARD/Tools/tg5040/Itch-io.pak`), a progress callback, and a
context for cancel. All paths are siblings of the live folder, on the same
card:

- `.Itch-io.pak.staged.zip` — the download
- `.Itch-io.pak.staged/` — the unpacked update
- `.Itch-io.pak.prev/` — the version being replaced, kept until the new one
  confirms
- `.Itch-io.pak.failed/` — an update that did not start, kept until the
  restored version starts

(`Itch-io.pak` stands for the live folder's actual name throughout.)

Steps:

1. Delete any leftover `.staged.zip` / `.staged/`. Check free space: zip size
   + 2.5 × zip size for the unpacked files, against the card.
2. Download through `partfile.Create(.Itch-io.pak.staged.zip)` with the
   existing `fetchAsset` loop (30 s idle timeout, size and SHA-256 check,
   `Abort()` on failure or cancel). Add the `Tools/<platform>` directory to
   the `partfile.Sweep` roots in `main_sdl.go`.
3. Unpack into `.staged/`. Reject absolute paths, `..`, `\`, symlinks and any
   entry that is not a regular file or directory; cap entry count and total
   size. Files get mode 0755 for `itchio` and `launch.sh`, 0644 otherwise
   (vfat ignores them; other filesystems do not).
4. Check the staged files:
   - `itchio`, `launch.sh` and `pak.json` exist;
   - `pak.json`'s `version` equals the release tag;
   - `itchio` is an ELF file with `e_machine == EM_AARCH64`;
   - `/bin/sh -n launch.sh` succeeds — the new launcher is what rolls back
     (§4), so a syntax error must never be installed.
5. Delete the zip and write `.staged/.complete` last. A `.staged/` without it
   is incomplete and is deleted at the next launch (§3.1).

Any failure deletes `.staged.zip` and `.staged/` and is logged with the reason.

### 3.1 Applying it (`appupdate.ApplyStaged`)

One function, unit-tested on temporary directories:

1. If `.staged/.complete` is missing, delete `.staged/` and return "nothing
   to apply".
2. Delete any `.prev/` left from an earlier update.
3. Write `$HOME/update_pending.json`:
   `{"from": "v1.1.0-rc4", "to": "v1.1.0-rc5", "at": "<RFC 3339>"}`.
   It is written before the renames so that there is no moment when the new
   version is in place without rollback protection. If the renames then fail,
   the current version starts, confirms (§4.1) and removes it: harmless.
4. Rename the live folder to `.prev/`, then `.staged/` to the live name. If the
   second rename fails, rename `.prev/` back, delete the pending file and
   return the error.
5. `syscall.Exec("/bin/sh", ["/bin/sh", "<live>/launch.sh"], os.Environ())`.
   Same PID and environment, so NextUI sees the same pak still running.

Called from two places:

- **Restart now.** The event loop sets `restartForUpdate` and leaves the loop
  as a normal quit does, so every deferred clean-up runs and SDL releases the
  display and controller. `main()` checks the flag after `runSDL()` returns,
  flushes the log, and calls `ApplyStaged`.
- **Later.** Early in `main()`, after firmware detection and logger setup and
  before SDL starts, if `.staged/.complete` exists, call `ApplyStaged`. The
  old version draws nothing.

If `ApplyStaged` fails, the app logs it and starts as the current version; the
staged folder is deleted so it is not retried on every launch.

### 4. Confirm or roll back

#### 4.1 The new binary confirms

After the first frame is presented and one input poll has completed, the app
deletes `$HOME/update_pending.json`, logs
`update: v1.1.0-rc4 → v1.1.0-rc5 confirmed`, and deletes `.prev/` (and any
`.failed/`) on a background goroutine.

#### 4.2 The launcher rolls back

`launch.sh`, at the point where it now runs `exec "$PAK_DIR/itchio"`:

- **No `$HOME/update_pending.json`:** unchanged, `exec`.
- **Pending:** run `itchio` without `exec` and wait for it. When it exits, if
  the pending file is still there, the update did not start:
  1. rename the live folder to `.Itch-io.pak.failed` (deleting an older one
     first) and `.Itch-io.pak.prev` back to the live name;
  2. `mv` the pending file to `$HOME/update_failed.json`;
  3. `exec /bin/sh <live>/launch.sh` — the restored version's launcher.

  If `.prev` is missing, the launcher only moves the pending file aside and
  exits with the binary's status; there is nothing to restore.

Rollback deletes the pending file before starting the old version, so it can
run at most once per update and cannot loop.

The launcher writes one line per decision to `$HOME/update_launcher.log`
(launch.sh has no other log); the restored app copies it into `itchio.log` at
startup and deletes it.

#### 4.3 The restored version reports it

At startup, an `update_failed.json` is read into the checker and deleted:

- App updates shows `v1.1.0-rc5 did not start, so v1.1.0-rc4 was kept.` and
  the row reads `Retry v1.1.0-rc5`.
- That version is recorded as failed in `update_state.json`; `ShouldNotify`
  returns false for it, so the corner notice does not reappear. A newer
  version is announced normally.

#### 4.4 What this does not catch

- A new `launch.sh` that passes `sh -n` but fails before its pending check.
- Power loss between the two renames in §3.1, which leaves no live folder. The
  pending file names both versions, but nothing launches a pak that NextUI no
  longer lists. The Pak Store's own install has a longer window with no
  folder at all.

### 5. Wiring

- `firmware.Env` gains `PakDir()`: the live pak folder on NextUI (from
  `os.Executable()`), `""` on muOS and host. `ViaInstall` is only produced
  when it is non-empty.
- The `UpdateChecker` interface gains `StartInstall()`, `CancelInstall()`,
  `InstallStatus()` and `RequestRestart()`, mirroring the ARCHIVE methods.
- Free space uses the existing `freeBytes`.

### 6. Debug override for the update source

For testing on hardware before two feature releases exist.

- If `$HOME/update_source_override` exists, its first line is a base URL
  (e.g. `http://127.0.0.1:8765/`). `Source.ReleasesURL` becomes
  `<base>releases.json` and `Source.PakJSONURL` becomes `<base>pak.json`.
  Asset URLs come from the served `releases.json` as usual.
- While it is active, every check logs
  `appupdate: WARNING using update source override <base>`, App updates shows
  `Test update source: <base>` under the status, and state goes to
  `update_state.override.json`, so ETags and announced versions never mix
  with the real ones.
- `scripts/update-fixture.sh` (host):
  - `update-fixture.sh serve <pak.zip>`: reads the version from the zip's
    `pak.json`; writes `releases.json` (one pre-release with that tag, the
    asset's size, `sha256:` digest and an `http://127.0.0.1:8765/…` download
    URL) and a `pak.json`; serves them with `python3 -m http.server`; runs
    `adb reverse tcp:8765 tcp:8765`; writes the override file on the device.
    Stops serving and removes the reverse on Ctrl-C.
  - `update-fixture.sh serve --broken <pak.zip>`: the same, but the served zip
    has its `itchio` replaced by the device's own `/bin/busybox` (pulled with
    `adb pull`). It passes every staging check — an aarch64 ELF — and exits at
    once with "applet not found", which is exactly the rollback case.
  - `update-fixture.sh off`: deletes the override file on the device.
- Hardware procedure: versions must parse as `vX.Y.Z[-rcN]`, so use two
  unreleased rc numbers. Build with `pak.json` at `v1.1.0-rc5` and deploy it
  with `deploy.sh`; copy the zip aside; build again with `pak.json` at
  `v1.1.0-rc6`, and `update-fixture.sh serve` that zip. Install it from App
  updates. Both builds use `release.sh --allow-dirty`; restore `pak.json`
  afterwards. Because the override uses its own state file, nothing the
  fixture announces leaks into the real channel.

### 7. Logging

- `appupdate: install start v… → v… asset=… size=…`, download progress at
  debug, `verify ok|mismatch`, `unpack <n> files <bytes>`, each staging check
  with its result, `staged … in <duration>`, cancel.
- `appupdate: apply: rename <a> → <b>` for each rename, `exec <launch.sh>`,
  and every failure with its path and error.
- Startup: `update: pending v… → v…`, `confirmed`, `.prev removed`,
  `rolled back (from update_failed.json)`, and the launcher log lines.
- The override warning (§6).

### 8. Error handling summary

| Failure | Behaviour |
|---|---|
| No NextUI asset / no digest | release-page QR only, no Install row |
| Offline or GitHub error during download | partial file deleted, `netstate.Describe` message, Install stays available |
| Not enough space | message, nothing written |
| Size or digest mismatch | partial deleted, integrity message |
| Bad zip entry, missing file, wrong version, wrong arch, `sh -n` fails | staged folder deleted, `The update is damaged: <reason>` |
| Cancel | partial and staged folder deleted |
| Incomplete `.staged/` at launch | deleted, logged |
| Second rename fails | first rename undone, error logged, app starts as the current version |
| New version exits before confirming | launcher rolls back once (§4.2), restored version reports it (§4.3) |
| `.prev` missing during rollback | pending file moved aside, nothing restored, logged |

## Testing

- `appupdate` unit tests:
  - `toRelease` fills the NextUI asset; `via()` returns `ViaInstall` exactly
    where rc3 returned `ViaReleasePage` and an asset with a digest exists, and
    never where it returned `ViaPakStore`.
  - `StageNextUI` against an `httptest` server: success; size and digest
    mismatch; cancel; zip with `..`, an absolute path, a symlink; missing
    `pak.json`; wrong version; an x86-64 `itchio`; a `launch.sh` with a syntax
    error. Each failure leaves no `.staged*` behind.
  - `ApplyStaged` on temp dirs: success, missing `.complete`, existing `.prev`,
    a second rename that fails (injected), pending file contents.
  - `ShouldNotify` stays quiet for a recorded failed version.
- `launch_test.sh` cases: no pending file (unchanged `exec`); pending and the
  binary confirms; pending and the binary exits without confirming (folders
  swapped back, `update_failed.json` written, old launcher run once); pending
  with no `.prev`.
- `release-pak_test.sh`: the NextUI zip's root holds `itchio`, `launch.sh`,
  `pak.json` (already true; asserted because the installer depends on it).
- Hardware, on the Brick with the fixture (§6): install and Restart now;
  install and Later, then relaunch from Tools; cancel mid-download; the
  `--broken` zip rolls back and shows the message; the notice does not return
  for the failed version. Then once on an H700 tester's device, since its pak
  folder is `Tools/h700/` and ships no `lib/`.
