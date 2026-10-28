package zipops

import (
	"archive/zip"
	"fmt"
	"io"
	"strconv"
	"time"
)

// List emits TSV with quoted paths so tabs, newlines and terminal controls
// inside entry names cannot change the output structure.
func List(archive string, output io.Writer, verbose bool) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open ZIP: %w", err)
	}
	defer zr.Close()
	header := "PATH\tSIZE\tCOMPRESSED\tMODIFIED"
	if verbose {
		header += "\tSAVED"
	}
	if _, err := fmt.Fprintln(output, header); err != nil {
		return err
	}
	for _, entry := range zr.File {
		line := fmt.Sprintf("%s\t%d\t%d\t%s", strconv.Quote(entry.Name),
			entry.UncompressedSize64, entry.CompressedSize64, entry.Modified.UTC().Format(time.RFC3339))
		if verbose {
			ratio := 0.0
			if entry.UncompressedSize64 > 0 {
				ratio = 100 * (1 - float64(entry.CompressedSize64)/float64(entry.UncompressedSize64))
			}
			line += fmt.Sprintf("\t%.1f%%", ratio)
		}
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}
