package snbt

import (
	"strconv"
	"strings"
)

// Builder is embedded into the generated PEG parser and accumulates results.
type Builder struct {
	stack []any
	keys  []string
}

// helper stack ops
func (b *Builder) push(v any) { b.stack = append(b.stack, v) }
func (b *Builder) pop() any {
	if len(b.stack) == 0 {
		return nil
	}
	v := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	return v
}
func (b *Builder) peek() any {
	if len(b.stack) == 0 {
		return nil
	}
	return b.stack[len(b.stack)-1]
}

// Public helpers used from grammar actions
func (b *Builder) BeginCompound()  { b.push(map[string]any{}) }
func (b *Builder) SetKey(k string) { b.keys = append(b.keys, k) }
func (b *Builder) PairSet() {
	v := b.pop()
	top := b.peek()
	if m, ok := top.(map[string]any); ok {
		if n := len(b.keys); n > 0 {
			key := b.keys[n-1]
			b.keys = b.keys[:n-1]
			m[key] = v
		}
	}
}

func (b *Builder) BeginList() { b.push([]any{}) }
func (b *Builder) ListAppend() {
	v := b.pop()
	top := b.peek()
	if l, ok := top.([]any); ok {
		l = append(l, v)
		// store back
		b.stack[len(b.stack)-1] = l
	}
}

func (b *Builder) PushString(s string) {
	// s is the inner content (no quotes). Unescape via strconv.Unquote.
	// Build a quoted string and unquote; fall back to raw on error.
	if unq, err := strconv.Unquote("\"" + s + "\""); err == nil {
		b.push(unq)
		return
	}
	b.push(s)
}

func (b *Builder) PushNumber(s string) {
	if containsDotOrExp(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			b.push(f)
			return
		}
	} else {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			b.push(i)
			return
		}
	}
	// Fallback: push as string
	b.push(s)
}

func (b *Builder) PushBool(v bool) { b.push(v) }

// PushDecimal parses a decimal with 'd' suffix preserving parts.
func (b *Builder) PushDecimal(s string) {
	if s == "" {
		return
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	// strip suffix (last rune)
	if n := len(s); n > 0 {
		s = s[:n-1]
	}
	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
	}
	b.push(Decimal{Sign: sign, Int: intPart, Frac: fracPart, Suffix: 'd'})
}

// PushShort parses a short with 's' suffix.
func (b *Builder) PushShort(s string) {
	if s == "" {
		return
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	// strip suffix
	digits := s[:len(s)-1]
	b.push(Short{Sign: sign, Digits: digits, Suffix: 's'})
}

// PushLong parses a long with 'l' suffix.
func (b *Builder) PushLong(s string) {
	if s == "" {
		return
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	digits := s[:len(s)-1]
	b.push(Long{Sign: sign, Digits: digits, Suffix: 'l'})
}

// PushFloat parses a float with 'f' suffix preserving parts.
func (b *Builder) PushFloat(s string) {
	if s == "" {
		return
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}
	// strip suffix
	s = s[:len(s)-1]
	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
	}
	b.push(FloatNum{Sign: sign, Int: intPart, Frac: fracPart, Suffix: 'f'})
}

func containsDotOrExp(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', 'e', 'E':
			return true
		}
	}
	return false
}
