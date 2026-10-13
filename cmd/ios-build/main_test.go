// cmd/ios-build/main_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
}

func assertTarget(t *testing.T, appPath, wantFlag, wantPath string) {
	t.Helper()
	flag, path, err := resolveBuildTarget(appPath, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flag != wantFlag || path != wantPath {
		t.Fatalf("got %s %s, want %s %s", flag, path, wantFlag, wantPath)
	}
}

func assertTargetError(t *testing.T, appPath, want string) {
	t.Helper()
	_, _, err := resolveBuildTarget(appPath, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveBuildTarget_Workspace(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, "MyApp.xcworkspace")
	assertTarget(t, appPath, "-workspace", filepath.Join(appPath, "MyApp.xcworkspace"))
}

func TestResolveBuildTarget_NotFound(t *testing.T) {
	assertTargetError(t, t.TempDir(), "no .xcworkspace or .xcodeproj found")
}

func TestResolveBuildTarget_MultipleWorkspaces(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, "App1.xcworkspace", "App2.xcworkspace")
	assertTargetError(t, appPath, "multiple .xcworkspace found")
}

func TestResolveBuildTarget_NestedWorkspace(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, filepath.Join("subdir", "MyApp.xcworkspace"))
	assertTarget(t, appPath, "-workspace", filepath.Join(appPath, "subdir", "MyApp.xcworkspace"))
}

func TestResolveBuildTarget_ProjectOnly(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, "MyApp.xcodeproj")
	assertTarget(t, appPath, "-project", filepath.Join(appPath, "MyApp.xcodeproj"))
}

func TestResolveBuildTarget_ProjectWithEmbeddedWorkspace(t *testing.T) {
	// Xcode keeps a project.xcworkspace inside every .xcodeproj; it is not a separate workspace.
	appPath := t.TempDir()
	mkdirs(t, appPath, filepath.Join("MyApp.xcodeproj", "project.xcworkspace"))
	assertTarget(t, appPath, "-project", filepath.Join(appPath, "MyApp.xcodeproj"))
}

func TestResolveBuildTarget_WorkspaceAndProject(t *testing.T) {
	// CocoaPods layout: the workspace wins over the project, and the project's
	// embedded workspace does not make it ambiguous.
	appPath := t.TempDir()
	mkdirs(t, appPath, "MyApp.xcworkspace", filepath.Join("MyApp.xcodeproj", "project.xcworkspace"),
		filepath.Join("Pods", "Pods.xcodeproj"))
	assertTarget(t, appPath, "-workspace", filepath.Join(appPath, "MyApp.xcworkspace"))
}

func TestResolveBuildTarget_MultipleProjects(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, "App1.xcodeproj", "App2.xcodeproj")
	assertTargetError(t, appPath, "multiple .xcodeproj found")
}

func TestResolveBuildTarget_Explicit(t *testing.T) {
	appPath := t.TempDir()
	mkdirs(t, appPath, "Other.xcworkspace")
	flag, path, err := resolveBuildTarget(appPath, "", "ios/MyApp.xcodeproj")
	if err != nil || flag != "-project" || path != "ios/MyApp.xcodeproj" {
		t.Fatalf("project: got %s %s %v", flag, path, err)
	}
	flag, path, err = resolveBuildTarget(appPath, "ios/MyApp.xcworkspace", "")
	if err != nil || flag != "-workspace" || path != "ios/MyApp.xcworkspace" {
		t.Fatalf("workspace: got %s %s %v", flag, path, err)
	}
	if _, _, err := resolveBuildTarget(appPath, "a.xcworkspace", "b.xcodeproj"); err == nil {
		t.Fatal("expected error when both workspace and project are set")
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
