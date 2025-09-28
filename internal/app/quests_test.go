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
		raw:         map[string]any{"id": "Q1", "tasks": []any{}},
		ID:          "Q1",
		Title:       "Quest Title",
		Subtitle:    " these arent multiline ",
		Description: "  Foo  \r\nBar  ",
	}

	q.Sync()

	if got := q.raw["id"]; got != "Q1" {
		t.Fatalf("id mismatch: got %v", got)
	}
	if got := q.raw["title"]; got != "Quest Title" {
		t.Fatalf("title mismatch: got %v", got)
	}
	if got := q.raw["subtitle"]; got != " these arent multiline " {
		t.Fatalf("subtitle mismatch: %#v", q.raw["subtitle"])
	}
	desc, ok := q.raw["description"].([]any)
	if !ok {
		t.Fatalf("description type mismatch: %#v", q.raw["description"])
	}
	if want := []string{"Foo", "Bar"}; !equalAnyStrings(desc, want) {
		t.Fatalf("description mismatch: got %v want %v", desc, want)
	}

	q.Subtitle = ""
	q.Description = ""
	q.Sync()
	if _, ok := q.raw["subtitle"]; ok {
		t.Fatalf("subtitle should be absent, got %#v", q.raw["subtitle"])
	}
	if _, ok := q.raw["description"]; ok {
		t.Fatalf("description should be absent, got %#v", q.raw["description"])
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
	if sub, ok := ch.raw["subtitle"].([]any); !ok || len(sub) != 0 {
		t.Fatalf("subtitle should be empty slice, got %#v", ch.raw["subtitle"])
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

func TestQuestSyncRoundTrip(t *testing.T) {
	orig := map[string]any{
		"id":          "OLD",
		"title":       "Old Title",
		"subtitle":    "Original Subtitle",
		"description": []any{"Start", "Middle"},
		"tasks":       []any{},
	}

	var buf bytes.Buffer
	if err := snbt.Encode(&buf, orig); err != nil {
		t.Fatalf("encode original: %v", err)
	}
	raw1, err := snbt.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	rm1, ok := raw1.(map[string]any)
	if !ok {
		t.Fatalf("decoded quest type = %T, want map[string]any", raw1)
	}
	q1, err := NewQuest(rm1)
	if err != nil {
		t.Fatalf("NewQuest: %v", err)
	}

	q1.Title = "New Title"
	q1.Subtitle = "Line One\nLine Two"
	q1.Description = "Alpha\n\nBeta"
	q1.Sync()

	var buf2 bytes.Buffer
	if err := snbt.Encode(&buf2, q1.raw); err != nil {
		t.Fatalf("encode synced: %v", err)
	}
	raw2, err := snbt.Decode(bytes.NewReader(buf2.Bytes()))
	if err != nil {
		t.Fatalf("decode synced: %v", err)
	}
	rm2, ok := raw2.(map[string]any)
	if !ok {
		t.Fatalf("decoded synced quest type = %T, want map[string]any", raw2)
	}
	q2, err := NewQuest(rm2)
	if err != nil {
		t.Fatalf("NewQuest round-trip: %v", err)
	}

	if q2.ID != q1.ID {
		t.Fatalf("id mismatch: got %q want %q", q2.ID, q1.ID)
	}
	if q2.Title != q1.Title {
		t.Fatalf("title mismatch: got %q want %q", q2.Title, q1.Title)
	}
	if q2.Subtitle != q1.Subtitle {
		t.Fatalf("subtitle mismatch: got %q want %q", q2.Subtitle, q1.Subtitle)
	}
	if q2.Description != q1.Description {
		t.Fatalf("description mismatch: got %q want %q", q2.Description, q1.Description)
	}
}
