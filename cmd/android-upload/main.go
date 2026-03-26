// cmd/android-upload/main.go
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/playstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be uploaded without actually uploading")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	serviceAccountB64 := os.Getenv("INPUT_SERVICE_ACCOUNT_JSON")
	packageName := os.Getenv("INPUT_PACKAGE_NAME")
	versionCode := os.Getenv("INPUT_VERSION_CODE")
	track := envOrDefault("INPUT_TRACK", "internal")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))
	artifactPath := os.Getenv("INPUT_ARTIFACT_PATH")

	if packageName == "" || versionCode == "" || artifactPath == "" || serviceAccountB64 == "" {
		return fmt.Errorf("INPUT_PACKAGE_NAME, INPUT_VERSION_CODE, INPUT_ARTIFACT_PATH, and INPUT_SERVICE_ACCOUNT_JSON are required")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would upload %s to Play Store package=%s track=%s version-code=%s\n",
			artifactPath, packageName, track, versionCode)
		return nil
	}

	client, err := playstore.NewClient(serviceAccountB64)
	if err != nil {
		return err
	}

	actions.Group("Uploading to Google Play")
	defer actions.EndGroup()

	editID, err := playstore.CreateEdit(client, packageName)
	if err != nil {
		return err
	}
	fmt.Printf("created edit: %s\n", editID)

	if err := uploadArtifact(client, packageName, editID, buildType, artifactPath); err != nil {
		return err
	}

	if err := playstore.PromoteTrack(client, packageName, editID, track, versionCode); err != nil {
		return err
	}

	if err := playstore.CommitEdit(client, packageName, editID); err != nil {
		return err
	}

	fmt.Println("upload complete")
	return nil
}

func uploadArtifact(client *http.Client, packageName, editID, buildType, artifactPath string) error {
	f, err := os.Open(artifactPath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()

	var resource string
	var mimeType string
	switch buildType {
	case "aab":
		resource = "bundles"
		mimeType = "application/octet-stream"
	default:
		resource = "apks"
		mimeType = "application/vnd.android.package-archive"
	}

	url := fmt.Sprintf("https://androidpublisher.googleapis.com/upload/androidpublisher/v3/applications/%s/edits/%s/%s?uploadType=media",
		packageName, editID, resource)

	req, _ := http.NewRequest("POST", url, f)
	req.Header.Set("Content-Type", mimeType)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("edits.%s.upload: %w", resource, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 409 {
		body, _ := io.ReadAll(resp.Body)
		actions.Error(fmt.Sprintf("version code conflict: %s", string(body)))
		return fmt.Errorf("edits.%s.upload: version code already exists (HTTP 409)", resource)
	}
	return checkStatus(resp, 200)
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	actions.Error(fmt.Sprintf("API error %d: %s", resp.StatusCode, string(body)))
	return fmt.Errorf("unexpected status %d", resp.StatusCode)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
