// cmd/ios-teardown-signing/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/mobile-actions/internal/actions"
	"github.com/your-org/mobile-actions/internal/exec"
)

type SigningState struct {
	KeychainName            string   `json:"keychain_name"`
	OriginalKeychains       []string `json:"original_keychains"`
	ProvisioningProfilePath string   `json:"provisioning_profile_path"`
}

func main() {
	if err := run(); err != nil {
		actions.Warning(fmt.Sprintf("teardown warning: %v", err))
		// Do not exit 1 — teardown failures are non-fatal
	}
}

func run() error {
	stateFile := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions", "signing-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("read signing-state.json: %w", err)
	}

	var state SigningState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse signing-state.json: %w", err)
	}

	var errs []string

	// Delete keychain (best-effort)
	if err := exec.Run("security", "delete-keychain", state.KeychainName); err != nil {
		errs = append(errs, fmt.Sprintf("delete keychain: %v", err))
	}

	// Restore original search list
	if len(state.OriginalKeychains) > 0 {
		if err := exec.Run("security", append([]string{"list-keychains", "-s"}, state.OriginalKeychains...)...); err != nil {
			errs = append(errs, fmt.Sprintf("restore keychain list: %v", err))
		}
	}

	// Remove provisioning profile
	if state.ProvisioningProfilePath != "" {
		if err := os.Remove(state.ProvisioningProfilePath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove provisioning profile: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("teardown errors: %v", errs)
	}
	fmt.Println("signing teardown complete")
	return nil
}
