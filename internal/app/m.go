package app

// Mis a map[string]any with some extra methods
type M map[string]any

// Has returns true if m has a value for key.
func (m M) Has(key string) bool {
	_, ok := m[key]
	return ok
}

// GetString returns the value of key as a string, or ""
func (m M) GetString(key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	// XXX: do we ever need a non-"" fallback?
	return ""
}

// GetAnys returns the value for key as a slice of any.
func (m M) GetAnys(key string) []any {
	v, _ := m[key].([]any)
	return v
}

// GetStrings returns the value for key as a string slice. Non-strings are skipped.
func (m M) GetStrings(key string) []string {
	v := m.GetAnys(key)
	ss := make([]string, 0, len(v))
	for _, x := range v {
		if s, ok := x.(string); ok {
			ss = append(ss, s)
		}
	}
	return ss
}
