package playstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2/google"
)

var playBaseURL = "https://androidpublisher.googleapis.com/androidpublisher/v3/applications"

// NewClient creates an authenticated Google Play API HTTP client from a
// base64-encoded service account JSON. The JSON is decoded in memory — no
// temp file is written.
func NewClient(serviceAccountB64 string) (*http.Client, error) {
	data, err := base64.StdEncoding.DecodeString(serviceAccountB64)
	if err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}
	conf, err := google.JWTConfigFromJSON(data, "https://www.googleapis.com/auth/androidpublisher")
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	return conf.Client(context.Background()), nil
}

// CreateEdit opens a new Play Store edit and returns its ID.
func CreateEdit(client *http.Client, packageName string) (string, error) {
	url := fmt.Sprintf("%s/%s/edits", playBaseURL, packageName)
	resp, err := client.Post(url, "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		return "", fmt.Errorf("edits.insert: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, 200); err != nil {
		return "", fmt.Errorf("edits.insert: %w", err)
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}

// PromoteTrack updates the given track to contain versionCode at 100% rollout.
// For alpha and beta tracks, userFraction is ignored by the Play Store API.
func PromoteTrack(client *http.Client, packageName, editID, track, versionCode string) error {
	url := fmt.Sprintf("%s/%s/edits/%s/tracks/%s", playBaseURL, packageName, editID, track)
	body := map[string]interface{}{
		"track": track,
		"releases": []map[string]interface{}{
			{
				"versionCodes": []string{versionCode},
				"status":       "completed",
				"userFraction": 1.0,
			},
		},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("edits.tracks.update: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp, 200)
}

// CommitEdit commits the edit. Retries with changesNotSentForReview=true if
// the API explicitly requests it (managed publishing accounts).
func CommitEdit(client *http.Client, packageName, editID string) error {
	url := fmt.Sprintf("%s/%s/edits/%s:commit", playBaseURL, packageName, editID)
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("edits.commit: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	}

	if resp.StatusCode == 400 && strings.Contains(string(body), "changesNotSentForReview") {
		fmt.Println("retrying commit with changesNotSentForReview=true")
		retryURL := fmt.Sprintf("%s/%s/edits/%s:commit?changesNotSentForReview=true", playBaseURL, packageName, editID)
		resp2, err := client.Post(retryURL, "application/json", nil)
		if err != nil {
			return fmt.Errorf("edits.commit: %w", err)
		}
		defer resp2.Body.Close()
		return checkStatus(resp2, 200)
	}

	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

func checkStatus(resp *http.Response, expected int) error {
	if resp.StatusCode == expected {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}
