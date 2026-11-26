package ingest

import (
	"fmt"
	"regexp"
	"strings"
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
