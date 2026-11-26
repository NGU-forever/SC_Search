package ingest

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	locOrgURL      = "https://ini.42kit.com/orginal/global.ini"
	locFullURL     = "https://ini.42kit.com/full/global.ini"
	locUserAgent   = "SC_Search-ingest/0.1"
	locGameVersion = "localization-full"
	locSource      = "localization"
)

var namePrefixes = []string{
	"vehicle_Name", "vehicle_name", "items_commodities_",
	"item_Name", "item_name_", "mineabletype_",
}

var descPrefixes = []string{"vehicle_Desc", "vehicle_desc"}

// ParseINI parses a localization global.ini into key→value.
func ParseINI(text string) map[string]string {
	text = strings.TrimPrefix(text, "\ufeff")
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, ";") || strings.HasPrefix(raw, "#") {
			continue
		}
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			continue
		}
		out[strings.TrimSpace(raw[:eq])] = strings.TrimSpace(raw[eq+1:])
	}
	return out
}

func isShortName(value string, maxLen int) bool {
	if value == "" || len(value) > maxLen {
		return false
	}
	if strings.Contains(value, "\n") || strings.Contains(value, `\n`) {
		return false
	}
	allDigit := true
	for _, r := range value {
		if r < '0' || r > '9' {
			allDigit = false
			break
		}
	}
	return !allDigit
}

func kindForKey(key string) string {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "vehicle_") {
		return "ship"
	}
	if strings.Contains(lower, "commodit") || strings.HasPrefix(lower, "mineabletype_") {
		return "commodity"
	}
	return "item"
}

func buildNameIndex(conn DBTX) (map[string]int64, error) {
	index := map[string]int64{}
	rows, err := conn.Query(`SELECT id, canonical_name FROM entities`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		index[strings.ToLower(name)] = id
	}
	rows.Close()
	arows, err := conn.Query(`SELECT entity_id, alias FROM aliases`)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var id int64
		var alias string
		if err := arows.Scan(&id, &alias); err != nil {
			return nil, err
		}
		index[strings.ToLower(alias)] = id
	}
	return index, arows.Err()
}

// ApplyLocalizationMaps mirrors Python apply_localization_maps.
func ApplyLocalizationMaps(conn DBTX, enMap, zhMap map[string]string, runID int64, gameVersion string) (int, error) {
	if gameVersion == "" {
		gameVersion = locGameVersion
	}
	index, err := buildNameIndex(conn)
	if err != nil {
		return 0, err
	}
	remember := func(entityID int64, names ...string) {
		for _, name := range names {
			if name != "" {
				index[strings.ToLower(name)] = entityID
			}
		}
	}
	ensure := func(english, kind string) (int64, error) {
		if hit, ok := index[strings.ToLower(english)]; ok {
			return hit, nil
		}
		entityID, err := UpsertEntity(conn, kind, english, gameVersion, "", "zh-loc-"+kind+"-"+english)
		if err != nil {
			return 0, err
		}
		if err := UpsertAlias(conn, entityID, english, "en", locSource); err != nil {
			return 0, err
		}
		remember(entityID, english)
		return entityID, nil
	}

	keys := map[string]struct{}{}
	for k := range enMap {
		if _, ok := zhMap[k]; ok {
			keys[k] = struct{}{}
		}
	}
	count := 0

	for key := range keys {
		match := false
		for _, p := range namePrefixes {
			if strings.HasPrefix(key, p) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		en, zh := enMap[key], zhMap[key]
		if en == "" || zh == "" || en == zh {
			continue
		}
		if !isShortName(en, 64) || !isShortName(zh, 80) {
			continue
		}
		entityID, err := ensure(en, kindForKey(key))
		if err != nil {
			return count, err
		}
		if err := UpsertAlias(conn, entityID, zh, "zh", locSource); err != nil {
			return count, err
		}
		remember(entityID, zh)
		count++
	}

	for key := range keys {
		descMatch := false
		for _, p := range descPrefixes {
			if strings.HasPrefix(key, p) {
				descMatch = true
				break
			}
		}
		if !descMatch {
			continue
		}
		zh := zhMap[key]
		if zh == "" || len(zh) < 8 {
			continue
		}
		nameKey := strings.ReplaceAll(key, "vehicle_Desc", "vehicle_name")
		nameKey = strings.ReplaceAll(nameKey, "vehicle_desc", "vehicle_name")
		shipEN := enMap[nameKey]
		if shipEN == "" {
			shipEN = enMap[strings.ReplaceAll(nameKey, "vehicle_name", "vehicle_Name")]
		}
		if shipEN == "" {
			shipEN = enMap[nameKey+"_short"]
			if shipEN == "" {
				alt := strings.ReplaceAll(nameKey, "vehicle_name", "vehicle_Name") + "_short"
				shipEN = enMap[alt]
			}
		}
		if shipEN == "" || !isShortName(shipEN, 80) {
			continue
		}
		entityID, err := ensure(shipEN, "ship")
		if err != nil {
			return count, err
		}
		runCopy := runID
		body := zh
		if len(body) > 4000 {
			body = body[:4000]
		}
		if err := UpsertFact(conn, entityID, "description_zh", locSource, gameVersion, &runCopy, body, nil, ""); err != nil {
			return count, err
		}
		count++
	}

	for key := range keys {
		en, zh := enMap[key], zhMap[key]
		if en == "" || zh == "" || en == zh {
			continue
		}
		if !isShortName(en, 64) || !isShortName(zh, 80) {
			continue
		}
		entityID, ok := index[strings.ToLower(en)]
		if !ok {
			continue
		}
		if err := UpsertAlias(conn, entityID, zh, "zh", locSource); err != nil {
			return count, err
		}
		remember(entityID, zh)
		count++
	}
	return count, nil
}

func loadCachedINIPair(rawDir string) (map[string]string, map[string]string, error) {
	orgPath := filepath.Join(rawDir, "global.orginal.ini")
	zhPath := filepath.Join(rawDir, "global.full.ini")
	orgRaw, err := os.ReadFile(orgPath)
	if err != nil {
		return nil, nil, err
	}
	zhRaw, err := os.ReadFile(zhPath)
	if err != nil {
		return nil, nil, err
	}
	return ParseINI(string(orgRaw)), ParseINI(string(zhRaw)), nil
}

func fetchINIPair(rawDir string, client *http.Client) (map[string]string, map[string]string, error) {
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	download := func(u string) ([]byte, error) {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", locUserAgent)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
		}
		return io.ReadAll(resp.Body)
	}
	orgRaw, err1 := download(locOrgURL)
	zhRaw, err2 := download(locFullURL)
	if err1 != nil || err2 != nil {
		if cachedEn, cachedZh, cerr := loadCachedINIPair(rawDir); cerr == nil {
			return cachedEn, cachedZh, nil
		}
		if err1 != nil {
			return nil, nil, fmt.Errorf("download failed and no local cache: %w", err1)
		}
		return nil, nil, fmt.Errorf("download failed and no local cache: %w", err2)
	}
	_ = os.MkdirAll(rawDir, 0o755)
	_ = os.WriteFile(filepath.Join(rawDir, "global.orginal.ini"), orgRaw, 0o644)
	_ = os.WriteFile(filepath.Join(rawDir, "global.full.ini"), zhRaw, 0o644)
	return ParseINI(string(orgRaw)), ParseINI(string(zhRaw)), nil
}

// SyncLocalization applies EN/ZH maps (or downloads / uses data/raw cache).
func SyncLocalization(conn *sql.DB, repoRoot string, enMap, zhMap map[string]string) (int, error) {
	runID, err := StartRun(conn, locSource, "")
	if err != nil {
		return 0, err
	}
	if enMap == nil || zhMap == nil {
		rawDir := filepath.Join(repoRoot, "data", "raw")
		enMap, zhMap, err = fetchINIPair(rawDir, nil)
		if err != nil {
			_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
			return 0, err
		}
	}
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyLocalizationMaps(tx, enMap, zhMap, runID, locGameVersion)
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	notes := "汉化组 global.ini CC BY-NC-SA 4.0; personal non-commercial use"
	if err := FinishRun(conn, runID, "ok", &rc, notes, locGameVersion); err != nil {
		return count, err
	}
	return count, nil
}
