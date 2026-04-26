package itchio_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestFetchAuthUploadsViaGamePage verifies that the game-page Bearer auth path
// returns uploads and resolver URLs without touching the butler API at all.
// This is the primary auth path and handles both direct and bundle purchases.
func TestFetchAuthUploadsViaGamePage(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()

	// Game page returns CSRF token.
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gpkey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`<html><body><input name="csrf_token" value="GPCSRF"/></body></html>`))
	})

	// download_url POST with Bearer auth returns a signed URL.
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer gpkey" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"url": ""})
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.FormValue("csrf_token") != "GPCSRF" {
			t.Errorf("expected csrf_token=GPCSRF, got %q", r.FormValue("csrf_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"url": srvURL + "/game/download/eyJpZCI6MTIzLCJleHBpcmVzIjo5OTk5OTk5OTk5fQ.SIG",
		})
	})

	// Signed download page.
	mux.HandleFunc("/game/download/eyJpZCI6MTIzLCJleHBpcmVzIjo5OTk5OTk5OTk5fQ.SIG", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="PAGECSRF2"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column">
      <div class="upload_name">
        <strong class="name" title="gp-game.gbc">gp-game.gbc</strong>
      </div>
    </div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="77777">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBase(srv.URL)
	uploads, err := c.FetchAuthUploadsViaGamePage("gpkey", srv.URL+"/game")
	if err != nil {
		t.Fatalf("FetchAuthUploadsViaGamePage: %v", err)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "gp-game.gbc" {
		t.Errorf("Filename = %q, want gp-game.gbc", uploads[0].Filename)
	}
	if uploads[0].UploadID != "77777" {
		t.Errorf("UploadID = %q, want 77777", uploads[0].UploadID)
	}
	if uploads[0].URL == "" {
		t.Error("URL should be set for game-page path")
	}
	if !strings.Contains(uploads[0].URL, "/game/file/77777") {
		t.Errorf("URL should contain /game/file/77777, got %q", uploads[0].URL)
	}
	if uploads[0].NeedsFormat {
		t.Error("NeedsFormat should be false for .gbc file")
	}
}

// TestFetchAuthUploadsViaGamePage_NotOwned verifies that an empty URL from
// download_url (game not owned or Bearer auth rejected) produces an error.
func TestFetchAuthUploadsViaGamePage_NotOwned(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><input name="csrf_token" value="X"/></body></html>`))
	})
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": ""})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	_, err := c.FetchAuthUploadsViaGamePage("badkey", srv.URL+"/game")
	if err == nil {
		t.Fatal("expected error for not-owned game, got nil")
	}
	if !strings.Contains(err.Error(), "not owned") && !strings.Contains(err.Error(), "purchase") {
		t.Errorf("error should mention ownership/purchase, got: %v", err)
	}
}

// TestFetchAuthUploads_BundlePurchase verifies that when owned-keys returns a
// downloads_url (signed page URL), we use the signed-page path rather than the
// butler uploads endpoint. This is the correct path for bundle purchases.
func TestFetchAuthUploads_BundlePurchase(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bundlekey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// downloads_url present — no numeric id needed.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"owned_keys": []map[string]interface{}{
				{"id": 0, "downloads_url": srvURL + "/game/download/SIGNEDTOKEN"},
			},
		})
	})

	// Signed download page for the bundle purchase.
	mux.HandleFunc("/game/download/SIGNEDTOKEN", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="BCSRF"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column">
      <div class="upload_name">
        <strong class="name" title="bundle-game.gbc">bundle-game.gbc</strong>
      </div>
    </div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="55555">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	})

	// butler uploads endpoint must NOT be called (bundle key would 500).
	mux.HandleFunc("/games/321/uploads", func(w http.ResponseWriter, r *http.Request) {
		t.Error("uploads endpoint should not be called for bundle purchases")
		http.Error(w, "server error", http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("bundlekey", "321")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "" {
		t.Errorf("keyID should be empty for signed-page path, got %q", keyID)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "bundle-game.gbc" {
		t.Errorf("Filename = %q, want bundle-game.gbc", uploads[0].Filename)
	}
	if uploads[0].UploadID != "55555" {
		t.Errorf("UploadID = %q, want 55555", uploads[0].UploadID)
	}
	if uploads[0].URL == "" {
		t.Error("URL should be set for signed-page path")
	}
	if uploads[0].NeedsFormat {
		t.Error("NeedsFormat should be false for .gbc file")
	}
}

func TestFetchAuthUploads_UnknownExt(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testkey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[{"id":42}]}`))
	})

	mux.HandleFunc("/games/777/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testkey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("download_key_id") != "42" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uploads":[
			{"id":1,"filename":"game.gbc"},
			{"id":2,"filename":"manual.pdf"},
			{"id":3,"filename":"Glory Hunters 2.0"}
		]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("testkey", "777")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "42" {
		t.Errorf("keyID = %q, want 42", keyID)
	}
	// manual.pdf dropped; game.gbc (known) + Glory Hunters 2.0 (unknown) kept
	if len(uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(uploads))
	}

	gbc := uploads[0]
	if gbc.Filename != "game.gbc" {
		t.Errorf("uploads[0].Filename = %q, want game.gbc", gbc.Filename)
	}
	if gbc.NeedsFormat {
		t.Errorf("uploads[0].NeedsFormat = true, want false for .gbc file")
	}

	unknown := uploads[1]
	if unknown.Filename != "Glory Hunters 2.0" {
		t.Errorf("uploads[1].Filename = %q, want 'Glory Hunters 2.0'", unknown.Filename)
	}
	if !unknown.NeedsFormat {
		t.Errorf("uploads[1].NeedsFormat = false, want true for unknown ext")
	}
	if unknown.UploadID != "3" {
		t.Errorf("uploads[1].UploadID = %q, want 3", unknown.UploadID)
	}

	for _, u := range uploads {
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped (skippable ext)")
		}
	}
}

func TestFetchAuthUploads_NotOwned500(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Returns a key ID even though the user doesn't actually own the game
		// (itch.io can return non-zero IDs for non-owners in some cases).
		w.Write([]byte(`{"owned_keys":[{"id":99}]}`))
	})
	mux.HandleFunc("/games/999/uploads", func(w http.ResponseWriter, r *http.Request) {
		// itch.io returns 500 when download_key_id doesn't grant access.
		http.Error(w, `{"errors":["There was a server error"]}`, http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	_, _, err := c.FetchAuthUploads("testkey", "999")
	if err == nil {
		t.Fatal("expected error for non-owned game, got nil")
	}
	if !strings.Contains(err.Error(), "not owned") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention ownership/invalid key, got: %v", err)
	}
}

func TestFetchAuthUploads_ZipTreatedAsKnown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[{"id":42}]}`))
	})
	mux.HandleFunc("/games/555/uploads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uploads":[
			{"id":1,"filename":"game.gbc"},
			{"id":2,"filename":"Glory Hunters 2.0.zip"},
			{"id":3,"filename":"Glory Hunters 1.3 ROM Files.zip"},
			{"id":4,"filename":"Glory Hunters 2.0"},
			{"id":5,"filename":"manual.pdf"}
		]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, _, err := c.FetchAuthUploads("testkey", "555")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	// pdf dropped; gbc + 2×zip (known) + extensionless (NeedsFormat) = 4
	if len(uploads) != 4 {
		t.Fatalf("expected 4 uploads, got %d", len(uploads))
	}

	for _, u := range uploads {
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped")
		}
	}

	byName := make(map[string]itchio.Upload)
	for _, u := range uploads {
		byName[u.Filename] = u
	}

	if byName["game.gbc"].NeedsFormat {
		t.Errorf("game.gbc should not need format")
	}
	if byName["Glory Hunters 2.0.zip"].NeedsFormat {
		t.Errorf("Glory Hunters 2.0.zip should not need format (zip is a known extension)")
	}
	if byName["Glory Hunters 1.3 ROM Files.zip"].NeedsFormat {
		t.Errorf("Glory Hunters 1.3 ROM Files.zip should not need format")
	}
	if !byName["Glory Hunters 2.0"].NeedsFormat {
		t.Errorf("Glory Hunters 2.0 (no extension) should need format")
	}
}

func TestFetchAuthUploads_EmptyObjectResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[{"id":77}]}`))
	})
	mux.HandleFunc("/games/888/uploads", func(w http.ResponseWriter, r *http.Request) {
		// itch.io sometimes returns an object instead of an array for empty upload lists.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uploads":{}}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, _, err := c.FetchAuthUploads("testkey", "888")
	if err != nil {
		t.Fatalf("expected no error for empty uploads object, got: %v", err)
	}
	if len(uploads) != 0 {
		t.Errorf("expected 0 uploads, got %d", len(uploads))
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

	// Butler uploads endpoint — requires Authorization header and download_key_id query param.
	mux.HandleFunc("/games/12345/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mykey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
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
