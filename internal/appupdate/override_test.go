package appupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOverrideBase(t *testing.T) {
	dir := t.TempDir()
	if _, ok := OverrideBase(dir); ok {
		t.Fatal("no file, no override")
	}
	os.WriteFile(filepath.Join(dir, overrideFile), []byte("http://127.0.0.1:8765\n# comment\n"), 0644)
	if b, ok := OverrideBase(dir); !ok || b != "http://127.0.0.1:8765/" {
		t.Fatalf("OverrideBase = %q %v", b, ok)
	}
	os.WriteFile(filepath.Join(dir, overrideFile), []byte("ftp://nope\n"), 0644)
	if _, ok := OverrideBase(dir); ok {
		t.Fatal("only http and https are accepted")
	}
}

func TestNewOverrideSource_readsTheFixture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ghRelease{{Tag: "v1.1.0-rc6", Prerelease: true,
			Assets: []ghAsset{{Name: NextUIAssetName("v1.1.0-rc6"), URL: "http://x/z", Size: 5, Digest: "sha256:aa"}}}})
	})
	mux.HandleFunc("/pak.json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"version":"v1.1.0-rc6"}`)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s := NewOverrideSource("test", srv.URL+"/")
	res, err := s.Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC == nil || !res.RC.HasNextUIAsset() {
		t.Fatalf("RC = %+v", res.RC)
	}
}
