// cmd/android-build/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindArtifact_AAB(t *testing.T) {
	appPath := t.TempDir()
	dir := filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(dir, "app-release.aab")
	if err := os.WriteFile(expected, []byte("fake aab"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := findArtifact(appPath, "aab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Fatalf("got %s, want %s", got, expected)
	}
}

func TestFindArtifact_APK(t *testing.T) {
	appPath := t.TempDir()
	dir := filepath.Join(appPath, "app", "build", "outputs", "apk", "release")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(dir, "app-release.apk")
	if err := os.WriteFile(expected, []byte("fake apk"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := findArtifact(appPath, "apk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Fatalf("got %s, want %s", got, expected)
	}
}

func TestFindArtifact_NotFound(t *testing.T) {
	appPath := t.TempDir()

	_, err := findArtifact(appPath, "aab")
	if err == nil {
		t.Fatal("expected error when no artifact found, got nil")
	}
	if !strings.Contains(err.Error(), "no .aab artifact found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindArtifact_APK_NotFound(t *testing.T) {
	appPath := t.TempDir()

	_, err := findArtifact(appPath, "apk")
	if err == nil {
		t.Fatal("expected error when no artifact found, got nil")
	}
	if !strings.Contains(err.Error(), "no .apk artifact found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindArtifact_ReturnsFirstWhenMultiple(t *testing.T) {
	appPath := t.TempDir()
	dir := filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app-1.aab"), []byte("fake aab 1"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app-2.aab"), []byte("fake aab 2"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := findArtifact(appPath, "aab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, ".aab") {
		t.Fatalf("expected .aab file, got: %s", got)
	}
}
