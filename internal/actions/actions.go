package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var w io.Writer = os.Stdout

// SetWriter replaces the output writer (for testing).
func SetWriter(out io.Writer) { w = out }

// ResetWriter restores the default output writer.
func ResetWriter() { w = os.Stdout }

// SetOutput writes name=value to the $GITHUB_OUTPUT file.
func SetOutput(name, value string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return fmt.Errorf("GITHUB_OUTPUT environment variable is not set")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
	return err
}

// AddMask masks a value in the runner log.
func AddMask(value string) { fmt.Fprintf(w, "::add-mask::%s\n", value) }

// Group begins a collapsible log group.
func Group(name string) { fmt.Fprintf(w, "::group::%s\n", name) }

// EndGroup ends a collapsible log group.
func EndGroup() { fmt.Fprintf(w, "::endgroup::\n") }

// Error logs an error annotation.
func Error(msg string) { fmt.Fprintf(w, "::error::%s\n", msg) }

// Warning logs a warning annotation.
func Warning(msg string) { fmt.Fprintf(w, "::warning::%s\n", msg) }
