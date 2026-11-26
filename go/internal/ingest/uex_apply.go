package ingest

import (
	"fmt"
	"os"
)

func ApplyUex(conn DBTX, runID int64, p UexPayload) (int, error) {
	gv := p.GameVersion
	if gv == "" {
		gv = pickGameVersion(p.Terminals, p.Commodities, p.Prices, p.Vehicles, p.VehiclePurchases)
	}
	if gv == "" {
		gv = "unknown"
	}
	n, err := applyUexPayload(conn, runID, gv, p.Terminals, p.Commodities, p.Prices)
	if err != nil {
		return n, err
	}
	n2, err := applyUexVehiclePurchases(conn, runID, gv, p.Terminals, p.Vehicles, p.VehiclePurchases)
	if err != nil {
		return n + n2, err
	}
	if err := pruneUexPrices(conn, runID, gv); err != nil {
		return n + n2, err
	}
	return n + n2, nil
}

// pruneUexPrices makes the successful UEX run the authoritative price snapshot:
// quotes for this source+version not written by runID are removed (upstream delisted them).
// Skipped when the run wrote no prices at all, so an empty/partial API reply cannot wipe the table.
// Set SC_SEARCH_UEX_PRUNE_OTHER_VERSIONS=1 to also drop uex quotes from other game versions.

func pruneUexPrices(conn DBTX, runID int64, gameVersion string) error {
	var written int
	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM prices
		WHERE source = ? AND game_version = ? AND ingest_run_id = ?
	`, uexSource, gameVersion, runID).Scan(&written); err != nil {
		return err
	}
	if written == 0 {
		return nil
	}
	if _, err := conn.Exec(`
		DELETE FROM prices
		WHERE source = ? AND game_version = ? AND (ingest_run_id IS NULL OR ingest_run_id != ?)
	`, uexSource, gameVersion, runID); err != nil {
		return err
	}
	if os.Getenv("SC_SEARCH_UEX_PRUNE_OTHER_VERSIONS") == "1" {
		if _, err := conn.Exec(`
			DELETE FROM prices WHERE source = ? AND game_version != ?
		`, uexSource, gameVersion); err != nil {
			return err
		}
	}
	return nil
}

func applyUexPayload(conn DBTX, runID int64, gameVersion string, terminals, commodities, prices []map[string]any) (int, error) {
	terminalIDs := map[int]int64{}
	commodityIDs := map[int]int64{}
	count := 0

	for _, t := range terminals {
		uexID, ok := asIntID(t["id"])
		name := firstNonEmpty(strAny(t["name"]), strAny(t["displayname"]), strAny(t["fullname"]))
		if !ok || name == "" {
			continue
		}
		entityID, err := UpsertEntity(conn, "terminal", name, gameVersion, strAny(t["code"]), fmt.Sprintf("uex-terminal-%d", uexID))
		if err != nil {
			return count, err
		}
		terminalIDs[uexID] = entityID
		if err := UpsertAlias(conn, entityID, name, "en", uexSource); err != nil {
			return count, err
		}
		sys := firstNonEmpty(
			strAny(t["star_system_name"]),
			strAny(t["system_name"]),
			strAny(t["star_system"]),
			inferSystemFromTerminalName(name),
		)
		if sys != "" {
			run := runID
			_ = UpsertFact(conn, entityID, "system", uexSource, gameVersion, &run, sys, nil, "")
		}
		count++
	}
	for _, c := range commodities {
		uexID, ok := asIntID(c["id"])
		name := strAny(c["name"])
		if !ok || name == "" {
			continue
		}
		uuid := strAny(c["uuid"])
		if uuid == "" {
			uuid = fmt.Sprintf("uex-commodity-%d", uexID)
		}
		entityID, err := UpsertEntity(conn, "commodity", name, gameVersion, strAny(c["slug"]), uuid)
		if err != nil {
			return count, err
		}
		commodityIDs[uexID] = entityID
		if err := UpsertAlias(conn, entityID, name, "en", uexSource); err != nil {
			return count, err
		}
		count++
	}
	for _, p := range prices {
		cUEX, okC := asIntID(p["id_commodity"])
		tUEX, okT := asIntID(p["id_terminal"])
		if !okC || !okT {
			continue
		}
		commodityID, hasC := commodityIDs[cUEX]
		terminalID, hasT := terminalIDs[tUEX]
		if !hasC {
			if cname := strAny(p["commodity_name"]); cname != "" {
				id, err := UpsertEntity(conn, "commodity", cname, gameVersion, "", fmt.Sprintf("uex-commodity-%d", cUEX))
				if err != nil {
					return count, err
				}
				commodityIDs[cUEX] = id
				commodityID = id
				_ = UpsertAlias(conn, id, cname, "en", uexSource)
				hasC = true
			}
		}
		if !hasT {
			tname := firstNonEmpty(strAny(p["terminal_name"]), strAny(p["terminal_displayname"]))
			if tname != "" {
				id, err := UpsertEntity(conn, "terminal", tname, gameVersion, "", fmt.Sprintf("uex-terminal-%d", tUEX))
				if err != nil {
					return count, err
				}
				terminalIDs[tUEX] = id
				terminalID = id
				_ = UpsertAlias(conn, id, tname, "en", uexSource)
				hasT = true
			}
		}
		if !hasC || !hasT {
			continue
		}
		reported := firstNonEmpty(strAny(p["date_modified"]), strAny(p["date_added"]))
		if buy := maybeFloat(p["price_buy"]); buy != nil && *buy > 0 {
			scu := maybeFloat(p["scu_buy"])
			if scu == nil {
				scu = maybeFloat(p["scu_buy_avg"])
			}
			if err := ReplacePrice(conn, commodityID, terminalID, "buy", *buy, uexSource, gameVersion, runID, scu, reported); err != nil {
				return count, err
			}
			count++
		}
		if sell := maybeFloat(p["price_sell"]); sell != nil && *sell > 0 {
			scu := maybeFloat(p["scu_sell"])
			if scu == nil {
				scu = maybeFloat(p["scu_sell_avg"])
			}
			if err := ReplacePrice(conn, commodityID, terminalID, "sell", *sell, uexSource, gameVersion, runID, scu, reported); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func applyUexVehiclePurchases(conn DBTX, runID int64, gameVersion string, terminals, vehicles, purchases []map[string]any) (int, error) {
	terminalIDs := map[int]int64{}
	vehicleIDs := map[int]int64{}
	count := 0

	for _, t := range terminals {
		uexID, ok := asIntID(t["id"])
		name := firstNonEmpty(strAny(t["name"]), strAny(t["displayname"]), strAny(t["nickname"]))
		if !ok || name == "" {
			continue
		}
		entityID, err := UpsertEntity(conn, "terminal", name, gameVersion, strAny(t["code"]), fmt.Sprintf("uex-terminal-%d", uexID))
		if err != nil {
			return count, err
		}
		terminalIDs[uexID] = entityID
		_ = UpsertAlias(conn, entityID, name, "en", uexSource)
	}
	for _, v := range vehicles {
		uexID, ok := asIntID(v["id"])
		name := strAny(v["name"])
		nameFull := firstNonEmpty(strAny(v["name_full"]), name)
		if !ok || name == "" {
			continue
		}
		shipID, found := findExistingShipID(conn, nameFull, name)
		if !found {
			var err error
			shipID, err = UpsertEntity(conn, "ship", nameFull, gameVersion, strAny(v["slug"]), fmt.Sprintf("uex-vehicle-%d", uexID))
			if err != nil {
				return count, err
			}
			count++
		}
		vehicleIDs[uexID] = shipID
		for _, alias := range []string{name, nameFull} {
			if alias != "" {
				_ = UpsertAlias(conn, shipID, alias, "en", uexSource)
			}
		}
	}
	for _, p := range purchases {
		vUEX, okV := asIntID(p["id_vehicle"])
		tUEX, okT := asIntID(p["id_terminal"])
		buy := maybeFloat(p["price_buy"])
		if !okV || !okT || buy == nil || *buy <= 0 {
			continue
		}
		shipID, hasS := vehicleIDs[vUEX]
		terminalID, hasT := terminalIDs[tUEX]
		if !hasS {
			if vname := strAny(p["vehicle_name"]); vname != "" {
				id, found := findExistingShipID(conn, vname)
				if !found {
					var err error
					id, err = UpsertEntity(conn, "ship", vname, gameVersion, "", fmt.Sprintf("uex-vehicle-%d", vUEX))
					if err != nil {
						return count, err
					}
				}
				vehicleIDs[vUEX] = id
				shipID = id
				_ = UpsertAlias(conn, id, vname, "en", uexSource)
				hasS = true
			}
		}
		if !hasT {
			if tname := strAny(p["terminal_name"]); tname != "" {
				id, err := UpsertEntity(conn, "terminal", tname, gameVersion, "", fmt.Sprintf("uex-terminal-%d", tUEX))
				if err != nil {
					return count, err
				}
				terminalIDs[tUEX] = id
				terminalID = id
				_ = UpsertAlias(conn, id, tname, "en", uexSource)
				hasT = true
			}
		}
		if !hasS || !hasT {
			continue
		}
		reported := firstNonEmpty(strAny(p["date_modified"]), strAny(p["date_added"]))
		if err := ReplacePrice(conn, shipID, terminalID, "buy", *buy, uexSource, gameVersion, runID, nil, reported); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
