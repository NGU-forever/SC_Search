package router

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed intent_tools.json
var intentToolsJSON []byte

var (
	playbooksOnce sync.Once
	playbooks     map[string]map[string]any
	playbooksErr  error
)

func loadPlaybooks() (map[string]map[string]any, error) {
	playbooksOnce.Do(func() {
		playbooksErr = json.Unmarshal(intentToolsJSON, &playbooks)
	})
	return playbooks, playbooksErr
}

func playbookFor(intent string) map[string]any {
	pb, err := loadPlaybooks()
	if err != nil || pb == nil {
		return nil
	}
	return pb[strings.TrimSpace(intent)]
}
