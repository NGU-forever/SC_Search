package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	uexDefaultBase = "https://api.uexcorp.space/2.0"
	uexUserAgent   = "SC_Search-ingest/0.1"
	uexSource      = "uex"
)

// UexPayload is the offline/test input for ApplyUex.
type UexPayload struct {
	Terminals        []map[string]any
	Commodities      []map[string]any
	Prices           []map[string]any
	Vehicles         []map[string]any
	VehiclePurchases []map[string]any
	GameVersion      string
}

func pickGameVersion(groups ...[]map[string]any) string {
	for _, g := range groups {
		for _, item := range g {
			if v := strAny(item["game_version"]); v != "" {
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

func findExistingShipID(conn *sql.DB, names ...string) (int64, bool) {
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
func ApplyUex(conn *sql.DB, runID int64, p UexPayload) (int, error) {
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
	return n + n2, err
}

func applyUexPayload(conn *sql.DB, runID int64, gameVersion string, terminals, commodities, prices []map[string]any) (int, error) {
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

func applyUexVehiclePurchases(conn *sql.DB, runID int64, gameVersion string, terminals, vehicles, purchases []map[string]any) (int, error) {
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadUEXToken(repoRoot string) string {
	if t := strings.TrimSpace(os.Getenv("UEX_API_TOKEN")); t != "" {
		return t
	}
	// Minimal .env parse for UEX_API_TOKEN=
	data, err := os.ReadFile(filepath.Join(repoRoot, "config", ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "UEX_API_TOKEN=") {
			return strings.Trim(strings.TrimPrefix(line, "UEX_API_TOKEN="), "\"'")
		}
	}
	return ""
}

func uexGet(client *http.Client, base, token, path string) ([]map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", uexUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("UEX rate limit reached")
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("UEX non-JSON response HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("UEX HTTP %d: %v", resp.StatusCode, envelope)
	}
	if st, ok := envelope["status"]; ok && st != nil && st != "ok" {
		return nil, fmt.Errorf("UEX status=%v: %v", st, envelope)
	}
	raw, ok := envelope["data"]
	if !ok {
		raw = envelope
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s response was not a list", path)
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// SyncUexLive fetches UEX API and applies (requires UEX_API_TOKEN).
func SyncUexLive(conn *sql.DB, repoRoot string) (int, error) {
	token := loadUEXToken(repoRoot)
	if token == "" {
		return 0, fmt.Errorf("UEX_API_TOKEN missing; set it in config/.env")
	}
	runID, err := StartRun(conn, uexSource, "")
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	base := uexDefaultBase
	terminals, err := uexGet(client, base, token, "terminals")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	commodities, err := uexGet(client, base, token, "commodities")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	prices, err := uexGet(client, base, token, "commodities_prices_all")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	vehicles, err := uexGet(client, base, token, "vehicles")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	purchases, err := uexGet(client, base, token, "vehicles_purchases_prices_all")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	payload := UexPayload{
		Terminals: terminals, Commodities: commodities, Prices: prices,
		Vehicles: vehicles, VehiclePurchases: purchases,
	}
	count, err := ApplyUex(conn, runID, payload)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	gv := pickGameVersion(terminals, commodities, prices, vehicles, purchases)
	if gv == "" {
		gv = "unknown"
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}

// SyncUexPayload is for tests / offline JSON dumps.
func SyncUexPayload(conn *sql.DB, payload UexPayload) (int, error) {
	runID, err := StartRun(conn, uexSource, "")
	if err != nil {
		return 0, err
	}
	count, err := ApplyUex(conn, runID, payload)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	gv := payload.GameVersion
	if gv == "" {
		gv = pickGameVersion(payload.Terminals, payload.Commodities, payload.Prices)
	}
	if gv == "" {
		gv = "unknown"
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}
