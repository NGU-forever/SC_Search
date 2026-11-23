package query

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed intents_catalog.json
var intentsCatalogJSON []byte

var (
	intentsOnce sync.Once
	intentsData map[string]any
	intentsErr  error
)

// IntentsCatalog returns the machine-readable Router playbook (parity with Python intents_catalog).
func IntentsCatalog() (map[string]any, error) {
	intentsOnce.Do(func() {
		intentsErr = json.Unmarshal(intentsCatalogJSON, &intentsData)
	})
	if intentsErr != nil {
		return nil, intentsErr
	}
	// Shallow copy top-level so callers cannot mutate the cache map itself.
	out := make(map[string]any, len(intentsData))
	for k, v := range intentsData {
		out[k] = v
	}
	return out, nil
}
