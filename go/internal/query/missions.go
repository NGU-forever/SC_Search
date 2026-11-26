package query

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var (
	tokenSplit = regexp.MustCompile(`[\s_\-]+`)
	spaceStrip = regexp.MustCompile(`[\s_\-]+`)
	stopwords  = map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "for": {}, "in": {}, "of": {}, "the": {}, "to": {},
	}
)

// DefaultMissionLimit is used when callers pass limit<=0 (DeepSeek P0: raise from 10).
const DefaultMissionLimit = 50

const missionSelectSQL = `
		SELECT
		  e.id AS entity_id,
		  e.canonical_name,
		  e.game_version,
		  fc.value_text AS contractor,
		  fr.value_num AS reward_uec,
		  fcost.value_num AS cost_uec,
		  fcd.value_num AS cooldown,
		  flt.value_num AS lifetime,
		  frep.value_num AS reputation_min,
		  freprank.value_text AS reputation_rank,
		  fpr.value_text AS pyro_regions,
		  fsite.value_text AS site_pool,
		  femin.value_num AS enemy_min,
		  femax.value_num AS enemy_max,
		  fmt.value_text AS mission_type,
		  fs.value_text AS systems,
		  COALESCE(
		    (
		      SELECT a.alias
		      FROM aliases a
		      WHERE a.entity_id = e.id
		        AND a.alias != e.canonical_name
		      ORDER BY length(a.alias) ASC, a.alias
		      LIMIT 1
		    ),
		    e.canonical_name
		  ) AS title
		FROM entities e
		INNER JOIN facts fc ON fc.entity_id = e.id AND fc.key = 'contractor'
		LEFT JOIN facts fr ON fr.entity_id = e.id AND fr.key = 'reward_uec'
		LEFT JOIN facts fcost ON fcost.entity_id = e.id AND fcost.key = 'cost_uec'
		LEFT JOIN facts fcd ON fcd.entity_id = e.id AND fcd.key = 'cooldown'
		LEFT JOIN facts flt ON flt.entity_id = e.id AND flt.key = 'lifetime'
		LEFT JOIN facts frep ON frep.entity_id = e.id AND frep.key = 'reputation_min'
		LEFT JOIN facts freprank ON freprank.entity_id = e.id AND freprank.key = 'reputation_rank'
		LEFT JOIN facts fpr ON fpr.entity_id = e.id AND fpr.key = 'pyro_regions'
		LEFT JOIN facts fsite ON fsite.entity_id = e.id AND fsite.key = 'site_pool'
		LEFT JOIN facts femin ON femin.entity_id = e.id AND femin.key = 'enemy_min'
		LEFT JOIN facts femax ON femax.entity_id = e.id AND femax.key = 'enemy_max'
		LEFT JOIN facts fmt ON fmt.entity_id = e.id AND fmt.key = 'mission_type'
		LEFT JOIN facts fs ON fs.entity_id = e.id AND fs.key = 'systems'
		WHERE e.kind = 'mission'
`

// MissionList is a truncated mission query with total/truncated metadata.
type MissionList struct {
	Rows      []map[string]any
	Total     int
	Truncated bool
	Limit     int
}

func NormalizeMissionText(text string) string {
	return spaceStrip.ReplaceAllString(strings.ToLower(text), "")
}

func RankKeywordTokens(rankKeyword string, faction string) []string {
	tokens := tokenSplit.Split(strings.TrimSpace(rankKeyword), -1)
	out := []string{}
	factionNorm := ""
	if strings.TrimSpace(faction) != "" {
		factionNorm = NormalizeMissionText(faction)
	}
	for _, t := range tokens {
		if t == "" {
			continue
		}
		n := NormalizeMissionText(t)
		if n == "" {
			continue
		}
		if _, stop := stopwords[n]; stop {
			continue
		}
		if factionNorm != "" && (strings.Contains(factionNorm, n) || strings.Contains(n, factionNorm)) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func missionFilters(db *sql.DB, faction, system string) (where string, args []any, err error) {
	where = ""
	args = []any{}
	if strings.TrimSpace(faction) != "" {
		pattern, err := contractorPattern(db, faction)
		if err != nil {
			return "", nil, err
		}
		where += " AND fc.value_text LIKE ? COLLATE NOCASE"
		args = append(args, "%"+pattern+"%")
	}
	if strings.TrimSpace(system) != "" {
		where += " AND COALESCE(fs.value_text, '') LIKE ? COLLATE NOCASE"
		args = append(args, "%"+strings.TrimSpace(system)+"%")
	}
	return where, args, nil
}

// ListMissions returns missions matching filters (default limit 50) plus total/truncated.
func ListMissions(db *sql.DB, faction, rankKeyword, system string, limit int) (MissionList, error) {
	return ListMissionsOpts(db, faction, rankKeyword, system, limit, false)
}

// ListMissionsOpts is ListMissions with optional fee-first ordering for mission_money.
func ListMissionsOpts(db *sql.DB, faction, rankKeyword, system string, limit int, preferFee bool) (MissionList, error) {
	if limit <= 0 {
		limit = DefaultMissionLimit
	}
	where, args, err := missionFilters(db, faction, system)
	if err != nil {
		return MissionList{}, err
	}
	tokens := []string{}
	if strings.TrimSpace(rankKeyword) != "" {
		tokens = RankKeywordTokens(rankKeyword, faction)
	}

	orderBy := " ORDER BY fr.value_num DESC NULLS LAST"
	if preferFee {
		// Surface buy-in / fee rows even when reward_uec is null.
		orderBy = " ORDER BY CASE WHEN fcost.value_num IS NOT NULL THEN 0 ELSE 1 END, COALESCE(fcost.value_num, 0) DESC, fr.value_num DESC NULLS LAST"
	}

	sqlText := missionSelectSQL + where
	queryArgs := append([]any{}, args...)
	if len(tokens) == 0 {
		sqlText += orderBy + " LIMIT ?"
		queryArgs = append(queryArgs, limit)
	} else {
		sqlText += orderBy
	}

	total, err := countMissions(db, where, args)
	if err != nil {
		return MissionList{}, err
	}

	rows, err := db.Query(sqlText, queryArgs...)
	if err != nil {
		return MissionList{}, err
	}
	items, err := scanMaps(rows)
	rows.Close()
	if err != nil {
		return MissionList{}, err
	}

	if len(tokens) == 0 {
		out := dropEntityID(items)
		return MissionList{
			Rows:      out,
			Total:     total,
			Truncated: total > len(out),
			Limit:     limit,
		}, nil
	}

	// Token path: total becomes match count after scoring (not unfiltered SQL total).
	ids := make([]int64, 0, len(items))
	for _, r := range items {
		ids = append(ids, asInt64(r["entity_id"]))
	}
	aliasMap, err := aliasesForEntities(db, ids)
	if err != nil {
		return MissionList{}, err
	}

	type scored struct {
		score float64
		row   map[string]any
	}
	var scoredRows []scored
	for _, row := range items {
		eid := asInt64(row["entity_id"])
		aliases := aliasMap[eid]
		canonical := anyx.String(row["canonical_name"])
		haystacks := haystacksForRow(canonical, aliases)
		if !tokensMatch(haystacks, tokens) {
			continue
		}
		score := scoreMatch(haystacks, tokens, aliases)
		clean := cloneWithout(row, "entity_id")
		scoredRows = append(scoredRows, scored{score: score, row: clean})
	}
	sort.SliceStable(scoredRows, func(i, j int) bool {
		if scoredRows[i].score != scoredRows[j].score {
			return scoredRows[i].score > scoredRows[j].score
		}
		ri := anyx.Float(scoredRows[i].row["reward_uec"])
		rj := anyx.Float(scoredRows[j].row["reward_uec"])
		return ri > rj
	})
	matchTotal := len(scoredRows)
	out := make([]map[string]any, 0, limit)
	for _, s := range scoredRows {
		if len(out) >= limit {
			break
		}
		out = append(out, s.row)
	}
	return MissionList{
		Rows:      out,
		Total:     matchTotal,
		Truncated: matchTotal > len(out),
		Limit:     limit,
	}, nil
}

func countMissions(db *sql.DB, where string, args []any) (int, error) {
	q := `
		SELECT COUNT(*)
		FROM entities e
		INNER JOIN facts fc ON fc.entity_id = e.id AND fc.key = 'contractor'
		LEFT JOIN facts fs ON fs.entity_id = e.id AND fs.key = 'systems'
		WHERE e.kind = 'mission'
	` + where
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func contractorPattern(db *sql.DB, faction string) (string, error) {
	text := strings.TrimSpace(faction)
	matches, err := ResolveName(db, text)
	if err != nil {
		return "", err
	}
	if m := pickEntity(matches, "faction"); m != nil {
		return anyx.String(m["canonical_name"]), nil
	}
	if len(matches) > 0 {
		return anyx.String(matches[0]["canonical_name"]), nil
	}
	return text, nil
}

func aliasesForEntities(db *sql.DB, ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	for _, id := range ids {
		out[id] = []string{}
	}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf("SELECT entity_id, alias FROM aliases WHERE entity_id IN (%s)", strings.Join(placeholders, ","))
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eid int64
		var alias string
		if err := rows.Scan(&eid, &alias); err != nil {
			return nil, err
		}
		out[eid] = append(out[eid], alias)
	}
	return out, rows.Err()
}

func haystacksForRow(canonical string, aliases []string) []string {
	raw := append([]string{canonical}, aliases...)
	out := []string{}
	for _, x := range raw {
		if x == "" {
			continue
		}
		out = append(out, NormalizeMissionText(x))
	}
	return out
}

func tokensMatch(haystacks, tokens []string) bool {
	norms := []string{}
	for _, t := range tokens {
		if n := NormalizeMissionText(t); n != "" {
			norms = append(norms, n)
		}
	}
	if len(norms) == 0 {
		return true
	}
	for _, n := range norms {
		ok := false
		for _, h := range haystacks {
			if strings.Contains(h, n) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func scoreMatch(haystacks, tokens, aliases []string) float64 {
	norms := []string{}
	for _, t := range tokens {
		if n := NormalizeMissionText(t); n != "" {
			norms = append(norms, n)
		}
	}
	if len(norms) == 0 {
		return 0
	}
	hits := 0
	for _, n := range norms {
		for _, h := range haystacks {
			if strings.Contains(h, n) {
				hits++
				break
			}
		}
	}
	shortest := 99
	if len(haystacks) > 0 {
		shortest = len(haystacks[0])
	}
	for _, a := range aliases {
		if a != "" && len(a) < shortest {
			shortest = len(a)
		}
	}
	return float64(hits)*100.0 - float64(shortest)
}

func dropEntityID(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, cloneWithout(item, "entity_id"))
	}
	return out
}

func cloneWithout(m map[string]any, drop string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == drop {
			continue
		}
		out[k] = v
	}
	return out
}
