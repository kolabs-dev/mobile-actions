// cmd/verify-checksums/main.go
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	file := flag.String("file", "", "path to checksums file (relative to CWD)")
	dir := flag.String("dir", ".", "directory containing the files to verify")
	flag.Parse()

	if *file == "" {
		fmt.Fprintln(os.Stderr, "error: --file is required")
		os.Exit(1)
	}

	if err := verify(*file, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("all checksums verified")
}

func verify(checksumsFile, dir string) error {
	f, err := os.Open(checksumsFile)
	if err != nil {
		return fmt.Errorf("open checksums file: %w", err)
	}
	defer f.Close()

	var failures []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// format: "<hash>  <filename>" (two spaces)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed checksums line: %q", line)
		}
		expectedHash, filename := parts[0], strings.TrimSpace(parts[1])
		filePath := filepath.Join(dir, filename)

		actualHash, err := sha256File(filePath)
		if err != nil {
			failures = append(failures, fmt.Sprintf("  %s: %v", filename, err))
			continue
		}
		if actualHash != expectedHash {
			failures = append(failures, fmt.Sprintf("  %s: hash mismatch (got %s, want %s)", filename, actualHash, expectedHash))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums file: %w", err)
	}

	if len(failures) > 0 {
		return fmt.Errorf("checksum verification failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
