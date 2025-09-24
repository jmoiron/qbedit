package snbt

import (
    "errors"
    "fmt"
    "io"
    "reflect"
    "sort"
    "strconv"
    "unicode/utf8"
)

// SelfEncoder can render itself to SNBT.
type SelfEncoder interface {
    SNBT() string
}

// Encode encodes a generic Value back to SNBT bytes.
// Supported types:
// - map[string]any (compound)
// - []any (list)
// - string, bool, int, int64, float64 and numeric aliases
func Encode(w io.Writer, v Value) error { return encodeValue(w, v) }

func encodeValue(w io.Writer, v any) error {
	switch x := v.(type) {
	case nil:
		return errors.New("snbt: cannot encode nil value")
	case map[string]any:
		return encodeCompound(w, x)
	case []any:
		return encodeList(w, x)
	case string:
		encodeString(w, x)
		return nil
	case bool:
		if x {
			io.WriteString(w, "true")
		} else {
			io.WriteString(w, "false")
		}
		return nil
	case int:
		io.WriteString(w, strconv.FormatInt(int64(x), 10))
		return nil
	case int64:
		io.WriteString(w, strconv.FormatInt(x, 10))
		return nil
	case float32:
		encodeFloat(w, float64(x))
		return nil
	case float64:
		encodeFloat(w, x)
		return nil
    default:
        if se, ok := v.(SelfEncoder); ok {
            io.WriteString(w, se.SNBT())
            return nil
        }
        rv := reflect.ValueOf(v)
        switch rv.Kind() {
        case reflect.Int8, reflect.Int16, reflect.Int32:
            io.WriteString(w, strconv.FormatInt(rv.Int(), 10))
            return nil
        case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
			io.WriteString(w, strconv.FormatUint(rv.Uint(), 10))
			return nil
		case reflect.Float32, reflect.Float64:
			encodeFloat(w, rv.Convert(reflect.TypeOf(float64(0))).Float())
			return nil
		}
	}
	return fmt.Errorf("snbt: unsupported type %T", v)
}

func encodeCompound(w io.Writer, m map[string]any) error {
	io.WriteString(w, "{")
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			io.WriteString(w, ", ")
		} else if len(keys) > 0 {
			io.WriteString(w, " ")
		}
		encodeKey(w, k)
		io.WriteString(w, ": ")
		if err := encodeValue(w, m[k]); err != nil {
			return err
		}
	}
	if len(m) > 0 {
		io.WriteString(w, " ")
	}
	io.WriteString(w, "}")
	return nil
}

func encodeList(w io.Writer, l []any) error {
	io.WriteString(w, "[")
	for i, it := range l {
		if i > 0 {
			io.WriteString(w, ", ")
		} else if len(l) > 0 {
			io.WriteString(w, " ")
		}
		if err := encodeValue(w, it); err != nil {
			return err
		}
	}
	if len(l) > 0 {
		io.WriteString(w, " ")
	}
	io.WriteString(w, "]")
	return nil
}

func encodeKey(w io.Writer, k string) {
	if isIdent(k) {
		io.WriteString(w, k)
		return
	}
	encodeString(w, k)
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(s)
	if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_') {
		return false
	}
	for _, r := range s[size:] {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func encodeString(w io.Writer, s string) {
	io.WriteString(w, "\"")
	for _, r := range s {
		switch r {
		case '\\':
			io.WriteString(w, "\\\\")
		case '"':
			io.WriteString(w, "\\\"")
		case '\n':
			io.WriteString(w, "\\n")
		case '\r':
			io.WriteString(w, "\\r")
		case '\t':
			io.WriteString(w, "\\t")
		default:
			if r < 0x20 {
				io.WriteString(w, "\\u")
				hex := strconv.FormatInt(int64(r), 16)
				for i := 0; i < 4-len(hex); i++ {
					io.WriteString(w, "0")
				}
				io.WriteString(w, hex)
			} else {
				io.WriteString(w, string(r))
			}
		}
	}
	io.WriteString(w, "\"")
}

func encodeFloat(w io.Writer, f float64) {
	// Use 'g' for compact form, but ensure a decimal point exists
	s := strconv.FormatFloat(f, 'g', -1, 64)
	hasDot := false
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' || s[i] == 'E' {
			hasDot = true
			break
		}
	}
	if !hasDot {
		s = s + ".0"
	}
	io.WriteString(w, s)
}
