package secrets

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// DecodeToFile decodes a base64 string and writes it to a temp file in
// $RUNNER_TEMP/mobile-actions/<pattern>. Returns the file path, a cleanup
// function that removes the file, and any error.
func DecodeToFile(encoded, pattern string) (string, func(), error) {
	runnerTemp := os.Getenv("RUNNER_TEMP")
	if runnerTemp == "" {
		return "", nil, fmt.Errorf("RUNNER_TEMP environment variable is not set")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("base64 decode: %w", err)
	}

	dir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}

	cleanup := func() { os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}
