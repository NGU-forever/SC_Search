#!/usr/bin/env bash
# Launch sc-search MCP over stdio. Prefers the Go binary; falls back to Python.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export SC_SEARCH_DB="${SC_SEARCH_DB:-$ROOT/data/sc_knowledge.sqlite}"
cd "$ROOT"

GO_BIN="$ROOT/go/sc-search"
if [[ ! -x "$GO_BIN" ]]; then
  if command -v go >/dev/null 2>&1; then
    (cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search)
  fi
fi
if [[ -x "$GO_BIN" ]]; then
  exec "$GO_BIN" mcp
fi

PY="$ROOT/.venv/bin/python"
if [[ ! -x "$PY" ]]; then
  echo "sc-search MCP: missing Go binary and $PY" >&2
  exit 1
fi
echo "sc-search MCP: Go binary missing; falling back to Python" >&2
exec "$PY" -m sc_search.mcp_server
