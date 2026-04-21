//go:build !headless

package main

import (
	"os"
	"path/filepath"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/ui"
	"github.com/veandco/go-sdl2/sdl"
)

func runSDL() {
	cfgPath := os.Getenv("HOME") + "/config.json"
	cachePath := filepath.Join(filepath.Dir(cfgPath), "games_cache.json")
	cfg, _ := settings.Load(cfgPath)

	// Apply log level and register the API key for redaction before anything
	// else is logged.
	logger.SetLevel(logger.LevelFromString(cfg.LogLevel))
	logger.RegisterSecret(cfg.APIKey, "[API-KEY]")

	// Log the environment header so the log file is self-describing.
	level := cfg.LogLevel
	if level == "" {
		level = "info"
	}
	logger.Info("platform=%s nextui=%s log_level=%s",
		readPlatform(), readNextUIVersion(), level)

	// Pre-init SDL2 to detect display resolution before creating the window.
	// Include JOYSTICK + GAMECONTROLLER so the device's physical buttons are
	// delivered as ControllerButtonEvents (the device SDL2 has built-in
	// mappings for TrimUI/Miyoo hardware). renderer.New will call sdl.Init
	// again — that is idempotent.
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK | sdl.INIT_GAMECONTROLLER); err != nil {
		logger.Error("sdl pre-init: %v", err)
		os.Exit(1)
	}

	// Open all connected game controllers so button events are delivered.
	for i := 0; i < sdl.NumJoysticks(); i++ {
		if sdl.IsGameController(i) {
			if gc := sdl.GameControllerOpen(i); gc != nil {
				defer gc.Close()
			}
		} else {
			if js := sdl.JoystickOpen(i); js != nil {
				defer js.Close()
			}
		}
	}

	w, h := int32(1024), int32(768) // sensible default for TrimUI Brick
	if dm, err := sdl.GetCurrentDisplayMode(0); err == nil {
		w, h = dm.W, dm.H
	}
	logger.Info("display: %dx%d", w, h)

	r, err := renderer.New("Itch.io", int(w), int(h))
	if err != nil {
		logger.Error("renderer init: %v", err)
		os.Exit(1)
	}
	defer r.Close()

	cache := renderer.NewImageCache(50)
	defer cache.Clear()

	client := itchio.NewClient()

	var current ui.Screen = ui.NewListScreen(client, cfg, cfgPath, cache, cachePath)

	for current != nil {
		// Upload any images that background goroutines finished fetching.
		cache.ProcessPending(r)

		for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
			current = current.HandleEvent(e)
			if current == nil {
				break
			}
		}
		if current != nil {
			current.Draw(r)
		}
		sdl.Delay(16) // ~60 fps
	}
}
