package query

import (
	"regexp"
	"strings"
)

// Canonical star-system keys used by price filters and terminal inference.
var systemCanonical = map[string]string{
	"stanton": "stanton", "斯坦顿": "stanton",
	"pyro": "pyro", "派罗": "pyro",
	"nyx": "nyx", "尼克斯": "nyx",
	"terra": "terra", "terra system": "terra",
}

// POI / station tokens that imply a star system when the terminal name lacks "Stanton"/"Pyro".
var systemPOIKeywords = map[string][]string{
	"stanton": {
		"area 18", "lorville", "new babbage", "orison", "grim hex", "everus harbor",
		"port tressler", "baijini", "hdm-", "hur-l", "cru-l", "arc-l", "mic-l",
		"hurston", "arccorp", "microtech", "crusader", "delamar", "levski",
		"comm-array", "tdd -", "cbd -", "io north",
	},
	"pyro": {
		"checkmate", "ruin station", "orbituary", "bloom", "terminus", "gaslight",
		"patch city", "rat's nest", "rats nest", "starlight service", "paradise cove",
		"rab-", "cluster nbd", "ignis", "vatra", "monox", "pyro i", "pyro iv", "pyro v",
		"pyr1", "pyr3", "pyr6",
	},
	"nyx": {
		"nyx gateway", "delamar", // Delamar historically Nyx; keep weak
	},
}

var parenSystemRE = regexp.MustCompile(`(?i)\((stanton|pyro|nyx|terra)\)`)

// NormalizeSystem returns a lowercase canonical system key, or "" if unknown.
func NormalizeSystem(system string) string {
	s := strings.ToLower(strings.TrimSpace(system))
	if s == "" {
		return ""
	}
	if c, ok := systemCanonical[s]; ok {
		return c
	}
	for alias, canon := range systemCanonical {
		if strings.Contains(s, alias) {
			return canon
		}
	}
	return s
}

// TerminalMatchesSystem reports whether a UEX terminal name belongs to the given system.
// Matches: explicit system substring, trailing "(System)", or known POI keywords.
func TerminalMatchesSystem(terminalName, system string) bool {
	canon := NormalizeSystem(system)
	if canon == "" {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(terminalName))
	if name == "" {
		return false
	}
	if strings.Contains(name, canon) {
		return true
	}
	if m := parenSystemRE.FindStringSubmatch(name); len(m) == 2 {
		return strings.EqualFold(m[1], canon)
	}
	for _, kw := range systemPOIKeywords[canon] {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// InferSystemFromTerminalName guesses star system from a terminal display name.
func InferSystemFromTerminalName(terminalName string) string {
	name := strings.ToLower(strings.TrimSpace(terminalName))
	if name == "" {
		return ""
	}
	if m := parenSystemRE.FindStringSubmatch(name); len(m) == 2 {
		return strings.ToLower(m[1])
	}
	for canon := range systemPOIKeywords {
		if strings.Contains(name, canon) {
			return canon
		}
	}
	for canon, kws := range systemPOIKeywords {
		for _, kw := range kws {
			if strings.Contains(name, kw) {
				return canon
			}
		}
	}
	return ""
}
