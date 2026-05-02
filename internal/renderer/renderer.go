//go:build !headless

package renderer

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
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

type Renderer struct {
	Window        *sdl.Window
	Renderer      *sdl.Renderer
	Font          *ttf.Font   // main font (~35px at 768)
	SmallFont     *ttf.Font   // hint/footer font (~24px at 768)
	primaryRanges []cmapRange // codepoints covered by Font, parsed at init
	fallbacks     []fallbackFont
	W, H          int32
	Theme         theme.Theme
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

// fontIndex returns 0 if the primary font covers ch, or the 1-based index of
// the first fallback font that covers it, or 0 if no fallback covers it either
// (rendering will use the primary font and may show tofu).
func (r *Renderer) fontIndex(ch rune) int {
	if r.primaryRanges == nil || inRanges(r.primaryRanges, ch) {
		return 0
	}
	for i, fb := range r.fallbacks {
		if fb.ranges == nil || inRanges(fb.ranges, ch) {
			return i + 1
		}
	}
	return 0
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

func (r *Renderer) DrawRect(x, y, w, h int32, red, green, blue uint8) {
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.FillRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

func (r *Renderer) DrawText(text string, x, y int32, red, green, blue uint8) error {
	runs := splitTextRuns(sanitizeText(text), r.fontIndex)
	color := sdl.Color{R: red, G: green, B: blue, A: 255}
	cx := x
	for _, run := range runs {
		font := r.mainFont(run.fontIdx)
		surface, err := font.RenderUTF8Blended(run.text, color)
		if err != nil {
			return err
		}
		texture, err := r.Renderer.CreateTextureFromSurface(surface)
		surface.Free()
		if err != nil {
			return err
		}
		_, _, tw, th, _ := texture.Query()
		r.Renderer.Copy(texture, nil, &sdl.Rect{X: cx, Y: y, W: tw, H: th})
		texture.Destroy()
		cx += tw
	}
	return nil
}

// TextSize returns the pixel width and height of text without drawing it.
func (r *Renderer) TextSize(text string) (int32, int32) {
	runs := splitTextRuns(sanitizeText(text), r.fontIndex)
	var totalW, maxH int32
	for _, run := range runs {
		w, h, err := r.mainFont(run.fontIdx).SizeUTF8(run.text)
		if err != nil {
			continue
		}
		totalW += int32(w)
		if int32(h) > maxH {
			maxH = int32(h)
		}
	}
	return totalW, maxH
}

// DrawBoldText renders text using SDL_ttf bold style synthesis.
func (r *Renderer) DrawBoldText(text string, x, y int32, red, green, blue uint8) error {
	r.Font.SetStyle(ttf.STYLE_BOLD)
	defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	return r.DrawText(text, x, y, red, green, blue)
}

// BoldTextSize returns the pixel width and height of text measured in bold style.
func (r *Renderer) BoldTextSize(text string) (int32, int32) {
	r.Font.SetStyle(ttf.STYLE_BOLD)
	defer r.Font.SetStyle(ttf.STYLE_NORMAL)
	return r.TextSize(text)
}

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

func (r *Renderer) DrawTextureAt(tex *sdl.Texture, x, y, w, h int32) {
	r.Renderer.Copy(tex, nil, &sdl.Rect{X: x, Y: y, W: w, H: h})
}

// SetClipRect sets the clipping rectangle for rendering.
func (r *Renderer) SetClipRect(x, y, w, h int32) {
	rect := sdl.Rect{X: x, Y: y, W: w, H: h}
	r.Renderer.SetClipRect(&rect)
}

// ClearClipRect removes any clipping rectangle.
func (r *Renderer) ClearClipRect() {
	r.Renderer.SetClipRect(nil)
}

// WrapText breaks text into lines that fit within maxWidth pixels.
func (r *Renderer) WrapText(text string, maxWidth int32) []string {
	// Sanitize once up front so measurements (via TextSize) match rendering.
	text = sanitizeText(text)
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
	runs := splitTextRuns(sanitizeText(text), r.fontIndex)
	color := sdl.Color{R: red, G: green, B: blue, A: 255}
	cx := x
	for _, run := range runs {
		font := r.smallFont(run.fontIdx)
		surface, err := font.RenderUTF8Blended(run.text, color)
		if err != nil {
			return err
		}
		texture, err := r.Renderer.CreateTextureFromSurface(surface)
		surface.Free()
		if err != nil {
			return err
		}
		_, _, tw, th, _ := texture.Query()
		r.Renderer.Copy(texture, nil, &sdl.Rect{X: cx, Y: y, W: tw, H: th})
		texture.Destroy()
		cx += tw
	}
	return nil
}

// SmallTextSize returns the pixel width and height of text in the small font.
func (r *Renderer) SmallTextSize(text string) (int32, int32) {
	runs := splitTextRuns(sanitizeText(text), r.fontIndex)
	var totalW, maxH int32
	for _, run := range runs {
		w, h, err := r.smallFont(run.fontIdx).SizeUTF8(run.text)
		if err != nil {
			continue
		}
		totalW += int32(w)
		if int32(h) > maxH {
			maxH = int32(h)
		}
	}
	return totalW, maxH
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
// The border radius is clamped to h/2 so it is always a true capsule.
func (r *Renderer) DrawPill(x, y, w, h int32, red, green, blue uint8) {
	radius := h / 2
	if radius < 1 {
		radius = 1
	}
	r.Renderer.SetDrawColor(red, green, blue, 255)
	r.Renderer.FillRect(&sdl.Rect{X: x + radius, Y: y, W: w - radius*2, H: h})
	drawFilledCircle(r.Renderer, x+radius, y+radius, radius, red, green, blue)
	drawFilledCircle(r.Renderer, x+w-radius, y+radius, radius, red, green, blue)
}

// DrawCircleBadge draws a filled circle badge (used for face buttons A, B).
// cx, cy is the center; d is the diameter.
func (r *Renderer) DrawCircleBadge(cx, cy, d int32, red, green, blue uint8) {
	drawFilledCircle(r.Renderer, cx, cy, d/2, red, green, blue)
}

// drawFilledCircle draws a filled circle using a midpoint scanline algorithm.
func drawFilledCircle(ren *sdl.Renderer, cx, cy, radius int32, red, green, blue uint8) {
	ren.SetDrawColor(red, green, blue, 255)
	for dy := -radius; dy <= radius; dy++ {
		dx := int32(math.Round(math.Sqrt(float64(radius*radius - dy*dy))))
		ren.DrawLine(cx-dx, cy+dy, cx+dx, cy+dy)
	}
}

// MeasureTagPills returns the total pixel height that DrawTagPills would consume
// for the given tags without rendering anything.
func (r *Renderer) MeasureTagPills(tags []string, x, maxW, lineH int32) int32 {
	if len(tags) == 0 {
		return 0
	}
	const hPad = int32(6)
	const gap = int32(6)
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
	return cy + lineH
}

// DrawTagPills renders a slice of tag strings as pill badges that wrap across
// lines. Each pill has fgR/fgG/fgB text on bgR/bgG/bgB background.
// maxW is the available pixel width; lineH is the vertical step between rows.
// Returns the total height consumed (0 if tags is empty).
func (r *Renderer) DrawTagPills(tags []string, x, y, maxW, lineH int32,
	fgR, fgG, fgB, bgR, bgG, bgB uint8) int32 {

	const hPad = int32(6)
	const vPad = int32(3)
	const gap = int32(6)

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
		r.DrawSmallText(tag, cx+hPad, cy+vPad, fgR, fgG, fgB)
		cx += pillW + gap
	}
	if len(tags) == 0 {
		return 0
	}
	return cy - y + lineH
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
			r.DrawSmallTextCentered(h.Label, cx, badgeCY-smallH/2, int32(badgeDiam), acTxt[0], acTxt[1], acTxt[2])
			cx += int32(badgeDiam) + 6
		case BadgePill:
			const hPad = int32(5)
			pillW := labelW + hPad*2
			pillH := smallH + 4
			pillY := y - 2
			r.DrawPill(cx, pillY, pillW, pillH, ac[0], ac[1], ac[2])
			r.DrawSmallText(h.Label, cx+hPad, pillY+2, acTxt[0], acTxt[1], acTxt[2])
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
