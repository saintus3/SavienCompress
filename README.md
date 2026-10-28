# SavienCompress

`savien` is a small, streaming ZIP CLI written in Go using only the standard
library. It compresses files/directories, safely extracts archives, and lists
archive contents.

## Install / build

Requires Go **1.24.13 or newer** (uses `os.Root` for filesystem confinement).

```sh
go install github.com/saintus3/SavienCompress/cmd/savien@latest
# Or, from a checkout:
go build -o savien ./cmd/savien
./savien --help
```

`go install` places the binary in `GOBIN`, or `$(go env GOPATH)/bin` by default;
add that directory to your `PATH`. On Windows, build with `-o savien.exe`.

## Usage

```sh
savien compress <input> -o <output.zip> [--overwrite] [--verbose]
savien extract <archive.zip> -o <dir> [--overwrite] [--verbose]
savien list <archive.zip> [--verbose]

savien compress report.txt -o report.zip
savien compress ./photos -o photos.zip
savien list photos.zip --verbose
savien extract photos.zip -o ./restored
savien extract photos.zip -o ./restored --overwrite
```

Options may appear before or after the input. Use `--` before positional paths
beginning with `-`, for example `savien compress -o backup.zip -- -notes.txt`.
Both `savien --help` and `savien <command> --help` print help.

- A single file is stored by basename. A directory stores its **contents**
  relative to that directory, including empty subdirectories, not its own name.
  An empty input directory produces a valid empty ZIP.
- Compression uses Deflate for files and streams data rather than buffering
  entire files. The output parent directory must already exist, and the output
  must be outside the input directory. Symlinks and special files are rejected.
  Source names must also satisfy the extraction path rules below, so generated
  archives can be read back by `savien`.
- Compression refuses existing output unless `--overwrite` is supplied.
  A temporary archive is completed before publishing it; failed compression
  leaves an existing archive untouched. No-overwrite publication uses a hard
  link, so the output filesystem must support hard links.
- Extraction creates the destination and missing subdirectories. Existing
  directories are reused; existing files cause an error unless `--overwrite`
  is supplied. Symlinks, special files, and overwriting the input archive are
  rejected. Overwrite replaces files rather than modifying hard-linked targets.
- `--verbose` sends per-entry progress to stderr for compress/extract. Successful
  non-verbose compress/extract commands are silent.

Exit codes: **0** success (including help), **1** filesystem/archive/operation
error, **2** invalid command or arguments. Errors go to stderr.

### List format

Output is tab-separated with one header and one line per entry:

```text
PATH	SIZE	COMPRESSED	MODIFIED
"sub/report.txt"	1024	256	2026-01-02T03:04:06Z
```

Paths are Go-quoted strings (including quotes); control characters are escaped,
so embedded tabs/newlines or terminal escape sequences cannot alter the table.
Sizes are decimal bytes; timestamps are UTC RFC3339. `--verbose` appends a
`SAVED` column: `100 * (1 - compressed / size)`, or `0.0%` for zero-size entries.
Negative savings are possible for small files. An empty ZIP prints the header
only. Listing reads ZIP metadata; extraction also verifies file checksums.

## Security and MVP limitations

- Entry paths are validated before extraction starts. Absolute paths, `..`
  components, backslashes, drive/colon paths, NULs, and unsupported entry types
  are rejected. `os.Root` additionally confines file operations to the opened
  destination, including when symlinks change concurrently. Existing symlink
  components are rejected; archives cannot create symlinks.
- Use a destination you control. Filesystem mounts/device files are not a
  sandbox boundary, and concurrent modification of the destination is not
  supported. Unix, macOS, and Windows are the intended platforms.
- Extraction creates files with mode `0644` and directories with `0755`, subject
  to the process umask. Archive permissions, ownership, and timestamps are **not**
  restored; compression records source modes/times and listing displays times.
- Extraction is not transactional: an error can leave earlier entries extracted.
  A failed file copy is removed; `--overwrite` can remove a previous file even
  if a later checksum/write error occurs. Use a fresh directory for untrusted
  archives. Duplicate file entries follow the same overwrite policy.
- There are no decompression size/ratio quotas, password/encryption support,
  split archives, or non-ZIP formats. Untrusted archives can consume disk/CPU;
  enforce external resource limits. ZIP metadata uses memory proportional to
  entry count; file contents are streamed.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l cmd internal integration
```

Tests cover argument handling, binary exit codes, file/directory round trips
with SHA-256 comparisons, nested/empty/non-ASCII fixtures, generated large data,
overwrite behavior, corrupt ZIPs, Zip Slip, and symlink/hardlink protections.

Pushes and pull requests run tests, vet, and formatting checks on Linux, macOS,
and Windows. Pushing a `v*` tag runs verification and builds Linux/macOS/Windows
amd64 and arm64 binaries. Download the `savien-<os>-<arch>` workflow artifact
from the **Release artifacts** Actions run; each includes `savien` (`savien.exe`
on Windows) and `SHA256SUMS`. This baseline uploads Actions artifacts, not a
published GitHub Release.

## Saved issue mapping

- #5: CLI scaffolding, commands, help, logging, and exit codes.
- #4: Streaming single-file and recursive-directory ZIP compression.
- #3: Rooted extraction, Zip Slip protection, overwrite and metadata policy.
- #2: Stable listing columns and verbose compression savings.
- #6: Unit/integration/security tests, fixtures, documentation, CI and tag builds.