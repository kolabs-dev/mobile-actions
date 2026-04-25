package appstore

import (
	"bytes"
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

// PromoteBuild submits a TestFlight-processed build for App Store review.
// It resolves app and build IDs, creates or reuses an App Store version,
// attaches the build, and submits for review (fire-and-forget).
// whatsNew is set on all localizations when non-empty; App Store Connect requires
// it for submissions.
func PromoteBuild(c *Client, bundleID, version, buildNumber, whatsNew string) error {
	appID, err := resolveAppID(c, bundleID)
	if err != nil {
		return err
	}
	buildID, err := findBuild(c, appID, version, buildNumber)
	if err != nil {
		return err
	}
	versionID, err := findOrCreateVersion(c, appID, version)
	if err != nil {
		return err
	}
	if whatsNew != "" {
		if err := updateLocalization(c, versionID, whatsNew); err != nil {
			return err
		}
	}
	if err := attachBuild(c, versionID, buildID); err != nil {
		return err
	}
	return submitForReview(c, appID, versionID)
}

// findOrCreateVersion returns the App Store version ID for the given version string.
// Creates it if it does not exist. If a version in PREPARE_FOR_SUBMISSION state already
// exists, it is reused. Any other existing state is an error.
func findOrCreateVersion(c *Client, appID, version string) (string, error) {
	createURL := fmt.Sprintf("%s/v1/appStoreVersions", ascBaseURL)
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "appStoreVersions",
			"attributes": map[string]interface{}{
				"platform":      "IOS",
				"versionString": version,
			},
			"relationships": map[string]interface{}{
				"app": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "apps",
						"id":   appID,
					},
				},
			},
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, createURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("findOrCreateVersion: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findOrCreateVersion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 201 {
		var result struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("findOrCreateVersion: decode: %w", err)
		}
		return result.Data.ID, nil
	}

	if resp.StatusCode == 409 {
		conflictBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("findOrCreateVersion: 409 conflict creating version %s: %s\n", version, string(conflictBody))
		return findExistingVersion(c, appID, version)
	}

	respBody, _ := io.ReadAll(resp.Body)
	return "", fmt.Errorf("findOrCreateVersion: unexpected status %d: %s", resp.StatusCode, string(respBody))
}

// findExistingVersion looks up an existing App Store version by version string.
// Only versions in PREPARE_FOR_SUBMISSION state may be reused.
func findExistingVersion(c *Client, appID, version string) (string, error) {
	url := fmt.Sprintf("%s/v1/apps/%s/appStoreVersions?filter[versionString]=%s&filter[platform]=IOS&filter[appStoreState]=PREPARE_FOR_SUBMISSION,WAITING_FOR_REVIEW,IN_REVIEW,PENDING_CONTRACT,WAITING_FOR_EXPORT_COMPLIANCE,PENDING_DEVELOPER_RELEASE,PROCESSING_FOR_APP_STORE,READY_FOR_SALE,REPLACED_WITH_NEW_VERSION,DEVELOPER_REMOVED_FROM_SALE,REMOVED_FROM_SALE,DEVELOPER_REJECTED,REJECTED,METADATA_REJECTED",
		ascBaseURL, appID, version)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("findExistingVersion: %w", err)
	}
	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				AppStoreState string `json:"appStoreState"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("findExistingVersion: decode: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("App Store version %s not found after conflict", version)
	}
	v := result.Data[0]
	if v.Attributes.AppStoreState != "PREPARE_FOR_SUBMISSION" {
		return "", fmt.Errorf("App Store version %s already exists in state %s and cannot be edited",
			version, v.Attributes.AppStoreState)
	}
	return v.ID, nil
}

// updateLocalization sets whatsNew on every localization of an App Store version.
func updateLocalization(c *Client, versionID, whatsNew string) error {
	listURL := fmt.Sprintf("%s/v1/appStoreVersions/%s/appStoreVersionLocalizations", ascBaseURL, versionID)
	listReq, err := http.NewRequest(http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("updateLocalization: %w", err)
	}
	listResp, err := c.do(listReq)
	if err != nil {
		return fmt.Errorf("updateLocalization: %w", err)
	}
	defer listResp.Body.Close()
	if err := checkStatus(listResp, 200); err != nil {
		return fmt.Errorf("updateLocalization: %w", err)
	}
	var locResult struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&locResult); err != nil {
		return fmt.Errorf("updateLocalization: decode: %w", err)
	}
	for _, loc := range locResult.Data {
		patchURL := fmt.Sprintf("%s/v1/appStoreVersionLocalizations/%s", ascBaseURL, loc.ID)
		body := map[string]interface{}{
			"data": map[string]interface{}{
				"type": "appStoreVersionLocalizations",
				"id":   loc.ID,
				"attributes": map[string]interface{}{
					"whatsNew": whatsNew,
				},
			},
		}
		data, _ := json.Marshal(body)
		patchReq, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("updateLocalization: %w", err)
		}
		patchReq.Header.Set("Content-Type", "application/json")
		patchResp, err := c.do(patchReq)
		if err != nil {
			return fmt.Errorf("updateLocalization: %w", err)
		}
		io.ReadAll(patchResp.Body)
		patchResp.Body.Close()
		if err := checkStatus(patchResp, 200); err != nil {
			return fmt.Errorf("updateLocalization: patch %s: %w", loc.ID, err)
		}
	}
	return nil
}

// attachBuild sets the build on an App Store version.
func attachBuild(c *Client, versionID, buildID string) error {
	url := fmt.Sprintf("%s/v1/appStoreVersions/%s/relationships/build", ascBaseURL, versionID)
	body := map[string]interface{}{
		"data": map[string]string{
			"type": "builds",
			"id":   buildID,
		},
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("attachBuild: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("attachBuild: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp, 204)
}

// submitForReview submits an App Store version for review using the reviewSubmissions API.
// Three steps: create submission → add version item → PATCH submitted:true.
func submitForReview(c *Client, appID, versionID string) error {
	// Step 1: create the review submission
	subURL := fmt.Sprintf("%s/v1/reviewSubmissions", ascBaseURL)
	subBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "reviewSubmissions",
			"attributes": map[string]interface{}{
				"platform": "IOS",
			},
			"relationships": map[string]interface{}{
				"app": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "apps",
						"id":   appID,
					},
				},
			},
		},
	}
	subData, _ := json.Marshal(subBody)
	subReq, err := http.NewRequest(http.MethodPost, subURL, bytes.NewReader(subData))
	if err != nil {
		return fmt.Errorf("submitForReview: create submission: %w", err)
	}
	subReq.Header.Set("Content-Type", "application/json")
	subResp, err := c.do(subReq)
	if err != nil {
		return fmt.Errorf("submitForReview: create submission: %w", err)
	}
	defer subResp.Body.Close()
	if err := checkStatus(subResp, 201); err != nil {
		return fmt.Errorf("submitForReview: create submission: %w", err)
	}
	var subResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(subResp.Body).Decode(&subResult); err != nil {
		return fmt.Errorf("submitForReview: decode submission: %w", err)
	}
	submissionID := subResult.Data.ID

	// Step 2: add the App Store version as a submission item
	itemURL := fmt.Sprintf("%s/v1/reviewSubmissionItems", ascBaseURL)
	itemBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "reviewSubmissionItems",
			"relationships": map[string]interface{}{
				"reviewSubmission": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "reviewSubmissions",
						"id":   submissionID,
					},
				},
				"appStoreVersion": map[string]interface{}{
					"data": map[string]interface{}{
						"type": "appStoreVersions",
						"id":   versionID,
					},
				},
			},
		},
	}
	itemData, _ := json.Marshal(itemBody)
	itemReq, err := http.NewRequest(http.MethodPost, itemURL, bytes.NewReader(itemData))
	if err != nil {
		return fmt.Errorf("submitForReview: add item: %w", err)
	}
	itemReq.Header.Set("Content-Type", "application/json")
	itemResp, err := c.do(itemReq)
	if err != nil {
		return fmt.Errorf("submitForReview: add item: %w", err)
	}
	defer itemResp.Body.Close()
	if err := checkStatus(itemResp, 201); err != nil {
		return fmt.Errorf("submitForReview: add item: %w", err)
	}
	io.ReadAll(itemResp.Body)

	// Step 3: set submitted:true to trigger review
	patchURL := fmt.Sprintf("%s/v1/reviewSubmissions/%s", ascBaseURL, submissionID)
	patchBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "reviewSubmissions",
			"id":   submissionID,
			"attributes": map[string]interface{}{
				"submitted": true,
			},
		},
	}
	patchData, _ := json.Marshal(patchBody)
	patchReq, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(patchData))
	if err != nil {
		return fmt.Errorf("submitForReview: submit: %w", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := c.do(patchReq)
	if err != nil {
		return fmt.Errorf("submitForReview: submit: %w", err)
	}
	defer patchResp.Body.Close()
	io.ReadAll(patchResp.Body)
	return checkStatus(patchResp, 200)
}
