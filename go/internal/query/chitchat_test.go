package query

import "testing"

func TestIsChitchat(t *testing.T) {
	yes := []string{"你好", "你好！", "Hi", "谢谢你，今天心情不错", "星际公民好玩吗", "在吗", "再见"}
	no := []string{"卡拉克货舱多大", "黄金哪里卖最贵", "Bit Zeros 任务手续费", "流星改装方案", "这个任务怎么接"}
	for _, q := range yes {
		if !IsChitchat(q) {
			t.Errorf("IsChitchat(%q)=false, want true", q)
		}
	}
	for _, q := range no {
		if IsChitchat(q) {
			t.Errorf("IsChitchat(%q)=true, want false", q)
		}
	}
	if IsChitchat("   ") {
		t.Error("blank input must not be chitchat")
	}
}
