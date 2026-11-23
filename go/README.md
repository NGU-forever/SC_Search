# SC Search (Go)

Dual-track Go MCP server.

**Native in Go:** resolve, prices, freshness, entity facts, missions, locations, docs,
intents, route, worker, query_plan, and full `sc_answer` (including guides/blueprint/loadout).

**Still Python:** ingest CLI/adapters (P4). `ErrUseBridge` remains only as a safety fallback
for unexpected answer intents.

## Build

```bash
cd go
go build -o sc-search ./cmd/sc-search
```

## Run MCP (stdio)

From the repo root (or any subdirectory; the binary finds the root):

```bash
./go/sc-search mcp
```

Cursor / client config example:

```json
{
  "mcpServers": {
    "sc-search": {
      "command": "/ABS/PATH/SC_Search/go/sc-search",
      "args": ["mcp"]
    }
  }
}
```

## Smoke

```bash
./go/sc-search bridge-smoke
```

## Tests

```bash
cd go && go test ./...
```
