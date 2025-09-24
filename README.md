qbedit
=====

A web UI tool for viewing and editing FTB Quests quest books.

Install
-------

Requires a recent Go toolchain.

```
# Install the latest version
go install github.com/jmoiron/qbedit@latest
```

This places the `qbedit` binary in your `GOBIN` (usually `$GOPATH/bin` or `$HOME/go/bin`).

Quick Start
-----------

```
# Run against an FTB Quests directory (contains quests/chapters/*.snbt)
qbedit --addr 0.0.0.0:8222 /path/to/ftbquests
```

- Open http://localhost:8222
- Use the sidebar to navigate chapters and quests
- Dark mode toggle is in the sidebar footer

![v0-dark-mode](https://github.com/user-attachments/assets/ca0e15de-5a15-406d-ac70-3d305317eaef)

Flags:
- `--addr` (default `0.0.0.0:8222`) — listen address
- `--mcv`  (default `1.20.1`)      — Minecraft version tag
- `-v` to increase verbosity

Development
-----------

```
# Regenerate the SNBT parser after grammar changes
go generate ./snbt

# Run tests
go test ./...
```
