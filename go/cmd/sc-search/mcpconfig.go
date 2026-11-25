package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mcpConfigPlaceholder* are the tokens used by config/mcp_config.example.json.
const (
	placeholderCommand = "<server.command>"
	placeholderDB      = "<server.env.SC_SEARCH_DB>"
)

// renderMCPConfigs writes config/mcp_config.json plus project-level
// .mcp.json / .cursor/mcp.json. Hosts only guarantee command+args+env, so the
// generated server block deliberately omits the non-standard "cwd" field.
func renderMCPConfigs(root, bin, db string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absBin, err := filepath.Abs(bin)
	if err != nil {
		return err
	}
	absDB, err := filepath.Abs(db)
	if err != nil {
		return err
	}

	server := map[string]any{
		"name":      "sc-search",
		"transport": "stdio",
		"command":   absBin,
		"args":      []any{"mcp"},
		"env":       map[string]any{"SC_SEARCH_DB": absDB},
	}
	serversOnly := map[string]any{"mcpServers": map[string]any{"sc-search": mcpServerEntry(absBin, absDB)}}

	data := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(absRoot, "config", "mcp_config.example.json")); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parse mcp_config.example.json: %w", err)
		}
		replacePlaceholders(data, absBin, absDB)
	} else {
		data["schema_version"] = 1
	}
	data["description"] = "本机绝对路径由 setup_mcp 生成。Agent「初始化」读取此文件，按 client_hints 适配当前客户端。"
	data["server"] = server
	data["mcpServers"] = serversOnly["mcpServers"]

	out := filepath.Join(absRoot, "config", "mcp_config.json")
	if err := writeJSON(out, data); err != nil {
		return err
	}
	fmt.Printf("已写入 %s\n", out)

	for _, rel := range []string{".mcp.json", filepath.Join(".cursor", "mcp.json")} {
		dest := filepath.Join(absRoot, rel)
		if err := writeJSON(dest, serversOnly); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", dest)
	}
	return nil
}

func mcpServerEntry(bin, db string) map[string]any {
	return map[string]any{
		"command": bin,
		"args":    []any{"mcp"},
		"env":     map[string]any{"SC_SEARCH_DB": db},
	}
}

func writeJSON(dest string, v any) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dest, append(raw, '\n'), 0o644)
}

// replacePlaceholders walks arbitrary config JSON and swaps the template tokens.
func replacePlaceholders(node any, bin, db string) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok {
				v[k] = strings.NewReplacer(
					placeholderCommand, bin,
					placeholderDB, db,
				).Replace(s)
				continue
			}
			replacePlaceholders(child, bin, db)
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				v[i] = strings.NewReplacer(
					placeholderCommand, bin,
					placeholderDB, db,
				).Replace(s)
				continue
			}
			replacePlaceholders(child, bin, db)
		}
	}
}
