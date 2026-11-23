// Package bridge invokes the Python MCP handlers during dual-track migration.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Caller runs `python -m sc_search.mcp_server.bridge TOOL ARGS_JSON`.
type Caller struct {
	// RepoRoot is the SC_Search repository root (parent of src/ and go/).
	RepoRoot string
	// Python is the interpreter or launcher (default: uv).
	Python string
	// Timeout per tool call.
	Timeout time.Duration
}

func New(repoRoot string) *Caller {
	return &Caller{
		RepoRoot: repoRoot,
		Python:   "uv",
		Timeout:  60 * time.Second,
	}
}

// Call returns the raw JSON envelope string from the Python handler.
func (c *Caller) Call(ctx context.Context, tool string, args map[string]any) (string, error) {
	if c.RepoRoot == "" {
		return "", fmt.Errorf("bridge: RepoRoot is empty")
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	launcher := c.Python
	if launcher == "" {
		launcher = "uv"
	}
	if launcher == "uv" {
		cmd = exec.CommandContext(ctx, "uv", "run", "python", "-m", "sc_search.mcp_server.bridge", tool, string(payload))
	} else {
		cmd = exec.CommandContext(ctx, launcher, "-m", "sc_search.mcp_server.bridge", tool, string(payload))
	}
	cmd.Dir = c.RepoRoot
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(c.RepoRoot, "src"))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("bridge %s: %s", tool, msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("bridge %s: empty stdout", tool)
	}
	return out, nil
}

// FindRepoRoot walks up from start (or cwd) looking for go.mod + src/sc_search.
func FindRepoRoot(start string) (string, error) {
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
		goMod := filepath.Join(dir, "go", "go.mod")
		srcMarker := filepath.Join(dir, "src", "sc_search")
		if fileExists(goMod) && dirExists(srcMarker) {
			return dir, nil
		}
		// Also allow running from go/ subdirectory.
		parentMod := filepath.Join(dir, "go.mod")
		parentSrc := filepath.Join(filepath.Dir(dir), "src", "sc_search")
		if fileExists(parentMod) && dirExists(parentSrc) && filepath.Base(dir) == "go" {
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

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
