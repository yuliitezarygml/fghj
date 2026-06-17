package downloader

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	// Supabase configuration extracted from index.js
	supabaseURL     = "https://vergltjrbkvurnkzlcps.supabase.co"
	supabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InZlcmdsdGpyYmt2dXJua3psY3BzIiwicm9sZSI6ImFub24iLCJpYXQiOjE3Njk2NjEwMTEsImV4cCI6MjA4NTIzNzAxMX0.NDbs8C08X59q__QkiGpJJXkHC2-NSmhZxjt_-pLpEBg"
	// GitHub manifest mirror URL
	githubMirrorBase = "https://raw.githubusercontent.com/qwe213312/k25FCdfEOoEJ42S6/main"
)

// Supabase edge function request payload
type SupabaseRequest struct {
	Action     string `json:"action"`
	DepotID    string `json:"depotId"`
	ManifestID string `json:"manifestId"`
	APIKey     string `json:"apiKey"`
}

// Supabase edge function response payload
type SupabaseResponse struct {
	Base64 string `json:"base64"`
	Error  string `json:"error"`
}

// DownloadFromGithub fetches a manifest file from the public github mirror
func DownloadFromGithub(client *http.Client, depotID, manifestID, destPath string) (bool, error) {
	url := fmt.Sprintf("%s/%s_%s.manifest", githubMirrorBase, depotID, manifestID)
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil // Silent fallback, try Server 2
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("mirror returned status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return false, err
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return false, err
	}

	fmt.Printf("   -> [Success] Downloaded from Github mirror (%.2f MB)\n", float64(written)/(1024*1024))
	return true, nil
}

// DownloadFromSupabase invokes the Supabase Edge Function to fetch the manifest (and download it from Steam)
func DownloadFromSupabase(client *http.Client, depotID, manifestID, apiKey, destPath string) (bool, error) {
	url := fmt.Sprintf("%s/functions/v1/server2-manifest", supabaseURL)

	payload := SupabaseRequest{
		Action:     "download",
		DepotID:    depotID,
		ManifestID: manifestID,
		APIKey:     apiKey,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", supabaseAnonKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("supabase function returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var subResp SupabaseResponse
	if err := json.Unmarshal(body, &subResp); err != nil {
		return false, err
	}

	if subResp.Error != "" {
		return false, fmt.Errorf("supabase proxy error: %s", subResp.Error)
	}

	if subResp.Base64 == "" {
		return false, fmt.Errorf("response did not contain base64 content")
	}

	// Decode base64 manifest content
	manifestBytes, err := base64.StdEncoding.DecodeString(subResp.Base64)
	if err != nil {
		return false, fmt.Errorf("failed to decode base64 manifest: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return false, err
	}
	defer out.Close()

	written, err := out.Write(manifestBytes)
	if err != nil {
		return false, err
	}

	fmt.Printf("   -> [Success] Downloaded from Supabase proxy (%.2f MB)\n", float64(written)/(1024*1024))
	return true, nil
}
