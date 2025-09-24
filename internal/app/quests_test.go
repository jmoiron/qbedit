package app

import (
	"bytes"
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
    groups := []Group{
        {ID: "G1", Title: "G1"},
        {ID: "G2", Title: "G2"},
        {ID: "G3", Title: "G3"},
        {ID: "G4", Title: "G4"},
    }
    chapters := []Chapter{
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
