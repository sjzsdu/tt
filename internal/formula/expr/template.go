package expr

import (
	"regexp"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_-]*(?:\.[a-zA-Z_][a-zA-Z0-9_-]*)*)\s*\}\}`)

type Lookup func(path string) (string, bool)

func RenderTemplate(input string, lookup Lookup) string {
	return templatePattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		if value, ok := lookup(parts[1]); ok {
			return value
		}
		return match
	})
}
