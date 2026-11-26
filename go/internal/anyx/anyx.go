// Package anyx provides small, dependency-free conversions from dynamic
// (map[string]any / JSON-decoded) values to concrete Go types.
//
// It is a leaf package: it imports only the standard library and must never
// import another package from this repository.
package anyx

import "strings"

// String returns v when it is a string, and "" otherwise. Values are not
// trimmed and non-string types are not formatted (use fmt.Sprint for that).
func String(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Bool returns v when it is a bool, and false otherwise.
func Bool(v any) bool {
	b, _ := v.(bool)
	return b
}

// Float returns v as a float64 for float64, int64 and int, and 0 otherwise.
func Float(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	default:
		return 0
	}
}

// Int64 returns v as an int64 for int64, int and float64, and 0 otherwise.
// Float values are truncated.
func Int64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}

// Maps converts v to []map[string]any. It accepts []map[string]any and []any
// (non-map elements are dropped). A []any input always yields a non-nil slice;
// any other input yields nil.
func Maps(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

// StringMap returns v when it is a map[string]any, and nil otherwise.
func StringMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// Slice returns v when it is a []any, and nil otherwise.
func Slice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// Strings converts v to []string. It accepts []string as-is and []any by
// keeping string elements only (no trimming, empty strings preserved). Any
// other input yields nil.
func Strings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// First returns the first non-empty value in vals, or "" when there is none.
// Values are not trimmed.
func First(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// FirstField returns the first string field from row for keys whose trimmed
// value is non-empty and not the literal "<nil>". The returned value is
// trimmed.
func FirstField(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := strings.TrimSpace(String(row[k])); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

// Unique drops empty strings and duplicate entries, preserving first-seen
// order. The result is always non-nil.
func Unique(xs []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, x := range xs {
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

// AppendUnique appends s to xs unless s is empty or already present.
func AppendUnique(xs []string, s string) []string {
	if s == "" {
		return xs
	}
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}
