package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"
)

// version is set at build time via -ldflags:
//
//	-X main.version=vX.Y.Z
var version = "dev"

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logPath := logFilePath()
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
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
			log.Printf("PANIC: %v\n%s", r, debug.Stack())
		}
	}()

	log.Printf("itchio-pak %s starting", version)

	if *headless {
		log.Println("headless mode: exiting cleanly")
		os.Exit(0)
	}

	log.Println("starting SDL run")
	runSDL()
	log.Println("SDL run finished")
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
