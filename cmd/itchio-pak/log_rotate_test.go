package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// runContent builds a single log run starting with the startup sentinel line.
func runContent(version string, extraLines ...string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "[INFO]  itchio-pak %s starting\n", version)
	for _, line := range extraLines {
		fmt.Fprintln(&b, line)
	}
	return b.Bytes()
}

// writeTempLog writes content to a temp file and returns its path.
func writeTempLog(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "itchio-pak-*.log")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestRotateLog_NonexistentFile(t *testing.T) {
	// Must not panic or error on a file that does not exist yet.
	rotateLog(t.TempDir() + "/nonexistent.log")
}

func TestRotateLog_EmptyFile(t *testing.T) {
	path := writeTempLog(t, nil)
	rotateLog(path)
	got, _ := os.ReadFile(path)
	if len(got) != 0 {
		t.Errorf("expected empty file after rotating empty log, got %d bytes", len(got))
	}
}

func TestRotateLog_UnderLimits_NoChange(t *testing.T) {
	// Three small runs — under both the run count and size limits.
	content := bytes.Join([][]byte{
		runContent("v1.0.1", "line a"),
		runContent("v1.0.2", "line b"),
		runContent("v1.0.3", "line c"),
	}, nil)
	path := writeTempLog(t, content)
	rotateLog(path)
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, content) {
		t.Error("file should be unchanged when under both limits")
	}
}

func TestRotateLog_ExactlyMaxRunsMinus1_NoChange(t *testing.T) {
	// Exactly logMaxRuns-1 (4) prior runs — no trimming needed.
	var parts [][]byte
	for i := 1; i <= 4; i++ {
		parts = append(parts, runContent(fmt.Sprintf("v1.0.%d", i)))
	}
	content := bytes.Join(parts, nil)
	path := writeTempLog(t, content)
	rotateLog(path)
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, content) {
		t.Error("file should be unchanged with exactly logMaxRuns-1 runs")
	}
}

func TestRotateLog_TrimsOldestRunsWhenOverCount(t *testing.T) {
	// 6 runs — should keep newest 4 (logMaxRuns-1), dropping v1.0.1 and v1.0.2.
	var parts [][]byte
	for i := 1; i <= 6; i++ {
		parts = append(parts, runContent(fmt.Sprintf("v1.0.%d", i)))
	}
	path := writeTempLog(t, bytes.Join(parts, nil))
	rotateLog(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, old := range []string{"v1.0.1", "v1.0.2"} {
		if strings.Contains(s, old) {
			t.Errorf("old run %s should have been trimmed", old)
		}
	}
	for _, keep := range []string{"v1.0.3", "v1.0.4", "v1.0.5", "v1.0.6"} {
		if !strings.Contains(s, keep) {
			t.Errorf("run %s should have been retained", keep)
		}
	}
}

func TestRotateLog_SizeCapTrimsOldestRuns(t *testing.T) {
	// 4 runs × ~3 MB each = ~12 MB total, over the 10 MB cap.
	// After trimming the oldest run: 3 × ~3 MB ≈ 9 MB — within cap.
	bigLine := strings.Repeat("x", 3*1024*1024)
	var parts [][]byte
	for i := 1; i <= 4; i++ {
		parts = append(parts, runContent(fmt.Sprintf("v1.0.%d", i), bigLine))
	}
	path := writeTempLog(t, bytes.Join(parts, nil))
	rotateLog(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "v1.0.1") {
		t.Error("oldest run should have been trimmed by the size cap")
	}
	for _, keep := range []string{"v1.0.2", "v1.0.3", "v1.0.4"} {
		if !strings.Contains(s, keep) {
			t.Errorf("run %s should have been retained", keep)
		}
	}
}

func TestRotateLog_SizeCapKeepsAtLeastOneRun(t *testing.T) {
	// A single run larger than 10 MB must be kept — never discard the only context.
	bigLine := strings.Repeat("x", 11*1024*1024)
	content := runContent("v1.0.1", bigLine)
	path := writeTempLog(t, content)
	rotateLog(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "v1.0.1") {
		t.Error("single run must be kept even when it exceeds the size cap")
	}
}

func TestSplitLogRuns_NoSentinel(t *testing.T) {
	// Content with no startup sentinel should come back as one chunk.
	data := []byte("some random log lines\nno sentinel here\n")
	runs := splitLogRuns(data)
	if len(runs) != 1 {
		t.Errorf("expected 1 run for content with no sentinel, got %d", len(runs))
	}
	if !bytes.Equal(runs[0], data) {
		t.Error("single run should equal the original data")
	}
}

func TestSplitLogRuns_MultipleSentinels(t *testing.T) {
	chunks := [][]byte{
		runContent("v1.0.1", "a", "b"),
		runContent("v1.0.2", "c"),
		runContent("v1.0.3"),
	}
	runs := splitLogRuns(bytes.Join(chunks, nil))
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	for i, v := range []string{"v1.0.1", "v1.0.2", "v1.0.3"} {
		if !strings.Contains(string(runs[i]), v) {
			t.Errorf("run[%d] should contain %s", i, v)
		}
	}
}

func TestSplitLogRuns_PreservesContent(t *testing.T) {
	// Joining the split runs back together must reproduce the original bytes exactly.
	data := bytes.Join([][]byte{
		runContent("v1.0.1", "alpha"),
		runContent("v1.0.2", "beta"),
	}, nil)
	runs := splitLogRuns(data)
	if got := bytes.Join(runs, nil); !bytes.Equal(got, data) {
		t.Error("joining split runs should reproduce the original content exactly")
	}
}

func TestIsRunStartLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"[INFO]  itchio-pak v1.0.9 starting\n", true},
		{"[INFO]  itchio-pak v1.0.10-rc1 starting\n", true},
		{"[INFO]  itchio-pak dev starting\n", true},
		{"[INFO]  platform=tg5040 nextui=v2.0\n", false},
		{"[ERROR] something went wrong\n", false},
		{"[INFO]  Git commit: abc1234\n", false},
		{"", false},
	}
	for _, c := range cases {
		got := isRunStartLine(c.line)
		if got != c.want {
			t.Errorf("isRunStartLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}
