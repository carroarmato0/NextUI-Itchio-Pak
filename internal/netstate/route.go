package netstate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HasRoute reports whether the device has a default route on an interface
// that is up. It reads two procfs/sysfs files and makes no request, so it can
// run every few seconds. When the files cannot be read it answers true: not
// knowing is no reason to tell the user they are offline.
func HasRoute(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "proc/net/route"))
	if err != nil {
		return true
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 4 || f[1] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x1 == 0 { // RTF_UP
			continue
		}
		st, err := os.ReadFile(filepath.Join(root, "sys/class/net", f[0], "operstate"))
		if err != nil {
			return true
		}
		switch strings.TrimSpace(string(st)) {
		case "up", "unknown":
			return true
		}
	}
	return false
}
