package appupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// ErrIntegrity: the download does not match the release's size or digest.
var ErrIntegrity = errors.New("download failed the integrity check")

var errIdle = errors.New("download stalled: no data for 30 s")

// SpaceError: the card has less room than the asset plus 10 %.
type SpaceError struct{ Need, Have int64 }

func (e *SpaceError) Error() string {
	return fmt.Sprintf("not enough space: need %d bytes, %d free", e.Need, e.Have)
}

// idleTimeout also covers the wait for response headers. A variable so
// tests can shorten it.
var idleTimeout = 30 * time.Second

// Archive Manager's SAFE_ARCHIVE rejects oversized archives; stay well
// inside what a real .muxapp (≈13 MB, a few hundred entries) needs.
const (
	maxEntries   = 10000
	maxEntrySize = 256 << 20
	maxTotalSize = 1 << 30
)

var ourArchive = regexp.MustCompile(`^Itch-io\.muOS\.v\d+\.\d+\.\d+(-rc\d+)?\.muxapp$`)

// SaveToArchive downloads rel's .muxapp into dir for muOS's Archive Manager,
// verifying size, digest and zip layout before it takes its final name. On
// any failure or cancel nothing is left behind. On success every other
// Itch-io .muxapp in dir is removed, so ARCHIVE holds exactly the one the
// user chose (spec §5).
func SaveToArchive(ctx context.Context, hc *http.Client, userAgent string, rel Release, dir string,
	progress func(done, total int64)) (string, error) {
	if dir == "" || rel.Asset == "" || rel.Size <= 0 || !strings.HasPrefix(rel.Digest, "sha256:") {
		return "", fmt.Errorf("%s has no downloadable muOS asset with a digest", rel.Tag)
	}
	name := AssetName(rel.Tag)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("appupdate: create %s: %v", dir, err)
		return "", err
	}
	need := rel.Size + rel.Size/10
	if free, ok := freeBytes(dir); ok && free < need {
		logger.Warn("appupdate: not enough space in %s: need %d, have %d", dir, need, free)
		return "", &SpaceError{Need: need, Have: free}
	}

	dest := filepath.Join(dir, name)
	pf, err := partfile.Create(dest)
	if err != nil {
		logger.Error("appupdate: create partial for %s: %v", dest, err)
		return "", err
	}
	defer pf.Abort() // no-op after Commit

	start := time.Now()
	logger.Info("appupdate: downloading %s (%d bytes) to %s", name, rel.Size, dir)
	sum, done, err := fetchAsset(ctx, hc, userAgent, rel.Asset, pf, rel.Size, progress)
	if err != nil {
		logger.Error("appupdate: download %s failed after %d bytes: %s", name, done, netstate.Detail(err))
		return "", err
	}
	if done != rel.Size {
		logger.Error("appupdate: %s is %d bytes, release says %d", name, done, rel.Size)
		return "", ErrIntegrity
	}
	want := strings.TrimPrefix(rel.Digest, "sha256:")
	if !strings.EqualFold(sum, want) {
		logger.Error("appupdate: verify mismatch for %s: got sha256:%s want sha256:%s", name, sum, want)
		return "", ErrIntegrity
	}
	logger.Info("appupdate: verify ok for %s", name)
	if err := ValidateMuxapp(partfile.PathFor(dest)); err != nil {
		logger.Error("appupdate: %s rejected: %v", name, err)
		return "", err
	}
	if err := pf.Commit(); err != nil {
		return "", err
	}
	logger.Info("appupdate: saved %s in %v", dest, time.Since(start).Round(time.Millisecond))
	CleanOtherArchives(dir, name)
	return dest, nil
}

// fetchAsset streams url into w, hashing as it goes. The idle timer is armed
// before the request, so a server that never answers is caught too.
func fetchAsset(ctx context.Context, hc *http.Client, userAgent, url string, w io.Writer, total int64,
	progress func(done, total int64)) (string, int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var idled atomic.Bool
	idle := time.AfterFunc(idleTimeout, func() { idled.Store(true); cancel() })
	defer idle.Stop()
	fail := func(err error) error {
		if idled.Load() {
			return errIdle
		}
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, &netstate.StatusError{What: "download update", Code: resp.StatusCode}
	}

	// Read at most one byte past the release's size: an oversized body then
	// fails SaveToArchive's size check instead of streaming on until the
	// card is full.
	body := io.LimitReader(resp.Body, total+1)
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var done, lastLogged int64
	for {
		idle.Reset(idleTimeout)
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return "", done, err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
			if done-lastLogged >= 1<<20 {
				logger.Debug("appupdate: downloaded %d/%d bytes", done, total)
				lastLogged = done
			}
		}
		if rerr == io.EOF {
			if done > total {
				logger.Warn("appupdate: download body runs past the release's %d bytes; stopped at %d", total, done)
			}
			break
		}
		if rerr != nil {
			return "", done, fail(rerr)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), done, nil
}

// ValidateMuxapp applies Archive Manager's SAFE_ARCHIVE rules, and ours:
// every entry under Itch-io/, and the launcher present.
func ValidateMuxapp(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a zip: %w", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > maxEntries {
		return fmt.Errorf("%d entries", len(zr.File))
	}
	var total uint64
	launcher := false
	for _, f := range zr.File {
		n := f.Name
		switch {
		case n == "" || strings.HasPrefix(n, "/") || strings.Contains(n, "\\"):
			return fmt.Errorf("unsafe entry %q", n)
		case !strings.HasPrefix(n, "Itch-io/"):
			return fmt.Errorf("entry %q is outside Itch-io/", n)
		case f.UncompressedSize64 > maxEntrySize:
			return fmt.Errorf("entry %q is too large", n)
		}
		for _, seg := range strings.Split(n, "/") {
			if seg == ".." {
				return fmt.Errorf("unsafe entry %q", n)
			}
		}
		if m := f.Mode(); m&fs.ModeSymlink != 0 || !(m.IsRegular() || m.IsDir()) {
			return fmt.Errorf("entry %q is not a plain file or folder", n)
		}
		total += f.UncompressedSize64
		if n == "Itch-io/mux_launch.sh" {
			launcher = true
		}
	}
	if total > maxTotalSize {
		return fmt.Errorf("%d bytes uncompressed", total)
	}
	if !launcher {
		return errors.New("no Itch-io/mux_launch.sh")
	}
	return nil
}

// CleanOtherArchives removes every Itch-io .muxapp in dir except keep. Only
// names matching our release naming are touched.
func CleanOtherArchives(dir, keep string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("appupdate: list %s: %v", dir, err)
		return nil
	}
	var removed []string
	for _, e := range entries {
		n := e.Name()
		if n == keep || e.IsDir() || !ourArchive.MatchString(n) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			logger.Warn("appupdate: remove old %s: %v", n, err)
			continue
		}
		logger.Info("appupdate: removed %s from ARCHIVE", n)
		removed = append(removed, n)
	}
	return removed
}
