package renderer

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

// makeTestGIF returns a 2×2 animated GIF with nFrames frames.
// Frame i gets palette index (i % 2), delay d[i] centiseconds.
func makeTestGIF(nFrames int, delays []int) *gif.GIF {
	palette := color.Palette{color.Black, color.White}
	images := make([]*image.Paletted, nFrames)
	for i := range images {
		img := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		img.SetColorIndex(0, 0, uint8(i%2))
		images[i] = img
	}
	d := delays
	if d == nil {
		d = make([]int, nFrames)
	}
	return &gif.GIF{
		Image:  images,
		Delay:  d,
		Config: image.Config{Width: 2, Height: 2, ColorModel: palette},
	}
}

func TestRenderGIFFramesCount(t *testing.T) {
	g := makeTestGIF(3, []int{10, 20, 15})
	anim := renderGIFFrames(g)
	if len(anim.frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(anim.frames))
	}
	if len(anim.delays) != 3 {
		t.Fatalf("expected 3 delays, got %d", len(anim.delays))
	}
}

func TestRenderGIFFramesCapAtMax(t *testing.T) {
	g := makeTestGIF(maxGIFFrames+5, nil)
	for i := range g.Delay {
		g.Delay[i] = 5
	}
	anim := renderGIFFrames(g)
	if len(anim.frames) != maxGIFFrames {
		t.Fatalf("expected frames capped at %d, got %d", maxGIFFrames, len(anim.frames))
	}
}

func TestRenderGIFFramesDimensions(t *testing.T) {
	g := makeTestGIF(2, []int{5, 5})
	anim := renderGIFFrames(g)
	if anim.w != 2 || anim.h != 2 {
		t.Fatalf("expected 2×2, got %d×%d", anim.w, anim.h)
	}
	if anim.pitch != 2*4 {
		t.Fatalf("expected pitch 8, got %d", anim.pitch)
	}
	for i, f := range anim.frames {
		if len(f) != int(anim.w)*int(anim.h)*4 {
			t.Fatalf("frame %d: expected %d bytes, got %d", i, int(anim.w)*int(anim.h)*4, len(f))
		}
	}
}

func TestGIFAnimZeroDelayDefaulted(t *testing.T) {
	g := makeTestGIF(2, []int{0, 0})
	anim := renderGIFFrames(g)
	for i, d := range anim.delays {
		if d != 100*time.Millisecond {
			t.Errorf("frame %d: delay = %v, want 100ms (zero-delay default)", i, d)
		}
	}
}

func TestGIFAnimNonZeroDelay(t *testing.T) {
	g := makeTestGIF(2, []int{10, 25}) // 10cs = 100ms, 25cs = 250ms
	anim := renderGIFFrames(g)
	if anim.delays[0] != 100*time.Millisecond {
		t.Errorf("frame 0: got %v, want 100ms", anim.delays[0])
	}
	if anim.delays[1] != 250*time.Millisecond {
		t.Errorf("frame 1: got %v, want 250ms", anim.delays[1])
	}
}

func TestGIFAnimFrameAdvance(t *testing.T) {
	base := time.Now()
	a := &gifAnim{
		frames: [][]uint8{make([]uint8, 4), make([]uint8, 4), make([]uint8, 4)},
		delays: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 150 * time.Millisecond},
		w:      1,
		h:      1,
		pitch:  4,
		cur:    0,
		nextAt: base,
	}

	// time.After is strict: not after base itself → no advance.
	if _, ok := a.advance(base); ok {
		t.Fatal("advance at nextAt (not strictly after) should not fire")
	}

	// 1ns after → should advance to frame 1.
	t0 := base.Add(time.Nanosecond)
	idx, ok := a.advance(t0)
	if !ok {
		t.Fatal("expected advance=true")
	}
	if idx != 1 {
		t.Fatalf("expected frame 1, got %d", idx)
	}
	wantNextAt := t0.Add(200 * time.Millisecond)
	if !a.nextAt.Equal(wantNextAt) {
		t.Fatalf("nextAt = %v, want %v", a.nextAt, wantNextAt)
	}

	// Same timestamp: no second advance.
	if _, ok := a.advance(t0); ok {
		t.Fatal("should not advance twice at same timestamp")
	}

	// Past frame 1 delay → frame 2.
	t1 := t0.Add(200*time.Millisecond + time.Nanosecond)
	idx, ok = a.advance(t1)
	if !ok || idx != 2 {
		t.Fatalf("expected advance to frame 2, got idx=%d ok=%v", idx, ok)
	}

	// Past frame 2 delay → wrap to frame 0.
	t2 := t1.Add(150*time.Millisecond + time.Nanosecond)
	idx, ok = a.advance(t2)
	if !ok || idx != 0 {
		t.Fatalf("expected wrap to frame 0, got idx=%d ok=%v", idx, ok)
	}
}

func TestGIFAnimAdvanceSingleFrame(t *testing.T) {
	a := &gifAnim{
		frames: [][]uint8{make([]uint8, 4)},
		delays: []time.Duration{100 * time.Millisecond},
		w:      1,
		h:      1,
		pitch:  4,
		cur:    0,
		nextAt: time.Now().Add(-time.Second),
	}
	idx, ok := a.advance(time.Now())
	if !ok || idx != 0 {
		t.Fatalf("single-frame: expected advance to 0, got idx=%d ok=%v", idx, ok)
	}
}
