package itchio_test

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// truncatingServer answers every request with a response that promises
// `promise` bytes, sends `send`, then closes the socket: Wi-Fi dropping
// mid-download, at the TCP level, with no net/http server logic in the way.
func truncatingServer(t *testing.T, promise int, send []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				bufio.NewReader(c).ReadString('\n') // request line; the rest is ignored
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", promise)
				c.Write(send)
			}(c)
		}
	}()
	return "http://" + ln.Addr().String() + "/f.gb"
}

func TestStreamToFile_failureKeepsExistingFile(t *testing.T) {
	defer netstate.ResetForTest()
	srvURL := truncatingServer(t, 1000, []byte("NEWDATA"))
	dest := filepath.Join(t.TempDir(), "game.gb")
	os.WriteFile(dest, []byte("WORKING ROM"), 0644)

	err := itchio.NewClient().DownloadURL(srvURL, dest, nil)
	if err == nil {
		t.Fatal("expected an error for a truncated download")
	}
	if b, _ := os.ReadFile(dest); string(b) != "WORKING ROM" {
		t.Fatalf("installed ROM changed to %q", b)
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind")
	}
}

func TestStreamToFile_shortBodyRejected(t *testing.T) {
	defer netstate.ResetForTest()
	// Content-Length says 10, the handler writes 4 and returns cleanly: net/http
	// reports this as unexpected EOF; either way it must not be committed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.Write([]byte("abcd"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "g.gb")
	if err := itchio.NewClient().DownloadURL(srv.URL, dest, nil); err == nil {
		t.Fatal("short body accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("short download was committed")
	}
}

func TestStreamToFile_idleTimeout(t *testing.T) {
	defer netstate.ResetForTest()
	defer itchio.SetStreamIdleTimeoutForTest(100 * time.Millisecond)()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	dest := filepath.Join(t.TempDir(), "g.gb")
	start := time.Now()
	err := itchio.NewClient().DownloadURL(srv.URL, dest, nil)
	if err == nil {
		t.Fatal("stalled download succeeded")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("idle timeout did not fire (took %v)", time.Since(start))
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("dest left behind after idle timeout")
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind after idle timeout")
	}
}

// TestStreamToFile_idleTimeoutBeforeHeaders covers a stall that happens
// before the server ever writes a response: the handler blocks first, so
// nothing but the idle timer arming before dlClient.Do can end this. It must
// still fail well within the test timeout, must classify as an idle/network
// timeout rather than a plain cancellation, and must leave neither dest nor
// the partial file behind (the partfile.Create call never even happens).
func TestStreamToFile_idleTimeoutBeforeHeaders(t *testing.T) {
	defer netstate.ResetForTest()
	defer itchio.SetStreamIdleTimeoutForTest(100 * time.Millisecond)()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	dest := filepath.Join(t.TempDir(), "g.gb")
	start := time.Now()
	err := itchio.NewClient().DownloadURL(srv.URL, dest, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("stall before headers succeeded")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("idle timeout did not fire before headers (took %v)", elapsed)
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) && netstate.Classify(err) != netstate.ReasonUnreachable {
		t.Fatalf("err = %v, want it to wrap os.ErrDeadlineExceeded or classify Unreachable", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("dest left behind after a stall before headers")
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind after a stall before headers")
	}
}

func TestStreamToFile_successReplacesFile(t *testing.T) {
	defer netstate.ResetForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NEW ROM"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "g.gb")
	os.WriteFile(dest, []byte("OLD"), 0644)
	if err := itchio.NewClient().DownloadURL(srv.URL, dest, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW ROM" {
		t.Fatalf("dest = %q", b)
	}
}

// The rate limiter holding a download back is not the connection going
// silent: a 429 cooldown longer than the idle timeout must not abort the
// download as "no data", nor take the app offline.
func TestStreamToFile_rateLimitCooldownIsNotIdle(t *testing.T) {
	defer netstate.ResetForTest()
	defer itchio.SetStreamIdleTimeoutForTest(200 * time.Millisecond)()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1") // the shortest cooldown the limiter honours
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("NEW ROM"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "g.gb")
	if err := itchio.NewClient().DownloadURL(srv.URL, dest, nil); err != nil {
		t.Fatalf("download failed across a 1s cooldown with a 200ms idle timeout: %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW ROM" {
		t.Fatalf("dest = %q", b)
	}
	if netstate.Offline() {
		t.Fatal("netstate offline after waiting out a rate-limit cooldown")
	}
}
