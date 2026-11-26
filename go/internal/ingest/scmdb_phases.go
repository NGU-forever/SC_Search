package ingest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func (a *scmdbApply) factions(payload map[string]any) error {
	for guid, metaRaw := range anyx.StringMap(payload["factions"]) {
		meta := anyx.StringMap(metaRaw)
		if meta == nil {
			continue
		}
		name := anyx.First(anyx.String(meta["name"]), guid)
		entityID, err := UpsertEntity(a.conn, "faction", name, a.version, "", "scmdb-faction-"+guid)
		if err != nil {
			return err
		}
		_ = UpsertAlias(a.conn, entityID, name, "en", scmdbSource)
		a.factionIDs[guid] = entityID
		a.count++
	}
	return nil
}

func (a *scmdbApply) blueprintPools(payload map[string]any) error {
	for poolGUID, poolRaw := range anyx.StringMap(payload["blueprintPools"]) {
		pool := anyx.StringMap(poolRaw)
		if pool == nil {
			continue
		}
		poolName := anyx.First(anyx.String(pool["name"]), poolGUID)
		poolEntity, err := UpsertEntity(a.conn, "blueprint_pool", poolName, a.version, "", "scmdb-bppool-"+poolGUID)
		if err != nil {
			return err
		}
		_ = UpsertAlias(a.conn, poolEntity, poolName, "en", scmdbSource)
		a.poolIDs[poolGUID] = poolEntity
		_ = UpsertFact(a.conn, poolEntity, "source_tag", scmdbSource, a.version, &a.runID, anyx.String(pool["source"]), nil, "")
		a.count++
		for _, bpRaw := range anyx.Slice(pool["blueprints"]) {
			bp := anyx.StringMap(bpRaw)
			if bp == nil {
				continue
			}
			bpName := anyx.First(anyx.String(bp["name"]), anyx.String(bp["blueprintRecord"]), "unknown")
			bpUUID := anyx.First(anyx.String(bp["blueprintRecord"]), bpName)
			bpEntity, err := UpsertEntity(a.conn, "blueprint", bpName, a.version, "", "scmdb-bp-"+bpUUID)
			if err != nil {
				return err
			}
			_ = UpsertAlias(a.conn, bpEntity, bpName, "en", scmdbSource)
			weight := maybeFloat(bp["weight"])
			_ = UpsertFact(a.conn, bpEntity, "weight", scmdbSource, a.version, &a.runID, "", weight, "")
			if ec := anyx.String(bp["entityClass"]); ec != "" {
				_ = UpsertFact(a.conn, bpEntity, "entity_class", scmdbSource, a.version, &a.runID, ec, nil, "")
			}
			_ = UpsertFact(a.conn, bpEntity, "pool_name", scmdbSource, a.version, &a.runID, poolName, nil, "")
			_ = InsertRelation(a.conn, poolEntity, bpEntity, "pool_contains_blueprint", scmdbSource, a.version, weight, "")
			a.count++
		}
	}
	return nil
}

func (a *scmdbApply) locations(payload map[string]any) error {
	locationPools := anyx.StringMap(payload["locationPools"])
	for locKey, locRaw := range locationPools {
		loc := anyx.StringMap(locRaw)
		if loc == nil {
			continue
		}
		name := strings.TrimSpace(anyx.First(anyx.String(loc["name"]), locKey))
		if name == "" || strings.HasPrefix(name, "@") {
			continue
		}
		entityID, err := UpsertEntity(a.conn, "location", name, a.version, "", "scmdb-loc-"+locKey)
		if err != nil {
			return err
		}
		_ = UpsertAlias(a.conn, entityID, name, "en", scmdbSource)
		a.locationIDs[locKey] = entityID
		a.factText(entityID, "location_type", loc["type"])
		a.factText(entityID, "system", loc["system"])
		a.factText(entityID, "planet", loc["planet"])
		a.factText(entityID, "moon", loc["moon"])
		a.factText(entityID, "scmdb_location_id", locKey)
		a.count++
	}
	return nil
}

func (a *scmdbApply) missions(payload map[string]any) error {
	contracts, _ := payload["contracts"].([]any)
	locationPools := anyx.StringMap(payload["locationPools"])
	for _, cRaw := range contracts {
		contract := anyx.StringMap(cRaw)
		if contract == nil {
			continue
		}
		if err := a.applyContract(contract, locationPools); err != nil {
			return err
		}
		a.count++
	}
	return nil
}

func (a *scmdbApply) applyContract(contract map[string]any, locationPools map[string]any) error {
	debugName := anyx.String(contract["debugName"])
	title := anyx.First(anyx.String(contract["title"]), debugName, anyx.String(contract["id"]), "mission")
	canonical := anyx.First(debugName, title)
	missionID := anyx.First(anyx.String(contract["id"]), debugName, title)
	entityID, err := UpsertEntity(a.conn, "mission", canonical, a.version, debugName, "scmdb-mission-"+missionID)
	if err != nil {
		return err
	}
	_ = UpsertAlias(a.conn, entityID, canonical, "en", scmdbSource)
	if title != canonical {
		_ = UpsertAlias(a.conn, entityID, title, "en", scmdbSource)
	}
	a.missionFacts(contract, entityID, debugName, canonical)
	a.missionSiteFacts(contract, entityID, locationPools)
	return a.missionRelations(contract, entityID)
}

func (a *scmdbApply) missionFacts(contract map[string]any, entityID int64, debugName, canonical string) {
	a.factNum(entityID, "reward_uec", contract["rewardUEC"], "aUEC")
	a.factNum(entityID, "cost_uec", contract["buyIn"], "aUEC")
	// SCMDB: personalCooldownTime is minutes; timeToComplete is hours.
	a.factNum(entityID, "cooldown", contract["personalCooldownTime"], "min")
	a.factNum(entityID, "abandoned_cooldown", contract["abandonedCooldownTime"], "min")
	a.factNum(entityID, "lifetime", contract["timeToComplete"], "hr")
	a.factText(entityID, "cooldown_unit", "min")
	a.factText(entityID, "lifetime_unit", "hr")
	a.factText(entityID, "mission_type", anyx.First(anyx.String(contract["missionType"]), anyx.String(contract["missionTypeKey"])))
	a.factText(entityID, "category", contract["category"])
	a.factText(entityID, "contractor", contract["contractorName"])
	a.factText(entityID, "debug_name", contract["debugName"])
	if regionTag, band := missionDisambiguationTags(debugName, canonical); regionTag != "" {
		a.factText(entityID, "region_tag", regionTag)
		a.factText(entityID, "difficulty_band", band)
	}
	if contract["illegal"] != nil {
		if b, ok := contract["illegal"].(bool); ok && b {
			a.factText(entityID, "illegal", "1")
		} else if b, ok := contract["illegal"].(bool); ok && !b {
			a.factText(entityID, "illegal", "0")
		} else {
			a.factText(entityID, "illegal", fmt.Sprint(contract["illegal"]))
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
			a.factText(entityID, "systems", strings.Join(parts, ","))
		} else {
			a.factText(entityID, "systems", fmt.Sprint(systems))
		}
	}
	if minStanding := anyx.StringMap(contract["minStanding"]); minStanding != nil {
		a.factText(entityID, "reputation_rank", minStanding["name"])
		a.factNum(entityID, "reputation_min", minStanding["minReputation"], "")
	}
	if maxStanding := anyx.StringMap(contract["maxStanding"]); maxStanding != nil {
		a.factText(entityID, "reputation_rank_max", maxStanding["name"])
		a.factNum(entityID, "reputation_max", maxStanding["minReputation"], "")
	}
	if pyro := contract["pyroRegion"]; pyro != nil {
		if arr, ok := pyro.([]any); ok && len(arr) > 0 {
			parts := make([]string, 0, len(arr))
			for _, r := range arr {
				parts = append(parts, fmt.Sprint(r))
			}
			a.factText(entityID, "pyro_regions", strings.Join(parts, ","))
		} else {
			a.factText(entityID, "pyro_regions", fmt.Sprint(pyro))
		}
	}
}

func (a *scmdbApply) missionSiteFacts(contract map[string]any, entityID int64, locationPools map[string]any) {
	byRegion := missionSitePoolByRegion(contract, locationPools)
	if len(byRegion) > 0 {
		raw, _ := json.Marshal(byRegion)
		a.factText(entityID, "site_pool_by_region", string(raw))
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
			a.factText(entityID, "site_pool", string(rawFlat))
		}
	}
	emin, emax := enemyShipCounts(contract["shipEncounters"])
	if emin != nil {
		a.factNum(entityID, "enemy_min", *emin, "")
	}
	if emax != nil {
		a.factNum(entityID, "enemy_max", *emax, "")
	}
}

func (a *scmdbApply) missionRelations(contract map[string]any, entityID int64) error {
	if fg := anyx.String(contract["factionGuid"]); fg != "" {
		if fid, ok := a.factionIDs[fg]; ok {
			_ = InsertRelation(a.conn, entityID, fid, "mission_faction", scmdbSource, a.version, nil, "")
		}
	}
	for _, locKey := range missionLocationIDs(contract) {
		if toID, ok := a.locationIDs[locKey]; ok {
			_ = InsertRelation(a.conn, entityID, toID, "mission_location", scmdbSource, a.version, nil, "")
		}
	}
	for _, rewardRaw := range anyx.Slice(contract["blueprintRewards"]) {
		reward := anyx.StringMap(rewardRaw)
		if reward == nil {
			continue
		}
		poolGUID := anyx.String(reward["blueprintPool"])
		poolEntity, ok := a.poolIDs[poolGUID]
		if !ok && anyx.String(reward["poolName"]) != "" {
			var err error
			poolEntity, err = UpsertEntity(a.conn, "blueprint_pool", anyx.String(reward["poolName"]), a.version, "",
				"scmdb-bppool-"+anyx.First(poolGUID, anyx.String(reward["poolName"])))
			if err != nil {
				return err
			}
			a.poolIDs[poolGUID] = poolEntity
			ok = true
		}
		if !ok {
			continue
		}
		chance := maybeFloat(reward["chance"])
		meta, _ := json.Marshal(map[string]any{"poolName": reward["poolName"], "trigger": reward["trigger"]})
		_ = InsertRelation(a.conn, entityID, poolEntity, "mission_reward_blueprint_pool", scmdbSource, a.version, chance, string(meta))
	}
	for _, itemRaw := range anyx.Slice(contract["itemRewards"]) {
		item := anyx.StringMap(itemRaw)
		if item == nil {
			continue
		}
		itemName := anyx.First(anyx.String(item["name"]), anyx.String(item["entityClass"]), "item")
		itemUUID := anyx.First(anyx.String(item["entityClass"]), itemName)
		itemEntity, err := UpsertEntity(a.conn, "item", itemName, a.version, "", "scmdb-item-"+itemUUID)
		if err != nil {
			return err
		}
		_ = UpsertAlias(a.conn, itemEntity, itemName, "en", scmdbSource)
		amt := maybeFloat(item["amount"])
		_ = InsertRelation(a.conn, entityID, itemEntity, "mission_reward_item", scmdbSource, a.version, amt, "")
	}
	return nil
}
