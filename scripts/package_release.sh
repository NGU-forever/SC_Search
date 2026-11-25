#!/usr/bin/env bash
# Build a Release zip: prebuilt binary + prefilled SQLite + docs/scripts.
# Usage:
#   ./scripts/package_release.sh [--version v0.1.0] [--goos darwin] [--goarch arm64]
#   ./scripts/package_release.sh --skip-build --bin /path/to/sc-search --db /path/to.sqlite
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION=""
GOOS_T="${GOOS:-$(go env GOOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')}"
GOARCH_T="${GOARCH:-$(go env GOARCH 2>/dev/null || uname -m)}"
SKIP_BUILD=0
BIN_IN=""
DB_IN=""
OUT_DIR="$ROOT/dist"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --goos) GOOS_T="$2"; shift 2 ;;
    --goarch) GOARCH_T="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --bin) BIN_IN="$2"; shift 2 ;;
    --db) DB_IN="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,6p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

# Normalize uname arch names when go env unavailable
case "$GOOS_T" in
  darwin|linux|windows) ;;
  Darwin) GOOS_T=darwin ;;
  Linux) GOOS_T=linux ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT) GOOS_T=windows ;;
esac
case "$GOARCH_T" in
  amd64|arm64|386) ;;
  x86_64|x64) GOARCH_T=amd64 ;;
  aarch64|arm64) GOARCH_T=arm64 ;;
esac

if [[ -z "$VERSION" ]]; then
  if git describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git describe --tags --exact-match)"
  elif git rev-parse --short HEAD >/dev/null 2>&1; then
    VERSION="dev-$(git rev-parse --short HEAD)"
  else
    VERSION="dev"
  fi
fi
VERSION="${VERSION#v}"

DB_SRC="${DB_IN:-$ROOT/data/sc_knowledge.sqlite}"
if [[ ! -f "$DB_SRC" ]]; then
  echo "缺少数据库: $DB_SRC（先 sync，或传 --db）" >&2
  exit 1
fi

EXT=""
BIN_NAME="sc-search"
if [[ "$GOOS_T" == "windows" ]]; then
  EXT=".exe"
  BIN_NAME="sc-search.exe"
fi

STAGE="$ROOT/release-staging/sc-search-${VERSION}-${GOOS_T}-${GOARCH_T}"
rm -rf "$STAGE"
mkdir -p "$STAGE/go" "$STAGE/data" "$STAGE/config" "$STAGE/scripts"

if [[ "$SKIP_BUILD" -eq 1 ]]; then
  if [[ -z "$BIN_IN" || ! -f "$BIN_IN" ]]; then
    echo "--skip-build 需要有效的 --bin" >&2
    exit 1
  fi
  cp "$BIN_IN" "$STAGE/go/$BIN_NAME"
else
  if ! command -v go >/dev/null 2>&1; then
    echo "需要 Go 才能打包，或使用 --skip-build --bin ..." >&2
    exit 1
  fi
  echo "==> build ${GOOS_T}/${GOARCH_T}"
  (
    cd "$ROOT/go"
    CGO_ENABLED=0 GOOS="$GOOS_T" GOARCH="$GOARCH_T" \
      go build -trimpath -ldflags="-s -w" -o "$STAGE/go/$BIN_NAME" ./cmd/sc-search
  )
fi
chmod +x "$STAGE/go/$BIN_NAME" 2>/dev/null || true

# Marker so FindRoot works without full Go sources in the zip
printf 'sc-search release %s (%s/%s)\n' "$VERSION" "$GOOS_T" "$GOARCH_T" >"$STAGE/.sc-search-root"
# Keep go.mod so older binaries / docs that look for go/go.mod still resolve
cp "$ROOT/go/go.mod" "$STAGE/go/go.mod"

cp "$DB_SRC" "$STAGE/data/sc_knowledge.sqlite"
[[ -f "$ROOT/data/community_guides.json" ]] && cp "$ROOT/data/community_guides.json" "$STAGE/data/"
[[ -f "$ROOT/data/seed_aliases.json" ]] && cp "$ROOT/data/seed_aliases.json" "$STAGE/data/"
[[ -f "$ROOT/data/.gitkeep" ]] && cp "$ROOT/data/.gitkeep" "$STAGE/data/"

cp "$ROOT/config/mcp_config.example.json" "$STAGE/config/"
cp "$ROOT/config/mcp.cursor.example.json" "$STAGE/config/"
cp "$ROOT/config/.env.example" "$STAGE/config/"

cp "$ROOT/scripts/setup_mcp.sh" "$STAGE/scripts/"
cp "$ROOT/scripts/setup_mcp.ps1" "$STAGE/scripts/"
cp "$ROOT/scripts/render_mcp_config.py" "$STAGE/scripts/"
cp "$ROOT/scripts/run_mcp.sh" "$STAGE/scripts/"
cp "$ROOT/scripts/run_mcp.ps1" "$STAGE/scripts/"
# Dev-only sync scripts are optional for advanced users refreshing data
cp "$ROOT/scripts/sync_all.sh" "$STAGE/scripts/"
cp "$ROOT/scripts/sync_all.ps1" "$STAGE/scripts/"
chmod +x "$STAGE/scripts/"*.sh

for f in README.md AGENTS.md ORCHESTRATION.md SOURCES.md LICENSE; do
  [[ -f "$ROOT/$f" ]] && cp "$ROOT/$f" "$STAGE/"
done

cat >"$STAGE/RELEASE.txt" <<EOF
SC Search ${VERSION} (${GOOS_T}/${GOARCH_T})

解压后任选其一：
  1) 对 Agent 说「初始化」——生成 config/mcp_config.json 并按当前客户端适配 MCP；不编译、不同步库
  2) 手动: ./scripts/setup_mcp.sh   (Windows: .\\scripts\\setup_mcp.ps1)

然后在客户端重载 MCP。查实时物价可选填 config/.env 的 UEX_API_TOKEN 后再 sync。
EOF

mkdir -p "$OUT_DIR"
ZIP="$OUT_DIR/sc-search-${VERSION}-${GOOS_T}-${GOARCH_T}.zip"
rm -f "$ZIP"
(
  cd "$(dirname "$STAGE")"
  zip -qr "$ZIP" "$(basename "$STAGE")"
)

echo "==> $ZIP"
ls -lh "$ZIP"
