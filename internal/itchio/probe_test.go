package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func TestProbe_headsTheBaseURL(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
	}))
	defer srv.Close()
	if err := itchio.NewClientWithBase(srv.URL).Probe(); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodHead || path != "/" {
		t.Fatalf("probe sent %s %s, want HEAD /", method, path)
	}
	if netstate.Current().Status != netstate.StatusOnline {
		t.Fatal("the client's transport did not report to netstate")
	}
}

func TestFetchGames_serverErrorIsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := itchio.NewClientWithBase(srv.URL).FetchGames(1)
	m, ok := netstate.Describe(err, "itch.io")
	if !ok || m.Title != "itch.io is having problems" {
		t.Fatalf("Describe(%v) = %+v, %v", err, m, ok)
	}
}
