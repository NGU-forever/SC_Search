package query

import (
	"database/sql"
	"fmt"
)

// Queryer is satisfied by *sql.DB and *sql.Tx (read helpers reused during ingest).
type Queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func scanMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeSQL(raw[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func normalizeSQL(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	case int64:
		return t
	case float64:
		return t
	case string:
		return t
	case bool:
		return t
	default:
		return fmt.Sprint(t)
	}
}
