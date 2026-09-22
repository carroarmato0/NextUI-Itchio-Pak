package main

import (
	"bytes"
	"os"
	"strings"
)

const (
	logMaxBytes = 10 * 1024 * 1024 // 10 MB
	logMaxRuns  = 5                 // total runs to keep (newest N-1 old + the one about to start)
)

// rotateLog trims the log file at path before a new run begins so that at most
// (logMaxRuns-1) prior runs are retained and the file stays under logMaxBytes.
// A "run" starts at the line emitted by: logger.Info("itchio %s starting", version).
// The file is left unchanged if it does not exist, is empty, or needs no trimming.
func rotateLog(path string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}

	runs := splitLogRuns(data)

	// Keep at most logMaxRuns-1 prior runs; the current run is about to be appended.
	if len(runs) > logMaxRuns-1 {
		runs = runs[len(runs)-(logMaxRuns-1):]
	}

	// Trim from the front if still over the size cap, but always keep at least one run.
	total := totalRunBytes(runs)
	for total > logMaxBytes && len(runs) > 1 {
		total -= len(runs[0])
		runs = runs[1:]
	}

	trimmed := bytes.Join(runs, nil)
	if bytes.Equal(trimmed, data) {
		return
	}
	_ = os.WriteFile(path, trimmed, 0644)
}

// splitLogRuns splits log data into per-run chunks. Each chunk begins at the
// startup sentinel line. Content before the first sentinel is its own chunk.
func splitLogRuns(data []byte) [][]byte {
	lines := bytes.SplitAfter(data, []byte("\n"))
	var runs [][]byte
	var current []byte
	for _, line := range lines {
		if isRunStartLine(string(line)) && len(current) > 0 {
			runs = append(runs, current)
			current = append([]byte(nil), line...)
		} else {
			current = append(current, line...)
		}
	}
	if len(current) > 0 {
		runs = append(runs, current)
	}
	return runs
}

// isRunStartLine reports whether line is the startup sentinel written by main.
// Matching on "itchio" rather than the full binary name keeps logs written by
// pre-rename builds ("itchio-pak ... starting") splitting into runs correctly.
func isRunStartLine(line string) bool {
	return strings.Contains(line, "itchio") && strings.Contains(line, "starting")
}

func totalRunBytes(runs [][]byte) int {
	n := 0
	for _, r := range runs {
		n += len(r)
	}
	return n
}
