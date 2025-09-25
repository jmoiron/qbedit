package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/qbedit/snbt"
)

type QuestBook struct {
	// root is the root path for this QuestBook; it should be a directory with a
	// "quests" directory inside it, likely called 'ftbquests'.
	root string

	Quests   []*Quest
	Chapters []*Chapter
	Groups   []*Group

	// questMap maps a quest ID to a quest
	questMap map[string]*Quest
	// chapterMap maps a chapter "path" to a chapter
	chapterMap map[string]*Chapter
}

// NewQuestBook instantiates a questbook from a path.
func NewQuestBook(path string) *QuestBook {
	qb := &QuestBook{
		root:       path,
		questMap:   make(map[string]*Quest),
		chapterMap: make(map[string]*Chapter),
	}

	// Load group definitions if present
	var groupsOrder []Group
	if f, err := os.Open(filepath.Join(path, "quests", "chapter_groups.snbt")); err == nil {
		defer f.Close()
		if gs, err := scanGroups(f); err == nil {
			groupsOrder = gs
		}
	}
	groupsByID := make(map[string]*Group)
	unlistedOrder := make([]string, 0)
	for _, g := range groupsOrder {
		gcopy := g
		groupsByID[g.ID] = &gcopy
	}

	// Scan chapters
	chaptersDir := filepath.Join(path, "quests", "chapters")
	entries, err := os.ReadDir(chaptersDir)
	if err != nil {
		// return empty questbook on error
		return qb
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".snbt") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".snbt")
		fp := filepath.Join(chaptersDir, e.Name())
		f, err := os.Open(fp)
		if err != nil {
			continue
		}
		v, err := snbt.Decode(f)
		_ = f.Close()
		if err != nil {
			continue
		}
		rm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		ch := NewChapterWithName(rm, name)
		chp := &ch
		qb.Chapters = append(qb.Chapters, chp)
		qb.chapterMap[name] = chp
		// collect quests and index by ID
		for i := range chp.Quests {
			q := &chp.Quests[i]
			qb.Quests = append(qb.Quests, q)
			if q.ID != "" {
				qb.questMap[q.ID] = q
			}
		}
		// assign chapter to group if specified
		if chp.GroupID != "" {
			gid := chp.GroupID
			grp, ok := groupsByID[gid]
			if !ok {
				title := gid
				groupsByID[gid] = &Group{ID: gid, Title: title}
				grp = groupsByID[gid]
				unlistedOrder = append(unlistedOrder, gid)
			}
			grp.Chapters = append(grp.Chapters, *chp)
		}
	}

	// Build ordered groups slice
	for _, g := range groupsOrder {
		if grp, ok := groupsByID[g.ID]; ok {
			sort.Slice(grp.Chapters, func(i, j int) bool {
				if grp.Chapters[i].OrderIndex != grp.Chapters[j].OrderIndex {
					return grp.Chapters[i].OrderIndex < grp.Chapters[j].OrderIndex
				}
				return grp.Chapters[i].Title < grp.Chapters[j].Title
			})
			if len(grp.Chapters) > 0 {
				qb.Groups = append(qb.Groups, grp)
			}
			delete(groupsByID, g.ID)
		}
	}
	// add remaining groups in encounter order
	for _, gid := range unlistedOrder {
		if grp, ok := groupsByID[gid]; ok {
			sort.Slice(grp.Chapters, func(i, j int) bool {
				if grp.Chapters[i].OrderIndex != grp.Chapters[j].OrderIndex {
					return grp.Chapters[i].OrderIndex < grp.Chapters[j].OrderIndex
				}
				return grp.Chapters[i].Title < grp.Chapters[j].Title
			})
			qb.Groups = append(qb.Groups, grp)
		}
	}

	// Flat chapter sort by title (useful for callers)
	sort.Slice(qb.Chapters, func(i, j int) bool { return qb.Chapters[i].Title < qb.Chapters[j].Title })
	return qb
}

func (q *QuestBook) TopItems() []*TopItem {
	// Convert pointers to value slices for existing builder
	return buildTopItems(q.Groups, q.Chapters)
}

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

// NewQuest creates a new Quest from a raw generic SNBT value.
func NewQuest(raw any) *Quest {
	q := QuestFromRaw(raw)
	return &q
}

// QuestFromRaw constructs a Quest from a generic SNBT-decoded value.
func QuestFromRaw(raw any) Quest {
	q := Quest{Raw: raw}
	rm, ok := raw.(map[string]any)
	if !ok {
		return q
	}

	m := M(rm)
	q.ID = m.GetString("id")
	q.Title = m.GetString("title")
	q.Subtitle = m.GetString("subtitle")
	q.Description = m.GetString("description")

	// try multi-string version of description
	if q.Description == "" {
		ss := m.GetStrings("description")
		q.Description = strings.Join(ss, "\n")
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

// NewChapter constructs a Chapter from a decoded SNBT map.
// The caller should set Chapter.Name from the filename and may override Title
// if empty. Raw is preserved for convenience.
func NewChapter(rm map[string]any) Chapter {
	ch := Chapter{Raw: rm}
	m := M(rm)

	ch.ID = m.GetString("id")
	ch.Title = m.GetString("title")
	ch.Filename = m.GetString("filename")
	ch.Icon = m.GetString("icon")
	ch.GroupID = m.GetString("group")

	if oi, ok := m["order_index"]; ok {
		switch n := oi.(type) {
		case int64:
			ch.OrderIndex = int(n)
		case float64:
			ch.OrderIndex = int(n)
		case int:
			ch.OrderIndex = n
		}
	}

	ch.Subtitle = m.GetStrings("subtitle")
	ch.QuestLinks = m.GetAnys("quest_links")

	for _, qv := range m.GetAnys("quests") {
		ch.Quests = append(ch.Quests, QuestFromRaw(qv))
	}

	return ch
}

// NewChapterWithName builds a Chapter from a map and sets its Name.
// If Title is empty, it defaults to the provided name.
func NewChapterWithName(m map[string]any, name string) Chapter {
	ch := NewChapter(m)
	ch.Name = name
	if ch.Title == "" {
		ch.Title = name
	}
	return ch
}

// Group organizes chapters under a heading.
type Group struct {
	ID       string
	Title    string
	Chapters []Chapter
}

type ItemType int

const (
	GroupType ItemType = iota
	ChapterType
)

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
	rm, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chapter_groups: not a compound")
	}
	m := M(rm)

	arr := m.GetAnys("chapter_groups")
	groups := make([]Group, 0, len(arr))

	for _, it := range arr {
		if mm, ok := it.(map[string]any); ok {
			id, _ := mm["id"].(string)
			if id == "" {
				continue
			}
			title, _ := mm["title"].(string)
			groups = append(groups, Group{ID: id, Title: title})
		}
	}
	return groups, nil
}

// buildTopItems interleaves ungrouped chapters by absolute OrderIndex with
// groups in the order provided. Starting at index 0, for each index i:
//   - If there are ungrouped chapters with OrderIndex == i, emit all of them
//     (sorted by title), then advance to i+1.
//   - Otherwise, if there are groups remaining, emit the next group, then
//     advance to i+1.
//
// Continue until all ungrouped chapters and groups are emitted.
func buildTopItems(groups []*Group, chapters []*Chapter) []*TopItem {
	var ungrouped []*Chapter
	for _, c := range chapters {
		if c.GroupID == "" {
			ungrouped = append(ungrouped, c)
		}
	}
	// sort the ungrouped chapters by their order index
	sort.Slice(ungrouped, func(i, j int) bool { return ungrouped[i].OrderIndex < ungrouped[j].OrderIndex })

	items := make([]*TopItem, len(ungrouped)+len(groups))

	for i := range len(items) {
		if len(ungrouped) > 0 && ungrouped[0].OrderIndex == i {
			items[i] = &TopItem{
				Kind:    "chapter",
				Chapter: ungrouped[0],
			}
			ungrouped = ungrouped[1:]
			continue
		}
		// if we get to here we definitely have groups left
		items[i] = &TopItem{
			Kind:  "group",
			Group: groups[0],
		}
		groups = groups[1:]
	}

	return items
}
