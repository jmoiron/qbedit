package app

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-sprout/sprout"
	"github.com/jmoiron/qbedit/internal/app/mcformat"
	"github.com/jmoiron/qbedit/snbt"
)

// Chapter type is defined in quests.go

type App struct {
	Root      string
	MCVersion string
	Verbose   int
	Chapters  []Chapter
	tpl       *template.Template
	Parsed    int
	Failed    int
	Failures  []Failure
	Groups    []Group
	Top       []TopItem
}

type Failure struct {
	Name string
	Path string
	Err  string
}

// Group and TopItem types are defined in quests.go

//go:embed templates/*.gohtml static/*
var templatesFS embed.FS

func New(root, mc string, verbose int) (*App, error) {
	a := &App{Root: root, MCVersion: mc, Verbose: verbose}
	if err := a.scan(); err != nil {
		return nil, err
	}
	// Load templates from embedded FS
	sub, _ := fs.Sub(templatesFS, "templates")
	sh := sprout.New()
	funcs := sh.Build()
	// extend with a small helper
	funcs["eq"] = func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) }
	funcs["mc"] = func(s string) template.HTML { return mcformat.Format(s) }
	// helpers for pagination math
	funcs["add"] = func(a, b int) int { return a + b }
	funcs["mul"] = func(a, b int) int { return a * b }
	funcs["min"] = func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}
	funcs["ceilDiv"] = func(a, b int) int {
		if b <= 0 {
			return 0
		}
		return (a + b - 1) / b
	}
	tpl, err := template.New("base").Funcs(funcs).ParseFS(sub, "*.gohtml")
	if err != nil {
		return nil, err
	}
	a.tpl = tpl
	return a, nil
}

func (a *App) scan() error {
	chaptersDir := filepath.Join(a.Root, "quests", "chapters")
	entries, err := os.ReadDir(chaptersDir)
	if err != nil {
		return fmt.Errorf("read chapters dir: %w", err)
	}
	// Load group definitions
	var groupsOrder []Group
	if f, err := os.Open(filepath.Join(a.Root, "quests", "chapter_groups.snbt")); err == nil {
		defer f.Close()
		if gs, err := scanGroups(f); err == nil {
			groupsOrder = gs
		}
	}
	groupsByID := make(map[string]*Group)
	// Track encounter order for any groups not listed in chapter_groups
	unlistedOrder := make([]string, 0)
	for _, g := range groupsOrder {
		gcopy := g
		groupsByID[g.ID] = &gcopy
	}

	var chapters []Chapter
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".snbt") {
			continue
		}
		path := filepath.Join(chaptersDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			log.Printf("scan open %s: %v", path, err)
			a.Failed++
			a.Failures = append(a.Failures, Failure{Name: e.Name(), Path: path, Err: err.Error()})
			continue
		}
		v, err := snbt.Decode(f)
		f.Close()
		if err != nil {
			log.Printf("decode %s: %v", path, err)
			a.Failed++
			a.Failures = append(a.Failures, Failure{Name: e.Name(), Path: path, Err: err.Error()})
			continue
		}
		chapter := Chapter{Name: strings.TrimSuffix(e.Name(), ".snbt")}
		if m, ok := v.(map[string]any); ok {
			chapter.Raw = m
			if id, ok := m["id"].(string); ok {
				chapter.ID = id
			}
			if t, ok := m["title"].(string); ok {
				chapter.Title = t
			}
			if fn, ok := m["filename"].(string); ok {
				chapter.Filename = fn
			}
			if icon, ok := m["icon"].(string); ok {
				chapter.Icon = icon
			}
			if gid, ok := m["group"].(string); ok {
				chapter.GroupID = gid
			}
			if oi, ok := m["order_index"]; ok {
				switch n := oi.(type) {
				case int64:
					chapter.OrderIndex = int(n)
				case float64:
					chapter.OrderIndex = int(n)
				}
			}
			if sl, ok := m["subtitle"].([]any); ok {
				subs := make([]string, 0, len(sl))
				for _, v := range sl {
					if s, ok := v.(string); ok {
						subs = append(subs, s)
					}
				}
				chapter.Subtitle = subs
			}
			if ql, ok := m["quest_links"].([]any); ok {
				chapter.QuestLinks = ql
			}
			// Extract quests
			if ql, ok := m["quests"].([]any); ok {
				for _, qv := range ql {
					chapter.Quests = append(chapter.Quests, QuestFromRaw(qv))
				}
			}
		}
		if chapter.Title == "" {
			chapter.Title = chapter.Name
		}
		chapters = append(chapters, chapter)
		// assign to group bucket
		// Only assign to a group if the chapter declares one
		if chapter.GroupID != "" {
			gid := chapter.GroupID
			grp, ok := groupsByID[gid]
			if !ok {
				// create placeholder group if not defined in chapter_groups
				title := gid
				groupsByID[gid] = &Group{ID: gid, Title: title}
				grp = groupsByID[gid]
				unlistedOrder = append(unlistedOrder, gid)
			}
			grp.Chapters = append(grp.Chapters, chapter)
		}
		a.Parsed++
	}
	// Build ordered groups slice
	var groups []Group
	// keep defined order first
	for _, g := range groupsOrder {
		if grp, ok := groupsByID[g.ID]; ok {
			// sort chapters within group
			sort.Slice(grp.Chapters, func(i, j int) bool {
				if grp.Chapters[i].OrderIndex != grp.Chapters[j].OrderIndex {
					return grp.Chapters[i].OrderIndex < grp.Chapters[j].OrderIndex
				}
				return grp.Chapters[i].Title < grp.Chapters[j].Title
			})
			if len(grp.Chapters) > 0 {
				groups = append(groups, *grp)
			}
			delete(groupsByID, g.ID)
		}
	}
	// add any remaining groups (not listed in chapter_groups) preserving encounter order
	for _, gid := range unlistedOrder {
		grp, ok := groupsByID[gid]
		if !ok {
			continue
		}
		sort.Slice(grp.Chapters, func(i, j int) bool {
			if grp.Chapters[i].OrderIndex != grp.Chapters[j].OrderIndex {
				return grp.Chapters[i].OrderIndex < grp.Chapters[j].OrderIndex
			}
			return grp.Chapters[i].Title < grp.Chapters[j].Title
		})
		groups = append(groups, *grp)
	}
	a.Groups = groups
	// Keep flat list as well (sorted by title) if needed elsewhere
	sort.Slice(chapters, func(i, j int) bool { return chapters[i].Title < chapters[j].Title })
	a.Chapters = chapters

	// Build top-level interleaved tree using helper
	a.Top = buildTopItems(groups, chapters)
	return nil
}

// scanGroups is defined in quests.go

func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	if a.Verbose > 0 {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	// Static assets
	mime.AddExtensionType(".css", "text/css")
	staticFS, _ := fs.Sub(templatesFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/", a.index)
	r.Get("/batch/", a.batch)
	r.Get("/batch/edit", a.batchEdit)
	r.Get("/chapter/{chapter}", a.chapterDetail)
	r.Get("/chapter/{chapter}/{quest}", a.questDetail)
	r.Post("/chapter/{chapter}/{quest}/save", a.questSave)
	r.Get("/chapter/{chapter}/raw", a.chapterRaw)
	r.Get("/errors", a.errors)

	return r
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// baseData returns common template data to keep the sidebar consistent.
func (a *App) baseData(r *http.Request, title string) map[string]any {
	// Dark mode detection precedence:
	// 1) Explicit query param ?dark=true forces dark for this render
	// 2) Fallback to cookie set by client toggle
	themeDark := false
	if v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("dark"))); v != "" {
		if v == "1" || v == "true" || v == "t" || v == "yes" || v == "on" {
			themeDark = true
		}
	} else if c, err := r.Cookie("theme"); err == nil && c != nil && c.Value == "dark" {
		themeDark = true
	}
	return map[string]any{
		"Chapters":    a.Chapters,
		"Groups":      a.Groups,
		"Top":         a.Top,
		"MCVersion":   a.MCVersion,
		"Title":       title,
		"Parsed":      a.Parsed,
		"Failed":      a.Failed,
		"HasFailures": a.Failed > 0,
		"ThemeDark":   themeDark,
	}
}

// index handles GET "/".
func (a *App) index(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r, "qbedit")
	a.render(w, "index.gohtml", data)
}

// batch handles GET "/batch/" and displays a search form plus results.
func (a *App) batch(w http.ResponseWriter, r *http.Request) {
	// Only show search form here; results are on /batch/edit
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cg := strings.TrimSpace(r.URL.Query().Get("cg"))
	noTitle := r.URL.Query().Has("no_title")
	noSubtitle := r.URL.Query().Has("no_subtitle")
	noDesc := r.URL.Query().Has("no_desc")
	caseSensitive := r.URL.Query().Has("case")
	perPage := 5
	if n := strings.TrimSpace(r.URL.Query().Get("n")); n != "" {
		switch n {
		case "10":
			perPage = 10
		case "20":
			perPage = 20
		}
	}

	data := a.baseData(r, "Batch Editor")
	data["Form"] = map[string]any{
		"cg": cg, "q": q,
		"no_title": noTitle, "no_subtitle": noSubtitle, "no_desc": noDesc,
		"case": caseSensitive,
		"n":    perPage,
	}
	// Provide options for the Chapter/Group datalist
	var cgOptions []string
	for _, g := range a.Groups {
		if g.Title != "" {
			cgOptions = append(cgOptions, g.Title)
		}
	}
	for _, ch := range a.Chapters {
		if ch.Title != "" {
			cgOptions = append(cgOptions, ch.Title)
		}
	}
	data["CGOptions"] = cgOptions
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		data["BatchMsg"] = msg
	}
	a.render(w, "batch.gohtml", data)
}

// batchEdit performs the search and displays results in the normal layout, using
// the site's left pane to render the search result tree instead of the global chapters.
func (a *App) batchEdit(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cg := strings.TrimSpace(r.URL.Query().Get("cg"))
	noTitle := r.URL.Query().Has("no_title")
	noSubtitle := r.URL.Query().Has("no_subtitle")
	noDesc := r.URL.Query().Has("no_desc")
	caseSensitive := r.URL.Query().Has("case")
	perPage := 5
	if n := strings.TrimSpace(r.URL.Query().Get("n")); n != "" {
		switch n {
		case "10":
			perPage = 10
		case "20":
			perPage = 20
		}
	}
	page := 1
	if p := strings.TrimSpace(r.URL.Query().Get("p")); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	// Scope by Chapter/Group
	scope := make(map[string]bool)
	if cg != "" {
		lc := strings.ToLower(cg)
		for _, g := range a.Groups {
			if strings.Contains(strings.ToLower(g.Title), lc) || strings.EqualFold(g.ID, cg) {
				for _, ch := range g.Chapters {
					scope[ch.Name] = true
				}
			}
		}
		for _, ch := range a.Chapters {
			if strings.Contains(strings.ToLower(ch.Title), lc) || strings.EqualFold(ch.Name, cg) {
				scope[ch.Name] = true
			}
		}
	}

	// Collect matches
	type QRef struct {
		Chapter *Chapter
		Quest   *Quest
	}
	var matches []QRef
	// Strip MC color/format codes (&x or §x), preserve case; lower later if needed.
	stripCodes := func(s string) string {
		if s == "" {
			return s
		}
		if !strings.ContainsAny(s, "&§") {
			return s
		}
		b := make([]rune, 0, len(s))
		skip := false
		for _, r := range s {
			if skip {
				skip = false
				continue
			}
			if r == '&' || r == '§' {
				skip = true
				continue
			}
			b = append(b, r)
		}
		return string(b)
	}
	// A query matches when all query terms appear as substrings in any of the quest fields.
	// Terms are whitespace-split.
	terms := []string{}
	for _, part := range strings.Fields(q) {
		p := strings.TrimSpace(part)
		if !caseSensitive {
			p = strings.ToLower(p)
		}
		if p != "" {
			terms = append(terms, p)
		}
	}
	matchQuest := func(qs *Quest) bool {
		if len(terms) == 0 {
			return true
		}
		t1 := stripCodes(qs.Title)
		t2 := stripCodes(qs.Subtitle)
		t3 := stripCodes(qs.Description)
		t4 := stripCodes(qs.GetTitle())
		if !caseSensitive {
			t1 = strings.ToLower(t1)
			t2 = strings.ToLower(t2)
			t3 = strings.ToLower(t3)
			t4 = strings.ToLower(t4)
		}
		for _, term := range terms {
			if !(strings.Contains(t1, term) || strings.Contains(t2, term) || strings.Contains(t3, term) || strings.Contains(t4, term)) {
				return false
			}
		}
		return true
	}
	for i := range a.Chapters {
		ch := &a.Chapters[i]
		if len(scope) > 0 && !scope[ch.Name] {
			continue
		}
		for j := range ch.Quests {
			qs := &ch.Quests[j]
			if noTitle && qs.Title != "" {
				continue
			}
			if noSubtitle && qs.Subtitle != "" {
				continue
			}
			if noDesc && qs.Description != "" {
				continue
			}
			if !matchQuest(qs) {
				continue
			}
			matches = append(matches, QRef{Chapter: ch, Quest: qs})
		}
	}
	if len(matches) == 0 {
		// Redirect back to /batch/ with a message
		// Preserve the user's query parameters
		qs := r.URL.Query()
		qs.Set("msg", "No results")
		http.Redirect(w, r, "/batch/?"+qs.Encode(), http.StatusSeeOther)
		return
	}

	// Build sidebar structure for results
	type SideQuest struct{ ID, Title string }
	type SideChapter struct {
		Name, Title string
		Quests      []SideQuest
	}
	type SideGroup struct {
		Title    string
		Chapters []SideChapter
	}
	var sb []SideGroup
	byChapter := make(map[string][]SideQuest)
	for _, mr := range matches {
		title := mr.Quest.GetTitle()
		byChapter[mr.Chapter.Name] = append(byChapter[mr.Chapter.Name], SideQuest{ID: mr.Quest.ID, Title: title})
	}
	for _, g := range a.Groups {
		var sc []SideChapter
		for _, ch := range g.Chapters {
			if qs, ok := byChapter[ch.Name]; ok && len(qs) > 0 {
				sc = append(sc, SideChapter{Name: ch.Name, Title: ch.Title, Quests: qs})
				delete(byChapter, ch.Name)
			}
		}
		if len(sc) > 0 {
			sb = append(sb, SideGroup{Title: g.Title, Chapters: sc})
		}
	}
	if len(byChapter) > 0 {
		var sc []SideChapter
		for _, ch := range a.Chapters {
			if ch.GroupID != "" {
				continue
			}
			if qs, ok := byChapter[ch.Name]; ok && len(qs) > 0 {
				sc = append(sc, SideChapter{Name: ch.Name, Title: ch.Title, Quests: qs})
			}
		}
		if len(sc) > 0 {
			sb = append(sb, SideGroup{Title: "Ungrouped", Chapters: sc})
		}
	}

	// Pagination
	total := len(matches)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	pageMatches := matches[start:end]

	data := a.baseData(r, "Batch Editor")
	data["BatchSidebar"] = sb
	data["BatchMatches"] = pageMatches
	data["BatchTotal"] = total
	data["BatchPerPage"] = perPage
	data["BatchPage"] = page
	data["Form"] = map[string]any{
		"cg": cg, "q": q,
		"no_title": noTitle, "no_subtitle": noSubtitle, "no_desc": noDesc,
		"case": caseSensitive,
		"n":    perPage,
	}
	a.render(w, "batch_edit.gohtml", data)
}

// chapterDetail handles GET "/chapter/{chapter}".
func (a *App) chapterDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "chapter")
	var ch *Chapter
	for i := range a.Chapters {
		if a.Chapters[i].Name == name {
			ch = &a.Chapters[i]
			break
		}
	}
	if ch == nil {
		http.NotFound(w, r)
		return
	}
	data := a.baseData(r, ch.Title)
	data["Chapter"] = ch
	data["SelectedChapter"] = ch.Name
	a.render(w, "chapter.gohtml", data)
}

// errors handles GET "/errors".
func (a *App) errors(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r, "Errors")
	data["Failures"] = a.Failures
	a.render(w, "errors.gohtml", data)
}

// chapterRaw handles GET "/chapter/{chapter}/raw".
func (a *App) chapterRaw(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "chapter")
	var ch *Chapter
	for i := range a.Chapters {
		if a.Chapters[i].Name == name {
			ch = &a.Chapters[i]
			break
		}
	}
	if ch == nil {
		http.NotFound(w, r)
		return
	}
	// Read raw file contents
	path := filepath.Join(a.Root, "quests", "chapters", ch.Name+".snbt")
	data := a.baseData(r, "Raw: "+ch.Title)
	data["Chapter"] = ch
	data["SelectedChapter"] = ch.Name
	if b, err := os.ReadFile(path); err == nil {
		data["Raw"] = string(b)
	} else {
		data["Raw"] = fmt.Sprintf("(error reading %s: %v)", path, err)
	}
	a.render(w, "chapter_raw.gohtml", data)
}

// questDetail handles GET "/chapter/{chapter}/{quest}".
func (a *App) questDetail(w http.ResponseWriter, r *http.Request) {
	cname := chi.URLParam(r, "chapter")
	qid := chi.URLParam(r, "quest")
	var ch *Chapter
	for i := range a.Chapters {
		if a.Chapters[i].Name == cname {
			ch = &a.Chapters[i]
			break
		}
	}
	if ch == nil {
		http.NotFound(w, r)
		return
	}
	var q *Quest
	for i := range ch.Quests {
		if ch.Quests[i].ID == qid {
			q = &ch.Quests[i]
			break
		}
	}
	if q == nil {
		http.NotFound(w, r)
		return
	}
	title := q.GetTitle()
	if title == "" {
		title = "Edit Quest"
	}
	data := a.baseData(r, title)
	data["SelectedChapter"] = ch.Name
	data["Chapter"] = ch
	data["Quest"] = q
	a.render(w, "quest.gohtml", data)
}

// questSave handles POST "/chapter/{chapter}/{quest}/save" to persist edits.
func (a *App) questSave(w http.ResponseWriter, r *http.Request) {
	isAjax := r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json")
	writeJSON := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	if err := r.ParseForm(); err != nil {
		if isAjax {
			writeJSON(http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid form"})
		} else {
			http.Error(w, "invalid form", http.StatusBadRequest)
		}
		return
	}
	cname := chi.URLParam(r, "chapter")
	qid := chi.URLParam(r, "quest")
	title := strings.TrimSpace(r.Form.Get("title"))
	subtitle := strings.TrimSpace(r.Form.Get("subtitle"))
	desc := r.Form.Get("description")

	// Load raw chapter file
	path := filepath.Join(a.Root, "quests", "chapters", cname+".snbt")
	f, err := os.Open(path)
	if err != nil {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		} else {
			http.Error(w, "open chapter: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	v, err := snbt.Decode(f)
	f.Close()
	if err != nil {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		} else {
			http.Error(w, "decode: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	m, ok := v.(map[string]any)
	if !ok {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": "chapter not a compound"})
		} else {
			http.Error(w, "chapter not a compound", http.StatusInternalServerError)
		}
		return
	}
	arr, ok := m["quests"].([]any)
	if !ok {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": "chapter missing quests"})
		} else {
			http.Error(w, "chapter missing quests", http.StatusInternalServerError)
		}
		return
	}
	// Find quest by id
	found := false
	for i := range arr {
		if qm, ok := arr[i].(map[string]any); ok {
			if id, _ := qm["id"].(string); id == qid {
				if title != "" {
					qm["title"] = title
				} else {
					delete(qm, "title")
				}
				if subtitle != "" {
					qm["subtitle"] = subtitle
				} else {
					delete(qm, "subtitle")
				}
				// description as list of strings split by lines
				dlines := strings.Split(desc, "\n")
				// trim trailing empty lines
				j := len(dlines)
				for j > 0 && strings.TrimSpace(dlines[j-1]) == "" {
					j--
				}
				dlines = dlines[:j]
				if len(dlines) > 0 {
					dl := make([]any, 0, len(dlines))
					for _, s := range dlines {
						dl = append(dl, s)
					}
					qm["description"] = dl
				} else {
					delete(qm, "description")
				}
				arr[i] = qm
				found = true
				break
			}
		}
	}
	if !found {
		if isAjax {
			writeJSON(http.StatusNotFound, map[string]any{"ok": false, "error": "quest not found"})
		} else {
			http.Error(w, "quest not found", http.StatusNotFound)
		}
		return
	}
	m["quests"] = arr
	// Encode back to file
	var buf bytes.Buffer
	if err := snbt.Encode(&buf, m); err != nil {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		} else {
			http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		if isAjax {
			writeJSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		} else {
			http.Error(w, "write: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// Refresh in-memory data
	if err := a.scan(); err != nil {
		log.Printf("rescan error: %v", err)
	}
	if isAjax {
		writeJSON(http.StatusOK, map[string]any{"ok": true})
		return
	}
	// Redirect back to quest detail
	http.Redirect(w, r, "/chapter/"+cname+"/"+qid, http.StatusSeeOther)
}
