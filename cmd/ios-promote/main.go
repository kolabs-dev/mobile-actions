// cmd/ios-promote/main.go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/appstore"
)

var dryRun = flag.Bool("dry-run", false, "log what would be promoted without calling the API")

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
	bundleID := os.Getenv("INPUT_BUNDLE_ID")
	version := os.Getenv("INPUT_VERSION")
	buildNumber := os.Getenv("INPUT_BUILD_NUMBER")

	if keyB64 == "" || keyID == "" || issuerID == "" || bundleID == "" || version == "" || buildNumber == "" {
		return fmt.Errorf("INPUT_APP_STORE_CONNECT_KEY, INPUT_APP_STORE_CONNECT_KEY_ID, INPUT_APP_STORE_CONNECT_ISSUER_ID, INPUT_BUNDLE_ID, INPUT_VERSION, and INPUT_BUILD_NUMBER are required")
	}

	if *dryRun {
		fmt.Printf("[dry-run] would promote build %s(%s) of %s for App Store review\n",
			version, buildNumber, bundleID)
		return nil
	}

	client, err := appstore.NewClient(keyB64, keyID, issuerID)
	if err != nil {
		return err
	}

	actions.Group("Submitting for App Store review")
	defer actions.EndGroup()

	if err := appstore.PromoteBuild(client, bundleID, version, buildNumber); err != nil {
		return err
	}

	fmt.Printf("submitted %s(%s) of %s for App Store review\n", version, buildNumber, bundleID)
	return nil
}
