from __future__ import annotations

import sqlite3


def search_docs(conn: sqlite3.Connection, query: str, limit: int = 5) -> list[dict]:
    q = query.strip()
    if not q:
        return []
    # FTS5 query: quote tokens loosely by joining with AND for simple phrases
    fts_query = " ".join(q.split())
    try:
        rows = conn.execute(
            """
            SELECT d.id, d.title, d.url, d.body, d.source, d.game_version, d.entity_id,
                   snippet(documents_fts, 1, '[', ']', '…', 24) AS snippet
            FROM documents_fts
            JOIN documents d ON d.id = documents_fts.rowid
            WHERE documents_fts MATCH ?
            ORDER BY rank
            LIMIT ?
            """,
            (fts_query, limit),
        ).fetchall()
    except sqlite3.OperationalError:
        # Fallback LIKE if FTS query syntax fails
        like = f"%{q}%"
        rows = conn.execute(
            """
            SELECT id, title, url, body, source, game_version, entity_id,
                   substr(body, 1, 160) AS snippet
            FROM documents
            WHERE title LIKE ? OR body LIKE ?
            LIMIT ?
            """,
            (like, like, limit),
        ).fetchall()
    return [dict(r) for r in rows]
