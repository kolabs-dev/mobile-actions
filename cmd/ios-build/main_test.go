// cmd/ios-build/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectWorkspace_Found(t *testing.T) {
	appPath := t.TempDir()
	wsDir := filepath.Join(appPath, "MyApp.xcworkspace")
	if err := os.Mkdir(wsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := detectWorkspace(appPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wsDir {
		t.Fatalf("got %s, want %s", got, wsDir)
	}
}

func TestDetectWorkspace_NotFound(t *testing.T) {
	appPath := t.TempDir()

	_, err := detectWorkspace(appPath)
	if err == nil {
		t.Fatal("expected error when no workspace found, got nil")
	}
	if !strings.Contains(err.Error(), "no .xcworkspace found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectWorkspace_MultipleFound(t *testing.T) {
	appPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(appPath, "App1.xcworkspace"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(appPath, "App2.xcworkspace"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := detectWorkspace(appPath)
	if err == nil {
		t.Fatal("expected error when multiple workspaces found, got nil")
	}
	if !strings.Contains(err.Error(), "multiple .xcworkspace found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectWorkspace_NestedWorkspace(t *testing.T) {
	appPath := t.TempDir()
	nested := filepath.Join(appPath, "subdir", "MyApp.xcworkspace")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := detectWorkspace(appPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nested {
		t.Fatalf("got %s, want %s", got, nested)
	}
}

func TestWriteExportOptions_ContainsExpectedValues(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "ExportOptions.plist")

	err := writeExportOptions(outPath, "TEAM123", "com.example.myapp", "profile-uuid-456", "app-store")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"TEAM123",
		"com.example.myapp",
		"profile-uuid-456",
		"app-store",
		"manual",
		"uploadSymbols",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in ExportOptions.plist, got:\n%s", want, content)
		}
	}
}

func TestWriteExportOptions_InvalidPath(t *testing.T) {
	err := writeExportOptions("/nonexistent-dir/ExportOptions.plist", "T1", "com.x", "uuid", "app-store")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestFindIPA_Found(t *testing.T) {
	exportPath := t.TempDir()
	ipaPath := filepath.Join(exportPath, "MyApp.ipa")
	if err := os.WriteFile(ipaPath, []byte("fake ipa"), 0644); err != nil {
		t.Fatalf("write ipa: %v", err)
	}

	got, err := findIPA(exportPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ipaPath {
		t.Fatalf("got %s, want %s", got, ipaPath)
	}
}

func TestFindIPA_NotFound(t *testing.T) {
	exportPath := t.TempDir()

	_, err := findIPA(exportPath)
	if err == nil {
		t.Fatal("expected error when no IPA found, got nil")
	}
	if !strings.Contains(err.Error(), "no .ipa found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindIPA_NestedIPA(t *testing.T) {
	exportPath := t.TempDir()
	nested := filepath.Join(exportPath, "Apps", "MyApp.ipa")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(nested, []byte("fake ipa"), 0644); err != nil {
		t.Fatalf("write ipa: %v", err)
	}

	got, err := findIPA(exportPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nested {
		t.Fatalf("got %s, want %s", got, nested)
	}
}
