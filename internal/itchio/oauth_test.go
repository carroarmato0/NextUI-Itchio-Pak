package itchio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The RFC 7636 appendix B test vector.
func TestPKCEChallenge_RFC7636Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	if got := PKCEChallenge(verifier); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("PKCEChallenge = %q", got)
	}
}

func TestGeneratePKCE(t *testing.T) {
	v1, c1, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	v2, _, _ := GeneratePKCE()
	if len(v1) < 43 || len(v1) > 128 {
		t.Errorf("verifier length %d, the spec allows 43..128", len(v1))
	}
	if strings.ContainsAny(v1, "+/=") {
		t.Errorf("verifier %q is not base64url without padding", v1)
	}
	if v1 == v2 {
		t.Error("two verifiers are identical")
	}
	if c1 != PKCEChallenge(v1) {
		t.Error("challenge does not match its verifier")
	}
}

// fakeOAuth is a scripted api.itch.io for the device grant.
type fakeOAuth struct {
	t *testing.T

	mu        sync.Mutex
	start     url.Values
	polls     int
	pollReply []func(w http.ResponseWriter) // one per poll; the last repeats
	exchange  url.Values
	startCode int // non-zero: answer /oauth/device with this status
}

func (f *fakeOAuth) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/device", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" {
			f.t.Errorf("start: %s, Authorization=%q", r.Method, r.Header.Get("Authorization"))
		}
		r.ParseForm()
		f.start = r.PostForm
		if f.startCode != 0 {
			w.WriteHeader(f.startCode)
			w.Write([]byte(`{"errors":["not found"]}`))
			return
		}
		w.Write([]byte(`{"device_code":"DEVCODE-secret","user_code":"KX7T-4MPB",
			"verification_uri":"https://itch.io/user/oauth/device",
			"verification_uri_complete":"https://itch.io/user/oauth/device?code=abc",
			"expires_in":600,"interval":5}`))
	})
	mux.HandleFunc("/oauth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		r.ParseForm()
		if r.PostForm.Get("device_code") != "DEVCODE-secret" || r.PostForm.Get("client_id") != OAuthClientID {
			f.t.Errorf("poll form = %v", r.PostForm)
		}
		i := f.polls
		if i >= len(f.pollReply) {
			i = len(f.pollReply) - 1
		}
		f.polls++
		f.pollReply[i](w)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		r.ParseForm()
		f.exchange = r.PostForm
		w.Write([]byte(`{"access_token":"TOKEN-secret","token_type":"bearer",
			"scope":"profile:me profile:owned game:view:uploads"}`))
	})
	return mux
}

func status(s string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { json.NewEncoder(w).Encode(map[string]string{"status": s}) }
}

func approved(w http.ResponseWriter) {
	w.Write([]byte(`{"status":"approved","code":"AUTHCODE-secret"}`))
}

func tooMany(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) }

func invalidGrant(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`{"errors":["invalid_grant"]}`))
}

// newFakeLogin starts a login against f and records every wait instead of sleeping.
func newFakeLogin(t *testing.T, f *fakeOAuth, deviceInfo string) (*DeviceLogin, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c := NewClientWithBaseAndButler(srv.URL, srv.URL)
	login, err := c.BeginDeviceLogin(context.Background(), deviceInfo)
	if err != nil {
		t.Fatalf("BeginDeviceLogin: %v", err)
	}
	var waits []time.Duration
	login.sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	return login, &waits
}

func TestBeginDeviceLogin_startRequest(t *testing.T) {
	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){approved}}
	login, _ := newFakeLogin(t, f, "")

	want := map[string]string{
		"client_id":             OAuthClientID,
		"scope":                 "profile:me profile:owned game:view:uploads",
		"code_challenge_method": "S256",
	}
	for k, v := range want {
		if got := f.start.Get(k); got != v {
			t.Errorf("start %s = %q, want %q", k, got, v)
		}
	}
	if f.start.Get("code_challenge") != PKCEChallenge(login.verifier) {
		t.Error("code_challenge is not the S256 of the login's verifier")
	}
	if f.start.Has("code_verifier") {
		t.Error("the verifier must not be sent when starting")
	}
	if login.UserCode != "KX7T-4MPB" || login.QRURL != "https://itch.io/user/oauth/device?code=abc" {
		t.Errorf("login = %+v", login)
	}
	if login.Expires.IsZero() || time.Until(login.Expires) < 9*time.Minute {
		t.Errorf("Expires = %v, want ~10 minutes from now", login.Expires)
	}
}

func TestBeginDeviceLogin_unapprovedClientIsUnavailable(t *testing.T) {
	f := &fakeOAuth{t: t, startCode: http.StatusNotFound}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := NewClientWithBaseAndButler(srv.URL, srv.URL)
	if _, err := c.BeginDeviceLogin(context.Background(), ""); !errors.Is(err, ErrSignInUnavailable) {
		t.Errorf("err = %v, want ErrSignInUnavailable", err)
	}
}

func TestDeviceLogin_pendingThenApproved(t *testing.T) {
	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending"), status("pending"), approved}}
	login, waits := newFakeLogin(t, f, "TrimUI Brick, NextUI 20260719-0, NextUI-Itchio-Pak 1.1.0")

	token, err := login.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if token != "TOKEN-secret" {
		t.Errorf("token = %q", token)
	}
	if f.polls != 3 {
		t.Errorf("polled %d times, want 3", f.polls)
	}
	// Waits happen between polls, at the server's interval.
	if len(*waits) != 2 || (*waits)[0] != 5*time.Second {
		t.Errorf("waits = %v, want two of 5s", *waits)
	}
	ex := f.exchange
	for k, v := range map[string]string{
		"grant_type": "authorization_code", "code": "AUTHCODE-secret", "code_verifier": login.verifier,
		"redirect_uri": "urn:itchio:poll", "client_id": OAuthClientID,
		"device_info": "TrimUI Brick, NextUI 20260719-0, NextUI-Itchio-Pak 1.1.0",
	} {
		if ex.Get(k) != v {
			t.Errorf("exchange %s = %q, want %q", k, ex.Get(k), v)
		}
	}
}

func TestDeviceLogin_slowDownDoublesInterval(t *testing.T) {
	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending"), tooMany, status("pending"), approved}}
	login, waits := newFakeLogin(t, f, "")
	if _, err := login.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 10 * time.Second}
	if len(*waits) != len(want) {
		t.Fatalf("waits = %v, want %v", *waits, want)
	}
	for i := range want {
		if (*waits)[i] != want[i] {
			t.Errorf("waits = %v, want %v", *waits, want)
			break
		}
	}
}

func TestDeviceLogin_outcomes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply func(http.ResponseWriter)
		want  error
	}{
		{"denied", status("denied"), ErrSignInDenied},
		{"expired", status("expired"), ErrSignInExpired},
		{"invalid grant starts over", invalidGrant, ErrSignInExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending"), tc.reply}}
			login, _ := newFakeLogin(t, f, "")
			if _, err := login.Wait(context.Background()); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if f.exchange != nil {
				t.Error("exchanged a code that was never approved")
			}
		})
	}
}

// The request's own lifetime ends the wait even if the server keeps saying pending.
func TestDeviceLogin_expiresLocally(t *testing.T) {
	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending")}}
	login, _ := newFakeLogin(t, f, "")
	login.Expires = time.Now().Add(-time.Second)
	if _, err := login.Wait(context.Background()); !errors.Is(err, ErrSignInExpired) {
		t.Errorf("err = %v, want ErrSignInExpired", err)
	}
}

func TestDeviceLogin_cancelStopsPolling(t *testing.T) {
	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending")}}
	login, _ := newFakeLogin(t, f, "")
	ctx, cancel := context.WithCancel(context.Background())
	login.sleep = func(ctx context.Context, d time.Duration) error {
		cancel() // the user pressed B while waiting
		return ctx.Err()
	}
	if _, err := login.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if f.polls != 1 {
		t.Errorf("polled %d times after cancel, want 1", f.polls)
	}
}

// None of the secrets of the flow may reach the log.
func TestDeviceLogin_logsNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	f := &fakeOAuth{t: t, pollReply: []func(http.ResponseWriter){status("pending"), approved}}
	login, _ := newFakeLogin(t, f, "")
	if _, err := login.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"DEVCODE-secret", "AUTHCODE-secret", "TOKEN-secret", login.verifier} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, buf.String())
		}
	}
	if !strings.Contains(buf.String(), "oauth:") {
		t.Error("the flow logged nothing at all")
	}
}

func TestDeviceInfo(t *testing.T) {
	info := UAInfo{AppVersion: "v1.1.0", System: "NextUI", FirmwareVersion: "NextUI-20260719-0",
		Device: "tg5040", DeviceLabel: "TrimUI Brick", Platform: "linux/arm64"}
	if got := DeviceInfo(info, true); got != "TrimUI Brick, NextUI 20260719-0, NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("detailed = %q", got)
	}
	if got := DeviceInfo(info, false); got != "NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("opted out = %q", got)
	}
	// An unmapped board is named by its code; an unknown system leaves no device part.
	if got := DeviceInfo(UAInfo{AppVersion: "1.1.0", System: "muOS", Device: "rg-new", DeviceLabel: "rg-new"}, true); got != "rg-new, muOS, NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("unmapped = %q", got)
	}
	if got := DeviceInfo(UAInfo{AppVersion: "1.1.0", System: "unknown-system"}, true); got != "NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("unknown system = %q", got)
	}
}

// A revoked or invalid token must be told apart from a network failure: the
// first signs the user out, the second must not.
func TestValidateAPIKey_rejectedTokenIsErrTokenRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClientWithBaseAndButler(srv.URL, srv.URL)
	if _, _, err := c.ValidateAPIKey("revoked"); !errors.Is(err, ErrTokenRejected) {
		t.Errorf("err = %v, want ErrTokenRejected", err)
	}

	down := NewClientWithBaseAndButler("http://127.0.0.1:1", "http://127.0.0.1:1")
	if _, _, err := down.ValidateAPIKey("t"); err == nil || errors.Is(err, ErrTokenRejected) {
		t.Errorf("unreachable server: err = %v, want a non-rejection error", err)
	}
}

func TestCurrentDeviceInfo_followsShareSetting(t *testing.T) {
	t.Cleanup(func() { ConfigureUserAgent(UAInfo{}, true) })
	ConfigureUserAgent(UAInfo{AppVersion: "v1.1.0", System: "muOS", FirmwareVersion: "2601.0_JACARANDA",
		Device: "tui-spoon", DeviceLabel: "TrimUI Smart Pro"}, true)
	if got := CurrentDeviceInfo(); got != "TrimUI Smart Pro, muOS 2601.0_JACARANDA, NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("shared = %q", got)
	}
	SetShareDeviceInfo(false)
	if got := CurrentDeviceInfo(); got != "NextUI-Itchio-Pak 1.1.0" {
		t.Errorf("opted out = %q", got)
	}
}
