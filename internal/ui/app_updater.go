//go:build !headless

package ui

import (
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
)

// AppUpdater is satisfied by *appupdate.Checker; an interface so offscreen
// scenes and tests can use a stand-in.
type AppUpdater interface {
	// Enabled is false for a dev or unparseable build, or a firmware that
	// never checks: Settings → App updates then says so instead of waiting.
	Enabled() bool
	Verdict() appupdate.Verdict
	Channel() appupdate.Channel
	SetChannel(appupdate.Channel)
	CheckNow()
	IsRunning() bool
	CheckedAt() time.Time
	LastError() error
	RateLimitedUntil() time.Time
	PendingNotice() (appupdate.Verdict, bool)
	MarkNotified(ch appupdate.Channel, tag string)
	StartArchiveSave()
	CancelArchiveSave()
	ArchiveStatus() appupdate.ArchiveStatus
	StartInstall()
	CancelInstall()
	InstallStatus() appupdate.InstallStatus
	RequestRestart()
	// SourceOverride is the test update source in use, "" normally.
	SourceOverride() string
}

var (
	appUpdMu sync.RWMutex
	appUpd   AppUpdater
)

// SetAppUpdater installs the app-update checker. main_sdl.go calls it once at
// startup. Unset (the host, tests) hides Settings → App updates and the notice.
func SetAppUpdater(u AppUpdater) {
	appUpdMu.Lock()
	appUpd = u
	appUpdMu.Unlock()
}

func appUpdater() AppUpdater {
	appUpdMu.RLock()
	defer appUpdMu.RUnlock()
	return appUpd
}
