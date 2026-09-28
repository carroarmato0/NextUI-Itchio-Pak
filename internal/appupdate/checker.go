package appupdate

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

// Config is what the checker needs from the app.
type Config struct {
	Firmware   firmware.Kind
	Running    string // this build's version, e.g. "v1.1.0-rc3"
	Channel    Channel
	StatePath  string // update_state.json; "" keeps state in memory only
	StoreDB    string // firmware.Env.PakStoreDB()
	ArchiveDir string // firmware.Env.ArchiveDir()
	UserAgent  string // itchio.BuildUserAgent(info, false)
	Source     *Source
	Notify     func()
}

// ArchiveState is where a Save to ARCHIVE is.
type ArchiveState int

const (
	ArchiveIdle ArchiveState = iota
	ArchiveRunning
	ArchiveDone
	ArchiveFailed
)

// ArchiveStatus is a snapshot of the ARCHIVE download.
type ArchiveStatus struct {
	State       ArchiveState
	Done, Total int64
	Path        string
	Err         error
}

// offlineNow is netstate's, replaceable in tests.
var offlineNow = netstate.Offline

// Checker runs update checks in the background and answers the UI. Every
// method is safe from any goroutine and none blocks on the network.
type Checker struct {
	cfg       Config
	running   Version
	runningOK bool
	src       *Source
	dl        *http.Client

	busy atomic.Bool
	owed atomic.Bool
	// rerun is set when SetChannel wants a check while one is already
	// running: the running goroutine consumes it and runs once more for the
	// channel now in effect, instead of the request being dropped.
	rerun atomic.Bool
	wg    sync.WaitGroup // background work; tests wait on it

	mu            sync.Mutex
	st            *State
	channel       Channel
	store         pakstore.Result
	lastErr       error
	archive       ArchiveStatus
	cancelArchive context.CancelFunc
}

// NewChecker loads the saved state and reads the Store database (one small
// local file); it makes no request.
func NewChecker(cfg Config) *Checker {
	c := &Checker{cfg: cfg, channel: cfg.Channel, src: cfg.Source, dl: newClient(0)}
	c.running, c.runningOK = Parse(cfg.Running)
	if c.src == nil {
		c.src = NewSource(cfg.UserAgent)
	}
	c.st = LoadState(cfg.StatePath)
	c.store = c.lookupStore()
	logger.Info("appupdate: running=%s parsed=%v firmware=%s channel=%s store=%s",
		cfg.Running, c.runningOK, cfg.Firmware, c.channel, c.store)
	return c
}

func (c *Checker) lookupStore() pakstore.Result {
	if c.cfg.Firmware != firmware.KindNextUI {
		return pakstore.Result{Status: pakstore.NotInstalled, Reason: "not NextUI"}
	}
	return pakstore.Lookup(c.cfg.StoreDB)
}

// Enabled: checks run only on a real firmware, for a build with a release tag.
func (c *Checker) Enabled() bool {
	return c.runningOK && (c.cfg.Firmware == firmware.KindNextUI || c.cfg.Firmware == firmware.KindMuOS)
}

// managedLocked: NextUI and the Store has (or may have) a row. Caller holds mu.
func (c *Checker) managedLocked() bool {
	return c.cfg.Firmware == firmware.KindNextUI && c.store.Status != pakstore.NotInstalled
}

// Start is the once-per-launch check.
func (c *Checker) Start() { c.maybeCheck("launch") }

// CheckNow is "Check now" on the Updates screen.
func (c *Checker) CheckNow() {
	logger.Info("appupdate: check requested from Settings")
	c.maybeCheck("manual")
}

// RetryAfterReconnect is registered with netstate.OnReconnect. It runs on the
// netstate goroutine, so it only starts a goroutine, and only when owed.
func (c *Checker) RetryAfterReconnect() {
	if c.owed.Swap(false) {
		logger.Info("appupdate: connection back, running the postponed check")
		c.maybeCheck("reconnect")
	}
}

// gateOK reports whether a check is allowed to start right now: enabled, the
// channel isn't Off, we're not rate-limited, and we're online. Split out of
// maybeCheck so the mid-run rerun (see rerun below) can apply the same gates
// without re-entering the busy check, which it must not touch: the rerun
// runs inside the goroutine that already holds busy.
func (c *Checker) gateOK(why string) bool {
	if !c.Enabled() {
		logger.Debug("appupdate: %s check skipped: firmware=%s running=%q", why, c.cfg.Firmware, c.cfg.Running)
		return false
	}
	c.mu.Lock()
	ch, notBefore := c.channel, c.st.NotBefore
	c.mu.Unlock()
	if ch == Off {
		logger.Debug("appupdate: %s check skipped: channel is off", why)
		return false
	}
	if time.Now().Before(notBefore) {
		logger.Info("appupdate: %s check skipped: rate-limited until %s", why, notBefore.Format(time.RFC3339))
		return false
	}
	// Owed first, then read: the same order as inventory's postponeIfOffline,
	// so a reconnect landing between the two is not lost.
	c.owed.Store(true)
	if offlineNow() {
		logger.Info("appupdate: offline, %s check owed until the connection is back", why)
		return false
	}
	c.owed.Store(false)
	return true
}

// checkStillNeeded reports whether the channel currently in effect has no
// cached result yet — the same rule SetChannel uses to decide whether a
// switch needs a check at all (spec §2a).
func (c *Checker) checkStillNeeded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := c.channel
	if ch == Off {
		return false
	}
	if c.managedLocked() && ch == Stable {
		return c.st.PakJSON == nil
	}
	return c.st.Latest[ch] == nil
}

func (c *Checker) maybeCheck(why string) {
	if !c.gateOK(why) {
		return
	}
	if !c.busy.CompareAndSwap(false, true) {
		logger.Debug("appupdate: %s check skipped: one is already running", why)
		return
	}
	c.startCheck(why)
}

// startCheck dispatches the goroutine that runs the check. Caller must have
// already CompareAndSwap'd busy from false to true: that CAS is the single
// source of truth for "a check is running", so anyone who loses it (finds
// busy already true) is guaranteed this goroutine has not yet reached, or
// is still inside, the rerun loop below — busy is only cleared by the
// deferred Store(false), which runs after the loop exits. That is what lets
// SetChannel queue a rerun by CAS-losing here instead of racing a separate
// busy.Load().
func (c *Checker) startCheck(why string) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.notify()
		defer c.busy.Store(false)
		c.run(why)
		// A channel switch that landed while this check was running (spec
		// §2): rerun once more, still under busy, for whatever channel is
		// now in effect — but only if it still needs one; the channel
		// switched to may already have been filled by the check just run
		// (Releases fills both channels in one response).
		for c.rerun.Swap(false) {
			if !c.checkStillNeeded() || !c.gateOK("rerun") {
				continue
			}
			logger.Info("appupdate: channel changed mid-check, running once more")
			c.run("rerun")
		}
	}()
}

// abandonIfOffline reports whether err means the network, not GitHub or the
// Store, is the problem (spec §8: "Offline: skip and mark owed; run on
// reconnect"). When it is, the check is abandoned right here rather than
// treated as a failed check: owed is set (before anything else, same
// owed-then-read ordering as gateOK, so a reconnect racing in cannot be
// missed) and the caller returns without touching c.st or c.lastErr, so a
// transient outage never overwrites the last good state or rate-limits the
// service.
//
// Releases and PakJSON read their response bodies themselves
// (io.ReadAll / json.Decode) after the request's round trip already
// completed, so a body read that fails offline (e.g. the connection drops
// mid-download) happens after netstate.Transport's own RoundTrip-level
// Report and is never seen by it — report it explicitly here or a
// reconnect never finds this check owed.
func (c *Checker) abandonIfOffline(err error, why string) bool {
	if !netstate.Classify(err).Offline() {
		return false
	}
	c.owed.Store(true)
	netstate.Report(err)
	logger.Info("appupdate: %s check failed offline (%s), owed until the connection is back", why, netstate.Detail(err))
	return true
}

func (c *Checker) run(why string) {
	start := time.Now()
	store := c.lookupStore()

	c.mu.Lock()
	c.store = store
	ch := c.channel
	managed := c.managedLocked()
	etagRel, etagPak := c.st.ETag[etagReleases], c.st.ETag[etagPakJSON]
	c.mu.Unlock()

	// Stable on a Store-managed install reads the Store's own source. An rc on
	// a Store-managed install also reads it, for the downgrade warning (§2a).
	useReleases := !(managed && ch == Stable)
	usePak := managed && (ch == Stable || c.running.IsRC())
	logger.Info("appupdate: check start why=%s channel=%s releases=%v pakjson=%v store=%s",
		why, ch, useReleases, usePak, store)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var rel ReleasesResult
	var relErr, pakErr error
	var pak *Release
	var pakETag string
	var pakNM bool
	if useReleases {
		rel, relErr = c.src.Releases(ctx, etagRel)
		if c.abandonIfOffline(relErr, why) {
			return
		}
	}
	if usePak {
		pak, pakETag, pakNM, pakErr = c.src.PakJSON(ctx, etagPak)
		if c.abandonIfOffline(pakErr, why) {
			return
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	failed := relErr
	if useReleases && relErr == nil {
		c.st.ETag[etagReleases] = rel.ETag
		if !rel.NotModified {
			if rel.RC != nil {
				c.st.Latest[RC] = rel.RC
			}
			if rel.Stable != nil {
				c.st.Latest[Stable] = rel.Stable
			} else {
				logger.Warn("appupdate: no stable release on the page; keeping %s", tagOf(c.st.Latest[Stable]))
			}
		}
	}
	if usePak {
		if pakErr != nil {
			if failed == nil {
				failed = pakErr
			}
		} else {
			c.st.ETag[etagPakJSON] = pakETag
			if !pakNM {
				c.st.PakJSON = pak
			}
		}
	}
	var rl *RateLimitError
	switch {
	case errors.As(failed, &rl):
		c.st.NotBefore = rl.Until
		logger.Warn("appupdate: rate-limit back-off until %s", rl.Until.Format(time.RFC3339))
	case failed != nil:
		logger.Warn("appupdate: check failed after %v: %s", time.Since(start).Round(time.Millisecond), netstate.Detail(failed))
	default:
		c.st.CheckedAt = time.Now()
	}
	c.lastErr = failed
	c.saveLocked()
	v := c.verdictLocked()
	logger.Info("appupdate: verdict %s running=%s latest=%s store=%s via=%s store_offers=%q in %v",
		v.Kind, c.running, tagOf(v.Latest), c.store, v.Via, v.StoreOffers, time.Since(start).Round(time.Millisecond))
}

func (c *Checker) saveLocked() {
	if c.cfg.StatePath == "" {
		return
	}
	if err := c.st.Save(c.cfg.StatePath); err != nil {
		logger.Error("appupdate: save state: %v", err)
	}
}

func (c *Checker) verdictLocked() Verdict {
	if !c.runningOK {
		return Verdict{Channel: c.channel}
	}
	latest := c.st.Latest[c.channel]
	if c.managedLocked() && c.channel == Stable {
		latest = c.st.PakJSON
	}
	return Decide(Inputs{Firmware: c.cfg.Firmware, Running: c.running, Channel: c.channel,
		Store: c.store, Latest: latest, PakJSON: c.st.PakJSON})
}

// Verdict is the current answer, from the last successful check.
func (c *Checker) Verdict() Verdict {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verdictLocked()
}

func (c *Checker) IsRunning() bool { return c.busy.Load() }

func (c *Checker) CheckedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.CheckedAt
}

func (c *Checker) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// RateLimitedUntil is when GitHub allows the next check; zero when now.
func (c *Checker) RateLimitedUntil() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.st.NotBefore) {
		return c.st.NotBefore
	}
	return time.Time{}
}

func (c *Checker) Channel() Channel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.channel
}

// SetChannel switches channel. The cached result for it is used straight
// away; a check runs only when there is none (spec §2a).
func (c *Checker) SetChannel(ch Channel) {
	c.mu.Lock()
	prev := c.channel
	c.channel = ch
	c.mu.Unlock()
	needed := ch != Off && c.checkStillNeeded()
	logger.Info("appupdate: channel %s → %s (needs check=%v)", prev, ch, needed)
	if !needed {
		return
	}
	if !c.gateOK("channel") {
		return
	}
	if c.busy.CompareAndSwap(false, true) {
		c.startCheck("channel")
		return
	}
	// A check is already running for the old channel; it would otherwise
	// finish, clear busy, and drop this channel's need for data entirely
	// (spec §2a: switching "triggers a check otherwise"). Queue instead:
	// the running goroutine consumes rerun after run(), still under busy,
	// and runs once more for whatever channel is now in effect.
	c.rerun.Store(true)
	logger.Info("appupdate: channel changed to %s while a check is running, queued a rerun", ch)
}

// PendingNotice reports whether the notice is due (spec §2).
func (c *Checker) PendingNotice() (Verdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.verdictLocked()
	return v, ShouldNotify(v, c.st.Notified[v.Channel])
}

// MarkNotified records that the user has been told about tag on ch: when the
// notice finished its animation, or when the Updates screen showed it. It
// never lowers the recorded version.
func (c *Checker) MarkNotified(ch Channel, tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ShouldNotify(Verdict{Kind: Available, Latest: &Release{Tag: tag}}, c.st.Notified[ch]) {
		return
	}
	c.st.Notified[ch] = tag
	c.saveLocked()
	logger.Info("appupdate: notified channel=%s version=%s", ch, tag)
}

// StartArchiveSave downloads the current ViaArchive release into ARCHIVE/.
func (c *Checker) StartArchiveSave() {
	c.mu.Lock()
	v := c.verdictLocked()
	if v.Kind != Available || v.Via != ViaArchive || c.archive.State == ArchiveRunning || c.cfg.ArchiveDir == "" {
		c.mu.Unlock()
		logger.Debug("appupdate: Save to ARCHIVE ignored: verdict=%s via=%s state=%d", v.Kind, v.Via, c.archive.State)
		return
	}
	r := *v.Latest
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelArchive = cancel
	c.archive = ArchiveStatus{State: ArchiveRunning, Total: r.Size}
	c.mu.Unlock()

	logger.Info("appupdate: Save to ARCHIVE started for %s", r.Tag)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		path, err := SaveToArchive(ctx, c.dl, c.cfg.UserAgent, r, c.cfg.ArchiveDir, func(done, _ int64) {
			c.mu.Lock()
			c.archive.Done = done
			c.mu.Unlock()
		})
		c.mu.Lock()
		if err != nil {
			c.archive.State, c.archive.Err = ArchiveFailed, err
		} else {
			c.archive.State, c.archive.Path = ArchiveDone, path
		}
		c.cancelArchive = nil
		c.mu.Unlock()
		// SaveToArchive's error can embed the signed GitHub redirect URL (a
		// *url.Error); never log %v on it directly.
		switch {
		case err == nil:
			logger.Info("appupdate: Save to ARCHIVE saved %s to %s", r.Tag, path)
		case errors.Is(err, context.Canceled):
			// User-initiated (CancelArchiveSave) or app shutdown, not a
			// failure. ArchiveStatus still reports ArchiveFailed with
			// Err = context.Canceled; the UI words that as "Download
			// cancelled."
			logger.Info("appupdate: Save to ARCHIVE cancelled for %s", r.Tag)
		default:
			logger.Warn("appupdate: Save to ARCHIVE failed for %s: %s", r.Tag, netstate.Detail(err))
		}
		c.notify()
	}()
}

// CancelArchiveSave stops a running download; nothing is left behind.
func (c *Checker) CancelArchiveSave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelArchive != nil {
		logger.Info("appupdate: Save to ARCHIVE cancelled")
		c.cancelArchive()
	}
}

func (c *Checker) ArchiveStatus() ArchiveStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.archive
}

func (c *Checker) notify() {
	if c.cfg.Notify != nil {
		c.cfg.Notify()
	}
}
