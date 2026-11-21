# SC Search Phase C (Wiki + aliases + FTS) Implementation Plan

> Cursor implements; Antigravity `gemini-3.8-flash` verifies/commits.

**Goal:** Ingest ship (and light item) facts from `api.star-citizen.wiki`, English wiki extracts into FTS documents, Chinese/slang aliases via seed (+ soft-fail CitizenWiki), expose `sc_search_docs`.

**Note:** `citizenwiki.cn` returns Cloudflare 403 to automated HTTP. Phase C uses `data/seed_aliases.json` for 俗称; CitizenWiki adapter tries API and records failed run without crashing.

**CLI:** `sync scw`, `sync wiki`, `sync aliases`

**MCP:** `sc_search_docs(query, limit=5)`
