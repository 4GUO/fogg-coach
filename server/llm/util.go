package llm

import (
	"encoding/json"
	"regexp"
	"strings"
)

var jsonFence = regexp.MustCompile("(?s)^```(?:json)?\\s*|\\s*```$")

// jsonUnmarshalLoose 容忍 ```json 围栏与前后杂文
func jsonUnmarshalLoose(s string, v any) error {
	s = strings.TrimSpace(jsonFence.ReplaceAllString(strings.TrimSpace(s), ""))
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	return json.Unmarshal([]byte(s), v)
}

func mergeUnique(base, add []string) []string {
	seen := map[string]bool{}
	for _, b := range base {
		seen[b] = true
	}
	for _, a := range add {
		if !seen[a] {
			base = append(base, a)
			seen[a] = true
		}
	}
	return base
}
