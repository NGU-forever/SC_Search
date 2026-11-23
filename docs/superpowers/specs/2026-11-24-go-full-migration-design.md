# SC Search — Full Go Migration Design

**Date:** 2026-11-24  
**Status:** In progress — P0–P3 done; P4 most adapters (scmdb/scw remain); P5 cutover

## Progress (2026-11-24)

| Phase | Status | Notes / tip commits |
|-------|--------|---------------------|
| P0 answer.py split | done | `f338e8a` |
| P1 Go MCP + bridge | done | `22192d7` |
| P2 native query tools | done | prices/facts/missions/docs/plan/answer … through `3c960b8` |
| P3 router/worker | done | `189b27b`, `7663970` |
| P4 ingest | partial | `sync aliases|wiki|localization|uex`; scmdb/scw still Python |
| P5 cutover | not started | Point Cursor MCP at `go/sc-search mcp`; retire Python runtime |

## Goals

1. Split oversized Python `answer.py` into focused modules (behavior-preserving).
2. Rewrite MCP, query, router/worker, and ingest in Go under `go/`.
3. Dual-track cutover: Python remains default until each Go tool passes parity tests.
4. Workflow: Cursor implements; Antigravity Gemini 3.8 Flash runs tests and commits.

## Non-goals (this migration)

- Changing MCP tool names or `{ok, data, meta, error}` response envelope.
- Changing SQLite schema or `data/` layout.
- Rewriting browser orchestration / Erkul flows (docs stay; agents still drive browser).

## Constraints

- Shared DB: `data/sc_knowledge.sqlite` (schema frozen during dual-track).
- MCP tools stay discoverable under the same names (`sc_answer`, `sc_route`, …).
- Cutover unit = one tool (or one finished layer), never a half-port.
- Go lives in-repo under `go/` (`cmd/sc-search`, `internal/...`); Python stays in `src/` until P5.

## Phases

| Phase | Deliverable | Exit criteria |
|-------|-------------|-----------------|
| P0 | Python `query/answer/` package split | Existing pytest green; `answer_question` API unchanged |
| P1 | Go MCP shell + response envelope; optional Python bridge | Cursor can launch Go MCP; tool names/envelope match |
| P2 | Go `internal/query` tool-by-tool | Parity tests vs Python per tool; flip tools as they pass |
| P3 | Go router/worker (`sc_route`, `sc_worker_run`, intents) | Contract parity with Python |
| P4 | Go ingest CLI + adapters | Can rebuild equivalent SQLite |
| P5 | Default Cursor config → Go; remove Python runtime | Single Go binary sufficient |

## Repo layout

```text
SC_Search/
  src/sc_search/          # Python (dual-track)
  go/
    go.mod
    cmd/sc-search/        # mcp | ingest
    internal/mcp/
    internal/query/
    internal/router/
    internal/worker/
    internal/ingest/
    internal/db/
  data/                   # shared
  tests/                  # Python tests until P5
  docs/superpowers/
```

## P0 — `answer.py` split (detail)

Convert `src/sc_search/query/answer.py` → package `src/sc_search/query/answer/`:

| Module | Responsibility |
|--------|----------------|
| `__init__.py` | Public export: `answer_question` |
| `core.py` | Orchestrator `answer_question` only |
| `constants.py` | Regexes, limits, ZH maps, guide intent sets |
| `evidence.py` | Evidence builders, slim_*, pick_facts, display helpers |
| `draft.py` | Chinese draft strings / live suffix / guide links |
| `acquisition.py` | Blueprint/shop SQL helpers + acquisition evidence |
| `confidence.py` | Confidence + soft-verify |

Import path remains `from sc_search.query.answer import answer_question`.

## Dual-track cutover

1. Implement Go tool behind feature flag or separate binary command.
2. Golden / side-by-side tests against Python for the same fixtures.
3. When green, Cursor MCP config points that tool (or whole server) at Go.
4. Keep Python until P5 for rollback.

## Agent workflow

- **Cursor (this thread):** implement code per phase.
- **Antigravity / `gemini-3.8-flash`:** after each phase checkpoint — run tests, fix only test-harness issues if needed, **commit** with a clear message. Does not redesign architecture.

## Success (overall)

- Go MCP serves all current tools with equivalent JSON.
- Ingest rebuilds DB from the same sources.
- Python package removed or archived; docs (`ORCHESTRATION.md`, `AGENTS.md`) updated to Go entrypoints.
