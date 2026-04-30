package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// version is set at build time via -ldflags:
//
//	-X main.version=vX.Y.Z
var version = "dev"

// gitCommit is set at build time via -ldflags:
//
//	-X main.gitCommit=xxxxxxx
var gitCommit = "unknown"

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logPath := logFilePath()
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	rotateLog(logPath)
	logFile, err := os.OpenFile(logPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		// Redirect fd 2 (stderr) so Go runtime panics land in the log too.
		// Dup2 is not available on Linux ARM64; Dup3 with flags=0 is equivalent.
		_ = syscall.Dup3(int(logFile.Fd()), 2, 0)
		defer logFile.Close()
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Error("PANIC: %v\n%s", r, debug.Stack())
		}
	}()

	logger.Info("itchio-pak %s starting", version)
	logger.Info("Git commit: %s", gitCommit)

	if *headless {
		logger.Info("headless mode: exiting cleanly")
		os.Exit(0)
	}

	runSDL()
}

// logFilePath returns the path for the log file.
// On device, NextUI sets PLATFORM (e.g. "tg5040") and logs are written to the
// conventional location used by other Paks:
//
//	/mnt/SDCARD/.userdata/<PLATFORM>/logs/itchio-pak.log
//
// When PLATFORM is unset (development / CI), it falls back to $HOME/itchio-pak.log.
func logFilePath() string {
	if platform := os.Getenv("PLATFORM"); platform != "" {
		return filepath.Join("/mnt/SDCARD/.userdata", platform, "logs", "itchio-pak.log")
	}
	return filepath.Join(os.Getenv("HOME"), "itchio-pak.log")
}

// readPlatform returns the PLATFORM env var, or "unknown" if unset.
func readPlatform() string {
	if p := os.Getenv("PLATFORM"); p != "" {
		return p
	}
	return "unknown"
}

// readNextUIVersion reads the first non-empty line of the NextUI version file.
// Returns "unknown" if the file is absent, empty, or unreadable — absence is
// expected when running outside NextUI (dev machine, other launchers).
func readNextUIVersion() string {
	data, err := os.ReadFile("/mnt/SDCARD/.system/version.txt")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "unknown"
}
