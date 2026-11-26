// Package testfixture builds a small, deterministic knowledge DB through the real
// ingest write paths so tests do not depend on the developer's live database.
package testfixture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/frankxiong/sc-search/internal/ingest"
)

const (
	// LiveVersion is the SCMDB version stored as the Live reference.
	LiveVersion = "4.10.3-live.13220821"
	// FreshUEXVersion matches Live and must not be marked stale.
	FreshUEXVersion = "4.10.3"
	// StaleUEXVersion lags Live and must trigger the stale price wording.
	StaleUEXVersion = "3.24.2"
)

// Options selects fixture variants.
type Options struct {
	// UEXGameVersion overrides the UEX snapshot version. Empty = FreshUEXVersion.
	UEXGameVersion string
}

// New builds a fixture DB with a fresh (Live-matching) UEX snapshot.
func New(t *testing.T) *sql.DB {
	t.Helper()
	return NewWithOptions(t, Options{})
}

// NewWithOptions builds a fixture DB. Callers own the returned *sql.DB until cleanup.
func NewWithOptions(t *testing.T, opts Options) *sql.DB {
	t.Helper()
	conn, err := ingest.Open(filepath.Join(t.TempDir(), "fixture.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := ingest.InitDB(conn); err != nil {
		t.Fatal(err)
	}

	seedShips(t, conn)
	uexVersion := opts.UEXGameVersion
	if uexVersion == "" {
		uexVersion = FreshUEXVersion
	}
	seedPrices(t, conn, uexVersion)
	seedMissions(t, conn)
	seedBlueprints(t, conn)
	seedDocs(t, conn)
	seedAliases(t, conn)
	return conn
}

// Root writes the review/guide assets the fixture needs and returns a repo-like root.
func Root(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	guides := map[string][]map[string]string{
		"loadout":     {{"title": "Erkul Browse", "url": "https://erkul.games/browse", "kind": "tool"}},
		"mining":      {{"title": "Mining guide", "url": "https://example.invalid/mining", "kind": "guide"}},
		"client":      {{"title": "Client help", "url": "https://example.invalid/client", "kind": "guide"}},
		"pledge":      {{"title": "Pledge FAQ", "url": "https://example.invalid/pledge", "kind": "guide"}},
		"event":       {{"title": "Event guide", "url": "https://example.invalid/event", "kind": "guide"}},
		"earn_meta":   {{"title": "Earn starter", "url": "https://example.invalid/earn", "kind": "guide"}},
		"howto":       {{"title": "How to", "url": "https://example.invalid/howto", "kind": "guide"}},
		"ship_shop":   {{"title": "Ship shop", "url": "https://example.invalid/ships", "kind": "guide"}},
		"trade_route": {{"title": "Trade routes", "url": "https://example.invalid/trade", "kind": "guide"}},
	}
	raw, err := json.MarshalIndent(guides, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "community_guides.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func seedShips(t *testing.T, conn *sql.DB) {
	t.Helper()
	vehicles := []map[string]any{
		scwVehicle("u-gladius", "Gladius", "Aegis Gladius", "gladius", 0,
			map[string]any{"name": "Aegis Dynamics", "code": "AEGS"},
			map[string][2]string{
				"size": {"Small", "小"}, "type": {"Combat", "战斗"},
				"production_status": {"Flight Ready", "可飞行"}, "career": {"Combat", "打击"},
			},
			[][2]string{{"Combat", "战斗"}, {"Exploration", "勘探"}},
		),
		scwVehicle("u-meteor", "Meteor", "RSI Meteor", "meteor", 0,
			map[string]any{"name": "Roberts Space Industries", "code": "RSI"},
			map[string][2]string{
				"size": {"Small", "小"}, "type": {"Combat", "战斗"},
				"production_status": {"Flight Ready", "可飞行"}, "career": {"Combat", "打击"},
			},
			[][2]string{{"Combat", "战斗"}},
		),
		scwVehicle("u-carrack", "Carrack", "Anvil Carrack", "carrack", 456,
			map[string]any{"name": "Anvil Aerospace", "code": "ANVL"},
			map[string][2]string{
				"size": {"Large", "大"}, "type": {"Exploration", "勘探"},
				"production_status": {"Flight Ready", "可飞行"}, "career": {"Exploration", "勘探"},
			},
			[][2]string{{"Exploration", "勘探"}, {"Cargo", "运输"}},
		),
	}
	if _, err := ingest.SyncScwVehicles(conn, vehicles); err != nil {
		t.Fatal(err)
	}
}

func scwVehicle(uuid, name, gameName, slug string, cargo float64, manufacturer map[string]any, labels map[string][2]string, foci [][2]string) map[string]any {
	v := map[string]any{
		"uuid": uuid, "name": name, "game_name": gameName, "slug": slug,
		"cargo_capacity": cargo, "manufacturer": manufacturer,
	}
	for key, pair := range labels {
		v[key] = map[string]any{"en_EN": pair[0], "zh_CN": pair[1]}
	}
	if len(foci) > 0 {
		arr := make([]any, 0, len(foci))
		for _, pair := range foci {
			arr = append(arr, map[string]any{"en_EN": pair[0], "zh_CN": pair[1]})
		}
		v["foci"] = arr
	}
	return v
}

func seedPrices(t *testing.T, conn *sql.DB, gameVersion string) {
	t.Helper()
	terminals := []map[string]any{
		{"id": 1.0, "name": "Area 18 TDD", "code": "A18", "star_system_name": "Stanton"},
		{"id": 2.0, "name": "Checkmate", "code": "CHK", "star_system_name": "Pyro"},
	}
	commodities := []map[string]any{
		{"id": 10.0, "name": "Gold", "slug": "gold"},
		{"id": 11.0, "name": "Quantanium", "slug": "quantanium"},
	}
	prices := []map[string]any{
		{"id_commodity": 10.0, "id_terminal": 1.0, "price_buy": 7000.0, "price_sell": 9500.0, "scu_buy": 100.0, "scu_sell": 100.0},
		{"id_commodity": 10.0, "id_terminal": 2.0, "price_buy": 7200.0, "price_sell": 8800.0, "scu_buy": 80.0, "scu_sell": 80.0},
		{"id_commodity": 11.0, "id_terminal": 1.0, "price_buy": 6500.0, "price_sell": 8800.0, "scu_buy": 40.0, "scu_sell": 40.0},
		{"id_commodity": 11.0, "id_terminal": 2.0, "price_buy": 6400.0, "price_sell": 9100.0, "scu_buy": 60.0, "scu_sell": 60.0},
	}
	if _, err := ingest.SyncUexPayload(conn, ingest.UexPayload{
		GameVersion: gameVersion,
		Terminals:   terminals,
		Commodities: commodities,
		Prices:      prices,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedMissions(t *testing.T, conn *sql.DB) {
	t.Helper()
	factions := map[string]any{
		"f-bit": map[string]any{"name": "Bit Zeros"},
		"f-red": map[string]any{"name": "Red Wind Linehaul"},
	}
	locations := map[string]any{
		"loc-checkmate": map[string]any{"name": "Checkmate", "type": "Station", "system": "Pyro"},
		"loc-area18":    map[string]any{"name": "Area 18 TDD", "type": "LandingZone", "system": "Stanton"},
	}
	contracts := []any{
		map[string]any{
			"id": "mission-bit-heist", "debugName": "Bit Zeros Ship Heist", "title": "Ship Heist",
			"contractorName": "Bit Zeros", "factionGuid": "f-bit",
			"rewardUEC": 120000.0, "buyIn": 13500.0, "personalCooldownTime": 45.0,
			"timeToComplete": 2.0, "missionType": "Mercenary",
			"availableSystems": []any{"Pyro"},
			"locations":        []any{"loc-checkmate", "loc-area18"},
			"minStanding":      map[string]any{"name": "Junior", "minReputation": 500.0},
		},
		map[string]any{
			"id": "mission-bit-escort", "debugName": "Bit Zeros Cargo Escort", "title": "Cargo Escort",
			"contractorName": "Bit Zeros", "factionGuid": "f-bit",
			"rewardUEC": 45000.0, "buyIn": 5000.0, "personalCooldownTime": 30.0,
			"timeToComplete": 1.0, "missionType": "Hauling",
			"availableSystems": []any{"Pyro"},
			"minStanding":      map[string]any{"name": "Junior", "minReputation": 250.0},
		},
		map[string]any{
			"id": "mission-bit-bounty", "debugName": "Bit Zeros Bounty", "title": "Bounty",
			"contractorName": "Bit Zeros", "factionGuid": "f-bit",
			"rewardUEC": 90000.0, "buyIn": 8000.0, "personalCooldownTime": 60.0,
			"timeToComplete": 1.5, "missionType": "Bounty",
			"availableSystems": []any{"Pyro"},
			"minStanding":      map[string]any{"name": "Member", "minReputation": 1000.0},
		},
		map[string]any{
			"id": "mission-red-junior", "debugName": "Red Wind Linehaul Junior Delivery", "title": "Junior Delivery",
			"contractorName": "Red Wind Linehaul", "factionGuid": "f-red",
			"rewardUEC": 30000.0, "buyIn": 2500.0, "personalCooldownTime": 20.0,
			"timeToComplete": 1.0, "missionType": "Hauling",
			"availableSystems": []any{"Stanton"},
			"locations":        []any{"loc-area18"},
			"minStanding":      map[string]any{"name": "Junior", "minReputation": 100.0},
		},
		map[string]any{
			"id": "mission-red-senior", "debugName": "Red Wind Linehaul Senior Haulage", "title": "Senior Haulage",
			"contractorName": "Red Wind Linehaul", "factionGuid": "f-red",
			"rewardUEC": 60000.0, "buyIn": 4000.0, "personalCooldownTime": 40.0,
			"timeToComplete": 1.5, "missionType": "Hauling",
			"availableSystems": []any{"Stanton"},
			"locations":        []any{"loc-area18"},
			"minStanding":      map[string]any{"name": "Senior", "minReputation": 1500.0},
		},
	}
	// Extra Bit Zeros missions push the faction past the 50-row mission list cap,
	// so the worker can exercise the truncation warning (返回 50/51).
	for i := 1; i <= 48; i++ {
		suffix := fmt.Sprintf("%02d", i)
		contracts = append(contracts, map[string]any{
			"id":               "mission-bit-op-" + suffix,
			"debugName":        "Bit Zeros Mercenary Op " + suffix,
			"title":            "Mercenary Op " + suffix,
			"contractorName":   "Bit Zeros",
			"factionGuid":      "f-bit",
			"rewardUEC":        float64(20000 + (i%7)*5000),
			"timeToComplete":   1.0,
			"missionType":      "Mercenary",
			"availableSystems": []any{"Pyro"},
			"minStanding":      map[string]any{"name": "Junior", "minReputation": 200.0},
		})
	}
	payload := map[string]any{
		"version":       LiveVersion,
		"factions":      factions,
		"locationPools": locations,
		"contracts":     contracts,
	}
	if _, err := ingest.SyncScmdbPayload(conn, payload); err != nil {
		t.Fatal(err)
	}
}

func seedBlueprints(t *testing.T, conn *sql.DB) {
	t.Helper()
	rows := []map[string]any{
		{
			"uuid": "bp-test", "output_name": "Test BP", "craft_time_seconds": 60.0,
			"ingredients": []any{
				map[string]any{"name": "Agricium", "kind": "resource", "quantity_scu": 1.5},
				map[string]any{"name": "Bexalite", "kind": "resource", "quantity_scu": 2.0},
			},
		},
		{
			"uuid": "bp-salvage", "output_name": "Salvage Rig Blueprint", "craft_time_seconds": 120.0,
			"ingredients": []any{
				map[string]any{"name": "Bexalite", "kind": "resource", "quantity_scu": 0.5},
				map[string]any{"name": "Titanium", "kind": "item", "quantity": 4.0},
			},
		},
	}
	if _, err := ingest.SyncScwBlueprints(conn, rows); err != nil {
		t.Fatal(err)
	}
}

func seedDocs(t *testing.T, conn *sql.DB) {
	t.Helper()
	var carrackID int64
	if err := conn.QueryRow(`SELECT id FROM entities WHERE kind = 'ship' AND canonical_name = 'Anvil Carrack'`).Scan(&carrackID); err != nil {
		t.Fatal(err)
	}
	if _, err := ingest.SyncWikiDocuments(conn, []ingest.WikiDoc{
		{Title: "Anvil Carrack", Body: "The Anvil Carrack is a multi-crew exploration ship with a 456 SCU cargo hold.", URL: "https://example.invalid/carrack", EntityID: &carrackID},
		{Title: "Quantanium", Body: "Quantanium is a volatile refining commodity used in high-end components."},
	}); err != nil {
		t.Fatal(err)
	}
}

func seedAliases(t *testing.T, conn *sql.DB) {
	t.Helper()
	entries := []struct {
		kind, canonical, alias, lang string
	}{
		{"ship", "Aegis Gladius", "短剑", "zh"},
		{"ship", "RSI Meteor", "流星", "zh"},
		{"ship", "Anvil Carrack", "卡拉克", "zh"},
		{"commodity", "Gold", "黄金", "zh"},
		{"commodity", "Quantanium", "量子矿", "zh"},
		{"terminal", "Area 18 TDD", "Area18", "en"},
		{"faction", "Red Wind Linehaul", "红风", "zh"},
	}
	for _, e := range entries {
		var id int64
		err := conn.QueryRow(`
			SELECT id FROM entities
			WHERE kind = ? AND canonical_name = ?
			ORDER BY id LIMIT 1
		`, e.kind, e.canonical).Scan(&id)
		if err != nil {
			t.Fatalf("fixture alias target %s/%s: %v", e.kind, e.canonical, err)
		}
		if err := ingest.UpsertAlias(conn, id, e.alias, e.lang, "seed"); err != nil {
			t.Fatal(err)
		}
	}
}
