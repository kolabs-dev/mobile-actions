// cmd/android-sign/main.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"

	"github.com/kolabs-dev/mobile-actions/internal/actions"
	"github.com/kolabs-dev/mobile-actions/internal/exec"
	"github.com/kolabs-dev/mobile-actions/internal/secrets"
)

func main() {
	if err := run(); err != nil {
		actions.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	unsignedPath := os.Getenv("INPUT_UNSIGNED_ARTIFACT_PATH")
	if unsignedPath == "" {
		return fmt.Errorf("INPUT_UNSIGNED_ARTIFACT_PATH is required")
	}
	keystoreB64 := os.Getenv("INPUT_KEYSTORE")
	keystorePass := os.Getenv("INPUT_KEYSTORE_PASSWORD")
	keyAlias := os.Getenv("INPUT_KEY_ALIAS")
	keyPass := os.Getenv("INPUT_KEY_PASSWORD")
	buildType := strings.ToLower(envOrDefault("INPUT_BUILD_TYPE", "aab"))

	actions.AddMask(keystorePass)
	actions.AddMask(keyPass)

	keystorePath, cleanKeystore, err := secrets.DecodeToFile(keystoreB64, "keystore-*.jks")
	if err != nil {
		return fmt.Errorf("decode keystore: %w", err)
	}
	defer cleanKeystore()

	signedDir := filepath.Join(os.Getenv("RUNNER_TEMP"), "mobile-actions", "signed")
	if err := os.MkdirAll(signedDir, 0755); err != nil {
		return fmt.Errorf("create signed dir: %w", err)
	}

	var signedPath string

	switch buildType {
	case "aab":
		signedPath, err = signAAB(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass)
	case "apk":
		signedPath, err = signAPK(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass)
	default:
		return fmt.Errorf("unknown build-type %q", buildType)
	}
	if err != nil {
		return err
	}

	fmt.Printf("signed artifact: %s\n", signedPath)
	return actions.SetOutput("artifact-path", signedPath)
}

func signAAB(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass string) (string, error) {
	actions.Group("Signing AAB with jarsigner")
	defer actions.EndGroup()

	// jarsigner signs in-place; work on a copy
	dest := filepath.Join(signedDir, "app-release.aab")
	if err := copyFile(unsignedPath, dest); err != nil {
		return "", fmt.Errorf("copy aab: %w", err)
	}

	err := exec.Run("jarsigner",
		"-verbose",
		"-sigalg", "SHA256withRSA",
		"-digestalg", "SHA-256",
		"-keystore", keystorePath,
		"-storepass", keystorePass,
		"-keypass", keyPass,
		dest,
		keyAlias,
	)
	if err != nil {
		return "", fmt.Errorf("jarsigner: %w", err)
	}
	return dest, nil
}

func signAPK(unsignedPath, signedDir, keystorePath, keystorePass, keyAlias, keyPass string) (string, error) {
	actions.Group("Signing APK with apksigner")
	defer actions.EndGroup()

	apksigner, err := findApksigner()
	if err != nil {
		return "", err
	}

	dest := filepath.Join(signedDir, "app-release.apk")
	err = exec.Run(apksigner,
		"sign",
		"--ks", keystorePath,
		"--ks-key-alias", keyAlias,
		"--ks-pass", "pass:"+keystorePass,
		"--key-pass", "pass:"+keyPass,
		"--out", dest,
		unsignedPath,
	)
	if err != nil {
		return "", fmt.Errorf("apksigner: %w", err)
	}
	return dest, nil
}

// findApksigner resolves the highest-versioned apksigner from $ANDROID_SDK_ROOT/build-tools/.
func findApksigner() (string, error) {
	sdkRoot := os.Getenv("ANDROID_SDK_ROOT")
	if sdkRoot == "" {
		return "", fmt.Errorf("ANDROID_SDK_ROOT is not set")
	}
	buildToolsDir := filepath.Join(sdkRoot, "build-tools")
	entries, err := os.ReadDir(buildToolsDir)
	if err != nil {
		return "", fmt.Errorf("read build-tools dir: %w", err)
	}

	type entry struct {
		v    *semver.Version
		name string
	}
	var versions []entry
	for _, e := range entries {
		v, err := semver.NewVersion(e.Name())
		if err != nil {
			continue
		}
		versions = append(versions, entry{v, e.Name()})
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no valid build-tools versions found in %s", buildToolsDir)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].v.GreaterThan(versions[j].v)
	})
	return filepath.Join(buildToolsDir, versions[0].name, "apksigner"), nil
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
