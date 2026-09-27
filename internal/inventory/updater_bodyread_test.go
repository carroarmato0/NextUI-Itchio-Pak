package inventory_test

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// syncBuffer is a log sink the service goroutine can write to safely.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	var b syncBuffer
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &b
}

// truncatingListener answers every request with headers promising 1000 bytes,
// sends a few, then closes: the connection drops while data.json is read. The
// transport saw a response, so only the updater can tell netstate.
func truncatingListener(t *testing.T) string {
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
				bufio.NewReader(c).ReadString('\n')
				fmt.Fprint(c, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{\"id\":")
			}(c)
		}
	}()
	return ln.Addr().String()
}

func runOneCheck(t *testing.T, addr string) {
	t.Helper()
	dir := t.TempDir()
	inv := newInv()
	rom := filepath.Join(dir, "a.gb")
	os.WriteFile(rom, []byte("ROM"), 0644)
	inv.Add("http://"+addr+"/a", inventory.Entry{Title: "a", IsFree: true},
		inventory.DownloadedFile{Filename: "a.gb", DestPath: rom, DownloadedAt: time.Now()})
	client := itchio.NewClientWithBaseAndButler("http://"+addr, "http://"+addr)
	svc := inventory.NewUpdateService(inv, filepath.Join(dir, "inventory.json"), client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()
}

// abandonLine is the updater's "abandoning this check" log line.
func abandonLine(t *testing.T, logs string) string {
	t.Helper()
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "abandoning this check") {
			return line
		}
	}
	t.Fatalf("no abandon line logged:\n%s", logs)
	return ""
}

// A body-read failure abandons the run and must take netstate offline, or no
// reconnect ever runs the owed check.
func TestUpdateService_bodyReadFailureGoesOffline(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	logs := captureLog(t)
	runOneCheck(t, truncatingListener(t))

	if !netstate.Offline() {
		t.Fatal("netstate not offline after data.json was cut off mid-body")
	}
	if line := abandonLine(t, logs.String()); strings.Contains(line, "http://") {
		t.Errorf("abandon line logs the URL: %s", line)
	}
}

// A failed request's error quotes the data.json URL; the log line must not.
func TestUpdateService_abandonLogHasNoURL(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	logs := captureLog(t)
	addr, _ := resetListener(t)
	runOneCheck(t, addr)

	if line := abandonLine(t, logs.String()); strings.Contains(line, "http://") {
		t.Errorf("abandon line logs the URL: %s", line)
	}
}

// A data.json that answers 200 with an empty body decodes to io.EOF. The
// network worked: the check must not be abandoned, and the app must not go
// offline, or the same game would end every future run.
func TestUpdateService_emptyDataJSONDoesNotAbandon(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	logs := captureLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // empty body for every request
	}))
	defer srv.Close()
	runOneCheck(t, strings.TrimPrefix(srv.URL, "http://"))

	if netstate.Offline() {
		t.Fatal("netstate offline after an empty 200")
	}
	if strings.Contains(logs.String(), "abandoning this check") {
		t.Fatalf("check abandoned on an empty 200:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "transient error") {
		t.Fatalf("empty data.json not treated as a per-game transient error:\n%s", logs.String())
	}
}
