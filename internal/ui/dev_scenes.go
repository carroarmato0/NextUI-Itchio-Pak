//go:build !headless

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
)

// Dev scenes: construct any screen, in any interesting state, from fixture data.
//
// This lives inside package ui rather than a separate package because the states
// worth screenshotting are held in unexported fields — a detail screen with its
// confirm modal up, a settings page scrolled to the bottom. Reaching those from
// outside would mean exporting internals that nothing in production needs.
//
// Nothing here runs in the shipped app: no production path calls Scenes(), and
// the whole file is excluded from the headless build alongside the rest of the
// SDL code.

// SceneDeps carries everything a scene needs to build a screen. DevSetup fills
// one in from a scratch directory.
type SceneDeps struct {
	Client    *itchio.Client
	Cfg       *settings.Config
	CfgPath   string
	Cache     *renderer.ImageCache
	Inv       *inventory.Inventory
	InvPath   string
	CachePath string
	Games     []itchio.Game
	Detail    *itchio.GameDetail
	Theme     theme.Theme
}

// Scene is one named, reproducible screen state.
type Scene struct {
	Name string
	// Desc says what the scene is for, so `devshot --list` explains itself.
	Desc string
	// Build returns the screen, already in the state to capture.
	Build func(SceneDeps) Screen
}

// DevFixtureGames returns a deterministic game list chosen to exercise the
// layout rather than to look tidy: a very long title that must truncate, a
// non-Latin title that needs the fallback fonts, a free game, a paid one, and
// one with an unusually long tag list.
func DevFixtureGames() []itchio.Game {
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	return []itchio.Game{
		{
			Title: "Tobu Tobu Girl Deluxe", Author: "tangramgames",
			URL: "https://tangramgames.itch.io/tobu-tobu-girl-deluxe", IsFree: true,
			Tags:        []string{"platformer", "gameboy", "pixel-art", "retro", "arcade", "2d", "cute"},
			PublishedAt: base, Platform: "GBC",
		},
		{
			Title:  "A Very Long Game Title That Will Certainly Need Truncating On Screen",
			Author: "verbose-dev", URL: "https://verbose-dev.itch.io/long", IsFree: true,
			Tags: []string{"adventure"}, PublishedAt: base.Add(-time.Hour), Platform: "GB",
		},
		{
			Title: "夢天使 Yume Tenshi", Author: "スタジオ", // exercises fallback fonts
			URL: "https://studio.itch.io/yume-tenshi", IsFree: true,
			Tags: []string{"rpg", "jrpg"}, PublishedAt: base.Add(-2 * time.Hour), Platform: "GBC",
		},
		{
			Title: "Paid Puzzle Deluxe", Author: "indie-studio",
			URL: "https://indie-studio.itch.io/paid-puzzle", IsFree: false, Price: 4.99,
			Tags: []string{"puzzle"}, PublishedAt: base.Add(-3 * time.Hour), Platform: "GBC",
		},
		{
			Title: "Downloaded Already", Author: "someone",
			URL: "https://someone.itch.io/downloaded", IsFree: true,
			Tags: []string{"action"}, PublishedAt: base.Add(-4 * time.Hour), Platform: "GB",
		},
		{
			Title: "Needs An Update", Author: "updater",
			URL: "https://updater.itch.io/needs-update", IsFree: true,
			Tags: []string{"shooter"}, PublishedAt: base.Add(-5 * time.Hour), Platform: "GBC",
		},
	}
}

// DevFixtureDetail returns a detail page with enough description and tags to
// overflow the viewport, which is the case the full-height capture exists for.
func DevFixtureDetail(g itchio.Game) *itchio.GameDetail {
	return &itchio.GameDetail{
		Game: g,
		Description: "A frantic vertical platformer for the Game Boy Color. " +
			"Help Tobu rescue her friend from the sky by bouncing ever higher, " +
			"dodging birds, clouds and the occasional falling piano.\n\n" +
			"Features:\n" +
			"- Twelve stages across four worlds\n" +
			"- Original chiptune soundtrack\n" +
			"- Password save system\n" +
			"- Two-player versus over link cable\n\n" +
			"This description is deliberately long so the detail screen scrolls " +
			"past the bottom of the display, which is exactly the content the " +
			"full-height capture is meant to reveal. Everything below the first " +
			"fold is invisible on a single device screenshot.",
		ScreenshotURLs: []string{"https://example.invalid/1.png", "https://example.invalid/2.png"},
		PageTags: []string{"2d", "bird", "gameboy", "gbc", "gbstudio", "photography",
			"pixel-art", "retro", "platformer", "arcade", "cute", "chiptune"},
		Uploads: []itchio.Upload{
			{Filename: "tobu-tobu-girl-deluxe.gbc", URL: "https://example.invalid/a.gbc", UploadID: "1"},
			{Filename: "tobu-tobu-girl-deluxe-soundtrack.zip", URL: "https://example.invalid/b.zip", UploadID: "2"},
			{Filename: "manual.pdf", URL: "https://example.invalid/c.pdf", UploadID: "3", NeedsFormat: true},
		},
		GameID: "123456",
	}
}

// DevSetup writes fixture state into dir and returns deps pointing at it.
//
// The games cache is written with a current timestamp on purpose: ListScreen
// loads a fresh cache synchronously in its constructor and only reaches for the
// network when the cache is stale, so a fresh one keeps scene construction
// deterministic and offline.
func DevSetup(dir string, th theme.Theme) (SceneDeps, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SceneDeps{}, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	games := DevFixtureGames()
	cachePath := filepath.Join(dir, "games_cache.json")
	if err := itchio.SaveGamesCache(cachePath, games); err != nil {
		return SceneDeps{}, fmt.Errorf("write fixture cache: %w", err)
	}

	invPath := filepath.Join(dir, "inventory.json")
	inv, _ := inventory.Load(invPath)

	cfg := &settings.Config{
		ROMLocation:   "auto",
		LogLevel:      "info",
		SortMode:      "free",
		NextUITheme:   true,
		UnifiedNaming: true,
		MusicDownload: "auto",
		MusicLocation: "auto",
		Pico8Core:     "pico8",
	}
	cfgPath := filepath.Join(dir, "config.json")
	_ = cfg.Save(cfgPath)

	client := itchio.NewClient()
	return SceneDeps{
		Client:    client,
		Cfg:       cfg,
		CfgPath:   cfgPath,
		Cache:     renderer.NewImageCache(8, client.HTTPClient()),
		Inv:       inv,
		InvPath:   invPath,
		CachePath: cachePath,
		Games:     games,
		Detail:    DevFixtureDetail(games[0]),
		Theme:     th,
	}, nil
}

// devList builds the list screen every other scene uses as its parent.
func devList(d SceneDeps) *ListScreen {
	return NewListScreen(d.Client, d.Cfg, d.CfgPath, d.Cache, d.CachePath,
		d.Inv, d.InvPath, nil, d.Theme, d.Theme, true, "Dev Palette", func(bool) {},
		filepath.Join(filepath.Dir(d.CfgPath), "owned_cache.json"))
}

// devDetail builds a detail screen with its data already loaded, skipping the
// network fetch a real navigation would perform.
func devDetail(d SceneDeps) *DetailScreen {
	list := devList(d)
	s := NewDetailScreen(d.Client, d.Cfg, d.CfgPath, d.Cache, d.Games[0],
		d.Inv, d.InvPath, list, nil, d.Theme, d.Theme, true, "Dev Palette", func(bool) {})
	s.detail = d.Detail
	s.loading = false
	// Mirror what the detail fetch does, so the scene matches a real navigation.
	labels := make([]string, len(d.Detail.ScreenshotURLs))
	for i := range d.Detail.ScreenshotURLs {
		labels[i] = fmt.Sprintf("Image %d/%d  (←→)", i+1, len(d.Detail.ScreenshotURLs))
	}
	s.screenshotLabels = labels
	return s
}

// devScenes is the registry. Keep names stable: they end up in filenames and in
// the audit output.
var devScenes = []Scene{
	{"list", "Game list with mixed free/paid/long/unicode titles", func(d SceneDeps) Screen {
		return devList(d)
	}},
	{"detail", "Game detail with long description and many tags", func(d SceneDeps) Screen {
		return devDetail(d)
	}},
	{"detail-modal-info", "Detail with an informational modal open", func(d SceneDeps) Screen {
		s := devDetail(d)
		s.modal = detailModal{
			active: true, kind: modalKindInfo,
			title: "Download unavailable",
			body:  "This game has no downloadable files for your platform. Open it in a browser to play the HTML5 version.",
		}
		return s
	}},
	{"detail-modal-confirm", "Detail with the destructive delete confirmation open", func(d SceneDeps) Screen {
		s := devDetail(d)
		s.modal = detailModal{
			active: true, kind: modalKindDeleteConfirm,
			title:     "Delete ROM?",
			body:      "This removes tobu-tobu-girl-deluxe.gbc from your SD card. The save file is kept.",
			onConfirm: func() {},
		}
		return s
	}},
	{"settings", "Settings menu", func(d SceneDeps) Screen {
		return NewSettingsScreen(d.Client, d.Cfg, d.CfgPath, d.Inv, d.InvPath, d.Cache,
			devList(d), nil, nil, d.Theme, d.Theme, true, "Dev Palette", func(bool) {}, nil)
	}},
	{"about", "About screen", func(d SceneDeps) Screen {
		return NewAboutScreen(devList(d))
	}},
	{"filter", "Filter panel", func(d SceneDeps) Screen {
		return NewFilterScreen(devList(d), "GBC", "free", "puzzle", func(string, string, string) {})
	}},
	{"content-moderation", "Content moderation settings", func(d SceneDeps) Screen {
		return NewContentModerationScreen(d.Cfg, d.CfgPath, devList(d))
	}},
	{"tag-filter", "Adult content tag filter", func(d SceneDeps) Screen {
		return NewAdultContentFilterScreen(d.Cfg, d.CfgPath, devList(d))
	}},
	{"manage-downloads", "Manage downloaded files for a game", func(d SceneDeps) Screen {
		return NewManageDownloadsScreen(d.Inv, d.InvPath, d.Games[0].URL, d.Cfg, devList(d))
	}},
	{"keyboard", "On-screen keyboard", func(d SceneDeps) Screen {
		return NewKeyboardScreen(devList(d), "search term", func(string) {})
	}},
	{"rom-picker", "Multiple ROM files to choose from", func(d SceneDeps) Screen {
		return NewROMPickerScreen(d.Client, d.Cfg, d.CfgPath, d.Cache, d.Games[0], d.Detail,
			devFixtureUploads(), d.Inv, d.InvPath, devList(d))
	}},
}

// devFixtureUploads mirrors the detail fixture's uploads as roms.Upload values.
func devFixtureUploads() []roms.Upload {
	return []roms.Upload{
		{Filename: "tobu-tobu-girl-deluxe.gbc", URL: "https://example.invalid/a.gbc"},
		{Filename: "tobu-tobu-girl-deluxe-alt.gb", URL: "https://example.invalid/b.gb"},
	}
}

// Scenes returns the registry sorted by name.
func Scenes() []Scene {
	out := append([]Scene(nil), devScenes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SceneByName looks up one scene.
func SceneByName(name string) (Scene, bool) {
	for _, s := range devScenes {
		if s.Name == name {
			return s, true
		}
	}
	return Scene{}, false
}

// DevScrollable is implemented by screens that scroll their content, so the
// capture harness can walk a whole page instead of only its first screenful.
type DevScrollable interface {
	// DevSetScroll moves the viewport to the given pixel offset.
	DevSetScroll(y int32)
	// DevScrollExtent reports total content height and viewport height. Both are
	// only known after a Draw, so callers must draw once before asking.
	DevScrollExtent() (content, viewport int32)
}

func (s *DetailScreen) DevSetScroll(y int32) { s.scrollY = y }
func (s *DetailScreen) DevScrollExtent() (int32, int32) {
	return s.contentHeight, s.viewportH
}
