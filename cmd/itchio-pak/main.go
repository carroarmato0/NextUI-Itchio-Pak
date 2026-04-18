package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	headless := flag.Bool("headless", false, "skip SDL2 init (CI mode)")
	flag.Parse()

	logFile, err := os.OpenFile(os.Getenv("HOME")+"/itchio-pak.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	if *headless {
		log.Println("headless mode: exiting cleanly")
		os.Exit(0)
	}

	// SDL2 UI wired in Task 17
	log.Fatal("SDL2 not yet wired up")
}
