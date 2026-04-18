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
	cfg, _ := settings.Load(cfgPath)

	// Pre-init SDL2 to detect display resolution before creating the window.
	// renderer.New will call sdl.Init again, which is idempotent.
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		log.Fatalf("sdl pre-init: %v", err)
	}
	w, h := int32(1024), int32(768) // sensible default for TrimUI Brick
	if dm, err := sdl.GetCurrentDisplayMode(0); err == nil {
		w, h = dm.W, dm.H
	}
	log.Printf("display: %dx%d", w, h)

	r, err := renderer.New("Itch.io", int(w), int(h))
	if err != nil {
		log.Fatalf("renderer init: %v", err)
	}
	defer r.Close()

	cache := renderer.NewImageCache(50)
	defer cache.Clear()

	client := itchio.NewClient()

	var current ui.Screen = ui.NewListScreen(client, cfg, cfgPath, cache)

	for current != nil {
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
