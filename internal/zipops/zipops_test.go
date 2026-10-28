package zipops

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type testEntry struct {
	name string
	body string
	mode fs.FileMode
}

func writeZIP(t *testing.T, entries ...testEntry) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "input.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetModTime(time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC))
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		dst, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(dst, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func mustWrite(t *testing.T, name, data string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertContent(t *testing.T, name, want string) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil || string(data) != want {
		t.Fatalf("%s = %q, %v; want %q", name, data, err, want)
	}
}

func fileHash(t *testing.T, name string) [32]byte {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func TestRoundTrip(t *testing.T) {
	temp := t.TempDir()
	input := filepath.Join(temp, "source")
	if err := os.MkdirAll(filepath.Join(input, "empty-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	fixtures := os.DirFS("../../testdata/nested")
	if err := fs.WalkDir(fixtures, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(input, filepath.FromSlash(name))
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := fs.ReadFile(fixtures, name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(input, "empty"), "")
	large, err := os.Create(filepath.Join(input, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	block := bytes.Repeat([]byte("streaming-data\n"), 4096)
	for i := 0; i < 128; i++ {
		if _, err := large.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := large.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(temp, "result.zip")
	var progress []string
	opts := Options{Progress: func(name string) { progress = append(progress, name) }}
	if err := Compress(input, archive, opts); err != nil {
		t.Fatal(err)
	}
	if len(progress) == 0 {
		t.Fatal("no compression progress")
	}
	output := filepath.Join(temp, "output")
	if err := Extract(archive, output, opts); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(input, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(input, name)
		if err != nil {
			return err
		}
		target := filepath.Join(output, rel)
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		if entry.IsDir() != info.IsDir() {
			t.Fatalf("entry type mismatch: %s", rel)
		}
		if !entry.IsDir() && fileHash(t, name) != fileHash(t, target) {
			t.Fatalf("hash mismatch: %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCompressSingleAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	input, output := filepath.Join(dir, "hello.txt"), filepath.Join(dir, "out.zip")
	mustWrite(t, input, "hello")
	if err := Compress(input, output, Options{}); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "hello.txt" {
		t.Fatalf("unexpected entries: %v", reader.File)
	}
	reader.Close()
	before := fileHash(t, output)
	if err := Compress(input, output, Options{}); err == nil {
		t.Fatal("existing output accepted")
	}
	if fileHash(t, output) != before {
		t.Fatal("existing output modified")
	}
	mustWrite(t, input, "updated")
	if err := Compress(input, output, Options{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if before == fileHash(t, output) {
		t.Fatal("overwrite did not replace archive")
	}
	if err := Compress(input, input, Options{Overwrite: true}); err == nil {
		t.Fatal("input overwritten")
	}
	assertContent(t, input, "updated")
	if err := Compress(dir, filepath.Join(dir, "inside.zip"), Options{}); err == nil {
		t.Fatal("output inside source accepted")
	}
}

func TestEmptyDirectory(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "empty.zip")
	if err := Compress(t.TempDir(), archive, Options{}); err != nil {
		t.Fatal(err)
	}
	var listing bytes.Buffer
	if err := List(archive, &listing, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(listing.String(), "\n") != 1 {
		t.Fatalf("empty ZIP output: %q", listing.String())
	}
	if err := Extract(archive, filepath.Join(t.TempDir(), "out"), Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestEntryPath(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "a/../evil", "/absolute",
		`..\evil`, `a\..\evil`, `C:\evil`, "C:evil", "//server/share", "", ".", "./", "a\x00b"} {
		t.Run(name, func(t *testing.T) {
			if _, err := entryPath(name); err == nil {
				t.Fatalf("unsafe path accepted: %q", name)
			}
		})
	}
	for _, name := range []string{"file", "nested/file", "./nested/file", "한글.txt", "dir/"} {
		if _, err := entryPath(name); err != nil {
			t.Fatalf("safe path %q rejected: %v", name, err)
		}
	}
}

func TestExtractRejectsUnsafeEntriesBeforeWriting(t *testing.T) {
	for _, name := range []string{"../evil", "/evil", `..\evil`, "C:/evil", "nested/../../evil"} {
		t.Run(name, func(t *testing.T) {
			archive := writeZIP(t, testEntry{name: "safe", body: "safe"}, testEntry{name: name, body: "evil"})
			destination := filepath.Join(t.TempDir(), "out")
			if err := Extract(archive, destination, Options{}); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("output created before validation: %v", err)
			}
		})
	}
	for _, mode := range []fs.FileMode{fs.ModeSymlink | 0777, fs.ModeNamedPipe | 0600} {
		archive := writeZIP(t, testEntry{name: "special", body: "target", mode: mode})
		if err := Extract(archive, t.TempDir(), Options{}); err == nil {
			t.Fatalf("special mode %v accepted", mode)
		}
	}
}

func TestExtractOverwrite(t *testing.T) {
	archive := writeZIP(t, testEntry{name: "nested/"}, testEntry{name: "nested/file", body: "new"})
	destination := t.TempDir()
	if err := Extract(archive, destination, Options{}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(destination, "nested", "file")
	mustWrite(t, file, "old")
	if err := Extract(archive, destination, Options{}); err == nil {
		t.Fatal("existing file accepted")
	}
	assertContent(t, file, "old")
	if err := Extract(archive, destination, Options{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	assertContent(t, file, "new")
	if err := Extract(archive, file, Options{}); err == nil {
		t.Fatal("file accepted as destination directory")
	}
}

func TestExtractSymlinkAndHardlinkProtection(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	mustWrite(t, outside, "unchanged")
	for _, directoryLink := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directoryLink], func(t *testing.T) {
			destination := t.TempDir()
			target, name := outside, "link"
			if directoryLink {
				target, name = filepath.Dir(outside), "link/outside"
			}
			if err := os.Symlink(target, filepath.Join(destination, "link")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			archive := writeZIP(t, testEntry{name: name, body: "changed"})
			if err := Extract(archive, destination, Options{Overwrite: true}); err == nil {
				t.Fatal("symlink accepted")
			}
			assertContent(t, outside, "unchanged")
		})
	}
	t.Run("hardlink", func(t *testing.T) {
		destination := t.TempDir()
		if err := os.Link(outside, filepath.Join(destination, "linked")); err != nil {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		archive := writeZIP(t, testEntry{name: "linked", body: "changed"})
		if err := Extract(archive, destination, Options{Overwrite: true}); err != nil {
			t.Fatal(err)
		}
		assertContent(t, outside, "unchanged")
		assertContent(t, filepath.Join(destination, "linked"), "changed")
	})
	t.Run("destination-symlink", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(t.TempDir(), link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := Extract(writeZIP(t, testEntry{name: "file"}), link, Options{}); err == nil {
			t.Fatal("symlink destination accepted")
		}
	})
}

func TestExtractCannotOverwriteArchive(t *testing.T) {
	archive := writeZIP(t, testEntry{name: "input.zip", body: "destroyed"})
	before := fileHash(t, archive)
	if err := Extract(archive, filepath.Dir(archive), Options{Overwrite: true}); err == nil {
		t.Fatal("input archive overwritten")
	}
	if before != fileHash(t, archive) {
		t.Fatal("input archive changed")
	}
}

func TestCompressFailurePreservesOutput(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(source, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	output := filepath.Join(t.TempDir(), "existing.zip")
	mustWrite(t, output, "original")
	if err := Compress(source, output, Options{Overwrite: true}); err == nil {
		t.Fatal("symlink input accepted")
	}
	assertContent(t, output, "original")
	files, err := filepath.Glob(filepath.Join(filepath.Dir(output), ".savien-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary archives leaked: %v, %v", files, err)
	}
}

func TestCompressRejectsNonportableNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot create these source filenames")
	}
	for _, name := range []string{"report:2026.txt", `back\slash.txt`} {
		t.Run(name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), name)
			mustWrite(t, input, "contents")
			output := filepath.Join(t.TempDir(), "out.zip")
			if err := Compress(input, output, Options{}); err == nil {
				t.Fatal("created archive with an unsupported entry name")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failed archive published: %v", err)
			}
		})
	}
}

func TestWithinDirectory(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	for _, tt := range []struct {
		target string
		want   bool
	}{
		{source, true},
		{filepath.Join(source, "out.zip"), true},
		{source + "-sibling", false},
		{filepath.Join(filepath.Dir(source), "out.zip"), false},
	} {
		got, err := withinDirectory(source, tt.target)
		if err != nil || got != tt.want {
			t.Fatalf("withinDirectory(%q, %q) = %v, %v; want %v", source, tt.target, got, err, tt.want)
		}
	}
	if runtime.GOOS == "windows" {
		got, err := withinDirectory(`C:\data`, `D:\backup.zip`)
		if err != nil || got {
			t.Fatalf("different drive treated as inside source: %v, %v", got, err)
		}
	}
}

func TestCorruptZIP(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.zip")
	mustWrite(t, bad, "not a ZIP")
	if err := Extract(bad, t.TempDir(), Options{}); err == nil || !strings.Contains(err.Error(), "ZIP") {
		t.Fatalf("expected clear ZIP error: %v", err)
	}
	if err := List(bad, io.Discard, false); err == nil {
		t.Fatal("invalid ZIP listed")
	}
	archive := writeZIP(t, testEntry{name: "data", body: "payload"})
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 0xff
	if err := os.WriteFile(archive, data, 0644); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := Extract(archive, destination, Options{}); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("expected checksum failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "data")); !os.IsNotExist(err) {
		t.Fatalf("corrupt output not removed: %v", err)
	}
}

func TestList(t *testing.T) {
	archive := writeZIP(t, testEntry{name: "한글\t\n\x1b.txt", body: "hello"}, testEntry{name: "empty"})
	for _, verbose := range []bool{false, true} {
		var output bytes.Buffer
		if err := List(archive, &output, verbose); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		if len(lines) != 3 {
			t.Fatalf("entry names changed row structure: %q", output.String())
		}
		columns := strings.Split(lines[1], "\t")
		want := []string{`"한글\t\n\x1b.txt"`, "5", "5", "2026-01-02T03:04:06Z"}
		if verbose {
			want = append(want, "0.0%")
		}
		if !reflect.DeepEqual(columns, want) {
			t.Fatalf("columns = %q, want %q", columns, want)
		}
		if strings.Contains(output.String(), "NaN") || strings.Contains(output.String(), "Inf") {
			t.Fatal("invalid empty-entry ratio")
		}
	}
	if err := List(archive, failingWriter{}, false); err == nil {
		t.Fatal("output write failure ignored")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
