// cmd/ios-upload/main.go
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	intexec "github.com/kolabs-dev/mobile-actions/internal/exec"
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
	keyB64 := os.Getenv("INPUT_APP_STORE_CONNECT_KEY")
	keyID := os.Getenv("INPUT_APP_STORE_CONNECT_KEY_ID")
	issuerID := os.Getenv("INPUT_APP_STORE_CONNECT_ISSUER_ID")
	ipaPath := os.Getenv("INPUT_ARTIFACT_PATH")

	if keyB64 == "" || keyID == "" || issuerID == "" || ipaPath == "" {
		return fmt.Errorf("INPUT_APP_STORE_CONNECT_KEY, INPUT_APP_STORE_CONNECT_KEY_ID, INPUT_APP_STORE_CONNECT_ISSUER_ID, and INPUT_ARTIFACT_PATH are required")
	}

	itmsPath, err := resolveITMSTransporter()
	if err != nil {
		return err
	}

	p8Path, cleanP8, err := installP8Key(keyB64, keyID)
	if err != nil {
		return err
	}
	defer cleanP8()

	if *dryRun {
		fmt.Printf("[dry-run] would run: %s -m upload -f %s -apiKey %s -apiIssuer %s (key at %s)\n",
			itmsPath, ipaPath, keyID, issuerID, p8Path)
		return nil
	}

	actions.Group("Uploading IPA via iTMSTransporter")
	err = intexec.Run(itmsPath,
		"-m", "upload",
		"-f", ipaPath,
		"-apiKey", keyID,
		"-apiIssuer", issuerID,
	)
	actions.EndGroup()
	if err != nil {
		return fmt.Errorf("iTMSTransporter upload: %w", err)
	}

	fmt.Println("upload complete")
	return nil
}

func resolveITMSTransporter() (string, error) {
	// Try xcrun first — works across Xcode versions regardless of install path.
	if path, err := intexec.RunOutput("xcrun", "-find", "iTMSTransporter"); err == nil && path != "" {
		return path, nil
	}

	// Fall back to manual path resolution for older Xcode installations.
	developerDir, err := intexec.RunOutput("xcode-select", "-p")
	if err != nil {
		return "", fmt.Errorf("xcode-select -p: %w", err)
	}
	xcodeContents := filepath.Dir(developerDir)
	path := filepath.Join(xcodeContents,
		"SharedFrameworks",
		"ContentDeliveryServices.framework",
		"Versions", "A", "itms", "bin", "iTMSTransporter")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("iTMSTransporter not found at %s: %w", path, err)
	}
	return path, nil
}

func installP8Key(keyB64, keyID string) (string, func(), error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	keyDir := filepath.Join(homeDir, ".appstoreconnect", "private_keys")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", nil, err
	}
	keyPath := filepath.Join(keyDir, "AuthKey_"+keyID+".p8")
	rawKey, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", nil, fmt.Errorf("decode p8 key: %w", err)
	}
	if err := os.WriteFile(keyPath, rawKey, 0600); err != nil {
		return "", nil, fmt.Errorf("write p8 key: %w", err)
	}
	cleanup := func() { os.Remove(keyPath) }
	return keyPath, cleanup, nil
}
