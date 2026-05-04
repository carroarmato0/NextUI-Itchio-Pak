package inventory_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// minimalPNG returns the bytes of a 1x1 white PNG.
func minimalPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestUpdateService_RepairsMissingCoverArt(t *testing.T) {
	pngData := minimalPNG()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cover.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	if err := os.WriteFile(romPath, []byte("ROM"), 0644); err != nil {
		t.Fatal(err)
	}

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	// Use srv.URL+"/game" as gameURL; FetchUploads will 404 but cover art runs first.
	gameURL := srv.URL + "/game"
	inv.Add(gameURL,
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}

	client := itchio.NewClientWithBase(srv.URL)
	svc := inventory.NewUpdateService(inv, invPath, client, nil)

	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	if _, err := os.Stat(artPath); err != nil {
		t.Errorf("cover art not created at %s: %v", artPath, err)
	}
}

func TestUpdateService_SkipsCoverArtIfPresent(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cover.png" {
			callCount++
			w.Header().Set("Content-Type", "image/png")
			w.Write(minimalPNG())
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)

	// Pre-create the cover art so it already exists.
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	// IsFree: false → checkPaidGame; no FetchUploads call, avoids file-listing HTTP traffic.
	gameURL := srv.URL + "/game"
	inv.Add(gameURL,
		inventory.Entry{Title: "G", IsFree: false, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if callCount != 0 {
		t.Errorf("cover art HTTP GET called %d times, want 0 (art already present)", callCount)
	}
}

// freeGameServer builds an httptest.Server that mimics a free itch.io game page
// with the given upload filenames. Pass status 404 to simulate a removed game.
func freeGameServer(t *testing.T, status int, filenames []string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()

	if status != http.StatusOK {
		mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", status)
		})
		srv = httptest.NewServer(mux)
		return srv
	}

	var srvURL string

	// GET /game — game page with CSRF token
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="TESTCSRF"/></head></html>`))
	})

	// POST /game/download_url — returns signed URL (last path segment is the key)
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(map[string]string{"url": srvURL + "/dl/TESTKEY"})
		w.Write(data)
	})

	// GET /dl/TESTKEY — signed download page with upload list
	mux.HandleFunc("/dl/TESTKEY", func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		body.WriteString(`<html><head><meta name="csrf_token" value="DLCSRF"/></head><body>`)
		for i, fn := range filenames {
			body.WriteString(`<div class="upload"><div class="info_column"><div class="upload_name">`)
			body.WriteString(`<strong class="name" title="` + fn + `">` + fn + `</strong>`)
			body.WriteString(`</div></div><div class="actions">`)
			body.WriteString(fmt.Sprintf(`<a class="button download_btn" href="javascript:void(0);" data-upload_id="%d">Download</a>`, 100+i))
			body.WriteString(`</div></div>`)
		}
		body.WriteString(`</body></html>`)
		w.Write(body.Bytes())
	})

	srv = httptest.NewServer(mux)
	srvURL = srv.URL
	return srv
}

// freeGameServerDynamic is like freeGameServer but reads filenames from the
// pointed-to slice at request time, so callers can mutate it between runs.
func freeGameServerDynamic(t *testing.T, filenames *[]string) *httptest.Server {
	t.Helper()
	var srvURL string
	mux := http.NewServeMux()

	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="TESTCSRF"/></head></html>`))
	})
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(map[string]string{"url": srvURL + "/dl/TESTKEY"})
		w.Write(data)
	})
	mux.HandleFunc("/dl/TESTKEY", func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		body.WriteString(`<html><head><meta name="csrf_token" value="DLCSRF"/></head><body>`)
		for i, fn := range *filenames {
			body.WriteString(`<div class="upload"><div class="info_column"><div class="upload_name">`)
			body.WriteString(`<strong class="name" title="` + fn + `">` + fn + `</strong>`)
			body.WriteString(`</div></div><div class="actions">`)
			body.WriteString(fmt.Sprintf(`<a class="button download_btn" href="javascript:void(0);" data-upload_id="%d">Download</a>`, 100+i))
			body.WriteString(`</div></div>`)
		}
		body.WriteString(`</body></html>`)
		w.Write(body.Bytes())
	})

	srv := httptest.NewServer(mux)
	srvURL = srv.URL
	return srv
}

func TestUpdateService_Marks404AsRemoved(t *testing.T) {
	srv := freeGameServer(t, http.StatusNotFound, nil)
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	// Pre-create cover art so repair is skipped.
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if !inv.IsRemoved(srv.URL + "/game") {
		t.Error("IsRemoved: want true after 404 from upstream")
	}
}

func TestUpdateService_DiffAddsNewFile(t *testing.T) {
	// First check: only game.gb is available. Second check: game-v2.gb is added.
	filenames := []string{"game.gb"}
	srv := freeGameServerDynamic(t, &filenames)
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)

	// First check: establishes baseline (game.gb only, IsNew=false for all).
	done1 := make(chan struct{})
	svc1 := inventory.NewUpdateService(inv, invPath, client, nil)
	svc1.Start(func() { close(done1) })
	<-done1
	svc1.Stop()

	if inv.HasPendingUpdates(srv.URL + "/game") {
		t.Error("HasPendingUpdates: want false after first check (no genuinely new files yet)")
	}

	// Developer publishes game-v2.gb.
	filenames = append(filenames, "game-v2.gb")

	// Second check: game-v2.gb appears and is flagged as genuinely new.
	done2 := make(chan struct{})
	svc2 := inventory.NewUpdateService(inv, invPath, client, nil)
	svc2.Start(func() { close(done2) })
	<-done2
	svc2.Stop()

	if !inv.HasPendingUpdates(srv.URL + "/game") {
		t.Error("HasPendingUpdates: want true after new file detected upstream on second check")
	}
}

func TestUpdateService_DiffPrunesVanishedFile(t *testing.T) {
	// Game page now only has game.gb — game-v2.gb has vanished.
	srv := freeGameServer(t, http.StatusOK, []string{"game.gb"})
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	// Seed a previously-known file that has now vanished.
	inv.SetUpstreamFiles(srv.URL+"/game", []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "100", SeenAt: time.Now().Add(-time.Hour)},
		{Filename: "game-v2.gb", UploadID: "101", SeenAt: time.Now().Add(-time.Hour)},
	})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	e, _ := inv.Lookup(srv.URL + "/game")
	for _, u := range e.KnownUpstreamFiles {
		if u.Filename == "game-v2.gb" {
			t.Error("vanished file game-v2.gb should have been pruned from KnownUpstreamFiles")
		}
	}
}

func TestUpdateService_MarksRemovedWhenDownloadedFileVanishesFromStore(t *testing.T) {
	// Upstream now only has game-v2.gb — the originally downloaded game.gb is gone.
	srv := freeGameServer(t, http.StatusOK, []string{"game-v2.gb"})
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	inv.Add(srv.URL+"/game",
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if !inv.IsRemoved(srv.URL + "/game") {
		t.Error("IsRemoved: want true when downloaded file is no longer available upstream")
	}
}

func TestUpdateService_ClearsRemovedWhenDownloadedFileReappearsInStore(t *testing.T) {
	// Upstream has game.gb again after it was previously removed.
	srv := freeGameServer(t, http.StatusOK, []string{"game.gb"})
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	gameURL := srv.URL + "/game"
	inv.Add(gameURL,
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	// Seed a prior removal state.
	inv.MarkRemoved(gameURL)
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)
	done := make(chan struct{})
	svc := inventory.NewUpdateService(inv, invPath, client, nil)
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	if inv.IsRemoved(gameURL) {
		t.Error("IsRemoved: want false when downloaded file has reappeared upstream")
	}
}

func TestUpdateService_DismissedUpdateDoesNotReappearOnRestart(t *testing.T) {
	// Start with only game.gb; game-v2.gb appears after first check.
	filenames := []string{"game.gb"}
	srv := freeGameServerDynamic(t, &filenames)
	defer srv.Close()

	dir := t.TempDir()
	romPath := filepath.Join(dir, "game.gb")
	os.WriteFile(romPath, []byte("ROM"), 0644)
	artPath := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
	os.MkdirAll(filepath.Dir(artPath), 0755)
	os.WriteFile(artPath, minimalPNG(), 0644)

	invPath := filepath.Join(dir, "inventory.json")
	inv := &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
	gameURL := srv.URL + "/game"
	inv.Add(gameURL,
		inventory.Entry{Title: "G", IsFree: true, CoverURL: srv.URL + "/cover.png"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	inv.Save(invPath)

	client := itchio.NewClientWithBase(srv.URL)

	// First check: baseline (game.gb only), no pending updates yet.
	done1 := make(chan struct{})
	svc1 := inventory.NewUpdateService(inv, invPath, client, nil)
	svc1.Start(func() { close(done1) })
	<-done1
	svc1.Stop()
	if inv.HasPendingUpdates(gameURL) {
		t.Fatal("HasPendingUpdates: want false after first check (no new files yet)")
	}

	// Developer publishes game-v2.gb.
	filenames = append(filenames, "game-v2.gb")

	// Second check: game-v2.gb detected as genuinely new.
	done2 := make(chan struct{})
	svc2 := inventory.NewUpdateService(inv, invPath, client, nil)
	svc2.Start(func() { close(done2) })
	<-done2
	svc2.Stop()
	if !inv.HasPendingUpdates(gameURL) {
		t.Fatal("HasPendingUpdates: want true after new file detected on second check")
	}

	// User dismisses the update and we save to disk.
	inv.DismissUpdate(gameURL)
	if err := inv.Save(invPath); err != nil {
		t.Fatal(err)
	}
	if inv.HasPendingUpdates(gameURL) {
		t.Fatal("HasPendingUpdates: want false immediately after dismiss")
	}

	// Simulate app restart: reload inventory from disk, run third check.
	inv3, _ := inventory.Load(invPath)
	done3 := make(chan struct{})
	svc3 := inventory.NewUpdateService(inv3, invPath, client, nil)
	svc3.Start(func() { close(done3) })
	<-done3
	svc3.Stop()

	if inv3.HasPendingUpdates(gameURL) {
		t.Error("HasPendingUpdates: want false — dismissed update must not reappear after restart + re-check")
	}
}
