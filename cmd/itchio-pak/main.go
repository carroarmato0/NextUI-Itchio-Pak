package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
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
	headless    := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	cpuProfile  := flag.String("cpuprofile", "", "write CPU profile to `file`")
	memProfile  := flag.String("memprofile", "", "write memory profile to `file` on exit")
	pprofAddr   := flag.String("pprof", "", "start pprof HTTP server on `addr` (e.g. :6060)")
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
	logger.Info("git commit: %s", gitCommit)
	p := readPlatform()
	logger.Info("platform:   %s (%s)", p, platformDescription(p))
	logger.Info("nextui:     %s", readNextUIVersion())
	profilingDesc := "off"
	if *cpuProfile != "" || *memProfile != "" || *pprofAddr != "" {
		var parts []string
		if *cpuProfile != "" {
			parts = append(parts, "cpu="+*cpuProfile)
		}
		if *memProfile != "" {
			parts = append(parts, "mem="+*memProfile)
		}
		if *pprofAddr != "" {
			parts = append(parts, "pprof="+*pprofAddr)
		}
		profilingDesc = strings.Join(parts, " ")
	}
	logger.Info("profiling:  %s", profilingDesc)

	if *pprofAddr != "" {
		go func() {
			logger.Info("pprof: listening on %s", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				logger.Error("pprof: %v", err)
			}
		}()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			logger.Error("cpuprofile: %v", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			logger.Error("cpuprofile start: %v", err)
			f.Close()
			os.Exit(1)
		}
		logger.Info("cpuprofile: writing to %s", *cpuProfile)
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
			logger.Info("cpuprofile: written to %s", *cpuProfile)
		}()
	}

	// When profiling to files, install a signal handler so that SIGTERM/SIGINT/
	// SIGHUP (e.g. Ctrl-C in the terminal, adb disconnect, or NextUI killing the
	// process) still flushes profiles before exit. Without this, Go's deferred
	// cleanup is skipped and the profile files are never written.
	if *cpuProfile != "" || *memProfile != "" {
		cpuProf := *cpuProfile
		memProf := *memProfile
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		go func() {
			sig := <-sigCh
			logger.Info("profiling: caught signal %v — flushing profiles", sig)
			pprof.StopCPUProfile() // no-op if not started; flushes the file
			if cpuProf != "" {
				logger.Info("cpuprofile: written to %s", cpuProf)
			}
			if memProf != "" {
				if f, err := os.Create(memProf); err == nil {
					pprof.WriteHeapProfile(f)
					f.Close()
					logger.Info("memprofile: written to %s", memProf)
				} else {
					logger.Error("memprofile: %v", err)
				}
			}
			os.Exit(0)
		}()
	}

	if *headless {
		logger.Info("headless mode: exiting cleanly")
		os.Exit(0)
	}

	runSDL()

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			logger.Error("memprofile: %v", err)
		} else {
			defer f.Close()
			if err := pprof.WriteHeapProfile(f); err != nil {
				logger.Error("memprofile write: %v", err)
			} else {
				logger.Info("memprofile: written to %s", *memProfile)
			}
		}
	}
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

// platformDescription returns a human-readable device name for a NextUI platform code.
func platformDescription(platform string) string {
	switch platform {
	case "tg5040":
		return "TrimUI Brick / Smart Pro"
	case "tg5050":
		return "TrimUI Smart Pro S"
	case "my355":
		return "Miyoo Flip"
	default:
		return "unknown device"
	}
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
