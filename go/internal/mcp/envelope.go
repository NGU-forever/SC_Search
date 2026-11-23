package mcp

// Envelope helpers return map objects so MCP structuredContent is a record
// (Cursor/T3 reject outputSchema type=string / structuredContent string).

func okEnv(data any, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	return map[string]any{
		"ok":   true,
		"data": data,
		"meta": meta,
	}
}

func errEnv(message string) map[string]any {
	return map[string]any{
		"ok":    false,
		"error": message,
		"data":  nil,
		"meta":  map[string]any{},
	}
}

func noMatchEnv(message string, data any) map[string]any {
	if data == nil {
		data = []any{}
	}
	return okEnv(data, map[string]any{
		"why":     "no_match",
		"message": message,
	})
}
