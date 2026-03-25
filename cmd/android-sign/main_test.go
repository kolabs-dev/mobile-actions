// cmd/android-sign/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindApksigner_PicksHighestVersion(t *testing.T) {
	sdkRoot := t.TempDir()
	buildTools := filepath.Join(sdkRoot, "build-tools")
	for _, v := range []string{"33.0.0", "34.0.1", "34.0.0"} {
		dir := filepath.Join(buildTools, v)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", v, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "apksigner"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("write apksigner %s: %v", v, err)
		}
	}
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	got, err := findApksigner()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(buildTools, "34.0.1", "apksigner")
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestFindApksigner_MissingSDKRoot(t *testing.T) {
	t.Setenv("ANDROID_SDK_ROOT", "")

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when ANDROID_SDK_ROOT is empty, got nil")
	}
	if !strings.Contains(err.Error(), "ANDROID_SDK_ROOT") {
		t.Fatalf("expected ANDROID_SDK_ROOT in error, got: %v", err)
	}
}

func TestFindApksigner_NoValidVersions(t *testing.T) {
	sdkRoot := t.TempDir()
	buildTools := filepath.Join(sdkRoot, "build-tools")
	if err := os.MkdirAll(buildTools, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(buildTools, "not-a-version"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(buildTools, "also-bad"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when no valid semver versions found, got nil")
	}
	if !strings.Contains(err.Error(), "no valid build-tools versions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindApksigner_BuildToolsDirMissing(t *testing.T) {
	sdkRoot := t.TempDir()
	// Don't create build-tools directory
	t.Setenv("ANDROID_SDK_ROOT", sdkRoot)

	_, err := findApksigner()
	if err == nil {
		t.Fatal("expected error when build-tools dir missing, got nil")
	}
	if !strings.Contains(err.Error(), "read build-tools dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")
	content := []byte("test content 1234")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestCopyFile_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dest.txt"))
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}
