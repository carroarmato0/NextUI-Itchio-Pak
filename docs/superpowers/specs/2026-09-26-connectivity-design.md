# Connectivity: knowing when the app is offline — design

Date: 2026-09-26 · Target: v1.1.0-rc2, before app updates
(`docs/superpowers/specs/2026-09-26-app-updates-design.md` depends on it)
Branch: `feature/1.1-connectivity`, from `release/1.1.0`

## Goal

Everything the app does needs the network, yet it has no notion of being
offline. Make it know, say so in plain words, stop hammering while offline,
pick up again by itself when the connection returns, and never damage an
installed game because a connection dropped.

## How it behaves today (read 2026-09-26 on `release/1.1.0`)

- **Game list with a cache:** shown at once; the background refresh fails with
  only a log line (`cache: full fetch failed`) and is never retried.
- **Game list without a cache:** the list screen draws `"Error: " + err.Error()`,
  i.e. raw Go text such as `Get "https://itch.io/…": dial tcp: lookup itch.io:
  no such host`. A special case still explains a Cloudflare 403 and asks the
  user to visit itch.io in a browser — obsolete since itch.io fixed the
  blocking upstream.
- **Inventory update service:** one request per installed game; offline, each
  fails as a "transient error" (good: network errors never mark a game
  removed), but with the client's 30 s timeout a dropped link can keep it busy
  for minutes, and Settings shows `checking…`, then an old time, with no hint
  why.
- **Cover art:** only decoding failures go into `ImageCache.failed`; a network
  failure is forgotten, so `Get` queues the same fetch again on every redraw.
  Offline, scrolling the list fires a doomed request per visible cover per
  frame (two at a time, 20 s timeout each).
- **Downloads (`itchio.streamToFile`):**
  - write with `os.Create(dest)` straight to the final ROM path. When updating
    a game, the working ROM is truncated at the first byte; if the stream then
    fails, a partial file is left where the launcher lists it and the good copy
    is gone;
  - have no overall timeout (correct for big files) and no idle timeout, so a
    stalled connection can hang the download screen indefinitely;
  - do not check the byte count against `Content-Length`.
- **Game page, file list, sign-in:** each shows its own `s.err` text.
- **Nothing reads the device's network state**, although both firmwares make it
  cheap: on the Brick (NextUI) and Smart Pro (muOS), `/sys/class/net/wlan0/operstate`
  is `up` with a default route in `/proc/net/route`; NextUI also writes
  `wifi=1` to `minuisettings.txt`.
- **No real-time clock on many of these devices.** A wrong date makes every
  certificate look expired or not yet valid; today that surfaces as an
  unreadable `x509` error.

## Design

### 1. `internal/netstate` (headless-safe)

**Classification (`classify.go`).** `Classify(err error) Reason`, unwrapping
with `errors.As`/`errors.Is`:

| Reason | Recognised from | Counts as offline |
|---|---|---|
| `NoNetwork` | `ENETUNREACH`, `EHOSTUNREACH`, or the local check (below) says no route | yes |
| `DNS` | `*net.DNSError` | yes |
| `Unreachable` | `ECONNREFUSED`, `ECONNRESET`, dial or TLS-handshake timeout, `os.ErrDeadlineExceeded`, mid-body `io.ErrUnexpectedEOF` | yes |
| `Clock` | `x509.CertificateInvalidError{Reason: Expired}` (covers not-yet-valid) **and** the device date is before the build date | yes |
| `Intercepted` | `x509.UnknownAuthorityError`, `x509.HostnameError` — typically a hotel/public Wi-Fi sign-in page | yes |
| `Canceled` | `context.Canceled` (the user pressed Back) | no, ignored |
| `Other` | anything else, including HTTP statuses | no |

The build date comes in by `-ldflags` next to `gitCommit`. A clock is judged
wrong only when it is earlier than the build date; an `Expired` error with a
plausible clock is reported as `Intercepted`, never guessed as a clock problem.

**State (`state.go`).** One process-wide value: `Unknown` at start, `Online`,
or `Offline(reason, since)`.

- Any HTTP response — whatever its status, 4xx and 5xx included — means the
  network works: `Online`. An itch.io outage is an HTTP problem, not "offline".
- A failure that counts as offline sets `Offline(reason)`.
- `Canceled` and `Other` leave the state alone.
- `Subscribe(func(State))` for listeners; each notification also pushes the
  SDL wake-up event through a callback set by `main_sdl.go`, so the UI redraws.
- Everything logged on each transition: `netstate: online -> offline reason=dns (lookup itch.io: …)`.

**Feeding it.** A `RoundTripper` wrapper records the outcome of every request.
It sits inside `itchio`'s transport chain, so the itch.io client, the image
cache (which already uses `client.HTTPClient()`), and the app-update checker
(given the same wrapper) all feed one state. Errors that happen after the
response starts — reading a download body — are reported explicitly with
`netstate.Report(err)`.

**No extra traffic while online.** There is no periodic "ping". itch.io asked
the app to be a considerate client, and the requests the app makes anyway are
enough to tell. The only extra request is the reconnect probe in §2, and it
only runs while offline, with back-off.

### 2. Getting back online

While `Offline`, a goroutine runs a **local** check every 5 s: a default route
in `/proc/net/route` on an interface whose `operstate` is `up`. It costs no
network traffic.

- No route: stay `Offline(NoNetwork)`. This is also what makes a Wi-Fi switched
  off in the system menu read as "No Wi-Fi" instead of a DNS error.
- A route (again): make **one** `HEAD https://itch.io/` request (no body, the
  cheapest request that proves itch.io answers) with back-off 15 s, 30 s, 60 s … capped at 5 min,
  reset whenever the route changes. Success flips the state to `Online`.
- Stopped while `Online`; nothing polls on a healthy connection.

On the way back to `Online`, deferred work runs once, in this order: game-list
refresh (if it was skipped or failed), inventory check (if one was aborted),
app-update check (if it was skipped), and covers that failed while offline are
requested again.

### 3. Background work while offline

- **Inventory service:** checks `netstate` before each request; the first
  offline-class failure aborts the run. The per-game `checked_at` times are not
  touched, and the service remembers a run is owed. Settings shows `offline`
  instead of `checking…`.
- **Game-list refresh:** a failed `buildCache` is remembered and retried on
  reconnect instead of waiting for the next launch.
- **Image cache:** while offline, `Get` queues no fetches (it returns nil, the
  placeholder shows). On reconnect the cache's notify callback fires once, the
  screen redraws, and visible covers are fetched. Decoding failures stay in
  `failed` as now.
- **App-update check:** skipped while offline, run on reconnect.

### 4. Downloads that cannot break an installed game

`streamToFile`:

- writes to `.<name>.itchio-part` in the destination folder (dot-prefixed so neither
  launcher lists it; same folder, so the final rename stays on one filesystem);
- aborts after 30 s with no bytes received (an idle timer reset on each read),
  reporting `Unreachable`;
- checks the byte count against `Content-Length` when there is one;
- on success, renames the partial file over `dest`, so an existing ROM is replaced
  in one step; on any failure or cancel, deletes the partial file and leaves the
  existing ROM untouched;
- logs the outcome and the bytes written.

The ZIP flow already downloads to a temp file first; it gets the idle timeout
and the length check too.

**Abandoned partial files.** A crash, a kill, a dead battery or a power-off
mid-download skips the cleanup above. Partial files are handled by a journal
plus a startup sweep, both owned by one small helper
(`internal/partfile`) that every writer of partial files goes through: game
downloads, music downloads, and the muOS Save to ARCHIVE.

- **Naming.** `.<name>.itchio-part`. The distinctive suffix means the app only
  ever deletes files it created, never another tool's `.part` or `.tmp`.
- **Journal.** Before a partial file is created, its absolute path is added to
  `partials.json` in the data directory (written atomically, like the
  settings). It is removed from the journal after the final rename or the
  delete. This covers folders the user picked by hand (ROM Location: ask), which
  no scan of known folders would find.
- **Startup.** Every journal entry that still exists, and still carries the
  suffix, is deleted, then the journal is cleared. This runs synchronously
  before the first screen: it is a handful of `unlink`s at most.
- **Sweep.** A background pass then lists, non-recursively, the folders the app
  writes to by default — every ROM folder from `firmware.Env`, the music
  folder, `ARCHIVE/` on muOS — and deletes files with the suffix that are not
  in the in-process set of downloads currently running. That catches leftovers
  whose journal was lost (a corrupt `partials.json`, or files left by a build
  from before the journal existed) without racing a download the user
  starts in the first seconds.
- **No resume.** Partial files are always deleted, never resumed: itch.io's
  download URLs are signed and short-lived, so resuming would mean a fresh
  request anyway, and a restarted download of a GB/GBC/GBA-sized file costs
  seconds.
- **Logged:** each file removed, with its size and whether the journal or the
  sweep found it; a journal that failed to parse.

### 5. What the user sees

**One message component** (`internal/ui/net_problem.go`) replaces the ad-hoc
error text on: the list screen with no cache, the game page, the file list,
downloads, sign-in, and the cache-refresh screen. It maps the reason to a
title and a hint:

| Reason | Title | Hint |
|---|---|---|
| `NoNetwork` | No Wi-Fi connection | Turn on Wi-Fi in the system settings. |
| `DNS`, `Unreachable` | Can't reach itch.io | Check your Wi-Fi connection, then try again. |
| `Clock` | The date and time are wrong | Secure connections fail until the clock is set. |
| `Intercepted` | This network is blocking the connection | Public Wi-Fi may need you to sign in on a phone or computer first. |
| HTTP 5xx | itch.io is having problems | Try again in a few minutes. |
| other | Something went wrong | the error's first line, shortened |

The raw error always goes to the log, never the screen. Footer: A Retry,
B Back. On screens that retry by themselves on reconnect (the list screen),
the message changes on its own as soon as the state flips.

The Cloudflare branch on the list screen is removed; a 403 becomes an ordinary
HTTP error.

**Offline chip.** While `Offline`, the list screen's header shows a small
`Offline` pill in the theme's Warning colour. The cached list stays fully
usable — browsing, search, filters, and installed games (play, delete,
rename). Actions the user starts (open a game page, download, sign in, Retry)
always try the network, even while the state says Offline: the attempt is
the quickest way to find out the connection is back, and a successful one
flips the state to Online for everything else.

All text follows `abbreviate(w)` where it has to fit a narrow screen; no
colour literals.

### 6. Logging

Transitions (with reason and the underlying error), partial-file cleanup (see §4), each local route check
result at debug, each reconnect attempt and its outcome, deferred work being
queued and run, the image cache fetch resuming on reconnect, and download
partial-file creation, idle abort, length mismatch, rename and cleanup.

## Testing

- `Classify` against real error values: `*net.DNSError`, a connection refused
  by a closed `httptest` port, an `httptest` handler that stalls (dial and idle
  timeouts), a server with a certificate outside its validity window with the
  clock before and after the build date, an unknown-authority certificate,
  `context.Canceled`.
- State machine: transitions, HTTP 5xx counts as online, `Canceled` ignored,
  subscribers notified once per transition.
- Local route check against fixture copies of `/proc/net/route` and
  `operstate` (with/without default route, interface down) under a temp root.
- Reconnect loop with an injected clock: back-off sequence, reset on route
  change, deferred work run once in order.
- Inventory service aborts on the first offline failure and leaves `checked_at`
  alone; the image cache queues no fetch while offline.
- Downloads: failure mid-stream leaves the original ROM byte-identical and no
  `.itchio-part`; idle timeout fires; short body rejected; success replaces the file.
- `partfile`: journal entries survive a simulated crash and are deleted at
  startup; the sweep deletes only suffixed files, skips active downloads, and
  leaves other `.part`/`.tmp` files alone; a corrupt journal falls back to the
  sweep.
- `devshot` scenes: the message component for each reason, list screen with the
  Offline chip, at all four geometries in the palette audit.
- On hardware, both firmwares: Wi-Fi off mid-session (chip appears, inventory
  stops, turning Wi-Fi on recovers without relaunch and covers fill in); start
  with Wi-Fi off and no cache; Wi-Fi off during a game update (the old ROM
  survives); a date set to 2020.

## Out of scope

- Telling Wi-Fi-off apart from Wi-Fi-on-but-not-associated beyond the route
  check.
- Opening the firmware's Wi-Fi settings from the app.
- Offline queueing of downloads.
