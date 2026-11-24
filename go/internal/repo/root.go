package repo

import (
	"fmt"
	"os"
	"path/filepath"
)

const releaseMarker = ".sc-search-root"

// FindRoot walks up from start (or cwd) looking for the project root.
// Recognizes: go/go.mod (dev checkout), or .sc-search-root (Release zip).
func FindRoot(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, releaseMarker)) {
			return dir, nil
		}
		if fileExists(filepath.Join(dir, "go", "go.mod")) {
			return dir, nil
		}
		if fileExists(filepath.Join(dir, "go.mod")) && filepath.Base(dir) == "go" {
			return filepath.Dir(dir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root not found from %s", start)
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
