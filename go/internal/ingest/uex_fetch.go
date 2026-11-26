package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func loadUEXToken(repoRoot string) string {
	if t := strings.TrimSpace(os.Getenv("UEX_API_TOKEN")); t != "" {
		return t
	}
	// Minimal .env parse for UEX_API_TOKEN=
	data, err := os.ReadFile(filepath.Join(repoRoot, "config", ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "UEX_API_TOKEN=") {
			return strings.Trim(strings.TrimPrefix(line, "UEX_API_TOKEN="), "\"'")
		}
	}
	return ""
}

func uexGet(client *http.Client, base, token, path string) ([]map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", uexUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("UEX rate limit reached")
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("UEX non-JSON response HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("UEX HTTP %d: %v", resp.StatusCode, envelope)
	}
	if st, ok := envelope["status"]; ok && st != nil && st != "ok" {
		return nil, fmt.Errorf("UEX status=%v: %v", st, envelope)
	}
	raw, ok := envelope["data"]
	if !ok {
		raw = envelope
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s response was not a list", path)
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// SyncUexLive fetches UEX API and applies (requires UEX_API_TOKEN).
