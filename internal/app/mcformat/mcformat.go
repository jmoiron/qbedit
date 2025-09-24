package mcformat

import (
    "html/template"
    "strings"
)

// Format converts Minecraft color/format codes to HTML using CSS classes.
// Supports both '§' and '&' prefixes.
// Color codes: 0-9, a-f. Formats: k (obfuscated), l (bold), m (strikethrough), n (underline), o (italic), r (reset).
// Returns a template.HTML with spans carrying classes like `mc-color-red`, `mc-bold`, etc.
func Format(s string) template.HTML {
    type state struct {
        color string
        bold bool
        italic bool
        underline bool
        strike bool
        obf bool
    }
    st := state{}
    var b strings.Builder
    writeSpanOpen := func() {
        classes := make([]string, 0, 6)
        classes = append(classes, "mc-text")
        if st.color != "" {
            classes = append(classes, "mc-"+st.color)
        }
        if st.bold { classes = append(classes, "mc-bold") }
        if st.italic { classes = append(classes, "mc-italic") }
        if st.underline { classes = append(classes, "mc-underline") }
        if st.strike { classes = append(classes, "mc-strike") }
        if st.obf { classes = append(classes, "mc-obf") }
        b.WriteString("<span class=\"")
        b.WriteString(strings.Join(classes, " "))
        b.WriteString("\">")
    }
    open := false
    closeSpan := func() {
        if open {
            b.WriteString("</span>")
            open = false
        }
    }
    reset := func() {
        st = state{}
    }
    setColor := func(code rune) {
        switch code {
        case '0': st.color = "c0"
        case '1': st.color = "c1"
        case '2': st.color = "c2"
        case '3': st.color = "c3"
        case '4': st.color = "c4"
        case '5': st.color = "c5"
        case '6': st.color = "c6"
        case '7': st.color = "c7"
        case '8': st.color = "c8"
        case '9': st.color = "c9"
        case 'a', 'A': st.color = "ca"
        case 'b', 'B': st.color = "cb"
        case 'c', 'C': st.color = "cc"
        case 'd', 'D': st.color = "cd"
        case 'e', 'E': st.color = "ce"
        case 'f', 'F': st.color = "cf"
        default:
            // unknown, ignore
        }
    }
    esc := func(r rune) {
        switch r {
        case '&': b.WriteString("&amp;")
        case '<': b.WriteString("&lt;")
        case '>': b.WriteString("&gt;")
        case '"': b.WriteString("&quot;")
        case '\'': b.WriteString("&#39;")
        default:
            b.WriteRune(r)
        }
    }
    i := 0
    rs := []rune(s)
    for i < len(rs) {
        r := rs[i]
        if (r == '§' || r == '&') && i+1 < len(rs) {
            code := rs[i+1]
            // formatting or color codes
            switch code {
            case 'k','K': // obfuscated
                closeSpan(); st.obf = true; writeSpanOpen(); open = true
            case 'l','L': closeSpan(); st.bold = true; writeSpanOpen(); open = true
            case 'm','M': closeSpan(); st.strike = true; writeSpanOpen(); open = true
            case 'n','N': closeSpan(); st.underline = true; writeSpanOpen(); open = true
            case 'o','O': closeSpan(); st.italic = true; writeSpanOpen(); open = true
            case 'r','R': closeSpan(); reset()
            default:
                // color
                closeSpan(); setColor(code); writeSpanOpen(); open = true
            }
            i += 2
            continue
        }
        if !open {
            writeSpanOpen(); open = true
        }
        esc(r)
        i++
    }
    closeSpan()
    return template.HTML(b.String())
}

