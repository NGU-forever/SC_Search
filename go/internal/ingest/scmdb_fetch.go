package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func scmdbGetJSON(client *http.Client, path string) (any, error) {
	var last error
	for _, base := range []string{scmdbMirrorBase, scmdbBase} {
		url := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			last = err
			continue
		}
		req.Header.Set("User-Agent", scmdbUserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode >= 400 {
			last = fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
			continue
		}
		text := strings.TrimLeft(strings.TrimPrefix(string(body), "\ufeff"), " \t\r\n")
		if strings.HasPrefix(text, "<") {
			last = fmt.Errorf("HTML instead of JSON at %s", url)
			continue
		}
		var out any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			last = err
			continue
		}
		return out, nil
	}
	return nil, fmt.Errorf("failed to fetch %s: %v", path, last)
}

func resolveScmdbVersion(client *http.Client, channel, version string) (map[string]any, error) {
	raw, err := scmdbGetJSON(client, "versions.json")
	if err != nil {
		return nil, err
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("versions.json was not a list")
	}
	var versions []map[string]any
	for _, item := range arr {
		m := asStringMap(item)
		if m == nil || strAny(m["version"]) == "" || strAny(m["file"]) == "" {
			continue
		}
		versions = append(versions, m)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no SCMDB versions available")
	}
	if version != "" {
		needle := strings.ToLower(strings.TrimSpace(version))
		for _, item := range versions {
			ver := strings.ToLower(strAny(item["version"]))
			if ver == needle || strings.Contains(ver, needle) {
				return item, nil
			}
		}
		return nil, fmt.Errorf("SCMDB version not found: %s", version)
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		channel = "live"
	}
	if channel == "latest" || channel == "any" || channel == "all" {
		return versions[0], nil
	}
	if channel != "live" && channel != "ptu" {
		return nil, fmt.Errorf("channel must be live, ptu, or latest")
	}
	needle := "-" + channel + "."
	for _, item := range versions {
		if strings.Contains(strings.ToLower(strAny(item["version"])), needle) {
			return item, nil
		}
	}
	return versions[0], nil
}

// SyncScmdbPayload applies an in-memory SCMDB dump (tests / offline).
