// cmd/verify-checksums/main_test.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSha256File_Success(t *testing.T) {
	content := []byte("hello world")
	f, err := os.CreateTemp(t.TempDir(), "*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	f.Close()

	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])

	got, err := sha256File(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestSha256File_Missing(t *testing.T) {
	_, err := sha256File(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestVerify_AllMatch(t *testing.T) {
	dir := t.TempDir()
	content1 := []byte("file one content")
	content2 := []byte("file two content")
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), content1, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), content2, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	h1 := sha256.Sum256(content1)
	h2 := sha256.Sum256(content2)
	checksums := fmt.Sprintf("%s  a.bin\n%s  b.bin\n",
		hex.EncodeToString(h1[:]), hex.EncodeToString(h2[:]))
	checkFile := filepath.Join(dir, "sums.txt")
	if err := os.WriteFile(checkFile, []byte(checksums), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := verify(checkFile, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("real content"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	checkFile := filepath.Join(dir, "sums.txt")
	if err := os.WriteFile(checkFile, []byte(wrong+"  a.bin\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected 'hash mismatch' in error, got: %v", err)
	}
}

func TestVerify_MissingFile(t *testing.T) {
	dir := t.TempDir()
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	checkFile := filepath.Join(dir, "sums.txt")
	if err := os.WriteFile(checkFile, []byte(wrong+"  missing.bin\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVerify_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	checkFile := filepath.Join(dir, "sums.txt")
	// Single space instead of two — malformed
	if err := os.WriteFile(checkFile, []byte("abc123 single-space.bin\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	err := verify(checkFile, dir)
	if err == nil {
		t.Fatal("expected error for malformed line, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected 'malformed' in error, got: %v", err)
	}
}

func TestVerify_EmptyLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	checkFile := filepath.Join(dir, "sums.txt")
	if err := os.WriteFile(checkFile, []byte("\n\n\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := verify(checkFile, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_MissingChecksumsFile(t *testing.T) {
	err := verify(filepath.Join(t.TempDir(), "no-such-file.txt"), t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing checksums file, got nil")
	}
}
