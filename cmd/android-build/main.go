// cmd/android-build/main.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/exec"
)

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	appPath := envOrDefault("INPUT_APP_PATH", ".")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))
	gradleTask := os.Getenv("INPUT_GRADLE_TASK")

	if gradleTask == "" {
		switch buildType {
		case "aab":
			gradleTask = "bundleRelease"
		case "apk":
			gradleTask = "assembleRelease"
		default:
			return fmt.Errorf("unknown build-type %q: must be 'aab' or 'apk'", buildType)
		}
	}

	gradlew := filepath.Join(appPath, "gradlew")
	actions.Group(fmt.Sprintf("Running Gradle task: %s", gradleTask))
	if err := exec.Run(gradlew, gradleTask); err != nil {
		actions.EndGroup()
		return fmt.Errorf("gradle build failed: %w", err)
	}
	actions.EndGroup()

	artifactPath, err := findArtifact(appPath, buildType)
	if err != nil {
		return err
	}

	fmt.Printf("unsigned artifact: %s\n", artifactPath)
	return actions.SetOutput("unsigned-artifact-path", artifactPath)
}

// findArtifact searches for the Gradle output artifact in the standard output directories.
func findArtifact(appPath, buildType string) (string, error) {
	var searchPath string
	var ext string
	switch buildType {
	case "aab":
		searchPath = filepath.Join(appPath, "app", "build", "outputs", "bundle", "release")
		ext = ".aab"
	case "apk":
		searchPath = filepath.Join(appPath, "app", "build", "outputs", "apk", "release")
		ext = ".apk"
	}

	var found []string
	_ = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ext) {
			found = append(found, path)
		}
		return nil
	})

	if len(found) == 0 {
		return "", fmt.Errorf("no %s artifact found in %s", ext, searchPath)
	}
	return found[0], nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
