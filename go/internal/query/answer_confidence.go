package query

func recomputeConfidence(intent, planConf string, hadScored, hasEntity bool, facts, prices, docs []map[string]any, missing []string) string {
	_ = missing
	switch intent {
	case "facts":
		if hadScored && len(facts) > 0 {
			return "high"
		}
		if hasEntity && len(facts) > 0 {
			return "medium"
		}
		return "low"
	case "prices":
		if len(prices) > 0 {
			return "high"
		}
		return "low"
	case "mission_list":
		if len(missing) == 0 {
			return "high"
		}
		return "low"
	case "docs":
		if len(docs) > 0 {
			return "medium"
		}
		return "low"
	case "live_web":
		return planConf
	default:
		if hasEntity {
			return planConf
		}
		return "low"
	}
}

func needsSoftVerify(intent, conf string, hadScored bool, facts, prices, docs []map[string]any) bool {
	if conf != "low" && conf != "medium" {
		return false
	}
	switch intent {
	case "facts":
		return !hadScored
	case "prices":
		return len(prices) == 0
	case "docs":
		return len(docs) == 0
	default:
		return false
	}
}
