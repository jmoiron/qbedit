# qbedit todos/ideas

- Recolor terms consistently
  - Configure a dictionary: term → color code (Minecraft `&` codes or hex mapping).
  - Operates across title, subtitle, description lines.
  - Options: ignore case, whole‑word only, preserve existing color, or force override.
  - Dry‑run with preview and per‑occurrence accept/reject.

- Global search and replace
  - Plain, case‑insensitive, or regex modes.
  - Scope controls: entire book, group(s), chapter(s), bulk selection, field type(s).
  - Preserve MC color codes; option to strip or normalize codes while replacing.

- Normalize formatting
  - Trim trailing whitespace/blank lines in descriptions.
  - Standardize punctuation (eg, sentence periods, ellipses), smart quotes toggle.

- Terminology/style enforcement
  - Style guide rules (eg, "GregTech" vs "Gregtech"); report and auto‑fix.
  - Color guide management & rules, eg "Mods" all get `&2`, and define what strings are "Mods"
  - Lint pass: title length, subtitle length, description line length/soft wrap.

- Sidebar behavior in bulk mode
  - Display current page's steps as "active"
  - Quick filters (show only changed, only remaining, only conflicts).

- Undo stack for the current session (in‑memory) and last‑write (on disk).
- Conflict detection: alert if file changed on disk since load; offer merge.

- Link/dependency
  - Show dependents/dependencies in editor as a type of navigation
  - Duplicate quest finder - replace with quest links when quests have same criteria

- Media/icon helpers
  - Export QB icons somehow?

- SNBT I/O stability
  - Stable field order on write to minimize diffs (configurable).
  - Pretty-print to match Java more

- Transformation pipeline
  - Compose multiple transforms (search/replace, recolor, normalize) and run in one pass.
  - Save/replay pipelines; export/import as different formats.

- Keyboard shortcuts throughout (save: Ctrl/Cmd‑S; next/prev in bulk; toggle dark).

- Routing/state
  - Add `mode` query param (`mode=bulk`, `mode=review`) and `ids` list for selections.
  - Middleware to capture dark/mode params and expose to templates.

- Batch engine
  - Define a `Transform` interface with `Match(Quest) bool` and `Apply(*Quest) ChangeSet`.
  - Central preview that aggregates ChangeSets and renders diffs.

- Persistence
  - Write through the existing SNBT encoder; add `.bak` creation and optional git commit.

- Testing
  - Golden tests for transforms and SNBT round‑trips on representative chapters.

