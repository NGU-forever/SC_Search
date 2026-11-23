package mcp

import (
	"encoding/json"
)

func okJSON(data any, meta map[string]any) string {
	if meta == nil {
		meta = map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{
		"ok":   true,
		"data": data,
		"meta": meta,
	})
	return string(b)
}

func errJSON(message string) string {
	b, _ := json.Marshal(map[string]any{
		"ok":    false,
		"error": message,
		"data":  nil,
		"meta":  map[string]any{},
	})
	return string(b)
}

func noMatchJSON(message string, data any) string {
	if data == nil {
		data = []any{}
	}
	return okJSON(data, map[string]any{
		"why":     "no_match",
		"message": message,
	})
}
