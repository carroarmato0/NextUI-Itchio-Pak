//go:build !headless

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/power"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/carroarmato0/nextui-itchio-pak/internal/ui"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	userEventInventoryUpdate = int32(0) // UpdateService finished a check
	userEventPowerSleep      = int32(1) // power: short press
	userEventPowerShutdown   = int32(2) // power: long press
)

func runSDL() {
	cfgPath := os.Getenv("HOME") + "/config.json"
	cachePath := filepath.Join(filepath.Dir(cfgPath), "games_cache.json")
	ownedCachePath := filepath.Join(filepath.Dir(cfgPath), "owned_cache.json")
	cfg, _ := settings.Load(cfgPath)

	// Apply log level and register the API key for redaction before anything
	// else is logged. LOG_LEVEL env var overrides the config value so the
	// dev-screenshot script can force debug logging without editing config.
	logger.SetLevel(logger.LevelFromString(cfg.LogLevel))
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logger.SetLevel(logger.LevelFromString(envLevel))
	}
	logger.RegisterSecret(cfg.APIKey, "[API-KEY]")

	inventoryPath := filepath.Join(filepath.Dir(cfgPath), "inventory.json")
	inv, _ := inventory.Load(inventoryPath)
	inv.VerifyAndClean(inventoryPath)

	level := cfg.LogLevel
	if level == "" {
		level = "info"
	}
	logger.Info("log_level:  %s", level)

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

	const miniSettingsPath = "/mnt/SDCARD/.userdata/shared/minuisettings.txt"
	nextUITheme, themeAvailable := theme.Load(miniSettingsPath)
	defaultTheme := theme.Defaults()

	activeTheme := defaultTheme
	if cfg.NextUITheme && themeAvailable {
		activeTheme = nextUITheme
	}
	logger.Info("theme: available=%v, active=%v", themeAvailable, cfg.NextUITheme && themeAvailable)

	r, err := renderer.New("Itch.io", int(w), int(h), activeTheme)
	if err != nil {
		logger.Error("renderer init: %v", err)
		os.Exit(1)
	}
	defer r.Close()

	onThemeToggle := func(enabled bool) {
		if enabled && themeAvailable {
			r.Theme = nextUITheme
		} else {
			r.Theme = defaultTheme
		}
		logger.Debug("renderer: theme updated (NextUI active: %v)", enabled && themeAvailable)
	}

	client := itchio.NewClient()

	cache := renderer.NewImageCache(50, client.HTTPClient())
	defer cache.Clear()
	cache.SetNotify(func() {
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: -1})
	})

	updateSvc := inventory.NewUpdateService(inv, inventoryPath, client, func() {
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: userEventInventoryUpdate})
	})
	updateSvc.Start(nil)
	defer updateSvc.Stop()

	powerMgr := power.NewManager(func(action power.Action) {
		code := userEventPowerSleep
		if action == power.ActionShutdown {
			code = userEventPowerShutdown
		}
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: code})
	})
	powerMgr.Start()

	listScreen := ui.NewListScreen(client, cfg, cfgPath, cache, cachePath, inv, inventoryPath, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle, ownedCachePath)
	var current ui.Screen
	if devScreen := os.Getenv("DEV_START_SCREEN"); devScreen != "" {
		logger.Info("dev: DEV_START_SCREEN=%q", devScreen)
		current = ui.NewDevStartScreen(devScreen, listScreen, client, cfg, cfgPath, cache, inv, inventoryPath, updateSvc, nextUITheme, defaultTheme, themeAvailable, onThemeToggle)
	} else {
		current = listScreen
	}

	// pendingQuit and pendingAction are set together; only read when pendingQuit is true.
	var (
		pendingQuit   bool
		pendingAction = power.ActionSleep
	)

	platform := readPlatform()
	if platform == "my355" {
		const joyTypePath = "/sys/class/miyooio_chr_dev/joy_type"
		logger.Debug("input: checking for my355 joy_type workaround at %s", joyTypePath)
		if _, err := os.Stat(joyTypePath); err == nil {
			logger.Info("input: applying my355 joy_type workaround (-1)")
			if err := os.WriteFile(joyTypePath, []byte("-1"), 0644); err != nil {
				logger.Error("input: failed to apply joy_type workaround: %v", err)
			}
			defer func() {
				logger.Info("input: restoring my355 joy_type (0)")
				if err := os.WriteFile(joyTypePath, []byte("0"), 0644); err != nil {
					logger.Error("input: failed to restore joy_type: %v", err)
				}
			}()
		}
	}

loop:
	for current != nil {
		// Upload any images that background goroutines finished fetching.
		// Returns true if at least one texture was uploaded this call.
		newImages := cache.ProcessPending(r)

		// Block until an SDL event arrives.
		// Four modes:
		//   16ms  — screen needs continuous redraws (download progress, spinners)
		//   poll  — textures just uploaded; draw them before blocking again
		//  500ms  — screen is static but has a pending timed animation (e.g. title
		//           scroll delay). This guarantees the loop wakes before the
		//           animation window opens even if no other events fire.
		//  ∞      — truly idle: no redraws needed, image-cache notify and user
		//           input are the only expected wakeups.
		//
		// The poll case fixes a race where ProcessPending uploads a texture in
		// iteration N+1 (after the notify UserEvent woke iteration N's WaitEvent),
		// but then WaitEvent blocks indefinitely because no further event arrives.
		gotEvent := false
		var e sdl.Event
		if current.NeedsRedraw() {
			e = sdl.WaitEventTimeout(16)
		} else if newImages {
			e = sdl.PollEvent()
		} else if current.HasPendingAnimation() {
			e = sdl.WaitEventTimeout(500)
		} else {
			e = sdl.WaitEvent()
		}
		for e != nil {
			gotEvent = true
			if pendingQuit {
				e = sdl.PollEvent()
				continue // drain input while waiting for tasks
			}
			// Intercept SDL_QUIT (SIGTERM from NextUI) before screens see it.
			if _, ok := e.(*sdl.QuitEvent); ok {
				current = nil
				break loop
			}
			// Intercept UserEvents before screens see them.
			if uev, ok := e.(*sdl.UserEvent); ok {
				switch uev.Code {
				case userEventInventoryUpdate:
					// Update-svc finished a check; rebuild the list view so
					// new [UP]/[!] badges and DL-sort order are immediately visible.
					listScreen.ScheduleRebuild()
					// Fall through — do NOT continue. FetchUploadsScreen also uses
					// UserEvent code 0 for its goroutine-done signal, so the event
					// must still reach current.HandleEvent(e).
				case userEventPowerSleep:
					logger.Info("power: sleep requested, waiting for tasks")
					pendingQuit = true
					pendingAction = power.ActionSleep
					updateSvc.Stop()
					e = sdl.PollEvent()
					continue
				case userEventPowerShutdown:
					logger.Info("power: shutdown requested, waiting for tasks")
					pendingQuit = true
					pendingAction = power.ActionShutdown
					updateSvc.Stop()
					e = sdl.PollEvent()
					continue
				}
			}
			current = current.HandleEvent(e)
			if current == nil {
				break loop
			}
			e = sdl.PollEvent()
		}
		if current == nil {
			break loop
		}
		if pendingQuit {
			var busy bool
			if bc, ok := current.(ui.BusyChecker); ok {
				busy = bc.IsBusy()
			}
			if !busy && !updateSvc.IsRunning() {
				if pendingAction == power.ActionShutdown {
					logger.Info("power: all tasks done, writing /tmp/poweroff")
					if err := os.WriteFile("/tmp/poweroff", []byte{}, 0644); err != nil {
						logger.Error("power: /tmp/poweroff: %v", err)
					}
					break loop // exit cleanly; NextUI detects /tmp/poweroff and shuts down
				}
				suspendPath := filepath.Join(os.Getenv("SYSTEM_PATH"), "bin", "suspend")
				if _, err := os.Stat(suspendPath); err != nil {
					logger.Warn("power: suspend script not found at %s, exiting instead", suspendPath)
					current = nil
				} else {
					logger.Info("power: all tasks done, calling %s", suspendPath)
					if err := exec.Command(suspendPath).Run(); err != nil {
						logger.Error("power: suspend: %v", err)
					}
					logger.Info("power: resumed from sleep")
					powerMgr.PostWake()
					// Flush any power UserEvents the goroutine queued while
					// processing the wake-up key press. They arrived before
					// suspend.Run() returned, so PostWake() alone is too late.
					for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
						if uev, ok := e.(*sdl.UserEvent); ok &&
							(uev.Code == userEventPowerSleep || uev.Code == userEventPowerShutdown) {
							logger.Info("power: discarding buffered wake-up event")
							continue
						}
						current = current.HandleEvent(e)
						if current == nil {
							break loop
						}
					}
					pendingQuit = false
				}
			} else {
				drawPowerPendingOverlay(r, pendingAction)
			}
		} else if gotEvent || newImages || current.NeedsRedraw() {
			current.Draw(r)
		}
	}
}

func drawPowerPendingOverlay(r *renderer.Renderer, action power.Action) {
	// This was the one draw path with no theme at all: a hardcoded #141414
	// clear with light grey text. On a light palette it flashed a dark panel
	// on the way to sleep.
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])
	subtitle := "Finishing up before sleep…"
	if action == power.ActionShutdown {
		subtitle = "Finishing up before shutdown…"
	}
	_, mainH := r.TextSize("Ag")
	mid := r.H / 2
	mt := r.Theme.ContrastText(bg)
	ht := r.Theme.HintText
	r.DrawTextCentered("Please wait", 0, mid-mainH-6, r.W, mt[0], mt[1], mt[2])
	r.DrawSmallTextCentered(subtitle, 0, mid+6, r.W, ht[0], ht[1], ht[2])
	r.Present()
}
