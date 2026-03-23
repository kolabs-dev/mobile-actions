package exec

import (
	"bytes"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
)

// Run executes a command, streaming stdout and stderr to os.Stdout/os.Stderr.
// Returns a wrapped error with the command name on failure.
func Run(name string, args ...string) error {
	cmd := osexec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// RunOutput executes a command and returns its combined stdout output as a string.
// stderr is still streamed to os.Stderr.
func RunOutput(name string, args ...string) (string, error) {
	var stdout bytes.Buffer
	cmd := osexec.Command(name, args...)
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
