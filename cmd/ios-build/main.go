// cmd/ios-build/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"howett.net/plist"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	intexec "github.com/kolabs-dev/mobile-actions/internal/exec"
)

const exportOptionsTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>method</key><string>{{.Method}}</string>
  <key>teamID</key><string>{{.TeamID}}</string>
  <key>signingStyle</key><string>manual</string>
  <key>provisioningProfiles</key>
  <dict>
    <key>{{.BundleID}}</key><string>{{.ProfileUUID}}</string>
  </dict>
  <key>destination</key><string>export</string>
  <key>uploadSymbols</key><true/>
</dict>
</plist>`

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	appPath := envOrDefault("INPUT_APP_PATH", ".")
	workspace := os.Getenv("INPUT_WORKSPACE")
	project := os.Getenv("INPUT_PROJECT")
	scheme := os.Getenv("INPUT_SCHEME")
	bundleID := os.Getenv("INPUT_BUNDLE_ID")
	teamID := os.Getenv("INPUT_TEAM_ID")
	destination := envOrDefault("INPUT_DESTINATION", "testflight")
	runnerTemp := os.Getenv("RUNNER_TEMP")

	if scheme == "" || bundleID == "" || teamID == "" {
		return fmt.Errorf("INPUT_SCHEME, INPUT_BUNDLE_ID, and INPUT_TEAM_ID are required")
	}
	if destination != "testflight" && destination != "app-store" {
		return fmt.Errorf("INPUT_DESTINATION must be 'testflight' or 'app-store', got %q", destination)
	}
	fmt.Printf("destination: %s\n", destination)

	// Pick the workspace or project to archive (auto-detected if neither is set)
	targetFlag, targetPath, err := resolveBuildTarget(appPath, workspace, project)
	if err != nil {
		return err
	}
	fmt.Printf("building %s %s\n", targetFlag, targetPath)

	// Get provisioning profile UUID from signing-state.json
	profileUUID, err := provisioningProfileUUID(runnerTemp)
	if err != nil {
		return err
	}

	// Write ExportOptions.plist
	exportDir := filepath.Join(runnerTemp, "mobile-actions")
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return err
	}
	exportOptionsPath := filepath.Join(exportDir, "ExportOptions.plist")
	if err := writeExportOptions(exportOptionsPath, teamID, bundleID, profileUUID, "app-store"); err != nil {
		return fmt.Errorf("write ExportOptions.plist: %w", err)
	}

	archivePath := filepath.Join(exportDir, scheme+".xcarchive")
	exportPath := filepath.Join(exportDir, "export")

	// Archive
	actions.Group("xcodebuild archive")
	err = intexec.Run("xcodebuild", "archive",
		targetFlag, targetPath,
		"-scheme", scheme,
		"-configuration", "Release",
		"-destination", "generic/platform=iOS",
		"-archivePath", archivePath,
		"-quiet",
		"CODE_SIGN_STYLE=Manual",
		"CODE_SIGN_IDENTITY=Apple Distribution",
		"DEVELOPMENT_TEAM="+teamID,
		"PROVISIONING_PROFILE="+profileUUID,
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("xcodebuild archive: %w", err)
	}

	// Export
	actions.Group("xcodebuild -exportArchive")
	err = intexec.Run("xcodebuild", "-exportArchive",
		"-archivePath", archivePath,
		"-exportOptionsPlist", exportOptionsPath,
		"-exportPath", exportPath,
		"-quiet",
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("xcodebuild exportArchive: %w", err)
	}

	// Find the exported IPA
	ipaPath, err := findIPA(exportPath)
	if err != nil {
		return err
	}
	fmt.Printf("exported IPA: %s\n", ipaPath)
	return actions.SetOutput("artifact-path", ipaPath)
}

// resolveBuildTarget returns the xcodebuild flag ("-workspace" or "-project")
// and path to archive. An explicit workspace or project wins; otherwise a
// workspace under appPath is preferred, then a single .xcodeproj.
func resolveBuildTarget(appPath, workspace, project string) (string, string, error) {
	switch {
	case workspace != "" && project != "":
		return "", "", fmt.Errorf("set only one of INPUT_WORKSPACE and INPUT_PROJECT")
	case workspace != "":
		return "-workspace", workspace, nil
	case project != "":
		return "-project", project, nil
	}

	workspaces, projects := findXcodeContainers(appPath)
	switch {
	case len(workspaces) == 1:
		return "-workspace", workspaces[0], nil
	case len(workspaces) > 1:
		return "", "", fmt.Errorf("multiple .xcworkspace found in %s: %v — set INPUT_WORKSPACE explicitly", appPath, workspaces)
	case len(projects) == 1:
		return "-project", projects[0], nil
	case len(projects) > 1:
		return "", "", fmt.Errorf("multiple .xcodeproj found in %s: %v — set INPUT_PROJECT or INPUT_WORKSPACE explicitly", appPath, projects)
	default:
		return "", "", fmt.Errorf("no .xcworkspace or .xcodeproj found in %s", appPath)
	}
}

// findXcodeContainers lists the .xcworkspace and .xcodeproj directories under
// appPath. It does not descend into either, so the project.xcworkspace that
// Xcode keeps inside every .xcodeproj is not counted as a workspace.
func findXcodeContainers(appPath string) (workspaces, projects []string) {
	_ = filepath.WalkDir(appPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".xcworkspace"):
			workspaces = append(workspaces, path)
			return filepath.SkipDir
		case strings.HasSuffix(path, ".xcodeproj"):
			projects = append(projects, path)
			return filepath.SkipDir
		}
		return nil
	})
	return workspaces, projects
}

func provisioningProfileUUID(runnerTemp string) (string, error) {
	stateFile := filepath.Join(runnerTemp, "mobile-actions", "signing-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return "", fmt.Errorf("read signing-state.json: %w", err)
	}
	var state struct {
		ProvisioningProfilePath string `json:"provisioning_profile_path"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}

	// Unwrap CMS and parse UUID via internal/exec
	plistStr, err := intexec.RunOutput("security", "cms", "-D", "-i", state.ProvisioningProfilePath)
	if err != nil {
		return "", fmt.Errorf("security cms -D: %w", err)
	}
	var profile map[string]interface{}
	if _, err := plist.Unmarshal([]byte(plistStr), &profile); err != nil {
		return "", fmt.Errorf("parse plist: %w", err)
	}
	uuid, _ := profile["UUID"].(string)
	if uuid == "" {
		return "", fmt.Errorf("UUID not found in provisioning profile")
	}
	return uuid, nil
}

func writeExportOptions(path, teamID, bundleID, profileUUID, method string) error {
	tmpl := template.Must(template.New("export").Parse(exportOptionsTmpl))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, map[string]string{
		"TeamID":      teamID,
		"BundleID":    bundleID,
		"ProfileUUID": profileUUID,
		"Method":      method,
	})
}

func findIPA(exportPath string) (string, error) {
	var found []string
	_ = filepath.WalkDir(exportPath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".ipa") {
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return "", fmt.Errorf("no .ipa found in %s", exportPath)
	}
	return found[0], nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
