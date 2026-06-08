package roms

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// rangePrefetchSize is how many bytes are fetched per HTTP Range request.
// zip.NewReader makes many tiny reads (30–100 bytes) per central directory
// entry. Without prefetching, a ZIP with 500 entries would need 1000+
// HTTP round trips. A 128 KB prefetch amortises that to ~10 requests.
const rangePrefetchSize = int64(128 * 1024)

// rangeReaderAt implements io.ReaderAt using HTTP Range requests with an
// aggressive prefetch cache to reduce round trips to O(totalSize/prefetch).
type rangeReaderAt struct {
	url        string
	client     *http.Client
	size       int64
	mu         sync.Mutex
	chunks     []rangeChunk
	onProgress func(fetched, totalFile int64) // called after each HTTP fetch; nil = no-op
}

type rangeChunk struct {
	start int64
	data  []byte
}

func (r *rangeReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	end := off + int64(len(p)) - 1
	if end >= r.size {
		end = r.size - 1
	}
	need := end - off + 1
	if need <= 0 {
		return 0, io.EOF
	}

	// Serve from cache when available.
	for _, chunk := range r.chunks {
		chunkEnd := chunk.start + int64(len(chunk.data)) - 1
		if off >= chunk.start && end <= chunkEnd {
			src := chunk.data[off-chunk.start : off-chunk.start+need]
			n := copy(p, src)
			if int64(n) < need {
				return n, io.EOF
			}
			return n, nil
		}
	}

	// Cache miss — fetch a prefetch-sized window starting at off so that
	// subsequent sequential reads (central directory entries) are all served
	// from cache, reducing HTTP round trips by ~100×.
	fetchEnd := off + rangePrefetchSize - 1
	if fetchEnd >= r.size {
		fetchEnd = r.size - 1
	}
	logger.Debug("zip-inspect: range fetch bytes=%d-%d (%d KB)", off, fetchEnd, (fetchEnd-off+1)/1024)

	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, fetchEnd))
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("range: server returned %d (expected 206)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	r.chunks = append(r.chunks, rangeChunk{start: off, data: data})
	// Keep only the 4 most recent chunks (≤ 4×128 KB = 512 KB peak) so the
	// reader doesn't accumulate memory across the full inspection.
	if len(r.chunks) > 4 {
		r.chunks = r.chunks[len(r.chunks)-4:]
	}

	if r.onProgress != nil {
		// Sum bytes held across all current chunks as a proxy for total fetched.
		var total int64
		for _, c := range r.chunks {
			total += int64(len(c.data))
		}
		r.onProgress(total, r.size)
	}

	n := copy(p, data[:need])
	if int64(n) < need {
		return n, io.EOF
	}
	return n, nil
}

// InspectRemoteZIP reads the ZIP central directory via HTTP Range requests.
// Falls back to a full download only when the server does not support Range at
// all. The probe sequence is:
//  1. HEAD — cheapest; gives size + Accept-Ranges in one shot.
//  2. Range GET probe (bytes=-1) — used when HEAD fails or is blocked (e.g.
//     403); the 206 Content-Range header reveals the total file size.
//  3. Full download — last resort for servers that reject Range entirely.
//
// onProgress is called after each HTTP fetch with (bytesRead, totalFileSize).
// Pass nil to omit progress reporting.
func InspectRemoteZIP(client *http.Client, cdnURL string, onProgress func(fetched, total int64)) (ZIPManifest, error) {
	size, ok := probeSizeAndRange(client, cdnURL)
	if ok {
		m, err := inspectViaRange(client, cdnURL, size, onProgress)
		if err == nil {
			return m, nil
		}
		logger.Debug("zip-inspect: range path failed (%v), falling back to full download", err)
	}
	return inspectViaFullDownload(client, cdnURL)
}

// probeSizeAndRange returns the total byte size of the remote file and true
// when the server supports Range requests. It tries HEAD first; if that fails
// or does not advertise Accept-Ranges, it probes with a Range GET.
func probeSizeAndRange(client *http.Client, url string) (int64, bool) {
	if resp, err := client.Head(url); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && resp.ContentLength > 0 &&
			strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") {
			logger.Debug("zip-inspect: HEAD ok size=%d range=yes", resp.ContentLength)
			return resp.ContentLength, true
		}
		logger.Debug("zip-inspect: HEAD returned status=%d cl=%d accept-ranges=%q — trying range probe",
			resp.StatusCode, resp.ContentLength, resp.Header.Get("Accept-Ranges"))
	}

	// Range GET probe: request the last 1 byte. A 206 response includes a
	// Content-Range header that reveals the total file size.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Range", "bytes=-1")
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 8192)) //nolint:errcheck
	resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		logger.Debug("zip-inspect: range probe returned %d — no range support, will full-download", resp.StatusCode)
		return 0, false
	}
	total := parseContentRangeTotal(resp.Header.Get("Content-Range"))
	if total <= 0 {
		return 0, false
	}
	logger.Debug("zip-inspect: range probe ok total=%d", total)
	return total, true
}

// parseContentRangeTotal extracts the total size from a Content-Range header
// value of the form "bytes start-end/total".
func parseContentRangeTotal(cr string) int64 {
	idx := strings.LastIndex(cr, "/")
	if idx < 0 || idx == len(cr)-1 {
		return 0
	}
	var total int64
	if _, err := fmt.Sscanf(cr[idx+1:], "%d", &total); err != nil {
		return 0
	}
	return total
}

func inspectViaRange(client *http.Client, cdnURL string, size int64, onProgress func(int64, int64)) (ZIPManifest, error) {
	rra := &rangeReaderAt{url: cdnURL, client: client, size: size, onProgress: onProgress}
	r, err := zip.NewReader(rra, size)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("zip.NewReader: %w", err)
	}
	return manifestFromZipReader(r), nil
}

func inspectViaFullDownload(client *http.Client, cdnURL string) (ZIPManifest, error) {
	tmp, err := os.CreateTemp("", "itchio-inspect-*.zip")
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	logger.Debug("zip-inspect: full-download fallback, temp=%s", tmpPath)

	resp, err := client.Get(cdnURL)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("full download: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("open temp: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return ZIPManifest{}, fmt.Errorf("write temp: %w", err)
	}
	f.Close()

	r, err := zip.OpenReader(tmpPath)
	if err != nil {
		return ZIPManifest{}, fmt.Errorf("zip.OpenReader: %w", err)
	}
	defer r.Close()
	return manifestFromZipReader(&r.Reader), nil
}

func manifestFromZipReader(r *zip.Reader) ZIPManifest {
	var m ZIPManifest
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// __MACOSX is a macOS-specific directory that ZIP archives created on
		// macOS include automatically. It contains resource fork metadata that
		// is not meaningful outside macOS; skip it entirely.
		if IsInMacOSMetaDir(f.Name) {
			continue
		}
		name := filepath.Base(f.Name)
		kind := ClassifyEntry(name)

		// For entries the extension-based classifier cannot identify, read the
		// file header and attempt magic-byte detection. This handles uploads
		// whose filenames carry no extension or a version-number suffix (e.g.
		// "soulbound_v1_0" → detected as .p8 from the pico-8 text header).
		// Skip magic detection for known image extensions: a .png is always
		// artwork even if it is 128 px wide (e.g. raspi/linux Pico-8 exports).
		if kind == KindOther && !isImageExt(strings.ToLower(filepath.Ext(name))) {
			if detected := classifyByMagic(f); detected != "" {
				stem := strings.TrimSuffix(name, filepath.Ext(name))
				name = stem + detected
				kind = KindROM
			}
		}

		m.Entries = append(m.Entries, ZIPEntry{
			Name: name,
			Kind: kind,
			Size: f.UncompressedSize64,
		})
	}
	return m
}

// isImageExt reports whether ext is a common image format extension.
// Files with these extensions are always artwork and must never be promoted
// to a ROM via magic-byte detection (e.g. a 128-px-wide PNG is a Pico-8
// cart only when named .p8.png, not when named .png).
func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".svg", ".ico", ".tif", ".tiff":
		return true
	}
	return false
}

// IsInMacOSMetaDir reports whether an archive entry path is inside the
// __MACOSX metadata directory that macOS automatically injects into ZIP and
// 7z archives. Backslashes are normalised before the check.
func IsInMacOSMetaDir(name string) bool {
	name = filepath.ToSlash(strings.ReplaceAll(name, "\\", "/"))
	return strings.HasPrefix(name, "__MACOSX/") || strings.Contains(name, "/__MACOSX/")
}

// classifyByMagic opens a ZIP entry, reads the first DetectBufSize uncompressed
// bytes, and returns the detected ROM extension. Returns "" on any error or when
// no signature matches. Works for both local and remote ZIPs (remote reads
// trigger HTTP Range requests via the underlying ReaderAt).
func classifyByMagic(f *zip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	buf := make([]byte, DetectBufSize)
	n, _ := io.ReadFull(rc, buf)
	return DetectROMExt(buf[:n])
}
