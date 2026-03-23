// cmd/ios-setup-signing/main.go
package main

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"

	"github.com/your-org/mobile-actions/internal/actions"
	intexec "github.com/your-org/mobile-actions/internal/exec"
	intsecrets "github.com/your-org/mobile-actions/internal/secrets"
)

type SigningState struct {
	KeychainName            string   `json:"keychain_name"`
	OriginalKeychains       []string `json:"original_keychains"`
	ProvisioningProfilePath string   `json:"provisioning_profile_path"`
}

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	certB64 := os.Getenv("INPUT_CERTIFICATE")
	certPass := os.Getenv("INPUT_CERTIFICATE_PASSWORD")
	profileB64 := os.Getenv("INPUT_PROVISIONING_PROFILE")
	runID := envOrDefault("GITHUB_RUN_ID", "local")

	actions.AddMask(certPass)

	// Decode secrets to temp files
	certPath, cleanCert, err := intsecrets.DecodeToFile(certB64, "cert-*.p12")
	if err != nil {
		return fmt.Errorf("decode certificate: %w", err)
	}
	defer cleanCert()

	profilePath, cleanProfile, err := intsecrets.DecodeToFile(profileB64, "profile-*.mobileprovision")
	if err != nil {
		return fmt.Errorf("decode provisioning profile: %w", err)
	}
	defer cleanProfile()

	// Save original keychain list
	originalKeychains, err := listKeychains()
	if err != nil {
		return fmt.Errorf("list keychains: %w", err)
	}

	// Create temp keychain
	keychainName := fmt.Sprintf("mobile-actions-%s.keychain-db", runID)
	keychainPass := generateKeychainPassword()
	actions.AddMask(keychainPass)

	actions.Group("Setting up signing keychain")
	defer actions.EndGroup()

	if err := intexec.Run("security", "create-keychain", "-p", keychainPass, keychainName); err != nil {
		return fmt.Errorf("create keychain: %w", err)
	}
	if err := intexec.Run("security", "unlock-keychain", "-p", keychainPass, keychainName); err != nil {
		return fmt.Errorf("unlock keychain: %w", err)
	}
	if err := intexec.Run("security", "set-keychain-settings", "-lut", "21600", keychainName); err != nil {
		return fmt.Errorf("set keychain settings: %w", err)
	}

	// Prepend to search list
	searchList := append([]string{keychainName}, originalKeychains...)
	if err := intexec.Run("security", append([]string{"list-keychains", "-s"}, searchList...)...); err != nil {
		return fmt.Errorf("set keychain search list: %w", err)
	}

	// Import certificate
	if err := intexec.Run("security", "import", certPath,
		"-k", keychainName,
		"-T", "/usr/bin/codesign",
		"-T", "/usr/bin/security",
		"-P", certPass,
	); err != nil {
		return fmt.Errorf("import certificate: %w", err)
	}

	// Allow non-interactive access
	if err := intexec.Run("security", "set-key-partition-list",
		"-S", "apple-tool:,apple:",
		"-s", "-k", keychainPass, keychainName,
	); err != nil {
		return fmt.Errorf("set key partition list: %w", err)
	}

	// Install provisioning profile
	profileDestPath, err := installProvisioningProfile(profilePath)
	if err != nil {
		return fmt.Errorf("install provisioning profile: %w", err)
	}

	// Write signing state
	state := SigningState{
		KeychainName:            keychainName,
		OriginalKeychains:       originalKeychains,
		ProvisioningProfilePath: profileDestPath,
	}
	if err := writeSigningState(state); err != nil {
		return fmt.Errorf("write signing state: %w", err)
	}

	fmt.Println("signing setup complete")
	return nil
}

func listKeychains() ([]string, error) {
	out, err := intexec.RunOutput("security", "list-keychains")
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.Trim(line, `"`))
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func installProvisioningProfile(profilePath string) (string, error) {
	// Unwrap CMS envelope to get raw plist via internal/exec
	plistStr, err := intexec.RunOutput("security", "cms", "-D", "-i", profilePath)
	if err != nil {
		return "", fmt.Errorf("security cms -D: %w", err)
	}

	// Parse plist to extract UUID
	var profile map[string]interface{}
	if _, err := plist.Unmarshal([]byte(plistStr), &profile); err != nil {
		return "", fmt.Errorf("parse provisioning profile plist: %w", err)
	}
	uuid, ok := profile["UUID"].(string)
	if !ok || uuid == "" {
		return "", fmt.Errorf("UUID not found in provisioning profile")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	destDir := filepath.Join(homeDir, "Library", "MobileDevice", "Provisioning Profiles")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, uuid+".mobileprovision")

	if err := copyFile(profilePath, dest); err != nil {
		return "", fmt.Errorf("copy provisioning profile: %w", err)
	}
	return dest, nil
}

func writeSigningState(state SigningState) error {
	dir := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "signing-state.json"), data, 0600)
}

func generateKeychainPassword() string {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return fmt.Sprintf("%x", b)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
