package query

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerLiveWeb(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	res.Draft = "这类信息需要官网或 Spectrum；本地库不覆盖实时 Patch。需要的话再开浏览器。"
	res.FollowUps = []string{"打开 RSI 查 Patch Notes"}
	return res, nil
}

func answerLiveLocal(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	if c.Live == "" {
		res.Draft = "本地库尚未记录 Live 版本；请先 sync scmdb。"
		res.Missing = append(res.Missing, "live_version")
	} else {
		res.Draft = fmt.Sprintf("本地对照的当前 Live 版本为 %s。", c.Live)
		res.FollowUps = []string{"看各源同步时间", "打开 RSI 查 Patch Notes"}
	}
	return res, nil
}

func answerDataSync(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	rows, err := DataFreshness(c.DB, "")
	if err != nil {
		return res, err
	}
	annotated, meta := AnnotateFreshnessRows(c.DB, rows)
	ev["meta"] = meta
	if len(annotated) == 0 {
		res.Draft = "本地没有 ingest 同步记录；请先 sync。"
		res.Missing = append(res.Missing, "ingest_runs")
		return res, nil
	}
	bits := []string{}
	for i, r := range annotated {
		if i >= 5 {
			break
		}
		src := anyx.String(r["source"])
		fin := anyx.String(r["finished_at"])
		if src == "" || fin == "" {
			continue
		}
		bits = append(bits, fmt.Sprintf("%s@%s", src, fin))
	}
	ver := c.Live
	if ver == "" {
		ver = anyx.String(meta["as_of"])
	}
	if len(bits) == 0 {
		res.Draft = "本地有 ingest 记录但缺少完成时间。"
	} else if ver != "" {
		res.Draft = fmt.Sprintf("本地数据最近同步（对照 Live %s）：%s。", ver, strings.Join(bits, "；"))
	} else {
		res.Draft = fmt.Sprintf("本地数据最近同步：%s。", strings.Join(bits, "；"))
	}
	res.FollowUps = []string{"只看 UEX", "只看 SCMDB"}
	return res, nil
}
