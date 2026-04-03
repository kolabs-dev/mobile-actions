package appstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testKeyB64 generates a base64-encoded PKCS#8 PEM EC P-256 private key (.p8 format).
func testKeyB64(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBlock)
}

// testClientWithServer creates a Client using a generated EC P-256 key and the given test server's HTTP client.
func testClientWithServer(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &Client{
		keyID:      "test-key",
		issuerID:   "test-issuer",
		privateKey: key,
		httpClient: srv.Client(),
	}
}

func TestNewClient_InvalidBase64(t *testing.T) {
	_, err := NewClient("not-valid-base64!!!", "kid", "iss")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestNewClient_NoPEMBlock(t *testing.T) {
	_, err := NewClient(base64.StdEncoding.EncodeToString([]byte("not pem")), "kid", "iss")
	if err == nil {
		t.Fatal("expected error for missing PEM block, got nil")
	}
}

func TestNewClient_Valid(t *testing.T) {
	c, err := NewClient(testKeyB64(t), "kid1", "iss1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.keyID != "kid1" {
		t.Errorf("keyID = %q, want kid1", c.keyID)
	}
	if c.issuerID != "iss1" {
		t.Errorf("issuerID = %q, want iss1", c.issuerID)
	}
}

func TestGenerateJWT_Structure(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := &Client{keyID: "kid1", issuerID: "iss1", privateKey: key, httpClient: http.DefaultClient}

	token, err := c.generateJWT()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "ES256" {
		t.Errorf("alg = %q, want ES256", header["alg"])
	}
	if header["kid"] != "kid1" {
		t.Errorf("kid = %q, want kid1", header["kid"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("typ = %q, want JWT", header["typ"])
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["iss"] != "iss1" {
		t.Errorf("iss = %v, want iss1", payload["iss"])
	}
	if payload["aud"] != "appstoreconnect-v1" {
		t.Errorf("aud = %v, want appstoreconnect-v1", payload["aud"])
	}
	exp := int64(payload["exp"].(float64))
	iat := int64(payload["iat"].(float64))
	if exp-iat != 1200 {
		t.Errorf("exp-iat = %d, want 1200", exp-iat)
	}
}

func TestGenerateJWT_SignatureVerifiable(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c := &Client{keyID: "kid1", issuerID: "iss1", privateKey: key, httpClient: http.DefaultClient}

	token, err := c.generateJWT()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := strings.Split(token, ".")
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sigBytes) != 64 {
		t.Fatalf("signature length = %d, want 64 (r||s for P-256)", len(sigBytes))
	}

	// Reconstruct and verify the signature
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))
	if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
		t.Error("JWT signature verification failed")
	}
}

func TestResolveAppID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("filter[bundleId]") != "com.example.app" {
			t.Errorf("unexpected bundleId query param: %s", r.URL.Query().Get("filter[bundleId]"))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[{"id":"app-123","type":"apps"}]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	id, err := resolveAppID(testClientWithServer(t, srv), "com.example.app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "app-123" {
		t.Errorf("id = %q, want app-123", id)
	}
}

func TestResolveAppID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := resolveAppID(testClientWithServer(t, srv), "com.missing.app")
	if err == nil {
		t.Fatal("expected error for missing app, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolveAppID_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`unauthorized`))
	}))
	defer srv.Close()

	orig := ascBaseURL
	ascBaseURL = srv.URL
	defer func() { ascBaseURL = orig }()

	_, err := resolveAppID(testClientWithServer(t, srv), "com.example.app")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
