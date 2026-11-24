#!/usr/bin/env bash
# Wire MCP configs for a ready tree (Release zip or already-built checkout).
# Does NOT build Go and does NOT sync/write the knowledge DB.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="$ROOT/go/sc-search"
if [[ ! -x "$BIN" ]]; then
  if [[ -x "$ROOT/go/sc-search.exe" ]]; then
    BIN="$ROOT/go/sc-search.exe"
  else
    echo "缺少可执行文件: $ROOT/go/sc-search" >&2
    echo "请使用 Release 解压包，或先 go build。" >&2
    exit 1
  fi
fi

DB="$ROOT/data/sc_knowledge.sqlite"
if [[ ! -f "$DB" ]]; then
  echo "缺少数据库: $DB" >&2
  echo "Release 包应自带；开发树请先 ./scripts/sync_all.sh" >&2
  exit 1
fi

write_mcp_json() {
  local dest="$1"
  mkdir -p "$(dirname "$dest")"
  cat >"$dest" <<EOF
{
  "mcpServers": {
    "sc-search": {
      "command": "$BIN",
      "args": ["mcp"],
      "cwd": "$ROOT",
      "env": {
        "SC_SEARCH_DB": "$DB"
      }
    }
  }
}
EOF
  echo "已写入 $dest"
}

write_mcp_json "$ROOT/.mcp.json"
write_mcp_json "$ROOT/.cursor/mcp.json"

echo ""
echo "就绪（未编译、未改库）。请在客户端重载 MCP / 新开对话后再提问。"
echo "  二进制: ${BIN}"
echo "  数据库: ${DB}"
echo "豆包等工作端: STDIO, command=${BIN}, args=mcp, SC_SEARCH_DB=${DB}"
