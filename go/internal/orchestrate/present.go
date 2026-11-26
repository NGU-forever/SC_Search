package orchestrate

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
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
	blocks := newAnswerBlocks(st)
	facts, hasProj := projectedFacts(st, &blocks)
	if !hasProj {
		facts = append(facts, evidenceFacts(st)...)
	}
	if len(facts) > 8 {
		facts = facts[:8]
	}
	blocks.Facts = facts
	setBlockHeadline(st, &blocks, facts)
	applyBlockOutcome(st, &blocks, facts)
	st.Blocks = blocks
	st.Missing = blocks.Missing
	st.Warnings = blocks.Warnings
}

// newAnswerBlocks seeds the generic block envelope (version, warnings, sources).
func newAnswerBlocks(st *State) AnswerBlocks {
	blocks := AnswerBlocks{
		Version:  asString(st.Meta["as_of"]),
		Warnings: append([]string{}, st.Warnings...),
		Missing:  append([]string{}, st.Missing...),
	}
	if blocks.Version == "" && st.Answer != nil {
		blocks.Version = asString(st.Answer["live_game_version"])
	}
	if anyx.Bool(st.Meta["stale"]) {
		src := joinAny(st.Meta["stale_sources"])
		msg := "本地快照可能过时（stale）"
		if src != "" {
			msg += "，来源: " + src
		}
		msg += "；勿把旧 game_version 当成当前 Live。"
		blocks.Warnings = anyx.AppendUnique(blocks.Warnings, msg)
	}
	if srcs := joinAny(st.Meta["sources"]); srcs != "" {
		for _, s := range strings.Split(srcs, ", ") {
			if s != "" {
				blocks.Sources = anyx.AppendUnique(blocks.Sources, s)
			}
		}
	}
	return blocks
}

// projectedFacts uses a primary projector when one applies, otherwise the
// answer evidence fallback. It also records the projector draft on the state.
func projectedFacts(st *State, blocks *AnswerBlocks) ([]FactBlock, bool) {
	proj, hasProj := projectFor(st)
	facts := []FactBlock{}
	if hasProj {
		facts = append(facts, proj.Facts...)
		for _, m := range proj.Missing {
			blocks.Missing = anyx.AppendUnique(blocks.Missing, m)
		}
		for _, w := range proj.Warnings {
			blocks.Warnings = anyx.AppendUnique(blocks.Warnings, w)
		}
		if proj.Headline != "" {
			blocks.Headline = proj.Headline
		}
		st.projDraft = proj.Draft
		return facts, true
	}
	if st.Answer == nil {
		return facts, false
	}
	ev, ok := st.Answer["evidence"].(map[string]any)
	if !ok {
		return facts, false
	}
	for _, f := range anyx.Maps(ev["facts"]) {
		label := asString(f["key"])
		val := factValue(f)
		if label != "" && val != "" {
			facts = append(facts, FactBlock{Label: label, Value: val})
		}
	}
	for _, m := range asStringSlice(ev["missing"]) {
		blocks.Missing = anyx.AppendUnique(blocks.Missing, m)
	}
	return facts, false
}

// evidenceFacts projects facts from raw worker evidence when no projector applies.
func evidenceFacts(st *State) []FactBlock {
	facts := []FactBlock{}
	for _, ev := range st.Evidence {
		tool := asString(ev["tool"])
		switch tool {
		case "sc_resolve_name":
			rows := anyx.Maps(ev["data"])
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
				facts = append(facts, factRows(data["facts"])...)
			}
		case "sc_best_prices":
			facts = append(facts, priceFacts(ev["data"])...)
		}
	}
	return facts
}

func factRows(raw any) []FactBlock {
	facts := []FactBlock{}
	for _, f := range anyx.Maps(raw) {
		label := asString(f["key"])
		val := factValue(f)
		if label != "" && val != "" {
			facts = append(facts, FactBlock{Label: label, Value: val})
		}
	}
	return facts
}

func priceFacts(raw any) []FactBlock {
	facts := []FactBlock{}
	rows := anyx.Maps(raw)
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
	return facts
}

// setBlockHeadline falls back from facts, to primary, to a generic title.
func setBlockHeadline(st *State, blocks *AnswerBlocks, facts []FactBlock) {
	if blocks.Headline != "" {
		return
	}
	if len(facts) > 0 {
		blocks.Headline = facts[0].Label + ": " + facts[0].Value
	} else if st.Primary != "" {
		blocks.Headline = "意图 " + st.Primary
	} else {
		blocks.Headline = "本地查询结果"
	}
}

// applyBlockOutcome appends no-match/failure warnings and updates the status.
func applyBlockOutcome(st *State, blocks *AnswerBlocks, facts []FactBlock) {
	emptyEvidence := anyx.Bool(st.Worker["evidence_empty"])
	if emptyEvidence && len(facts) == 0 {
		blocks.Missing = anyx.AppendUnique(blocks.Missing, "no_match")
		blocks.Warnings = anyx.AppendUnique(blocks.Warnings, "本地未命中可用证据，勿编造库外数字。")
		if st.Status == "" || st.Status == StatusCompleted {
			st.Status = StatusPartial
		}
	}
	if len(anyx.Maps(st.Worker["failed_calls"])) > 0 && len(facts) > 0 {
		st.Status = StatusPartial
		blocks.Warnings = anyx.AppendUnique(blocks.Warnings, "部分工具调用失败，结论仅基于可用证据。")
	}
	for _, m := range blocks.Missing {
		if m != "" && m != "use_sc_route" {
			blocks.Warnings = anyx.AppendUnique(blocks.Warnings, "本地未收录: "+m)
		}
	}
}

func buildDraft(st *State) {
	if st.projDraft != "" {
		st.Draft = st.projDraft
		if st.Blocks.Version != "" {
			st.Draft += "\n版本对照 Live " + st.Blocks.Version + "。"
		}
		if len(st.Blocks.Warnings) > 0 {
			st.Draft += "\n注意：" + strings.Join(st.Blocks.Warnings, "；")
		}
		return
	}
	if st.Answer != nil {
		if d := asString(st.Answer["answer_draft"]); d != "" && !anyx.Bool(st.Answer["route_required"]) {
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
		out = anyx.AppendUnique(out, "确认后打开 "+site)
		out = anyx.AppendUnique(out, "只要本地结论")
	}
	if anyx.Bool(st.Meta["stale"]) {
		out = anyx.AppendUnique(out, "同步数据")
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

// asStringSlice trims entries and drops empties, unlike anyx.Strings, so it
// stays package-local.
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

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
