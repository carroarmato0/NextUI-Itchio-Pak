# Splore Cart Seeding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Seed a `Splore.p8` cartridge into the active Pico-8 ROM directory at startup (and keep it in sync when the user switches cores), so the official Pico-8 binary users can launch the built-in BBS browser directly from the ROM list, while Fake08 users see a clear "not supported" message.

**Architecture:** A new `internal/roms/splore.go` adds two thin exported functions (`EnsureSploreCart`, `CleanSploreCart`) that delegate to unexported helpers taking a `dir` string — this makes them unit-testable without a real device. `main_sdl.go` calls `EnsureSploreCart` at startup; `screen_pico8_core_migrate.go` calls `CleanSploreCart(old)` + `EnsureSploreCart(new)` after a successful core migration.

**Tech Stack:** Go stdlib (`os`), `internal/roms` (existing `Pico8ROMDir`), `internal/logger`.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/roms/splore.go` | Cart content constant, `EnsureSploreCart`, `CleanSploreCart`, internal dir-based helpers |
| Create | `internal/roms/splore_test.go` | Unit tests via `t.TempDir()` (package `roms`, not `roms_test`) |
| Modify | `cmd/itchio-pak/main_sdl.go` | Call `roms.EnsureSploreCart(cfg.Pico8Core)` after inventory init |
| Modify | `internal/ui/screen_pico8_core_migrate.go` | Call `CleanSploreCart` + `EnsureSploreCart` after successful migration |

---

## Task 1: `internal/roms/splore.go` + tests (TDD)

**Files:**
- Create: `internal/roms/splore_test.go`
- Create: `internal/roms/splore.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/roms/splore_test.go`:

```go
package roms

import (
	"os"
	"testing"
)

func TestEnsureSploreCartInDir_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("ensureSploreCartInDir: %v", err)
	}
	path := dir + sploreCartFilename
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Splore.p8 not created: %v", err)
	}
	if string(data) != sploreCartContent {
		t.Errorf("cart content mismatch\ngot:  %q\nwant: %q", string(data), sploreCartContent)
	}
}

func TestEnsureSploreCartInDir_Idempotent(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	stat1, err := os.Stat(dir + sploreCartFilename)
	if err != nil {
		t.Fatalf("stat after first call: %v", err)
	}
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
	stat2, err := os.Stat(dir + sploreCartFilename)
	if err != nil {
		t.Fatalf("stat after second call: %v", err)
	}
	if stat1.ModTime() != stat2.ModTime() {
		t.Error("second call rewrote the file (not idempotent)")
	}
}

func TestCleanSploreCartInDir_RemovesFile(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := ensureSploreCartInDir(dir); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := cleanSploreCartInDir(dir); err != nil {
		t.Fatalf("cleanSploreCartInDir: %v", err)
	}
	if _, err := os.Stat(dir + sploreCartFilename); !os.IsNotExist(err) {
		t.Errorf("Splore.p8 should be gone; stat returned: %v", err)
	}
}

func TestCleanSploreCartInDir_ToleratesMissingFile(t *testing.T) {
	dir := t.TempDir() + "/p8/"
	if err := cleanSploreCartInDir(dir); err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```sh
./scripts/test.sh
```

Expected: compilation error — `ensureSploreCartInDir`, `cleanSploreCartInDir`, `sploreCartFilename`, `sploreCartContent` undefined.

- [ ] **Step 3: Implement `internal/roms/splore.go`**

Create `internal/roms/splore.go`:

```go
package roms

import (
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const sploreCartFilename = "Splore.p8"

const sploreCartContent = `pico-8 cartridge // http://www.pico-8.com
version 16
__lua__
function _init()
  if type(splore)=="function" then
    splore()
  else
    cls(1)
    print("splore not available",4,52,7)
    print("requires official pico-8",4,60,6)
    print("not supported by this core",4,68,13)
  end
end
`

// EnsureSploreCart creates the Pico-8 ROM directory for core if it does not
// exist, then writes Splore.p8 if not already present. Idempotent.
func EnsureSploreCart(core string) error {
	return ensureSploreCartInDir(Pico8ROMDir(core))
}

// CleanSploreCart removes Splore.p8 from the Pico-8 ROM directory for core.
// A missing file is not an error.
func CleanSploreCart(core string) error {
	return cleanSploreCartInDir(Pico8ROMDir(core))
}

func ensureSploreCartInDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn("splore: mkdir %s: %v", dir, err)
		return err
	}
	path := dir + sploreCartFilename
	if _, err := os.Stat(path); err == nil {
		return nil // already present
	}
	if err := os.WriteFile(path, []byte(sploreCartContent), 0644); err != nil {
		logger.Warn("splore: write %s: %v", path, err)
		return err
	}
	logger.Info("splore: seeded %s", path)
	return nil
}

func cleanSploreCartInDir(dir string) error {
	path := dir + sploreCartFilename
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Warn("splore: remove %s: %v", path, err)
		return err
	}
	logger.Info("splore: cleaned %s", path)
	return nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```sh
./scripts/test.sh
```

Expected: all tests pass including the four new `TestEnsureSploreCartInDir_*` / `TestCleanSploreCartInDir_*` tests.

- [ ] **Step 5: Commit**

```sh
git add internal/roms/splore.go internal/roms/splore_test.go
git commit -m "feat(roms): add EnsureSploreCart / CleanSploreCart"
```

---

## Task 2: Seed the cart at startup

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`

The call goes after `inv.VerifyAndClean(inventoryPath)` (line 44) and before the `level :=` block (line 46). Add the `roms` import.

- [ ] **Step 1: Add the startup call**

In `cmd/itchio-pak/main_sdl.go`, add `"github.com/carroarmato0/nextui-itchio-pak/internal/roms"` to the import block, then insert after `inv.VerifyAndClean(inventoryPath)`:

```go
	inv.VerifyAndClean(inventoryPath)

	if err := roms.EnsureSploreCart(cfg.Pico8Core); err != nil {
		logger.Warn("startup: splore cart: %v", err)
	}

	level := cfg.LogLevel
```

- [ ] **Step 2: Build to verify compilation**

```sh
./scripts/build.sh native
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Commit**

```sh
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat(startup): seed Splore.p8 into Pico-8 ROM dir on launch"
```

---

## Task 3: Sync the cart on core migration

**Files:**
- Modify: `internal/ui/screen_pico8_core_migrate.go`

In `startMigration` (line 207), add the splore cart calls after `inventory.MigratePico8Files` succeeds and before `s.cfg.Pico8Core = s.newCore`. Add the `roms` import.

- [ ] **Step 1: Add the migration calls**

In `internal/ui/screen_pico8_core_migrate.go`, the `roms` package is already imported. In the `startMigration` goroutine, replace the block starting at the `if err := inventory.MigratePico8Files(...)` call:

```go
		if err := inventory.MigratePico8Files(s.inv, s.invPath, oldDir, newDir); err != nil {
			logger.Warn("pico8-migrate: failed: %v", err)
			s.err = err
			s.storeState(pico8StateError)
			return
		}

		if err := roms.CleanSploreCart(s.oldCore); err != nil {
			logger.Warn("pico8-migrate: clean splore cart: %v", err)
		}
		if err := roms.EnsureSploreCart(s.newCore); err != nil {
			logger.Warn("pico8-migrate: seed splore cart: %v", err)
		}

		s.cfg.Pico8Core = s.newCore
```

- [ ] **Step 2: Build to verify compilation**

```sh
./scripts/build.sh native
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Run full test suite**

```sh
./scripts/test.sh
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```sh
git add internal/ui/screen_pico8_core_migrate.go
git commit -m "feat(migration): sync Splore.p8 when switching Pico-8 core"
```
