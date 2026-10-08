package inventory_test

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// resetListener accepts connections and resets them at once, counting them:
// every request through it fails with ECONNRESET. (A plain Close would send a
// FIN, which the client sees as io.EOF with no response — also offline — but
// a reset is the unambiguous case.)
func resetListener(t *testing.T) (addr string, accepts *int32) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var n int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&n, 1)
			if tc, ok := c.(*net.TCPConn); ok {
				tc.SetLinger(0) // Close sends RST instead of FIN
			}
			c.Close()
		}
	}()
	return ln.Addr().String(), &n
}

func TestUpdateService_abortsOnFirstOfflineFailure(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	addr, accepts := resetListener(t)
	dir := t.TempDir()
	inv := newInv()
	for _, name := range []string{"a", "b", "c"} {
		rom := filepath.Join(dir, name+".gb")
		os.WriteFile(rom, []byte("ROM"), 0644)
		inv.Add("http://"+addr+"/"+name, inventory.Entry{Title: name, IsFree: true},
			inventory.DownloadedFile{Filename: name + ".gb", DestPath: rom, DownloadedAt: time.Now()})
	}
	before := inv.LatestCheckedAt()

	client := itchio.NewClientWithBaseAndButler("http://"+addr, "http://"+addr)
	svc := inventory.NewUpdateService(inv, filepath.Join(dir, "inventory.json"), client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()

	// data.json for the first game fails; nothing else should be attempted.
	// The HTTP client may retry a reset connection once, so allow up to 2.
	if n := atomic.LoadInt32(accepts); n > 2 {
		t.Fatalf("%d connections made; the run should stop after the first failure", n)
	}
	if !inv.LatestCheckedAt().Equal(before) {
		t.Fatal("checked_at changed during an offline run")
	}
	if !netstate.Offline() {
		t.Fatal("netstate not offline after the failures")
	}
}
