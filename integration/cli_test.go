package integration

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "savien")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/savien")
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	run := func(want int, args ...string) (string, string) {
		t.Helper()
		command := exec.Command(binary, args...)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		code := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != want {
			t.Fatalf("%v: exit %d, want %d\n%s", args, code, want, &stderr)
		}
		return stdout.String(), stderr.String()
	}
	for _, args := range [][]string{{"--help"}, {"compress", "--help"}, {"extract", "--help"}, {"list", "--help"}} {
		stdout, stderr := run(0, args...)
		if !strings.Contains(stdout, "Usage: savien") || stderr != "" {
			t.Fatalf("help output: %q, %q", stdout, stderr)
		}
	}
	run(2, "compress", "missing")
	run(2, "list", "missing", "--unknown")
	run(1, "list", filepath.Join(temp, "missing.zip"))
	archive, destination := filepath.Join(temp, "out.zip"), filepath.Join(temp, "out")
	_, progress := run(0, "compress", "../testdata/nested", "-o", archive, "--verbose")
	if !strings.Contains(progress, "compress:") {
		t.Fatal("missing verbose progress")
	}
	run(1, "compress", "../testdata/nested", "-o", archive)
	run(0, "compress", "-o", archive, "--overwrite", "../testdata/nested")
	listing, _ := run(0, "list", archive, "--verbose")
	if !strings.Contains(listing, "PATH\tSIZE\tCOMPRESSED\tMODIFIED\tSAVED\n") ||
		!strings.Contains(listing, `"sub/한글.txt"`) {
		t.Fatalf("unexpected listing: %s", listing)
	}
	run(0, "extract", archive, "-o", destination)
	run(1, "extract", archive, "-o", destination)
	run(0, "extract", archive, "-o", destination, "--overwrite")
	expected, err := os.ReadFile("../testdata/nested/sub/한글.txt")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(destination, "sub", "한글.txt"))
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatalf("round-trip mismatch: %q, %v", actual, err)
	}
	malicious := filepath.Join(temp, "malicious.zip")
	file, err := os.Create(malicious)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escaped")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, stderr := run(1, "extract", malicious, "-o", destination)
	if !strings.Contains(stderr, "unsafe ZIP path") {
		t.Fatalf("missing security error: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(temp, "escaped")); !os.IsNotExist(err) {
		t.Fatal("ZIP traversal escaped destination")
	}
}
