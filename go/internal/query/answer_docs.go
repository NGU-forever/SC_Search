package query

import "github.com/frankxiong/sc-search/internal/anyx"

func answerDocs(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	rows, err := SearchDocs(c.DB, c.Question, c.DocLimit)
	if err != nil {
		return res, err
	}
	docs := []map[string]any{}
	for _, r := range rows {
		snip := anyx.String(r["snippet"])
		if snip == "" {
			snip = anyx.String(r["body"])
		}
		if len(snip) > 240 {
			snip = snip[:240]
		}
		docs = append(docs, map[string]any{"title": r["title"], "snippet": snip})
	}
	ev["docs"] = docs
	if len(rows) > 0 {
		snip := anyx.String(docs[0]["snippet"])
		if snip != "" {
			if len(snip) > 120 {
				snip = snip[:120]
			}
			res.Draft = "资料摘要：" + snip + "…"
		} else {
			res.Draft = "找到相关文档。"
		}
		res.Detail = true
		res.FollowUps = []string{"展开更多 Wiki 摘要"}
	} else {
		res.Missing = append(res.Missing, "docs")
		res.Draft = "本地 Wiki 摘要未命中。"
	}
	return res, nil
}
