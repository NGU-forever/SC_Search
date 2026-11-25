#!/usr/bin/env bash
# Wire MCP configs for a ready tree (Release zip or already-built checkout).
# Does NOT build Go and does NOT sync/write the knowledge DB.
# Writes: config/mcp_config.json (from example + absolute paths), .mcp.json, .cursor/mcp.json
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

if ! command -v python3 >/dev/null 2>&1; then
  echo "需要 python3 以渲染 config/mcp_config.json" >&2
  exit 1
fi

python3 "$ROOT/scripts/render_mcp_config.py" "$ROOT" "$BIN" "$DB"

echo ""
echo "就绪（未编译、未改库）。请在客户端重载 MCP / 新开对话后再提问。"
echo "  权威配置: $ROOT/config/mcp_config.json"
echo "  客户端 hints: cursor / claude_code / claude_desktop / chatgpt / deepseek / kimi / zcode / antigravity / doubao / …"
echo "  二进制: ${BIN}"
echo "  数据库: ${DB}"
