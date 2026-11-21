#!/usr/bin/env bash
# 一键初始化并同步 SC Search 本地知识库
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -x "$ROOT/.venv/bin/python" ]]; then
  PY="$ROOT/.venv/bin/python"
elif command -v python3 >/dev/null 2>&1; then
  PY="python3"
else
  echo "未找到 Python。请先按 README 创建 .venv 并 pip install -e ." >&2
  exit 1
fi

echo "==> 使用: $PY"
echo "==> 项目: $ROOT"

if [[ ! -f "$ROOT/config/.env" && -f "$ROOT/config/.env.example" ]]; then
  cp "$ROOT/config/.env.example" "$ROOT/config/.env"
  echo "已创建 config/.env（可填入 UEX_API_TOKEN 以同步物价）"
fi

run() {
  echo ""
  echo "==> $*"
  "$PY" "$@"
}

run -m sc_search.ingest init

# UEX 需要 Token；失败不中断后续
set +e
run -m sc_search.ingest sync uex
UEX_EC=$?
set -e
if [[ "$UEX_EC" -ne 0 ]]; then
  echo "（跳过）uex 同步失败——若只需任务/飞船/中文，可忽略；查物价请配置 config/.env 中 UEX_API_TOKEN"
fi

run -m sc_search.ingest sync scmdb --channel live
run -m sc_search.ingest sync scw
run -m sc_search.ingest sync wiki
run -m sc_search.ingest sync aliases
run -m sc_search.ingest sync zh

echo ""
echo "==> 同步完成，最近状态："
run -m sc_search.ingest status

echo ""
echo "完成。请在 Cursor 中重载 MCP（sc-search）后再提问。"
