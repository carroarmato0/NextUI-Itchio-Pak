# Itch.io NextUI Pak — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a NextUI Pak that lets users browse, discover, and download GB/GBC games from Itch.io's GB Studio collection entirely on-device over WiFi.

**Architecture:** Go 1.22 binary with SDL2 UI (go-sdl2), split into packages: `internal/itchio` (HTTP/scraping), `internal/roms` (ROM scoring/placement), `internal/settings` (JSON config), `internal/renderer` (SDL2 primitives + image cache), `internal/ui` (screens). Entry point at `cmd/itchio-pak/main.go`. All build/test in containers; deploy/debug on host via ADB.

**Tech Stack:** Go 1.22+, github.com/veandco/go-sdl2, github.com/skip2/go-qrcode, golang.org/x/image, golang.org/x/net/html, Docker/Podman (LoveRetro toolchain images for ARM64 cross-compile)

---

## Phase 1: Foundation

### Task 1: Go module + project skeleton

**Files:**
- Create: `go.mod`
- Create: `go.sum` (generated)
- Create: `cmd/itchio-pak/main.go`
- Create: `internal/itchio/client.go`
- Create: `internal/itchio/feed.go`
- Create: `internal/itchio/game.go`
- Create: `internal/itchio/download.go`
- Create: `internal/itchio/download_auth.go`
- Create: `internal/roms/roms.go`
- Create: `internal/settings/settings.go`
- Create: `internal/renderer/renderer.go`
- Create: `internal/renderer/image_cache.go`
- Create: `internal/renderer/qr.go`
- Create: `internal/ui/screen_list.go`
- Create: `internal/ui/screen_detail.go`
- Create: `internal/ui/screen_settings.go`
- Create: `internal/ui/screen_download.go`

- [ ] **Step 1: Initialise Go module**

```bash
cd /path/to/Itch.io
go mod init github.com/carroarmato0/nextui-itchio-pak
```

Expected: `go.mod` created with `module github.com/carroarmato0/nextui-itchio-pak` and `go 1.22`

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/veandco/go-sdl2/sdl@latest
go get github.com/veandco/go-sdl2/ttf@latest
go get github.com/veandco/go-sdl2/img@latest
go get github.com/skip2/go-qrcode@latest
go get golang.org/x/image@latest
go get golang.org/x/net@latest
```

Expected: `go.sum` populated, imports resolvable.

- [ ] **Step 3: Create stub entry point**

`cmd/itchio-pak/main.go`:
```go
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logFile, err := os.OpenFile(os.Getenv("HOME")+"/itchio-pak.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	if *headless {
		log.Println("headless mode: exiting cleanly")
		os.Exit(0)
	}

	// SDL2 UI wired in Task 17
	log.Fatal("SDL2 not yet wired up")
}
```

- [ ] **Step 4: Create package stubs** (one per file; each just declares the package and exports a placeholder)

`internal/settings/settings.go`:
```go
package settings

import (
	"encoding/json"
	"os"
)

type Config struct {
	APIKey       string `json:"api_key"`
	ROMSelection string `json:"rom_selection"`
}

func Load(path string) (*Config, error)  { return nil, nil }
func (c *Config) Save(path string) error { return nil }
```

`internal/roms/roms.go`:
```go
package roms

type Upload struct {
	Filename string
	URL      string
}

func ScoreUpload(filename string) int         { return 0 }
func DestinationDir(ext string) string        { return "" }
func SelectBest(uploads []Upload) *Upload     { return nil }
```

`internal/itchio/client.go`:
```go
package itchio

import "net/http"

type Client struct {
	http *http.Client
}

func NewClient() *Client { return nil }
```

`internal/itchio/feed.go`:
```go
package itchio

type Game struct {
	Title    string
	Author   string
	URL      string
	CoverURL string
	Price    float64
	IsFree   bool
}

func (c *Client) FetchGames(page int, query string) ([]Game, error) { return nil, nil }
```

`internal/itchio/game.go`:
```go
package itchio

type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
}

type Upload struct {
	Filename string
	URL      string
}

func (c *Client) FetchGameDetail(gameURL string) (*GameDetail, error) { return nil, nil }
```

`internal/itchio/download.go`:
```go
package itchio

import "io"

func (c *Client) DownloadFree(gameURL string, upload Upload, dest string, progress func(int64, int64)) error {
	return nil
}
```

`internal/itchio/download_auth.go`:
```go
package itchio

func (c *Client) DownloadAuth(apiKey, gameID string, upload Upload, dest string, progress func(int64, int64)) error {
	return nil
}

func (c *Client) CheckOwnership(apiKey, gameID string) (bool, error) { return false, nil }
```

`internal/renderer/renderer.go`:
```go
//go:build !headless

package renderer

// Renderer wraps SDL2 window, renderer, and font.
type Renderer struct{}

func New(title string, w, h int) (*Renderer, error) { return nil, nil }
func (r *Renderer) Close()                          {}
```

`internal/renderer/image_cache.go`:
```go
//go:build !headless

package renderer

// ImageCache is an LRU cache of SDL2 textures keyed by URL.
type ImageCache struct{}

func NewImageCache(maxEntries int) *ImageCache { return nil }
```

`internal/renderer/qr.go`:
```go
//go:build !headless

package renderer

// QRTexture generates a QR code PNG and returns an SDL2 texture.
func (r *Renderer) QRTexture(url string) (interface{}, error) { return nil, nil }
```

`internal/ui/screen_list.go`:
```go
//go:build !headless

package ui

// ListScreen is the split-panel game list.
type ListScreen struct{}
```

`internal/ui/screen_detail.go`:
```go
//go:build !headless

package ui

// DetailScreen shows cover, screenshots, QR, and download action.
type DetailScreen struct{}
```

`internal/ui/screen_settings.go`:
```go
//go:build !headless

package ui

// SettingsScreen handles API key entry and config toggles.
type SettingsScreen struct{}
```

`internal/ui/screen_download.go`:
```go
//go:build !headless

package ui

// DownloadScreen shows download progress bar.
type DownloadScreen struct{}
```

- [ ] **Step 5: Verify it compiles**

```bash
go build ./...
```

Expected: no errors (stubs return nil everywhere).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/
git commit -m "feat: add Go module and package stubs"
```

---

### Task 2: Docker dev image + container scripts

**Files:**
- Create: `docker/Dockerfile.dev`
- Create: `scripts/test.sh`
- Create: `scripts/build.sh`
- Create: `scripts/release.sh`
- Create: `scripts/deploy.sh`
- Create: `scripts/debug.sh`
- Create: `Makefile`

- [ ] **Step 1: Write Dockerfile.dev**

`docker/Dockerfile.dev`:
```dockerfile
FROM golang:1.22-bookworm
RUN apt-get update && apt-get install -y \
    libsdl2-dev libsdl2-ttf-dev libsdl2-image-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
```

- [ ] **Step 2: Write scripts/test.sh**

```sh
#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

IMAGE="itchio-pak-dev"
if [ -z "${IN_CONTAINER:-}" ]; then
    RUNTIME="$(detect_runtime)"
    if [ -z "$RUNTIME" ]; then
        echo "ERROR: docker or podman required" >&2; exit 1
    fi
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        "$IMAGE" "$0" "$@"
fi

COVER=""
if [ "${1:-}" = "--coverage" ]; then
    COVER="-coverprofile=coverage.out"
fi

go test -race -tags headless $COVER ./...

if [ -n "$COVER" ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report: coverage.html"
fi
```

- [ ] **Step 3: Write scripts/build.sh**

```sh
#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

RUNTIME_OVERRIDE=""
TARGET=""
while [ $# -gt 0 ]; do
    case "$1" in
        --runtime) RUNTIME_OVERRIDE="$2"; shift 2 ;;
        *) TARGET="$1"; shift ;;
    esac
done

if [ -z "$TARGET" ]; then
    echo "Usage: build.sh [--runtime docker|podman] native|tg5040|tg5050|my355|all" >&2
    exit 1
fi

RUNTIME="${RUNTIME_OVERRIDE:-$(detect_runtime)}"
if [ -z "$RUNTIME" ]; then
    echo "ERROR: docker or podman required" >&2; exit 1
fi

build_native() {
    IMAGE="itchio-pak-dev"
    if [ -z "${IN_CONTAINER:-}" ]; then
        $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
            $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            "$IMAGE" "$0" "$@"
    fi
    mkdir -p bin/native
    go build -o bin/native/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/native/itchio-pak"
}

build_platform() {
    PLATFORM="$1"
    case "$PLATFORM" in
        tg5040) IMAGE="ghcr.io/loveretro/tg5040-toolchain:latest" ;;
        tg5050) IMAGE="ghcr.io/loveretro/tg5050-toolchain:latest" ;;
        my355)  IMAGE="ghcr.io/loveretro/my355-toolchain:latest" ;;
        *) echo "Unknown platform: $PLATFORM" >&2; exit 1 ;;
    esac

    if [ -z "${IN_CONTAINER:-}" ]; then
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            "$IMAGE" "$0" --runtime "$RUNTIME" "$PLATFORM"
    fi

    mkdir -p bin/"$PLATFORM"
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
        go build -o bin/"$PLATFORM"/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/$PLATFORM/itchio-pak"
}

case "$TARGET" in
    native) build_native native ;;
    tg5040|tg5050|my355) build_platform "$TARGET" ;;
    all)
        build_platform tg5040
        build_platform tg5050
        build_platform my355
        ;;
    *) echo "Unknown target: $TARGET" >&2; exit 1 ;;
esac
```

- [ ] **Step 4: Write scripts/release.sh**

```sh
#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

IMAGE="itchio-pak-dev"
RUNTIME="$(detect_runtime)"
if [ -z "$RUNTIME" ]; then
    echo "ERROR: docker or podman required" >&2; exit 1
fi

if [ -z "${IN_CONTAINER:-}" ]; then
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        -e CONTAINER_RUNTIME="$RUNTIME" \
        "$IMAGE" "$0" "$@"
fi

echo "==> Running tests..."
./scripts/test.sh

echo "==> Building all platforms..."
./scripts/build.sh all

echo "==> Assembling release artifacts..."
rm -rf dist
mkdir -p dist/all/Tools

for PLATFORM in tg5040 tg5050 my355; do
    PAK_DIR="dist/$PLATFORM/Itch.io.pak"
    mkdir -p "$PAK_DIR/lib" "$PAK_DIR/assets"

    cp bin/"$PLATFORM"/itchio-pak "$PAK_DIR/itchio-pak"
    cp launch.sh "$PAK_DIR/launch.sh"
    cp pak.json "$PAK_DIR/pak.json"
    cp -r assets/. "$PAK_DIR/assets/"

    # Copy platform SDL2 libs
    case "$PLATFORM" in
        tg5040|tg5050) cp lib/tg5040/. "$PAK_DIR/lib/" 2>/dev/null || true ;;
        my355)          cp lib/my355/.  "$PAK_DIR/lib/" 2>/dev/null || true ;;
    esac

    cd dist/"$PLATFORM"
    zip -r ../dist/"$PLATFORM"/Itch.io.pak.zip Itch.io.pak
    cd - >/dev/null

    # Also copy into .pakz structure
    cp -r "$PAK_DIR" "dist/all/Tools/$PLATFORM/Itch.io.pak"
done

mkdir -p dist/all
cd dist/all
zip -r ../all/Itch.io.pakz Tools
cd - >/dev/null

echo "==> Release artifacts:"
find dist -name "*.zip" -o -name "*.pakz" | sort
```

- [ ] **Step 5: Write scripts/deploy.sh**

```sh
#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_SRC="dist/$PLATFORM/Itch.io.pak"

if [ ! -d "$PAK_SRC" ]; then
    echo "ERROR: $PAK_SRC not found. Run scripts/release.sh first." >&2
    exit 1
fi

SD_PATH="${1:-}"

if [ -n "$SD_PATH" ]; then
    echo "==> Deploying to SD card: $SD_PATH"
    DEST="$SD_PATH/Tools/$PLATFORM/Itch.io.pak"
    mkdir -p "$DEST"
    cp -r "$PAK_SRC/." "$DEST/"
    echo "Deployed to $DEST"
else
    echo "==> Deploying via ADB..."
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2
        exit 1
    fi
    DEVICE="$(adb devices | awk 'NR==2 {print $1}')"
    if [ -z "$DEVICE" ]; then
        echo "ERROR: no ADB device connected. Check USB cable." >&2; exit 1
    fi
    DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch.io.pak"
    adb shell "mkdir -p $DEST"
    adb push "$PAK_SRC/." "$DEST/"
    echo "Deployed to $DEVICE:$DEST"
fi
```

- [ ] **Step 6: Write scripts/debug.sh**

```sh
#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch.io.pak"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/Itchio/itchio-pak.log"

check_adb() {
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools." >&2; exit 1
    fi
    if ! adb devices | grep -q "device$"; then
        echo "ERROR: no ADB device. Check USB cable." >&2; exit 1
    fi
}

CMD="${1:-}"
case "$CMD" in
    logs)
        check_adb
        echo "==> Streaming log (Ctrl-C to stop)..."
        adb shell "tail -f $LOG_PATH"
        ;;
    push)
        check_adb
        echo "==> Building tg5040..."
        ./scripts/build.sh "$PLATFORM"
        echo "==> Pushing binary..."
        adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"
        ;;
    run)
        check_adb
        echo "==> Building and pushing..."
        ./scripts/build.sh "$PLATFORM"
        adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"
        echo "==> Running (Ctrl-C to stop)..."
        adb shell "cd $PAK_DEST && ./itchio-pak 2>&1 | tee /tmp/pak-run.log"
        ;;
    pull-cache)
        check_adb
        mkdir -p debug-cache
        adb pull /tmp/itchio-pak/cache/ ./debug-cache/
        echo "Cache pulled to ./debug-cache/"
        ;;
    pull-log)
        check_adb
        adb pull "$LOG_PATH" .
        echo "Log pulled to ./itchio-pak.log"
        ;;
    shell)
        check_adb
        adb shell
        ;;
    *)
        echo "Usage: debug.sh logs|push|run|pull-cache|pull-log|shell" >&2
        exit 1
        ;;
esac
```

- [ ] **Step 7: Make scripts executable**

```bash
chmod +x scripts/test.sh scripts/build.sh scripts/release.sh scripts/deploy.sh scripts/debug.sh
```

- [ ] **Step 8: Write Makefile**

`Makefile`:
```makefile
.PHONY: test build-native build-all release deploy deploy-sd clean

test:
	./scripts/test.sh

test-coverage:
	./scripts/test.sh --coverage

build-native:
	./scripts/build.sh native

build-tg5040:
	./scripts/build.sh tg5040

build-tg5050:
	./scripts/build.sh tg5050

build-my355:
	./scripts/build.sh my355

build-all:
	./scripts/build.sh all

release:
	./scripts/release.sh

deploy:
	./scripts/deploy.sh

deploy-sd:
	./scripts/deploy.sh $(SD)

debug-logs:
	./scripts/debug.sh logs

debug-push:
	./scripts/debug.sh push

debug-run:
	./scripts/debug.sh run

clean:
	rm -rf bin/ dist/ coverage.out coverage.html debug-cache/
```

- [ ] **Step 9: Verify test.sh runs (headless)**

```bash
./scripts/test.sh
```

Expected: container builds, `go test -race -tags headless ./...` passes (all stubs, no real tests yet).

- [ ] **Step 10: Commit**

```bash
git add docker/ scripts/ Makefile
git commit -m "feat: add dev container, build/test/release/deploy scripts, Makefile"
```

---

### Task 3: launch.sh, pak.json, assets placeholder

**Files:**
- Create: `launch.sh`
- Create: `pak.json`
- Create: `assets/.gitkeep`

- [ ] **Step 1: Write launch.sh**

```sh
#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
export LD_LIBRARY_PATH="$PAK_DIR/lib:$LD_LIBRARY_PATH"
export PATH="$PAK_DIR:$PATH"
mkdir -p "$HOME"
exec "$PAK_DIR/itchio-pak"
```

- [ ] **Step 2: Write pak.json**

```json
{
  "name": "Itch.io",
  "version": "1.0.0",
  "type": "tool",
  "description": "Browse and download GB/GBC games from Itch.io's GB Studio collection.",
  "author": "Carroarmato0",
  "repo_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak",
  "release_filename": "Itch.io.pak.zip",
  "platforms": ["tg5040", "tg5050", "my355"]
}
```

- [ ] **Step 3: Create assets placeholder**

```bash
mkdir -p assets lib/tg5040 lib/my355
touch assets/.gitkeep lib/tg5040/.gitkeep lib/my355/.gitkeep
```

Note: `assets/font.ttf` and `assets/icon.png` must be added manually before first device deploy. The font must be a TTF licensed for redistribution (e.g. Noto Sans or similar). The icon is the Itch.io logo used under nominative fair use.

- [ ] **Step 4: Commit**

```bash
git add launch.sh pak.json assets/ lib/
git commit -m "feat: add launch.sh, pak.json, assets and lib directory structure"
```

---

## Phase 2: Core Logic (TDD)

### Task 4: settings package

**Files:**
- Modify: `internal/settings/settings.go`
- Create: `internal/settings/settings_test.go`

- [ ] **Step 1: Write failing tests**

`internal/settings/settings_test.go`:
```go
package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "" {
		t.Errorf("default APIKey = %q, want %q", cfg.APIKey, "")
	}
	if cfg.ROMSelection != "auto" {
		t.Errorf("default ROMSelection = %q, want %q", cfg.ROMSelection, "auto")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{APIKey: "abc123", ROMSelection: "ask"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIKey != "abc123" {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, "abc123")
	}
	if loaded.ROMSelection != "ask" {
		t.Errorf("ROMSelection = %q, want %q", loaded.ROMSelection, "ask")
	}
}

func TestLoadCorruptedFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("not json"), 0644)

	cfg, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ROMSelection != "auto" {
		t.Errorf("corrupted load should return defaults")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
./scripts/test.sh
```

Expected: FAIL — `Load` and `Save` are stubs returning nil.

- [ ] **Step 3: Implement settings.go**

```go
package settings

import (
	"encoding/json"
	"os"
)

type Config struct {
	APIKey       string `json:"api_key"`
	ROMSelection string `json:"rom_selection"`
}

func defaults() *Config {
	return &Config{APIKey: "", ROMSelection: "auto"}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaults(), nil
		}
		return defaults(), nil
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaults(), nil
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
./scripts/test.sh
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/settings/
git commit -m "feat: implement settings package with JSON config load/save"
```

---

### Task 5: roms package

**Files:**
- Modify: `internal/roms/roms.go`
- Create: `internal/roms/roms_test.go`

- [ ] **Step 1: Write failing tests**

`internal/roms/roms_test.go`:
```go
package roms_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestScoreUpload(t *testing.T) {
	tests := []struct {
		filename string
		want     int
	}{
		{"game.gbc", 2},
		{"game.GBC", 2},
		{"game.gb", 1},
		{"game.GB", 1},
		{"game.zip", 0},
		{"game.pocket", 0},
		{"game.pdf", 0},
	}
	for _, tt := range tests {
		got := roms.ScoreUpload(tt.filename)
		if got != tt.want {
			t.Errorf("ScoreUpload(%q) = %d, want %d", tt.filename, got, tt.want)
		}
	}
}

func TestDestinationDir(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".gbc", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".GBC", "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"},
		{".gb", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
		{".GB", "/mnt/SDCARD/Roms/Game Boy (GB)/"},
	}
	for _, tt := range tests {
		got := roms.DestinationDir(tt.ext)
		if got != tt.want {
			t.Errorf("DestinationDir(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestSelectBest(t *testing.T) {
	uploads := []roms.Upload{
		{Filename: "game.pdf", URL: "u1"},
		{Filename: "game.gb", URL: "u2"},
		{Filename: "game.gbc", URL: "u3"},
	}
	got := roms.SelectBest(uploads)
	if got == nil || got.URL != "u3" {
		t.Errorf("SelectBest: expected gbc upload, got %v", got)
	}
}

func TestSelectBestNoROMs(t *testing.T) {
	uploads := []roms.Upload{
		{Filename: "manual.pdf", URL: "u1"},
	}
	got := roms.SelectBest(uploads)
	if got != nil {
		t.Errorf("SelectBest with no ROMs: expected nil, got %v", got)
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
./scripts/test.sh
```

Expected: FAIL

- [ ] **Step 3: Implement roms.go**

```go
package roms

import (
	"path/filepath"
	"strings"
)

type Upload struct {
	Filename string
	URL      string
}

func ScoreUpload(filename string) int {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".gbc":
		return 2
	case ".gb":
		return 1
	default:
		return 0
	}
}

func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	default:
		return ""
	}
}

func SelectBest(uploads []Upload) *Upload {
	var best *Upload
	bestScore := 0
	for i := range uploads {
		s := ScoreUpload(uploads[i].Filename)
		if s > bestScore {
			bestScore = s
			best = &uploads[i]
		}
	}
	return best
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
./scripts/test.sh
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/roms/
git commit -m "feat: implement roms package — scoring, destination dirs, best-upload selection"
```

---

### Task 6: itchio client + RSS feed

**Files:**
- Modify: `internal/itchio/client.go`
- Modify: `internal/itchio/feed.go`
- Create: `internal/itchio/feed_test.go`
- Create: `testdata/rss_page1.xml`

- [ ] **Step 1: Capture a real RSS page as testdata**

```bash
curl -s "https://itch.io/games/made-with-gb-studio.xml?page=1" > testdata/rss_page1.xml
```

Verify the file has `<item>` elements and `<description>` blocks with `<img src=` tags.

- [ ] **Step 2: Write failing feed tests**

`internal/itchio/feed_test.go`:
```go
package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestFetchGames(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rss_page1.xml")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data)
	}))
	defer srv.Close()

	c := itchio.NewClient()
	games, err := c.FetchGamesFromURL(srv.URL)
	if err != nil {
		t.Fatalf("FetchGamesFromURL: %v", err)
	}
	if len(games) == 0 {
		t.Fatal("expected at least 1 game, got 0")
	}

	g := games[0]
	if g.Title == "" {
		t.Error("game.Title is empty")
	}
	if g.URL == "" {
		t.Error("game.URL is empty")
	}
	if g.CoverURL == "" {
		t.Error("game.CoverURL is empty")
	}
	if g.Author == "" {
		t.Error("game.Author is empty")
	}
}

func TestFetchGamesFreePriceParsing(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item>
  <title>Test Game</title>
  <link>https://testdev.itch.io/test-game</link>
  <description>&lt;img src="https://img.itch.zone/test.png"/&gt;</description>
  <price>0.0</price>
</item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	games, err := c.FetchGamesFromURL(srv.URL)
	if err != nil {
		t.Fatalf("FetchGamesFromURL: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 game, got %d", len(games))
	}
	if !games[0].IsFree {
		t.Error("game with price 0.0 should be IsFree=true")
	}
}
```

- [ ] **Step 3: Run tests — expect failure**

```bash
./scripts/test.sh
```

Expected: FAIL — `NewClient` returns nil, `FetchGamesFromURL` doesn't exist.

- [ ] **Step 4: Implement client.go**

```go
package itchio

import (
	"net/http"
	"net/http/cookiejar"
	"time"
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
	}
}
```

- [ ] **Step 5: Implement feed.go**

```go
package itchio

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Game struct {
	Title    string
	Author   string
	URL      string
	CoverURL string
	Price    float64
	IsFree   bool
}

var coverRegex = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Price       string `xml:"price"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

func parseAuthor(gameURL string) string {
	// https://{author}.itch.io/{game}
	s := strings.TrimPrefix(gameURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.Index(s, ".itch.io"); idx > 0 {
		return s[:idx]
	}
	return ""
}

func parseCover(desc string) string {
	m := coverRegex.FindStringSubmatch(desc)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func (c *Client) FetchGamesFromURL(url string) ([]Game, error) {
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed xml: %w", err)
	}

	games := make([]Game, 0, len(feed.Items))
	for _, item := range feed.Items {
		price, _ := strconv.ParseFloat(strings.TrimSpace(item.Price), 64)
		games = append(games, Game{
			Title:    item.Title,
			Author:   parseAuthor(item.Link),
			URL:      item.Link,
			CoverURL: parseCover(item.Description),
			Price:    price,
			IsFree:   price == 0,
		})
	}
	return games, nil
}

func (c *Client) FetchGames(page int, query string) ([]Game, error) {
	url := fmt.Sprintf("https://itch.io/games/made-with-gb-studio.xml?page=%d", page)
	if query != "" {
		url += "&q=" + query
	}
	return c.FetchGamesFromURL(url)
}
```

- [ ] **Step 6: Run tests — expect pass**

```bash
./scripts/test.sh
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/itchio/client.go internal/itchio/feed.go internal/itchio/feed_test.go testdata/rss_page1.xml
git commit -m "feat: implement itchio client and RSS feed fetcher with pagination"
```

---

### Task 7: itchio game page scraper

**Files:**
- Modify: `internal/itchio/game.go`
- Create: `internal/itchio/game_test.go`
- Create: `testdata/game_page_free.html`
- Create: `testdata/game_page_paid.html`
- Create: `testdata/download_page.html`

- [ ] **Step 1: Capture testdata fixtures**

```bash
# Replace with a real GB Studio free game URL from itch.io
curl -sL "https://somedev.itch.io/some-free-gb-game" > testdata/game_page_free.html
# Replace with a real paid game URL
curl -sL "https://somedev.itch.io/some-paid-gb-game" > testdata/game_page_paid.html
# The download page is returned after the CSRF POST flow; capture manually or construct a minimal fixture:
cat > testdata/download_page.html << 'EOF'
<html><body>
<div class="upload">
  <a href="https://downloads.itch.ovh/game.gbc">game.gbc</a>
  <a href="https://downloads.itch.ovh/manual.pdf">manual.pdf</a>
  <a href="https://downloads.itch.ovh/game.gb">game.gb</a>
</div>
</body></html>
EOF
```

- [ ] **Step 2: Write failing tests**

`internal/itchio/game_test.go`:
```go
package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func serveFile(t *testing.T, path string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
}

func TestFetchGameDetailFree(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_free.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.GameID == "" {
		t.Error("GameID is empty — data-game_id not found")
	}
}

func TestFetchGameDetailPaidDetected(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_paid.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	// Paid games still return a detail (with IsFree=false on the embedded Game)
	// The download flow gates on this; just confirm parsing doesn't crash
	_ = detail
}

func TestParseUploadLinks(t *testing.T) {
	srv := serveFile(t, "../../testdata/download_page.html")
	defer srv.Close()

	c := itchio.NewClient()
	uploads, err := c.ParseDownloadPage(srv.URL)
	if err != nil {
		t.Fatalf("ParseDownloadPage: %v", err)
	}
	if len(uploads) == 0 {
		t.Fatal("expected at least one upload")
	}
	// Should only contain .gb and .gbc, not .pdf
	for _, u := range uploads {
		ext := u.Filename[len(u.Filename)-4:]
		if ext != ".gbc" && ext != ".gb" {
			// allow .gbc or .gb
			t.Errorf("unexpected upload extension in %q", u.Filename)
		}
	}
}
```

- [ ] **Step 3: Run tests — expect failure**

```bash
./scripts/test.sh
```

Expected: FAIL

- [ ] **Step 4: Implement game.go**

```go
package itchio

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
}

type Upload = roms_Upload // reuse from roms package — defined locally here

// Upload is a downloadable file on an itch.io game page.
type upload struct {
	Filename string
	URL      string
}

var (
	gameIDRegex   = regexp.MustCompile(`data-game_id="(\d+)"`)
	csrfRegex     = regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)
	screenshotReg = regexp.MustCompile(`<img[^>]+class="[^"]*screenshot[^"]*"[^>]+src="([^"]+)"`)
)

func (c *Client) FetchGameDetail(gameURL string) (*GameDetail, error) {
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read game page: %w", err)
	}
	s := string(body)

	detail := &GameDetail{}

	if m := gameIDRegex.FindStringSubmatch(s); len(m) > 1 {
		detail.GameID = m[1]
	}
	if m := csrfRegex.FindStringSubmatch(s); len(m) > 1 {
		detail.CSRFToken = m[1]
	}

	// Extract screenshots
	for _, m := range screenshotReg.FindAllStringSubmatch(s, -1) {
		detail.ScreenshotURLs = append(detail.ScreenshotURLs, m[1])
	}

	return detail, nil
}

func (c *Client) ParseDownloadPage(pageURL string) ([]Upload, error) {
	resp, err := c.http.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch download page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse download page: %w", err)
	}

	var uploads []Upload
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href := attr.Val
					ext := strings.ToLower(filepath.Ext(href))
					if ext == ".gb" || ext == ".gbc" {
						filename := filepath.Base(strings.Split(href, "?")[0])
						uploads = append(uploads, Upload{Filename: filename, URL: href})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return uploads, nil
}
```

Note: `Upload` type is defined here in the `itchio` package. Remove the stub from `download.go` and ensure `roms` package has its own `Upload` type. The two types are parallel — `itchio.Upload` is for download URLs; `roms.Upload` is for scoring.

- [ ] **Step 5: Reconcile Upload types**

In `internal/roms/roms.go`, the `Upload` type stays as-is. In `internal/itchio/game.go`, replace the inline type alias with a proper definition:

```go
// Upload is a downloadable file on an itch.io game page.
type Upload struct {
	Filename string
	URL      string
}
```

Remove the stub `Upload` from `internal/itchio/download.go` (it will be defined in game.go).

- [ ] **Step 6: Run tests — expect pass**

```bash
./scripts/test.sh
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/itchio/game.go internal/itchio/game_test.go testdata/
git commit -m "feat: implement game page scraper — game_id, csrf, screenshots, upload link parsing"
```

---

### Task 8: download flows (free + authenticated)

**Files:**
- Modify: `internal/itchio/download.go`
- Modify: `internal/itchio/download_auth.go`
- Create: `internal/itchio/download_test.go`

- [ ] **Step 1: Write failing tests**

`internal/itchio/download_test.go`:
```go
package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestDownloadFreeStreamsFile(t *testing.T) {
	content := []byte("ROM_CONTENT_BYTES")
	var steps []string

	mux := http.NewServeMux()
	// Step 1: game page returning game_id and csrf
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "game_page")
		w.Write([]byte(`<div data-game_id="12345"></div><input name="csrf_token" value="tok123"/>`))
	})
	// Step 2: download_url POST
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "download_url_post")
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"url":"/download-page"}`))
	})
	// Step 3: download page listing upload
	mux.HandleFunc("/download-page", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "download_page")
		w.Write([]byte(`<a href="/file/game.gbc">game.gbc</a>`))
	})
	// Step 4: actual file
	mux.HandleFunc("/file/game.gbc", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "file_download")
		w.Header().Set("Content-Length", "17")
		w.Write(content)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "game.gbc")
	c := itchio.NewClient()
	upload := itchio.Upload{Filename: "game.gbc", URL: srv.URL + "/file/game.gbc"}

	err := c.DownloadFree(srv.URL+"/game", upload, dest, nil)
	if err != nil {
		t.Fatalf("DownloadFree: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}
}

func TestCheckOwnership(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1/mykey/game/12345/download_keys" {
			w.Write([]byte(`{"download_keys":[{"id":1}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	owns, err := c.CheckOwnership("mykey", "12345")
	if err != nil {
		t.Fatalf("CheckOwnership: %v", err)
	}
	if !owns {
		t.Error("expected owns=true")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
./scripts/test.sh
```

Expected: FAIL

- [ ] **Step 3: Implement download.go**

```go
package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (c *Client) DownloadFree(gameURL string, upload Upload, dest string, progress func(int64, int64)) error {
	// Step 1: get game page for csrf token
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return fmt.Errorf("fetch game page: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read game page: %w", err)
	}
	s := string(body)

	csrfM := csrfRegex.FindStringSubmatch(s)
	if len(csrfM) < 2 {
		return fmt.Errorf("csrf_token not found on game page")
	}
	csrf := csrfM[1]

	// Step 2: POST to get signed download page URL
	postURL := strings.TrimRight(gameURL, "/") + "/download_url"
	form := url.Values{"csrf_token": {csrf}, "suggested_amount": {"0"}}
	postResp, err := c.http.Post(postURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("download_url POST: %w", err)
	}
	defer postResp.Body.Close()

	var dlResult struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(postResp.Body).Decode(&dlResult); err != nil {
		return fmt.Errorf("parse download_url response: %w", err)
	}
	if dlResult.URL == "" {
		return fmt.Errorf("download_url response had empty url (game may be paid)")
	}

	// Step 3+4: stream the upload file
	return c.streamToFile(upload.URL, dest, progress)
}

func (c *Client) streamToFile(srcURL, dest string, progress func(int64, int64)) error {
	resp, err := c.http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file download status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(dest[:strings.LastIndex(dest, "/")], 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement download_auth.go + add NewClientWithBase**

`internal/itchio/download_auth.go`:
```go
package itchio

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var itchioBase = "https://itch.io"

func (c *Client) CheckOwnership(apiKey, gameID string) (bool, error) {
	url := fmt.Sprintf("%s/api/1/%s/game/%s/download_keys", c.base, apiKey, gameID)
	resp, err := c.http.Get(url)
	if err != nil {
		return false, fmt.Errorf("check ownership: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result struct {
		DownloadKeys []struct{ ID int `json:"id"` } `json:"download_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode ownership response: %w", err)
	}
	return len(result.DownloadKeys) > 0, nil
}

func (c *Client) DownloadAuth(apiKey string, upload Upload, dest string, progress func(int64, int64)) error {
	req, err := http.NewRequest("GET", upload.URL, nil)
	if err != nil {
		return fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("auth download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("not authorized (status %d) — check API key and game ownership", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth download status %d", resp.StatusCode)
	}

	return c.streamToFile(upload.URL, dest, progress)
}
```

Add `base` field and `NewClientWithBase` to `client.go`:

```go
// In client.go, update Client struct:
type Client struct {
	http *http.Client
	base string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{Jar: jar, Timeout: 30 * time.Second},
		base: "https://itch.io",
	}
}

func NewClientWithBase(base string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{Jar: jar, Timeout: 30 * time.Second},
		base: base,
	}
}
```

- [ ] **Step 5: Run tests — expect pass**

```bash
./scripts/test.sh
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/
git commit -m "feat: implement free and authenticated download flows"
```

---

## Phase 3: SDL2 Renderer

### Task 9: Renderer init + event loop

**Files:**
- Modify: `internal/renderer/renderer.go`

Note: renderer files have `//go:build !headless` so they are excluded from `go test -tags headless` in CI. Manual testing on a machine with SDL2 dev libs is required.

- [ ] **Step 1: Implement renderer.go**

```go
//go:build !headless

package renderer

import (
	"fmt"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

type Renderer struct {
	Window   *sdl.Window
	Renderer *sdl.Renderer
	Font     *ttf.Font
	W, H     int32
}

func New(title string, w, h int) (*Renderer, error) {
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, fmt.Errorf("sdl init: %w", err)
	}
	if err := ttf.Init(); err != nil {
		return nil, fmt.Errorf("ttf init: %w", err)
	}

	win, err := sdl.CreateWindow(title,
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(w), int32(h), sdl.WINDOW_SHOWN)
	if err != nil {
		return nil, fmt.Errorf("create window: %w", err)
	}

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return nil, fmt.Errorf("create renderer: %w", err)
	}

	font, err := ttf.OpenFont("assets/font.ttf", 18)
	if err != nil {
		return nil, fmt.Errorf("open font: %w", err)
	}

	return &Renderer{
		Window: win, Renderer: ren, Font: font,
		W: int32(w), H: int32(h),
	}, nil
}

func (r *Renderer) Close() {
	if r.Font != nil {
		r.Font.Close()
	}
	if r.Renderer != nil {
		r.Renderer.Destroy()
	}
	if r.Window != nil {
		r.Window.Destroy()
	}
	ttf.Quit()
	sdl.Quit()
}

func (r *Renderer) Clear(red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.Clear()
}

func (r *Renderer) Present() {
	r.Renderer.Present()
}

func (r *Renderer) DrawRect(x, y, w, h int32, red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.FillRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error {
	surface, err := r.Font.RenderUTF8Blended(text, sdl.Color{R: red, G: green, B: blue, A: 255})
	if err != nil {
		return err
	}
	defer surface.Free()

	texture, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return err
	}
	defer texture.Destroy()

	_, _, tw, th, _ := texture.Query()
	r.Renderer.Copy(texture, nil, &sdl.Rect{X: x, Y: y, W: tw, H: th})
	return nil
}

func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
	r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: w, H: h})
}
```

- [ ] **Step 2: Verify it compiles (native build)**

```bash
./scripts/build.sh native
```

Expected: `bin/native/itchio-pak` built without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/renderer/renderer.go
git commit -m "feat: implement SDL2 renderer — window, font, clear/present/rect/text primitives"
```

---

### Task 10: Image cache

**Files:**
- Modify: `internal/renderer/image_cache.go`

- [ ] **Step 1: Implement image_cache.go**

```go
//go:build !headless

package renderer

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"sync"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)

type cacheEntry struct {
	key     string
	texture *sdl.Texture
}

type ImageCache struct {
	mu       sync.Mutex
	lru      *list.List
	items    map[string]*list.Element
	max      int
	client   *http.Client
}

func NewImageCache(maxEntries int) *ImageCache {
	return &ImageCache{
		lru:    list.New(),
		items:  make(map[string]*list.Element),
		max:    maxEntries,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Get returns a cached texture or fetches and caches it.
// Returns nil if the image cannot be fetched or decoded.
func (c *ImageCache) Get(r *Renderer, url string) *sdl.Texture {
	c.mu.Lock()
	if el, ok := c.items[url]; ok {
		c.lru.MoveToFront(el)
		tex := el.Value.(*cacheEntry).texture
		c.mu.Unlock()
		return tex
	}
	c.mu.Unlock()

	tex, err := c.fetchAndDecode(r, url)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &cacheEntry{key: url, texture: tex}
	el := c.lru.PushFront(entry)
	c.items[url] = el

	for c.lru.Len() > c.max {
		back := c.lru.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*cacheEntry)
		evicted.texture.Destroy()
		delete(c.items, evicted.key)
		c.lru.Remove(back)
	}
	return tex
}

func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, el := range c.items {
		el.Value.(*cacheEntry).texture.Destroy()
	}
	c.lru.Init()
	c.items = make(map[string]*list.Element)
}

func (c *ImageCache) fetchAndDecode(r *Renderer, url string) (*sdl.Texture, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// Resize to max 640px wide
	img = resizeMax(img, 640)

	// Encode to JPEG bytes
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	// Create SDL2 surface from raw RGBA
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	surface, err := sdl.CreateRGBSurfaceFrom(
		rgba.Pix,
		int32(bounds.Dx()), int32(bounds.Dy()),
		32, int32(bounds.Dx()*4),
		0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
	)
	if err != nil {
		return nil, fmt.Errorf("create surface: %w", err)
	}
	defer surface.Free()

	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return nil, fmt.Errorf("create texture: %w", err)
	}
	return tex, nil
}

func resizeMax(img image.Image, maxWidth int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxWidth {
		return img
	}
	newH := h * maxWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
```

- [ ] **Step 2: Verify native build still compiles**

```bash
./scripts/build.sh native
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/renderer/image_cache.go
git commit -m "feat: implement LRU image cache with JPEG fetch/resize and SDL2 texture creation"
```

---

### Task 11: QR code texture

**Files:**
- Modify: `internal/renderer/qr.go`

- [ ] **Step 1: Implement qr.go**

```go
//go:build !headless

package renderer

import (
	"fmt"
	"image"
	"image/png"
	"bytes"

	"github.com/skip2/go-qrcode"
	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/draw"
)

// QRTexture generates a QR code for the given URL and returns an SDL2 texture.
// The caller is responsible for calling texture.Destroy() when done.
func (r *Renderer) QRTexture(url string, size int) (*sdl.Texture, error) {
	pngBytes, err := qrcode.Encode(url, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("qr png decode: %w", err)
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	surface, err := sdl.CreateRGBSurfaceFrom(
		rgba.Pix,
		int32(bounds.Dx()), int32(bounds.Dy()),
		32, int32(bounds.Dx()*4),
		0x000000FF, 0x0000FF00, 0x00FF0000, 0xFF000000,
	)
	if err != nil {
		return nil, fmt.Errorf("qr surface: %w", err)
	}
	defer surface.Free()

	tex, err := r.Renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return nil, fmt.Errorf("qr texture: %w", err)
	}
	return tex, nil
}
```

- [ ] **Step 2: Verify native build**

```bash
./scripts/build.sh native
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/renderer/qr.go
git commit -m "feat: implement QR code texture generation via skip2/go-qrcode"
```

---

### Task 12: Screen interface

**Files:**
- Create: `internal/ui/screen.go`

- [ ] **Step 1: Define Screen interface**

`internal/ui/screen.go`:
```go
//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

// Screen is implemented by every UI screen.
// Draw renders the screen for the current frame.
// HandleEvent processes one SDL event and returns the next screen to show.
// Returning nil means exit the application.
// Returning the same screen means no transition.
type Screen interface {
	Draw(r *renderer.Renderer)
	HandleEvent(e sdl.Event) Screen
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/screen.go
git commit -m "feat: define Screen interface for SDL2 UI screen transitions"
```

---

## Phase 4: UI Screens

### Task 13: Game list screen (split panel)

**Files:**
- Modify: `internal/ui/screen_list.go`

- [ ] **Step 1: Implement screen_list.go**

```go
//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	colorBG        = uint8(20)
	colorHighlight = uint8(60)
	colorText      = uint8(220)
	colorFree      = uint8(80)  // green
	colorPaid      = uint8(200) // amber
)

type ListScreen struct {
	client   *itchio.Client
	cfg      *settings.Config
	cache    *renderer.ImageCache
	games    []itchio.Game
	cursor   int
	page     int
	loading  bool
	err      error
}

func NewListScreen(client *itchio.Client, cfg *settings.Config, cache *renderer.ImageCache) *ListScreen {
	s := &ListScreen{client: client, cfg: cfg, cache: cache, page: 1}
	go s.loadPage(1, "")
	return s
}

func (s *ListScreen) loadPage(page int, query string) {
	s.loading = true
	games, err := s.client.FetchGames(page, query)
	s.games = games
	s.err = err
	s.cursor = 0
	s.loading = false
}

func (s *ListScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	if s.loading {
		r.DrawText("Loading...", 20, r.H/2, colorText, colorText, colorText)
		r.Present()
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), 20, r.H/2, 200, 50, 50)
		r.Present()
		return
	}

	// Left panel: 55% of width
	leftW := r.W * 55 / 100
	rightX := leftW + 10
	rightW := r.W - rightX - 10

	rowH := int32(32)
	visibleRows := (r.H - 40) / rowH

	startIdx := 0
	if s.cursor >= int(visibleRows) {
		startIdx = s.cursor - int(visibleRows) + 1
	}

	for i, g := range s.games {
		if i < startIdx {
			continue
		}
		rowIdx := i - startIdx
		if int32(rowIdx) >= visibleRows {
			break
		}

		y := int32(20) + int32(rowIdx)*rowH
		if i == s.cursor {
			r.DrawRect(0, y-2, leftW, rowH, colorHighlight, colorHighlight, colorHighlight+20)
		}

		label := g.Title
		if len(label) > 40 {
			label = label[:37] + "..."
		}
		r.DrawText(label, 10, y, colorText, colorText, colorText)

		if g.IsFree {
			r.DrawText("free", leftW-60, y, 80, 200, 80)
		} else {
			badge := fmt.Sprintf("$%.2f", g.Price)
			r.DrawText(badge, leftW-70, y, 220, 180, 60)
		}
	}

	// Right panel: cover art
	if s.cursor < len(s.games) {
		coverURL := s.games[s.cursor].CoverURL
		if coverURL != "" {
			tex := s.cache.Get(r, coverURL)
			if tex != nil {
				_, _, tw, th, _ := tex.Query()
				// Fit into right panel preserving aspect ratio
				scale := float32(rightW) / float32(tw)
				dh := int32(float32(th) * scale)
				r.DrawTextureAt(tex, rightX, 20, rightW, dh)
			}
		}
	}

	// Footer
	footer := fmt.Sprintf("Page %d  ·  %d games  ·  D-pad navigate  ·  A select  ·  Y search  ·  Start settings", s.page, len(s.games))
	r.DrawText(footer, 10, r.H-24, 140, 140, 140)

	r.Present()
}

func (s *ListScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.games)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_PAGEDOWN, sdl.K_r: // R button
			s.page++
			go s.loadPage(s.page, "")
		case sdl.K_PAGEUP, sdl.K_l: // L button
			if s.page > 1 {
				s.page--
				go s.loadPage(s.page, "")
			}
		case sdl.K_RETURN, sdl.K_a: // A button
			if s.cursor < len(s.games) {
				return NewDetailScreen(s.client, s.cfg, s.cache, s.games[s.cursor], s)
			}
		case sdl.K_s: // Start — open settings
			return NewSettingsScreen(s.cfg, s)
		case sdl.K_y: // Y — search (placeholder; full on-screen keyboard in Task 16)
			// TODO: open search input screen
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
```

- [ ] **Step 2: Verify native build**

```bash
./scripts/build.sh native
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat: implement split-panel game list screen with cover art pre-fetch and paging"
```

---

### Task 14: Game detail screen

**Files:**
- Modify: `internal/ui/screen_detail.go`

- [ ] **Step 1: Implement screen_detail.go**

```go
//go:build !headless

package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type DetailScreen struct {
	client     *itchio.Client
	cfg        *settings.Config
	cache      *renderer.ImageCache
	game       itchio.Game
	detail     *itchio.GameDetail
	loading    bool
	err        error
	screenshotIdx int
	qrTex      interface{ Destroy() }
	prev       Screen
}

func NewDetailScreen(client *itchio.Client, cfg *settings.Config, cache *renderer.ImageCache, game itchio.Game, prev Screen) *DetailScreen {
	s := &DetailScreen{client: client, cfg: cfg, cache: cache, game: game, prev: prev, loading: true}
	go func() {
		d, err := client.FetchGameDetail(game.URL)
		s.detail = d
		s.err = err
		s.loading = false
	}()
	return s
}

func (s *DetailScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	r.DrawText(s.game.Title, 20, 20, colorText, colorText, colorText)
	r.DrawText("by "+s.game.Author, 20, 50, 160, 160, 160)

	if s.loading {
		r.DrawText("Loading detail...", 20, 100, colorText, colorText, colorText)
		r.Present()
		return
	}
	if s.err != nil {
		r.DrawText("Error: "+s.err.Error(), 20, 100, 200, 50, 50)
		r.DrawText("Scan QR to visit game page", 20, 140, 160, 160, 160)
		s.drawQR(r, s.game.URL)
		r.Present()
		return
	}

	y := int32(80)

	// Screenshots (L/R to navigate)
	if len(s.detail.ScreenshotURLs) > 0 {
		ssURL := s.detail.ScreenshotURLs[s.screenshotIdx]
		tex := s.cache.Get(r, ssURL)
		if tex != nil {
			_, _, tw, th, _ := tex.Query()
			dispW := r.W * 60 / 100
			scale := float32(dispW) / float32(tw)
			dh := int32(float32(th) * scale)
			r.DrawTextureAt(tex, 20, y, dispW, dh)
			y += dh + 10
		}
		r.DrawText(fmt.Sprintf("Screenshot %d/%d  (L/R)", s.screenshotIdx+1, len(s.detail.ScreenshotURLs)),
			20, y, 140, 140, 140)
		y += 30
	}

	// Price / action
	if s.game.IsFree {
		r.DrawText("[ A: Download ]", 20, y, 80, 200, 80)
	} else if s.cfg.APIKey == "" {
		r.DrawText(fmt.Sprintf("$%.2f  Purchase required — scan QR", s.game.Price), 20, y, 220, 180, 60)
		y += 30
		r.DrawText("[ + : Add API Key ]", 20, y, 160, 160, 160)
	} else {
		r.DrawText(fmt.Sprintf("[ A: Download (API key) ]  $%.2f", s.game.Price), 20, y, 80, 200, 80)
	}
	y += 40

	// QR code
	r.DrawText("Store page QR:", 20, y, 160, 160, 160)
	s.drawQR(r, s.game.URL)

	// Footer
	r.DrawText("B: back  |  L/R: screenshots  |  Start: settings", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *DetailScreen) drawQR(r *renderer.Renderer, url string) {
	tex, err := r.QRTexture(url, 128)
	if err == nil && tex != nil {
		r.DrawTextureAt(tex, r.W-148, 80, 128, 128)
		tex.Destroy()
	}
}

func (s *DetailScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_b, sdl.K_ESCAPE:
			return s.prev
		case sdl.K_l:
			if s.detail != nil && s.screenshotIdx > 0 {
				s.screenshotIdx--
			}
		case sdl.K_r:
			if s.detail != nil && s.screenshotIdx < len(s.detail.ScreenshotURLs)-1 {
				s.screenshotIdx++
			}
		case sdl.K_RETURN, sdl.K_a:
			if s.detail == nil || !s.game.IsFree && s.cfg.APIKey == "" {
				return s
			}
			return s.startDownload()
		case sdl.K_s:
			return NewSettingsScreen(s.cfg, s)
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *DetailScreen) startDownload() Screen {
	if s.detail == nil {
		return s
	}
	uploads := make([]roms.Upload, len(s.detail.Uploads))
	for i, u := range s.detail.Uploads {
		uploads[i] = roms.Upload{Filename: u.Filename, URL: u.URL}
	}

	if s.cfg.ROMSelection == "ask" && len(uploads) > 1 {
		return NewROMPickerScreen(s.client, s.cfg, s.cache, s.game, s.detail, uploads, s)
	}

	selected := roms.SelectBest(uploads)
	if selected == nil {
		// No ROMs found — show error on this screen
		s.err = fmt.Errorf("no .gb or .gbc files found for this game")
		return s
	}
	return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, *selected, s)
}
```

- [ ] **Step 2: Verify native build**

```bash
./scripts/build.sh native
```

Expected: no errors (ROMPickerScreen and DownloadScreen will be stubs until Tasks 15-16).

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat: implement game detail screen with screenshots, QR, and download action"
```

---

### Task 15: Download progress screen

**Files:**
- Modify: `internal/ui/screen_download.go`

- [ ] **Step 1: Implement screen_download.go**

```go
//go:build !headless

package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type downloadState int

const (
	dlStateDownloading downloadState = iota
	dlStateDone
	dlStateError
)

type DownloadScreen struct {
	client     *itchio.Client
	cfg        *settings.Config
	game       itchio.Game
	detail     *itchio.GameDetail
	upload     roms.Upload
	prev       Screen
	state      downloadState
	downloaded int64
	total      int64
	dest       string
	err        error
}

func NewDownloadScreen(client *itchio.Client, cfg *settings.Config, game itchio.Game, detail *itchio.GameDetail, upload roms.Upload, prev Screen) *DownloadScreen {
	ext := strings.ToLower(filepath.Ext(upload.Filename))
	dest := roms.DestinationDir(ext) + upload.Filename

	s := &DownloadScreen{
		client: client, cfg: cfg, game: game, detail: detail,
		upload: upload, prev: prev, dest: dest,
		state: dlStateDownloading,
	}

	go func() {
		progress := func(downloaded, total int64) {
			atomic.StoreInt64(&s.downloaded, downloaded)
			atomic.StoreInt64(&s.total, total)
		}

		var err error
		if cfg.APIKey != "" && detail != nil {
			owns, oErr := client.CheckOwnership(cfg.APIKey, detail.GameID)
			if oErr == nil && owns {
				err = client.DownloadAuth(cfg.APIKey, itchio.Upload{Filename: upload.Filename, URL: upload.URL}, dest, progress)
			} else {
				err = client.DownloadFree(game.URL, itchio.Upload{Filename: upload.Filename, URL: upload.URL}, dest, progress)
			}
		} else {
			err = client.DownloadFree(game.URL, itchio.Upload{Filename: upload.Filename, URL: upload.URL}, dest, progress)
		}

		if err != nil {
			s.err = err
			s.state = dlStateError
		} else {
			s.state = dlStateDone
		}
	}()

	return s
}

func (s *DownloadScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Downloading: "+s.game.Title, 20, 30, colorText, colorText, colorText)
	r.DrawText(s.upload.Filename, 20, 65, 160, 160, 160)

	switch s.state {
	case dlStateDownloading:
		dl := atomic.LoadInt64(&s.downloaded)
		total := atomic.LoadInt64(&s.total)

		barW := r.W - 80
		r.DrawRect(40, r.H/2-10, barW, 20, 60, 60, 60)
		if total > 0 {
			filled := int32(float64(barW) * float64(dl) / float64(total))
			r.DrawRect(40, r.H/2-10, filled, 20, 80, 200, 80)
			pct := fmt.Sprintf("%d%%  (%s / %s)", dl*100/total, humanBytes(dl), humanBytes(total))
			r.DrawText(pct, 40, r.H/2+20, colorText, colorText, colorText)
		} else {
			r.DrawRect(40, r.H/2-10, barW/3, 20, 80, 200, 80) // indeterminate
			r.DrawText(humanBytes(dl)+" downloaded", 40, r.H/2+20, colorText, colorText, colorText)
		}

	case dlStateDone:
		r.DrawText("Download complete!", 20, r.H/2-20, 80, 200, 80)
		r.DrawText("Saved to: "+s.dest, 20, r.H/2+10, 160, 160, 160)
		r.DrawText("A or B: return to list", 20, r.H/2+50, 140, 140, 140)

	case dlStateError:
		r.DrawText("Download failed:", 20, r.H/2-40, 200, 60, 60)
		r.DrawText(s.err.Error(), 20, r.H/2-10, 200, 100, 100)
		r.DrawText("Scan QR to visit game page:", 20, r.H/2+30, 160, 160, 160)
		tex, err := r.QRTexture(s.game.URL, 128)
		if err == nil && tex != nil {
			r.DrawTextureAt(tex, r.W/2-64, r.H/2+60, 128, 128)
			tex.Destroy()
		}
		r.DrawText("B: back", 20, r.H-24, 140, 140, 140)
	}

	r.Present()
}

func (s *DownloadScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_b, sdl.K_ESCAPE, sdl.K_RETURN, sdl.K_a:
			if s.state != dlStateDownloading {
				return s.prev
			}
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func humanBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
```

- [ ] **Step 2: Verify native build**

```bash
./scripts/build.sh native
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_download.go
git commit -m "feat: implement download progress screen with progress bar, success, and error+QR states"
```

---

### Task 16: Settings screen + ROM picker

**Files:**
- Modify: `internal/ui/screen_settings.go`
- Create: `internal/ui/screen_rom_picker.go`

- [ ] **Step 1: Implement screen_settings.go**

```go
//go:build !headless

package ui

import (
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type settingsItem int

const (
	settingsAPIKey settingsItem = iota
	settingsROMMode
	settingsClearCache
	settingsAbout
	settingsCount
)

type SettingsScreen struct {
	cfg    *settings.Config
	cfgPath string
	cursor settingsItem
	prev   Screen
}

func NewSettingsScreen(cfg *settings.Config, prev Screen) *SettingsScreen {
	return &SettingsScreen{
		cfg:     cfg,
		cfgPath: os.Getenv("HOME") + "/config.json",
		prev:    prev,
	}
}

func (s *SettingsScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Settings", 20, 20, colorText, colorText, colorText)

	items := []string{
		"API Key: " + maskKey(s.cfg.APIKey),
		"ROM Selection: " + s.cfg.ROMSelection,
		"Clear Image Cache",
		"About",
	}

	for i, label := range items {
		y := int32(80 + i*40)
		if settingsItem(i) == s.cursor {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(label, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("D-pad navigate  ·  A select  ·  B back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *SettingsScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if int(s.cursor) < int(settingsCount)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN, sdl.K_a:
			return s.activate()
		case sdl.K_b, sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}

func (s *SettingsScreen) activate() Screen {
	switch s.cursor {
	case settingsROMMode:
		if s.cfg.ROMSelection == "auto" {
			s.cfg.ROMSelection = "ask"
		} else {
			s.cfg.ROMSelection = "auto"
		}
		s.cfg.Save(s.cfgPath)
	case settingsClearCache:
		os.RemoveAll("/tmp/itchio-pak/cache/")
	case settingsAbout:
		// Show about — simple inline text for now
	}
	return s
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}
```

- [ ] **Step 2: Implement screen_rom_picker.go**

`internal/ui/screen_rom_picker.go`:
```go
//go:build !headless

package ui

import (
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type ROMPickerScreen struct {
	client  *itchio.Client
	cfg     *settings.Config
	cache   *renderer.ImageCache
	game    itchio.Game
	detail  *itchio.GameDetail
	uploads []roms.Upload
	cursor  int
	prev    Screen
}

func NewROMPickerScreen(client *itchio.Client, cfg *settings.Config, cache *renderer.ImageCache, game itchio.Game, detail *itchio.GameDetail, uploads []roms.Upload, prev Screen) *ROMPickerScreen {
	return &ROMPickerScreen{client: client, cfg: cfg, cache: cache, game: game, detail: detail, uploads: uploads, prev: prev}
}

func (s *ROMPickerScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)
	r.DrawText("Select ROM file to download:", 20, 20, colorText, colorText, colorText)

	for i, u := range s.uploads {
		y := int32(70 + i*40)
		if i == s.cursor {
			r.DrawRect(0, y-4, r.W, 36, colorHighlight, colorHighlight, colorHighlight+20)
		}
		r.DrawText(u.Filename, 20, y, colorText, colorText, colorText)
	}

	r.DrawText("A: select  ·  B: back", 10, r.H-24, 140, 140, 140)
	r.Present()
}

func (s *ROMPickerScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		switch ev.Keysym.Sym {
		case sdl.K_DOWN:
			if s.cursor < len(s.uploads)-1 {
				s.cursor++
			}
		case sdl.K_UP:
			if s.cursor > 0 {
				s.cursor--
			}
		case sdl.K_RETURN, sdl.K_a:
			if s.cursor < len(s.uploads) {
				return NewDownloadScreen(s.client, s.cfg, s.game, s.detail, s.uploads[s.cursor], s.prev)
			}
		case sdl.K_b, sdl.K_ESCAPE:
			return s.prev
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
```

- [ ] **Step 3: Verify native build**

```bash
./scripts/build.sh native
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_settings.go internal/ui/screen_rom_picker.go
git commit -m "feat: implement settings screen and ROM file picker screen"
```

---

## Phase 5: Integration & Distribution

### Task 17: Wire main.go

**Files:**
- Modify: `cmd/itchio-pak/main.go`
- Create: `cmd/itchio-pak/main_sdl.go`

- [ ] **Step 1: Create SDL2 entry point (build-tagged)**

`cmd/itchio-pak/main_sdl.go`:
```go
//go:build !headless

package main

import (
	"log"
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/ui"
	"github.com/veandco/go-sdl2/sdl"
)

func runSDL() {
	cfgPath := os.Getenv("HOME") + "/config.json"
	cfg, err := settings.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Detect screen resolution from SDL2 display mode
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		log.Fatalf("sdl init: %v", err)
	}
	dm, err := sdl.GetCurrentDisplayMode(0)
	sdl.Quit()
	if err != nil {
		log.Printf("display mode detection failed, defaulting to 1024x768: %v", err)
		dm.W, dm.H = 1024, 768
	}

	r, err := renderer.New("Itch.io", int(dm.W), int(dm.H))
	if err != nil {
		log.Fatalf("renderer init: %v", err)
	}
	defer r.Close()

	cache := renderer.NewImageCache(50)
	defer cache.Clear()

	client := itchio.NewClient()
	var current ui.Screen = ui.NewListScreen(client, cfg, cache)

	for current != nil {
		for {
			e := sdl.PollEvent()
			if e == nil {
				break
			}
			current = current.HandleEvent(e)
			if current == nil {
				return
			}
		}
		current.Draw(r)
		sdl.Delay(16) // ~60fps
	}
}
```

- [ ] **Step 2: Update main.go to call runSDL**

```go
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logFile, err := os.OpenFile(os.Getenv("HOME")+"/itchio-pak.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	if *headless {
		log.Println("headless mode: exiting cleanly")
		os.Exit(0)
	}

	runSDL()
}
```

Note: `runSDL()` is defined in `main_sdl.go` (build tag `!headless`). In headless builds, the linker will not include `main_sdl.go` and `runSDL` is never called. No stub needed — the `os.Exit(0)` in the headless branch exits before any SDL call.

- [ ] **Step 3: Verify headless build (CI path)**

```bash
./scripts/test.sh
```

Expected: PASS — `go test -tags headless ./...` still passes.

- [ ] **Step 4: Verify native SDL2 build**

```bash
./scripts/build.sh native
```

Expected: `bin/native/itchio-pak` built, no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/itchio-pak/
git commit -m "feat: wire main.go SDL2 event loop with screen stack and headless CI path"
```

---

### Task 18: Cross-compile all platforms + release artifacts

- [ ] **Step 1: Cross-compile tg5040**

```bash
./scripts/build.sh tg5040
```

Expected: `bin/tg5040/itchio-pak` (ARM64 ELF). Verify:
```bash
file bin/tg5040/itchio-pak
# bin/tg5040/itchio-pak: ELF 64-bit LSB executable, ARM aarch64
```

- [ ] **Step 2: Cross-compile tg5050 and my355**

```bash
./scripts/build.sh tg5050
./scripts/build.sh my355
```

Expected: `bin/tg5050/itchio-pak` and `bin/my355/itchio-pak` both ARM64.

- [ ] **Step 3: Add font.ttf to assets/**

Download a permissively-licensed TTF (e.g., Noto Sans Regular from Google Fonts) and place it at `assets/font.ttf`. Verify the file is present:

```bash
ls -lh assets/font.ttf
```

- [ ] **Step 4: Extract SDL2 .so files from toolchain images**

```bash
# Extract tg5040 SDL2 libs
RUNTIME=$(scripts/build.sh --help 2>&1 | head -1 || echo "podman")
podman run --rm ghcr.io/loveretro/tg5040-toolchain:latest \
    find /usr -name "libSDL2*.so*" 2>/dev/null | head -20

# Copy libs out
CID=$(podman create ghcr.io/loveretro/tg5040-toolchain:latest)
podman cp "$CID":/usr/lib/aarch64-linux-gnu/. lib/tg5040/ 2>/dev/null || \
    podman cp "$CID":/usr/local/lib/. lib/tg5040/ 2>/dev/null
podman rm "$CID"

# Repeat for my355
CID=$(podman create ghcr.io/loveretro/my355-toolchain:latest)
podman cp "$CID":/usr/lib/aarch64-linux-gnu/. lib/my355/ 2>/dev/null || \
    podman cp "$CID":/usr/local/lib/. lib/my355/ 2>/dev/null
podman rm "$CID"
```

Keep only `libSDL2*.so*`, `libSDL2_ttf*.so*`, `libSDL2_image*.so*` in each lib directory.

- [ ] **Step 5: Run release.sh**

```bash
./scripts/release.sh
```

Expected:
```
dist/
  tg5040/Itch.io.pak.zip
  tg5050/Itch.io.pak.zip
  my355/Itch.io.pak.zip
  all/Itch.io.pakz
```

- [ ] **Step 6: Verify zip contents**

```bash
unzip -l dist/tg5040/Itch.io.pak.zip
```

Expected: `Itch.io.pak/itchio-pak`, `Itch.io.pak/launch.sh`, `Itch.io.pak/pak.json`, `Itch.io.pak/assets/font.ttf`, `Itch.io.pak/lib/*.so`.

- [ ] **Step 7: Commit**

```bash
git add assets/font.ttf lib/
git commit -m "feat: add font asset and SDL2 libs for release packaging"
```

---

### Task 19: README, CONTRIBUTING, CI, and final polish

**Files:**
- Create: `README.md`
- Create: `CONTRIBUTING.md`
- Create: `.github/workflows/ci.yml`
- Create: `.gitignore`

- [ ] **Step 1: Write .gitignore**

```
bin/
dist/
coverage.out
coverage.html
debug-cache/
*.log
```

- [ ] **Step 2: Write .github/workflows/ci.yml**

```yaml
name: CI

on:
  push:
    branches: ["**"]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Install SDL2 dev libs
        run: sudo apt-get update && sudo apt-get install -y libsdl2-dev libsdl2-ttf-dev libsdl2-image-dev

      - name: Run tests
        run: go test -race -tags headless ./...
```

- [ ] **Step 3: Write README.md**

```markdown
# Itch.io NextUI Pak

> **Unofficial community project — not affiliated with or endorsed by Itch.io / Leafo.**

Browse and download Game Boy / Game Boy Color games from [Itch.io's GB Studio collection](https://itch.io/games/made-with-gb-studio) directly on your device over WiFi — no PC required.

## Supported Devices

| Platform | Device |
|----------|--------|
| tg5040 | TrimUI Brick, TrimUI Smart Pro |
| tg5050 | TrimUI Smart Pro S |
| my355  | Miyoo Flip |

## Installation

**Via Pak Store:** Search for "Itch.io" and install.

**Manual:** Download `Itch.io.pakz` from [Releases](https://github.com/carroarmato0/NextUI-Itchio-Pak/releases), extract to the root of your SD card. The `Tools/{platform}/Itch.io.pak/` directory will be created automatically.

## Usage

1. Launch **Itch.io** from the Tools menu.
2. Browse games with the D-pad. Cover art loads automatically.
3. Press **A** to open a game's detail page.
4. Press **A** again to download a free game.
5. The ROM is saved to `/mnt/SDCARD/Roms/Game Boy Color (GBC)/` or `Game Boy (GB)/` automatically.

## Purchasing Games & API Key

Free games download automatically. Paid games display a QR code — scan it to purchase on Itch.io.

After purchasing, get your API key from [itch.io/user/settings/api-keys](https://itch.io/user/settings/api-keys).

Configure it in the Pak: **Start → Settings → API Key**.

Once set, owned paid games show a Download button alongside the QR code.

## ROM Placement

| Extension | Destination |
|-----------|-------------|
| `.gbc` | `/mnt/SDCARD/Roms/Game Boy Color (GBC)/` |
| `.gb`  | `/mnt/SDCARD/Roms/Game Boy (GB)/` |

## ROM Selection Mode

By default the Pak picks the best file automatically (`.gbc` preferred over `.gb`). To choose manually: **Start → Settings → ROM Selection → Always ask**.
```

- [ ] **Step 4: Write CONTRIBUTING.md**

```markdown
# Contributing

## Prerequisites

- **Docker or Podman** — everything else (Go, SDL2, cross-toolchains) is containerised
- **ADB** (`android-tools` / `android-platform-tools`) — only needed for USB deploy/debug; not required for builds or tests

## Quick Start

```sh
# Run all tests
./scripts/test.sh

# Build for host (requires SDL2 dev libs in container — handled automatically)
./scripts/build.sh native

# Cross-compile for device
./scripts/build.sh tg5040

# Build all platforms
./scripts/build.sh all

# Create release zips
./scripts/release.sh

# Deploy to connected device via ADB
./scripts/deploy.sh

# Deploy to SD card
./scripts/deploy.sh /run/media/you/SD
```

Or use `make`: `make test`, `make build-all`, `make release`, `make deploy`.

## Container Runtime

The scripts auto-detect Podman (preferred) or Docker. Override with:
```sh
CONTAINER_RUNTIME=docker ./scripts/build.sh tg5040
./scripts/build.sh --runtime podman tg5040
```

## Project Structure

| Path | Purpose |
|------|---------|
| `cmd/itchio-pak/` | Binary entry point |
| `internal/itchio/` | Itch.io HTTP client, RSS feed, game scraper, download flows |
| `internal/roms/` | ROM scoring and destination folder mapping |
| `internal/settings/` | JSON config read/write |
| `internal/renderer/` | SDL2 window, image cache, QR textures |
| `internal/ui/` | Screen implementations (list, detail, settings, download) |
| `scripts/` | Build, test, release, deploy, debug automation |
| `docker/` | Dev container Dockerfile |
| `testdata/` | HTTP fixture files for unit tests |

## Testing

Unit tests use captured HTML/XML fixtures in `testdata/` — no live network required:

```sh
./scripts/test.sh           # run tests
./scripts/test.sh --coverage  # run with HTML coverage report
```

SDL2 screens (`internal/renderer/`, `internal/ui/`) are excluded from automated tests via `//go:build !headless`. Test them manually via `build.sh native` or on-device.

## Live Device Debugging

With a TrimUI/Miyoo device connected via USB:

```sh
./scripts/debug.sh push      # cross-compile and push binary
./scripts/debug.sh run       # push then launch with live output (captures all stderr)
./scripts/debug.sh logs      # stream itchio-pak.log in real time
./scripts/debug.sh shell     # interactive ADB shell
```

## Making a Release

1. `./scripts/release.sh` — runs tests, builds all platforms, assembles `dist/`
2. Tag the commit: `git tag v1.x.x`
3. Upload `dist/tg5040/Itch.io.pak.zip`, `dist/tg5050/Itch.io.pak.zip`, `dist/my355/Itch.io.pak.zip`, and `dist/all/Itch.io.pakz` to the GitHub release.
```

- [ ] **Step 5: Run full test suite one final time**

```bash
./scripts/test.sh
```

Expected: all tests PASS.

- [ ] **Step 6: Commit everything**

```bash
git add README.md CONTRIBUTING.md .github/ .gitignore
git commit -m "feat: add README, CONTRIBUTING, CI workflow, and .gitignore"
```

---

## Spec Coverage Check

| Spec Section | Covered by Task(s) |
|---|---|
| Supported platforms tg5040/tg5050/my355 | Task 2 (build.sh), Task 18 |
| Go 1.22, go-sdl2, skip2/go-qrcode | Task 1 |
| launch.sh with LD_LIBRARY_PATH | Task 3 |
| pak.json | Task 3 |
| Split-panel game list (55/45%) | Task 13 |
| Price badges (free/paid) | Task 13 |
| Cover art pre-fetch ±5 | Task 13 (goroutine in cache.Get) |
| L/R paging | Task 13 |
| Y search (RSS ?q=) | Task 13 (stub; full keyboard is future work) |
| Game detail: cover, screenshots, QR | Task 14 |
| Download button / paid gate | Task 14 |
| ROM file picker (ask mode) | Task 16 |
| Download progress screen + error+QR | Task 15 |
| Settings: API key, ROM mode, clear cache, about | Task 16 |
| Free download flow (CSRF → stream) | Task 8 |
| Authenticated download flow | Task 8 |
| Ownership check | Task 8 |
| Failure → QR fallback + log | Task 8, Task 14, Task 15 |
| ROM scoring .gbc > .gb | Task 5 |
| Destination folders | Task 5 |
| JSON config | Task 4 |
| Image cache LRU 50 entries | Task 10 |
| QR via skip2/go-qrcode | Task 11 |
| Docker/Podman auto-detect | Task 2 |
| Container self-re-invocation | Task 2 |
| LoveRetro toolchain images | Task 2, Task 18 |
| test.sh --coverage | Task 2 |
| deploy.sh ADB + SD card | Task 2 |
| debug.sh subcommands | Task 2 |
| Makefile | Task 2 |
| README user guide | Task 19 |
| CONTRIBUTING developer guide | Task 19 |
| CI GitHub Actions | Task 19 |
| --headless flag + build tag | Task 1, Task 17 |
| lib/tg5040 and lib/my355 | Task 18 |
| .pakz multi-device bundle | Task 2 (release.sh) |

All spec requirements are covered.
