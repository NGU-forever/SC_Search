#!/usr/bin/env python3
"""Render config/mcp_config.json from mcp_config.example.json with absolute paths.

Also writes project .mcp.json and .cursor/mcp.json (mcpServers only).
"""
from __future__ import annotations

import json
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 4:
        print(
            "usage: render_mcp_config.py <ROOT> <BIN> <DB>",
            file=sys.stderr,
        )
        return 2
    root = Path(sys.argv[1]).resolve()
    bin_path = Path(sys.argv[2]).resolve()
    db_path = Path(sys.argv[3]).resolve()
    example = root / "config" / "mcp_config.example.json"
    if not example.is_file():
        print(f"missing template: {example}", file=sys.stderr)
        return 1

    data = json.loads(example.read_text(encoding="utf-8"))
    server = {
        "name": "sc-search",
        "transport": "stdio",
        "command": str(bin_path),
        "args": ["mcp"],
        "cwd": str(root),
        "env": {"SC_SEARCH_DB": str(db_path)},
    }
    mcp_servers = {
        "sc-search": {
            "command": str(bin_path),
            "args": ["mcp"],
            "cwd": str(root),
            "env": {"SC_SEARCH_DB": str(db_path)},
        }
    }
    data["description"] = (
        "本机绝对路径由 setup_mcp / render_mcp_config.py 生成。"
        "Agent「初始化」读取此文件，按 client_hints 适配当前客户端。"
    )
    data["server"] = server
    data["mcpServers"] = mcp_servers

    # Fill placeholder fields for UI-oriented clients.
    hints = data.get("client_hints") or {}
    doubao = hints.get("doubao") or {}
    fields = doubao.get("fields")
    if isinstance(fields, dict):
        fields["命令"] = str(bin_path)
        fields["环境变量"] = f"SC_SEARCH_DB={db_path}"
        doubao["fields"] = fields
        hints["doubao"] = doubao
    chatgpt = hints.get("chatgpt") or {}
    if "toml_example" in chatgpt:
        chatgpt["toml_example"] = (
            "[mcp_servers.sc-search]\n"
            f'command = "{bin_path}"\n'
            'args = ["mcp"]\n'
            f'env = {{ SC_SEARCH_DB = "{db_path}" }}'
        )
        hints["chatgpt"] = chatgpt
    zcode = hints.get("zcode") or {}
    native = zcode.get("native_shape")
    if isinstance(native, dict):
        zcode["native_shape"] = {
            "mcp": {
                "servers": {
                    "sc-search": {
                        "command": str(bin_path),
                        "args": ["mcp"],
                        "env": {"SC_SEARCH_DB": str(db_path)},
                    }
                }
            }
        }
        hints["zcode"] = zcode
    data["client_hints"] = hints

    out = root / "config" / "mcp_config.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(
        json.dumps(data, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"已写入 {out}")

    servers_only = {"mcpServers": mcp_servers}
    for rel in (".mcp.json", ".cursor/mcp.json"):
        dest = root / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(
            json.dumps(servers_only, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        print(f"已写入 {dest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
