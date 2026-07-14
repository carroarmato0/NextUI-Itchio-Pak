# Smart ZIP Support — Design Spec

**Date:** 2026-05-11  
**Status:** Approved  

---

## Overview

Itch.io game uploads frequently use ZIP files. Today the app streams a ZIP directly to
the ROM folder unchanged. This spec adds two smart behaviours:

1. **Remote ZIP inspection** — read the ZIP's central directory via HTTP Range requests
   (without downloading the full file) to classify contents before committing to a download.
2. **Selective extraction** — route ROM files to their correct ROM folders, music files to
   `/mnt/SDCARD/Music/<game>/`, and discard the ZIP when extraction is needed. When a ZIP
   contains only a single ROM the ZIP is kept as-is (emulators load it directly).

---

## Content Classification

Files inside a ZIP are classified by extension:

| Kind | Extensions |
|------|-----------|
| ROM | `.gb` `.gbc` `.gba` |
| Music | `.mp3` `.ogg` `.flac` `.wav` `.opus` `.mod` `.xm` `.s3m` `.it` |
| Other | everything else (readmes, images, PDFs, etc.) — ignored |

---

## ZIP Routing Table

| ZIP contents | Route | Result |
|---|---|---|
| Single ROM, no music | `DownloadScreen` (unchanged) | ZIP kept in ROM folder |
| Multiple ROMs, same extension | `ZIPContentsScreen` (version picker) | User picks one; extracted; ZIP discarded |
| Multiple ROMs, mixed extensions (GB + GBC) | `ZIPDownloadScreen` (auto) or `ZIPContentsScreen` (ask) | One of each extracted to respective folder; ZIP discarded |
| ROM(s) + music | `ZIPContentsScreen` (ask) or `ZIPDownloadScreen` (auto) | ROMs extracted; music extracted; ZIP discarded |
| Music only | Same as above | Music extracted to Music folder; ZIP discarded |
| Nothing recognised | Current behaviour | `NeedsFormat=true` or skipped |

Multiple ROMs of the **same extension** always force `ZIPContentsScreen` regardless of
`MusicDownload` setting — with unified naming on (the default), silent auto-extraction
would cause filename conflicts.

---

## New Settings

Two fields added to `settings.Config`, mirroring the existing `ROMSelection` / `ROMLocation` pattern:

```go
MusicDownload string `json:"music_download,omitempty"` // "auto" | "ask" | "off"
MusicLocation string `json:"music_location,omitempty"` // "auto" | "ask"
```

**Defaults:** `MusicDownload = "off"`, `MusicLocation = "auto"`.

`MusicDownload = "off"` because NextUI does not officially support music players; the
feature is opt-in.

| MusicDownload | Behaviour |
|---|---|
| `"off"` | Music files in ZIP are ignored; only ROMs are extracted |
| `"auto"` | Music extracted automatically to default Music folder |
| `"ask"` | `ZIPContentsScreen` shown; user chooses what to download |

| MusicLocation | Behaviour |
|---|---|
| `"auto"` | Music saved to `/mnt/SDCARD/Music/<sanitised game title>/` |
| `"ask"` | `MusicLocationPickerScreen` shown before download begins |

---

## Data Model Changes

### `inventory.DownloadedFile`

One new field:

```go
type DownloadedFile struct {
    Filename     string    `json:"filename"`
    DestPath     string    `json:"dest_path"`
    DownloadedAt time.Time `json:"downloaded_at"`
    UnifiedName  bool      `json:"unified_name,omitempty"`
    FileType     string    `json:"file_type,omitempty"` // "rom" | "music"; empty == "rom"
}
```

`omitempty` ensures existing inventory JSON files deserialise with `FileType == ""`, which
is treated as `"rom"` everywhere in code. No migration step required.

Constants:
```go
const (
    FileTypeROM   = "rom"
    FileTypeMusic = "music"
)
```

### `internal/roms/zip_classify.go` (new file)

```go
type FileKind int

const (
    KindOther FileKind = iota
    KindROM            // .gb / .gbc / .gba
    KindMusic          // .mp3 .ogg .flac .wav .opus .mod .xm .s3m .it
)

type ZIPEntry struct {
    Name string
    Kind FileKind
    Size uint64 // uncompressed bytes
}

type ZIPManifest struct {
    Entries []ZIPEntry
}

// Helpers: HasROMs, HasMusic, ROMCount, MusicCount, IsSingleROMOnly
// ROMsByExt returns a map[string][]ZIPEntry grouping ROMs by extension.
```

---

## Architecture: New & Changed Files

### New — `internal/roms/`

| File | Purpose |
|---|---|
| `zip_classify.go` | `FileKind`, `ZIPEntry`, `ZIPManifest`, classification helpers |
| `zip_remote.go` | `InspectRemoteZIP` — HTTP Range-based central directory reader; fallback to full download |

### New — `internal/ui/`

| File | Purpose |
|---|---|
| `screen_zip_inspect.go` | Async transitional screen ("Inspecting ZIP…"); routes based on manifest + settings |
| `screen_zip_contents.go` | User-facing contents summary; version picker when multiple ROMs of same ext present |
| `screen_zip_download.go` | Downloads ZIP to temp; extracts; records all files in inventory |
| `screen_music_location_picker.go` | Location picker for music folder (parallel to `screen_location_picker.go`) |

### Changed

| File | Change |
|---|---|
| `internal/settings/settings.go` | Add `MusicDownload`, `MusicLocation`; update `defaults()` |
| `internal/inventory/inventory.go` | Add `FileType` to `DownloadedFile`; add constants |
| `internal/itchio/download.go` | Split `DownloadFree` into `ResolveFreeURL` + stream step; split `DownloadAuthUpload` into `ResolveAuthURL` + stream step |
| `internal/ui/screen_fetch_uploads.go` | Route ZIP uploads → `ZIPInspectScreen` in `nextScreen()` |
| `internal/ui/screen_rom_picker.go` | Route ZIP uploads → `ZIPInspectScreen` in `chooseUpload()` |
| `internal/ui/screen_settings.go` | Add Music Download + Music Location settings UI |
| `internal/ui/screen_manage_downloads.go` | FileType-aware deletion: "Delete ROM" / "Delete Soundtrack" / "Delete All" |
| `internal/roms/roms.go` | Add `MusicDestinationDir(gameTitle string) string` |

---

## Screen Flow

```
FetchUploadsScreen / ROMPickerScreen
        │
        │  ZIP upload selected
        ▼
ZIPInspectScreen          ← async: resolve CDN URL → HEAD + Range (or full temp-download fallback)
        │
        ├─ 1 ROM, no music ──────────────────► DownloadScreen  (unchanged; ZIP kept)
        │
        ├─ multiple ROMs same ext ───────────► ZIPContentsScreen  (version picker)
        │                                              │
        ├─ MusicDownload="ask" ──────────────► ZIPContentsScreen  (ROM/music choices)
        │                                              │
        │  ◄────────────────────────────────────────── │ user confirmed
        │
        ├─ MusicLocation="ask" (music will be downloaded)
        │         └──────────────────────────► MusicLocationPickerScreen
        │                                              │
        │  ◄────────────────────────────────────────── │ location confirmed
        │
        ▼
ZIPDownloadScreen         ← streams ZIP to temp; extracts; records inventory; shows summary
```

---

## ZIP Inspection Mechanics (`zip_remote.go`)

**CDN URL resolution** — before Range requests can be issued, `ZIPInspectScreen`'s goroutine
resolves the actual CDN URL via new helpers split out of the existing download functions:

- `client.ResolveFreeURL(upload Upload) (string, error)` — performs the CSRF/key POST and
  returns the CDN URL without streaming. Extracted from the current `DownloadFree`.
- `client.ResolveAuthURL(apiKey, uploadID, downloadKeyID string) (string, error)` — performs
  the API resolve GET and returns the CDN URL. Extracted from `DownloadAuthUpload`.

The existing `DownloadFree` and `DownloadAuthUpload` are updated to call these helpers
internally, keeping their public signatures unchanged.

The resolved CDN URL is stored in `ZIPPlan` and reused by `ZIPDownloadScreen` — the resolver
endpoint is not called a second time.

**Central directory reading** — ZIP central directory lives at the end of the file. Reading
it needs at most three HTTP requests:

1. **HEAD** — get `Content-Length`; confirm `Accept-Ranges: bytes`
2. **Range: last 65,558 bytes** — covers EOCD (22 bytes + up to 65,535-byte comment) and
   usually the full central directory for any typical game ZIP
3. **Optional Range** — only if the CD offset lies outside the already-fetched window
   (unusually large ZIPs with huge central directories)

Go's `archive/zip.NewReader(r io.ReaderAt, size int64)` drives steps 3–5 internally.
A `rangeReaderAt` struct implementing `io.ReaderAt` issues `GET` with `Range:` headers
and caches fetched chunks to avoid redundant requests.

**Fallback** — triggered when:
- Server omits `Accept-Ranges` header
- Range response is `200` (not `206`)
- Any Range request errors

Falls back to `streamToFile(url, tempPath)` using the existing helper, then
`zip.OpenReader(tempPath)` locally. Temp file cleaned up after extraction regardless.

---

## Extraction Logic (`ZIPDownloadScreen`)

**Phase 1 — Download**  
ZIP streamed to `os.CreateTemp("", "itchio-zip-*.zip")`. `defer os.Remove(tempPath)`
registered immediately. Progress bar shows download bytes.

**Phase 2 — Extract**  
Screen shows "Extracting…". For each ZIP entry (only entries selected by the user in
`ZIPContentsScreen` are processed; entries the user declined are skipped):

```
KindROM   → roms.DestinationDir(ext) + roms.SanitiseFilename(entry.Name)
              apply unified naming if cfg.UnifiedNaming
              os.MkdirAll(dir); extract bytes

KindMusic → musicDir + roms.SanitiseFilename(entry.Name)
              musicDir: cfg.MusicLocation="auto" → roms.MusicDestinationDir(gameTitle)
                        cfg.MusicLocation="ask"  → user-picked path
              os.MkdirAll(musicDir); extract bytes
              (skipped entirely when cfg.MusicDownload="off")

KindOther → skip
```

`roms.SanitiseFilename` (existing, in `sanitise.go`) is used for all extracted filenames.
`roms.MusicDestinationDir(gameTitle)` calls `roms.SanitiseFilename` internally to produce
a safe directory name from the game title.

`/mnt/SDCARD/Music/` and the game subfolder are created via `os.MkdirAll` — no
pre-existence check needed. If creation fails, music extraction is skipped and the
summary screen notes the failure; ROM extraction continues.

**Inventory** — one `DownloadedFile` per successfully extracted file:
- `Filename` is always set to the **original ZIP upload filename** so update detection
  (which compares `KnownUpstreamFiles` by upload filename) continues to work correctly
- `FileType` is `"rom"` or `"music"` accordingly

---

## Deletion (`screen_manage_downloads.go`)

When a game has mixed `FileType` entries, offer three options:

- **Delete ROM** — removes ROM-typed files + cover art; music files and entries kept
- **Delete Soundtrack** — removes music-typed files and entries; ROM kept
- **Delete All** — current behaviour; removes everything

When only one type is present, show a single **Delete** option as today.

---

## Inventory Cleanup (`VerifyAndClean`)

No code changes needed. `VerifyAndClean` already calls `os.Stat(f.DestPath)` for every
`DownloadedFile` regardless of `FileType`. Music files deleted from the SD card (e.g.
the user deletes the Music subfolder) are pruned automatically on the next run.

Resulting states:

| Condition | Outcome |
|---|---|
| Music files deleted, ROM present | Music entries pruned; game stays in inventory |
| ROM deleted, music present | ROM entry pruned; game stays in inventory |
| All files deleted | All entries pruned; game removed from inventory |
| Music subfolder deleted wholesale | All affected entries pruned on next `VerifyAndClean` |

---

## Error Handling

| Failure | Behaviour |
|---|---|
| Range not supported | Silent fallback to full temp-download + local inspect |
| Full-download fallback fails | `ZIPInspectScreen` shows error; B/A → Back |
| Manifest contains nothing recognised | Falls through to `NeedsFormat` path |
| `os.MkdirAll` fails during extraction | Abort; show error; temp file cleaned up |
| Individual entry extraction fails | Log warning; skip entry; continue; summary notes skipped files |
| All extractions fail | Show error screen |
| Music folder creation fails | Music extraction skipped; ROM extraction continues; summary notes failure |

---

## Testing

### `internal/roms/zip_classify.go`
- Table-driven tests for `ClassifyEntry`: each music extension, each ROM extension, unknown extensions, case-insensitivity

### `internal/roms/zip_remote.go`
- `InspectRemoteZIP` against `httptest.Server` serving a test ZIP containing a ROM + music file + text file
- Manifest correctly classifies all three kinds
- Fallback path: server returns `200` instead of `206` → fallback triggers; manifest still correct
- Server returns no `Accept-Ranges` header → fallback triggers

### `internal/roms/zip_classify.go` — `ZIPManifest` helpers
- `HasROMs`, `HasMusic`, `ROMCount`, `MusicCount`, `IsSingleROMOnly` unit tests

### `internal/inventory/inventory.go`
- `DownloadedFile` with `FileType="music"` round-trips through JSON
- `DownloadedFile` with absent `file_type` field deserialises as `""` (treated as ROM)
- `VerifyAndClean` with mixed entries: missing music file pruned, ROM entry kept; missing ROM pruned, music entry kept; both missing, entry removed

### `internal/settings/settings.go`
- `MusicDownload` and `MusicLocation` default correctly when absent from JSON (backward compat with existing config files)

### UI screens
SDL-gated; not unit-tested — consistent with all other screens in the project.
