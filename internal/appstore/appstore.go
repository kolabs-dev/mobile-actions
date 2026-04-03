package appstore

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ascBaseURL = "https://api.appstoreconnect.apple.com"

// Client holds App Store Connect API credentials and an HTTP client.
type Client struct {
	keyID      string
	issuerID   string
	privateKey *rsa.PrivateKey
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

	// Type-assert to *rsa.PrivateKey
	rsaKey, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return &Client{
		keyID:      keyID,
		issuerID:   issuerID,
		privateKey: rsaKey,
		httpClient: http.DefaultClient,
	}, nil
}

// generateJWT creates a signed RS256 JWT for the ASC API.
// Header: {"alg":"RS256","kid":keyID,"typ":"JWT"}
// Payload: {"iss":issuerID,"iat":now,"exp":now+1200,"aud":"appstoreconnect-v1"}
// Signature: RS256 using PKCS1v15
func (c *Client) generateJWT() (string, error) {
	now := time.Now().Unix()
	exp := now + 1200

	// Create header
	header := map[string]string{
		"alg": "RS256",
		"kid": c.keyID,
		"typ": "JWT",
	}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Create payload
	payload := map[string]interface{}{
		"iss": c.issuerID,
		"iat": now,
		"exp": exp,
		"aud": "appstoreconnect-v1",
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Create signature
	message := headerB64 + "." + payloadB64
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + signatureB64, nil
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

// PromoteBuild placeholder — will be replaced in Task 7.
func PromoteBuild(c *Client, bundleID, version, buildNumber string) error {
	return fmt.Errorf("not implemented")
}
