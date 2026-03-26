// cmd/android-promote/main.go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/playstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be promoted without actually calling the API")

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
	targetTrack := os.Getenv("INPUT_TARGET_TRACK")

	if serviceAccountB64 == "" || packageName == "" || versionCode == "" || targetTrack == "" {
		return fmt.Errorf("INPUT_SERVICE_ACCOUNT_JSON, INPUT_PACKAGE_NAME, INPUT_VERSION_CODE, and INPUT_TARGET_TRACK are required")
	}

	switch targetTrack {
	case "alpha", "beta", "production":
		// valid
	default:
		return fmt.Errorf("target-track must be one of: alpha, beta, production")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would promote version-code=%s to track=%s for package=%s\n",
			versionCode, targetTrack, packageName)
		return nil
	}

	client, err := playstore.NewClient(serviceAccountB64)
	if err != nil {
		return err
	}

	actions.Group("Promoting on Google Play")
	defer actions.EndGroup()

	editID, err := playstore.CreateEdit(client, packageName)
	if err != nil {
		return err
	}
	fmt.Printf("created edit: %s\n", editID)

	if err := playstore.PromoteTrack(client, packageName, editID, targetTrack, versionCode); err != nil {
		return err
	}

	if err := playstore.CommitEdit(client, packageName, editID); err != nil {
		return err
	}

	fmt.Printf("promoted version-code=%s to track=%s\n", versionCode, targetTrack)
	return nil
}
