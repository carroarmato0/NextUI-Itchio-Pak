//go:build !headless

package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)

type signInState int32

const (
	signInStarting signInState = iota
	signInWaiting
	signInSuccess
	signInExpired
	signInDenied
	signInUnavailable
	signInFailed
)

// SignInScreen signs the user in with a QR code: the phone approves, the
// handheld polls and receives an API key. See docs/mockups/signin.html.
type SignInScreen struct {
	client       *itchio.Client
	cfg          *settings.Config
	cfgPath      string
	prev         Screen
	onOwnedReady func([]itchio.OwnedGame)

	state  int32 // signInState, atomic
	cancel context.CancelFunc

	mu         sync.Mutex // guards the fields the worker writes
	login      *itchio.DeviceLogin
	username   string
	ownedCount int

	qrTex        *sdl.Texture
	qrURL        string
	lastSecsDrew int
}

// NewSignInScreen starts a sign-in right away. prev is shown on B, and after
// a successful sign-in.
func NewSignInScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, prev Screen,
	onOwnedReady func([]itchio.OwnedGame)) *SignInScreen {
	s := &SignInScreen{client: client, cfg: cfg, cfgPath: cfgPath, prev: prev, onOwnedReady: onOwnedReady}
	s.start()
	return s
}

func (s *SignInScreen) loadState() signInState   { return signInState(atomic.LoadInt32(&s.state)) }
func (s *SignInScreen) storeState(st signInState) { atomic.StoreInt32(&s.state, int32(st)) }

// start begins a fresh sign-in attempt in the background.
func (s *SignInScreen) start() {
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Lock()
	s.login = nil
	s.mu.Unlock()
	s.storeState(signInStarting)
	logger.Info("signin: starting")

	go func() {
		defer sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
		login, err := s.client.BeginDeviceLogin(ctx, itchio.CurrentDeviceInfo())
		if err != nil {
			s.fail(ctx, err)
			return
		}
		s.mu.Lock()
		s.login = login
		s.mu.Unlock()
		s.storeState(signInWaiting)
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})

		token, err := login.Wait(ctx)
		if err != nil {
			s.fail(ctx, err)
			return
		}
		username, owned, err := s.client.ValidateAPIKey(token)
		if err != nil {
			logger.Error("signin: token received but profile check failed: %v", err)
			s.fail(ctx, err)
			return
		}
		s.cfg.SetSignedIn(token, username, time.Now())
		s.client.SetAuthToken(token)
		s.cfg.OnboardingSeen = true
		if err := s.cfg.Save(s.cfgPath); err != nil {
			logger.Error("signin: could not save the sign-in: %v", err)
		}
		if s.onOwnedReady != nil {
			s.onOwnedReady(owned)
		}
		s.mu.Lock()
		s.username, s.ownedCount = username, len(owned)
		s.mu.Unlock()
		logger.Info("signin: signed in, %d owned game(s)", len(owned))
		s.storeState(signInSuccess)
	}()
}

func (s *SignInScreen) fail(ctx context.Context, err error) {
	switch {
	case ctx.Err() != nil:
		return // cancelled with B: nothing to show
	case errors.Is(err, itchio.ErrSignInUnavailable):
		s.storeState(signInUnavailable)
	case errors.Is(err, itchio.ErrSignInDenied):
		s.storeState(signInDenied)
	case errors.Is(err, itchio.ErrSignInExpired):
		s.storeState(signInExpired)
	default:
		logger.Warn("signin: %v", err)
		s.storeState(signInFailed)
	}
}

func (s *SignInScreen) remaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.login == nil {
		return 0
	}
	return time.Until(s.login.Expires)
}

// NeedsRedraw is true once per second while waiting, for the countdown.
func (s *SignInScreen) NeedsRedraw() bool {
	return s.loadState() == signInWaiting && int(s.remaining().Seconds()) != s.lastSecsDrew
}

func (s *SignInScreen) HasPendingAnimation() bool {
	st := s.loadState()
	return st == signInWaiting || st == signInStarting
}

func (s *SignInScreen) Draw(r *renderer.Renderer) {
	bg := r.Theme.Background
	r.Clear(bg[0], bg[1], bg[2])
	textY := r.DrawHeaderBar(signInHeaderH)
	mt := r.Theme.MainText
	r.DrawText("Sign in to itch.io", 20, textY, mt[0], mt[1], mt[2])

	var hints []renderer.FooterHint
	switch st := s.loadState(); st {
	case signInStarting:
		_, fontH := r.TextSize("Ag")
		mid := signInHeaderH + (r.H-signInHeaderH-signInFooterH)/2
		r.DrawTextCentered("Getting a sign-in code", 0, mid-fontH-10, r.W, mt[0], mt[1], mt[2])
		drawLoadingDots(r, mid+8)
		hints = []renderer.FooterHint{{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"}}
	case signInWaiting:
		s.drawWaiting(r)
		hints = []renderer.FooterHint{{Kind: renderer.BadgeCircle, Label: "B", Text: "Cancel"}}
	default:
		title, body, tone, h := s.outcome(r, st)
		s.drawOutcome(r, title, body, tone)
		hints = h
	}
	ftrY := r.DrawFooterBar(signInFooterH)
	r.DrawFooterHints(hints, ftrY)
	r.Present()
}

func (s *SignInScreen) outcome(r *renderer.Renderer, st signInState) (title, body string, tone [3]uint8, hints []renderer.FooterHint) {
	a := func(text string) renderer.FooterHint {
		return renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "A", Text: text}
	}
	back := renderer.FooterHint{Kind: renderer.BadgeCircle, Label: "B", Text: "Back"}
	switch st {
	case signInSuccess:
		s.mu.Lock()
		user, n := s.username, s.ownedCount
		s.mu.Unlock()
		return "Signed in as " + user,
			fmt.Sprintf("You own %d games on itch.io. The paid ones can now be downloaded from their game pages.", n),
			r.Theme.SuccessAction(), []renderer.FooterHint{a("Continue")}
	case signInExpired:
		return "This code has expired", "Codes last 10 minutes. Get a new one and scan it again.",
			r.Theme.Warning(), []renderer.FooterHint{a("New code"), back}
	case signInDenied:
		return "Sign-in was declined",
			"Someone chose Deny on the phone. If that wasn't intended, get a new code and approve it.",
			r.Theme.Warning(), []renderer.FooterHint{a("New code"), back}
	case signInUnavailable:
		return "Sign-in isn't available yet",
			"itch.io hasn't enabled QR sign-in for this app. Free games still download without an account.",
			r.Theme.Warning(), []renderer.FooterHint{back}
	default:
		return "Can't reach itch.io", "Check that Wi-Fi is on and connected, then try again.",
			r.Theme.Error(), []renderer.FooterHint{a("Try again"), back}
	}
}

func (s *SignInScreen) drawOutcome(r *renderer.Renderer, title, body string, tone [3]uint8) {
	_, fontH := r.TextSize("Ag")
	x := r.W / 10
	w := r.W - 2*x
	lineH := fontH + fontH/6
	lines := r.WrapText(body, w)
	total := fontH + fontH/2 + int32(len(lines))*lineH
	y := signInHeaderH + (r.H-signInHeaderH-signInFooterH-total)/2
	r.DrawText(title, x, y, tone[0], tone[1], tone[2])
	lt := r.Theme.ListText
	r.DrawWrappedText(body, x, y+fontH+fontH/2, w, lineH, lt[0], lt[1], lt[2])
}

func (s *SignInScreen) drawWaiting(r *renderer.Renderer) {
	s.mu.Lock()
	login := s.login
	s.mu.Unlock()
	if login == nil {
		return
	}
	modules, err := renderer.QRModules(login.QRURL)
	if err != nil {
		logger.Error("signin: %v", err)
		return
	}
	_, fontH := r.TextSize("Ag")
	_, smallFH := r.SmallTextSize("Ag")
	l := layoutSignIn(r.W, r.H, modules, fontH, smallFH)

	if s.qrTex == nil || s.qrURL != login.QRURL {
		if s.qrTex != nil {
			s.qrTex.Destroy()
		}
		s.qrTex, err = r.QRTexture(login.QRURL, int(l.QRSize))
		s.qrURL = login.QRURL
		if err != nil {
			logger.Error("signin: QR texture: %v", err)
			return
		}
		logger.Debug("signin: QR %dpx, %dpx modules", l.QRSize, l.Module)
	}
	r.DrawTextureAt(s.qrTex, l.QRX, l.QRY, l.QRSize, l.QRSize)

	left := s.remaining()
	if left < 0 {
		left = 0
	}
	secs := int(left.Seconds())
	s.lastSecsDrew = secs
	timer := fmt.Sprintf("Expires in %d:%02d", secs/60, secs%60)
	address := strings.TrimPrefix(login.QRURL, "https://")

	mt, ht, mu := r.Theme.MainText, r.Theme.HintText, r.Theme.Muted()
	codeSize := l.CodeSize
	for codeSize > int(fontH) {
		if cw, _ := r.DisplayTextSize(login.UserCode, codeSize); cw <= l.TextW {
			break
		}
		codeSize -= 2
	}
	codeW, codeH := r.DisplayTextSize(login.UserCode, codeSize)
	barW := l.TextW * 38 / 100
	barH := max32(smallFH/5, 3)
	frac := float64(left) / float64(10*time.Minute)
	if frac > 1 {
		frac = 1
	}
	chip := r.Theme.Chip()

	if l.Stacked {
		y := l.TextY
		r.DrawSmallTextCentered("Scan with your phone, then check it shows this code", l.TextX, y, l.TextW, ht[0], ht[1], ht[2])
		y += smallFH + smallFH/4
		r.DrawDisplayText(login.UserCode, (r.W-codeW)/2, y, codeSize, mt[0], mt[1], mt[2])
		y += codeH + smallFH/3
		tw, _ := r.SmallTextSize(timer)
		bx := (r.W - (barW + smallFH/2 + tw)) / 2
		r.DrawRect(bx, y+smallFH/2, barW, barH, chip[0], chip[1], chip[2])
		r.DrawRect(bx, y+smallFH/2, int32(float64(barW)*frac), barH, ht[0], ht[1], ht[2])
		r.DrawSmallText(timer, bx+barW+smallFH/2, y, ht[0], ht[1], ht[2])
		y = r.H - signInFooterH - signInPad/2 - smallFH
		r.DrawSmallTextCentered(address, 0, y, r.W, mu[0], mu[1], mu[2])
		return
	}

	s.drawColumn(r, l, login.UserCode, codeSize, codeH, timer, address, frac, fontH, smallFH)
}

// drawColumn fills the text column beside the QR code. On short screens the
// column is only as tall as the code, so the optional lines — the lead-in and
// the "Can't scan?" label — are dropped until the rest fits.
func (s *SignInScreen) drawColumn(r *renderer.Renderer, l signInLayout, code string, codeSize int,
	codeH int32, timer, address string, frac float64, fontH, smallFH int32) {
	mt, ht, mu := r.Theme.MainText, r.Theme.HintText, r.Theme.Muted()
	chip := r.Theme.Chip()
	lineH := smallFH + 2
	hint := wrapSmallWords(r, "Check that your phone shows this code", l.TextW)
	addr := breakSmallText(r, address, l.TextW)
	gap := smallFH / 3

	lead, label := true, true
	need := func() int32 {
		h := int32(len(hint))*lineH + codeH + gap + smallFH + gap + int32(len(addr))*lineH
		if lead {
			h += fontH + fontH/2
		}
		if label {
			h += lineH + gap
		}
		return h
	}
	if need() > l.QRSize {
		lead = false
	}
	if need() > l.QRSize {
		label = false
	}

	x, y := l.TextX, l.TextY
	if lead {
		r.DrawText("Scan with your phone", x, y, mt[0], mt[1], mt[2])
		y += fontH + fontH/2
	}
	for _, line := range hint {
		r.DrawSmallText(line, x, y, ht[0], ht[1], ht[2])
		y += lineH
	}
	r.DrawDisplayText(code, x, y, codeSize, mt[0], mt[1], mt[2])
	y += codeH + gap
	barW := l.TextW * 38 / 100
	barH := max32(smallFH/5, 3)
	r.DrawRect(x, y+smallFH/2, barW, barH, chip[0], chip[1], chip[2])
	r.DrawRect(x, y+smallFH/2, int32(float64(barW)*frac), barH, ht[0], ht[1], ht[2])
	r.DrawSmallText(timer, x+barW+smallFH/2, y, ht[0], ht[1], ht[2])

	// The address sits at the bottom of the column, for anyone who can't scan.
	ay := l.QRY + l.QRSize - int32(len(addr))*lineH
	if label {
		ay -= lineH
		r.DrawSmallText("Can't scan? Open this address:", x, ay, ht[0], ht[1], ht[2])
		ay += lineH
	}
	for i, line := range addr {
		r.DrawSmallText(line, x, ay+int32(i)*lineH, mu[0], mu[1], mu[2])
	}
}

// wrapSmallWords word-wraps s in small text to lines no wider than w.
func wrapSmallWords(r *renderer.Renderer, s string, w int32) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if tw, _ := r.SmallTextSize(next); tw > w && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		line = next
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// breakSmallText splits s into lines of small text no wider than w. A URL has
// no spaces to wrap at, so it breaks between characters.
func breakSmallText(r *renderer.Renderer, s string, w int32) []string {
	var lines []string
	line := ""
	for _, ch := range s {
		if tw, _ := r.SmallTextSize(line + string(ch)); tw > w && line != "" {
			lines = append(lines, line)
			line = ""
		}
		line += string(ch)
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (s *SignInScreen) close() Screen {
	if s.cancel != nil {
		s.cancel()
	}
	if s.qrTex != nil {
		s.qrTex.Destroy()
		s.qrTex = nil
	}
	return s.prev
}

func (s *SignInScreen) HandleEvent(e sdl.Event) Screen {
	var a, b bool
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type == sdl.KEYDOWN {
			a, b = ev.Keysym.Sym == sdl.K_RETURN, ev.Keysym.Sym == sdl.K_ESCAPE
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type == sdl.CONTROLLERBUTTONDOWN {
			a, b = ev.Button == btnA, ev.Button == btnB
		}
	}
	st := s.loadState()
	switch {
	case b && st != signInSuccess:
		logger.Info("signin: cancelled by user")
		return s.close()
	case a && st == signInSuccess:
		return s.close()
	case a && (st == signInExpired || st == signInDenied || st == signInFailed):
		s.start()
	}
	return s
}
