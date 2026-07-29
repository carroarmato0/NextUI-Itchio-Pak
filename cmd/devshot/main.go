//go:build !headless

// devshot renders app screens to PNG on a development host — no device, no
// display, no network.
//
// It exists so the screen × palette matrix is something you can actually look
// at. NextUI ships eighteen colour palettes, seven of them light, and the app
// has two dozen screens. Verifying that on hardware means a relaunch per shot;
// here the whole matrix takes a few seconds, which is cheap enough to run on
// every change.
//
//	go run ./cmd/devshot --list
//	go run ./cmd/devshot --scene detail --palette "Catppuccin Latte"
//	go run ./cmd/devshot --all --palettes all --sheet --audit
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/carroarmato0/nextui-itchio-pak/internal/ui"
)

func main() {
	var (
		sceneName  = flag.String("scene", "list", "scene to render (see --list)")
		all        = flag.Bool("all", false, "render every scene")
		palette    = flag.String("palette", "", "NextUI palette name, or \"default\" for the app's own theme")
		palettes   = flag.String("palettes", "", "comma-separated palette names, or \"all\" for every shipped palette")
		outDir     = flag.String("out-dir", "/tmp/itchio-screenshots/devshot", "output directory")
		width      = flag.Int("width", 1024, "render width")
		height     = flag.Int("height", 768, "render height")
		full       = flag.Bool("full", false, "capture the whole scrollable page, not just the first screenful")
		sheet      = flag.Bool("sheet", false, "write a contact sheet per scene tiling all palettes")
		audit      = flag.Bool("audit", false, "check drawn text contrast and report unreadable combinations")
		listOnly   = flag.Bool("list", false, "list available scenes and exit")
		paletteDir = flag.String("palette-dir", "", "directory of NextUI palette .txt files (default: bundled fixtures, then the device path)")
		verbose    = flag.Bool("v", false, "debug logging")
	)
	flag.Parse()

	logger.SetLevel(logger.LevelInfo)
	if *verbose {
		logger.SetLevel(logger.LevelDebug)
	}

	if *listOnly {
		for _, s := range ui.Scenes() {
			fmt.Printf("  %-22s %s\n", s.Name, s.Desc)
		}
		return
	}

	scenes, err := resolveScenes(*sceneName, *all)
	if err != nil {
		fail(err)
	}
	pals, err := resolvePalettes(*palette, *palettes, *paletteDir)
	if err != nil {
		fail(err)
	}

	fmt.Printf("==> %d scene(s) × %d palette(s) → %s\n", len(scenes), len(pals), *outDir)

	var findings []finding
	sheets := map[string][]tile{}

	for _, p := range pals {
		for _, sc := range scenes {
			res, err := render(sc, p, *width, *height, *full, *audit)
			if err != nil {
				fail(fmt.Errorf("%s@%s: %w", sc.Name, p.label, err))
			}
			name := fmt.Sprintf("%s@%s.png", sc.Name, sanitise(p.label))
			out := filepath.Join(*outDir, name)
			if err := renderer.WritePNG(out, res.img); err != nil {
				fail(err)
			}
			b := res.img.Bounds()
			fmt.Printf("    %-34s %4dx%-5d %s\n", name, b.Dx(), b.Dy(), res.note)
			findings = append(findings, res.findings...)
			if *sheet {
				sheets[sc.Name] = append(sheets[sc.Name], tile{label: p.label, img: res.img})
			}
		}
	}

	if *sheet {
		for scName, tiles := range sheets {
			out := filepath.Join(*outDir, scName+".sheet.png")
			if err := renderer.WritePNG(out, buildSheet(tiles)); err != nil {
				fail(err)
			}
			fmt.Printf("==> contact sheet: %s (%d palettes)\n", out, len(tiles))
		}
	}

	if *audit {
		reportAudit(findings)
		if countFailures(findings) > 0 {
			os.Exit(1)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func resolveScenes(name string, all bool) ([]ui.Scene, error) {
	if all {
		return ui.Scenes(), nil
	}
	sc, ok := ui.SceneByName(name)
	if !ok {
		var names []string
		for _, s := range ui.Scenes() {
			names = append(names, s.Name)
		}
		return nil, fmt.Errorf("unknown scene %q; available: %s", name, strings.Join(names, ", "))
	}
	return []ui.Scene{sc}, nil
}

// palChoice pairs a theme with the label used in filenames and sheets.
type palChoice struct {
	label string
	th    theme.Theme
}

// resolvePalettes turns the flags into concrete themes. "default" is the app's
// own bundled theme — the one a device without minuisettings.txt renders.
func resolvePalettes(one, many, dir string) ([]palChoice, error) {
	if one == "" && many == "" {
		return []palChoice{{"default", theme.Defaults()}}, nil
	}
	var want []string
	if one != "" {
		want = append(want, one)
	}
	if many != "" && many != "all" {
		want = append(want, strings.Split(many, ",")...)
	}

	if many == "all" || len(want) > 0 {
		found := loadPalettes(dir)
		if many == "all" {
			if len(found) == 0 {
				return nil, fmt.Errorf("no palette files found; pass --palette-dir")
			}
			out := []palChoice{{"default", theme.Defaults()}}
			for _, p := range found {
				out = append(out, palChoice{p.Name, themeFromPalette(p)})
			}
			return out, nil
		}
		var out []palChoice
		for _, w := range want {
			w = strings.TrimSpace(w)
			if strings.EqualFold(w, "default") {
				out = append(out, palChoice{"default", theme.Defaults()})
				continue
			}
			var hit *theme.Palette
			for i := range found {
				if strings.EqualFold(found[i].Name, w) {
					hit = &found[i]
					break
				}
			}
			if hit == nil {
				return nil, fmt.Errorf("palette %q not found; try --palettes all", w)
			}
			out = append(out, palChoice{hit.Name, themeFromPalette(*hit)})
		}
		return out, nil
	}
	return []palChoice{{"default", theme.Defaults()}}, nil
}

// loadPalettes looks in an explicit directory, then the repo's bundled copies,
// then the on-device path — so this works checked out anywhere.
func loadPalettes(dir string) []theme.Palette {
	if dir != "" {
		return theme.EnumeratePalettes(dir, "")
	}
	for _, d := range []string{
		"testdata/palettes",
		theme.BuiltinPaletteDir,
	} {
		if pals := theme.EnumeratePalettes(d, ""); len(pals) > 0 {
			return pals
		}
	}
	return nil
}

// themeFromPalette applies a palette's seven colours using the same key-to-field
// mapping Load uses, so a rendered palette matches what the device would show.
func themeFromPalette(p theme.Palette) theme.Theme {
	th := theme.Defaults()
	th.Accent = p.Colors[0].RGB()
	th.TitlePill = p.Colors[1].RGB()
	th.HeaderBG = p.Colors[2].RGB()
	th.ListText = p.Colors[3].RGB()
	th.MainText = p.Colors[3].RGB()
	th.AccentText = p.Colors[4].RGB()
	th.HintText = p.Colors[5].RGB()
	th.Background = p.Colors[6].RGB()
	return th
}

func sanitise(s string) string {
	return strings.NewReplacer(" ", "-", "/", "-", "&", "and").Replace(s)
}

type tile struct {
	label string
	img   image.Image
}

// buildSheet tiles every palette's render of one scene into a single grid, so a
// whole palette sweep can be judged in one look instead of eighteen files.
func buildSheet(tiles []tile) image.Image {
	if len(tiles) == 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	sort.Slice(tiles, func(i, j int) bool { return tiles[i].label < tiles[j].label })

	const cols = 6
	const scale = 3 // downscale factor; keeps a sheet legible but manageable
	tw := tiles[0].img.Bounds().Dx() / scale
	th := tiles[0].img.Bounds().Dy() / scale
	rows := (len(tiles) + cols - 1) / cols

	const gap = 6
	out := image.NewRGBA(image.Rect(0, 0, cols*(tw+gap)+gap, rows*(th+gap)+gap))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: image.Black.C}, image.Point{}, draw.Src)

	for i, t := range tiles {
		cx := (i % cols) * (tw + gap)
		cy := (i / cols) * (th + gap)
		dst := image.Rect(gap+cx, gap+cy, gap+cx+tw, gap+cy+th)
		nearestScale(out, dst, t.img, scale)
	}
	return out
}

// nearestScale copies src into dst shrunk by an integer factor. Nearest
// neighbour is deliberate: a contact sheet should show aliasing and thin-line
// dropout rather than smooth them away.
func nearestScale(dst *image.RGBA, r image.Rectangle, src image.Image, factor int) {
	sb := src.Bounds()
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			sx, sy := sb.Min.X+x*factor, sb.Min.Y+y*factor
			if sx >= sb.Max.X || sy >= sb.Max.Y {
				continue
			}
			dst.Set(r.Min.X+x, r.Min.Y+y, src.At(sx, sy))
		}
	}
}
