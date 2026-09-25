package itchio

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestScanOwnedKeys_gameIDsCommaListAndLimit(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("game_ids")
		w.Write([]byte(`{"owned_keys":[],"per_page":50}`))
	}))
	defer srv.Close()
	c := NewClientWithBaseAndButler(srv.URL, srv.URL)

	if _, err := c.scanOwnedKeys("k", []string{"1", "22", "333"}); err != nil {
		t.Fatal(err)
	}
	if got != "1,22,333" {
		t.Errorf("game_ids = %q, want 1,22,333", got)
	}

	if _, err := c.scanOwnedKeys("k", nil); err != nil || got != "" {
		t.Errorf("full scan sent game_ids=%q (err %v), want none", got, err)
	}

	many := make([]string, maxOwnedKeysGameIDs+1)
	for i := range many {
		many[i] = strconv.Itoa(i + 1)
	}
	if _, err := c.scanOwnedKeys("k", many); err == nil {
		t.Errorf("%d game IDs accepted, want an error above %d", len(many), maxOwnedKeysGameIDs)
	}
}
