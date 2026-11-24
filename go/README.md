# SC Search (Go)

Go-only MCP server + ingest CLI for the Star Citizen local knowledge base.

**MCP tools:** resolve, prices, freshness, entity facts, missions, locations, docs,
intents, route, worker, query_plan, and full `sc_answer`.

**Ingest:** `sc-search ingest init|status|sync` for
`aliases|wiki|localization|uex|scmdb|scw|scw-blueprints` (`zh` is an alias of `localization`).

## Build

```bash
cd go
go build -o sc-search ./cmd/sc-search
```

End users: download a GitHub Release zip (binary + SQLite) and run `../scripts/setup_mcp.sh` — no Go required.

## Run MCP (stdio)

```bash
./go/sc-search mcp
# or
./scripts/run_mcp.sh
./scripts/setup_mcp.sh   # write .mcp.json / .cursor/mcp.json only
```

Cursor / client config:

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

## Tests

```bash
cd go && go test ./...
```
