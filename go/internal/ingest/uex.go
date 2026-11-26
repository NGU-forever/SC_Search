package ingest

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

const (
	uexDefaultBase = "https://api.uexcorp.space/2.0"
	uexUserAgent   = "SC_Search-ingest/0.1"
	uexSource      = "uex"
)

type UexPayload struct {
	Terminals        []map[string]any
	Commodities      []map[string]any
	Prices           []map[string]any
	Vehicles         []map[string]any
	VehiclePurchases []map[string]any
	GameVersion      string
}

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
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyUex(tx, runID, payload)
	})
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
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyUex(tx, runID, payload)
	})
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
