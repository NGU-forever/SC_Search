package router

import (
	"regexp"
	"strings"
	"unicode"
)

var metaRE = regexp.MustCompile(`(?i)^(开荒|刷钱|赚钱|怎么赚|如何赚|萌新|新手|怎么玩|如何玩|第一桶金|推荐玩法|有什么玩|干什么|earn|money|grind|newbie|starter)$`)

func isMetaKeyword(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return false
	}
	if metaRE.MatchString(s) {
		return true
	}
	if len([]rune(s)) <= 6 && isAllHan(s) {
		for _, tok := range []string{"赚", "开荒", "萌新", "新手", "刷钱"} {
			if strings.Contains(s, tok) {
				return true
			}
		}
	}
	return false
}

func isAllHan(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return len(s) > 0
}

func docsQuery(slotVal, question string) string {
	q := strings.TrimSpace(question)
	s := strings.TrimSpace(slotVal)
	if s == "" || isMetaKeyword(s) {
		if q != "" {
			return q
		}
		return s
	}
	return s
}

func missionRankKeyword(candidates ...string) string {
	for _, c := range candidates {
		s := strings.TrimSpace(c)
		if s == "" || isMetaKeyword(s) {
			continue
		}
		return s
	}
	return ""
}
