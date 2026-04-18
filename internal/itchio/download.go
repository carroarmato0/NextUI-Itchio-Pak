package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) DownloadFree(gameURL string, upload Upload, dest string, progress func(int64, int64)) error {
	// Step 1: get game page for csrf token
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return fmt.Errorf("fetch game page: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read game page: %w", err)
	}

	csrfM := csrfRegex.FindStringSubmatch(string(body))
	if len(csrfM) < 2 {
		return fmt.Errorf("csrf_token not found on game page")
	}
	csrf := csrfM[1]

	// Step 2: POST to get signed download page URL
	postURL := strings.TrimRight(gameURL, "/") + "/download_url"
	form := url.Values{"csrf_token": {csrf}, "suggested_amount": {"0"}}
	postResp, err := c.http.Post(postURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("download_url POST: %w", err)
	}
	defer postResp.Body.Close()

	var dlResult struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(postResp.Body).Decode(&dlResult); err != nil {
		return fmt.Errorf("parse download_url response: %w", err)
	}
	if dlResult.URL == "" {
		return fmt.Errorf("download_url response had empty url (game may be paid)")
	}

	// Step 3+4: stream the upload file directly using the upload URL
	// (the download page listing is an intermediate step; for free downloads
	// we have the upload URL already from FetchGameDetail, so stream directly)
	return c.streamToFile(upload.URL, dest, progress)
}

func (c *Client) streamToFile(srcURL, dest string, progress func(int64, int64)) error {
	resp, err := c.http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file download status %d", resp.StatusCode)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
	}
	return nil
}
