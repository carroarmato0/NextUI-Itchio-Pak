# Logging Design

**Date:** 2026-04-21
**Status:** Approved

## Problem

The app currently has inconsistent logging: some packages (`download.go`,
`game.go`) have detailed flow logs, others (`feed.go`, `settings.go`,
`screen_download.go`) have none at all. All output uses the stdlib `log`
package with no level filtering, so there is no way to reduce noise on a
healthy device or increase detail when diagnosing a bug.

The log file is also the primary diagnostic artefact requested in bug reports,
so it must reliably capture the most useful information when something goes
wrong.

## Goals

- Consistent, level-aware logging across every package.
- Low I/O footprint at the default level (INFO): only key transitions and
  errors are written.
- Full request/response detail available at DEBUG level, toggled from the
  Settings UI without a restart.
- Startup environment header so a log file alone identifies the device,
  NextUI version, app version, and active log level.
- Well-structured output that is human-readable in a plain text viewer and
  grep-friendly for automated analysis.
- No sensitive user data (API key, account-linked tokens) written to the log
  under any level or circumstance.

## Non-Goals

- External logging infrastructure (syslog, remote log shipping).
- Log rotation or size capping. The log file is opened with `O_APPEND` so it
  grows across sessions. At INFO level the volume is low enough that this is
  not a practical concern on SD card storage.
- Per-subsystem log levels (one global level is sufficient).

## Output Format

```
2026/04/21 14:32:01 [INFO]  itchio-pak v1.0.3 starting
2026/04/21 14:32:01 [INFO]  platform=tg5040 nextui=NextUI-20260407-0 log_level=info
2026/04/21 14:32:05 [DEBUG] feed: fetching https://itch.io/games/made-with-gb-studio.xml?page=1
2026/04/21 14:32:06 [INFO]  feed: page 1 returned 36 games
2026/04/21 14:32:09 [ERROR] download: HTTP 403 from resolver: {"errors":["must purchase"]}
```

- Timestamp prefix is supplied by stdlib `log` (`log.SetFlags(log.LstdFlags)`).
- Level tag is left-padded to 7 characters (`[DEBUG] `, `[INFO]  `, `[WARN]  `,
  `[ERROR] `) so message columns align.
- Message convention: `subsystem: free-form text`, with `key=value` pairs for
  structured fields where useful.

## Level Assignment Rules

| Level | When to use |
|-------|-------------|
| `[ERROR]` | Any `err != nil` condition, HTTP non-2xx, parse failures, missing required tokens (CSRF, download key). The app cannot complete the current operation. |
| `[WARN]`  | Degraded-but-recoverable: config file corrupted (defaults used), version file absent, zero uploads after extension filter, CSRF missing on signed download page. |
| `[INFO]`  | Key lifecycle transitions: startup, display resolution, page loaded (N games), download started, download complete. |
| `[DEBUG]` | Full flow detail: URLs fetched, HTTP status codes on success, extracted keys and upload IDs, page tag lists, CSRF token presence, streamToFile progress. |

## Architecture

### `internal/logger` package

Single file (`logger.go`). No structs or instances — package-level state only.

```go
type Level int32

const (
    LevelDebug Level = iota
    LevelInfo
    LevelWarn
    LevelError
)

// SetLevel sets the minimum level. Safe to call from any goroutine.
func SetLevel(l Level)

// LevelFromString maps "debug" → LevelDebug, anything else → LevelInfo.
// Unknown/empty strings resolve to LevelInfo so misconfigured values are safe.
func LevelFromString(s string) Level

func Debug(format string, args ...any)
func Info(format string, args ...any)
func Warn(format string, args ...any)
func Error(format string, args ...any)
```

Internally, each function checks the atomic level before calling `log.Printf`
with the appropriate `[LEVEL]` prefix. The stdlib `log` package retains its
default flags so the timestamp is prepended automatically.

Level is stored as `sync/atomic` int32 so `SetLevel` is safe to call from the
Settings screen goroutine without a mutex.

### Config change (`internal/settings`)

```go
type Config struct {
    // existing fields ...
    LogLevel string `json:"log_level,omitempty"` // "info" | "debug", default "info"
}
```

`omitempty` preserves backward compatibility: existing config files without
this field unmarshal to `""`, which `LevelFromString` maps to `LevelInfo`.

`defaults()` leaves `LogLevel` as `""` (resolves to INFO at runtime).

### Settings UI (`internal/ui/screen_settings.go`)

New row added to the settings list, positioned after the ROM options and before
the content moderation section:

```
Log Level        [ Info ]    ←→ to change
```

- Left/Right cycles between `"info"` and `"debug"`.
- On change: `cfg.Save(cfgPath)` is called, then
  `logger.SetLevel(logger.LevelFromString(cfg.LogLevel))` — takes effect
  immediately, no restart required.
- Display label: `"Info"` or `"Debug"` (title-case for readability; stored
  lowercase in JSON).

### Startup environment header (`cmd/itchio-pak/main.go`)

Called after the log file is opened and the logger is initialised, before
`runSDL()`:

```go
logger.Info("itchio-pak %s starting", version)
logger.Info("platform=%s nextui=%s log_level=%s",
    readPlatform(), readNextUIVersion(), cfg.LogLevel)
```

**`readPlatform()`** — returns `os.Getenv("PLATFORM")`, falls back to
`"unknown"`.

**`readNextUIVersion()`** — reads `/mnt/SDCARD/.system/version.txt`, returns
the first non-empty trimmed line. If the file is missing, empty, or unreadable,
returns `"unknown"` silently (no log entry — absence is expected outside
NextUI).

## Secret Redaction

### Problem

The itch.io API key appears directly in URL path segments:

```
https://itch.io/api/1/<API_KEY>/game/<GAME_ID>/uploads?download_key_id=<KEY_ID>
https://itch.io/api/1/<API_KEY>/upload/<UPLOAD_ID>/download?download_key_id=<KEY_ID>
```

At DEBUG level these URLs would be logged, exposing the key in the log file.
Download key JWTs (which tie a request to a specific purchase) and CSRF tokens
are also sensitive. A log file is often shared publicly in a bug report, so
none of these values must appear in any form.

### Design

**Logger-level secret registry** is the primary defence. The `internal/logger`
package exposes:

```go
// RegisterSecret registers a plaintext value to be fully replaced in all
// future log output. label is the replacement string written instead
// (e.g. "[API-KEY]"). Calling with an empty value is a no-op.
// Safe to call from any goroutine.
func RegisterSecret(value, label string)
```

Every string that passes through `Debug`, `Info`, `Warn`, or `Error` is run
through an internal `redact(s string) string` function before being written.
`redact` performs `strings.ReplaceAll` for each registered secret in order.

Secrets are stored in a `sync.RWMutex`-protected slice of `{plain, label}`
pairs. The slice is expected to be very short (one entry for the API key in
normal use), so linear scan is fine.

**Registration points:**

| Secret | Label | When registered |
|--------|-------|-----------------|
| `cfg.APIKey` | `[API-KEY]` | `main.go` after config is loaded; Settings screen after a new key is saved |

**Empty-value guard:** `RegisterSecret("", label)` is a no-op. This means the
call is unconditional at startup — no need for an `if cfg.APIKey != ""` guard
at the call site.

**CSRF tokens and signed download URLs** are *never logged as raw values*.
Call sites log only metadata:
- `csrf=present` / `csrf=absent` rather than the token value
- `key=present` / `key=absent` rather than the JWT string
- Signed download page URLs are logged as `<redacted>` unconditionally

**Download key IDs** (numeric, from `owned-keys`) are not account-unique on
their own but are still omitted from logs — logging the filename and upload ID
is sufficient for debugging.

### What the log looks like with redaction active

```
2026/04/21 14:32:09 [DEBUG] auth: resolving upload id=12345678 via [API-KEY]
2026/04/21 14:32:09 [DEBUG] auth: GET https://itch.io/api/1/[API-KEY]/game/99999/uploads
2026/04/21 14:32:09 [DEBUG] uploads: POST csrf=present key=present
```

### What this does NOT protect against

- Log lines written before `RegisterSecret` is called (the startup header and
  SDL init lines). The API key is not available at that point, so there is
  nothing to redact.
- Secrets embedded in error messages returned by the itch.io API (e.g. an
  error body that echoes back a URL). These are truncated to 200 characters in
  error log lines, which limits exposure but does not eliminate it. This is an
  acceptable residual risk given that API errors echoing credentials back would
  be an itch.io bug.

## Per-File Logging Changes

### `internal/itchio/feed.go`

| Site | Level | Message |
|------|-------|---------|
| `FetchGamesFromURL` start | DEBUG | `feed: fetching <url>` |
| HTTP non-2xx | ERROR | `feed: HTTP <status> from <url>` |
| `xml.Unmarshal` failure | ERROR | `feed: parse XML: <err>` |
| Success | DEBUG | `feed: parsed <count> items from XML` |
| `FetchTotalGames` HTTP non-2xx | ERROR | `feed: total-games HTTP <status>` |
| `FetchTotalGames` regex miss | WARN | `feed: result count not found on browse page` |

### `internal/itchio/game.go` — `FetchGameDetail`

| Site | Level | Message |
|------|-------|---------|
| Fetch start | DEBUG | `game: fetching detail <url>` |
| HTTP non-2xx | ERROR | `game: detail page HTTP <status> for <url>` |
| gameID missing | WARN | `game: gameID not found on page (paid download will not work)` |
| CSRF missing | WARN | `game: CSRF token not found on page` |
| Screenshot count | DEBUG | `game: <n> screenshots found` (existing, reclassified) |
| Page tags | DEBUG | `game: <n> page tags: [...]` (existing, reclassified) |

### `internal/itchio/game.go` — `ParseDownloadPage`

| Site | Level | Message |
|------|-------|---------|
| Fetch start | DEBUG | `download-page: fetching <url>` |
| HTTP non-2xx | ERROR | `download-page: HTTP <status>` |
| CSRF missing | WARN | `download-page: CSRF token not found (resolver POST may fail)` |
| Upload found | DEBUG | `download-page: found upload <filename> id=<id>` |
| Non-ROM skipped | DEBUG | `download-page: skipping <filename> (not .gb/.gbc)` |
| Result | INFO | `download-page: <n> uploads available` |

### `internal/itchio/download.go` — `FetchUploads`

| Site | Level | Message |
|------|-------|---------|
| Game page HTTP non-2xx | ERROR | `uploads: game page HTTP <status>` |
| CSRF missing | ERROR (already returned as err) | — |
| Signed URL received | DEBUG | `uploads: signed download URL received` (URL itself not logged) |
| Extracted key | DEBUG | `uploads: download key extracted` (value not logged) |
| Each upload | DEBUG | `uploads: found <filename> id=<upload_id>` |

### `internal/itchio/download.go` — `streamToFile`

| Site | Level | Message |
|------|-------|---------|
| Start | INFO | `stream: → <dest> (<content-length> bytes)` (CDN URL not logged — may contain signed tokens) |
| Complete | INFO | `stream: done, wrote <n> bytes` |
| Write error | ERROR | `stream: write error after <n> bytes: <err>` |
| Read error | ERROR | `stream: read error after <n> bytes: <err>` |

### `internal/itchio/download_auth.go`

| Site | Level | Message |
|------|-------|---------|
| Owned-keys request | DEBUG | `auth: fetching owned keys for game_id=<id>` (API key redacted by registry) |
| Owned-keys HTTP non-2xx | ERROR | `auth: owned-keys HTTP <status>` |
| Upload list request | DEBUG | `auth: fetching upload list for game_id=<id>` (API key redacted by registry) |
| Zero `.gb`/`.gbc` after filter | WARN | `auth: no .gb/.gbc uploads found (game has <n> total uploads)` |
| Download key ID | — | not logged (ties request to user account) |
| Each upload | DEBUG | `auth: found <filename> id=<upload_id>` |
| CDN resolve start | DEBUG | `auth: resolving CDN for upload id=<upload_id>` (API key redacted by registry) |
| Streaming start | INFO | `auth: streaming to <dest>` |

### `internal/settings/settings.go`

| Site | Level | Message |
|------|-------|---------|
| File missing (first run) | DEBUG | `settings: config not found at <path>, using defaults` |
| JSON parse failure | WARN | `settings: config at <path> is invalid, using defaults: <err>` |
| Save failure | ERROR | `settings: failed to save config to <path>: <err>` |

### `internal/ui/screen_download.go`

| Site | Level | Message |
|------|-------|---------|
| Download goroutine start | INFO | `download: starting "<title>" file=<filename> dest=<dest> auth=<bool>` |
| Download success | INFO | `download: complete file=<filename>` |
| Download failure | ERROR | `download: failed file=<filename>: <err>` |

### `internal/ui/screen_fetch_uploads.go`

| Site | Level | Message |
|------|-------|---------|
| Path decision | DEBUG | existing log reclassified to DEBUG |
| Zero uploads (auth path) | WARN | `fetch: no .gb/.gbc uploads found for game (auth path)` |
| Zero uploads (free path) | WARN | `fetch: no .gb/.gbc uploads found for game (free path)` |

## README Update

Add a **Diagnostics** paragraph to the Settings section:

> **Log Level** — `Info` (default) records key events and all errors. Set to
> `Debug` to capture the full HTTP request/response flow — useful when
> reporting a bug involving a download failure or a feed that won't load. The
> log file is written to `.userdata/<platform>/logs/itchio-pak.log` on the SD
> card.

## Bug Report Template Update

Add a hint to the Log File field description:

> If the bug involves a network or download failure, set **Log Level → Debug**
> in Settings (press Start → scroll to Log Level), reproduce the issue, then
> attach the log. Debug logs capture the full HTTP flow and are much more
> useful for diagnosing scraping or download problems.

## Migration Summary

All existing `log.Printf` / `log.Println` / `log.Fatalf` call sites are
replaced with the appropriate `logger.*` call. `log.Fatalf` (used for fatal
init errors in `main_sdl.go`) becomes `logger.Error` followed by `os.Exit(1)`
— or remains `log.Fatalf` since it fires before the logger is warm; this is
acceptable.

Import of `"log"` is removed from each file once migrated; `"log"` is retained
only in `main.go` for the file setup (before `logger.SetLevel` is called) and
in `logger.go` itself.
