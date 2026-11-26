// Package mcp registers the sc-search MCP tools (Go-native only).
package mcp

import (
	"database/sql"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
)

// NewServer builds the stdio MCP server. db must be non-nil.
func NewServer(db *sql.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sc-search",
		Version: "1.1.0-go",
	}, nil)

	d := serverDeps{db: db}
	registerEntityTools(server, d)
	registerPriceTools(server, d)
	registerMissionTools(server, d)
	registerMetaTools(server, d)
	registerOrchestrationTools(server, d)
	return server
}

// serverDeps carries the shared state used by every tool registration group.
type serverDeps struct {
	db *sql.DB
}

// envResult returns structured map (not JSON string) so hosts that validate
// structuredContent as object succeed.
func (d serverDeps) envResult(env map[string]any, isErr bool) (*mcp.CallToolResult, map[string]any, error) {
	return &mcp.CallToolResult{IsError: isErr}, env, nil
}

func (d serverDeps) requireDB() (*mcp.CallToolResult, map[string]any, error) {
	return d.envResult(errEnv("database unavailable"), true)
}

func (d serverDeps) withFreshness(rows []map[string]any) map[string]any {
	meta := query.MetaFromRows(rows)
	if d.db == nil {
		return meta
	}
	return query.AttachFreshness(d.db, meta)
}
