package snbt

import (
	"strconv"
	"strings"
)

// Decimal preserves SNBT decimal with suffix (e.g., -0.75d).
// It keeps the integer and fractional parts for round-tripping.
type Decimal struct {
	Sign   int    // -1 or +1
	Int    string // digits left of '.'
	Frac   string // digits right of '.', may be empty
	Suffix byte   // 'd' or 'D'
}

func (d Decimal) Float() float64 {
	var b strings.Builder
	if d.Sign < 0 {
		b.WriteByte('-')
	}
	b.WriteString(d.Int)
	if d.Frac != "" {
		b.WriteByte('.')
		b.WriteString(d.Frac)
	}
	f, _ := strconv.ParseFloat(b.String(), 64)
	return f
}

func (d Decimal) SNBT() string {
	var b strings.Builder
	if d.Sign < 0 {
		b.WriteByte('-')
	}
	b.WriteString(d.Int)
	if d.Frac != "" {
		b.WriteByte('.')
		b.WriteString(d.Frac)
	}
	if d.Suffix == 0 {
		b.WriteByte('d')
	} else {
		b.WriteByte(d.Suffix)
	}
	return b.String()
}
