CREATE TABLE IF NOT EXISTS ingest_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL,
  game_version TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL,
  row_count INTEGER,
  notes TEXT
);

CREATE TABLE IF NOT EXISTS entities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  slug TEXT,
  uuid TEXT,
  game_version TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entities_uuid
  ON entities(kind, uuid, game_version)
  WHERE uuid IS NOT NULL AND uuid != '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_entities_name
  ON entities(kind, canonical_name, game_version);

CREATE TABLE IF NOT EXISTS aliases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id INTEGER NOT NULL REFERENCES entities(id),
  alias TEXT NOT NULL,
  lang TEXT NOT NULL,
  source TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias);

CREATE UNIQUE INDEX IF NOT EXISTS idx_aliases_unique
  ON aliases(entity_id, alias, lang, source);

-- P0 hot-path indexes (queries use COLLATE NOCASE; binary indexes do not hit).
CREATE INDEX IF NOT EXISTS idx_aliases_alias_nocase
  ON aliases(alias COLLATE NOCASE, entity_id);

CREATE INDEX IF NOT EXISTS idx_entities_canonical_nocase
  ON entities(canonical_name COLLATE NOCASE, kind);

CREATE TABLE IF NOT EXISTS facts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id INTEGER NOT NULL REFERENCES entities(id),
  key TEXT NOT NULL,
  value_text TEXT,
  value_num REAL,
  unit TEXT,
  source TEXT NOT NULL,
  game_version TEXT NOT NULL,
  ingest_run_id INTEGER REFERENCES ingest_runs(id)
);

CREATE INDEX IF NOT EXISTS idx_facts_entity_key
  ON facts(entity_id, key);

CREATE TABLE IF NOT EXISTS prices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  commodity_entity_id INTEGER NOT NULL REFERENCES entities(id),
  terminal_entity_id INTEGER NOT NULL REFERENCES entities(id),
  side TEXT NOT NULL,
  price REAL NOT NULL,
  scu_available REAL,
  reported_at TEXT,
  source TEXT NOT NULL,
  game_version TEXT NOT NULL,
  ingest_run_id INTEGER REFERENCES ingest_runs(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_prices_latest
  ON prices(commodity_entity_id, terminal_entity_id, side, source, game_version);

CREATE INDEX IF NOT EXISTS idx_prices_commodity_side
  ON prices(commodity_entity_id, side);

CREATE INDEX IF NOT EXISTS idx_prices_terminal
  ON prices(terminal_entity_id);

CREATE INDEX IF NOT EXISTS idx_prices_best
  ON prices(commodity_entity_id, side, price);

CREATE TABLE IF NOT EXISTS relations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  from_id INTEGER NOT NULL REFERENCES entities(id),
  to_id INTEGER NOT NULL REFERENCES entities(id),
  rel_type TEXT NOT NULL,
  qty REAL,
  meta_json TEXT,
  source TEXT NOT NULL,
  game_version TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_relations_from_type_to
  ON relations(from_id, rel_type, to_id);

CREATE INDEX IF NOT EXISTS idx_relations_to_type_from
  ON relations(to_id, rel_type, from_id);

CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id INTEGER REFERENCES entities(id),
  title TEXT NOT NULL,
  url TEXT,
  body TEXT NOT NULL,
  source TEXT NOT NULL,
  game_version TEXT NOT NULL,
  ingest_run_id INTEGER REFERENCES ingest_runs(id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
  title,
  body,
  content='documents',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS documents_ai AFTER INSERT ON documents BEGIN
  INSERT INTO documents_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS documents_ad AFTER DELETE ON documents BEGIN
  INSERT INTO documents_fts(documents_fts, rowid, title, body)
    VALUES('delete', old.id, old.title, old.body);
END;

CREATE TRIGGER IF NOT EXISTS documents_au AFTER UPDATE ON documents BEGIN
  INSERT INTO documents_fts(documents_fts, rowid, title, body)
    VALUES('delete', old.id, old.title, old.body);
  INSERT INTO documents_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
