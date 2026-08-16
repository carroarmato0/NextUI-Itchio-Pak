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
	"sort"
	"strings"
	"syscall"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
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
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to `file`")
	memProfile := flag.String("memprofile", "", "write memory profile to `file` on exit")
	pprofAddr := flag.String("pprof", "", "start pprof HTTP server on `addr` (e.g. :6060)")
	flag.Parse()

	// Resolve the firmware before anything touches a device path: the log
	// location, ROM destinations and available features all hang off this.
	env := firmware.Detect()
	firmware.SetActive(env)

	logPath := env.LogPath()
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

	// Apply LOG_LEVEL before the startup banner. runSDL applies the configured
	// level and then this variable again, so precedence is unchanged; without
	// it, everything logged before the UI starts is stuck at the default level.
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logger.SetLevel(logger.LevelFromString(envLevel))
	}

	logger.Info("itchio %s starting", version)
	logger.Info("git commit: %s", gitCommit)
	logger.Info("firmware:   %s", env.Kind())
	logger.Info("device:     %s (%s)", deviceOrUnknown(env.Device()), env.DeviceLabel())
	// H700 is one PLATFORM across eleven SKUs, so without these two a bug
	// report cannot be told apart from ten other handhelds. RGXX_MODEL is the
	// stock firmware's own name for the board and is display-only upstream.
	if sku := os.Getenv("DEVICE"); sku != "" {
		// RGXX_MODEL is only set by Anbernic's stock firmware; other NextUI
		// platforms export DEVICE too, so printing "(stock model: unknown)"
		// unconditionally would put a meaningless parenthetical on every
		// TrimUI log line. Only show it when there is a real value to show.
		if model := os.Getenv("RGXX_MODEL"); model != "" {
			logger.Info("device sku: %s (stock model: %s)", sku, model)
		} else {
			logger.Info("device sku: %s", sku)
		}
	}
	logger.Info("fw version: %s", env.FirmwareVersion())
	logger.Info("storage:    root=%s data=%s", env.Root(), env.DataDir())
	// launch.sh silently picks a native SDL directory and a bundled one with no
	// log line of its own; this is the one place that records what it resolved.
	logger.Info("ld_library_path: %s", os.Getenv("LD_LIBRARY_PATH"))
	logRomDirs(env)
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

// logRomDirs records where each system's ROMs will be written. On firmware that
// discovers these by scanning the card rather than fixing them in advance, this
// is the line that turns "my game went missing" into a one-look diagnosis.
func logRomDirs(env *firmware.Env) {
	dirs := env.ROMDirs()
	keys := make([]string, 0, len(dirs))
	for k := range dirs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if dirs[k] == "" {
			continue
		}
		// The art destination is logged beside the ROM destination because it is
		// derived separately and can be wrong on its own: muOS files box art
		// under different system names than it displays, so a mismatch here
		// shows up as silently missing artwork and nothing else.
		logger.Debug("roms: %-10s -> %s", k, dirs[k])
		logger.Debug("art:  %-10s -> %s", k, env.CoverArtDirFor(dirs[k]))
	}
}

// deviceOrUnknown keeps the startup log readable when the firmware could not
// name the hardware, which is the normal case on a development machine.
func deviceOrUnknown(device string) string {
	if device == "" {
		return "unknown"
	}
	return device
}
