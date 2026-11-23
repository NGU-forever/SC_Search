#!/usr/bin/env bash
# Launch sc-search MCP over stdio (Go).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export SC_SEARCH_DB="${SC_SEARCH_DB:-$ROOT/data/sc_knowledge.sqlite}"
cd "$ROOT"

GO_BIN="$ROOT/go/sc-search"
if [[ ! -x "$GO_BIN" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "sc-search MCP: missing Go binary and go toolchain" >&2
    exit 1
  fi
  (cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search)
fi
exec "$GO_BIN" mcp
