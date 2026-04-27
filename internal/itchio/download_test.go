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

// TestFetchAuthUploads_BundlePurchaseKeyString verifies that when owned-keys
// returns no downloads_url but includes a key string, we construct the signed
// page URL from gameURL+key and use the signed-page path.
func TestFetchAuthUploads_BundlePurchaseKeyString(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bk" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// downloads_url absent; key string present (bundle purchase pattern).
		json.NewEncoder(w).Encode(map[string]interface{}{
			"owned_keys": []map[string]interface{}{
				{"id": 99, "key": "BUNDLEKEYSTRING", "downloads_url": ""},
			},
		})
	})

	// Signed page constructed from gameURL + key string.
	mux.HandleFunc("/game/download/BUNDLEKEYSTRING", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="BKCSRF"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column">
      <div class="upload_name">
        <strong class="name" title="bundle-ks.gbc">bundle-ks.gbc</strong>
      </div>
    </div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="66666">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	})

	// uploads endpoint must NOT be called.
	mux.HandleFunc("/api/1/bk/game/444/uploads", func(w http.ResponseWriter, r *http.Request) {
		t.Error("uploads endpoint should not be called when key string is present")
		http.Error(w, "server error", http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("bk", "444", srvURL+"/game")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "" {
		t.Errorf("keyID should be empty for signed-page path, got %q", keyID)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "bundle-ks.gbc" {
		t.Errorf("Filename = %q, want bundle-ks.gbc", uploads[0].Filename)
	}
	if uploads[0].URL == "" {
		t.Error("URL should be set for signed-page path")
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

	// uploads endpoint must NOT be called when downloads_url is present.
	mux.HandleFunc("/api/1/bundlekey/game/321/uploads", func(w http.ResponseWriter, r *http.Request) {
		t.Error("uploads endpoint should not be called when downloads_url is present")
		http.Error(w, "server error", http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("bundlekey", "321", "")
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

// TestFetchAuthUploads_BundleKeyOnly verifies that when owned-keys returns only a
// numeric bundle key ID (no downloads_url, no key string), FetchAuthUploads returns
// a user-facing error explaining that bundle downloads are not supported.
func TestFetchAuthUploads_BundleKeyOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Bundle purchase: numeric ID only, no downloads_url, no key string.
		w.Write([]byte(`{"owned_keys":[{"id":42}]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	_, _, err := c.FetchAuthUploads("testkey", "777", "")
	if err == nil {
		t.Fatal("expected error for bundle-key-only purchase, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "bundle") {
		t.Errorf("error should mention bundle, got: %v", err)
	}
}

// TestFetchAuthUploads_NoOwnedKeys verifies that when owned-keys returns an empty
// array, FetchAuthUploads returns an error indicating the game is not owned.
func TestFetchAuthUploads_NoOwnedKeys(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"owned_keys":[]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	_, _, err := c.FetchAuthUploads("testkey", "999", "")
	if err == nil {
		t.Fatal("expected error for game with no owned keys, got nil")
	}
	if !strings.Contains(err.Error(), "not owned") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention ownership/invalid key, got: %v", err)
	}
}

// TestFetchAuthUploads_SignedPageResolverURL verifies that the signed-page path
// builds a correct resolver URL embedding the download key and CSRF token.
func TestFetchAuthUploads_SignedPageResolverURL(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"owned_keys": []map[string]interface{}{
				{"id": 0, "downloads_url": srvURL + "/game/download/MYKEY"},
			},
		})
	})
	mux.HandleFunc("/game/download/MYKEY", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="CSRF1"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column"><div class="upload_name"><strong class="name" title="rom.gbc">rom.gbc</strong></div></div>
    <div class="actions"><a class="button download_btn" data-upload_id="9001">Download</a></div>
  </div>
</div></body></html>`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("myapikey", "555", "")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "" {
		t.Errorf("keyID should be empty for signed-page path, got %q", keyID)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	u := uploads[0]
	if u.Filename != "rom.gbc" {
		t.Errorf("Filename = %q, want rom.gbc", u.Filename)
	}
	if u.UploadID != "9001" {
		t.Errorf("UploadID = %q, want 9001", u.UploadID)
	}
	if !strings.Contains(u.URL, "key=MYKEY") {
		t.Errorf("resolver URL should contain key=MYKEY, got %q", u.URL)
	}
	if !strings.Contains(u.URL, "csrf=CSRF1") {
		t.Errorf("resolver URL should contain csrf=CSRF1, got %q", u.URL)
	}
}

// TestFetchAuthUploads_BundleKeyOnlyNoGameURL verifies that a bundle-key-only
// purchase without a gameURL also returns the bundle error (path 2b requires gameURL).
func TestFetchAuthUploads_BundleKeyOnlyNoGameURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// key string present but no gameURL supplied → must NOT try the signed page.
		w.Write([]byte(`{"owned_keys":[{"id":77,"key":"SOMEKEY"}]}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	// gameURL intentionally empty — path 2b should be skipped, hitting bundle error.
	_, _, err := c.FetchAuthUploads("testkey", "888", "")
	if err == nil {
		t.Fatal("expected error when key string present but gameURL empty, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "bundle") {
		t.Errorf("error should mention bundle, got: %v", err)
	}
}

// TestFetchAuthUploads verifies the primary success path: owned-keys returns a
// downloads_url, which is used to fetch the signed download page and build resolver URLs.
func TestFetchAuthUploads(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()

	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mykey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"owned_keys": []map[string]interface{}{
				{"id": 0, "downloads_url": srvURL + "/game/download/DLTOKEN"},
			},
		})
	})

	mux.HandleFunc("/game/download/DLTOKEN", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="csrf_token" value="CSRF99"/></head><body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column"><div class="upload_name"><strong class="name" title="game.gbc">game.gbc</strong></div></div>
    <div class="actions"><a class="button download_btn" data-upload_id="777">Download</a></div>
  </div>
</div></body></html>`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	uploads, keyID, err := c.FetchAuthUploads("mykey", "12345", "")
	if err != nil {
		t.Fatalf("FetchAuthUploads: %v", err)
	}
	if keyID != "" {
		t.Errorf("keyID should be empty for signed-page path, got %q", keyID)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].Filename != "game.gbc" || uploads[0].UploadID != "777" {
		t.Errorf("unexpected upload: %+v", uploads[0])
	}
	if uploads[0].URL == "" {
		t.Error("resolver URL should be set")
	}
}
