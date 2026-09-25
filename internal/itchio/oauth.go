package itchio

// QR-code sign-in: the OAuth 2.0 device authorization grant with PKCE, as
// itch.io documents it at https://itch.io/docs/api/oauth#qr-code-login-device-authorization-grant.
//
// The handheld shows a QR code, the user approves on their phone, and the app
// receives an API key. The shapes follow itch.io's own go-itchio
// (endpoints_device.go, pkce.go); it is not a dependency because it puts keys
// in URLs and brings its own HTTP stack, rate limiter and User-Agent.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	// OAuthClientID is this app's registered itch.io OAuth application. It
	// is public; the client secret is never used — PKCE replaces it here.
	OAuthClientID = "1c04adbe430987908821ce556ed22014"
	// OAuthScope asks for what the app uses and nothing else: the profile
	// (username), the owned-games list, and downloading files. "itch" is
	// reserved for itch.io's own apps.
	OAuthScope = "profile:me profile:owned game:view:uploads"
	// DeviceRedirectURI is sent at the exchange for a code obtained by polling.
	DeviceRedirectURI = "urn:itchio:poll"

	// defaultPollInterval is used when the server does not say.
	defaultPollInterval = 5 * time.Second
)

var (
	// ErrSignInUnavailable: itch.io has not enabled QR sign-in for this app
	// (the client is unknown or not yet approved).
	ErrSignInUnavailable = errors.New("sign-in is not available for this app yet")
	// ErrSignInDenied: the user pressed deny on their phone.
	ErrSignInDenied = errors.New("sign-in was declined")
	// ErrSignInExpired: the code timed out or was already used; start over.
	ErrSignInExpired = errors.New("the sign-in code expired")
)

// GeneratePKCE returns a fresh PKCE verifier and its S256 challenge (RFC 7636).
// The challenge goes in the start request, the verifier in the exchange.
func GeneratePKCE() (verifier, challenge string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b[:])
	return verifier, PKCEChallenge(verifier), nil
}

// PKCEChallenge is the S256 code challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// DeviceLogin is one sign-in attempt, from the QR code to the API key.
// Never log deviceCode or verifier.
type DeviceLogin struct {
	// UserCode is shown beside the QR code; the phone shows the same code.
	UserCode string
	// QRURL is what the QR code encodes: the approval page with the code.
	QRURL string
	// Expires is when the code stops working.
	Expires time.Time

	c          *Client
	deviceCode string
	verifier   string
	deviceInfo string
	interval   time.Duration
	sleep      func(ctx context.Context, d time.Duration) error // replaced in tests
}

// BeginDeviceLogin starts a sign-in. deviceInfo describes the handheld for
// itch.io's records (see DeviceInfo) and is sent only at the exchange.
func (c *Client) BeginDeviceLogin(ctx context.Context, deviceInfo string) (*DeviceLogin, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	var resp struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
	}
	status, err := c.oauthPost(ctx, "/oauth/device", url.Values{
		"client_id":             {OAuthClientID},
		"scope":                 {OAuthScope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}, &resp)
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusNotFound:
		logger.Warn("oauth: sign-in unavailable — client %s is not approved for QR login", OAuthClientID)
		return nil, ErrSignInUnavailable
	case status == http.StatusTooManyRequests:
		return nil, fmt.Errorf("start sign-in: itch.io is busy, try again in a moment")
	case status != http.StatusOK:
		return nil, fmt.Errorf("start sign-in: HTTP %d", status)
	case resp.DeviceCode == "" || resp.VerificationURIComplete == "":
		return nil, fmt.Errorf("start sign-in: incomplete response from itch.io")
	}
	interval := time.Duration(resp.Interval) * time.Second
	if interval <= 0 {
		interval = defaultPollInterval
	}
	logger.Info("oauth: sign-in started, expires in %ds, polling every %s", resp.ExpiresIn, interval)
	return &DeviceLogin{
		UserCode:   resp.UserCode,
		QRURL:      resp.VerificationURIComplete,
		Expires:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		c:          c,
		deviceCode: resp.DeviceCode,
		verifier:   verifier,
		deviceInfo: deviceInfo,
		interval:   interval,
		sleep:      sleepCtx,
	}, nil
}

// Wait polls until the user decides, then exchanges the approval for an API
// key. It returns ErrSignInDenied or ErrSignInExpired for those outcomes, and
// the context's error when cancelled.
func (l *DeviceLogin) Wait(ctx context.Context) (string, error) {
	for {
		if time.Now().After(l.Expires) {
			logger.Info("oauth: sign-in code expired")
			return "", ErrSignInExpired
		}
		code, done, err := l.poll(ctx)
		if err != nil || done {
			if err != nil {
				return "", err
			}
			return l.exchange(ctx, code)
		}
		// Wait after each answer rather than on a fixed timer: itch.io may
		// make the poll long-running later.
		if err := l.sleep(ctx, l.interval); err != nil {
			logger.Info("oauth: sign-in cancelled")
			return "", err
		}
	}
}

// poll asks once. done is true with the code when approved.
func (l *DeviceLogin) poll(ctx context.Context) (code string, done bool, err error) {
	var resp struct {
		Status   string   `json:"status"`
		Code     string   `json:"code"`
		Interval int64    `json:"interval"`
		Errors   []string `json:"errors"`
	}
	status, err := l.c.oauthPost(ctx, "/oauth/device/poll", url.Values{
		"client_id":   {OAuthClientID},
		"device_code": {l.deviceCode},
	}, &resp)
	if err != nil {
		return "", false, err
	}
	switch {
	case status == http.StatusTooManyRequests:
		l.interval *= 2
		logger.Info("oauth: polling too fast, slowing to every %s", l.interval)
		return "", false, nil
	case status == http.StatusBadRequest && containsString(resp.Errors, "invalid_grant"):
		logger.Warn("oauth: sign-in request no longer valid, a new code is needed")
		return "", false, ErrSignInExpired
	case status != http.StatusOK:
		return "", false, fmt.Errorf("check sign-in: HTTP %d", status)
	}
	switch resp.Status {
	case "pending":
		if resp.Interval > 0 {
			l.interval = time.Duration(resp.Interval) * time.Second
		}
		return "", false, nil
	case "approved":
		logger.Info("oauth: sign-in approved on the phone")
		return resp.Code, true, nil
	case "denied":
		logger.Info("oauth: sign-in declined on the phone")
		return "", false, ErrSignInDenied
	case "expired":
		logger.Info("oauth: sign-in code expired (server)")
		return "", false, ErrSignInExpired
	default:
		return "", false, fmt.Errorf("check sign-in: unexpected status %q", resp.Status)
	}
}

// exchange trades the approved code for an API key. The code is single-use
// and short-lived, so this runs straight after approval.
func (l *DeviceLogin) exchange(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {l.verifier},
		"redirect_uri":  {DeviceRedirectURI},
		"client_id":     {OAuthClientID},
	}
	if l.deviceInfo != "" {
		form.Set("device_info", l.deviceInfo)
	}
	var resp struct {
		AccessToken string   `json:"access_token"`
		Scope       string   `json:"scope"`
		Errors      []string `json:"errors"`
	}
	status, err := l.c.oauthPost(ctx, "/oauth/token", form, &resp)
	if err != nil {
		return "", err
	}
	if status == http.StatusBadRequest && containsString(resp.Errors, "invalid_grant") {
		logger.Warn("oauth: approval could not be exchanged, a new code is needed")
		return "", ErrSignInExpired
	}
	if status != http.StatusOK || resp.AccessToken == "" {
		return "", fmt.Errorf("finish sign-in: HTTP %d", status)
	}
	logger.RegisterSecret(resp.AccessToken, "[TOKEN]")
	logger.Info("oauth: signed in, scope %q", resp.Scope)
	return resp.AccessToken, nil
}

// oauthPost sends a form POST to api.itch.io and decodes a JSON answer into
// out whatever the status. The per-request timeout of c.http does not apply:
// itch.io may turn the poll into a long poll, so only ctx bounds it.
func (c *Client) oauthPost(ctx context.Context, path string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.butler+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: c.http.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, withoutURL(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if len(body) > 0 && json.Unmarshal(body, out) != nil {
		logger.Debug("oauth: %s answered HTTP %d with a non-JSON body", path, resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// DeviceInfo describes the handheld for itch.io's sign-in records, e.g.
// "TrimUI Brick, NextUI 20260719-0, NextUI-Itchio-Pak 1.1.0". With
// shareDevice false, as with the User-Agent opt-out, only the app is named.
func DeviceInfo(info UAInfo, shareDevice bool) string {
	app := uaProduct + " " + strings.TrimPrefix(info.AppVersion, "v")
	if info.AppVersion == "" {
		app = uaProduct + " dev"
	}
	if !shareDevice || info.System == "" || info.System == "unknown-system" {
		return app
	}
	device := info.DeviceLabel
	if device == "" || strings.EqualFold(device, "unknown device") {
		device = info.Device
	}
	system := info.System
	fw := info.FirmwareVersion
	if len(fw) > len(system) && strings.EqualFold(fw[:len(system)], system) {
		fw = strings.TrimLeft(fw[len(system):], "- ")
	}
	if fw != "" && !strings.EqualFold(fw, "unknown") {
		system += " " + fw
	}
	parts := []string{}
	if device != "" {
		parts = append(parts, device)
	}
	return strings.Join(append(parts, system, app), ", ")
}
