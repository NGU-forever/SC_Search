# Answer Split + Go Migration Implementation Plan

> **For agentic workers:** Cursor implements; Antigravity Gemini 3.8 Flash tests + commits after each phase checkpoint.

**Goal:** Behavior-preserving split of `answer.py` (P0), then dual-track full Go rewrite (P1–P5).

**Architecture:** Layer-cut migration with per-tool parity tests. Shared SQLite + frozen MCP envelope.

**Tech Stack:** Python 3.11+ (existing), Go + official `modelcontextprotocol/go-sdk`, SQLite, pytest / `go test`.

## Global Constraints

- Do not change `answer_question` signature or response dict shape in P0.
- Do not change MCP tool names or `{ok,data,meta,error}` envelope.
- Schema under `src/sc_search/ingest/schema.sql` frozen until an explicit P4 schema task.
- Commits after a checkpoint are done by Antigravity Gemini 3.8 Flash, not Cursor, unless the user says otherwise.

---

## Task 1: P0 — Split `answer.py` into package

**Files:**
- Create `src/sc_search/query/answer/constants.py`
- Create `src/sc_search/query/answer/evidence.py`
- Create `src/sc_search/query/answer/draft.py`
- Create `src/sc_search/query/answer/acquisition.py`
- Create `src/sc_search/query/answer/confidence.py`
- Create `src/sc_search/query/answer/core.py`
- Create `src/sc_search/query/answer/__init__.py`
- Delete `src/sc_search/query/answer.py`

- [ ] Move helpers into modules listed in the design spec (no behavior change)
- [ ] Keep `from sc_search.query.answer import answer_question` working
- [ ] Cursor: `pytest tests/test_answer.py tests/test_uex_vehicle_prices.py -q` green locally
- [ ] Gemini: re-run tests + commit `refactor(query): split answer.py into package modules`

## Task 2: P1 — Go module + MCP shell

**Files:**
- `go/go.mod`, `go/cmd/sc-search/main.go`, `go/internal/mcp/...`, `go/internal/db/...`

- [ ] `go mod init` (module path `github.com/...` or local `sc-search`)
- [ ] Stdio MCP server registering current tool names (handlers may bridge to Python initially)
- [ ] Envelope helpers matching Python `_ok` / `_err` / `_no_match`
- [ ] Document Cursor MCP command for the Go binary
- [ ] Gemini: `go test ./...` + smoke + commit

## Task 3: P2 — Port query tools

Order: resolve → prices → freshness → entities → missions → locations → docs → plan → answer.

- [ ] For each tool: Go impl + parity test vs Python fixtures
- [ ] Flip tool to native Go when green
- [ ] Gemini test+commit per tool or small batches

## Task 4: P3 — Router / worker

- [ ] Port `sc_intents`, `sc_route`, `sc_worker_run`
- [ ] Parity tests from `tests/test_worker_*.py` / intent tests
- [ ] Gemini test+commit

## Task 5: P4 — Ingest

- [ ] Port schema init + adapters (scmdb, uex, scw, wiki, localization, aliases)
- [ ] `sc-search ingest ...` CLI parity
- [ ] Gemini test+commit

## Task 6: P5 — Cutover

- [ ] Default MCP → Go binary
- [ ] Update `ORCHESTRATION.md` / `AGENTS.md` / README
- [ ] Remove or archive Python runtime
- [ ] Gemini final test+commit
