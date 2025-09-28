package app

import (
	"fmt"
	"io"
	"log/slog"
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
	// groupMap maps a group "ID" to a group
	groupMap map[string]*Group
}

// NewQuestBook instantiates a questbook from a path.
func NewQuestBook(path string) (*QuestBook, error) {
	qb := &QuestBook{
		root:       path,
		questMap:   make(map[string]*Quest),
		chapterMap: make(map[string]*Chapter),
		groupMap:   make(map[string]*Group),
	}

	// Load group definitions if present
	if err := qb.loadGroups(); err != nil {
		slog.Error("error loading chapter groups", "error", err)
		return nil, err
	}

	if err := qb.loadChapters(); err != nil {
		return nil, err
	}

	// add global accounting for quests and chapters
	// XXX: should we order the chapters first?
	for _, c := range qb.Chapters {
		// collect quests and index by ID
		for _, q := range c.Quests {
			qb.Quests = append(qb.Quests, q)
			qb.questMap[q.ID] = q
		}

		// add chapter to a group
		if c.GroupID != "" {
			g, ok := qb.groupMap[c.GroupID]
			if !ok {
				slog.Warn("unknown groupID in chapter", "groupID", c.GroupID, "chapter", c.Filename)
				continue
			}
			g.Chapters = append(g.Chapters, c)
		}
	}

	// order the quests within each group
	for _, g := range qb.Groups {
		// XXX: original code checked for same ordering and then sorted by title but
		// i don't think that's necessary or possible?
		sort.Slice(g.Chapters, func(i, j int) bool {
			return g.Chapters[i].OrderIndex < g.Chapters[j].OrderIndex
		})
	}

	// XXX: chapters could be sorted by their appearance in the quest book but
	// that's a bit tricky
	sort.Slice(qb.Chapters, func(i, j int) bool { return qb.Chapters[i].Title < qb.Chapters[j].Title })
	return qb, nil
}

func (q *QuestBook) TopItems() []*TopItem {
	// Convert pointers to value slices for existing builder
	return buildTopItems(q.Groups, q.Chapters)
}

func (q *QuestBook) loadGroups() error {
	gp := filepath.Join(q.root, "quests", "chapter_groups.snbt")
	f, err := os.Open(gp)
	if err != nil {
		return err
	}
	defer f.Close()

	groups, err := scanGroups(f)
	if err != nil {
		return err
	}
	q.Groups = groups

	groupMap := make(map[string]*Group)
	for _, g := range q.Groups {
		groupMap[g.ID] = g
	}
	q.groupMap = groupMap

	return nil
}

func (q *QuestBook) loadChapters() error {
	dir := filepath.Join(q.root, "quests", "chapters")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var chapters []*Chapter
	chapterMap := make(map[string]*Chapter)
	for _, e := range entries {
		// skip directories and non-snbt files
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".snbt") {
			continue
		}
		c, err := NewChapterFromPath(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		chapters = append(chapters, c)
		chapterMap[c.Name] = c
	}

	q.Chapters = chapters
	q.chapterMap = chapterMap
	return nil
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
func NewQuest(raw any) (*Quest, error) {
	rm, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("new quest expected compound, but got %T", rm)
	}

	m := M(rm)
	q := &Quest{
		Raw:         raw,
		ID:          m.GetString("id"),
		Title:       m.GetString("title"),
		Subtitle:    m.GetString("subtitle"),
		Description: m.GetString("description"),
	}

	// try multi-string version of description
	if q.Description == "" {
		ss := m.GetStrings("description")
		q.Description = strings.Join(ss, "\n")
	}

	return q, nil
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
	Quests     []*Quest

	// Raw retains the original decoded map for convenience
	Raw map[string]any
}

// TODO: clean up the constructors of Chapter

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
		q, err := NewQuest(qv)
		if err != nil {
			slog.Error("error loading quest", "chapter", ch.Filename, "quest", qv)
			continue
		}
		ch.Quests = append(ch.Quests, q)
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

func NewChapterFromPath(path string) (*Chapter, error) {
	fallback := strings.TrimSuffix(filepath.Base(path), ".snbt")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	v, err := snbt.Decode(f)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chapter at %s: expected compound, got %T", path, v)
	}
	ch := NewChapterWithName(m, fallback)
	return &ch, nil
}

// Group organizes chapters under a heading.
type Group struct {
	ID       string
	Title    string
	Chapters []*Chapter
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
func scanGroups(r io.Reader) ([]*Group, error) {
	v, err := snbt.Decode(r)
	if err != nil {
		return nil, err
	}
	rm, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chapter_groups: expecting a compound, found %v", v)
	}

	m := M(rm)
	arr := m.GetAnys("chapter_groups")
	groups := make([]*Group, 0, len(arr))

	for _, it := range arr {
		mm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		g := &Group{ID: M(mm).GetString("id"), Title: M(mm).GetString("title")}
		if g.ID == "" {
			continue
		}
		groups = append(groups, g)
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
