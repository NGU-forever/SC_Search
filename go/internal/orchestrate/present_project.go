package orchestrate

import (
	"fmt"
	"strconv"
)

// Projection is a primary-specific presentation of evidence.
type Projection struct {
	Headline string
	Facts    []FactBlock
	Draft    string
	Missing  []string
	Warnings []string
}

// EvidenceProjector maps worker evidence into answer_blocks/draft for one domain.
type EvidenceProjector interface {
	Supports(primary string) bool
	Project(st *State) Projection
}

var projectors = []EvidenceProjector{
	missionProjector{},
}

func projectFor(st *State) (Projection, bool) {
	for _, p := range projectors {
		if p.Supports(st.Primary) {
			proj := p.Project(st)
			if len(proj.Facts) > 0 || proj.Draft != "" || len(proj.Missing) > 0 {
				return proj, true
			}
			return Projection{}, false
		}
	}
	return Projection{}, false
}

func evidenceByTool(st *State, tool string) []map[string]any {
	out := []map[string]any{}
	for _, ev := range st.Evidence {
		if asString(ev["tool"]) != tool {
			continue
		}
		out = append(out, asMaps(ev["data"])...)
	}
	return out
}

func formatNum(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		if t == "" || t == "<nil>" {
			return ""
		}
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return formatNum(float64(t))
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	default:
		s := fmt.Sprintf("%v", v)
		if s == "" || s == "<nil>" {
			return ""
		}
		return s
	}
}
