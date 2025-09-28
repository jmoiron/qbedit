package app

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/jmoiron/qbedit/snbt"
)

const sampleChapterGroups = `{
	chapter_groups: [
		{ id: "693226878D58638A", title: "The Age of Technology" }
		{ id: "09D97B44850738EB", title: "Chemical Warfare" }
		{ id: "3F9DCC5BE002E182", title: "Space Program" }
		{ id: "5524D30A9D815436", title: "Logistics" }
		{ id: "1C1C4FB2AFCF489D", title: "Progression" }
	]
}
`

func TestScanGroups(t *testing.T) {
	r := bytes.NewBufferString(sampleChapterGroups)
	groups, err := scanGroups(r)
	if err != nil {
		v, derr := snbt.Decode(bytes.NewReader([]byte(sampleChapterGroups)))
		t.Fatalf("scanGroups error: %v; decodeErr=%v; decoded=%#v", err, derr, v)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups, got %d", len(groups))
	}
	want := []struct{ id, title string }{
		{"693226878D58638A", "The Age of Technology"},
		{"09D97B44850738EB", "Chemical Warfare"},
		{"3F9DCC5BE002E182", "Space Program"},
		{"5524D30A9D815436", "Logistics"},
		{"1C1C4FB2AFCF489D", "Progression"},
	}
	for i, g := range groups {
		if g.ID != want[i].id || g.Title != want[i].title {
			t.Fatalf("group %d mismatch: got (%s,%s) want (%s,%s)", i, g.ID, g.Title, want[i].id, want[i].title)
		}
	}
}

func TestBuildTopItems_Interleave(t *testing.T) {
	groups := []*Group{
		{ID: "G1", Title: "G1"},
		{ID: "G2", Title: "G2"},
		{ID: "G3", Title: "G3"},
		{ID: "G4", Title: "G4"},
	}
	chapters := []*Chapter{
		{Name: "intro", Title: "Introduction", OrderIndex: 0},
		{Name: "from_nothing", Title: "From Nothing", OrderIndex: 1},
		{Name: "later", Title: "Later", OrderIndex: 3},
	}
	top := buildTopItems(groups, chapters)
	got := make([]string, 0, len(top))
	for _, ti := range top {
		if ti.Kind == "chapter" {
			got = append(got, "C:"+ti.Chapter.Title)
		} else {
			got = append(got, "G:"+ti.Group.Title)
		}
	}
	want := []string{"C:Introduction", "C:From Nothing", "G:G1", "C:Later", "G:G2", "G:G3", "G:G4"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pos %d: got %q want %q (seq=%v)", i, got[i], want[i], got)
		}
	}
}

func TestQuestSyncMultistring(t *testing.T) {
	q := &Quest{
		raw:         map[string]any{"tasks": []any{}},
		ID:          "Q1",
		Title:       "Quest Title",
		Subtitle:    "  First Line  \r\n  \r\nSecond Line  ",
		Description: "  Foo  \r\nBar  ",
	}

	q.Sync()

	if got := q.raw["id"]; got != "Q1" {
		t.Fatalf("id mismatch: got %v", got)
	}
	if got := q.raw["title"]; got != "Quest Title" {
		t.Fatalf("title mismatch: got %v", got)
	}
	sub, ok := q.raw["subtitle"].([]any)
	if !ok {
		t.Fatalf("subtitle type mismatch: %#v", q.raw["subtitle"])
	}
	if want := []string{"First Line", "", "Second Line"}; !equalAnyStrings(sub, want) {
		t.Fatalf("subtitle mismatch: got %v want %v", sub, want)
	}
	desc, ok := q.raw["description"].([]any)
	if !ok {
		t.Fatalf("description type mismatch: %#v", q.raw["description"])
	}
	if want := []string{"Foo", "Bar"}; !equalAnyStrings(desc, want) {
		t.Fatalf("description mismatch: got %v want %v", desc, want)
	}

	q.Subtitle = ""
	q.Description = "\t"
	q.Sync()
	if _, ok := q.raw["subtitle"]; ok {
		t.Fatalf("subtitle should be cleared, got %v", q.raw["subtitle"])
	}
	if _, ok := q.raw["description"]; ok {
		t.Fatalf("description should be cleared, got %v", q.raw["description"])
	}
}

func TestChapterSync(t *testing.T) {
	q1 := &Quest{raw: map[string]any{}, ID: "Q1", Title: "One", Subtitle: "Line", Description: "Desc"}
	q2 := &Quest{raw: map[string]any{}, ID: "Q2"}
	ch := &Chapter{
		raw:        map[string]any{},
		ID:         "CH1",
		Title:      "Chapter Title",
		Filename:   "chapter",
		Icon:       "minecraft:stone",
		GroupID:    "Group",
		OrderIndex: 3,
		Subtitle:   []string{"LineA", "LineB"},
		QuestLinks: []any{"link"},
		Quests:     []*Quest{q1, q2},
	}

	ch.Sync()

	if got := ch.raw["id"]; got != "CH1" {
		t.Fatalf("id mismatch: got %v", got)
	}
	if got := ch.raw["title"]; got != "Chapter Title" {
		t.Fatalf("title mismatch: got %v", got)
	}
	if got := ch.raw["filename"]; got != "chapter" {
		t.Fatalf("filename mismatch: got %v", got)
	}
	if got := ch.raw["icon"]; got != "minecraft:stone" {
		t.Fatalf("icon mismatch: got %v", got)
	}
	if got := ch.raw["group"]; got != "Group" {
		t.Fatalf("group mismatch: got %v", got)
	}
	if got := ch.raw["order_index"]; got != 3 {
		t.Fatalf("order_index mismatch: got %v", got)
	}
	sub, ok := ch.raw["subtitle"].([]any)
	if !ok {
		t.Fatalf("subtitle type mismatch: %#v", ch.raw["subtitle"])
	}
	if want := []string{"LineA", "LineB"}; !equalAnyStrings(sub, want) {
		t.Fatalf("subtitle mismatch: got %v want %v", sub, want)
	}
	qlinks, ok := ch.raw["quest_links"].([]any)
	if !ok {
		t.Fatalf("quest_links type mismatch: %#v", ch.raw["quest_links"])
	}
	if len(qlinks) != 1 || qlinks[0] != "link" {
		t.Fatalf("quest_links mismatch: got %v", qlinks)
	}
	questsVal, ok := ch.raw["quests"].([]any)
	if !ok {
		t.Fatalf("quests type mismatch: %#v", ch.raw["quests"])
	}
	if len(questsVal) != 2 {
		t.Fatalf("quests length mismatch: got %d", len(questsVal))
	}
	if qrm, ok := questsVal[0].(map[string]any); !ok || !reflect.DeepEqual(qrm, q1.raw) {
		t.Fatalf("quests[0] mismatch: got %#v want %#v", questsVal[0], q1.raw)
	}
	if qrm, ok := questsVal[1].(map[string]any); !ok || !reflect.DeepEqual(qrm, q2.raw) {
		t.Fatalf("quests[1] mismatch: got %#v want %#v", questsVal[1], q2.raw)
	}

	ch.Subtitle = nil
	ch.QuestLinks = nil
	ch.Quests = nil
	ch.Sync()
	if _, ok := ch.raw["subtitle"]; ok {
		t.Fatalf("subtitle should be cleared, got %v", ch.raw["subtitle"])
	}
	if qlinks, ok := ch.raw["quest_links"].([]any); !ok || len(qlinks) != 0 {
		t.Fatalf("quest_links should be empty slice, got %#v", ch.raw["quest_links"])
	}
	if questsVal, ok := ch.raw["quests"].([]any); !ok || len(questsVal) != 0 {
		t.Fatalf("quests should be empty slice, got %#v", ch.raw["quests"])
	}
}

func equalAnyStrings(vals []any, want []string) bool {
	if len(vals) != len(want) {
		return false
	}
	for i, v := range vals {
		s, ok := v.(string)
		if !ok || s != want[i] {
			return false
		}
	}
	return true
}
