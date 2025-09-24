SNBT Parser

This package provides a parser and encoder for the SNBT (stringified NBT) format used by Minecraft data files (e.g., FTB Quests chapters).

Format reference: https://minecraft.fandom.com/wiki/NBT_format

Notes
- The parser is generated from `snbt.peg` using `github.com/pointlander/peg`.
- Regenerate the parser with: `go generate ./snbt`.

Usage

```go
package main

import (
    "bytes"
    "fmt"
    "strings"

    "github.com/jmoiron/qbedit/snbt"
)

func main() {
    // Inline SNBT example
    r := strings.NewReader(`{ title: "Hello", count: 1, active: true, tags: ["a", "b"] }`)

    // Decode into a generic value (any)
    value, err := snbt.Decode(r)
    if err != nil {
        panic(err)
    }

    // Encode the value back to SNBT bytes using an io.Writer
    var buf bytes.Buffer
    if err := snbt.Encode(&buf, value); err != nil {
        panic(err)
    }
    fmt.Println(buf.String())
}
```
