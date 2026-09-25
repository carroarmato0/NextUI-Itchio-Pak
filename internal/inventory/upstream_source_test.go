package inventory_test

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
)

// installed returns an inventory holding one installed game, already checked
// once from source, so later checks are not first checks.
func installed(t *testing.T, source string, files []inventory.UpstreamFile) (*inventory.Inventory, string) {
	t.Helper()
	inv, _ := inventory.Load(t.TempDir() + "/inv.json")
	const url = "https://studio.itch.io/game"
	inv.Add(url, inventory.Entry{Title: "Game", IsFree: true},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb", DownloadedAt: time.Now()})
	inv.SetUpstreamFilesFrom(url, source, files)
	return inv, url
}

// A re-upload under the same name is an update when the API says the
// file changed.
func TestUpstream_sameNameNewContentIsAnUpdate(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	if inv.HasPendingUpdates(url) {
		t.Fatal("pending right after the first check")
	}
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:bbb"}})
	if !inv.HasPendingUpdates(url) {
		t.Error("changed content under the same name not reported")
	}
}

func TestUpstream_unchangedContentIsNotAnUpdate(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	if inv.HasPendingUpdates(url) {
		t.Error("identical file reported as an update")
	}
}

// Entries checked before fingerprints existed get one silently.
func TestUpstream_firstFingerprintIsABaseline(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{{Filename: "game.gb", UploadID: "1"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	if inv.HasPendingUpdates(url) {
		t.Error("a missing fingerprint being filled in was reported as an update")
	}
}

// The page and the API list files slightly differently (the API includes
// uploads the page hides). The first check from a different source only
// records a baseline, so signing in or out never invents an update.
func TestUpstream_switchingSourceIsABaseline(t *testing.T) {
	inv, url := installed(t, inventory.SourcePage, []inventory.UpstreamFile{{Filename: "game.gb", UploadID: "1"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"},
		{Filename: "web_1.2.zip", UploadID: "2", Fingerprint: "md5:ccc"},
	})
	if inv.HasPendingUpdates(url) {
		t.Error("switching to the API reported an upload the page hid as new")
	}
	// Once on the API, a genuinely new upload is reported again.
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"},
		{Filename: "web_1.2.zip", UploadID: "2", Fingerprint: "md5:ccc"},
		{Filename: "game-v2.gb", UploadID: "3", Fingerprint: "md5:ddd"},
	})
	if !inv.HasPendingUpdates(url) {
		t.Error("a new upload after the baseline was not reported")
	}
}

// Old inventories have no source; they were written by the page flow.
func TestUpstream_legacyEntriesCountAsPage(t *testing.T) {
	inv, url := installed(t, "", []inventory.UpstreamFile{{Filename: "game.gb", UploadID: "1"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourcePage, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1"}, {Filename: "game-v2.gb", UploadID: "2"}})
	if !inv.HasPendingUpdates(url) {
		t.Error("a new upload on a legacy entry was swallowed as a baseline")
	}
}

// A file is recognised by its upload ID even when its shown name changes.
func TestUpstream_matchesByUploadID(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "Game (GB Rom)", UploadID: "1", Fingerprint: "md5:aaa"}})
	if inv.HasPendingUpdates(url) {
		t.Error("a renamed display name was reported as a new upload")
	}
}

// Downloading again gets the current version, so the update is done.
func TestUpstream_redownloadClearsChangedContent(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:bbb"}})
	inv.Add(url, inventory.Entry{Title: "Game"},
		inventory.DownloadedFile{Filename: "game.gb", DestPath: "/roms/game.gb", DownloadedAt: time.Now()})
	if inv.HasPendingUpdates(url) {
		t.Error("still pending after downloading the new version")
	}
}

// Dismissing the update hides it, as for new uploads.
func TestUpstream_dismissedChangeStaysHidden(t *testing.T) {
	inv, url := installed(t, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:aaa"}})
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:bbb"}})
	inv.DismissUpdate(url)
	inv.SetUpstreamFilesFrom(url, inventory.SourceAPI, []inventory.UpstreamFile{
		{Filename: "game.gb", UploadID: "1", Fingerprint: "md5:bbb"}})
	if inv.HasPendingUpdates(url) {
		t.Error("a dismissed update came back without a further change")
	}
}
