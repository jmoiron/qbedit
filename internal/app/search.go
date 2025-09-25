package app

import "strings"

// stripCodes removes Minecraft color/format codes (eg, &a, §b, &r) from a string.
// It preserves all other characters and does not alter case.
func stripCodes(s string) string {
	if s == "" {
		return s
	}
	if !strings.ContainsAny(s, "&§") {
		return s
	}
	b := make([]rune, 0, len(s))
	skip := false
	for _, r := range s {
		if skip {
			skip = false
			continue
		}
		if r == '&' || r == '§' {
			skip = true
			continue
		}
		b = append(b, r)
	}
	return string(b)
}

// matchQuest reports whether all query terms appear as substrings in any of the
// quest's text fields (title, subtitle, description, or GetTitle fallback).
// Terms should be pre-split; case-insensitive mode lowercases the fields.
func matchQuest(qs *Quest, terms []string, caseSensitive bool) bool {
	if len(terms) == 0 {
		return true
	}
	t1 := stripCodes(qs.Title)
	t2 := stripCodes(qs.Subtitle)
	t3 := stripCodes(qs.Description)
	t4 := stripCodes(qs.GetTitle())
	if !caseSensitive {
		t1 = strings.ToLower(t1)
		t2 = strings.ToLower(t2)
		t3 = strings.ToLower(t3)
		t4 = strings.ToLower(t4)
	}
	for _, term := range terms {
		if !(strings.Contains(t1, term) || strings.Contains(t2, term) || strings.Contains(t3, term) || strings.Contains(t4, term)) {
			return false
		}
	}
	return true
}
