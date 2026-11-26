package ingest

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var uexParenSystemRE = regexp.MustCompile(`(?i)\((stanton|pyro|nyx|terra)\)`)

// Lightweight terminal→system heuristic (mirrors query.InferSystemFromTerminalName; keep ingest free of query import).

func inferSystemFromTerminalName(terminalName string) string {
	name := strings.ToLower(strings.TrimSpace(terminalName))
	if name == "" {
		return ""
	}
	if m := uexParenSystemRE.FindStringSubmatch(name); len(m) == 2 {
		return strings.ToLower(m[1])
	}
	pois := []struct {
		system string
		tokens []string
	}{
		{"stanton", []string{"area 18", "lorville", "new babbage", "orison", "grim hex", "everus", "hur-l", "cru-l", "arc-l", "mic-l", "hurston", "arccorp", "microtech", "crusader"}},
		{"pyro", []string{"checkmate", "ruin station", "orbituary", "bloom", "terminus", "gaslight", "patch city", "rat's nest", "rab-", "starlight"}},
		{"nyx", []string{"nyx gateway"}},
	}
	for _, p := range pois {
		if strings.Contains(name, p.system) {
			return p.system
		}
		for _, tok := range p.tokens {
			if strings.Contains(name, tok) {
				return p.system
			}
		}
	}
	return ""
}

// UexPayload is the offline/test input for ApplyUex.

func pickGameVersion(groups ...[]map[string]any) string {
	for _, g := range groups {
		for _, item := range g {
			if v := anyx.String(item["game_version"]); v != "" {
				return v
			}
		}
	}
	return ""
}

func maybeFloat(v any) *float64 {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		return &t
	case int:
		f := float64(t)
		return &f
	case int64:
		f := float64(t)
		return &f
	case string:
		if t == "" {
			return nil
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func asIntID(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(t)
		return i, err == nil
	default:
		return 0, false
	}
}

func findExistingShipID(conn DBTX, names ...string) (int64, bool) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var id int64
		err := conn.QueryRow(`
			SELECT e.id FROM entities e
			WHERE e.kind = 'ship' AND e.canonical_name = ?
			ORDER BY CASE WHEN e.game_version LIKE 'scw%' THEN 0 ELSE 1 END, e.id
			LIMIT 1
		`, name).Scan(&id)
		if err == nil {
			return id, true
		}
		err = conn.QueryRow(`
			SELECT e.id FROM aliases a
			JOIN entities e ON e.id = a.entity_id
			WHERE e.kind = 'ship' AND a.alias = ? COLLATE NOCASE
			ORDER BY CASE WHEN e.game_version LIKE 'scw%' THEN 0 ELSE 1 END, e.id
			LIMIT 1
		`, name).Scan(&id)
		if err == nil {
			return id, true
		}
	}
	return 0, false
}

// ApplyUex writes terminals/commodities/prices (+ optional vehicles) without HTTP.
