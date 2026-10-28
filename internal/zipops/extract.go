package zipops

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// entryPath rejects nonportable paths as well as native traversal paths.
func entryPath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe ZIP path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe ZIP path %q", name)
		}
	}
	clean := path.Clean(name)
	local := filepath.FromSlash(clean)
	if clean == "." || !filepath.IsLocal(local) {
		return "", fmt.Errorf("unsafe ZIP path %q", name)
	}
	return local, nil
}

func Extract(archive, destination string, opts Options) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open ZIP: %w", err)
	}
	defer zr.Close()
	archiveInfo, err := os.Stat(archive)
	if err != nil {
		return err
	}
	// Validate the full central directory before creating any output.
	for _, entry := range zr.File {
		if _, err := entryPath(entry.Name); err != nil {
			return err
		}
		if !entry.Mode().IsRegular() && !entry.Mode().IsDir() {
			return fmt.Errorf("unsupported ZIP entry type: %q", entry.Name)
		}
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("destination must be a directory, not a symlink or file")
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	defer root.Close()
	for _, entry := range zr.File {
		name, err := entryPath(entry.Name)
		if err != nil {
			return err
		}
		dir := filepath.Dir(name)
		if entry.FileInfo().IsDir() {
			dir = name
		}
		if err := makeDirectories(root, dir); err != nil {
			return fmt.Errorf("entry %q: %w", entry.Name, err)
		}
		if !entry.FileInfo().IsDir() {
			if err := extractFile(root, name, entry, archiveInfo, opts.Overwrite); err != nil {
				return fmt.Errorf("entry %q: %w", entry.Name, err)
			}
		}
		opts.report(entry.Name)
	}
	return nil
}

func makeDirectories(root *os.Root, name string) error {
	if name == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(name, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := root.Mkdir(current, 0755); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("directory component is a symlink or file: %q", current)
		}
	}
	return nil
}

func extractFile(root *os.Root, name string, entry *zip.File, archiveInfo os.FileInfo, overwrite bool) error {
	info, err := root.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular file")
		}
		if os.SameFile(info, archiveInfo) {
			return fmt.Errorf("refusing to overwrite the input archive")
		}
		if !overwrite {
			return fmt.Errorf("file exists; use --overwrite")
		}
		// Unlink instead of truncating: existing hard links must not be modified.
		if err := root.Remove(name); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("read ZIP: %w", err)
	}
	defer src.Close()
	dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		root.Remove(name)
		return fmt.Errorf("read ZIP data: %w", copyErr)
	}
	if closeErr != nil {
		root.Remove(name)
		return closeErr
	}
	return nil
}
