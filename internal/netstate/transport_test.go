package netstate

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransport_feedsState(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &http.Client{Transport: Transport(http.DefaultTransport)}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	ln.Close()
	if _, err := c.Get("http://" + closed); err == nil {
		t.Fatal("expected an error from a closed port")
	}
	if !Offline() {
		t.Fatalf("after refused: %+v", Current())
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if Current().Status != StatusOnline {
		t.Fatalf("a 503 response should mean Online, got %+v", Current())
	}
}
