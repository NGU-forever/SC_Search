package orchestrate

import (
	"fmt"
	"strings"
)

func present(st *State) Phase {
	buildBlocks(st)
	buildDraft(st)
	st.Browser = browserGateFor(st.Primary, st.Path, searchTermFromState(st), st.Answer)
	st.FollowUps = followUps(st)

	if st.AskError != nil && st.Status == StatusFailed {
		return PhaseFailed
	}
	if st.Browser.Needed && st.Browser.WaitConfirm {
		st.Status = StatusAwaitingUser
		return PhaseAwaitUser
	}
	if st.Status == "" {
		st.Status = StatusCompleted
	}
	return PhaseDone
}

func buildBlocks(st *State) {
	blocks := AnswerBlocks{
		Version:  asString(st.Meta["as_of"]),
		Warnings: append([]string{}, st.Warnings...),
		Missing:  append([]string{}, st.Missing...),
	}
	if blocks.Version == "" && st.Answer != nil {
		blocks.Version = asString(st.Answer["live_game_version"])
	}
	if asBool(st.Meta["stale"]) {
		src := joinAny(st.Meta["stale_sources"])
		msg := "本地快照可能过时（stale）"
		if src != "" {
			msg += "，来源: " + src
		}
		msg += "；勿把旧 game_version 当成当前 Live。"
		blocks.Warnings = appendUnique(blocks.Warnings, msg)
	}
	if srcs := joinAny(st.Meta["sources"]); srcs != "" {
		for _, s := range strings.Split(srcs, ", ") {
			if s != "" {
				blocks.Sources = appendUnique(blocks.Sources, s)
			}
		}
	}

	facts := []FactBlock{}
	if st.Answer != nil {
		if ev, ok := st.Answer["evidence"].(map[string]any); ok {
			for _, f := range asMaps(ev["facts"]) {
				label := asString(f["key"])
				val := factValue(f)
				if label != "" && val != "" {
					facts = append(facts, FactBlock{Label: label, Value: val})
				}
			}
			for _, m := range asStringSlice(ev["missing"]) {
				blocks.Missing = appendUnique(blocks.Missing, m)
			}
		}
	}
	for _, ev := range st.Evidence {
		tool := asString(ev["tool"])
		switch tool {
		case "sc_resolve_name":
			rows := asMaps(ev["data"])
			if len(rows) > 0 {
				name := asString(rows[0]["canonical_name"])
				kind := asString(rows[0]["kind"])
				if name != "" {
					facts = append(facts, FactBlock{Label: "实体", Value: name})
				}
				if kind != "" {
					facts = append(facts, FactBlock{Label: "类型", Value: kind})
				}
			}
		case "sc_entity_facts":
			if data, ok := ev["data"].(map[string]any); ok {
				for _, f := range asMaps(data["facts"]) {
					label := asString(f["key"])
					val := factValue(f)
					if label != "" && val != "" {
						facts = append(facts, FactBlock{Label: label, Value: val})
					}
				}
			}
		case "sc_best_prices":
			rows := asMaps(ev["data"])
			for i, row := range rows {
				if i >= 3 {
					break
				}
				term := asString(row["terminal_name"])
				if term == "" {
					term = asString(row["terminal"])
				}
				price := fmt.Sprintf("%v", row["price"])
				if price == "<nil>" {
					price = fmt.Sprintf("%v", row["price_buy"])
					if price == "<nil>" {
						price = fmt.Sprintf("%v", row["price_sell"])
					}
				}
				if term != "" && price != "<nil>" {
					facts = append(facts, FactBlock{Label: "报价", Value: term + " " + price})
				}
			}
		}
	}
	if len(facts) > 8 {
		facts = facts[:8]
	}
	blocks.Facts = facts
	if len(facts) > 0 {
		blocks.Headline = facts[0].Label + ": " + facts[0].Value
	} else if st.Primary != "" {
		blocks.Headline = "意图 " + st.Primary
	} else {
		blocks.Headline = "本地查询结果"
	}
	emptyEvidence := asBool(st.Worker["evidence_empty"])
	if emptyEvidence && len(facts) == 0 {
		blocks.Missing = appendUnique(blocks.Missing, "no_match")
		blocks.Warnings = appendUnique(blocks.Warnings, "本地未命中可用证据，勿编造库外数字。")
		if st.Status == "" || st.Status == StatusCompleted {
			st.Status = StatusPartial
		}
	}
	if len(asMaps(st.Worker["failed_calls"])) > 0 && len(facts) > 0 {
		st.Status = StatusPartial
		blocks.Warnings = appendUnique(blocks.Warnings, "部分工具调用失败，结论仅基于可用证据。")
	}
	for _, m := range blocks.Missing {
		if m != "" && m != "use_sc_route" {
			blocks.Warnings = appendUnique(blocks.Warnings, "本地未收录: "+m)
		}
	}
	st.Blocks = blocks
	st.Missing = blocks.Missing
	st.Warnings = blocks.Warnings
}

func buildDraft(st *State) {
	if st.Answer != nil {
		if d := asString(st.Answer["answer_draft"]); d != "" && !asBool(st.Answer["route_required"]) {
			st.Draft = d
			if len(st.Blocks.Warnings) > 0 {
				st.Draft = st.Draft + "\n\n注意：" + strings.Join(st.Blocks.Warnings, "；")
			}
			return
		}
	}
	var b strings.Builder
	if st.Blocks.Headline != "" {
		b.WriteString(st.Blocks.Headline)
		b.WriteString("。")
	}
	if len(st.Blocks.Facts) > 0 {
		b.WriteString("\n")
		for _, f := range st.Blocks.Facts {
			b.WriteString("- ")
			b.WriteString(f.Label)
			b.WriteString(": ")
			b.WriteString(f.Value)
			b.WriteString("\n")
		}
	} else if containsStr(st.Blocks.Missing, "no_match") {
		b.WriteString("本地知识库未命中可用证据。")
	}
	if st.Blocks.Version != "" {
		b.WriteString("\n版本对照 Live ")
		b.WriteString(st.Blocks.Version)
		b.WriteString("。")
	}
	if len(st.Blocks.Warnings) > 0 {
		b.WriteString("\n注意：")
		b.WriteString(strings.Join(st.Blocks.Warnings, "；"))
	}
	if st.Primary == "loadout" {
		b.WriteString("\n改装：以下为本地静态/主流对照脚手架；需要 Erkul 计算器时请先确认后再打开。")
	}
	st.Draft = strings.TrimSpace(b.String())
	if st.Draft == "" {
		st.Draft = "本地查询完成，但没有可整理的字段。"
	}
}

func followUps(st *State) []string {
	out := []string{}
	if st.Answer != nil {
		if raw, ok := st.Answer["follow_ups"].([]string); ok {
			out = append(out, raw...)
		} else if raw, ok := st.Answer["follow_ups"].([]any); ok {
			for _, v := range raw {
				if s := asString(v); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	if st.Browser.Needed && st.Browser.WaitConfirm {
		site := st.Browser.Site
		if site == "" {
			site = "外链工具"
		}
		out = appendUnique(out, "确认后打开 "+site)
		out = appendUnique(out, "只要本地结论")
	}
	if asBool(st.Meta["stale"]) {
		out = appendUnique(out, "同步数据")
	}
	return out
}

func factValue(f map[string]any) string {
	if s := asString(f["value_text"]); s != "" {
		return s
	}
	if v, ok := f["value_num"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := f["value"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := asString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func joinAny(v any) string {
	switch t := v.(type) {
	case []string:
		return strings.Join(t, ", ")
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s := asString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return asString(v)
	}
}

func appendUnique(xs []string, s string) []string {
	if s == "" || containsStr(xs, s) {
		return xs
	}
	return append(xs, s)
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
