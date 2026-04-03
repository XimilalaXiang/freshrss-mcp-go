package textutil

import (
	"html"
	"regexp"
	"strings"
)

var tagRe = regexp.MustCompile(`(?s)<[^>]+>`)

// StripHTML removes tags; collapses whitespace for compact MCP output.
func StripHTML(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// TruncateAtWord cuts s to max runes at a word boundary when possible.
func TruncateAtWord(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		if len(r) > max {
			return string(r[:max])
		}
		return s
	}
	cut := string(r[:max-3])
	if i := strings.LastIndex(cut, " "); i > max/4 {
		return strings.TrimSpace(cut[:i]) + "..."
	}
	return cut + "..."
}
