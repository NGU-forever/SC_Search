package worker

import (
	"regexp"
	"strings"
)

var placeholder = regexp.MustCompile(`^\$([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)$`)

func canonicalFromResolve(data any) string {
	switch t := data.(type) {
	case []any:
		if len(t) == 0 {
			return ""
		}
		if row, ok := t[0].(map[string]any); ok {
			return firstName(row)
		}
	case []map[string]any:
		if len(t) == 0 {
			return ""
		}
		return firstName(t[0])
	case map[string]any:
		return firstName(t)
	}
	return ""
}

func firstName(row map[string]any) string {
	for _, key := range []string{"canonical_name", "name"} {
		if s, ok := row[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func bindingValue(callID, field string, results map[string]map[string]any) any {
	row := results[callID]
	if row == nil {
		return nil
	}
	if field == "canonical" {
		return canonicalFromResolve(row["data"])
	}
	if data, ok := row["data"].(map[string]any); ok {
		if v, ok := data[field]; ok {
			return v
		}
	}
	return nil
}

func bindArgs(args map[string]any, results map[string]map[string]any) map[string]any {
	out := map[string]any{}
	for key, val := range args {
		if s, ok := val.(string); ok {
			if m := placeholder.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
				bound := bindingValue(m[1], m[2], results)
				if bound == nil {
					out[key] = val
				} else {
					out[key] = bound
				}
				continue
			}
		}
		out[key] = val
	}
	return out
}
