package orchestrate

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func missionTitle(row map[string]any) string {
	if t := anyx.FirstField(row, "title"); t != "" {
		return t
	}
	return anyx.FirstField(row, "canonical_name")
}

func cleanMissionTitle(title string) string {
	if title == "" {
		return ""
	}
	// Drop unfilled SCMDB placeholders like [CLAIM] / [SHIP] / #[CLAIM].
	out := placeholderRE.ReplaceAllString(title, " ")
	out = strings.ReplaceAll(out, " : ", " ")
	out = strings.ReplaceAll(out, ": ", " ")
	out = strings.Join(strings.Fields(out), " ")
	out = strings.TrimSpace(strings.Trim(out, ":-–— "))
	if out == "" {
		return title
	}
	return out
}

var placeholderRE = regexp.MustCompile(`#?\[[A-Za-z_]+\]`)

func feeHeadline(rows []map[string]any) string {
	return numericRangeHeadline(rows, "cost_uec", "手续费")
}

func rewardHeadline(rows []map[string]any) string {
	return numericRangeHeadline(rows, "reward_uec", "奖励")
}

func numericRangeHeadline(rows []map[string]any, key, label string) string {
	var nums []int64
	for _, row := range rows {
		v := formatNum(row[key])
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return ""
	}
	min, max := nums[0], nums[0]
	for _, n := range nums[1:] {
		if n < min {
			min = n
		}
		if n > max {
			max = n
		}
	}
	if min == max {
		return label + ": " + strconv.FormatInt(min, 10) + " aUEC"
	}
	return label + ": " + strconv.FormatInt(min, 10) + "–" + strconv.FormatInt(max, 10) + " aUEC"
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
