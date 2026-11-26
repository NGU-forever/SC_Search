package query

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var nameTokenStop = map[string]struct{}{
	"blueprint": {}, "blueprints": {}, "crafting": {}, "craft": {}, "materials": {}, "material": {},
	"where": {}, "what": {}, "when": {}, "with": {}, "from": {}, "this": {}, "that": {}, "have": {},
	"need": {}, "make": {}, "made": {}, "obtain": {}, "price": {}, "prices": {}, "buy": {}, "sell": {},
	"loadout": {}, "rifle": {}, "pistol": {}, "energy": {}, "assault": {}, "weapon": {}, "weapons": {},
}

var cjkSingleAllow = map[rune]struct{}{
	'铝': {}, '铁': {}, '钨': {}, '金': {}, '铜': {}, '钛': {}, '钢': {}, '锡': {},
	'铅': {}, '锌': {}, '钴': {}, '镍': {}, '镁': {}, '锂': {}, '铍': {}, '银': {},
}

func entityBrief(m map[string]any) map[string]any {
	brief := map[string]any{
		"id": m["id"], "kind": m["kind"], "canonical_name": m["canonical_name"],
	}
	if anyx.String(m["matched_lang"]) == "zh" {
		if alias := anyx.String(m["matched_alias"]); alias != "" {
			brief["zh"] = alias
		}
	}
	return brief
}

func aliasLenOK(alias string) bool {
	a := strings.TrimSpace(alias)
	if a == "" {
		return false
	}
	if isASCII(a) {
		return len(a) >= 3
	}
	runes := []rune(a)
	if len(runes) == 1 {
		_, ok := cjkSingleAllow[runes[0]]
		return ok
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func resolveEntities(db *sql.DB, hint, question string) ([]map[string]any, error) {
	seen := map[int64]bool{}
	out := []map[string]any{}
	add := func(m map[string]any) {
		eid := asInt64(m["id"])
		if seen[eid] {
			return
		}
		seen[eid] = true
		out = append(out, entityBrief(m))
	}
	for _, t := range []string{hint, question} {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		hits, err := ResolveName(db, t)
		if err != nil {
			return nil, err
		}
		for _, m := range hits {
			add(m)
		}
	}
	q := strings.TrimSpace(question)
	if q != "" {
		preferBP := wantsBlueprintPrefer(q)
		tokens := extractShortcodes(q)
		if preferBP {
			tokens = append(tokens, extractNameTokens(q)...)
		}
		for _, tok := range tokens {
			hits, err := ResolveName(db, tok)
			if err != nil {
				return nil, err
			}
			if preferBP {
				seenHit := map[int64]bool{}
				for _, m := range hits {
					seenHit[asInt64(m["id"])] = true
				}
				bps, err := resolveBlueprintsForToken(db, tok)
				if err != nil {
					return nil, err
				}
				for _, m := range bps {
					id := asInt64(m["id"])
					if !seenHit[id] {
						hits = append(hits, m)
						seenHit[id] = true
					}
				}
			}
			for _, m := range hits {
				add(m)
			}
		}
		embedded, err := entitiesEmbeddedInQuestion(db, q)
		if err != nil {
			return nil, err
		}
		for _, m := range embedded {
			add(m)
		}
	}
	return preferBlueprintPrimary(out, q), nil
}

func entitiesEmbeddedInQuestion(db *sql.DB, question string) ([]map[string]any, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       a.alias AS matched_alias, a.lang AS matched_lang
		FROM aliases a
		JOIN entities e ON e.id = a.entity_id
		WHERE (? LIKE '%' || a.alias || '%') COLLATE NOCASE
		  AND length(trim(a.alias)) >= 1
		ORDER BY length(a.alias) DESC
		LIMIT 25
	`, q)
	if err != nil {
		return nil, err
	}
	items, err := scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	filtered := []map[string]any{}
	for _, r := range items {
		if aliasLenOK(anyx.String(r["matched_alias"])) {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		rows2, err := db.Query(`
			SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
			       e.canonical_name AS matched_alias, 'en' AS matched_lang
			FROM entities e
			WHERE (? LIKE '%' || e.canonical_name || '%') COLLATE NOCASE
			  AND length(trim(e.canonical_name)) >= 2
			ORDER BY length(e.canonical_name) DESC
			LIMIT 25
		`, q)
		if err != nil {
			return nil, err
		}
		items, err = scanMaps(rows2)
		rows2.Close()
		if err != nil {
			return nil, err
		}
		for _, r := range items {
			if aliasLenOK(anyx.String(r["matched_alias"])) {
				filtered = append(filtered, r)
			}
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	ranked := rankRows(filtered)
	rankOf := func(kind string) int {
		if r, ok := kindRank[kind]; ok {
			return r
		}
		return 99
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		li := len(anyx.String(ranked[i]["matched_alias"]))
		lj := len(anyx.String(ranked[j]["matched_alias"]))
		if li != lj {
			return li > lj
		}
		ki := rankOf(anyx.String(ranked[i]["kind"]))
		kj := rankOf(anyx.String(ranked[j]["kind"]))
		if ki != kj {
			return ki < kj
		}
		return anyx.String(ranked[i]["canonical_name"]) < anyx.String(ranked[j]["canonical_name"])
	})
	return ranked, nil
}

func extractShortcodes(question string) []string {
	found := []string{}
	seen := map[string]bool{}
	for _, loc := range shortcodeRE.FindAllStringSubmatchIndex(question, -1) {
		if len(loc) < 4 {
			continue
		}
		start, end := loc[2], loc[3]
		if !tokenBoundaryOK(question, start, end) {
			continue
		}
		tok := question[start:end]
		key := strings.ToLower(tok)
		if seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, tok)
	}
	return found
}

func extractNameTokens(question string) []string {
	found := []string{}
	seen := map[string]bool{}
	for _, loc := range nameTokenRE.FindAllStringSubmatchIndex(question, -1) {
		if len(loc) < 4 {
			continue
		}
		start, end := loc[2], loc[3]
		if !tokenBoundaryOK(question, start, end) {
			continue
		}
		tok := question[start:end]
		key := strings.ToLower(tok)
		if _, stop := nameTokenStop[key]; stop || seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, tok)
	}
	return found
}

func tokenBoundaryOK(s string, start, end int) bool {
	if start > 0 {
		prev := s[start-1]
		if (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
			return false
		}
	}
	if end < len(s) {
		next := s[end]
		if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
			return false
		}
	}
	return true
}

func resolveBlueprintsForToken(db *sql.DB, token string) ([]map[string]any, error) {
	tok := strings.TrimSpace(token)
	if len(tok) < 2 {
		return nil, nil
	}
	like := "%" + tok + "%"
	rows, err := db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       COALESCE(a.alias, e.canonical_name) AS matched_alias,
		       COALESCE(a.lang, 'en') AS matched_lang
		FROM entities e
		LEFT JOIN aliases a ON a.entity_id = e.id
		WHERE e.kind = 'blueprint'
		  AND (
		    e.canonical_name LIKE ? COLLATE NOCASE
		    OR a.alias LIKE ? COLLATE NOCASE
		  )
		ORDER BY length(e.canonical_name) ASC
		LIMIT 25
	`, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}

func wantsBlueprintPrefer(q string) bool {
	return blueprintCtxRE.MatchString(q) || obtainCtxRE.MatchString(q)
}

func preferBlueprintPrimary(entities []map[string]any, question string) []map[string]any {
	if len(entities) == 0 || !wantsBlueprintPrefer(question) {
		return entities
	}
	bps := []map[string]any{}
	rest := []map[string]any{}
	for _, e := range entities {
		if anyx.String(e["kind"]) == "blueprint" {
			bps = append(bps, e)
		} else {
			rest = append(rest, e)
		}
	}
	if len(bps) == 0 {
		return entities
	}
	tokens := append(extractShortcodes(question), extractNameTokens(question)...)
	sort.SliceStable(bps, func(i, j int) bool {
		return blueprintSortLess(bps[i], bps[j], tokens)
	})
	return append(bps, rest...)
}

func blueprintSortLess(a, b map[string]any, tokens []string) bool {
	ka := blueprintSortKey(a, tokens)
	kb := blueprintSortKey(b, tokens)
	if ka.skin != kb.skin {
		return ka.skin < kb.skin
	}
	if ka.tokenRank != kb.tokenRank {
		return ka.tokenRank < kb.tokenRank
	}
	if ka.nameLen != kb.nameLen {
		return ka.nameLen < kb.nameLen
	}
	return ka.lower < kb.lower
}

type bpKey struct {
	skin, tokenRank, nameLen int
	lower                    string
}

func blueprintSortKey(entity map[string]any, tokens []string) bpKey {
	name := anyx.String(entity["canonical_name"])
	lower := strings.ToLower(name)
	skin := 0
	if strings.Contains(name, `"`) || strings.Contains(name, "“") || strings.Contains(name, "”") {
		skin = 1
	}
	tokenRank := 99
	for i, tok := range tokens {
		t := strings.ToLower(tok)
		if lower == t || strings.HasPrefix(lower, t+" ") || strings.HasPrefix(lower, t+"-") {
			if i < tokenRank {
				tokenRank = i
			}
		}
	}
	return bpKey{skin: skin, tokenRank: tokenRank, nameLen: len(name), lower: lower}
}

func commodityOrTerminalPrimary(entities []map[string]any) bool {
	return hasCommodityOrTerminal(entities)
}

func hasCommodityOrTerminal(entities []map[string]any) bool {
	if len(entities) == 0 {
		return true
	}
	for _, e := range entities {
		k := anyx.String(e["kind"])
		if k == "commodity" || k == "terminal" {
			return true
		}
	}
	return false
}

func preferCommodityEntities(entities []map[string]any) []map[string]any {
	if len(entities) == 0 {
		return entities
	}
	preferred := []map[string]any{}
	rest := []map[string]any{}
	for _, e := range entities {
		k := anyx.String(e["kind"])
		if k == "commodity" || k == "terminal" {
			preferred = append(preferred, e)
		} else {
			rest = append(rest, e)
		}
	}
	if len(preferred) == 0 {
		return entities
	}
	return append(preferred, rest...)
}

func entityKind(entities []map[string]any, idx int) string {
	if idx < 0 || idx >= len(entities) {
		return ""
	}
	return anyx.String(entities[idx]["kind"])
}

func isMiningSetupQuestion(q string) bool {
	if miningSetupRE.MatchString(q) && (miningCoreRE.MatchString(q) ||
		regexp.MustCompile(`(?i)(激光头|采矿头|破矿|精炼模拟|regolith|mining\s*laser)`).MatchString(q)) {
		// Explicit setup terms always win; bare「采矿船货舱」 stays facts.
		if miningStaticRE.MatchString(q) && !regexp.MustCompile(`(?i)(激光|模块|配装|配激光|破矿|阻力|激光头|采矿头)`).MatchString(q) {
			return false
		}
		return true
	}
	return false
}

func filterMiningEntities(entities []map[string]any, question string) []map[string]any {
	out := []map[string]any{}
	for _, e := range entities {
		alias := strings.ToLower(anyx.String(e["zh"]))
		if alias == "" {
			alias = strings.ToLower(anyx.String(e["matched_alias"]))
		}
		name := strings.ToLower(anyx.String(e["canonical_name"]))
		// 「采矿」alone often aliases to ATLS GEO; drop unless user named the ship.
		if anyx.String(e["kind"]) == "ship" && (alias == "采矿" || alias == "挖矿") {
			if !strings.Contains(strings.ToLower(question), "atlas") &&
				!strings.Contains(question, "GEO") &&
				!strings.Contains(question, "阿特拉斯") &&
				!strings.Contains(name, strings.ToLower(strings.TrimSpace(question))) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func isShipBuyQuestion(q string, entities []map[string]any) bool {
	if !shipBuyAskRE.MatchString(q) {
		return false
	}
	if hasCommodityOrTerminal(entities) && entityKind(entities, 0) != "ship" {
		// Prefer trade when a commodity is present unless primary is clearly a ship.
		if commodityPrimary(entities) != nil && entityKind(entities, 0) != "ship" {
			return false
		}
	}
	for _, e := range entities {
		if anyx.String(e["kind"]) == "ship" {
			return true
		}
	}
	return false
}

func commodityPrimary(entities []map[string]any) map[string]any {
	for _, e := range entities {
		if anyx.String(e["kind"]) == "commodity" {
			return e
		}
	}
	return nil
}

func factionPrimary(entities []map[string]any) map[string]any {
	for _, e := range entities {
		if anyx.String(e["kind"]) == "faction" {
			return e
		}
	}
	return nil
}
