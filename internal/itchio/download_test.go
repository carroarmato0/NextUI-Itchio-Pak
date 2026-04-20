package itchio_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestDownloadFreeStreamsFile(t *testing.T) {
	content := []byte("ROM_CONTENT_BYTES")

	mux := http.NewServeMux()

	// Game page: provides CSRF token (used for the download_url POST)
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><input name="csrf_token" value="tok123"/></body></html>`))
	})

	// download_url endpoint: returns the signed download page URL (absolute URL).
	// The key path segment is a base64-encoded JWT: {"id":42,"expires":9999999999}
	// base64({"id":42,"expires":9999999999}) = eyJpZCI6NDIsImV4cGlyZXMiOjk5OTk5OTk5OTl9
	// srv is declared after NewServer, so we capture it via a pointer.
	var srvURL string
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		// Key contains {"id":42,...} so extractKeyID returns "42"
		json.NewEncoder(w).Encode(map[string]string{"url": srvURL + "/download/eyJpZCI6NDIsImV4cGlyZXMiOjk5OTk5OTk5OTl9.SIGNATURE"})
	})

	// Signed download page: real itch.io structure with CSRF token + data-upload_id
	mux.HandleFunc("/download/eyJpZCI6NDIsImV4cGlyZXMiOjk5OTk5OTk5OTl9.SIGNATURE", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="PAGECSRF"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column">
      <div class="upload_name">
        <strong class="name" title="game.gbc">game.gbc</strong>
      </div>
    </div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="99999">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	})

	// File resolver: POST with download_key_id (numeric) + csrf_token returns CDN URL
	mux.HandleFunc("/game/file/99999", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST to resolver, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.FormValue("download_key_id") != "42" {
			t.Errorf("expected download_key_id=42 in form body, got %q", r.FormValue("download_key_id"))
		}
		if r.FormValue("csrf_token") != "PAGECSRF" {
			t.Errorf("expected csrf_token=PAGECSRF in form body, got %q", r.FormValue("csrf_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": srvURL + "/cdn/game.gbc"})
	})

	// CDN: actual file content
	mux.HandleFunc("/cdn/game.gbc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "17")
		w.Write(content)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL // set after server starts so the handler closure can use it

	c := itchio.NewClientWithBase(srv.URL)

	// FetchUploads should return one .gbc upload with resolver URL
	uploads, err := c.FetchUploads(srv.URL + "/game")
	if err != nil {
		t.Fatalf("FetchUploads: %v", err)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "game.gbc" {
		t.Errorf("expected filename game.gbc, got %q", uploads[0].Filename)
	}

	// DownloadFree should resolve the CDN URL and stream the file
	dest := filepath.Join(t.TempDir(), "game.gbc")
	upload := itchio.Upload{Filename: uploads[0].Filename, URL: uploads[0].URL}
	if err := c.DownloadFree(srv.URL+"/game", upload, dest, nil); err != nil {
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

func TestFetchAuthUploads(t *testing.T) {
	// Intercept both api.itch.io (owned-keys) and itch.io (uploads).
	// We override the client's base and also serve the owned-keys path on the
	// same test server to avoid network calls.
	mux := http.NewServeMux()

	// Butler-style owned-keys endpoint (normally on api.itch.io).
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mykey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[{"id":999}]}`))
	})

	// v1 uploads endpoint with download_key_id.
	mux.HandleFunc("/api/1/mykey/game/12345/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download_key_id") != "999" {
			http.Error(w, "bad key", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uploads":[{"id":777,"filename":"game.gbc"},{"id":888,"filename":"manual.pdf"}]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("mykey", "12345")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "999" {
		t.Errorf("expected keyID=999, got %q", keyID)
	}
	// Only .gbc should be returned — .pdf must be filtered out.
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "game.gbc" || uploads[0].UploadID != "777" {
		t.Errorf("unexpected upload: %+v", uploads[0])
	}
}
