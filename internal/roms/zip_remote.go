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

// rangeReaderAt implements io.ReaderAt using HTTP Range requests with a
// chunk cache to coalesce the reads zip.NewReader makes internally.
type rangeReaderAt struct {
	url    string
	client *http.Client
	size   int64
	mu     sync.Mutex
	chunks []rangeChunk
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

	logger.Debug("zip-inspect: range request bytes=%d-%d", off, end)
	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, end))
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
	n := copy(p, data)
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
func InspectRemoteZIP(client *http.Client, cdnURL string) (ZIPManifest, error) {
	size, ok := probeSizeAndRange(client, cdnURL)
	if ok {
		m, err := inspectViaRange(client, cdnURL, size)
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

func inspectViaRange(client *http.Client, cdnURL string, size int64) (ZIPManifest, error) {
	rra := &rangeReaderAt{url: cdnURL, client: client, size: size}
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
		name := filepath.Base(f.Name)
		m.Entries = append(m.Entries, ZIPEntry{
			Name: name,
			Kind: ClassifyEntry(name),
			Size: f.UncompressedSize64,
		})
	}
	return m
}
