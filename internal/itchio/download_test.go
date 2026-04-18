package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestDownloadFreeStreamsFile(t *testing.T) {
	content := []byte("ROM_CONTENT_BYTES")
	var steps []string

	mux := http.NewServeMux()
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "game_page")
		// Provide csrf_token in a hidden input
		w.Write([]byte(`<html><body><input name="csrf_token" value="tok123"/></body></html>`))
	})
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "download_url_post")
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"url":"/download-page"}`))
	})
	mux.HandleFunc("/download-page", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "download_page")
		w.Write([]byte(`<a href="/file/game.gbc">game.gbc</a>`))
	})
	mux.HandleFunc("/file/game.gbc", func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, "file_download")
		w.Header().Set("Content-Length", "17")
		w.Write(content)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "game.gbc")
	c := itchio.NewClient()
	upload := itchio.Upload{Filename: "game.gbc", URL: srv.URL + "/file/game.gbc"}

	err := c.DownloadFree(srv.URL+"/game", upload, dest, nil)
	if err != nil {
		t.Fatalf("DownloadFree: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}
}

func TestCheckOwnership(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1/mykey/game/12345/download_keys" {
			w.Write([]byte(`{"download_keys":[{"id":1}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	owns, err := c.CheckOwnership("mykey", "12345")
	if err != nil {
		t.Fatalf("CheckOwnership: %v", err)
	}
	if !owns {
		t.Error("expected owns=true")
	}
}
