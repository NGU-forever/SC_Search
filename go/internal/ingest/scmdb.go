package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	scmdbMirrorBase = "https://raw.githubusercontent.com/KrovaxCode/SCMDB_DATA/main/data"
	scmdbBase       = "https://scmdb.net/data"
	scmdbUserAgent  = "SC_Search-ingest/0.1"
	scmdbSource     = "scmdb"
)

func enemyShipCounts(shipEncounters any) (min *float64, max *float64) {
	m, ok := shipEncounters.(map[string]any)
	if !ok {
		return nil, nil
	}
	cfg, _ := m["spawnConfig"].(map[string]any)
	if cfg == nil {
		return nil, nil
	}
	tmin := maybeFloat(cfg["totalMinShips"])
	tmax := maybeFloat(cfg["totalMaxShips"])
	if tmin != nil || tmax != nil {
		return tmin, tmax
	}
	var smin, smax float64
	found := false
	groups, _ := cfg["groups"].([]any)
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		if gm == nil {
			continue
		}
		waves, _ := gm["waves"].([]any)
		for _, w := range waves {
			wm, _ := w.(map[string]any)
			if wm == nil {
				continue
			}
			found = true
			if v := maybeFloat(wm["minShips"]); v != nil {
				smin += *v
			}
			if v := maybeFloat(wm["maxShips"]); v != nil {
				smax += *v
			}
		}
	}
	if found {
		return &smin, &smax
	}
	return nil, nil
}

func missionLocationIDs(contract map[string]any) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw any) {
		if raw == nil {
			return
		}
		key := fmt.Sprint(raw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	for _, lid := range asAnySlice(contract["locations"]) {
		add(lid)
	}
	for _, lid := range asAnySlice(contract["destinations"]) {
		add(lid)
	}
	for _, block := range asAnySlice(contract["locationSets"]) {
		bm, _ := block.(map[string]any)
		if bm == nil {
			continue
		}
		for _, lid := range asAnySlice(bm["locations"]) {
			add(lid)
		}
		for _, lid := range asAnySlice(bm["destinations"]) {
			add(lid)
		}
	}
	return out
}

func keepMissionSiteName(name, locType string) bool {
	return strings.HasPrefix(name, "RAB-") || strings.HasPrefix(name, "Cluster ") ||
		locType == "Station" || locType == "Outpost" || locType == "LandingZone" || locType == "Destination"
}

func siteNamesFromLocationIDs(ids []string, locationPools map[string]any, limit int) []string {
	if limit <= 0 {
		limit = 25
	}
	var names []string
	seen := map[string]struct{}{}
	for _, lid := range ids {
		loc, _ := locationPools[lid].(map[string]any)
		if loc == nil {
			continue
		}
		name := strings.TrimSpace(strAny(loc["name"]))
		if name == "" || strings.HasPrefix(name, "@") {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		if !keepMissionSiteName(name, strAny(loc["type"])) {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
		if len(names) >= limit {
			break
		}
	}
	return names
}

func missionSiteNames(contract map[string]any, locationPools map[string]any) []string {
	return siteNamesFromLocationIDs(missionLocationIDs(contract), locationPools, 40)
}

// missionSitePoolByRegion zips pyroRegion[i] with locationSets[i] when lengths match.
func missionSitePoolByRegion(contract map[string]any, locationPools map[string]any) map[string][]string {
	var regions []string
	if pyro := contract["pyroRegion"]; pyro != nil {
		for _, r := range asAnySlice(pyro) {
			regions = append(regions, fmt.Sprint(r))
		}
	}
	sets := asAnySlice(contract["locationSets"])
	out := map[string][]string{}
	if len(regions) > 0 && len(sets) == len(regions) {
		for i, region := range regions {
			block, _ := sets[i].(map[string]any)
			if block == nil {
				continue
			}
			var ids []string
			seen := map[string]struct{}{}
			add := func(raw any) {
				if raw == nil {
					return
				}
				key := fmt.Sprint(raw)
				if _, ok := seen[key]; ok {
					return
				}
				seen[key] = struct{}{}
				ids = append(ids, key)
			}
			for _, lid := range asAnySlice(block["locations"]) {
				add(lid)
			}
			for _, lid := range asAnySlice(block["destinations"]) {
				add(lid)
			}
			names := siteNamesFromLocationIDs(ids, locationPools, 25)
			if len(names) > 0 {
				out[region] = names
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if flat := missionSiteNames(contract, locationPools); len(flat) > 0 {
		out["*"] = flat
	}
	return out
}

var missionDebugBandRE = regexp.MustCompile(`(?i)_Region([A-Z])_(E|M|H)_`)

func missionDisambiguationTags(debugName, canonical string) (regionTag, difficultyBand string) {
	for _, s := range []string{debugName, canonical} {
		if m := missionDebugBandRE.FindStringSubmatch(s); len(m) == 3 {
			return "Region" + strings.ToUpper(m[1]), strings.ToUpper(m[2])
		}
	}
	return "", ""
}

func asAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func asStringMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// ApplyScmdbPayload mirrors Python apply_scmdb_payload.
func ApplyScmdbPayload(conn *sql.DB, payload map[string]any, runID int64, gameVersion string) (int, error) {
	if err := ClearSourceFactsRelations(conn, scmdbSource, gameVersion); err != nil {
		return 0, err
	}
	count := 0
	run := runID

	factionIDs := map[string]int64{}
	for guid, metaRaw := range asStringMap(payload["factions"]) {
		meta := asStringMap(metaRaw)
		if meta == nil {
			continue
		}
		name := firstNonEmpty(strAny(meta["name"]), guid)
		entityID, err := UpsertEntity(conn, "faction", name, gameVersion, "", "scmdb-faction-"+guid)
		if err != nil {
			return count, err
		}
		_ = UpsertAlias(conn, entityID, name, "en", scmdbSource)
		factionIDs[guid] = entityID
		count++
	}

	poolIDs := map[string]int64{}
	for poolGUID, poolRaw := range asStringMap(payload["blueprintPools"]) {
		pool := asStringMap(poolRaw)
		if pool == nil {
			continue
		}
		poolName := firstNonEmpty(strAny(pool["name"]), poolGUID)
		poolEntity, err := UpsertEntity(conn, "blueprint_pool", poolName, gameVersion, "", "scmdb-bppool-"+poolGUID)
		if err != nil {
			return count, err
		}
		_ = UpsertAlias(conn, poolEntity, poolName, "en", scmdbSource)
		poolIDs[poolGUID] = poolEntity
		_ = UpsertFact(conn, poolEntity, "source_tag", scmdbSource, gameVersion, &run, strAny(pool["source"]), nil, "")
		count++
		for _, bpRaw := range asAnySlice(pool["blueprints"]) {
			bp := asStringMap(bpRaw)
			if bp == nil {
				continue
			}
			bpName := firstNonEmpty(strAny(bp["name"]), strAny(bp["blueprintRecord"]), "unknown")
			bpUUID := firstNonEmpty(strAny(bp["blueprintRecord"]), bpName)
			bpEntity, err := UpsertEntity(conn, "blueprint", bpName, gameVersion, "", "scmdb-bp-"+bpUUID)
			if err != nil {
				return count, err
			}
			_ = UpsertAlias(conn, bpEntity, bpName, "en", scmdbSource)
			weight := maybeFloat(bp["weight"])
			_ = UpsertFact(conn, bpEntity, "weight", scmdbSource, gameVersion, &run, "", weight, "")
			if ec := strAny(bp["entityClass"]); ec != "" {
				_ = UpsertFact(conn, bpEntity, "entity_class", scmdbSource, gameVersion, &run, ec, nil, "")
			}
			_ = UpsertFact(conn, bpEntity, "pool_name", scmdbSource, gameVersion, &run, poolName, nil, "")
			_ = InsertRelation(conn, poolEntity, bpEntity, "pool_contains_blueprint", scmdbSource, gameVersion, weight, "")
			count++
		}
	}

	locationIDs := map[string]int64{}
	locationPools := asStringMap(payload["locationPools"])
	for locKey, locRaw := range locationPools {
		loc := asStringMap(locRaw)
		if loc == nil {
			continue
		}
		name := strings.TrimSpace(firstNonEmpty(strAny(loc["name"]), locKey))
		if name == "" || strings.HasPrefix(name, "@") {
			continue
		}
		entityID, err := UpsertEntity(conn, "location", name, gameVersion, "", "scmdb-loc-"+locKey)
		if err != nil {
			return count, err
		}
		_ = UpsertAlias(conn, entityID, name, "en", scmdbSource)
		locationIDs[locKey] = entityID
		locFact := func(key string, value any) {
			s := strAny(value)
			if s == "" {
				return
			}
			_ = UpsertFact(conn, entityID, key, scmdbSource, gameVersion, &run, s, nil, "")
		}
		locFact("location_type", loc["type"])
		locFact("system", loc["system"])
		locFact("planet", loc["planet"])
		locFact("moon", loc["moon"])
		locFact("scmdb_location_id", locKey)
		count++
	}

	contracts, _ := payload["contracts"].([]any)
	for _, cRaw := range contracts {
		contract := asStringMap(cRaw)
		if contract == nil {
			continue
		}
		debugName := strAny(contract["debugName"])
		title := firstNonEmpty(strAny(contract["title"]), debugName, strAny(contract["id"]), "mission")
		canonical := firstNonEmpty(debugName, title)
		missionID := firstNonEmpty(strAny(contract["id"]), debugName, title)
		entityID, err := UpsertEntity(conn, "mission", canonical, gameVersion, debugName, "scmdb-mission-"+missionID)
		if err != nil {
			return count, err
		}
		_ = UpsertAlias(conn, entityID, canonical, "en", scmdbSource)
		if title != canonical {
			_ = UpsertAlias(conn, entityID, title, "en", scmdbSource)
		}
		factText := func(key string, value any) {
			s := strAny(value)
			if s == "" {
				return
			}
			_ = UpsertFact(conn, entityID, key, scmdbSource, gameVersion, &run, s, nil, "")
		}
		factNum := func(key string, value any, unit string) {
			n := maybeFloat(value)
			if n == nil {
				return
			}
			_ = UpsertFact(conn, entityID, key, scmdbSource, gameVersion, &run, "", n, unit)
		}
		factNum("reward_uec", contract["rewardUEC"], "aUEC")
		factNum("cost_uec", contract["buyIn"], "aUEC")
		// SCMDB: personalCooldownTime is minutes; timeToComplete is hours.
		factNum("cooldown", contract["personalCooldownTime"], "min")
		factNum("abandoned_cooldown", contract["abandonedCooldownTime"], "min")
		factNum("lifetime", contract["timeToComplete"], "hr")
		factText("cooldown_unit", "min")
		factText("lifetime_unit", "hr")
		factText("mission_type", firstNonEmpty(strAny(contract["missionType"]), strAny(contract["missionTypeKey"])))
		factText("category", contract["category"])
		factText("contractor", contract["contractorName"])
		factText("debug_name", contract["debugName"])
		if regionTag, band := missionDisambiguationTags(debugName, canonical); regionTag != "" {
			factText("region_tag", regionTag)
			factText("difficulty_band", band)
		}
		if contract["illegal"] != nil {
			if b, ok := contract["illegal"].(bool); ok && b {
				factText("illegal", "1")
			} else if b, ok := contract["illegal"].(bool); ok && !b {
				factText("illegal", "0")
			} else {
				factText("illegal", fmt.Sprint(contract["illegal"]))
			}
		}
		systems := contract["availableSystems"]
		if systems == nil {
			systems = contract["systems"]
		}
		if systems != nil {
			if arr, ok := systems.([]any); ok {
				parts := make([]string, 0, len(arr))
				for _, s := range arr {
					parts = append(parts, fmt.Sprint(s))
				}
				factText("systems", strings.Join(parts, ","))
			} else {
				factText("systems", fmt.Sprint(systems))
			}
		}
		if minStanding := asStringMap(contract["minStanding"]); minStanding != nil {
			factText("reputation_rank", minStanding["name"])
			factNum("reputation_min", minStanding["minReputation"], "")
		}
		if maxStanding := asStringMap(contract["maxStanding"]); maxStanding != nil {
			factText("reputation_rank_max", maxStanding["name"])
			factNum("reputation_max", maxStanding["minReputation"], "")
		}
		if pyro := contract["pyroRegion"]; pyro != nil {
			if arr, ok := pyro.([]any); ok && len(arr) > 0 {
				parts := make([]string, 0, len(arr))
				for _, r := range arr {
					parts = append(parts, fmt.Sprint(r))
				}
				factText("pyro_regions", strings.Join(parts, ","))
			} else {
				factText("pyro_regions", fmt.Sprint(pyro))
			}
		}
		byRegion := missionSitePoolByRegion(contract, locationPools)
		if len(byRegion) > 0 {
			raw, _ := json.Marshal(byRegion)
			factText("site_pool_by_region", string(raw))
			// Flat pool for backward-compatible list/search consumers.
			flatSeen := map[string]struct{}{}
			var flat []string
			for _, region := range []string{"A", "B", "C", "D", "*"} {
				for _, n := range byRegion[region] {
					if _, ok := flatSeen[n]; ok {
						continue
					}
					flatSeen[n] = struct{}{}
					flat = append(flat, n)
				}
			}
			for region, names := range byRegion {
				if region == "A" || region == "B" || region == "C" || region == "D" || region == "*" {
					continue
				}
				for _, n := range names {
					if _, ok := flatSeen[n]; ok {
						continue
					}
					flatSeen[n] = struct{}{}
					flat = append(flat, n)
				}
			}
			if len(flat) > 0 {
				rawFlat, _ := json.Marshal(flat)
				factText("site_pool", string(rawFlat))
			}
		}
		emin, emax := enemyShipCounts(contract["shipEncounters"])
		if emin != nil {
			factNum("enemy_min", *emin, "")
		}
		if emax != nil {
			factNum("enemy_max", *emax, "")
		}
		if fg := strAny(contract["factionGuid"]); fg != "" {
			if fid, ok := factionIDs[fg]; ok {
				_ = InsertRelation(conn, entityID, fid, "mission_faction", scmdbSource, gameVersion, nil, "")
			}
		}
		for _, locKey := range missionLocationIDs(contract) {
			if toID, ok := locationIDs[locKey]; ok {
				_ = InsertRelation(conn, entityID, toID, "mission_location", scmdbSource, gameVersion, nil, "")
			}
		}
		for _, rewardRaw := range asAnySlice(contract["blueprintRewards"]) {
			reward := asStringMap(rewardRaw)
			if reward == nil {
				continue
			}
			poolGUID := strAny(reward["blueprintPool"])
			poolEntity, ok := poolIDs[poolGUID]
			if !ok && strAny(reward["poolName"]) != "" {
				var err error
				poolEntity, err = UpsertEntity(conn, "blueprint_pool", strAny(reward["poolName"]), gameVersion, "",
					"scmdb-bppool-"+firstNonEmpty(poolGUID, strAny(reward["poolName"])))
				if err != nil {
					return count, err
				}
				poolIDs[poolGUID] = poolEntity
				ok = true
			}
			if !ok {
				continue
			}
			chance := maybeFloat(reward["chance"])
			meta, _ := json.Marshal(map[string]any{"poolName": reward["poolName"], "trigger": reward["trigger"]})
			_ = InsertRelation(conn, entityID, poolEntity, "mission_reward_blueprint_pool", scmdbSource, gameVersion, chance, string(meta))
		}
		for _, itemRaw := range asAnySlice(contract["itemRewards"]) {
			item := asStringMap(itemRaw)
			if item == nil {
				continue
			}
			itemName := firstNonEmpty(strAny(item["name"]), strAny(item["entityClass"]), "item")
			itemUUID := firstNonEmpty(strAny(item["entityClass"]), itemName)
			itemEntity, err := UpsertEntity(conn, "item", itemName, gameVersion, "", "scmdb-item-"+itemUUID)
			if err != nil {
				return count, err
			}
			_ = UpsertAlias(conn, itemEntity, itemName, "en", scmdbSource)
			amt := maybeFloat(item["amount"])
			_ = InsertRelation(conn, entityID, itemEntity, "mission_reward_item", scmdbSource, gameVersion, amt, "")
		}
		count++
	}
	return count, nil
}

func scmdbGetJSON(client *http.Client, path string) (any, error) {
	var last error
	for _, base := range []string{scmdbMirrorBase, scmdbBase} {
		url := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			last = err
			continue
		}
		req.Header.Set("User-Agent", scmdbUserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode >= 400 {
			last = fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
			continue
		}
		text := strings.TrimLeft(strings.TrimPrefix(string(body), "\ufeff"), " \t\r\n")
		if strings.HasPrefix(text, "<") {
			last = fmt.Errorf("HTML instead of JSON at %s", url)
			continue
		}
		var out any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			last = err
			continue
		}
		return out, nil
	}
	return nil, fmt.Errorf("failed to fetch %s: %v", path, last)
}

func resolveScmdbVersion(client *http.Client, channel, version string) (map[string]any, error) {
	raw, err := scmdbGetJSON(client, "versions.json")
	if err != nil {
		return nil, err
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("versions.json was not a list")
	}
	var versions []map[string]any
	for _, item := range arr {
		m := asStringMap(item)
		if m == nil || strAny(m["version"]) == "" || strAny(m["file"]) == "" {
			continue
		}
		versions = append(versions, m)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no SCMDB versions available")
	}
	if version != "" {
		needle := strings.ToLower(strings.TrimSpace(version))
		for _, item := range versions {
			ver := strings.ToLower(strAny(item["version"]))
			if ver == needle || strings.Contains(ver, needle) {
				return item, nil
			}
		}
		return nil, fmt.Errorf("SCMDB version not found: %s", version)
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		channel = "live"
	}
	if channel == "latest" || channel == "any" || channel == "all" {
		return versions[0], nil
	}
	if channel != "live" && channel != "ptu" {
		return nil, fmt.Errorf("channel must be live, ptu, or latest")
	}
	needle := "-" + channel + "."
	for _, item := range versions {
		if strings.Contains(strings.ToLower(strAny(item["version"])), needle) {
			return item, nil
		}
	}
	return versions[0], nil
}

// SyncScmdbPayload applies an in-memory SCMDB dump (tests / offline).
func SyncScmdbPayload(conn *sql.DB, payload map[string]any) (int, error) {
	runID, err := StartRun(conn, scmdbSource, "")
	if err != nil {
		return 0, err
	}
	gv := firstNonEmpty(strAny(payload["version"]), "unknown")
	count, err := ApplyScmdbPayload(conn, payload, runID, gv)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}

// SyncScmdbLive downloads versions + merged JSON and applies.
func SyncScmdbLive(conn *sql.DB, repoRoot, channel, version string) (int, error) {
	runID, err := StartRun(conn, scmdbSource, "")
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	selected, err := resolveScmdbVersion(client, channel, version)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	raw, err := scmdbGetJSON(client, strAny(selected["file"]))
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	payload := asStringMap(raw)
	if payload == nil {
		err := fmt.Errorf("%s was not a JSON object", strAny(selected["file"]))
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rawDir := filepath.Join(repoRoot, "data", "raw")
	_ = os.MkdirAll(rawDir, 0o755)
	if b, err := json.Marshal(payload); err == nil {
		_ = os.WriteFile(filepath.Join(rawDir, strAny(selected["file"])), b, 0o644)
	}
	gv := firstNonEmpty(strAny(payload["version"]), strAny(selected["version"]))
	count, err := ApplyScmdbPayload(conn, payload, runID, gv)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}
