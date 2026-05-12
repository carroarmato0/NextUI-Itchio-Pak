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
// Falls back to full download when Range is not supported, ContentLength is unknown, or Range reading fails.
func InspectRemoteZIP(client *http.Client, cdnURL string) (ZIPManifest, error) {
	headResp, err := client.Head(cdnURL)
	if err != nil {
		return inspectViaFullDownload(client, cdnURL)
	}
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		logger.Debug("zip-inspect: HEAD returned %d, using fallback", headResp.StatusCode)
		return inspectViaFullDownload(client, cdnURL)
	}

	size := headResp.ContentLength
	supportsRange := strings.EqualFold(headResp.Header.Get("Accept-Ranges"), "bytes")

	if size > 0 && supportsRange {
		logger.Debug("zip-inspect: using range path url=%s size=%d", cdnURL, size)
		m, err := inspectViaRange(client, cdnURL, size)
		if err == nil {
			return m, nil
		}
	} else {
		logger.Debug("zip-inspect: using full-download path url=%s", cdnURL)
	}
	return inspectViaFullDownload(client, cdnURL)
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
