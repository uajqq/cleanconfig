package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/uajqq/cleanconfig/internal/formatter"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("cleanconfig", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var write bool
	var check bool
	var diff bool
	var stdinFilepath string
	var encoding string
	var indentWidth int
	var alignEquals bool
	var quoteStyle string
	var quotePolicy string
	var noFinalNewline bool
	var showVersion bool

	flags.BoolVar(&write, "write", false, "rewrite files in place")
	flags.BoolVar(&write, "w", false, "rewrite files in place")
	flags.BoolVar(&check, "check", false, "exit with status 1 if any file would change")
	flags.BoolVar(&check, "c", false, "exit with status 1 if any file would change")
	flags.BoolVar(&diff, "diff", false, "print a unified diff instead of formatted output")
	flags.StringVar(&stdinFilepath, "stdin-filepath", "", "path label to use when reading stdin; accepted for editor integrations")
	flags.StringVar(&encoding, "encoding", "utf-8", "file encoding for reading and writing files (only utf-8 is supported)")
	flags.IntVar(&indentWidth, "indent-width", 4, "spaces per ConfigObj section level")
	flags.BoolVar(&alignEquals, "align-equals", false, "align equals signs in contiguous assignment blocks")
	flags.StringVar(&quoteStyle, "quote-style", "double", "quote style for quoted values: double, single, auto, or preserve")
	flags.StringVar(&quotePolicy, "quote-policy", "existing", "quote existing quoted values only, or all value items: existing or all")
	flags.BoolVar(&noFinalNewline, "no-final-newline", false, "do not force a trailing newline")
	flags.BoolVar(&showVersion, "version", false, "print version and exit")

	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: cleanconfig [options] [FILE ...]")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Format ConfigObj/WeeWX .conf files. Omit FILE or use '-' to read stdin.")
		fmt.Fprintln(flags.Output())
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if !isUTF8Encoding(encoding) {
		fmt.Fprintf(stderr, "cleanconfig: unsupported encoding %q; only utf-8 is supported\n", encoding)
		return 2
	}

	opts := formatter.Options{
		IndentWidth:  indentWidth,
		AlignEquals:  alignEquals,
		QuoteStyle:   formatter.QuoteStyle(quoteStyle),
		QuotePolicy:  formatter.QuotePolicy(quotePolicy),
		FinalNewline: !noFinalNewline,
	}
	if err := opts.Validate(); err != nil {
		fmt.Fprintf(stderr, "cleanconfig: %v\n", err)
		return 2
	}

	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	if write {
		for _, path := range paths {
			if path == "-" {
				fmt.Fprintln(stderr, "cleanconfig: --write cannot be used with stdin")
				return 2
			}
		}
	}
	if len(paths) > 1 && !write && !check && !diff {
		fmt.Fprintln(stderr, "cleanconfig: multiple files require --write, --check, or --diff")
		return 2
	}

	changed := false
	hadError := false
	for _, path := range paths {
		label := path
		if path == "-" && stdinFilepath != "" {
			label = stdinFilepath
		}

		original, mode, err := readInput(path, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "cleanconfig: %s: %v\n", label, err)
			hadError = true
			continue
		}

		formatted, err := formatter.FormatConfig(original, opts)
		if err != nil {
			fmt.Fprintf(stderr, "cleanconfig: %s: %v\n", label, err)
			hadError = true
			continue
		}

		fileChanged := formatted != original
		if fileChanged {
			changed = true
		}

		switch {
		case check:
			if fileChanged {
				fmt.Fprintf(stderr, "would reformat %s\n", label)
			}
		case diff:
			if fileChanged {
				fmt.Fprint(stdout, unifiedDiff(original, formatted, label))
			}
		case write:
			if err := os.WriteFile(path, []byte(formatted), mode); err != nil {
				fmt.Fprintf(stderr, "cleanconfig: %s: %v\n", label, err)
				hadError = true
			}
		default:
			fmt.Fprint(stdout, formatted)
		}
	}

	if hadError {
		return 2
	}
	if check && changed {
		return 1
	}
	return 0
}

func readInput(path string, stdin io.Reader) (string, os.FileMode, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		return string(data), 0o644, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	data, err := os.ReadFile(path)
	return string(data), info.Mode().Perm(), err
}

func isUTF8Encoding(encoding string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(encoding, "_", "-"))
	return normalized == "utf-8" || normalized == "utf8"
}

func unifiedDiff(original string, formatted string, label string) string {
	var builder strings.Builder
	oldLines := splitDiffLines(original)
	newLines := splitDiffLines(formatted)

	fmt.Fprintf(&builder, "--- %s\n", label)
	fmt.Fprintf(&builder, "+++ %s\n", label)
	fmt.Fprintf(&builder, "@@ -1,%d +1,%d @@\n", countBodyLines(original), countBodyLines(formatted))
	for _, line := range oldLines {
		builder.WriteByte('-')
		writeDiffLine(&builder, line)
	}
	for _, line := range newLines {
		builder.WriteByte('+')
		writeDiffLine(&builder, line)
	}
	return builder.String()
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}

	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func writeDiffLine(builder *strings.Builder, line string) {
	builder.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		builder.WriteByte('\n')
	}
}

func countBodyLines(text string) int {
	if text == "" {
		return 0
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	count := strings.Count(normalized, "\n")
	if !strings.HasSuffix(normalized, "\n") {
		count++
	}
	return count
}
