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

The quest editor is able to utilize your browser's built in spell checking, allowing you to quickly fix typos and spelling mistakes.

![v0-dark-mode](https://github.com/user-attachments/assets/ca0e15de-5a15-406d-ac70-3d305317eaef)

When  you find a mistake, there is a _batch editing_ mode that lets you search for quests that might also contain that mistake and edit them all in one place. The _batch editor_ contains tools for you to edit all quests that lack a title, subtitle, or description, so you can quickly fill in quests that are missing information.

There is also a _color manager_, which lets you quickly synchronize styles across your questbook:

![color-manager](https://github.com/user-attachments/assets/819a0dd6-6c17-49f1-a07f-91d01064575b)


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
