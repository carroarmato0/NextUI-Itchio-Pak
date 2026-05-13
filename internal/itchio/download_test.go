package itchio_test

import (
	"encoding/json"
	"fmt"
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
	if err := c.DownloadFree(upload, dest, nil); err != nil {
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

// ownedKeysPage builds the JSON response body for a page of owned keys.
func ownedKeysPage(perPage int, keys []map[string]interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"per_page":   perPage,
		"owned_keys": keys,
	})
	return b
}

// TestFetchOwnedKeys_SinglePurchase verifies that a single matching key is
// returned when owned-keys contains exactly one entry for the target game.
func TestFetchOwnedKeys_SinglePurchase(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mykey" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(ownedKeysPage(50, []map[string]interface{}{
			{
				"id": 111, "game_id": 999, "purchase_id": 5001,
				"downloads": 2, "created_at": "2026-01-15T10:00:00.000000000Z",
			},
		}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	keys, err := c.FetchOwnedKeys("mykey", "999")
	if err != nil {
		t.Fatalf("FetchOwnedKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].ID != 111 {
		t.Errorf("ID = %d, want 111", keys[0].ID)
	}
	if keys[0].PurchaseID != 5001 {
		t.Errorf("PurchaseID = %d, want 5001", keys[0].PurchaseID)
	}
	if keys[0].Downloads != 2 {
		t.Errorf("Downloads = %d, want 2", keys[0].Downloads)
	}
	if keys[0].BundleSize != 1 {
		t.Errorf("BundleSize = %d, want 1 (single game in purchase)", keys[0].BundleSize)
	}
}

// TestFetchOwnedKeys_MultiPurchase verifies that multiple keys are returned
// when a game has been purchased more than once (e.g. individually + bundle),
// and that BundleSize correctly identifies each purchase type.
func TestFetchOwnedKeys_MultiPurchase(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// purchase_id=100: only game_id=42 → individual (BundleSize=1)
		// purchase_id=200: game_id=42 AND game_id=99 → bundle (BundleSize=2)
		w.Write(ownedKeysPage(50, []map[string]interface{}{
			{"id": 10, "game_id": 42, "purchase_id": 100, "downloads": 1, "created_at": "2026-01-01T00:00:00.000000000Z"},
			{"id": 20, "game_id": 42, "purchase_id": 200, "downloads": 0, "created_at": "2026-03-01T00:00:00.000000000Z"},
			{"id": 30, "game_id": 99, "purchase_id": 200, "downloads": 0, "created_at": "2026-03-01T00:00:00.000000000Z"},
		}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	keys, err := c.FetchOwnedKeys("key", "42")
	if err != nil {
		t.Fatalf("FetchOwnedKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys for game_id=42, got %d", len(keys))
	}
	if keys[0].ID != 10 || keys[1].ID != 20 {
		t.Errorf("unexpected key IDs: %d, %d", keys[0].ID, keys[1].ID)
	}
	// Individual purchase: purchase_id=100 appears for 1 game only.
	if keys[0].BundleSize != 1 {
		t.Errorf("key id=10: BundleSize = %d, want 1 (individual)", keys[0].BundleSize)
	}
	// Bundle purchase: purchase_id=200 appears for 2 games.
	if keys[1].BundleSize != 2 {
		t.Errorf("key id=20: BundleSize = %d, want 2 (bundle)", keys[1].BundleSize)
	}
}

// TestFetchOwnedKeys_NotOwned verifies that an error is returned when no key
// matches the requested game_id.
func TestFetchOwnedKeys_NotOwned(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Only contains a different game.
		w.Write(ownedKeysPage(50, []map[string]interface{}{
			{"id": 77, "game_id": 9999, "purchase_id": 1, "downloads": 0, "created_at": "2026-01-01T00:00:00.000000000Z"},
		}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	_, err := c.FetchOwnedKeys("key", "1234")
	if err == nil {
		t.Fatal("expected error when game not owned, got nil")
	}
	if !strings.Contains(err.Error(), "not owned") {
		t.Errorf("error should mention 'not owned', got: %v", err)
	}
}

// TestFetchOwnedKeys_LastPageEmptyObject verifies that FetchOwnedKeys handles
// the itch.io quirk where the last page returns {"owned_keys":{}} (an object)
// instead of an empty array.  Without the RawMessage guard the JSON decoder
// returns an UnmarshalTypeError and the function discards all keys collected
// from earlier pages.
func TestFetchOwnedKeys_LastPageEmptyObject(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if page == "1" {
			w.Write(ownedKeysPage(50, []map[string]interface{}{
				{"id": 42, "game_id": 1206111, "purchase_id": 9, "downloads": 2, "created_at": "2026-04-24T19:31:40.000000000Z"},
			}))
		} else {
			// Real itch.io response when there are no more keys.
			fmt.Fprint(w, `{"page":2,"per_page":50,"owned_keys":{}}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	keys, err := c.FetchOwnedKeys("key", "1206111")
	if err != nil {
		t.Fatalf("FetchOwnedKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != 42 {
		t.Errorf("expected key id=42, got %v", keys)
	}
}

// TestFetchOwnedKeys_Pagination verifies that FetchOwnedKeys pages through
// multiple pages to find a key that only appears on page 2.
func TestFetchOwnedKeys_Pagination(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		if page == "1" {
			// Full page of unrelated games — signals that there is a next page.
			keys := make([]map[string]interface{}, 50)
			for i := range keys {
				keys[i] = map[string]interface{}{
					"id": i + 1000, "game_id": i + 1, "purchase_id": i + 100,
					"downloads": 0, "created_at": "2026-01-01T00:00:00.000000000Z",
				}
			}
			w.Write(ownedKeysPage(50, keys))
		} else {
			// Page 2: smaller-than-perPage slice containing the target game.
			w.Write(ownedKeysPage(50, []map[string]interface{}{
				{"id": 555, "game_id": 7777, "purchase_id": 9, "downloads": 0, "created_at": "2026-04-01T00:00:00.000000000Z"},
			}))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	keys, err := c.FetchOwnedKeys("key", "7777")
	if err != nil {
		t.Fatalf("FetchOwnedKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != 555 {
		t.Errorf("expected key id=555, got %v", keys)
	}
	if calls != 2 {
		t.Errorf("expected 2 pages fetched, got %d", calls)
	}
}

// TestFetchUploadsForKey_ROM verifies that .gbc files are included, known
// non-ROMs are skipped, and unknown extensions are returned with NeedsFormat=true.
func TestFetchUploadsForKey_ROM(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/1/mykey/game/123/uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download_key_id") != "456" {
			http.Error(w, "bad download_key_id", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uploads": []map[string]interface{}{
				{"id": 1, "filename": "game.gbc"},
				{"id": 2, "filename": "manual.pdf"},  // skipped
				{"id": 3, "filename": "game.gb"},
				{"id": 4, "filename": "patch.ips"},   // NeedsFormat=true
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	uploads, err := c.FetchUploadsForKey("mykey", "123", "456")
	if err != nil {
		t.Fatalf("FetchUploadsForKey: %v", err)
	}
	if len(uploads) != 3 {
		t.Fatalf("expected 3 uploads (gbc, gb, ips), got %d", len(uploads))
	}
	byName := map[string]itchio.Upload{}
	for _, u := range uploads {
		byName[u.Filename] = u
	}
	if _, ok := byName["game.gbc"]; !ok {
		t.Error("game.gbc should be included")
	}
	if _, ok := byName["game.gb"]; !ok {
		t.Error("game.gb should be included")
	}
	if u, ok := byName["patch.ips"]; !ok || !u.NeedsFormat {
		t.Error("patch.ips should be included with NeedsFormat=true")
	}
	if _, ok := byName["manual.pdf"]; ok {
		t.Error("manual.pdf should be skipped")
	}
}

// TestFetchUploadsForKey_Empty verifies that an empty uploads list is handled
// without error (empty slice returned, no panic).
func TestFetchUploadsForKey_Empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/1/k/game/1/uploads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// itch.io returns an object instead of array when uploads list is empty.
		fmt.Fprint(w, `{"uploads":{}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	uploads, err := c.FetchUploadsForKey("k", "1", "99")
	if err != nil {
		t.Fatalf("FetchUploadsForKey: %v", err)
	}
	if len(uploads) != 0 {
		t.Errorf("expected 0 uploads for empty response, got %d", len(uploads))
	}
}

// TestFetchUploadsForKey_UploadIDPassedThrough verifies that the numeric upload
// ID from the API is preserved as a string in Upload.UploadID (needed by
// DownloadAuthUpload).
func TestFetchUploadsForKey_UploadIDPassedThrough(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/1/k/game/5/uploads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uploads": []map[string]interface{}{
				{"id": 98765, "filename": "rom.gbc"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	uploads, err := c.FetchUploadsForKey("k", "5", "1")
	if err != nil {
		t.Fatalf("FetchUploadsForKey: %v", err)
	}
	if len(uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(uploads))
	}
	if uploads[0].UploadID != "98765" {
		t.Errorf("UploadID = %q, want \"98765\"", uploads[0].UploadID)
	}
}

func TestResolveFreeURL(t *testing.T) {
	// Key encodes {"id":42,...} so extractKeyID returns "42"; csrf is passed through as-is.
	const testKey = "eyJpZCI6NDIsImV4cGlyZXMiOjk5OTk5OTk5OTl9.SIG"
	const testCSRF = "TESTCSRF"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/file/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST to resolver, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.FormValue("csrf_token") == "" {
			t.Errorf("csrf_token missing from POST form body")
		}
		if r.FormValue("download_key_id") == "" {
			t.Errorf("download_key_id missing from POST form body")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"url":"https://cdn.example.com/file.zip"}`)
	}))
	defer srv.Close()

	client := itchio.NewClientWithBase(srv.URL)
	upload := itchio.Upload{
		Filename: "game.zip",
		UploadID: "999",
		URL:      srv.URL + "/author/game/file/999?key=" + testKey + "&csrf=" + testCSRF,
	}
	cdnURL, err := client.ResolveFreeURL(upload)
	if err != nil {
		t.Fatalf("ResolveFreeURL: %v", err)
	}
	if cdnURL != "https://cdn.example.com/file.zip" {
		t.Errorf("cdnURL = %q, want %q", cdnURL, "https://cdn.example.com/file.zip")
	}
}

func TestResolveAuthURL(t *testing.T) {
	const uploadID = "555"
	const downloadKeyID = "777"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, uploadID) {
			t.Errorf("URL path %q does not contain upload ID %q", r.URL.Path, uploadID)
		}
		if !strings.Contains(r.URL.RawQuery, downloadKeyID) {
			t.Errorf("URL query %q does not contain download key ID %q", r.URL.RawQuery, downloadKeyID)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"url":"https://cdn.example.com/auth-file.zip"}`)
	}))
	defer srv.Close()

	client := itchio.NewClientWithBase(srv.URL)
	cdnURL, err := client.ResolveAuthURL("apikey", uploadID, downloadKeyID)
	if err != nil {
		t.Fatalf("ResolveAuthURL: %v", err)
	}
	if cdnURL != "https://cdn.example.com/auth-file.zip" {
		t.Errorf("cdnURL = %q, want %q", cdnURL, "https://cdn.example.com/auth-file.zip")
	}
}
