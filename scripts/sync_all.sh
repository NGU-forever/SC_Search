#!/usr/bin/env bash
# 一键初始化并同步 SC Search 本地知识库（Go）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="$ROOT/go/sc-search"
if [[ ! -x "$BIN" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "未找到 go 与 $BIN。请安装 Go 后重试。" >&2
    exit 1
  fi
  (cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search)
fi

echo "==> 使用: $BIN"
echo "==> 项目: $ROOT"

if [[ ! -f "$ROOT/config/.env" && -f "$ROOT/config/.env.example" ]]; then
  cp "$ROOT/config/.env.example" "$ROOT/config/.env"
  echo "已创建 config/.env（可填入 UEX_API_TOKEN 以同步物价）"
fi

run() {
  echo ""
  echo "==> $*"
  "$BIN" "$@"
}

run ingest init

set +e
run ingest sync uex
UEX_EC=$?
set -e
if [[ "$UEX_EC" -ne 0 ]]; then
  echo "（跳过）uex 同步失败——若只需任务/飞船/中文，可忽略；查物价请配置 config/.env 中 UEX_API_TOKEN"
fi

run ingest sync scmdb live
run ingest sync scw
run ingest sync wiki
run ingest sync aliases

set +e
run ingest sync localization
ZH_EC=$?
set -e
if [[ "$ZH_EC" -ne 0 ]]; then
  echo "（跳过）localization 同步失败——可稍后重试；若 data/raw 已有 global.*.ini 缓存，一般会自动回退使用"
fi

echo ""
echo "==> 同步完成，最近状态："
run ingest status

echo ""
echo "完成。请在 Cursor 中重载 MCP（sc-search）后再提问。"
