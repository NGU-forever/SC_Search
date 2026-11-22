#!/usr/bin/env bash
# Launch sc-search MCP over stdio. Resolves repo root from this script
# so MCP hosts do not need a correct relative cwd or resolved venv symlink.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PY="$ROOT/.venv/bin/python"
if [[ ! -x "$PY" ]]; then
  echo "sc-search MCP: missing $PY (create .venv and pip install -e .)" >&2
  exit 1
fi
export SC_SEARCH_DB="${SC_SEARCH_DB:-$ROOT/data/sc_knowledge.sqlite}"
cd "$ROOT"
exec "$PY" -m sc_search.mcp_server
