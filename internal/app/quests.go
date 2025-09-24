package app

import (
	"fmt"
	"io"
	"sort"

	"github.com/jmoiron/qbedit/snbt"
)

// Quest represents a single quest entry within a Chapter.
//
// For now, we leave quests unmodeled since different quest types carry
// different fields. This gives us flexibility to iterate on the exact
// structure once we begin editing/creating quests. The rendering layer
// can use type assertions or helpers to access common fields like
// "title" and "id" when present.
//
// Future: define a concrete struct per quest type (e.g., CheckmarkTask,
// ItemTask, etc.) and decode into the appropriate type based on the
// "type" field.
type Quest struct {
	Raw         any
	ID          string
	Title       string
	Subtitle    string
	Description string
}

// GetTitle returns the preferred display title for the quest.
// - If Title is set, returns it.
// - Otherwise inspects the first task; if it's an item task, returns the item id.
func (q Quest) GetTitle() string {
	if q.Title != "" {
		return q.Title
	}
	// Inspect first task
	m, ok := q.Raw.(map[string]any)
	if !ok {
		return ""
	}
	tasks, ok := m["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		return ""
	}
	t0, ok := tasks[0].(map[string]any)
	if !ok {
		return ""
	}
	// Prefer item key
	if v, ok := t0["item"]; ok {
		if s := itemToString(v); s != "" {
			return s
		}
	}
	// Some tasks may use 'id' for item
	if v, ok := t0["id"]; ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func itemToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		if id, ok := x["id"].(string); ok {
			return id
		}
		if it, ok := x["item"].(string); ok {
			return it
		}
	}
	return ""
}

// QuestFromRaw constructs a Quest from a generic SNBT-decoded value.
func QuestFromRaw(raw any) Quest {
	q := Quest{Raw: raw}
	m, ok := raw.(map[string]any)
	if !ok {
		return q
	}
	if id, ok := m["id"].(string); ok {
		q.ID = id
	}
	if t, ok := m["title"].(string); ok {
		q.Title = t
	}
	if st, ok := m["subtitle"].(string); ok {
		q.Subtitle = st
	}
	// Description may be a list of strings
	if dl, ok := m["description"].([]any); ok {
		parts := make([]string, 0, len(dl))
		for _, v := range dl {
			if s, ok := v.(string); ok {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			desc := parts[0]
			for i := 1; i < len(parts); i++ {
				desc += "\n" + parts[i]
			}
			q.Description = desc
		}
	} else if ds, ok := m["description"].(string); ok {
		q.Description = ds
	}
	return q
}

// Chapter models a quest chapter file.
type Chapter struct {
	// Name is the base filename (without .snbt) used in URLs.
	Name string
	// ID is the chapter's unique identifier from the file.
	ID    string
	Title string
	// Optional metadata we may use later
	Filename   string
	Icon       string
	Subtitle   []string
	QuestLinks []any
	// Group and ordering
	GroupID    string
	OrderIndex int
	// Quests as unmodeled items for now
	Quests []Quest
	// Raw retains the original decoded map for convenience
	Raw map[string]any
}

// Group organizes chapters under a heading.
type Group struct {
	ID       string
	Title    string
	Chapters []Chapter
}

// TopItem is the ordered node used to render the sidebar tree.
// Either a group or an ungrouped chapter.
type TopItem struct {
	Kind    string // "group" or "chapter"
	Group   *Group
	Chapter *Chapter
}

// scanGroups decodes a chapter_groups.snbt stream and returns groups in file order.
func scanGroups(r io.Reader) ([]Group, error) {
	v, err := snbt.Decode(r)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chapter_groups: not a compound")
	}
	var groups []Group
	if arr, ok := m["chapter_groups"].([]any); ok {
		groups = make([]Group, 0, len(arr))
		for _, it := range arr {
			if mm, ok := it.(map[string]any); ok {
				id, _ := mm["id"].(string)
				title, _ := mm["title"].(string)
				if id == "" {
					continue
				}
				groups = append(groups, Group{ID: id, Title: title})
			}
		}
		return groups, nil
	}
	if arr2, ok := m["chapter_groups"].([]map[string]any); ok {
		groups = make([]Group, 0, len(arr2))
		for _, mm := range arr2 {
			id, _ := mm["id"].(string)
			title, _ := mm["title"].(string)
			if id == "" {
				continue
			}
			groups = append(groups, Group{ID: id, Title: title})
		}
		return groups, nil
	}

	return nil, fmt.Errorf("chapter_groups: missing or invalid chapter_groups array")
}

// buildTopItems interleaves ungrouped chapters by absolute OrderIndex with
// groups in the order provided. Starting at index 0, for each index i:
//   - If there are ungrouped chapters with OrderIndex == i, emit all of them
//     (sorted by title), then advance to i+1.
//   - Otherwise, if there are groups remaining, emit the next group, then
//     advance to i+1.
//
// Continue until all ungrouped chapters and groups are emitted.
func buildTopItems(groups []Group, chapters []Chapter) []TopItem {
	var ungrouped []Chapter
	for _, c := range chapters {
		if c.GroupID == "" {
			ungrouped = append(ungrouped, c)
		}
	}
	// sort the ungrouped chapters by their order index
	sort.Slice(ungrouped, func(i, j int) bool { return ungrouped[i].OrderIndex < ungrouped[j].OrderIndex })

	items := make([]TopItem, len(ungrouped)+len(groups))

	for i := range len(items) {
		if len(ungrouped) > 0 && ungrouped[0].OrderIndex == i {
			items[i] = TopItem{
				Kind:    "chapter",
				Chapter: &ungrouped[0],
			}
			ungrouped = ungrouped[1:]
			continue
		}
		// if we get to here we definitely have groups left
		items[i] = TopItem{
			Kind:  "group",
			Group: &groups[0],
		}
		groups = groups[1:]
	}

	return items
}
