//go:build !headless

package renderer

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	itext "github.com/carroarmato0/nextui-itchio-pak/internal/text"
	"github.com/carroarmato0/nextui-itchio-pak/internal/theme"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// BadgeKind distinguishes face-button badges (circle) from function-key badges (pill).
type BadgeKind int

const (
	BadgeCircle BadgeKind = iota
	BadgePill
)

// FooterHint is one item in the footer hint bar.
type FooterHint struct {
	Kind  BadgeKind
	Label string // button label inside the badge, e.g. "A", "B", "START"
	Text  string // description text after the badge, e.g. "Open"
}

// fallbackFont pairs main + small TTF fonts for a single fallback script file,
// together with its parsed cmap ranges for fast glyph lookup.
type fallbackFont struct {
	main, small *ttf.Font
	ranges      []cmapRange
}

// pillKey identifies a unique pill shape by size and fill colour.
type pillKey struct {
	w, h    int32
	r, g, b uint8
}

type Renderer struct {
	Window        *sdl.Window
	Renderer      *sdl.Renderer
	Font          *ttf.Font   // main font (~35px at 768)
	SmallFont     *ttf.Font   // hint/footer font (~24px at 768)
	primaryRanges []cmapRange // codepoints covered by Font, parsed at init
	fallbacks     []fallbackFont
	W, H          int32
	Theme         theme.Theme
	texts         *textCache
	sizes         map[sizeKey][2]int32 // SizeUTF8 measurement cache (no GPU resources; no LRU needed)
	runeFont      map[rune]int         // fontIndex result per rune; populated lazily, never evicted
	wrapCache     map[wrapKey][]string // WrapText output keyed on (text, maxWidth); no LRU needed
	pillCache     map[pillKey]*sdl.Texture // pre-rendered pill textures; nil entry = render target unsupported
}

func New(title string, w, h int, th theme.Theme) (*Renderer, error) {
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

	// Scale font with screen height: h/22 gives ~35pt at 768.
	// Clamp to a minimum of 22 so it stays readable on very small displays.
	fontSize := h / 22
	if fontSize < 22 {
		fontSize = 22
	}
	font, err := ttf.OpenFont("assets/font.ttf", fontSize)
	if err != nil {
		return nil, fmt.Errorf("open font: %w", err)
	}

	// Small font for footer hint text — keeps it readable without taking much space.
	smallSize := h / 32
	if smallSize < 18 {
		smallSize = 18
	}
	smallFont, err := ttf.OpenFont("assets/font.ttf", smallSize)
	if err != nil {
		return nil, fmt.Errorf("open small font: %w", err)
	}

	r := &Renderer{
		Window: win, Renderer: ren, Font: font, SmallFont: smallFont,
		W: int32(w), H: int32(h),
		primaryRanges: buildGlyphRanges("assets/font.ttf"),
		Theme:         th,
		texts:         newTextCache(maxTextCacheEntries),
		sizes:         make(map[sizeKey][2]int32),
		runeFont:      make(map[rune]int),
		wrapCache:     make(map[wrapKey][]string),
		pillCache:     make(map[pillKey]*sdl.Texture),
	}
	if r.primaryRanges == nil {
		logger.Warn("renderer: could not parse primary font cmap; fallback fonts disabled")
	}

	// Load all assets/font_fallback_*.ttf files alphabetically.
	// Each covers a different script family (Arabic, Hebrew, Thai, Devanagari…).
	paths, _ := filepath.Glob("assets/font_fallback_*.ttf")
	sort.Strings(paths)
	for _, path := range paths {
		main, err := ttf.OpenFont(path, fontSize)
		if err != nil {
			logger.Warn("renderer: could not open fallback font %s: %v", filepath.Base(path), err)
			continue
		}
		small, err := ttf.OpenFont(path, smallSize)
		if err != nil {
			main.Close()
			logger.Warn("renderer: could not open fallback font (small) %s: %v", filepath.Base(path), err)
			continue
		}
		r.fallbacks = append(r.fallbacks, fallbackFont{
			main:   main,
			small:  small,
			ranges: buildGlyphRanges(path),
		})
		logger.Info("renderer: fallback font loaded: %s", filepath.Base(path))
	}

	return r, nil
}

func (r *Renderer) Close() {
	for _, tex := range r.pillCache {
		if tex != nil {
			tex.Destroy()
		}
	}
	for _, fb := range r.fallbacks {
		if fb.small != nil {
			fb.small.Close()
		}
		if fb.main != nil {
			fb.main.Close()
		}
	}
	if r.SmallFont != nil {
		r.SmallFont.Close()
	}
	if r.Font != nil {
		r.Font.Close()
	}
	if r.texts != nil {
		r.texts.clear() // must precede Renderer.Destroy to avoid double-free
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

// fontIndex returns 0 if the primary font covers ch, the 1-based index of
// the first fallback font that covers it, or -1 if ch is an emoji codepoint
// that no loaded font covers (the rune is silently dropped by splitTextRuns).
func (r *Renderer) fontIndex(ch rune) int {
	if idx, ok := r.runeFont[ch]; ok {
		return idx
	}
	var idx int
	if r.primaryRanges != nil && !inRanges(r.primaryRanges, ch) {
		for i, fb := range r.fallbacks {
			if fb.ranges == nil || inRanges(fb.ranges, ch) {
				idx = i + 1
				break
			}
		}
	}
	// Drop emoji that no font covers rather than falling through to tofu.
	// When NotoEmoji is loaded (Task 4), it covers all common emoji including
	// U+1F4BE (floppy disk). If the font fails to load, emoji are silently
	// dropped — acceptable degradation per spec.
	if idx == 0 && r.primaryRanges != nil && itext.IsEmoji(ch) {
		idx = -1
	}
	r.runeFont[ch] = idx
	return idx
}

// mainFont returns the main-size font for the given font index.
func (r *Renderer) mainFont(idx int) *ttf.Font {
	if idx > 0 && idx-1 < len(r.fallbacks) {
		return r.fallbacks[idx-1].main
	}
	return r.Font
}

// smallFont returns the small-size font for the given font index.
func (r *Renderer) smallFont(idx int) *ttf.Font {
	if idx > 0 && idx-1 < len(r.fallbacks) {
		return r.fallbacks[idx-1].small
	}
	return r.SmallFont
}

// drawRectBuf, pillBodyBuf, copyDstBuf, clipRectBuf prevent CGo escape-analysis from
// allocating a fresh sdl.Rect on every call through the CGo boundary.
// Only accessed from the SDL main goroutine — no locking needed.
var (
	drawRectBuf sdl.Rect
	pillBodyBuf sdl.Rect
	copyDstBuf  sdl.Rect
	clipRectBuf sdl.Rect
)

func (r *Renderer) DrawRect(x, y, w, h int32, red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	drawRectBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
	r.Renderer.FillRect(&drawRectBuf)
}

// drawRuns renders text with per-run texture caching. Each unique
// (run text, fontIdx, small, bold, color) tuple is uploaded to the GPU once
// and reused on every subsequent frame — eliminating the per-frame
// RenderUTF8Blended → CreateTextureFromSurface → Destroy cycle.
func (r *Renderer) drawRuns(text string, x, y int32, color sdl.Color, small, bold bool) {
	if bold {
		r.Font.SetStyle(ttf.STYLE_BOLD)
		defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	}
	runs := splitTextRuns(text, r.fontIndex)
	cx := x
	for _, run := range runs {
		key := textRunKey{
			text:   run.text,
			fontID: uint8(run.fontIdx),
			small:  small,
			bold:   bold,
			r:      color.R,
			g:      color.G,
			b:      color.B,
		}
		if val, ok := r.texts.get(key); ok {
			copyDstBuf = sdl.Rect{X: cx, Y: y, W: val.w, H: val.h}
			r.Renderer.Copy(val.tex, nil, &copyDstBuf)
			cx += val.w
			continue
		}
		var font *ttf.Font
		if small {
			font = r.smallFont(run.fontIdx)
		} else {
			font = r.mainFont(run.fontIdx)
		}
		surface, err := font.RenderUTF8Blended(run.text, color)
		if err != nil {
			continue
		}
		tex, err := r.Renderer.CreateTextureFromSurface(surface)
		surface.Free()
		if err != nil {
			continue
		}
		_, _, tw, th, _ := tex.Query()
		copyDstBuf = sdl.Rect{X: cx, Y: y, W: tw, H: th}
		r.Renderer.Copy(tex, nil, &copyDstBuf)
		r.texts.put(key, textRunVal{tex: tex, w: tw, h: th})
		cx += tw
	}
}

func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error {
	r.drawRuns(text, x, y, sdl.Color{R: red, G: green, B: blue, A: 255}, false, false)
	return nil
}

// textSizeImpl measures text width and height, caching SizeUTF8 results per run
// to avoid redundant CGo crossings when the same strings are measured every frame.
func (r *Renderer) textSizeImpl(text string, small, bold bool) (int32, int32) {
	if bold {
		r.Font.SetStyle(ttf.STYLE_BOLD)
		defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	}
	runs := splitTextRuns(text, r.fontIndex)
	var totalW, maxH int32
	for _, run := range runs {
		key := sizeKey{text: run.text, fontID: uint8(run.fontIdx), small: small, bold: bold}
		if v, ok := r.sizes[key]; ok {
			totalW += v[0]
			if v[1] > maxH {
				maxH = v[1]
			}
			continue
		}
		var font *ttf.Font
		if small {
			font = r.smallFont(run.fontIdx)
		} else {
			font = r.mainFont(run.fontIdx)
		}
		w, h, err := font.SizeUTF8(run.text)
		if err != nil {
			continue
		}
		r.sizes[key] = [2]int32{int32(w), int32(h)}
		totalW += int32(w)
		if int32(h) > maxH {
			maxH = int32(h)
		}
	}
	return totalW, maxH
}

// TextSize returns the pixel width and height of text without drawing it.
func (r *Renderer) TextSize(text string) (int32, int32) { return r.textSizeImpl(text, false, false) }

// DrawBoldText renders text using SDL_ttf bold style synthesis.
func (r *Renderer) DrawBoldText(text string, x, y int32, red, green, blue uint8) error {
	r.drawRuns(text, x, y, sdl.Color{R: red, G: green, B: blue, A: 255}, false, true)
	return nil
}

// BoldTextSize returns the pixel width and height of text measured in bold style.
func (r *Renderer) BoldTextSize(text string) (int32, int32) { return r.textSizeImpl(text, false, true) }

// DrawTextCentered draws text horizontally centered within a region [x, x+w].
func (r *Renderer) DrawTextCentered(text string, x, y, w int32, red, green, blue uint8) {
	tw, _ := r.TextSize(text)
	r.DrawText(text, x+(w-tw)/2, y, red, green, blue)
}

// DrawSmallTextCentered draws small hint text horizontally centered within [x, x+w].
func (r *Renderer) DrawSmallTextCentered(text string, x, y, w int32, red, green, blue uint8) {
	tw, _ := r.SmallTextSize(text)
	r.DrawSmallText(text, x+(w-tw)/2, y, red, green, blue)
}

// DrawTextCenteredInRect renders main-font text centered both horizontally and vertically within a rectangle.
func (r *Renderer) DrawTextCenteredInRect(text string, x, y, w, h int32, red, green, blue uint8) {
	tw, th := r.TextSize(text)
	r.DrawText(text, x+(w-tw)/2, y+(h-th)/2, red, green, blue)
}

// DrawSmallTextCenteredInRect renders small-font text centered both horizontally and vertically within a rectangle.
func (r *Renderer) DrawSmallTextCenteredInRect(text string, x, y, w, h int32, red, green, blue uint8) {
	tw, th := r.SmallTextSize(text)
	r.DrawSmallText(text, x+(w-tw)/2, y+(h-th)/2, red, green, blue)
}

func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
	copyDstBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
	r.Renderer.Copy(tex, nil, &copyDstBuf)
}

// scrollTextOffset computes the horizontal marquee offset for text that is
// `excess` pixels wider than its clip window. The cycle is:
//
//	pause at start → scroll left → pause at end → reset
func scrollTextOffset(excess int32) int32 {
	const speedPxPerSec = uint32(60)
	const pauseMs = uint32(1500)
	scrollMs := uint32(excess) * 1000 / speedPxPerSec
	cycleMs := pauseMs + scrollMs + pauseMs
	t := sdl.GetTicks() % cycleMs
	if t < pauseMs {
		return 0
	}
	t -= pauseMs
	if t >= scrollMs {
		return excess
	}
	return int32(t) * int32(speedPxPerSec) / 1000
}

// DrawScrollingText draws text within a clip window of width maxW. When the text
// is wider than maxW it autoscrolls horizontally; otherwise it is drawn normally.
func (r *Renderer) DrawScrollingText(text string, x, y, maxW int32, cr, cg, cb uint8) {
	tw, th := r.TextSize(text)
	if tw <= maxW {
		r.DrawText(text, x, y, cr, cg, cb)
		return
	}
	r.SetClipRect(x, y, maxW, th)
	r.DrawText(text, x-scrollTextOffset(tw-maxW), y, cr, cg, cb)
	r.ClearClipRect()
}

// DrawSmallScrollingText draws small-font text within a clip window of width maxW.
// When the text is wider than maxW it autoscrolls horizontally; otherwise drawn normally.
func (r *Renderer) DrawSmallScrollingText(text string, x, y, maxW int32, cr, cg, cb uint8) {
	tw, th := r.SmallTextSize(text)
	if tw <= maxW {
		r.DrawSmallText(text, x, y, cr, cg, cb)
		return
	}
	r.SetClipRect(x, y, maxW, th)
	r.DrawSmallText(text, x-scrollTextOffset(tw-maxW), y, cr, cg, cb)
	r.ClearClipRect()
}

// SetClipRect sets the clipping rectangle for rendering.
func (r *Renderer) SetClipRect(x, y, w, h int32) {
	clipRectBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
	r.Renderer.SetClipRect(&clipRectBuf)
}

// ClearClipRect removes any clipping rectangle.
func (r *Renderer) ClearClipRect() {
	r.Renderer.SetClipRect(nil)
}

// WrapText breaks text into lines that fit within maxWidth pixels.
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	key := wrapKey{text: text, maxWidth: maxWidth}
	if lines, ok := r.wrapCache[key]; ok {
		return lines
	}
	var lines []string
	for _, paragraph := range splitLines(text) {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := splitWords(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			test := current + " " + word
			tw, _ := r.TextSize(test)
			if tw > maxWidth {
				lines = append(lines, current)
				current = word
			} else {
				current = test
			}
		}
		lines = append(lines, current)
	}
	r.wrapCache[key] = lines[:len(lines):len(lines)]
	return lines
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func splitWords(s string) []string {
	var words []string
	for _, w := range strings.Fields(s) {
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

// DrawSmallText draws text using the small hint font.
func (r *Renderer) DrawSmallText(text string, x, y int32, red, green, blue uint8) error {
	r.drawRuns(text, x, y, sdl.Color{R: red, G: green, B: blue, A: 255}, true, false)
	return nil
}

// SmallTextSize returns the pixel width and height of text in the small font.
func (r *Renderer) SmallTextSize(text string) (int32, int32) {
	return r.textSizeImpl(text, true, false)
}

// DrawHeaderBar draws the header bar using theme colors and returns the
// Y coordinate for vertically-centred single-line text.
func (r *Renderer) DrawHeaderBar(h int32) int32 {
	bg := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, 0, r.W, h, bg[0], bg[1], bg[2])
	r.DrawRect(0, h, r.W, 2, ac[0], ac[1], ac[2])
	_, fh := r.TextSize("Ag")
	return (h - fh) / 2
}

// DrawFooterBar draws the footer bar using theme colors and returns the Y coordinate
// for vertically-centred small hint text.
func (r *Renderer) DrawFooterBar(h int32) int32 {
	bg := r.Theme.HeaderBG
	ac := r.Theme.Accent
	r.DrawRect(0, r.H-h, r.W, 2, ac[0], ac[1], ac[2])
	r.DrawRect(0, r.H-h+2, r.W, h-2, bg[0], bg[1], bg[2])
	_, fh := r.SmallTextSize("Ag")
	return r.H - h + 2 + (h-2-fh)/2
}

// DrawWrappedText renders word-wrapped text starting at (x, y) within maxWidth,
// using lineH pixels between lines. Returns the total height used.
func (r *Renderer) DrawWrappedText(text string, x, y, maxWidth, lineH int32, red, green, blue uint8) int32 {
	lines := r.WrapText(text, maxWidth)
	for i, line := range lines {
		if line != "" {
			r.DrawText(line, x, y+int32(i)*lineH, red, green, blue)
		}
	}
	return int32(len(lines)) * lineH
}

// DrawPill draws a filled pill (capsule) shape.
// On first call for a given (w, h, colour) the shape is pre-rendered into an
// SDL_Texture; subsequent calls blit that texture with a single Copy call,
// replacing ~5 CGo FillRects calls with one. Falls back to direct drawing if
// the renderer does not support render targets.
func (r *Renderer) DrawPill(x, y, w, h int32, red, green, blue uint8) {
	if w <= 0 || h <= 0 {
		return
	}
	key := pillKey{w, h, red, green, blue}
	tex, cached := r.pillCache[key]
	if !cached {
		tex = r.createPillTexture(w, h, red, green, blue)
		r.pillCache[key] = tex // nil stored on failure to prevent retrying every frame
	}
	if tex != nil {
		copyDstBuf = sdl.Rect{X: x, Y: y, W: w, H: h}
		r.Renderer.Copy(tex, nil, &copyDstBuf)
		return
	}
	// Fallback: direct geometry draw (render targets not supported).
	r.drawPillDirect(x, y, w, h, red, green, blue)
}

// drawPillDirect draws a pill without the texture cache. Used as a fallback
// when SDL render targets are unavailable.
func (r *Renderer) drawPillDirect(x, y, w, h int32, red, green, blue uint8) {
	radius := h / 2
	if radius < 1 {
		radius = 1
	}
	r.Renderer.SetDrawColor(red, green, blue, 255)
	pillBodyBuf = sdl.Rect{X: x + radius, Y: y, W: w - radius*2, H: h}
	r.Renderer.FillRect(&pillBodyBuf)
	drawFilledCircle(r.Renderer, x+radius, y+radius, radius, red, green, blue)
	drawFilledCircle(r.Renderer, x+w-radius, y+radius, radius, red, green, blue)
}

// createPillTexture pre-renders a pill into an SDL_Texture with the given
// dimensions and fill colour. Returns nil if the renderer does not support
// render targets (software fallback).
//
// The texture is created with SDL_BLENDMODE_BLEND so it composites correctly
// over any background when blitted. Fringe pixels are written with raw alpha
// (BLENDMODE_NONE during creation) so they are not double-applied when blitting.
func (r *Renderer) createPillTexture(w, h int32, red, green, blue uint8) *sdl.Texture {
	tex, err := r.Renderer.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_TARGET, w, h)
	if err != nil {
		logger.Debug("renderer: pill texture (%dx%d): %v — using direct draw fallback", w, h, err)
		return nil
	}
	if err := tex.SetBlendMode(sdl.BLENDMODE_BLEND); err != nil {
		tex.Destroy()
		return nil
	}

	prev := r.Renderer.GetRenderTarget()
	if err := r.Renderer.SetRenderTarget(tex); err != nil {
		tex.Destroy()
		logger.Debug("renderer: SetRenderTarget for pill: %v — using direct draw fallback", err)
		return nil
	}

	// Write raw RGBA into the texture (BLENDMODE_NONE avoids premultiplying
	// alpha against the transparent clear colour).
	r.Renderer.SetDrawBlendMode(sdl.BLENDMODE_NONE)
	r.Renderer.SetDrawColor(0, 0, 0, 0)
	r.Renderer.Clear()

	radius := h / 2
	if radius < 1 {
		radius = 1
	}
	r.Renderer.SetDrawColor(red, green, blue, 255)
	pillBodyBuf = sdl.Rect{X: radius, Y: 0, W: w - radius*2, H: h}
	r.Renderer.FillRect(&pillBodyBuf)
	drawFilledCircleRawAlpha(r.Renderer, radius, radius, radius, red, green, blue)
	drawFilledCircleRawAlpha(r.Renderer, w-radius, radius, radius, red, green, blue)

	r.Renderer.SetRenderTarget(prev)
	r.Renderer.SetDrawBlendMode(sdl.BLENDMODE_NONE)
	return tex
}

// DrawCircleBadge draws a filled circle badge (used for face buttons A, B).
// cx, cy is the center; d is the diameter.
func (r *Renderer) DrawCircleBadge(cx, cy, d int32, red, green, blue uint8) {
	drawFilledCircle(r.Renderer, cx, cy, d/2, red, green, blue)
}

// maxCircleRadius is the largest radius drawFilledCircle will receive.
// Devices range from 640×480 to 1280×720; footer badges are h/32 ≈ 15–22 px radius.
// 128 gives a safe margin without wasting stack space.
const maxCircleRadius = 128

// circleRectBuf is a reusable scratch buffer for drawFilledCircle.
// Solid rects occupy [0 : radius*2+1]; fringe rects follow immediately.
// Maximum total: (radius*2+1) + (radius*4+2) = radius*6+3.
// Package-level so CGo escape analysis never allocates it per call.
// Only accessed from the SDL main goroutine — no locking needed.
var circleRectBuf [maxCircleRadius*6 + 3]sdl.Rect

// circleExtentsCache stores the per-row x-extents (dxi) for each radius value,
// computed once on first use. Index is the radius (0 … maxCircleRadius).
// Only accessed from the SDL main goroutine — no locking needed.
var circleExtentsCache [maxCircleRadius + 1][]int32

// circleExtents returns the cached slice of x-extents for the given radius,
// computing and storing it on the first call for that value.
// Each element i is the half-width of the solid circle body at row i.
func circleExtents(radius int32) []int32 {
	if ext := circleExtentsCache[radius]; ext != nil {
		return ext
	}
	n := int(radius*2 + 1)
	r2 := float64(radius * radius)
	ext := make([]int32, n)
	for i := 0; i < n; i++ {
		dy := float64(int32(i) - radius)
		ext[i] = int32(math.Sqrt(math.Max(0, r2-dy*dy)))
	}
	circleExtentsCache[radius] = ext
	return ext
}

// drawFilledCircle draws a filled anti-aliased circle.
// The solid interior uses floor-quantised extents; a 1px fringe at 50% alpha
// softens the staircase edge. Two FillRects calls; no per-pixel alpha variation.
// Safe to call inside DrawPill — blend mode is restored to NONE on return.
// AA fringe is skipped for radius < 4: at that size the fringe is sub-pixel and
// saving 4 CGo calls per invocation outweighs the imperceptible quality loss.
// Per-row x-extents are cached by radius so math.Sqrt is only called once per
// distinct radius value across the entire session.
func drawFilledCircle(ren *sdl.Renderer, cx, cy, radius int32, red, green, blue uint8) {
	if radius > maxCircleRadius {
		radius = maxCircleRadius
	}
	ext := circleExtents(radius)
	n := len(ext)

	// Fill circleRectBuf using cached extents — no sqrt per call.
	for i, dxi := range ext {
		iy := cy + int32(i) - radius
		circleRectBuf[i] = sdl.Rect{X: cx - dxi, Y: iy, W: dxi*2 + 1, H: 1}
		// Fringe pixels one step outside the solid body.
		circleRectBuf[n+i*2] = sdl.Rect{X: cx - dxi - 1, Y: iy, W: 1, H: 1}
		circleRectBuf[n+i*2+1] = sdl.Rect{X: cx + dxi + 1, Y: iy, W: 1, H: 1}
	}

	ren.SetDrawBlendMode(sdl.BLENDMODE_NONE)
	ren.SetDrawColor(red, green, blue, 255)
	ren.FillRects(circleRectBuf[:n])

	if radius >= 4 {
		ren.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
		ren.SetDrawColor(red, green, blue, 128)
		ren.FillRects(circleRectBuf[n : n+n*2])
		ren.SetDrawBlendMode(sdl.BLENDMODE_NONE)
	}
}

// drawFilledCircleRawAlpha is identical to drawFilledCircle but always uses
// BLENDMODE_NONE, writing literal alpha values into the target. Used when
// rendering into a texture so that fringe pixels are stored as (r,g,b,128)
// rather than blended against the transparent clear colour.
func drawFilledCircleRawAlpha(ren *sdl.Renderer, cx, cy, radius int32, red, green, blue uint8) {
	if radius > maxCircleRadius {
		radius = maxCircleRadius
	}
	ext := circleExtents(radius)
	n := len(ext)
	for i, dxi := range ext {
		iy := cy + int32(i) - radius
		circleRectBuf[i] = sdl.Rect{X: cx - dxi, Y: iy, W: dxi*2 + 1, H: 1}
		circleRectBuf[n+i*2] = sdl.Rect{X: cx - dxi - 1, Y: iy, W: 1, H: 1}
		circleRectBuf[n+i*2+1] = sdl.Rect{X: cx + dxi + 1, Y: iy, W: 1, H: 1}
	}
	ren.SetDrawColor(red, green, blue, 255)
	ren.FillRects(circleRectBuf[:n])
	if radius >= 4 {
		ren.SetDrawColor(red, green, blue, 128)
		ren.FillRects(circleRectBuf[n : n+n*2])
	}
}

// MeasureTagPills returns the total pixel height that DrawTagPills would consume
// for the given tags without rendering anything.
func (r *Renderer) MeasureTagPills(tags []string, x, maxW, lineH int32) int32 {
	if len(tags) == 0 {
		return 0
	}
	const hPad = int32(8)
	const vPad = int32(2)
	const gap = int32(8)

	_, textH := r.SmallTextSize("Ag")
	pillH := textH + vPad*2

	cx := x
	cy := int32(0)
	for _, tag := range tags {
		tw, _ := r.SmallTextSize(tag)
		pillW := tw + hPad*2
		if cx > x && cx+pillW > x+maxW {
			cx = x
			cy += lineH
		}
		cx += pillW + gap
	}
	return cy + pillH
}

// DrawTagPills renders a slice of tag strings as pill badges that wrap across
// lines. Each pill has fgR/fgG/fgB text on bgR/bgG/bgB background.
// maxW is the available pixel width; lineH is the vertical step between rows.
// Returns the total height consumed (0 if tags is empty).
func (r *Renderer) DrawTagPills(tags []string, x, y, maxW, lineH int32,
	fgR, fgG, fgB, bgR, bgG, bgB uint8) int32 {

	const hPad = int32(8)
	const vPad = int32(2)
	const gap = int32(8)

	_, textH := r.SmallTextSize("Ag")
	pillH := textH + vPad*2

	cx := x
	cy := y

	for _, tag := range tags {
		tw, _ := r.SmallTextSize(tag)
		pillW := tw + hPad*2
		if cx > x && cx+pillW > x+maxW {
			cx = x
			cy += lineH
		}
		r.DrawPill(cx, cy, pillW, pillH, bgR, bgG, bgB)
		r.DrawSmallTextCenteredInRect(tag, cx, cy, pillW, pillH, fgR, fgG, fgB)
		cx += pillW + gap
	}
	if len(tags) == 0 {
		return 0
	}
	return cy - y + pillH
}

// DrawFooterHints renders the footer hint bar from a typed slice.
// Circle badges are used for face buttons (A, B); pill badges for function keys.
// y is the vertical center returned by DrawFooterBar.
func (r *Renderer) DrawFooterHints(hints []FooterHint, y int32) {
	ac := r.Theme.Accent
	acTxt := r.Theme.AccentText
	hint := r.Theme.HintText

	_, smallH := r.SmallTextSize("Ag")
	badgeDiam := smallH + 4
	cx := int32(10)

	for _, h := range hints {
		labelW, _ := r.SmallTextSize(h.Label)
		textW, _ := r.SmallTextSize(h.Text)

		switch h.Kind {
		case BadgeCircle:
			badgeCX := cx + int32(badgeDiam)/2
			badgeCY := y + smallH/2
			r.DrawCircleBadge(badgeCX, badgeCY, int32(badgeDiam), ac[0], ac[1], ac[2])
			r.DrawSmallTextCenteredInRect(h.Label, cx, badgeCY-int32(badgeDiam)/2, int32(badgeDiam), int32(badgeDiam), acTxt[0], acTxt[1], acTxt[2])
			cx += int32(badgeDiam) + 6
		case BadgePill:
			const hPad = int32(8)
			pillW := labelW + hPad*2
			pillH := smallH + 4
			pillY := y - 2
			r.DrawPill(cx, pillY, pillW, pillH, ac[0], ac[1], ac[2])
			r.DrawSmallTextCenteredInRect(h.Label, cx, pillY, pillW, pillH, acTxt[0], acTxt[1], acTxt[2])
			cx += pillW + 6
		}
		if h.Text != "" {
			r.DrawSmallText(h.Text, cx, y, hint[0], hint[1], hint[2])
			cx += textW + 14
		} else {
			cx += 8
		}
	}
}

// DrawModal draws a centred modal overlay with a title, wrapped body text, and
// footer hints. Used for confirmations and informational dialogs across all screens.
func (r *Renderer) DrawModal(title, body string, hints []FooterHint) {
	_, fontH := r.TextSize("Ag")
	lineH := fontH + 4

	marginX := r.W / 8
	pad := int32(20)
	panelW := r.W - marginX*2
	bodyMaxW := panelW - pad*2

	bodyLines := r.WrapText(body, bodyMaxW)
	bodyH := int32(len(bodyLines)) * lineH
	hintsH := int32(44)
	panelH := pad + fontH + pad/2 + bodyH + pad + hintsH
	if panelH > r.H*4/5 {
		panelH = r.H * 4 / 5
	}

	panelX := marginX
	panelY := (r.H - panelH) / 2

	bg := r.Theme.Background
	// Fill
	r.DrawRect(panelX, panelY, panelW, panelH, bg[0]+20, bg[1]+20, bg[2]+20)
	// Border (1px on each edge)
	r.DrawRect(panelX, panelY, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY+panelH-1, panelW, 1, 70, 70, 100)
	r.DrawRect(panelX, panelY, 1, panelH, 70, 70, 100)
	r.DrawRect(panelX+panelW-1, panelY, 1, panelH, 70, 70, 100)

	// Title
	mt := r.Theme.MainText
	r.DrawTextCentered(title, panelX, panelY+pad, panelW, mt[0], mt[1], mt[2])

	// Body
	ht := r.Theme.HintText
	r.DrawWrappedText(body, panelX+pad, panelY+pad+fontH+pad/2, bodyMaxW, lineH, ht[0], ht[1], ht[2])

	// Hints
	r.DrawFooterHints(hints, panelY+panelH-hintsH)
}
