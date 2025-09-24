package snbt

import (
	"io"
)

//go:generate peg -switch -inline -strict -output snbt_parser.go snbt.peg

// Value is the generic SNBT value type.
// - map[string]any for compounds
// - []any for lists
// - string for strings
// - float64 / int64 for numbers (initial)
// - bool for booleans
type Value = any

// Decode parses SNBT from an io.Reader into a generic Value using the generated parser.
func Decode(r io.Reader) (Value, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var p SNBT
	p.Buffer = string(input)
	if err := p.Init(); err != nil {
		return nil, err
	}
	if err := p.Parse(); err != nil {
		return nil, err
	}
	p.Execute()
	if len(p.stack) == 0 {
		return nil, nil
	}
	return p.stack[len(p.stack)-1], nil
}
