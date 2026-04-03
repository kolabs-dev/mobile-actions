package appstore

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

var ascBaseURL = "https://api.appstoreconnect.apple.com"

// Client holds App Store Connect API credentials and an HTTP client.
type Client struct {
	keyID      string
	issuerID   string
	privateKey *ecdsa.PrivateKey
	httpClient *http.Client
}

// NewClient creates a Client from a base64-encoded .p8 API key.
func NewClient(keyB64, keyID, issuerID string) (*Client, error) {
	// Decode base64
	keyPEM, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Parse PKCS#8 private key
	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 key: %w", err)
	}

	// Type-assert to *ecdsa.PrivateKey (Apple .p8 keys are always EC keys)
	ecKey, ok := privKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an EC private key")
	}

	return &Client{
		keyID:      keyID,
		issuerID:   issuerID,
		privateKey: ecKey,
		httpClient: http.DefaultClient,
	}, nil
}

// generateJWT creates a signed ES256 JWT for the ASC API.
// Header: {"alg":"ES256","kid":keyID,"typ":"JWT"}
// Payload: {"iss":issuerID,"iat":now,"exp":now+1200,"aud":"appstoreconnect-v1"}
// Signature: ES256 using ECDSA P-256, encoded as raw r||s (JWT P1363 format)
func (c *Client) generateJWT() (string, error) {
	now := time.Now().Unix()
	exp := now + 1200

	// Create header — json.Marshal cannot fail for map[string]string
	header := map[string]string{
		"alg": "ES256",
		"kid": c.keyID,
		"typ": "JWT",
	}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Create payload — json.Marshal cannot fail for map[string]interface{} with string/int values
	payload := map[string]interface{}{
		"iss": c.issuerID,
		"iat": now,
		"exp": exp,
		"aud": "appstoreconnect-v1",
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Sign with ECDSA P-256
	si := headerB64 + "." + payloadB64
	digest := sha256.Sum256([]byte(si))
	sig, err := ecdsa.SignASN1(rand.Reader, c.privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	// Convert DER-encoded ASN.1 signature to raw r||s (JWT ES256 / P1363 format)
	var ecSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &ecSig); err != nil {
		return "", fmt.Errorf("parse EC signature: %w", err)
	}
	sigBytes := make([]byte, 64)
	ecSig.R.FillBytes(sigBytes[:32])
	ecSig.S.FillBytes(sigBytes[32:])

	return si + "." + base64.RawURLEncoding.EncodeToString(sigBytes), nil
}

// do attaches a fresh JWT Authorization header and executes the request.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	token, err := c.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("generate JWT: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return c.httpClient.Do(req)
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

// resolveAppID returns the ASC app ID for the given bundle ID.
func resolveAppID(c *Client, bundleID string) (string, error) {
	url := fmt.Sprintf("%s/v1/apps?filter[bundleId]=%s", ascBaseURL, bundleID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("resolveAppID: %w", err)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("resolveAppID: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("app with bundle ID %q not found", bundleID)
	}
	return result.Data[0].ID, nil
}

// findBuild returns the build ID for a specific version + build number.
// The build must be fully processed (processingState=VALID).
func findBuild(c *Client, appID, version, buildNumber string) (string, error) {
	url := fmt.Sprintf("%s/v1/builds?filter[app]=%s&filter[version]=%s&filter[preReleaseVersion.version]=%s&filter[processingState]=VALID",
		ascBaseURL, appID, buildNumber, version)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("findBuild: %w", err)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("findBuild: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("build %s(%s) not found or still processing", version, buildNumber)
	}
	return result.Data[0].ID, nil
}

// PromoteBuild placeholder — will be replaced in Task 7.
func PromoteBuild(c *Client, bundleID, version, buildNumber string) error {
	return fmt.Errorf("not implemented")
}
