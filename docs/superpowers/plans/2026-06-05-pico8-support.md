# Pico-8 Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Pico-8 (`.p8` / `.p8.png`) game support to the Itch.io NextUI Pak, including feed discovery, direct download, ZIP extraction, multi-cart M3U generation, and cover art handling.

**Architecture:** A `ROMExt()` helper in `internal/roms` intercepts the `.p8.png` compound extension before it reaches the eight `filepath.Ext()` call sites that would otherwise misidentify it as a plain PNG. Multi-cart ZIPs (two or more Pico-8 files) are extracted to a named subdirectory with an auto-generated M3U playlist; the `.p8.png` format doubles as cover art so no extra network call is needed.

**Tech Stack:** Go 1.22+, SDL2 (UI screens have `//go:build !headless`), `./scripts/test.sh` runs all tests in a container.

---

## File Map

| File | What changes |
|------|-------------|
| `internal/roms/roms.go` | Add `ROMExt()`, `Pico8Dir`, `Pico8MultiCartDir()`, update `ScoreUpload`, `DestinationDir` |
| `internal/roms/sanitise.go` | Fix `ResolveUnifiedDest` to use `ROMExt` (else `.p8.png` renamed to `.png`) |
| `internal/roms/roms_test.go` | `TestROMExt`, extend `TestScoreUpload`/`TestDestinationDir` |
| `internal/roms/zip_classify.go` | Add `.p8`/`.p8.png` to `romExts`, fix `ClassifyEntry`, add `IsPico8MultiCart()` |
| `internal/roms/zip_classify_test.go` | Extend `TestClassifyEntry`, add `TestIsPico8MultiCart` |
| `internal/itchio/platforms.go` | Add P8 feed platform entry |
| `internal/itchio/game.go` | Fix `ParseDownloadPage` to recognise `.p8`/`.p8.png`; import `roms` |
| `internal/itchio/cover_art.go` | Add `CopyCoverArt()` package function |
| `internal/itchio/cover_art_test.go` | Add `TestCopyCoverArt` |
| `internal/inventory/inventory.go` | Add `FileTypeM3U = "m3u"` constant |
| `internal/ui/screen_fetch_uploads.go` | Use `roms.ROMExt` for ext lookup in `nextScreen()` |
| `internal/ui/screen_download.go` | Call `CopyCoverArt` instead of `DownloadCoverArt` for `.p8.png` |
| `internal/ui/screen_multi_download.go` | Call `CopyCoverArt` instead of `DownloadCoverArt` for `.p8.png` |
| `internal/ui/screen_zip_inspect.go` | Add `IsPico8MultiCart bool` to `ZIPPlan`; add multi-cart branch in `route()`; fix inner-ROM ext lookup |
| `internal/ui/screen_zip_download.go` | Fix `extractROM()` ext/stem/art; add `writePico8M3U()`; call it in `run()` |

---

## Task 1 — `ROMExt()` Helper

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/roms_test.go`

- [ ] **Step 1: Write the failing test**

Add to the end of `internal/roms/roms_test.go`:

```go
func TestROMExt(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"game.p8.png", ".p8.png"},
		{"game.P8.PNG", ".p8.png"},
		{"GAME.P8.PNG", ".p8.png"},
		{"game.p8", ".p8"},
		{"game.gbc", ".gbc"},
		{"game.png", ".png"},
		{"game", ""},
	}
	for _, tt := range tests {
		got := roms.ROMExt(tt.filename)
		if got != tt.want {
			t.Errorf("ROMExt(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```
./scripts/test.sh
```
Expected: FAIL — `roms.ROMExt undefined`

- [ ] **Step 3: Add `ROMExt` to `internal/roms/roms.go`**

Add after the existing imports block (no new imports needed — `strings` and `path/filepath` are already imported):

```go
// ROMExt returns the effective ROM extension for filename.
// For Pico-8 cartridges with the compound extension .p8.png it returns ".p8.png"
// rather than the ".png" that filepath.Ext would return.
func ROMExt(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".p8.png") {
		return ".p8.png"
	}
	return filepath.Ext(filename)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/roms/roms.go internal/roms/roms_test.go
git commit -m "feat(roms): add ROMExt helper for .p8.png compound extension"
```

---

## Task 2 — Pico-8 Constants, Scoring, Destination, and `ResolveUnifiedDest` Fix

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/sanitise.go`
- Modify: `internal/roms/roms_test.go`

- [ ] **Step 1: Write failing tests**

Extend `TestScoreUpload` in `internal/roms/roms_test.go` — add these cases to the existing `tests` slice:

```go
{"game.p8.png", 2},
{"game.P8.PNG", 2},
{"game.p8",     1},
{"game.P8",     1},
```

Extend `TestDestinationDir` — add these cases to the existing `tests` slice:

```go
{".p8",     "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
{".P8",     "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
{".p8.png", "/mnt/SDCARD/Roms/Pico-8 (P8)/"},
```

Add a new test after `TestMusicDestinationDir`:

```go
func TestPico8MultiCartDir(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Celeste Classic", "/mnt/SDCARD/Roms/Pico-8 (P8)/Celeste Classic/"},
		{"Game: Title?",    "/mnt/SDCARD/Roms/Pico-8 (P8)/Game Title/"},
		{"",                "/mnt/SDCARD/Roms/Pico-8 (P8)/Unknown/"},
	}
	for _, tt := range tests {
		got := roms.Pico8MultiCartDir(tt.title)
		if got != tt.want {
			t.Errorf("Pico8MultiCartDir(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
./scripts/test.sh
```
Expected: FAIL — `Pico8MultiCartDir undefined`, score/dest mismatches

- [ ] **Step 3: Update `internal/roms/roms.go`**

Add constant after the existing `GBADir` block:

```go
// Pico8Dir is the NextUI Pico-8 ROM directory.
const Pico8Dir = "/mnt/SDCARD/Roms/Pico-8 (P8)/"
```

Replace `ScoreUpload` entirely:

```go
func ScoreUpload(filename string) int {
	switch strings.ToLower(ROMExt(filename)) {
	case ".gbc", ".p8.png":
		return 2
	case ".gb", ".gba", ".nes", ".md", ".gen", ".smd", ".p8":
		return 1
	default:
		return 0
	}
}
```

Add `.p8`/`.p8.png` cases to `DestinationDir` (before the `.zip` case):

```go
	case ".p8", ".p8.png":
		return Pico8Dir
```

Add `Pico8MultiCartDir` after `MusicDestinationDir`:

```go
// Pico8MultiCartDir returns the subdirectory for a multi-cart Pico-8 game.
// All cart files and the generated M3U playlist are placed here.
func Pico8MultiCartDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8Dir + safe + "/"
}
```

- [ ] **Step 4: Fix `ResolveUnifiedDest` in `internal/roms/sanitise.go`**

Replace the first line of `ResolveUnifiedDest`:

```go
// Before:
ext := filepath.Ext(currentPath)

// After:
ext := ROMExt(filepath.Base(currentPath))
```

The full function signature and body are unchanged — only this one line:

```go
func ResolveUnifiedDest(currentPath, gameTitle string, allowOverwrite bool) (string, bool) {
	ext := ROMExt(filepath.Base(currentPath))
	candidate := SanitiseFilename(gameTitle, ext)
	// ... rest unchanged
```

- [ ] **Step 5: Run tests to confirm they pass**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/roms/roms.go internal/roms/sanitise.go internal/roms/roms_test.go
git commit -m "feat(roms): add Pico-8 constants, scoring, destination, and multi-cart dir"
```

---

## Task 3 — ZIP Classification for Pico-8

**Files:**
- Modify: `internal/roms/zip_classify.go`
- Modify: `internal/roms/zip_classify_test.go`

- [ ] **Step 1: Write failing tests**

Add to `TestClassifyEntry` in `internal/roms/zip_classify_test.go`:

```go
{"game.p8",     roms.KindROM},
{"game.P8",     roms.KindROM},
{"cart.p8.png", roms.KindROM},
{"cart.P8.PNG", roms.KindROM},
// cover.png must remain KindOther (not confused with .p8.png)
{"cover.png",   roms.KindOther},
```

Add a new test after `TestHasNoDuplicateROMExt`:

```go
func TestIsPico8MultiCart(t *testing.T) {
	tests := []struct {
		name    string
		entries []roms.ZIPEntry
		want    bool
	}{
		{
			name: "two p8 files → multi-cart",
			entries: []roms.ZIPEntry{
				{Name: "poom.p8", Kind: roms.KindROM},
				{Name: "poom-2.p8", Kind: roms.KindROM},
			},
			want: true,
		},
		{
			name: "one p8 + one p8.png → multi-cart",
			entries: []roms.ZIPEntry{
				{Name: "game.p8", Kind: roms.KindROM},
				{Name: "game.p8.png", Kind: roms.KindROM},
			},
			want: true,
		},
		{
			name: "one p8 only → not multi-cart",
			entries: []roms.ZIPEntry{
				{Name: "game.p8", Kind: roms.KindROM},
			},
			want: false,
		},
		{
			name: "no pico-8 files → not multi-cart",
			entries: []roms.ZIPEntry{
				{Name: "game.gbc", Kind: roms.KindROM},
				{Name: "game.gb", Kind: roms.KindROM},
			},
			want: false,
		},
		{
			name: "empty manifest → not multi-cart",
			entries: nil,
			want: false,
		},
	}
	for _, tt := range tests {
		m := roms.ZIPManifest{Entries: tt.entries}
		got := m.IsPico8MultiCart()
		if got != tt.want {
			t.Errorf("%s: IsPico8MultiCart() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
./scripts/test.sh
```
Expected: FAIL — `.p8`/`.p8.png` classified as `KindOther`, `IsPico8MultiCart` undefined

- [ ] **Step 3: Update `internal/roms/zip_classify.go`**

Add `.p8` and `.p8.png` to `romExts`:

```go
var romExts = map[string]bool{
	".gb": true, ".gbc": true, ".gba": true,
	".nes": true,
	".md": true, ".gen": true, ".smd": true,
	".p8": true, ".p8.png": true,
}
```

Replace `ClassifyEntry` so it uses `ROMExt` for the extension check:

```go
func ClassifyEntry(name string) FileKind {
	ext := strings.ToLower(ROMExt(name))
	if romExts[ext] {
		return KindROM
	}
	if musicExts[ext] {
		return KindMusic
	}
	return KindOther
}
```

Add `IsPico8MultiCart` after `HasDuplicateROMExt`:

```go
// IsPico8MultiCart reports whether the manifest contains more than one
// Pico-8 cartridge file (.p8 or .p8.png), indicating a multi-cart game.
func (m ZIPManifest) IsPico8MultiCart() bool {
	return len(m.ROMsByExt()[".p8"])+len(m.ROMsByExt()[".p8.png"]) > 1
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/roms/zip_classify.go internal/roms/zip_classify_test.go
git commit -m "feat(roms): classify .p8/.p8.png as ROM, add IsPico8MultiCart"
```

---

## Task 4 — Feed Platform and `ParseDownloadPage` Fix

**Files:**
- Modify: `internal/itchio/platforms.go`
- Modify: `internal/itchio/game.go`

- [ ] **Step 1: Add P8 platform to `internal/itchio/platforms.go`**

Append to the `AllPlatforms` slice (after the `MD` entry):

```go
	{
		Code:      "P8",
		Name:      "Pico-8",
		FeedSlugs: []string{"tag-pico-8"},
	},
```

- [ ] **Step 2: Fix `ParseDownloadPage` in `internal/itchio/game.go`**

Add `"github.com/carroarmato0/nextui-itchio-pak/internal/roms"` to the import block.

Inside `ParseDownloadPage`, in the `walkDoc` function, replace this block:

```go
// Before:
ext := strings.ToLower(filepath.Ext(u.Filename))
if ext == ".gb" || ext == ".gbc" || ext == ".gba" || ext == ".nes" || ext == ".md" || ext == ".gen" || ext == ".smd" || ext == ".zip" {
    logger.Debug("download-page: found ROM %s id=%s", u.Filename, u.UploadID)
    result.Uploads = append(result.Uploads, u)
} else if !isSkippableExt(ext) {
```

```go
// After:
ext := strings.ToLower(roms.ROMExt(u.Filename))
if ext == ".gb" || ext == ".gbc" || ext == ".gba" || ext == ".nes" ||
    ext == ".md" || ext == ".gen" || ext == ".smd" ||
    ext == ".p8" || ext == ".p8.png" || ext == ".zip" {
    logger.Debug("download-page: found ROM %s id=%s", u.Filename, u.UploadID)
    result.Uploads = append(result.Uploads, u)
} else if !isSkippableExt(ext) {
```

- [ ] **Step 3: Run tests to confirm nothing is broken**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/itchio/platforms.go internal/itchio/game.go
git commit -m "feat(itchio): add Pico-8 feed platform and recognise .p8/.p8.png downloads"
```

---

## Task 5 — `CopyCoverArt`, `FileTypeM3U`, and Test

**Files:**
- Modify: `internal/itchio/cover_art.go`
- Modify: `internal/itchio/cover_art_test.go`
- Modify: `internal/inventory/inventory.go`

- [ ] **Step 1: Write failing test**

Add to `internal/itchio/cover_art_test.go` (after the existing tests):

```go
// TestCopyCoverArt verifies that CopyCoverArt copies the ROM file to the
// .media/ sibling directory with the correct art filename.
func TestCopyCoverArt(t *testing.T) {
	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.p8.png")

	// Write a small PNG as the "ROM" (CopyCoverArt treats it as opaque bytes).
	if err := os.WriteFile(romPath, minimalPNG(), 0644); err != nil {
		t.Fatalf("write rom: %v", err)
	}

	if err := itchio.CopyCoverArt(romPath); err != nil {
		t.Fatalf("CopyCoverArt: %v", err)
	}

	// coverArtBasename strips the last ext: "game.p8.png" → stem "game.p8"
	// → art path ".media/game.p8.png"
	artPath := filepath.Join(dir, ".media", "game.p8.png")
	if _, err := os.Stat(artPath); os.IsNotExist(err) {
		t.Fatalf("art file not created at %s", artPath)
	}

	got, err := os.ReadFile(artPath)
	if err != nil {
		t.Fatalf("read art: %v", err)
	}
	if !bytes.Equal(got, minimalPNG()) {
		t.Error("art file content does not match ROM content")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```
./scripts/test.sh
```
Expected: FAIL — `itchio.CopyCoverArt undefined`

- [ ] **Step 3: Add `CopyCoverArt` to `internal/itchio/cover_art.go`**

Add after the `DownloadCoverArt` function. The function needs `io` which is already imported. Add `"io"` to the import block if not present (check — it is not currently imported in cover_art.go; add it).

```go
// CopyCoverArt copies the ROM file at romDestPath into the .media/ directory
// alongside it, using the same art filename that DownloadCoverArt would produce.
// Used for .p8.png cartridges, which are themselves valid PNG images — no
// separate network request is needed.
func CopyCoverArt(romDestPath string) error {
	dir := filepath.Dir(romDestPath)
	mediaDir := filepath.Join(dir, ".media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return fmt.Errorf("cover-art: mkdir: %w", err)
	}

	base := coverArtBasename(romDestPath)
	artPath := filepath.Join(mediaDir, base+".png")

	logger.Info("cover-art: copying .p8.png as art → %s", artPath)

	src, err := os.Open(romDestPath)
	if err != nil {
		return fmt.Errorf("cover-art: open source: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp(mediaDir, ".art-*.tmp")
	if err != nil {
		return fmt.Errorf("cover-art: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		return fmt.Errorf("cover-art: copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cover-art: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, artPath); err != nil {
		return fmt.Errorf("cover-art: rename: %w", err)
	}
	logger.Info("cover-art: saved → %s", artPath)
	return nil
}
```

Add `"io"` to the import block in `cover_art.go`.

- [ ] **Step 4: Add `FileTypeM3U` to `internal/inventory/inventory.go`**

Add after the existing constants (line 17):

```go
const (
	FileTypeROM   = "rom"
	FileTypeMusic = "music"
	FileTypeM3U   = "m3u"
)
```

- [ ] **Step 5: Run tests to confirm they pass**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/cover_art.go internal/itchio/cover_art_test.go internal/inventory/inventory.go
git commit -m "feat: add CopyCoverArt for .p8.png ROMs and FileTypeM3U inventory constant"
```

---

## Task 6 — Single-File Download Screens

**Files:**
- Modify: `internal/ui/screen_fetch_uploads.go`
- Modify: `internal/ui/screen_download.go`
- Modify: `internal/ui/screen_multi_download.go`

These three files have `//go:build !headless` and are excluded from the container test run. Changes are mechanical — replace two call patterns.

**Pattern A** — fix destination ext lookup (use `roms.ROMExt` not `filepath.Ext`):

In `internal/ui/screen_fetch_uploads.go`, inside `nextScreen()`, replace:

```go
// Before (appears twice — both in the single-upload branch and the multi-upload loop):
ext := strings.ToLower(filepath.Ext(upload.Filename))
dest := roms.DestinationDir(ext) + upload.Filename
```

```go
// After:
ext := strings.ToLower(roms.ROMExt(upload.Filename))
dest := roms.DestinationDir(ext) + upload.Filename
```

The first occurrence is at the single-upload auto-path (around line 286). The second is inside the multi-upload loop (around line 307). Replace both.

**Pattern B** — use `CopyCoverArt` for `.p8.png` (appears in `screen_download.go` and `screen_multi_download.go`):

In `internal/ui/screen_download.go`, replace:

```go
if artErr := client.DownloadCoverArt(game.CoverURL, finalDest); artErr != nil {
    logger.Warn("cover-art: game=%q url=%s: %v", game.Title, game.CoverURL, artErr)
}
```

```go
if roms.ROMExt(upload.Filename) == ".p8.png" {
    if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
        logger.Warn("cover-art: game=%q: %v", game.Title, artErr)
    }
} else if artErr := client.DownloadCoverArt(game.CoverURL, finalDest); artErr != nil {
    logger.Warn("cover-art: game=%q url=%s: %v", game.Title, game.CoverURL, artErr)
}
```

In `internal/ui/screen_multi_download.go`, in `runDownloads()`, replace:

```go
if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
    logger.Warn("cover-art: game=%q: %v", s.game.Title, artErr)
}
```

```go
if roms.ROMExt(dl.Upload.Filename) == ".p8.png" {
    if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
        logger.Warn("cover-art: game=%q: %v", s.game.Title, artErr)
    }
} else if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
    logger.Warn("cover-art: game=%q: %v", s.game.Title, artErr)
}
```

`itchio` is already imported in both files — no import changes needed.

- [ ] **Step 1: Apply all three file edits above**

- [ ] **Step 2: Run tests (headless build validates imports)**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_fetch_uploads.go internal/ui/screen_download.go internal/ui/screen_multi_download.go
git commit -m "feat(ui): use ROMExt for Pico-8 destination routing and CopyCoverArt for .p8.png"
```

---

## Task 7 — ZIP Inspect: Multi-Cart Branch and Inner-ROM Ext Fix

**Files:**
- Modify: `internal/ui/screen_zip_inspect.go`

- [ ] **Step 1: Add `IsPico8MultiCart` field to `ZIPPlan`**

In the `ZIPPlan` struct (around line 19), add the new field:

```go
type ZIPPlan struct {
	Upload   roms.Upload
	CDNURL   string
	Manifest roms.ZIPManifest

	DownloadROMs    bool
	DownloadMusic   bool
	IsPico8MultiCart bool // when true, skip per-ROM art; writePico8M3U handles it
	// SelectedROMs maps lowercase extension → chosen entry Name.
	SelectedROMs map[string]string
	// ROMDirs maps lowercase extension → chosen destination directory.
	ROMDirs  map[string]string
	MusicDir string
}
```

- [ ] **Step 2: Fix the inner-ROM ext lookup in `route()`**

In `route()`, in the `IsSingleROMOnly() && !HasOtherFiles()` branch, replace:

```go
// Before:
if inner := strings.ToLower(filepath.Ext(e.Name)); roms.DestinationDir(inner) != "" {
```

```go
// After:
if inner := strings.ToLower(roms.ROMExt(e.Name)); roms.DestinationDir(inner) != "" {
```

- [ ] **Step 3: Add the `IsPico8MultiCart` branch in `route()`**

Insert this block **after** the `IsSingleROMOnly()` block and **before** the `HasDuplicateROMExt()` check:

```go
// Pico-8 multi-cart: extract all carts to a named subdirectory and generate
// an M3U playlist. Checked before HasDuplicateROMExt so it never hits the
// single-selection picker.
if m.IsPico8MultiCart() {
    subDir := roms.Pico8MultiCartDir(s.game.Title)
    plan := s.plan
    plan.DownloadROMs = true
    plan.IsPico8MultiCart = true
    plan.ROMDirs = map[string]string{".p8": subDir, ".p8.png": subDir}
    plan.DownloadMusic = m.HasMusic() && s.cfg.MusicDownload == "auto"
    if plan.DownloadMusic {
        if s.cfg.MusicLocation == "ask" {
            return NewMusicLocationPickerScreen(s.client, s.cfg, s.cfgPath,
                s.game, s.detail, plan, s.inv, s.invPath, s.prev)
        }
        plan.MusicDir = roms.MusicDestinationDir(s.game.Title)
    }
    return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
}
```

- [ ] **Step 4: Run tests**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_zip_inspect.go
git commit -m "feat(ui): route Pico-8 multi-cart ZIPs to subdirectory extraction"
```

---

## Task 8 — ZIP Download: Extraction Fix and M3U Generation

**Files:**
- Modify: `internal/ui/screen_zip_download.go`

- [ ] **Step 1: Fix `extractROM()` — ext, stem, and cover art**

In `extractROM()`, replace the first three lines:

```go
// Before:
ext := strings.ToLower(filepath.Ext(baseName))
destDir := s.plan.ROMDirs[ext]
if destDir == "" {
    destDir = roms.DestinationDir(ext)
}
stem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
```

```go
// After:
ext := strings.ToLower(roms.ROMExt(baseName))
destDir := s.plan.ROMDirs[ext]
if destDir == "" {
    destDir = roms.DestinationDir(ext)
}
stem := strings.TrimSuffix(baseName, roms.ROMExt(baseName))
```

Still in `extractROM()`, replace the cover art call:

```go
// Before:
if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
    logger.Warn("zip-download: cover art: %v", artErr)
}
```

```go
// After:
if s.plan.IsPico8MultiCart {
    // cover art is handled once by writePico8M3U after all ROMs are extracted
} else if ext == ".p8.png" {
    if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
        logger.Warn("zip-download: cover art copy: %v", artErr)
    }
} else if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
    logger.Warn("zip-download: cover art: %v", artErr)
}
```

- [ ] **Step 2: Add `writePico8M3U` method**

Add after `extractMusic`:

```go
// writePico8M3U writes a .m3u playlist for a multi-cart Pico-8 game into subDir.
// The playlist lists cart filenames (no path prefix) sorted alphabetically.
// Returns the path of the written M3U file.
func (s *ZIPDownloadScreen) writePico8M3U(subDir string, extractedPaths []string) (string, error) {
	cleanDir := strings.TrimSuffix(subDir, "/")
	var carts []string
	for _, p := range extractedPaths {
		if filepath.Dir(p) != cleanDir {
			continue
		}
		ext := strings.ToLower(roms.ROMExt(filepath.Base(p)))
		if ext == ".p8" || ext == ".p8.png" {
			carts = append(carts, filepath.Base(p))
		}
	}
	if len(carts) == 0 {
		return "", fmt.Errorf("no Pico-8 cart files found in %s", subDir)
	}
	sort.Strings(carts)

	safe := roms.SanitiseFilename(s.game.Title, "")
	if safe == "" {
		safe = "Unknown"
	}
	m3uPath := filepath.Join(cleanDir, safe+".m3u")

	if err := os.WriteFile(m3uPath, []byte(strings.Join(carts, "\n")+"\n"), 0644); err != nil {
		return "", fmt.Errorf("write m3u: %w", err)
	}
	logger.Info("zip-download: wrote M3U %s with %d cart(s)", m3uPath, len(carts))
	return m3uPath, nil
}
```

Add `"sort"` to the import block.

- [ ] **Step 3: Call `writePico8M3U` in `run()`**

In `run()`, after the extraction loop and before the `len(s.extracted) == 0` check, add:

```go
// Generate M3U playlist for multi-cart Pico-8 games.
if s.plan.IsPico8MultiCart && len(s.extracted) > 0 {
    subDir := s.plan.ROMDirs[".p8"]
    if subDir == "" {
        subDir = s.plan.ROMDirs[".p8.png"]
    }
    m3uPath, m3uErr := s.writePico8M3U(subDir, s.extracted)
    if m3uErr != nil {
        logger.Warn("zip-download: M3U generation: %v", m3uErr)
    } else {
        s.extracted = append(s.extracted, m3uPath)
        s.inv.Add(s.game.URL, inventory.Entry{
            GameURL: s.game.URL, Title: s.game.Title,
            Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
        }, inventory.DownloadedFile{
            Filename:     filepath.Base(m3uPath),
            DestPath:     m3uPath,
            DownloadedAt: now,
            FileType:     inventory.FileTypeM3U,
        })
        if artErr := s.client.DownloadCoverArt(s.game.CoverURL, m3uPath); artErr != nil {
            logger.Warn("zip-download: M3U cover art: %v", artErr)
        }
    }
}
```

- [ ] **Step 4: Run tests**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_zip_download.go
git commit -m "feat(ui): extract Pico-8 multi-cart to subdirectory and generate M3U playlist"
```

---

## Task 9 — Final Test Run and Version Bump

- [ ] **Step 1: Run full test suite**

```
./scripts/test.sh
```
Expected: all tests PASS with no new failures

- [ ] **Step 2: Review the git log**

```bash
git log --oneline -10
```

Confirm the 8 feature commits are present and each has a focused message.

- [ ] **Step 3: Done**

All Pico-8 support is now in place. The feature covers:
- `.p8` and `.p8.png` recognised throughout the pipeline (scoring, destination, ZIP classification, download page scraping)
- P8 feed platform in `AllPlatforms` (itch.io `tag-pico-8` feed)
- `.p8.png` cover art copied from the ROM itself (no extra network call)
- Multi-cart ZIPs extracted to a named subdirectory with an M3U playlist
- `ResolveUnifiedDest` fixed so unified naming does not rename `.p8.png` ROMs to `.png`
