package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/saintus3/SavienCompress/internal/zipops"
)

const usage = `Usage: savien <command> [options]

Commands:
  compress <input> -o <output.zip> [--overwrite] [--verbose]
  extract <archive.zip> -o <dir> [--overwrite] [--verbose]
  list <archive.zip> [--verbose]

Use savien <command> --help for command help.
Exit codes: 0 success, 1 operation failed, 2 invalid arguments.
`

// Run executes a command without exiting the process.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	command := args[0]
	if command != "compress" && command != "extract" && command != "list" {
		fmt.Fprintf(stderr, "savien: unknown command %q\n%s", command, usage)
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	verbose := fs.Bool("verbose", false, "show progress on stderr (list: include ratio)")
	var output string
	var overwrite bool
	if command != "list" {
		fs.StringVar(&output, "o", "", "required output path")
		fs.BoolVar(&overwrite, "overwrite", false, "replace existing regular files")
	}
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: savien %s ", command)
		switch command {
		case "compress":
			fmt.Fprintln(fs.Output(), "<input> -o <output.zip> [--overwrite] [--verbose]")
		case "extract":
			fmt.Fprintln(fs.Output(), "<archive.zip> -o <dir> [--overwrite] [--verbose]")
		default:
			fmt.Fprintln(fs.Output(), "<archive.zip> [--verbose]")
		}
		fs.PrintDefaults()
	}
	normalized, err := interspersed(fs, args[1:])
	if err == nil {
		err = fs.Parse(normalized)
	}
	if errors.Is(err, flag.ErrHelp) {
		fs.SetOutput(stdout)
		fs.Usage()
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "savien: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 || (command != "list" && output == "") {
		fmt.Fprintln(stderr, "savien: expected exactly one input and, for compress/extract, -o <output>")
		fs.Usage()
		return 2
	}
	opts := zipops.Options{Overwrite: overwrite}
	if *verbose {
		opts.Progress = func(name string) { fmt.Fprintf(stderr, "%s: %q\n", command, name) }
	}
	switch command {
	case "compress":
		err = zipops.Compress(fs.Arg(0), output, opts)
	case "extract":
		err = zipops.Extract(fs.Arg(0), output, opts)
	case "list":
		err = zipops.List(fs.Arg(0), stdout, *verbose)
	}
	if err != nil {
		fmt.Fprintf(stderr, "savien: %s: %v\n", command, err)
		return 1
	}
	return 0
}

// The standard flag parser stops at the first positional argument. Reorder
// options so the documented input-before-options syntax also works.
func interspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var options, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		if name == "h" || name == "help" {
			return nil, flag.ErrHelp
		}
		f := fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("unknown option %q", arg)
		}
		options = append(options, arg)
		boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
		if !hasValue && !(ok && boolean.IsBoolFlag()) {
			i++
			if i == len(args) {
				return nil, fmt.Errorf("option %s needs a value", arg)
			}
			options = append(options, args[i])
		}
	}
	return append(append(options, "--"), positional...), nil
}
