package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

const uaHead = "NextUI-Itchio-Pak/1.0.25 (+https://github.com/carroarmato0/NextUI-Itchio-Pak"

func TestBuildUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		info     itchio.UAInfo
		detailed bool
		want     string
	}{
		{
			name:     "nextui fully known",
			info:     itchio.UAInfo{AppVersion: "v1.0.25", System: "NextUI", FirmwareVersion: "6.3.0", Device: "tg5040", DeviceLabel: "TrimUI Brick", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; NextUI 6.3.0; tg5040; TrimUI Brick; linux/arm64)",
		},
		{
			name:     "muos fully known",
			info:     itchio.UAInfo{AppVersion: "v1.0.25", System: "muOS", FirmwareVersion: "2601.0", Device: "tui-spoon", DeviceLabel: "TrimUI Smart Pro", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; muOS 2601.0; tui-spoon; TrimUI Smart Pro; linux/arm64)",
		},
		{
			name:     "opt-out sends only product and url",
			info:     itchio.UAInfo{AppVersion: "v1.0.25", System: "NextUI", FirmwareVersion: "6.3.0", Device: "tg5040", DeviceLabel: "TrimUI Brick", Platform: "linux/arm64"},
			detailed: false,
			want:     uaHead + ")",
		},
		{
			name:     "unmapped muos board: label is a copy of the code",
			info:     itchio.UAInfo{AppVersion: "1.0.25", System: "muOS", FirmwareVersion: "2601.0", Device: "rg-newboard", DeviceLabel: "rg-newboard", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; muOS 2601.0; rg-newboard; linux/arm64)",
		},
		{
			name:     "unknown nextui platform: placeholder label dropped",
			info:     itchio.UAInfo{AppVersion: "1.0.25", System: "NextUI", FirmwareVersion: "unknown", Device: "newplat", DeviceLabel: "unknown device", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; NextUI; newplat; linux/arm64)",
		},
		{
			name:     "unsupported system",
			info:     itchio.UAInfo{AppVersion: "1.0.25", System: "unknown-system", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; unknown-system; linux/arm64)",
		},
		{
			name:     "nothing known at all",
			info:     itchio.UAInfo{},
			detailed: true,
			want:     "NextUI-Itchio-Pak/dev (+https://github.com/carroarmato0/NextUI-Itchio-Pak; unknown-system; " + runtime.GOOS + "/" + runtime.GOARCH + ")",
		},
		{
			name:     "hostile firmware version",
			info:     itchio.UAInfo{AppVersion: "1.0.25", System: "NextUI", FirmwareVersion: "6.3 (beta); x\r\nInjected: yesé", Device: "tg5040", DeviceLabel: "TrimUI Brick", Platform: "linux/arm64"},
			detailed: true,
			want:     uaHead + "; NextUI 6.3 -beta- x-Injected: yes; tg5040; TrimUI Brick; linux/arm64)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := itchio.BuildUserAgent(tc.info, tc.detailed); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestBuildUserAgent_capsLongFields(t *testing.T) {
	info := itchio.UAInfo{AppVersion: "1.0.25", System: "NextUI", FirmwareVersion: strings.Repeat("A", 1024), Platform: "linux/arm64"}
	got := itchio.BuildUserAgent(info, true)
	if len(got) > 200 {
		t.Errorf("UA is %d bytes; a runaway version file must be capped: %q", len(got), got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("UA contains a line break: %q", got)
	}
}

func TestUAInfoFromEnv(t *testing.T) {
	t.Run("nextui reads its version file", func(t *testing.T) {
		t.Setenv("PLATFORM", "tg5040")
		prefix := t.TempDir()
		sys := filepath.Join(prefix, "mnt", "SDCARD", ".system")
		if err := os.MkdirAll(sys, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sys, "version.txt"), []byte("NextUI-6.3.0\nabc123\n"), 0644); err != nil {
			t.Fatal(err)
		}
		info := itchio.UAInfoFromEnv("v1.0.25", firmware.ForTest(firmware.KindNextUI, prefix))
		if info.System != "NextUI" || info.FirmwareVersion != "NextUI-6.3.0" || info.Device != "tg5040" || info.DeviceLabel == "" {
			t.Errorf("unexpected info: %+v", info)
		}
	})

	t.Run("host reports unknown-system and nothing else", func(t *testing.T) {
		info := itchio.UAInfoFromEnv("v1.0.25", firmware.ForTest(firmware.KindHost, t.TempDir()))
		want := itchio.UAInfo{AppVersion: "v1.0.25", System: "unknown-system", Platform: runtime.GOOS + "/" + runtime.GOARCH}
		if info != want {
			t.Errorf("got %+v, want %+v", info, want)
		}
	})

	t.Run("nil env", func(t *testing.T) {
		if got := itchio.UAInfoFromEnv("", nil).System; got != "unknown-system" {
			t.Errorf("System = %q, want unknown-system", got)
		}
	})
}

// The client must identify itself and must not pose as a browser.
func TestClient_sendsRealUserAgent(t *testing.T) {
	info := itchio.UAInfo{AppVersion: "v1.0.25", System: "NextUI", Device: "tg5040", DeviceLabel: "TrimUI Brick", Platform: "linux/arm64"}
	t.Cleanup(func() { itchio.ConfigureUserAgent(itchio.UAInfo{}, true) })

	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	defer srv.Close()
	c := itchio.NewClientWithBase(srv.URL)

	itchio.ConfigureUserAgent(info, true)
	c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")
	if ua := got.Get("User-Agent"); ua != itchio.BuildUserAgent(info, true) {
		t.Errorf("User-Agent = %q", ua)
	}
	for name := range got {
		l := strings.ToLower(name)
		if strings.HasPrefix(l, "sec-ch-ua") || strings.HasPrefix(l, "sec-fetch-") {
			t.Errorf("browser header %q still sent", name)
		}
	}

	// Opting out takes effect on the next request, no new client needed.
	itchio.SetShareDeviceInfo(false)
	c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")
	if ua := got.Get("User-Agent"); ua != itchio.BuildUserAgent(info, false) {
		t.Errorf("after opt-out User-Agent = %q", ua)
	}
}
