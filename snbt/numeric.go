package snbt

import (
    "strconv"
    "strings"
)

// Short preserves an SNBT short value like "123s".
type Short struct {
    Sign   int
    Digits string
    Suffix byte // 's' or 'S'
}

func (s Short) SNBT() string {
    if s.Suffix == 0 { s.Suffix = 's' }
    if s.Sign < 0 { return "-" + s.Digits + string(s.Suffix) }
    return s.Digits + string(s.Suffix)
}

// Long preserves an SNBT long value like "123l".
type Long struct {
    Sign   int
    Digits string
    Suffix byte // 'l' or 'L'
}

func (l Long) SNBT() string {
    if l.Suffix == 0 { l.Suffix = 'l' }
    if l.Sign < 0 { return "-" + l.Digits + string(l.Suffix) }
    return l.Digits + string(l.Suffix)
}

// FloatNum preserves an SNBT float value like "1.5f".
type FloatNum struct {
    Sign   int
    Int    string
    Frac   string
    Suffix byte // 'f' or 'F'
}

func (f FloatNum) Float() float64 {
    var b strings.Builder
    if f.Sign < 0 { b.WriteByte('-') }
    b.WriteString(f.Int)
    if f.Frac != "" { b.WriteByte('.'); b.WriteString(f.Frac) }
    v, _ := strconv.ParseFloat(b.String(), 64)
    return v
}

func (f FloatNum) SNBT() string {
    if f.Suffix == 0 { f.Suffix = 'f' }
    var b strings.Builder
    if f.Sign < 0 { b.WriteByte('-') }
    b.WriteString(f.Int)
    if f.Frac != "" { b.WriteByte('.'); b.WriteString(f.Frac) }
    b.WriteByte(f.Suffix)
    return b.String()
}

