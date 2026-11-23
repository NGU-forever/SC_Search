# SC Search (Go)

Dual-track Go MCP server. **P1:** tools bridge to the existing Python handlers via  
`python -m sc_search.mcp_server.bridge`.

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
