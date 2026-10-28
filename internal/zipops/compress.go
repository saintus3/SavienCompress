package zipops

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Options struct {
	Overwrite bool
	Progress  func(string)
}

func (o Options) report(name string) {
	if o.Progress != nil {
		o.Progress(name)
	}
}

// Compress stores directory contents relative to input, or a file by basename.
// The archive is published only after all data has been successfully written.
func Compress(input, output string, opts Options) error {
	info, err := os.Lstat(input)
	if err != nil {
		return fmt.Errorf("input: %w", err)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("input must be a regular file or directory")
	}
	outInfo, err := os.Lstat(output)
	if err == nil {
		if os.SameFile(info, outInfo) {
			return fmt.Errorf("input and output are the same file")
		}
		if !outInfo.Mode().IsRegular() {
			return fmt.Errorf("output must be a regular file")
		}
		if !opts.Overwrite {
			return fmt.Errorf("output exists; use --overwrite")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return fmt.Errorf("output directory: %w", err)
	}
	if info.IsDir() {
		source, err := filepath.EvalSymlinks(input)
		if err != nil {
			return err
		}
		source, err = filepath.Abs(source)
		if err != nil {
			return err
		}
		target, err := filepath.Abs(filepath.Join(parent, filepath.Base(output)))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, target)
		if err != nil {
			return err
		}
		if rel == "." || filepath.IsLocal(rel) {
			return fmt.Errorf("output must be outside the input directory")
		}
	}
	tmp, err := os.CreateTemp(parent, ".savien-*.zip")
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer os.Remove(tmp.Name())
	zw := zip.NewWriter(tmp)
	err = compressInput(zw, input, info, opts)
	zipErr := zw.Close()
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if zipErr != nil {
		return fmt.Errorf("finish ZIP: %w", zipErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if opts.Overwrite {
		err = os.Rename(tmp.Name(), output)
	} else {
		// Linking provides atomic no-clobber publication, unlike stat + rename.
		err = os.Link(tmp.Name(), output)
	}
	if err != nil {
		return fmt.Errorf("publish output: %w", err)
	}
	return nil
}

func compressInput(zw *zip.Writer, input string, info fs.FileInfo, opts Options) error {
	rootPath, start := input, "."
	if !info.IsDir() {
		rootPath, start = filepath.Dir(input), filepath.Base(input)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	return fs.WalkDir(root.FS(), start, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type: %q", name)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		opts.report(header.Name)
		if info.IsDir() {
			return nil
		}
		file, err := root.Open(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
