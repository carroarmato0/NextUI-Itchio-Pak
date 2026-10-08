package itchio

import (
	"runtime"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const (
	uaProduct = "NextUI-Itchio-Pak"
	uaURL     = "https://github.com/carroarmato0/NextUI-Itchio-Pak"

	// uaFieldMax caps each comment field. Firmware version files are free text
	// the app does not control; a runaway one must not balloon every request.
	uaFieldMax = 48
)

// UAInfo is everything the User-Agent may say about the client. Every field
// is optional except Platform: an unknown value is left out, never guessed.
type UAInfo struct {
	AppVersion      string // "v1.0.25" or "1.0.25"; empty or "dev" for dev builds
	System          string // "NextUI", "muOS", or "unknown-system"
	FirmwareVersion string // the firmware's own version string; "unknown" is omitted
	Device          string // firmware code for the hardware ("tg5040", "tui-spoon")
	DeviceLabel     string // human-readable name; omitted when it adds nothing
	Platform        string // GOOS/GOARCH — always known, so always sent
}

// UAInfoFromEnv collects UAInfo from the detected firmware. Anything the
// firmware could not recognise — an unsupported system, an unmapped board —
// arrives here as an empty or placeholder value and is dropped by the builder.
func UAInfoFromEnv(appVersion string, env *firmware.Env) UAInfo {
	info := UAInfo{
		AppVersion: appVersion,
		System:     "unknown-system",
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
	}
	if env == nil {
		return info
	}
	switch env.Kind() {
	case firmware.KindNextUI:
		info.System = "NextUI"
	case firmware.KindMuOS:
		info.System = "muOS"
	default:
		// KindHost: a developer machine, or a firmware DetectIn does not know.
		// Its version and device fields describe nothing real, so skip them.
		return info
	}
	info.FirmwareVersion = env.FirmwareVersion()
	info.Device = env.Device()
	info.DeviceLabel = env.DeviceLabel()
	return info
}

// BuildUserAgent renders the User-Agent. With detailed=false only the product
// token and project URL are sent — the opt-out form. It never returns "".
func BuildUserAgent(info UAInfo, detailed bool) string {
	version := strings.TrimPrefix(sanitizeUAField(info.AppVersion), "v")
	if version == "" {
		version = "dev"
	}
	head := uaProduct + "/" + version + " (+" + uaURL
	if !detailed {
		return head + ")"
	}

	fields := []string{}
	system := sanitizeUAField(info.System)
	if system == "" {
		system = "unknown-system"
	}
	fw := sanitizeUAField(info.FirmwareVersion)
	// NextUI's version file already names itself ("NextUI-20260719-0").
	if len(fw) > len(system) && strings.EqualFold(fw[:len(system)], system) &&
		(fw[len(system)] == '-' || fw[len(system)] == ' ') {
		fw = strings.TrimLeft(fw[len(system):], "- ")
	}
	if fw != "" && !strings.EqualFold(fw, "unknown") {
		system += " " + fw
	}
	fields = append(fields, system)

	device := sanitizeUAField(info.Device)
	if device != "" {
		fields = append(fields, device)
	}
	// Firmware fills the label with a placeholder or a copy of the code when
	// it does not recognise the hardware; neither tells itch.io anything.
	if label := sanitizeUAField(info.DeviceLabel); label != "" &&
		!strings.EqualFold(label, device) &&
		!strings.EqualFold(label, "unknown device") &&
		!strings.EqualFold(label, "host") {
		fields = append(fields, label)
	}

	platform := sanitizeUAField(info.Platform)
	if platform == "" {
		platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	fields = append(fields, platform)

	return head + "; " + strings.Join(fields, "; ") + ")"
}

// sanitizeUAField makes a value safe inside a User-Agent comment: printable
// ASCII only, no characters that would end the comment or split a field, runs
// of anything else collapsed to one "-", and a length cap.
func sanitizeUAField(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(s) {
		ok := r >= 0x20 && r < 0x7f && r != '(' && r != ')' && r != ';' && r != '\\'
		if !ok {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		b.WriteRune(r)
		lastDash = r == '-'
	}
	out := strings.Trim(b.String(), " -")
	if len(out) > uaFieldMax {
		out = strings.TrimRight(out[:uaFieldMax], " -")
	}
	return out
}

var (
	uaMu       sync.RWMutex
	uaInfo     = UAInfo{Platform: runtime.GOOS + "/" + runtime.GOARCH}
	uaDetailed = true
	uaCurrent  = BuildUserAgent(uaInfo, true)
)

// ConfigureUserAgent sets what the User-Agent reports and whether the device
// details are shared. Called once at startup, after firmware detection.
func ConfigureUserAgent(info UAInfo, shareDeviceInfo bool) {
	uaMu.Lock()
	uaInfo, uaDetailed = info, shareDeviceInfo
	uaCurrent = BuildUserAgent(uaInfo, uaDetailed)
	ua := uaCurrent
	uaMu.Unlock()
	logger.Info("client: user-agent %q", ua)
}

// SetShareDeviceInfo switches between the detailed and minimal User-Agent.
// Takes effect on the next request — no restart needed.
func SetShareDeviceInfo(on bool) {
	uaMu.Lock()
	uaDetailed = on
	uaCurrent = BuildUserAgent(uaInfo, uaDetailed)
	ua := uaCurrent
	uaMu.Unlock()
	logger.Info("client: share device info=%v, user-agent %q", on, ua)
}

// UserAgent is the User-Agent sent on every outbound request.
func UserAgent() string {
	uaMu.RLock()
	defer uaMu.RUnlock()
	return uaCurrent
}

// CurrentDeviceInfo is DeviceInfo for this device, honouring the "Share
// device info" setting the same way the User-Agent does.
func CurrentDeviceInfo() string {
	uaMu.RLock()
	defer uaMu.RUnlock()
	return DeviceInfo(uaInfo, uaDetailed)
}
