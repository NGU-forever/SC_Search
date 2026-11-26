package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	scwBase             = "https://api.star-citizen.wiki"
	scwUserAgent        = "SC_Search-ingest/0.1"
	scwAPISource        = "scw_api"
	scwAPIGameVersion   = "scw-live"
	scwBPSource         = "scw_blueprint"
	scwBPGameVersion    = "scw-blueprint-live"
)

func localized(value any, lang string) string {
	if value == nil {
		return ""
	}
	if m, ok := value.(map[string]any); ok {
		if v := strAny(m[lang]); v != "" {
			return v
		}
		if v := strAny(m["en_EN"]); v != "" {
			return v
		}
		for _, v := range m {
			if s := strAny(v); s != "" {
				return s
			}
		}
		return ""
	}
	return fmt.Sprint(value)
}

// ApplyScwVehicles mirrors Python apply_scw_vehicles.
func ApplyScwVehicles(conn DBTX, vehicles []map[string]any, runID int64, gameVersion string) (int, error) {
	if gameVersion == "" {
		gameVersion = scwAPIGameVersion
	}
	if err := ClearSourceFactsRelations(conn, scwAPISource, gameVersion); err != nil {
		return 0, err
	}
	run := runID
	count := 0
	for _, v := range vehicles {
		name := firstNonEmpty(strAny(v["name"]), strAny(v["game_name"]), strAny(v["slug"]))
		uuid := strAny(v["uuid"])
		if name == "" || uuid == "" {
			continue
		}
		canonical := firstNonEmpty(strAny(v["game_name"]), name)
		entityID, err := UpsertEntity(conn, "ship", canonical, gameVersion, strAny(v["slug"]), "scw-ship-"+uuid)
		if err != nil {
			return count, err
		}
		_ = UpsertAlias(conn, entityID, name, "en", scwAPISource)
		if canonical != name {
			_ = UpsertAlias(conn, entityID, canonical, "en", scwAPISource)
		}
		if slug := strAny(v["slug"]); slug != "" {
			_ = UpsertAlias(conn, entityID, slug, "en", scwAPISource)
		}
		if mfr := asStringMap(v["manufacturer"]); mfr != nil {
			if mn := strAny(mfr["name"]); mn != "" {
				_ = UpsertFact(conn, entityID, "manufacturer", scwAPISource, gameVersion, &run, mn, nil, "")
			}
			if code := strAny(mfr["code"]); code != "" {
				_ = UpsertFact(conn, entityID, "manufacturer_code", scwAPISource, gameVersion, &run, code, nil, "")
			}
		}
		if cargo := maybeFloat(v["cargo_capacity"]); cargo != nil {
			_ = UpsertFact(conn, entityID, "cargo_scu", scwAPISource, gameVersion, &run, "", cargo, "SCU")
		}
		for _, pair := range [][2]string{{"size", "size"}, {"type", "type"}, {"production_status", "production_status"}, {"career", "career"}} {
			en := localized(v[pair[0]], "en_EN")
			zh := localized(v[pair[0]], "zh_CN")
			if en != "" {
				_ = UpsertFact(conn, entityID, pair[1], scwAPISource, gameVersion, &run, en, nil, "")
			}
			if zh != "" && zh != en {
				_ = UpsertAlias(conn, entityID, zh, "zh", scwAPISource)
			}
		}
		foci := asAnySlice(v["foci"])
		if len(foci) > 0 {
			var labelsEN []string
			for _, focus := range foci {
				en := localized(focus, "en_EN")
				zh := localized(focus, "zh_CN")
				if en != "" {
					labelsEN = append(labelsEN, en)
				}
				if zh != "" {
					_ = UpsertAlias(conn, entityID, zh, "zh", scwAPISource)
				}
			}
			if len(labelsEN) > 0 {
				_ = UpsertFact(conn, entityID, "foci", scwAPISource, gameVersion, &run, strings.Join(labelsEN, ", "), nil, "")
			}
		}
		descSrc := v["description"]
		if descSrc == nil {
			descSrc = v["game_description"]
		}
		if descEN := localized(descSrc, "en_EN"); descEN != "" {
			if len(descEN) > 2000 {
				descEN = descEN[:2000]
			}
			_ = UpsertFact(conn, entityID, "description", scwAPISource, gameVersion, &run, descEN, nil, "")
		}
		if descZH := localized(descSrc, "zh_CN"); descZH != "" {
			if len(descZH) > 2000 {
				descZH = descZH[:2000]
			}
			_ = UpsertFact(conn, entityID, "description_zh", scwAPISource, gameVersion, &run, descZH, nil, "")
		}
		for _, massKey := range []string{"mass", "mass_total"} {
			if n := maybeFloat(v[massKey]); n != nil {
				_ = UpsertFact(conn, entityID, massKey, scwAPISource, gameVersion, &run, "", n, "kg")
			}
		}
		count++
	}
	return count, nil
}

func findBlueprintID(conn DBTX, apiUUID, outputName string) (int64, bool) {
	var id int64
	for _, uuid := range []string{"scmdb-bp-" + apiUUID, "scw-bp-" + apiUUID} {
		err := conn.QueryRow(`
			SELECT id FROM entities
			WHERE kind = 'blueprint' AND uuid = ? COLLATE NOCASE
			LIMIT 1
		`, uuid).Scan(&id)
		if err == nil {
			return id, true
		}
	}
	err := conn.QueryRow(`
		SELECT id FROM entities
		WHERE kind = 'blueprint' AND canonical_name = ? COLLATE NOCASE
		ORDER BY id LIMIT 1
	`, outputName).Scan(&id)
	return id, err == nil
}

// ApplyScwBlueprints mirrors Python apply_scw_blueprints.
func ApplyScwBlueprints(conn DBTX, blueprints []map[string]any, runID int64) (int, error) {
	if _, err := conn.Exec(`DELETE FROM facts WHERE source = ?`, scwBPSource); err != nil {
		return 0, err
	}
	if _, err := conn.Exec(`DELETE FROM relations WHERE source = ?`, scwBPSource); err != nil {
		return 0, err
	}
	run := runID
	count := 0
	for _, bp := range blueprints {
		apiUUID := strings.TrimSpace(strAny(bp["uuid"]))
		outputName := strings.TrimSpace(strAny(bp["output_name"]))
		if apiUUID == "" || outputName == "" {
			continue
		}
		entityID, ok := findBlueprintID(conn, apiUUID, outputName)
		if !ok {
			var err error
			entityID, err = UpsertEntity(conn, "blueprint", outputName, scwBPGameVersion, "", "scw-bp-"+apiUUID)
			if err != nil {
				return count, err
			}
			_ = UpsertAlias(conn, entityID, outputName, "en", scwBPSource)
		}
		if n := maybeFloat(bp["craft_time_seconds"]); n != nil {
			_ = UpsertFact(conn, entityID, "craft_time_seconds", scwBPSource, scwBPGameVersion, &run, "", n, "s")
		}
		if label := strAny(bp["craft_time_label"]); label != "" {
			_ = UpsertFact(conn, entityID, "craft_time_label", scwBPSource, scwBPGameVersion, &run, label, nil, "")
		}
		for _, ingRaw := range asAnySlice(bp["ingredients"]) {
			ing := asStringMap(ingRaw)
			if ing == nil {
				continue
			}
			matName := strings.TrimSpace(strAny(ing["name"]))
			if matName == "" {
				continue
			}
			kind := strings.ToLower(firstNonEmpty(strAny(ing["kind"]), "resource"))
			entityKind := "item"
			if kind == "resource" {
				entityKind = "commodity"
			}
			matUUID := firstNonEmpty(strAny(ing["resource_type_uuid"]), strAny(ing["item_uuid"]))
			uuid := ""
			if matUUID != "" {
				uuid = "scw-mat-" + matUUID
			}
			matID, err := UpsertEntity(conn, entityKind, matName, scwBPGameVersion, "", uuid)
			if err != nil {
				return count, err
			}
			_ = UpsertAlias(conn, matID, matName, "en", scwBPSource)
			var qty *float64
			unit := "count"
			if q := maybeFloat(ing["quantity_scu"]); q != nil {
				qty = q
				unit = "SCU"
			} else if q := maybeFloat(ing["quantity"]); q != nil {
				qty = q
			}
			meta, _ := json.Marshal(map[string]any{"unit": unit, "ingredient_kind": kind})
			_ = InsertRelation(conn, entityID, matID, "blueprint_requires", scwBPSource, scwBPGameVersion, qty, string(meta))
		}
		count++
	}
	return count, nil
}

func scwPaginate(client *http.Client, path string, pageSize int, delay time.Duration) ([]map[string]any, error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	page := 1
	var out []map[string]any
	for {
		q := url.Values{}
		q.Set("page[size]", strconv.Itoa(pageSize))
		q.Set("page[number]", strconv.Itoa(page))
		req, err := http.NewRequest(http.MethodGet, scwBase+path+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", scwUserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			snip := string(body)
			if len(snip) > 200 {
				snip = snip[:200]
			}
			return nil, fmt.Errorf("SCW HTTP %d: %s", resp.StatusCode, snip)
		}
		var envelope map[string]any
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, err
		}
		chunk := asAnySlice(envelope["data"])
		for _, item := range chunk {
			if m := asStringMap(item); m != nil {
				out = append(out, m)
			}
		}
		meta := asStringMap(envelope["meta"])
		last := page
		if meta != nil {
			if n := maybeFloat(meta["last_page"]); n != nil {
				last = int(*n)
			}
		}
		if page >= last || len(chunk) == 0 {
			break
		}
		page++
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	return out, nil
}

// SyncScwVehicles applies offline vehicle list or fetches /api/vehicles.
func SyncScwVehicles(conn *sql.DB, vehicles []map[string]any) (int, error) {
	runID, err := StartRun(conn, scwAPISource, "")
	if err != nil {
		return 0, err
	}
	var fetchErr error
	if vehicles == nil {
		vehicles, fetchErr = scwPaginate(nil, "/api/vehicles", 50, 50*time.Millisecond)
		if fetchErr != nil {
			_ = FinishRun(conn, runID, "failed", nil, fetchErr.Error(), "")
			return 0, fetchErr
		}
	}
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyScwVehicles(tx, vehicles, runID, scwAPIGameVersion)
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", scwAPIGameVersion); err != nil {
		return count, err
	}
	return count, nil
}

// SyncScwBlueprints applies offline blueprint rows or fetches /api/blueprints.
func SyncScwBlueprints(conn *sql.DB, rows []map[string]any) (int, error) {
	runID, err := StartRun(conn, scwBPSource, scwBPGameVersion)
	if err != nil {
		return 0, err
	}
	var fetchErr error
	if rows == nil {
		rows, fetchErr = scwPaginate(nil, "/api/blueprints", 50, 50*time.Millisecond)
		if fetchErr != nil {
			_ = FinishRun(conn, runID, "failed", nil, fetchErr.Error(), "")
			return 0, fetchErr
		}
	}
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyScwBlueprints(tx, rows, runID)
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", scwBPGameVersion); err != nil {
		return count, err
	}
	return count, nil
}
