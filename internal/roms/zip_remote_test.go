package roms_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// buildTestZIP creates an in-memory ZIP with the given files (name → content).
func buildTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	return buf.Bytes()
}

func TestInspectRemoteZIP_RangeSupported(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"game.gbc":    "romdata",
		"track01.mp3": "musicdata",
		"readme.txt":  "textdata",
	})

	var rangeRequestCount, fullBodyRequestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			// HEAD requests always go through ServeContent
			http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(data))
			return
		}
		if r.Header.Get("Range") != "" {
			atomic.AddInt32(&rangeRequestCount, 1)
		} else {
			atomic.AddInt32(&fullBodyRequestCount, 1)
		}
		http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip", nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP error: %v", err)
	}
	if atomic.LoadInt32(&rangeRequestCount) == 0 {
		t.Error("no Range requests issued — expected Range path to be used")
	}
	if atomic.LoadInt32(&fullBodyRequestCount) > 0 {
		t.Errorf("full-body GET was issued (%d times) — expected Range-only access", atomic.LoadInt32(&fullBodyRequestCount))
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1", manifest.ROMCount())
	}
	if manifest.MusicCount() != 1 {
		t.Errorf("MusicCount = %d, want 1", manifest.MusicCount())
	}
	otherCount := 0
	for _, e := range manifest.Entries {
		if e.Kind == roms.KindOther {
			otherCount++
		}
	}
	if otherCount != 1 {
		t.Errorf("KindOther count = %d, want 1", otherCount)
	}
}

func TestInspectRemoteZIP_FallbackOn200(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"game.gb": "romdata",
	})

	// Server returns 200 for everything (no range support signalled via Accept-Ranges)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		// No Accept-Ranges header
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip", nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP fallback error: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1", manifest.ROMCount())
	}
}

// TestInspectRemoteZIP_HeadForbiddenRangeProbe simulates the itch.io Butler CDN
// behaviour where HEAD returns 403 but GET with Range works. The inspector must
// succeed via the Range probe path without a full download.
func TestInspectRemoteZIP_HeadForbiddenRangeProbe(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"game.gbc":    "romdata",
		"track01.mp3": "musicdata",
	})

	var rangeCount, fullBodyCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Header.Get("Range") != "" {
			atomic.AddInt32(&rangeCount, 1)
		} else {
			atomic.AddInt32(&fullBodyCount, 1)
		}
		http.ServeContent(w, r, "test.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip", nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP error: %v", err)
	}
	if atomic.LoadInt32(&rangeCount) == 0 {
		t.Error("no Range requests issued — expected range probe path after 403 HEAD")
	}
	if atomic.LoadInt32(&fullBodyCount) > 0 {
		t.Errorf("full-body GET issued %d time(s) — expected range-only access", atomic.LoadInt32(&fullBodyCount))
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1", manifest.ROMCount())
	}
	if manifest.MusicCount() != 1 {
		t.Errorf("MusicCount = %d, want 1", manifest.MusicCount())
	}
}

func TestInspectRemoteZIP_MusicOnly(t *testing.T) {
	data := buildTestZIP(t, map[string]string{
		"track01.mp3": "music1",
		"track02.ogg": "music2",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(srv.Client(), srv.URL+"/test.zip", nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if manifest.HasROMs() {
		t.Error("HasROMs() = true, want false")
	}
	if manifest.MusicCount() != 2 {
		t.Errorf("MusicCount = %d, want 2", manifest.MusicCount())
	}
}

func TestInspectRemoteZIP_PNGNotPromotedByMagic(t *testing.T) {
	// A ZIP containing one real .p8.png cart plus .png exports for raspi/linux.
	// The .png files are 128-px-wide PNGs that would be detected as .p8.png by
	// magic bytes, but must not be counted as ROMs because .png is an image ext.
	// Minimal valid 128-px-wide PNG (1×1 is fine for testing the guard).
	pngHeader := []byte{
		0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, // PNG magic
		0x00, 0x00, 0x00, 0x0D, // IHDR chunk length
		'I', 'H', 'D', 'R', // "IHDR"
		0x00, 0x00, 0x00, 0x80, // width = 128 (0x80)
		0x00, 0x00, 0x00, 0x80, // height = 128
	}
	files := map[string]string{
		"game/pico8/cart.p8.png": "cart",
		"game/raspi/cart.png":    string(pngHeader),
		"game/linux/cart.png":    string(pngHeader),
	}
	data := buildTestZIP(t, files)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "game.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(http.DefaultClient, srv.URL, nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1 (.png platform exports must not be promoted to ROM)", manifest.ROMCount())
	}
}

func TestInspectRemoteZIP_NestedZIPNotPromotedByMagic(t *testing.T) {
	innerZIP := buildTestZIP(t, map[string]string{
		"Deadeus.gb": "romdata",
	})
	data := buildTestZIP(t, map[string]string{
		"Deadeus.gb":  "romdata",
		"Deadeus.zip": string(innerZIP),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "game.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(http.DefaultClient, srv.URL, nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Fatalf("ROMCount = %d, want 1 (nested ZIP must not be promoted to ROM)", manifest.ROMCount())
	}
	if got := len(manifest.ROMsByExt()[".gb"]); got != 1 {
		t.Fatalf(".gb ROM count = %d, want 1", got)
	}
	for _, e := range manifest.Entries {
		if e.Name == "Deadeus.zip" && e.Kind != roms.KindOther {
			t.Fatalf("Deadeus.zip kind = %v, want KindOther", e.Kind)
		}
	}
}

func TestInspectRemoteZIP_MacOSMetaDirExcluded(t *testing.T) {
	// ZIPs created on macOS include a __MACOSX/ directory containing resource
	// fork stubs. These have ROM extensions (._file.p8.png) but must not be
	// counted as playable files. Verify they are excluded from the manifest.
	files := map[string]string{
		"game/pico8/moss_moss.p8.png":                  "cart",
		"__MACOSX/game/pico8/._moss_moss.p8.png":       "mac-meta",
		"__MACOSX/game/pico8/._moss_moss_other.p8.png": "mac-meta",
		"game/pico8/._moss_moss_local.p8.png":          "mac-meta",
	}
	data := buildTestZIP(t, files)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "game.zip", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	manifest, err := roms.InspectRemoteZIP(http.DefaultClient, srv.URL, nil)
	if err != nil {
		t.Fatalf("InspectRemoteZIP: %v", err)
	}
	if manifest.ROMCount() != 1 {
		t.Errorf("ROMCount = %d, want 1 (macOS metadata must be excluded)", manifest.ROMCount())
	}
	p8png := manifest.ROMsByExt()[".p8.png"]
	if len(p8png) != 1 {
		t.Errorf("p8.png count = %d, want 1", len(p8png))
	}
	if len(p8png) == 1 && p8png[0].Name != "moss_moss.p8.png" {
		t.Errorf("p8.png name = %q, want %q", p8png[0].Name, "moss_moss.p8.png")
	}
}
