package main

import (
	"flag"
	"log"
	"os"
	"runtime/debug"
	"syscall"
)

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logFile, err := os.OpenFile(os.Getenv("HOME")+"/itchio-pak.log",
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

	if *headless {
		log.Println("headless mode: exiting cleanly")
		os.Exit(0)
	}

	log.Println("starting SDL run")
	runSDL()
	log.Println("SDL run finished")
}
