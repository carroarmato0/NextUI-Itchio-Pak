# Extensionless ROM Format Picker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a paid game's uploads have no `.gb`/`.gbc` extension, present those files to the user with a GB / GBC / ZIP format selector instead of showing "no downloadable files found."

**Architecture:** Add a `NeedsFormat bool` flag to both upload structs; classify each upload in the fetch layer (known ROM / skippable / unknown); in the UI, route to a new `FormatPickerScreen` when no known ROMs were returned. The chosen extension is appended to the filename before passing to the existing `LocationPickerScreen` / `DownloadScreen`, so all downstream logic (destination routing, cover-art, `LastROMDirs`) works unchanged.

**Tech Stack:** Go, SDL2 (UI screens only), itch.io public + Butler APIs.

---

## File Map

| File | Change |
|------|--------|
| `internal/itchio/game.go` | Add `NeedsFormat bool` to `Upload` struct; update `ParseDownloadPage` three-way filter |
| `internal/itchio/download.go` | Add `knownNonROMExts` map + `isSkippableExt` helper |
| `internal/itchio/download_auth.go` | Update `FetchAuthUploads` three-way filter; update summary log |
| `internal/roms/roms.go` | Add `NeedsFormat bool` to `Upload`; add `.zip` case to `DestinationDir` |
| `internal/ui/screen_fetch_uploads.go` | Copy `NeedsFormat` when building `roms.Upload`; update `nextScreen()` routing |
| `internal/ui/screen_format_picker.go` | **New** — format picker screen |
| `internal/itchio/game_test.go` | Add `TestParseDownloadPage_UnknownExt` |
| `internal/itchio/download_test.go` | Add `TestFetchAuthUploads_UnknownExt` |
| `internal/roms/roms_test.go` | Extend `TestDestinationDir` table with `.zip` cases |

---

## Task 1 — Add `NeedsFormat` to Upload structs

**Files:**
- Modify: `internal/itchio/game.go`
- Modify: `internal/roms/roms.go`

No logic changes in this task — struct additions only.

- [ ] **Step 1: Add `NeedsFormat bool` to `itchio.Upload`**

In `internal/itchio/game.go`, update the `Upload` struct:

```go
type Upload struct {
	Filename    string
	URL         string   // resolver or CDN URL
	UploadID    string   // itch.io upload ID (from data-upload_id)
	NeedsFormat bool     // true if extension unknown; user must choose GB, GBC, or ZIP
}
```

- [ ] **Step 2: Add `NeedsFormat bool` to `roms.Upload`**

In `internal/roms/roms.go`, update the `Upload` struct:

```go
type Upload struct {
	Filename      string
	URL           string
	UploadID      string // itch.io upload ID (API-based paid download)
	DownloadKeyID string // itch.io download key ID (API-based paid download)
	NeedsFormat   bool   // true if user must choose the format (GB, GBC, or ZIP)
}
```

- [ ] **Step 3: Verify compile**

```bash
go build -tags headless ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/itchio/game.go internal/roms/roms.go
git commit -m "feat: add NeedsFormat field to itchio.Upload and roms.Upload"
```

---

## Task 2 — `isSkippableExt` helper + `ParseDownloadPage` three-way filter

**Files:**
- Modify: `internal/itchio/download.go` (add helper)
- Modify: `internal/itchio/game.go` (update `ParseDownloadPage`)
- Modify: `internal/itchio/game_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Add to `internal/itchio/game_test.go`:

```go
func TestParseDownloadPage_UnknownExt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><meta name="csrf_token" value="CSRF"/></head>
<body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="game.gbc">game.gbc</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="1">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="Glory Hunters 2.0">Glory Hunters 2.0</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="2">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="manual.pdf">manual.pdf</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="3">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	result, err := c.ParseDownloadPage(srv.URL + "/dl/TOKEN")
	if err != nil {
		t.Fatalf("ParseDownloadPage: %v", err)
	}
	// manual.pdf must be dropped; game.gbc and Glory Hunters 2.0 must be kept
	if len(result.Uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(result.Uploads))
	}

	gbc := result.Uploads[0]
	if gbc.Filename != "game.gbc" {
		t.Errorf("uploads[0].Filename = %q, want game.gbc", gbc.Filename)
	}
	if gbc.NeedsFormat {
		t.Errorf("uploads[0].NeedsFormat = true, want false for .gbc file")
	}

	unknown := result.Uploads[1]
	if unknown.Filename != "Glory Hunters 2.0" {
		t.Errorf("uploads[1].Filename = %q, want 'Glory Hunters 2.0'", unknown.Filename)
	}
	if !unknown.NeedsFormat {
		t.Errorf("uploads[1].NeedsFormat = false, want true for unknown ext")
	}

	for _, u := range result.Uploads {
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped (skippable ext)")
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/itchio/... -run TestParseDownloadPage_UnknownExt -v
```

Expected: FAIL — test panics or `len(result.Uploads) != 2` (currently only .gbc is returned, not the unknown-ext file).

- [ ] **Step 3: Add `isSkippableExt` to `download.go`**

Add the following before the `presentAbsent` function in `internal/itchio/download.go`:

```go
// knownNonROMExts lists extensions that are definitely not GB/GBC ROM files.
// Uploads with these extensions are silently dropped when scanning a game's
// upload list. Anything not in this map (including no extension, version-number
// suffixes like ".0", and ".zip") is returned with NeedsFormat=true so the
// user can classify it manually.
var knownNonROMExts = map[string]bool{
	".7z": true, ".tar": true, ".gz": true, ".rar": true, ".bz2": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
	".mp3": true, ".ogg": true, ".wav": true, ".flac": true, ".aac": true,
	".pdf": true, ".txt": true, ".md": true, ".epub": true, ".mobi": true,
	".mp4": true, ".avi": true, ".mkv": true, ".mov": true,
	".exe": true, ".dmg": true, ".apk": true,
	".pocket": true, ".nes": true, ".gba": true, ".nds": true, ".sfc": true, ".smc": true,
}

func isSkippableExt(ext string) bool {
	return knownNonROMExts[strings.ToLower(ext)]
}
```

- [ ] **Step 4: Update the classification block in `ParseDownloadPage`**

In `internal/itchio/game.go`, replace the existing upload-classification block inside `walkDoc`:

```go
// old block to replace:
ext := strings.ToLower(filepath.Ext(u.Filename))
if ext == ".gb" || ext == ".gbc" {
    logger.Debug("download-page: found upload %s id=%s", u.Filename, u.UploadID)
    result.Uploads = append(result.Uploads, u)
} else {
    logger.Debug("download-page: skipping %s (not .gb/.gbc)", u.Filename)
}
```

Replace with:

```go
ext := strings.ToLower(filepath.Ext(u.Filename))
if ext == ".gb" || ext == ".gbc" {
    logger.Debug("download-page: found ROM %s id=%s", u.Filename, u.UploadID)
    result.Uploads = append(result.Uploads, u)
} else if !isSkippableExt(ext) {
    u.NeedsFormat = true
    logger.Debug("download-page: found unknown-format %s id=%s (user will choose format)", u.Filename, u.UploadID)
    result.Uploads = append(result.Uploads, u)
} else {
    logger.Debug("download-page: skipping %s (ext=%q)", u.Filename, ext)
}
```

Also update the summary log at the end of `ParseDownloadPage` (just before `return result, nil`):

```go
// replace:
logger.Info("download-page: %d uploads available", len(result.Uploads))

// with:
knownCount := 0
for _, u := range result.Uploads {
    if !u.NeedsFormat {
        knownCount++
    }
}
logger.Info("download-page: %d known ROM(s), %d unknown-format file(s)",
    knownCount, len(result.Uploads)-knownCount)
```

- [ ] **Step 5: Run test to confirm it passes**

```bash
go test ./internal/itchio/... -run TestParseDownloadPage_UnknownExt -v
```

Expected: PASS.

- [ ] **Step 6: Run full test suite**

```bash
go test ./internal/itchio/...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/itchio/download.go internal/itchio/game.go internal/itchio/game_test.go
git commit -m "feat: classify extensionless uploads as NeedsFormat in ParseDownloadPage"
```

---

## Task 3 — Three-way filter in `FetchAuthUploads`

**Files:**
- Modify: `internal/itchio/download_auth.go`
- Modify: `internal/itchio/download_test.go` (new test)

- [ ] **Step 1: Write the failing test**

Add to `internal/itchio/download_test.go`:

```go
func TestFetchAuthUploads_UnknownExt(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testkey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[{"id":42}]}`))
	})

	mux.HandleFunc("/api/1/testkey/game/777/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download_key_id") != "42" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uploads":[
			{"id":1,"filename":"game.gbc"},
			{"id":2,"filename":"manual.pdf"},
			{"id":3,"filename":"Glory Hunters 2.0"}
		]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("testkey", "777")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "42" {
		t.Errorf("keyID = %q, want 42", keyID)
	}
	// manual.pdf dropped; game.gbc (known) + Glory Hunters 2.0 (unknown) kept
	if len(uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(uploads))
	}

	gbc := uploads[0]
	if gbc.Filename != "game.gbc" {
		t.Errorf("uploads[0].Filename = %q, want game.gbc", gbc.Filename)
	}
	if gbc.NeedsFormat {
		t.Errorf("uploads[0].NeedsFormat = true, want false for .gbc file")
	}

	unknown := uploads[1]
	if unknown.Filename != "Glory Hunters 2.0" {
		t.Errorf("uploads[1].Filename = %q, want 'Glory Hunters 2.0'", unknown.Filename)
	}
	if !unknown.NeedsFormat {
		t.Errorf("uploads[1].NeedsFormat = false, want true for unknown ext")
	}
	if unknown.UploadID != "3" {
		t.Errorf("uploads[1].UploadID = %q, want 3", unknown.UploadID)
	}

	for _, u := range uploads {
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped (skippable ext)")
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/itchio/... -run TestFetchAuthUploads_UnknownExt -v
```

Expected: FAIL — `len(uploads) != 2` because the current code drops "Glory Hunters 2.0".

- [ ] **Step 3: Replace the upload-classification loop in `FetchAuthUploads`**

In `internal/itchio/download_auth.go`, replace the block that starts with `var uploads []Upload` through the existing `logger.Warn` at the end:

```go
var uploads []Upload
for _, u := range uploadsResult.Uploads {
	ext := strings.ToLower(filepath.Ext(u.Filename))
	if ext == ".gb" || ext == ".gbc" {
		uploads = append(uploads, Upload{
			Filename: u.Filename,
			UploadID: fmt.Sprintf("%d", u.ID),
		})
		logger.Debug("auth: found ROM %s id=%d", u.Filename, u.ID)
	} else if !isSkippableExt(ext) {
		uploads = append(uploads, Upload{
			Filename:    u.Filename,
			UploadID:    fmt.Sprintf("%d", u.ID),
			NeedsFormat: true,
		})
		logger.Debug("auth: found unknown-format %s id=%d (user will choose format)", u.Filename, u.ID)
	} else {
		logger.Debug("auth: skipping %s (ext=%q)", u.Filename, ext)
	}
}
knownCount := 0
for _, u := range uploads {
	if !u.NeedsFormat {
		knownCount++
	}
}
logger.Debug("auth: %d known ROM(s), %d unknown-format file(s) from %d total",
	knownCount, len(uploads)-knownCount, len(uploadsResult.Uploads))
if len(uploads) == 0 {
	logger.Warn("auth: no downloadable uploads found (game has %d uploads, all skipped)",
		len(uploadsResult.Uploads))
}
return uploads, downloadKeyID, nil
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/itchio/... -run TestFetchAuthUploads_UnknownExt -v
```

Expected: PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./internal/itchio/...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/download_auth.go internal/itchio/download_test.go
git commit -m "feat: classify extensionless uploads as NeedsFormat in FetchAuthUploads"
```

---

## Task 4 — Add `.zip` to `DestinationDir`

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/roms_test.go`

- [ ] **Step 1: Add `.zip` cases to the existing `TestDestinationDir` table**

In `internal/roms/roms_test.go`, extend the `tests` slice inside `TestDestinationDir`:

```go
// add these two entries to the existing table:
{".zip", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
{".ZIP", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/roms/... -run TestDestinationDir -v
```

Expected: FAIL — `DestinationDir(".zip")` currently returns `""`.

- [ ] **Step 3: Add `.zip` case to `DestinationDir`**

In `internal/roms/roms.go`, update the function:

```go
func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	case ".zip":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/roms/... -run TestDestinationDir -v
```

Expected: PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./internal/roms/...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/roms/roms.go internal/roms/roms_test.go
git commit -m "feat: add .zip case to DestinationDir (defaults to GBC folder)"
```

---

## Task 5 — Create `screen_format_picker.go`

**Files:**
- Create: `internal/ui/screen_format_picker.go`

- [ ] **Step 1: Create the file**

Create `internal/ui/screen_format_picker.go` with the following content:

```go
//go:build !headless

package ui

import (
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type formatChoice int

const (
	formatGB  formatChoice = iota // .gb
	formatGBC                     // .gbc
	formatZIP                     // .zip
)

func (f formatChoice) ext() string {
	switch f {
	case formatGB:
		return ".gb"
	case formatGBC:
		return ".gbc"
	case formatZIP:
		return ".zip"
	}
	return ".gbc"
}

func (f formatChoice) label() string {
	switch f {
	case formatGB:
		return "[GB]"
	case formatGBC:
		return "[GBC]"
	case formatZIP:
		return "[ZIP]"
	}
	return "[GBC]"
}

func (f formatChoice) next() formatChoice { return (f + 1) % 3 }
func (f formatChoice) prev() formatChoice { return (f + 2) % 3 }

// defaultFormatChoice returns ZIP when the filename already ends in .zip,
// GBC for everything else (most GB Studio games target Game Boy Color).
func defaultFormatChoice(filename string) formatChoice {
	if strings.ToLower(filepath.Ext(filename)) == ".zip" {
		return formatZIP
	}
	return formatGBC
}

// FormatPickerScreen is shown when a game's uploads have no recognized .gb/.gbc
// extension. The user selects a file and chooses GB, GBC, or ZIP before the
// download proceeds. The chosen extension is appended to the filename so all
// downstream routing (DestinationDir, LastROMDirs, cover-art) works correctly.
type FormatPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cfgPath string
	game    itchio.Game
	detail  *itchio.GameDetail
	uploads []roms.Upload
	formats []formatChoice // parallel to uploads
	cursor  int
	prev    Screen
}

func NewFormatPickerScreen(
	client *itchio.Client, cfg *settings.Config, cfgPath string,
	game itchio.Game, detail *itchio.GameDetail,
	uploads []roms.Upload, prev Screen,
) *FormatPickerScreen {
	formats := make([]formatChoice, len(uploads))
	for i, u := range uploads {
		formats[i] = defaultFormatChoice(u.Filename)
	}
	return &FormatPickerScreen{
		client: client, cfg: cfg, cfgPath: cfgPath,
		game: game, detail: detail,
		uploads: uploads, formats: formats,
		prev: prev,
	}
}

func (s *FormatPickerScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	footerH := int32(40)
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	headerH := fontH + smallFH + 16

	r.DrawRect(0, 0, r.W, headerH, 30, 30, 30)
	r.DrawRect(0, headerH, r.W, 2, 50, 50, 50)
	title := truncateToWidth(r, s.game.Title, r.W-24)
	r.DrawText(title, 12, 8, colorText, colorText, colorText)
	r.DrawSmallText("by "+s.game.Author, 12, 8+fontH+4, 140, 140, 140)

	contentTop := headerH + 8
	r.DrawSmallText("No .gb/.gbc detected — choose file and format:", 12, contentTop, 180, 160, 100)
	contentTop += smallFH + 10

	rowH := fontH + 20
	tagW := int32(52)

	for i, u := range s.uploads {
		y := contentTop + int32(i)*rowH
		if i == s.cursor {
			r.DrawRect(0, y-4, r.W, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}
		name := truncateToWidth(r, u.Filename, r.W-tagW-32)
		r.DrawText(name, 20, y, colorText, colorText, colorText)

		tagX := r.W - tagW - 8
		f := s.formats[i]
		switch f {
		case formatGB:
			r.DrawSmallText(f.label(), tagX, y+4, 120, 220, 120)
		case formatGBC:
			r.DrawSmallText(f.label(), tagX, y+4, 80, 180, 255)
		case formatZIP:
			r.DrawSmallText(f.label(), tagX, y+4, 220, 180, 80)
		}
	}

	ftrY := r.DrawFooterBar(footerH)
	r.DrawSmallText("↑↓: select  ◄►: GB/GBC/ZIP  B: download  A: back", 10, ftrY, 140, 140, 140)
	r.Present()
}

func (s *FormatPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.K_LEFT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].prev()
			}
		case sdl.K_RIGHT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].next()
			}
		case sdl.K_RETURN:
			return s.confirm()
		case sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		switch ev.Button {
		case sdl.CONTROLLER_BUTTON_DPAD_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].prev()
			}
		case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
			if s.cursor < len(s.formats) {
				s.formats[s.cursor] = s.formats[s.cursor].next()
			}
		case sdl.CONTROLLER_BUTTON_B:
			return s.confirm()
		case sdl.CONTROLLER_BUTTON_A:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *FormatPickerScreen) confirm() Screen {
	if s.cursor >= len(s.uploads) {
		return s
	}
	original := s.uploads[s.cursor]
	chosenExt := s.formats[s.cursor].ext()

	upload := original
	// Only append the extension if the file does not already carry it,
	// to avoid producing names like "game.zip.zip".
	if strings.ToLower(filepath.Ext(original.Filename)) != chosenExt {
		upload.Filename = original.Filename + chosenExt
	}
	logger.Info("format-picker: %q → %s", original.Filename,
		strings.ToUpper(strings.TrimPrefix(chosenExt, ".")))

	if s.cfg.ROMLocation == "ask" {
		return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.prev)
	}
	dest := roms.DestinationDir(chosenExt) + upload.Filename
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
}
```

- [ ] **Step 2: Compile check (headless — excludes the new file)**

```bash
go build -tags headless ./...
```

Expected: no errors from non-UI packages.

- [ ] **Step 3: Run all non-UI tests**

```bash
go test ./internal/itchio/... ./internal/roms/...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_format_picker.go
git commit -m "feat: add FormatPickerScreen for extensionless ROM uploads"
```

---

## Task 6 — Update `screen_fetch_uploads.go` routing

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`

- [ ] **Step 1: Copy `NeedsFormat` in the auth-path goroutine**

In `internal/ui/screen_fetch_uploads.go`, inside the goroutine, update the auth-path upload conversion loop:

```go
// replace:
for _, u := range authUploads {
    s.uploads = append(s.uploads, roms.Upload{
        Filename:      u.Filename,
        UploadID:      u.UploadID,
        DownloadKeyID: downloadKeyID,
    })
}
if len(s.uploads) == 0 {
    logger.Warn("fetch: no .gb/.gbc uploads found for game (auth path)")
    s.err = fmt.Errorf("no .gb or .gbc files found for this game")
    s.state = fetchError
} else {
    s.state = fetchDone
}

// with:
for _, u := range authUploads {
    s.uploads = append(s.uploads, roms.Upload{
        Filename:      u.Filename,
        UploadID:      u.UploadID,
        DownloadKeyID: downloadKeyID,
        NeedsFormat:   u.NeedsFormat,
    })
}
if len(s.uploads) == 0 {
    logger.Warn("fetch: no downloadable uploads found for game (auth path)")
    s.err = fmt.Errorf("no downloadable files found for this game")
    s.state = fetchError
} else {
    s.state = fetchDone
}
```

- [ ] **Step 2: Copy `NeedsFormat` in the free-path goroutine**

In the same goroutine, update the free-path upload conversion loop:

```go
// replace:
for _, u := range itchUploads {
    s.uploads = append(s.uploads, roms.Upload{Filename: u.Filename, URL: u.URL})
}
if len(s.uploads) == 0 {
    logger.Warn("fetch: no .gb/.gbc uploads found for game (free path)")
    s.err = fmt.Errorf("no .gb or .gbc files found for this game")
    s.state = fetchError
} else {
    s.state = fetchDone
}

// with:
for _, u := range itchUploads {
    s.uploads = append(s.uploads, roms.Upload{
        Filename:    u.Filename,
        URL:         u.URL,
        NeedsFormat: u.NeedsFormat,
    })
}
if len(s.uploads) == 0 {
    logger.Warn("fetch: no downloadable uploads found for game (free path)")
    s.err = fmt.Errorf("no downloadable files found for this game")
    s.state = fetchError
} else {
    s.state = fetchDone
}
```

- [ ] **Step 3: Replace `nextScreen()` with routing-aware version**

Replace the entire `nextScreen()` method:

```go
func (s *FetchUploadsScreen) nextScreen() Screen {
	var known, unknown []roms.Upload
	for _, u := range s.uploads {
		if u.NeedsFormat {
			unknown = append(unknown, u)
		} else {
			known = append(known, u)
		}
	}

	if len(known) > 0 {
		if len(known) == 1 {
			upload := known[0]
			if s.cfg.ROMLocation == "ask" {
				return NewLocationPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, upload, s.prev)
			}
			ext := strings.ToLower(filepath.Ext(upload.Filename))
			dest := roms.DestinationDir(ext) + upload.Filename
			return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, upload, dest, s.prev)
		}
		return NewROMPickerScreen(s.client, s.cfg, s.cfgPath, s.cache, s.game, s.detail, known, s.prev)
	}
	return NewFormatPickerScreen(s.client, s.cfg, s.cfgPath, s.game, s.detail, unknown, s.prev)
}
```

- [ ] **Step 4: Compile check**

```bash
go build -tags headless ./...
```

Expected: no errors. (The UI file has `//go:build !headless` so it is excluded from this check — errors in it will surface in step 5.)

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go
git commit -m "feat: route to FormatPickerScreen when only unknown-format uploads exist"
```

---

## Task 7 — Self-review and final verification

- [ ] **Step 1: Run the complete test suite**

```bash
go test ./...
```

Expected: all tests pass (UI packages excluded if SDL2 headers unavailable; use `-tags headless` to skip them: `go test -tags headless ./...`).

- [ ] **Step 2: Spec coverage check**

Verify each spec requirement has been implemented:

| Spec requirement | Implemented in |
|-----------------|----------------|
| `NeedsFormat bool` on both Upload structs | Task 1 |
| `isSkippableExt` — `.zip` NOT skipped | Task 2 (download.go) |
| `ParseDownloadPage` three-way filter | Task 2 (game.go) |
| `FetchAuthUploads` three-way filter | Task 3 |
| `DestinationDir(".zip")` = GBC folder | Task 4 |
| `LastROMDirs[".zip"]` remembered via LocationPickerScreen | Handled automatically — LocationPickerScreen keys on `filepath.Ext(upload.Filename)` which will be `.zip` |
| FormatPickerScreen: ↑↓ navigate, ◄► cycle GB/GBC/ZIP | Task 5 |
| Default: ZIP if original ext is .zip, GBC otherwise | Task 5 (`defaultFormatChoice`) |
| B: confirm → append ext (no double-append) → LocationPicker or DownloadScreen | Task 5 (`confirm()`) |
| A: back | Task 5 |
| `screen_fetch_uploads` routes to FormatPickerScreen when no known ROMs | Task 6 |

- [ ] **Step 3: Final commit if any fixups were made**

```bash
git add -p
git commit -m "fix: format picker follow-up corrections"
```
