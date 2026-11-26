package query

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func formatIntComma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = s[1:]
	}
	var out []byte
	for i, c := range reverseString(s) {
		if i > 0 && i%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	res := reverseString(string(out))
	if n < 0 {
		return "-" + res
	}
	return res
}

func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func formatGuideLinks(resources []map[string]string, limit int) string {
	lines := []string{}
	for i, r := range resources {
		if i >= limit {
			break
		}
		title := r["title"]
		if title == "" {
			title = "链接"
		}
		url := r["url"]
		if url != "" {
			lines = append(lines, "- "+title+"："+url)
		}
	}
	return strings.Join(lines, "\n")
}

func slimFact(f map[string]any) map[string]any {
	key := anyx.String(f["key"])
	return map[string]any{
		"key": key, "key_zh": labelFactKey(key),
		"value_text": f["value_text"], "value_num": f["value_num"],
		"unit": f["unit"], "source": f["source"], "game_version": f["game_version"],
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func castStringMaps(v any) []map[string]string {
	out := []map[string]string{}
	switch t := v.(type) {
	case []map[string]string:
		return t
	case []map[string]any:
		for _, m := range t {
			row := map[string]string{}
			for k, vv := range m {
				row[k] = anyx.String(vv)
			}
			out = append(out, row)
		}
	case []any:
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				row := map[string]string{}
				for k, vv := range m {
					row[k] = anyx.String(vv)
				}
				out = append(out, row)
			}
		}
	}
	return out
}

func firstKind(entities []map[string]any, kind string) map[string]any {
	for _, e := range entities {
		if anyx.String(e["kind"]) == kind {
			return e
		}
	}
	return nil
}
