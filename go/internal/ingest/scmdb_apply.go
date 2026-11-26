package ingest

import "github.com/frankxiong/sc-search/internal/anyx"

// scmdbApply carries the cross-phase state of one SCMDB payload apply.
type scmdbApply struct {
	conn        DBTX
	runID       int64
	version     string
	factionIDs  map[string]int64
	poolIDs     map[string]int64
	locationIDs map[string]int64
	count       int
}

// ApplyScmdbPayload mirrors Python apply_scmdb_payload.
func ApplyScmdbPayload(conn DBTX, payload map[string]any, runID int64, gameVersion string) (int, error) {
	if err := ClearSourceFactsRelations(conn, scmdbSource, gameVersion); err != nil {
		return 0, err
	}
	a := &scmdbApply{
		conn:        conn,
		runID:       runID,
		version:     gameVersion,
		factionIDs:  map[string]int64{},
		poolIDs:     map[string]int64{},
		locationIDs: map[string]int64{},
	}
	phases := []func(map[string]any) error{
		a.factions,
		a.blueprintPools,
		a.locations,
		a.missions,
	}
	for _, phase := range phases {
		if err := phase(payload); err != nil {
			return a.count, err
		}
	}
	return a.count, nil
}

func (a *scmdbApply) factText(entityID int64, key string, value any) {
	s := anyx.String(value)
	if s == "" {
		return
	}
	_ = UpsertFact(a.conn, entityID, key, scmdbSource, a.version, &a.runID, s, nil, "")
}

func (a *scmdbApply) factNum(entityID int64, key string, value any, unit string) {
	n := maybeFloat(value)
	if n == nil {
		return
	}
	_ = UpsertFact(a.conn, entityID, key, scmdbSource, a.version, &a.runID, "", n, unit)
}
