# Pico-8 ZIP Extraction Redesign

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the M3U-based multi-cart detection with a simpler rule: single Pico-8 ROM → flat in `Pico-8 (P8)/`, multiple files → extract all to `Pico-8 (P8)/GameTitle/` preserving ZIP path structure.

**Architecture:** Remove `IsPico8MultiCart` detection entirely. Route all Pico-8 ZIPs by file count: single `.p8`/`.p8.png` uses the existing flat extraction path (unified naming renames it); multiple `.p8`/`.p8.png` OR any `.lua` support files use a new `extractPico8ZIP()` that preserves relative paths under a game subdirectory.

**Tech Stack:** Go 1.22+, `./scripts/test.sh` runs all tests in a container. All UI files have `//go:build !headless`.

**Working directory:** `.worktrees/feature-pico8-support`

---

## Concrete behaviour after redesign

| Game | ZIP contents | Result |
|------|-------------|--------|
| Moss Moss | `moss-moss/game.p8` only | `Pico-8 (P8)/Moss Moss.p8` — flat, unified-named |
| Terra | `terra-itch.p8`, `terra-cem.p8`, `terra-real.p8` at root | `Pico-8 (P8)/Terra/terra-itch.p8`, `terra-cem.p8`, `terra-real.p8` |
| POOM | `poom.p8`…`poom28.p8` + `.lua` files at root | `Pico-8 (P8)/Poom/poom.p8`…`poom28.p8` + `.lua` files |
| Wrapped ZIP | `game-v1.0/game.p8` (single, wrapper dir) | `Pico-8 (P8)/Game Title.p8` — strip wrapper, flat |
| Wrapped multi | `game-v1.0/main.p8` + `game-v1.0/data.lua` | `Pico-8 (P8)/Game Title/main.p8`, `data.lua` |

---

## File Map

| File | Change |
|------|--------|
| `internal/roms/roms.go` | Rename `Pico8MultiCartDir` → `Pico8GameDir` |
| `internal/roms/roms_test.go` | Rename `TestPico8MultiCartDir` → `TestPico8GameDir` |
| `internal/roms/zip_classify.go` | Remove `IsPico8MultiCart()`; add `HasPico8ROMs()`, `HasLuaFiles()` |
| `internal/roms/zip_classify_test.go` | Remove `TestIsPico8MultiCart`; add `TestHasPico8ROMs`, `TestHasLuaFiles` |
| `internal/inventory/inventory.go` | Remove `FileTypeM3U` |
| `internal/ui/screen_zip_inspect.go` | Remove `IsPico8MultiCart bool` from `ZIPPlan`, add `Pico8GameDir string`; update `route()` |
| `internal/ui/screen_zip_download.go` | Remove `writePico8M3U`, M3U block, `sort` import; simplify `extractROM` art; add `topLevelDirPrefix`, `extractPico8ZIP`, dispatch in `run()` |

---

## Task 1 — roms package: rename, remove IsPico8MultiCart, add HasPico8ROMs/HasLuaFiles

**Files:**
- Modify: `internal/roms/roms.go`
- Modify: `internal/roms/roms_test.go`
- Modify: `internal/roms/zip_classify.go`
- Modify: `internal/roms/zip_classify_test.go`

- [ ] **Step 1: Update tests first**

In `internal/roms/roms_test.go`:
- Rename `TestPico8MultiCartDir` → `TestPico8GameDir` and update all `roms.Pico8MultiCartDir(` calls to `roms.Pico8GameDir(`

In `internal/roms/zip_classify_test.go`:
- Remove the entire `TestIsPico8MultiCart` function
- Add at the end:

```go
func TestHasPico8ROMs(t *testing.T) {
	tests := []struct {
		name    string
		entries []roms.ZIPEntry
		want    bool
	}{
		{
			name: "single p8 → has pico8",
			entries: []roms.ZIPEntry{{Name: "game.p8", Kind: roms.KindROM}},
			want: true,
		},
		{
			name: "p8.png → has pico8",
			entries: []roms.ZIPEntry{{Name: "cart.p8.png", Kind: roms.KindROM}},
			want: true,
		},
		{
			name: "only gbc → no pico8",
			entries: []roms.ZIPEntry{{Name: "game.gbc", Kind: roms.KindROM}},
			want: false,
		},
		{
			name: "empty → no pico8",
			entries: nil,
			want: false,
		},
	}
	for _, tt := range tests {
		m := roms.ZIPManifest{Entries: tt.entries}
		if got := m.HasPico8ROMs(); got != tt.want {
			t.Errorf("%s: HasPico8ROMs() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHasLuaFiles(t *testing.T) {
	tests := []struct {
		name    string
		entries []roms.ZIPEntry
		want    bool
	}{
		{
			name:    "lua present → true",
			entries: []roms.ZIPEntry{{Name: "helper.lua", Kind: roms.KindOther}},
			want:    true,
		},
		{
			name:    "UPPER.LUA → true",
			entries: []roms.ZIPEntry{{Name: "HELPER.LUA", Kind: roms.KindOther}},
			want:    true,
		},
		{
			name:    "no lua → false",
			entries: []roms.ZIPEntry{{Name: "game.p8", Kind: roms.KindROM}},
			want:    false,
		},
		{
			name:    "empty → false",
			entries: nil,
			want:    false,
		},
	}
	for _, tt := range tests {
		m := roms.ZIPManifest{Entries: tt.entries}
		if got := m.HasLuaFiles(); got != tt.want {
			t.Errorf("%s: HasLuaFiles() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
./scripts/test.sh
```
Expected: FAIL — `Pico8GameDir undefined`, `HasPico8ROMs undefined`, `HasLuaFiles undefined`

- [ ] **Step 3: Update `internal/roms/roms.go`**

Rename `Pico8MultiCartDir` to `Pico8GameDir` — change the function name only, body is unchanged:

```go
// Pico8GameDir returns the subdirectory for a Pico-8 game that ships with
// multiple files (.p8/.p8.png/.lua). All game files are extracted here.
func Pico8GameDir(gameTitle string) string {
	safe := SanitiseFilename(gameTitle, "")
	if safe == "" {
		safe = "Unknown"
	}
	return Pico8Dir + safe + "/"
}
```

- [ ] **Step 4: Update `internal/roms/zip_classify.go`**

Remove `IsPico8MultiCart()` entirely.

Add after `HasDuplicateROMExt`:

```go
// HasPico8ROMs reports whether the manifest contains at least one Pico-8
// cartridge file (.p8 or .p8.png).
func (m ZIPManifest) HasPico8ROMs() bool {
	return len(m.ROMsByExt()[".p8"])+len(m.ROMsByExt()[".p8.png"]) > 0
}

// HasLuaFiles reports whether the manifest contains any .lua file.
// Pico-8 games sometimes ship with Lua support scripts that must live
// alongside the cartridges.
func (m ZIPManifest) HasLuaFiles() bool {
	for _, e := range m.Entries {
		if strings.HasSuffix(strings.ToLower(e.Name), ".lua") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/roms/roms.go internal/roms/roms_test.go internal/roms/zip_classify.go internal/roms/zip_classify_test.go
git commit -m "refactor(roms): rename Pico8MultiCartDir→Pico8GameDir, replace IsPico8MultiCart with HasPico8ROMs/HasLuaFiles"
```

---

## Task 2 — Remove FileTypeM3U and update ZIPPlan struct

**Files:**
- Modify: `internal/inventory/inventory.go`
- Modify: `internal/ui/screen_zip_inspect.go`

- [ ] **Step 1: Remove `FileTypeM3U` from `internal/inventory/inventory.go`**

The constants block currently reads:
```go
const (
	FileTypeROM   = "rom"
	FileTypeMusic = "music"
	FileTypeM3U   = "m3u"
)
```

Remove `FileTypeM3U`:
```go
const (
	FileTypeROM   = "rom"
	FileTypeMusic = "music"
)
```

- [ ] **Step 2: Update `ZIPPlan` in `internal/ui/screen_zip_inspect.go`**

Remove `IsPico8MultiCart bool` and add `Pico8GameDir string`.

Replace the struct with:

```go
type ZIPPlan struct {
	Upload   roms.Upload
	CDNURL   string
	Manifest roms.ZIPManifest

	DownloadROMs  bool
	DownloadMusic bool
	// Pico8GameDir, when non-empty, triggers path-preserving extraction of all
	// .p8/.p8.png/.lua files from the ZIP into this directory.
	Pico8GameDir string
	// SelectedROMs maps lowercase extension → chosen entry Name.
	// Empty map means all ROMs in the manifest are selected.
	SelectedROMs map[string]string
	// ROMDirs maps lowercase extension → chosen destination directory.
	// Overrides DestinationDir when set (used for user-chosen GBA folder).
	ROMDirs  map[string]string
	MusicDir string
}
```

- [ ] **Step 3: Run tests**

```
./scripts/test.sh
```
Expected: PASS (compiler will catch any `IsPico8MultiCart` or `FileTypeM3U` references remaining)

If there are compile errors referencing the removed fields, fix them now (they'll be in `screen_zip_download.go` — remove the `IsPico8MultiCart` check and `FileTypeM3U` usage there; those are removed fully in Task 4).

- [ ] **Step 4: Commit**

```bash
git add internal/inventory/inventory.go internal/ui/screen_zip_inspect.go
git commit -m "refactor(ui): replace IsPico8MultiCart ZIPPlan field with Pico8GameDir"
```

---

## Task 3 — Update `route()` in `screen_zip_inspect.go`

**Files:**
- Modify: `internal/ui/screen_zip_inspect.go`

The new routing logic for Pico-8:
1. **Single Pico-8 ROM, no other files** (`IsSingleROMOnly() && !HasOtherFiles()` with inner ext `.p8`/`.p8.png`): force extraction (never keep the ZIP on disk — Pico-8 emulators can't load from ZIP)
2. **Multi-file Pico-8** (multiple `.p8`/`.p8.png`, OR single cart + `.lua` files): use path-preserving extraction to game subdirectory

- [ ] **Step 1: Rewrite the Pico-8-relevant sections of `route()`**

The full updated `route()` method:

```go
func (s *ZIPInspectScreen) route() Screen {
	m := s.plan.Manifest

	if !m.HasROMs() && !m.HasMusic() {
		logger.Warn("zip-inspect: manifest empty, returning to prev")
		return s.prev
	}

	manifestHasGBA := len(m.ROMsByExt()[".gba"]) > 0

	// GBA + "ask": route through contents screen before anything else so the
	// user can choose between Game Boy Advance (GBA) and Game Boy Advance (MGBA).
	if manifestHasGBA && s.cfg.ROMLocation == "ask" {
		return NewZIPContentsScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.plan, s.inv, s.invPath, s.prev)
	}

	// Single ROM, no music, no extra files.
	if m.IsSingleROMOnly() && !m.HasOtherFiles() {
		// Use the inner ROM's extension to route to the correct destination directory
		// (e.g., a ZIP containing a single .gba should land in the GBA folder).
		ext := strings.ToLower(roms.ROMExt(s.upload.Filename))
		for _, e := range m.Entries {
			if e.Kind == roms.KindROM {
				if inner := strings.ToLower(roms.ROMExt(e.Name)); roms.DestinationDir(inner) != "" {
					ext = inner
				}
				break
			}
		}

		if ext == ".p8" || ext == ".p8.png" {
			// Pico-8: always extract — emulators cannot load .p8/.p8.png from a ZIP.
			plan := s.plan
			plan.DownloadROMs = true
			return NewZIPDownloadScreen(s.client, s.cfg, s.game, s.detail, plan, s.inv, s.invPath, s.prev)
		}

		// Non-Pico-8: keep the ZIP on disk (most emulators support ZIP natively).
		dest := roms.DestinationDir(ext) + s.upload.Filename
		if existing := s.inv.ExistingDestPath(s.game.URL, s.upload.Filename); existing != "" {
			dest = existing
		}
		patched := s.upload
		patched.URL = s.plan.CDNURL
		return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, patched, dest, s.inv, s.invPath, s.prev)
	}

	// Multi-file Pico-8: extract all .p8/.p8.png/.lua files to a game
	// subdirectory, preserving the ZIP's relative path structure.
	p8Count := len(m.ROMsByExt()[".p8"]) + len(m.ROMsByExt()[".p8.png"])
	if p8Count > 1 || (p8Count == 1 && m.HasLuaFiles()) {
		gameDir := roms.Pico8GameDir(s.game.Title)
		plan := s.plan
		plan.DownloadROMs = true
		plan.Pico8GameDir = gameDir
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

	// Multiple ROMs of same extension, GBA present, or music choice needed → picker.
	if m.HasDuplicateROMExt() || (s.cfg.MusicDownload == "ask" && m.HasMusic()) || manifestHasGBA {
		return NewZIPContentsScreen(s.client, s.cfg, s.cfgPath, s.cache,
			s.game, s.detail, s.plan, s.inv, s.invPath, s.prev)
	}

	// Auto path.
	plan := s.plan
	plan.DownloadROMs = m.HasROMs()
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

- [ ] **Step 2: Run tests**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_zip_inspect.go
git commit -m "feat(ui): Pico-8 ZIPs always extracted; multi-file goes to game subdirectory"
```

---

## Task 4 — Rework `screen_zip_download.go`

**Files:**
- Modify: `internal/ui/screen_zip_download.go`

Remove all M3U machinery and add the new path-preserving extraction path.

- [ ] **Step 1: Remove M3U machinery**

1. Remove `"sort"` from the import block.
2. Remove the entire `writePico8M3U` method.
3. Remove the M3U generation block in `run()`:
   ```go
   // Remove this entire block:
   if s.plan.IsPico8MultiCart && len(s.extracted) > 0 {
       ...
   }
   ```
   Also remove the `inv.Save` call that was added specifically after the M3U inv.Add (the original `inv.Save` at line ~174 stays).

4. In `extractROM()`, simplify the cover art block — remove the `IsPico8MultiCart` check:
   ```go
   // Before:
   if s.plan.IsPico8MultiCart {
       // cover art is handled once by writePico8M3U after all ROMs are extracted
   } else if ext == ".p8.png" {
       ...
   } else if artErr := ...
   
   // After:
   if ext == ".p8.png" {
       if artErr := itchio.CopyCoverArt(finalDest); artErr != nil {
           logger.Warn("zip-download: cover art copy: %v", artErr)
       }
   } else if artErr := s.client.DownloadCoverArt(s.game.CoverURL, finalDest); artErr != nil {
       logger.Warn("zip-download: cover art: %v", artErr)
   }
   ```

- [ ] **Step 2: Add `topLevelDirPrefix` helper**

Add this package-level function (not a method) after the `extractZIPEntry` function:

```go
// topLevelDirPrefix returns the common top-level directory prefix shared by
// all paths, including the trailing slash. Returns "" when files are at the
// ZIP root or share no common top-level directory.
//
// Example: ["game-v1/main.p8", "game-v1/data.lua"] → "game-v1/"
// Example: ["main.p8", "data.lua"]                 → ""
func topLevelDirPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	first := func(p string) string {
		p = filepath.ToSlash(p)
		idx := strings.Index(p, "/")
		if idx < 0 {
			return ""
		}
		return p[:idx+1]
	}
	prefix := first(paths[0])
	if prefix == "" {
		return ""
	}
	for _, p := range paths[1:] {
		if first(p) != prefix {
			return ""
		}
	}
	return prefix
}
```

- [ ] **Step 3: Add `extractPico8ZIP` method**

Add after `extractMusic`:

```go
// extractPico8ZIP extracts all .p8, .p8.png, and .lua files from r into
// s.plan.Pico8GameDir, preserving relative paths from the ZIP after stripping
// any common top-level wrapper directory. Support files (.lua) required by
// Pico-8 carts are extracted alongside the cartridges.
func (s *ZIPDownloadScreen) extractPico8ZIP(r *zip.Reader, now time.Time) {
	gameDir := strings.TrimSuffix(s.plan.Pico8GameDir, "/")

	// Collect all relevant file paths to determine the common prefix to strip.
	var relevantPaths []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		if ext == ".p8" || ext == ".p8.png" || strings.HasSuffix(lower, ".lua") {
			relevantPaths = append(relevantPaths, name)
		}
	}
	prefix := topLevelDirPrefix(relevantPaths)
	logger.Debug("zip-download: pico8 strip-prefix=%q game-dir=%s", prefix, gameDir)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		base := filepath.Base(name)
		if strings.HasPrefix(base, "._") {
			continue
		}
		lower := strings.ToLower(base)
		ext := strings.ToLower(roms.ROMExt(base))
		isP8 := ext == ".p8" || ext == ".p8.png"
		isLua := strings.HasSuffix(lower, ".lua")
		if !isP8 && !isLua {
			continue
		}

		relPath := strings.TrimPrefix(name, prefix)
		dest := filepath.Join(gameDir, filepath.FromSlash(relPath))

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			logger.Warn("zip-download: pico8 mkdir %s: %v", filepath.Dir(dest), err)
			s.skipped = append(s.skipped, base)
			continue
		}
		if err := extractZIPEntry(f, dest); err != nil {
			logger.Warn("zip-download: pico8 extract %s: %v", base, err)
			s.skipped = append(s.skipped, base)
			continue
		}
		logger.Info("zip-download: pico8 extracted %s → %s", base, dest)
		s.extracted = append(s.extracted, dest)

		s.inv.Add(s.game.URL, inventory.Entry{
			GameURL: s.game.URL, Title: s.game.Title,
			Author: s.game.Author, CoverURL: s.game.CoverURL, IsFree: s.game.IsFree,
		}, inventory.DownloadedFile{
			Filename:     base,
			DestPath:     dest,
			DownloadedAt: now,
			FileType:     inventory.FileTypeROM,
		})
	}
}
```

- [ ] **Step 4: Dispatch `extractPico8ZIP` in `run()`**

In `run()`, immediately after `r, err := zip.OpenReader(tmpPath)` / `defer r.Close()`, add a dispatch block before the `now := time.Now()` line:

```go
r, err := zip.OpenReader(tmpPath)
if err != nil {
    // ... existing error handling
}
defer r.Close()

// Pico-8 multi-file: path-preserving extraction to game subdirectory.
if s.plan.Pico8GameDir != "" {
    now := time.Now()
    s.extractPico8ZIP(&r.Reader, now)
    if err := s.inv.Save(s.invPath); err != nil {
        logger.Warn("zip-download: save inventory: %v", err)
    }
    // Cover art: use itch.io promotional image, keyed to a synthetic .p8 path
    // so NextUI's .media lookup finds it for the game directory.
    if len(s.extracted) > 0 {
        safe := roms.SanitiseFilename(s.game.Title, "")
        if safe == "" {
            safe = "Unknown"
        }
        artRef := filepath.Join(strings.TrimSuffix(s.plan.Pico8GameDir, "/"), safe+".p8")
        if artErr := s.client.DownloadCoverArt(s.game.CoverURL, artRef); artErr != nil {
            logger.Warn("zip-download: pico8 cover art: %v", artErr)
        }
    }
    if len(s.extracted) == 0 {
        logger.Error("zip-download: pico8: no files extracted (skipped=%d)", len(s.skipped))
        s.err = fmt.Errorf("no Pico-8 files could be extracted from ZIP")
        s.storeState(zipDLError)
        return
    }
    logger.Info("zip-download: pico8 done, extracted %d file(s)", len(s.extracted))
    s.storeState(zipDLDone)
    return
}

now := time.Now()
for _, f := range r.File {
    // ... existing extraction loop unchanged
```

- [ ] **Step 5: Run tests**

```
./scripts/test.sh
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/screen_zip_download.go
git commit -m "feat(ui): path-preserving Pico-8 ZIP extraction, remove M3U generation"
```

---

## Task 5 — Final test run

- [ ] **Step 1: Run full test suite**

```
./scripts/test.sh
```
Expected: all packages PASS

- [ ] **Step 2: Review git log**

```bash
git log --oneline -15
```

Confirm 4 new commits on top of the existing 9.
