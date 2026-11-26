package query

import "strings"

// chitchatExact matches whole normalized messages (no SC content at all).
var chitchatExact = map[string]struct{}{
	"hi": {}, "hello": {}, "hey": {}, "ok": {}, "yo": {}, "thanks": {}, "thx": {},
}

// chitchatPhrases are conversational markers (greeting, thanks, mood, opinion
// about the game with no concrete object). Only consulted when the question
// also has no resolved entity and no fact intent; see IsChitchat.
var chitchatPhrases = []string{
	"你好", "您好", "嗨", "早上好", "下午好", "晚上好", "晚安", "在吗", "在不在",
	"谢谢", "感谢", "多谢", "辛苦了", "再见", "拜拜", "哈哈", "呵呵", "心情",
	"你是谁", "你叫什么", "你会什么", "你能做什么", "无聊", "开心", "累了",
	"好玩吗", "有意思吗", "好看吗", "好不好玩", "你觉得",
}

// IsChitchat reports whether a message looks like greeting, thanks, small talk
// or a subjective remark rather than a Star Citizen fact question. It is a
// conservative lexical check; callers must also require that no entity matched.
func IsChitchat(question string) bool {
	norm := normalizeChitchat(question)
	if norm == "" {
		return false
	}
	if _, ok := chitchatExact[norm]; ok {
		return true
	}
	for _, p := range chitchatPhrases {
		if strings.Contains(norm, p) {
			return true
		}
	}
	return false
}

func normalizeChitchat(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			continue
		case r == '，' || r == '。' || r == '！' || r == '？' || r == '、' || r == '~' || r == '～':
			continue
		case r == ',' || r == '.' || r == '!' || r == '?' || r == '~' || r == '…':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
