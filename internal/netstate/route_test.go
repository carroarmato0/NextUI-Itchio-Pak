package netstate

import (
	"os"
	"path/filepath"
	"testing"
)

const routeHeader = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"

func fakeRoot(t *testing.T, route string, operstate map[string]string) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "proc/net"), 0755)
	os.WriteFile(filepath.Join(root, "proc/net/route"), []byte(routeHeader+route), 0644)
	for iface, st := range operstate {
		d := filepath.Join(root, "sys/class/net", iface)
		os.MkdirAll(d, 0755)
		os.WriteFile(filepath.Join(d, "operstate"), []byte(st+"\n"), 0644)
	}
	return root
}

// Copied from the Brick with Wi-Fi on (2026-09-26).
const brickRoute = "wlan0\t00000000\t0132A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
	"wlan0\t0032A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n"

func TestHasRoute(t *testing.T) {
	if !HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "up"})) {
		t.Error("default route on an up interface: want true")
	}
	if HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "down"})) {
		t.Error("interface down: want false")
	}
	if HasRoute(fakeRoot(t, "wlan0\t0032A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n", map[string]string{"wlan0": "up"})) {
		t.Error("no default route: want false")
	}
	if !HasRoute(fakeRoot(t, brickRoute, map[string]string{"wlan0": "unknown"})) {
		t.Error("operstate unknown (some drivers): want true")
	}
	if !HasRoute(t.TempDir()) {
		t.Error("no /proc/net/route: cannot tell, must not claim offline")
	}
}
