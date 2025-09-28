package app

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
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
	QB        *QuestBook
	tpl       *template.Template
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
	// XXX: maybe if we error we still have the app UI visible?
	a.QB, _ = NewQuestBook(root)

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

// reload questbook from disk
func (a *App) reload() { a.QB, _ = NewQuestBook(a.Root) }

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
	r.Get("/colors/", a.colors)
	r.Post("/colors/recolor", a.colorsRecolor)
	r.Post("/colors/recolor_one", a.colorsRecolorOne)
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
	// Derive sidebar data from QuestBook
	var chapters []Chapter
	for _, cp := range a.QB.Chapters {
		if cp != nil {
			chapters = append(chapters, *cp)
		}
	}
	var groups []Group
	for _, gp := range a.QB.Groups {
		if gp != nil {
			groups = append(groups, *gp)
		}
	}
	top := a.QB.TopItems()
	return map[string]any{
		"Chapters":    chapters,
		"Groups":      groups,
		"Top":         top,
		"MCVersion":   a.MCVersion,
		"Title":       title,
		"Parsed":      len(a.QB.Chapters),
		"Failed":      0,
		"HasFailures": false,
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
	for _, g := range a.QB.Groups {
		if g.Title != "" {
			cgOptions = append(cgOptions, g.Title)
		}
	}
	for _, ch := range a.QB.Chapters {
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
	idsParam := strings.TrimSpace(r.URL.Query().Get("ids"))
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
		for _, g := range a.QB.Groups {
			if strings.Contains(strings.ToLower(g.Title), lc) || strings.EqualFold(g.ID, cg) {
				for _, ch := range g.Chapters {
					scope[ch.Name] = true
				}
			}
		}
		for _, ch := range a.QB.Chapters {
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
	if idsParam != "" {
		idset := make(map[string]struct{})
		for _, s := range strings.Split(idsParam, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				idset[s] = struct{}{}
			}
		}
		for _, ch := range a.QB.Chapters {
			for _, qs := range ch.Quests {
				if _, ok := idset[qs.ID]; ok {
					matches = append(matches, QRef{Chapter: ch, Quest: qs})
				}
			}
		}
	} else {
		for _, ch := range a.QB.Chapters {
			if len(scope) > 0 && !scope[ch.Name] {
				continue
			}
			for _, qs := range ch.Quests {
				if noTitle && qs.Title != "" {
					continue
				}
				if noSubtitle && qs.Subtitle != "" {
					continue
				}
				if noDesc && qs.Description != "" {
					continue
				}
				if !matchQuest(qs, terms, caseSensitive) {
					continue
				}
				matches = append(matches, QRef{Chapter: ch, Quest: qs})
			}
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
	for _, g := range a.QB.Groups {
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
		for _, ch := range a.QB.Chapters {
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
		"ids":  idsParam,
		"n":    perPage,
	}
	a.render(w, "batch_edit.gohtml", data)
}

// colors handles GET "/colors/" — Color Manager base with an inconsistency finder.
func (a *App) colors(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	cg := strings.TrimSpace(r.URL.Query().Get("cg"))
	ci := r.URL.Query().Has("ci") // case-insensitive if present
	// Per-page selector for visual consistency (not used for aggregation yet)
	perPage := 5
	if n := strings.TrimSpace(r.URL.Query().Get("n")); n != "" {
		switch n {
		case "10":
			perPage = 10
		case "20":
			perPage = 20
		}
	}

	data := a.baseData(r, "Color Manager")
	// Datalist options
	var cgOptions []string
	for _, g := range a.QB.Groups {
		if g.Title != "" {
			cgOptions = append(cgOptions, g.Title)
		}
	}
	for _, ch := range a.QB.Chapters {
		if ch.Title != "" {
			cgOptions = append(cgOptions, ch.Title)
		}
	}
	data["CGOptions"] = cgOptions
	data["Form"] = map[string]any{"cg": cg, "q": term, "ci": ci, "n": perPage}

	if term == "" {
		a.render(w, "colors.gohtml", data)
		return
	}

	// Scope selection
	scope := make(map[string]bool)
	if cg != "" {
		lc := strings.ToLower(cg)
		for _, g := range a.QB.Groups {
			if strings.Contains(strings.ToLower(g.Title), lc) || strings.EqualFold(g.ID, cg) {
				for _, ch := range g.Chapters {
					scope[ch.Name] = true
				}
			}
		}
		for _, ch := range a.QB.Chapters {
			if strings.Contains(strings.ToLower(ch.Title), lc) || strings.EqualFold(ch.Name, cg) {
				scope[ch.Name] = true
			}
		}
	}

	// Normalization
	matchTerm := term
	if ci {
		matchTerm = strings.ToLower(term)
	}

	// Count colors and capture quest ids for linking
	counts := make(map[string]int)                     // code -> count (code like "c6", "ca", empty for none)
	idsByColor := make(map[string]map[string]struct{}) // code -> set of quest IDs
	// Per-quest aggregated matches with highlighted segment text
	type TermHit struct {
		Code  string
		Seg   string
		Field string // title|subtitle|description
		DIdx  int    // description line index if Field==description; else -1
		Pos   int    // match position (visible rune index)
	}
	type QuestHit struct {
		Chapter, QID, Title string
		Hits                []TermHit
	}
	hitsByQuest := make(map[string]*QuestHit)
	process := func(chapter, qid, qtitle, s string, field string, didx int) {
		if s == "" {
			return
		}
		cur := ""
		var stripped []rune
		var colors []string
		var srcIdx []int
		rs := []rune(s)
		i := 0
		for i < len(rs) {
			rch := rs[i]
			if rch == '&' || rch == '\u00A7' {
				if i+1 < len(rs) {
					code := rs[i+1]
					if (code >= '0' && code <= '9') || (code >= 'a' && code <= 'f') || (code >= 'A' && code <= 'F') {
						if code >= 'A' && code <= 'F' {
							code = code - 'A' + 'a'
						}
						cur = "c" + string(code)
					} else if code == 'r' || code == 'R' {
						cur = ""
					}
					i += 2
					continue
				}
			}
			stripped = append(stripped, rch)
			colors = append(colors, cur)
			srcIdx = append(srcIdx, i)
			i++
		}
		text := string(stripped)
		hay := text
		needle := matchTerm
		if ci {
			hay = strings.ToLower(text)
		}
		if len(needle) == 0 {
			return
		}
		start := 0
		for start <= len(hay)-len(needle) {
			idx := strings.Index(hay[start:], needle)
			if idx < 0 {
				break
			}
			pos := start + idx
			if pos < len(colors) {
				c := colors[pos]
				counts[c]++
				if idsByColor[c] == nil {
					idsByColor[c] = make(map[string]struct{})
				}
				idsByColor[c][qid] = struct{}{}
				// Build highlighted segment:
				// - If a color code is active at the match, capture text between the
				//   prior color code and the next code (original behavior).
				// - If no color code is active, capture surrounding context: ~3 words
				//   on either side from the stripped text.
				var seg string
				if c != "" {
					src := srcIdx[pos]
					// previous '&' that begins a code
					prev := -1
					for p := src; p >= 0; p-- {
						if rs[p] == '&' || rs[p] == '\u00A7' {
							if p+1 < len(rs) {
								cc := rs[p+1]
								if (cc >= '0' && cc <= '9') || (cc >= 'a' && cc <= 'f') || (cc >= 'A' && cc <= 'F') || cc == 'r' || cc == 'R' {
									prev = p
									break
								}
							}
						}
					}
					// next '&' occurrence
					next := len(rs)
					for q := src + 1; q < len(rs); q++ {
						if rs[q] == '&' || rs[q] == '\u00A7' {
							next = q
							break
						}
					}
					// Extract visible characters excluding codes
					var vis []rune
					startVis := src
					if prev >= 0 {
						startVis = prev + 2
					}
					for q := startVis; q < next && q < len(rs); q++ {
						if rs[q] == '&' || rs[q] == '\u00A7' {
							q++
							continue
						}
						vis = append(vis, rs[q])
					}
					seg = string(vis)
				} else {
					// No active color: include ~3 words of context on either side from stripped text
					// Work with the original-cased stripped text (text) while using indexes from hay.
					// Helper to detect simple whitespace.
					isSpace := func(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
					bt := []byte(text)
					// Determine the corresponding start index in 'text' for this match
					// pos refers to index in 'hay'; for ASCII it aligns with 'text'.
					left := pos
					// Move left over up to 3 words
					words := 0
					for left > 0 && words < 3 {
						// skip any spaces immediately to the left
						for left > 0 && isSpace(bt[left-1]) {
							left--
						}
						// scan left through the previous word
						saw := false
						for left > 0 && !isSpace(bt[left-1]) {
							left--
							saw = true
						}
						if saw {
							words++
						} else {
							break
						}
					}
					// Right bound starting after the needle
					right := pos + len(needle)
					words = 0
					for right < len(bt) && words < 3 {
						// skip spaces to the right
						for right < len(bt) && isSpace(bt[right]) {
							right++
						}
						// scan through next word
						saw := false
						for right < len(bt) && !isSpace(bt[right]) {
							right++
							saw = true
						}
						if saw {
							words++
						} else {
							break
						}
					}
					if left < 0 {
						left = 0
					}
					if right > len(bt) {
						right = len(bt)
					}
					// Add ellipses only when context is truncated on that side
					prefix := left > 0
					suffix := right < len(bt)
					segStr := strings.TrimSpace(string(bt[left:right]))
					if prefix {
						segStr = "…" + segStr
					}
					if suffix {
						segStr = segStr + "…"
					}
					seg = segStr
				}
				qh := hitsByQuest[qid]
				if qh == nil {
					qh = &QuestHit{Chapter: chapter, QID: qid, Title: qtitle}
					hitsByQuest[qid] = qh
				}
				qh.Hits = append(qh.Hits, TermHit{Code: c, Seg: seg, Field: field, DIdx: didx, Pos: pos})
			}
			start = pos + len(needle)
		}
	}

	for _, ch := range a.QB.Chapters {
		if len(scope) > 0 && !scope[ch.Name] {
			continue
		}
		for _, qs := range ch.Quests {
			ttl := qs.GetTitle()
			process(ch.Name, qs.ID, ttl, qs.Title, "title", -1)
			process(ch.Name, qs.ID, ttl, qs.Subtitle, "subtitle", -1)
			// Handle description per raw line when available for precise targeting
			var qm = qs.raw
			if dl, ok := qm["description"].([]any); ok {
				for di := range dl {
					if s, ok := dl[di].(string); ok {
						process(ch.Name, qs.ID, ttl, s, "description", di)
					}
				}
			} else if s, ok := qm["description"].(string); ok {
				process(ch.Name, qs.ID, ttl, s, "description", -1)
			} else {
				// fallback to joined string held in struct
				if qs.Description != "" {
					process(ch.Name, qs.ID, ttl, qs.Description, "description", -1)
				}
			}
		}
	}

	type ColorCount struct {
		Code  string
		Count int
		IDs   string
	}
	var res []ColorCount
	for code, n := range counts {
		// Flatten ids in chapter order
		var ids []string
		if set := idsByColor[code]; set != nil {
			for _, ch := range a.QB.Chapters {
				for j := range ch.Quests {
					if _, ok := set[ch.Quests[j].ID]; ok {
						ids = append(ids, ch.Quests[j].ID)
					}
				}
			}
		}
		res = append(res, ColorCount{Code: code, Count: n, IDs: strings.Join(ids, ",")})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Count > res[j].Count })
	data["ColorResults"] = res
	data["Term"] = term
	// Build ordered per-quest results (one line per quest, dedup hits per quest)
	type QuestLine struct {
		Chapter, QID, Title string
		Hits                []TermHit
	}
	var qlines []QuestLine
	for _, ch := range a.QB.Chapters {
		if len(scope) > 0 && !scope[ch.Name] {
			continue
		}
		for _, qs := range ch.Quests {
			if qh := hitsByQuest[qs.ID]; qh != nil {
				seen := make(map[string]struct{})
				compact := make([]TermHit, 0, len(qh.Hits))
				for _, h := range qh.Hits {
					key := h.Field + "#" + strconv.Itoa(h.DIdx) + "#" + strconv.Itoa(h.Pos) + "#" + h.Code + "\x00" + h.Seg
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					compact = append(compact, h)
				}
				qlines = append(qlines, QuestLine{Chapter: qh.Chapter, QID: qh.QID, Title: qh.Title, Hits: compact})
			}
		}
	}
	data["QuestResults"] = qlines
	a.render(w, "colors.gohtml", data)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, isAjax bool, msg string, code int) {
	if isAjax {
		writeJSON(w, code, map[string]any{"ok": false, "erorr": msg})
		return
	}
	http.Error(w, msg, code)
}

// colorsRecolor handles POST /colors/recolor. It applies a color code to all
// occurrences of a term within the specified quest IDs, then rescans data.
func (a *App) colorsRecolor(w http.ResponseWriter, r *http.Request) {
	isAjax := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, isAjax, "invalid form", http.StatusBadRequest)
		return
	}
	term := strings.TrimSpace(r.Form.Get("term"))
	idsParam := strings.TrimSpace(r.Form.Get("ids"))
	color := strings.TrimSpace(r.Form.Get("color"))
	ci := r.Form.Get("ci") == "1" || strings.EqualFold(r.Form.Get("ci"), "true")
	if term == "" || idsParam == "" || len(color) != 1 {
		writeError(w, isAjax, "missing term/ids/color", http.StatusBadRequest)
		return
	}
	c := color[0]
	if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
		writeError(w, isAjax, "invalid color", http.StatusBadRequest)
		return
	}
	if c >= 'A' && c <= 'F' {
		c = c - 'A' + 'a'
	}

	// Build index questID -> chapter name
	type target struct {
		Chapter string
		ID      string
	}
	idset := make(map[string]struct{})
	var targets []target
	for _, id := range strings.Split(idsParam, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		idset[id] = struct{}{}
	}
	for _, ch := range a.QB.Chapters {
		for _, qs := range ch.Quests {
			if _, ok := idset[qs.ID]; ok {
				targets = append(targets, target{Chapter: ch.Name, ID: qs.ID})
			}
		}
	}
	if len(targets) == 0 {
		writeError(w, isAjax, "no matching quests", http.StatusNotFound)
		return
	}

	// Group targets by chapter and update files
	byChapter := make(map[string]map[string]struct{})
	for _, t := range targets {
		if byChapter[t.Chapter] == nil {
			byChapter[t.Chapter] = make(map[string]struct{})
		}
		byChapter[t.Chapter][t.ID] = struct{}{}
	}

	for cname, qids := range byChapter {
		path := filepath.Join(a.Root, "quests", "chapters", cname+".snbt")
		f, err := os.Open(path)
		if err != nil {
			writeError(w, isAjax, "open: "+err.Error(), http.StatusInternalServerError)
			return
		}
		v, err := snbt.Decode(f)
		f.Close()
		if err != nil {
			writeError(w, isAjax, "decode: "+err.Error(), http.StatusInternalServerError)
			return
		}
		m, ok := v.(map[string]any)
		if !ok {
			writeError(w, isAjax, "chapter not a compound", http.StatusInternalServerError)
			return
		}
		arr, ok := m["quests"].([]any)
		if !ok {
			writeError(w, isAjax, "chapter missing quests", http.StatusInternalServerError)
			return
		}
		// update any matching quests
		for i := range arr {
			qm, ok := arr[i].(map[string]any)
			if !ok {
				continue
			}
			id, _ := qm["id"].(string)
			if _, ok := qids[id]; !ok {
				continue
			}
			// fields: title, subtitle, description (list of strings or string)
			if s, ok := qm["title"].(string); ok {
				qm["title"] = recolorString(s, term, c, ci)
			}
			if s, ok := qm["subtitle"].(string); ok {
				qm["subtitle"] = recolorString(s, term, c, ci)
			}
			if dl, ok := qm["description"].([]any); ok {
				for j := range dl {
					if s, ok2 := dl[j].(string); ok2 {
						dl[j] = recolorString(s, term, c, ci)
					}
				}
				qm["description"] = dl
			} else if s, ok := qm["description"].(string); ok {
				qm["description"] = recolorString(s, term, c, ci)
			}
			arr[i] = qm
		}
		m["quests"] = arr
		var buf bytes.Buffer
		if err := snbt.Encode(&buf, m); err != nil {
			writeError(w, isAjax, "encode: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			writeError(w, isAjax, "write: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// refresh in-memory data
	a.reload()
	if isAjax {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// colorsRecolorOne handles POST /colors/recolor_one to recolor a single occurrence
// of a term in a specific quest field.
func (a *App) colorsRecolorOne(w http.ResponseWriter, r *http.Request) {
	isAjax := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		slog.Error("error parsing multipart form", "error", err)
		writeError(w, isAjax, "invalid form", http.StatusBadRequest)
		return
	}

	qid := strings.TrimSpace(r.Form.Get("qid"))
	term := strings.TrimSpace(r.Form.Get("term"))
	field := strings.TrimSpace(r.Form.Get("field")) // title|subtitle|description
	didxStr := strings.TrimSpace(r.Form.Get("didx"))
	posStr := strings.TrimSpace(r.Form.Get("pos"))
	color := strings.TrimSpace(r.Form.Get("color"))
	ci := r.Form.Get("ci") == "1" || strings.EqualFold(r.Form.Get("ci"), "true")

	if qid == "" || term == "" || field == "" || posStr == "" || len(color) != 1 {
		writeError(w, isAjax, "missing params", http.StatusBadRequest)
		return
	}
	c := color[0]
	if c >= 'A' && c <= 'F' {
		c = c - 'A' + 'a'
	}
	if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
		writeError(w, isAjax, "invalid color", http.StatusBadRequest)
		return
	}
	didx := -1
	if didxStr != "" {
		if n, err := strconv.Atoi(didxStr); err == nil {
			didx = n
		}
	}
	pos, err := strconv.Atoi(posStr)
	if err != nil {
		writeError(w, isAjax, "bad pos", http.StatusBadRequest)
		return
	}

	// locate quest and chapter
	var ch *Chapter
	for _, c := range a.QB.Chapters {
		for j := range c.Quests {
			if c.Quests[j].ID == qid {
				ch = c
				break
			}
		}
		if ch != nil {
			break
		}
	}
	if ch == nil {
		writeError(w, isAjax, "quest not found", http.StatusNotFound)
		return
	}

	path := filepath.Join(a.Root, "quests", "chapters", ch.Name+".snbt")
	f, err := os.Open(path)
	if err != nil {
		writeError(w, isAjax, "open: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v, err := snbt.Decode(f)
	f.Close()
	if err != nil {
		writeError(w, isAjax, "decode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	m, ok := v.(map[string]any)
	if !ok {
		writeError(w, isAjax, "chapter not a compound", http.StatusInternalServerError)
		return
	}
	arr, ok := m["quests"].([]any)
	if !ok {
		writeError(w, isAjax, "chapter missing quests", http.StatusInternalServerError)
		return
	}

	// update one quest/field occurrence
	for i := range arr {
		qm, ok := arr[i].(map[string]any)
		if !ok {
			continue
		}
		if id, _ := qm["id"].(string); id != qid {
			continue
		}
		// helper to update a string field in qm
		updateField := func(key string, s string) {
			if s == "" {
				return
			}
			qm[key] = recolorOne(s, term, c, ci, pos)
		}
		switch field {
		case "title":
			if s, ok := qm["title"].(string); ok {
				updateField("title", s)
			}
		case "subtitle":
			if s, ok := qm["subtitle"].(string); ok {
				updateField("subtitle", s)
			}
		case "description":
			if dl, ok := qm["description"].([]any); ok {
				// Operate across the joined string; but apply to the one line where the match was detected if didx >= 0
				if didx >= 0 && didx < len(dl) {
					if s, ok := dl[didx].(string); ok {
						dl[didx] = recolorOne(s, term, c, ci, pos)
					}
					qm["description"] = dl
				} else {
					// fallback: join all lines and operate once (rare)
					// Not ideal, but keeps behavior consistent if we didn't track didx
				}
			} else if s, ok := qm["description"].(string); ok {
				updateField("description", s)
			}
		}
		arr[i] = qm
		break
	}
	m["quests"] = arr
	var buf bytes.Buffer
	if err := snbt.Encode(&buf, m); err != nil {
		writeError(w, isAjax, "encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		writeError(w, isAjax, "write: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.reload()
	if isAjax {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// recolorOne modifies only the specific match at targetPos (in stripped text index).
// If a color is active for that match, it replaces the color code as in recolorString.
// If no color is active, wraps the term in &<color> and &r.
func recolorOne(s, term string, color byte, ci bool, targetPos int) string {
	if s == "" || term == "" {
		return s
	}
	rs := []rune(s)
	var stripped []rune
	var colorsAt []string
	var srcIdx []int
	var codeIdxAt []int // index of color code char after '&' if active
	cur := ""
	lastCodeIdx := -1
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '&' || r == '\u00A7' {
			if i+1 < len(rs) {
				code := rs[i+1]
				if (code >= '0' && code <= '9') || (code >= 'a' && code <= 'f') || (code >= 'A' && code <= 'F') {
					if code >= 'A' && code <= 'F' {
						code = code - 'A' + 'a'
					}
					cur = "c" + string(code)
					lastCodeIdx = i + 1
				} else if code == 'r' || code == 'R' {
					cur = ""
					lastCodeIdx = -1
				}
				i++
				continue
			}
		}
		stripped = append(stripped, r)
		colorsAt = append(colorsAt, cur)
		srcIdx = append(srcIdx, i)
		codeIdxAt = append(codeIdxAt, lastCodeIdx)
	}
	hay := string(stripped)
	needle := term
	if ci {
		hay = strings.ToLower(hay)
		needle = strings.ToLower(term)
	}
	if len(needle) == 0 || len(hay) < len(needle) {
		return s
	}
	start := 0
	for start <= len(hay)-len(needle) {
		idx := strings.Index(hay[start:], needle)
		if idx < 0 {
			break
		}
		pos := start + idx
		if pos == targetPos {
			// perform change
			if colorsAt[pos] != "" {
				// replace existing color code
				codeIdx := codeIdxAt[pos]
				if codeIdx >= 0 && codeIdx < len(rs) {
					rs[codeIdx] = rune(color)
				}
				return string(rs)
			}
			// no active color: wrap the term only
			startSrc := srcIdx[pos]
			endSrc := srcIdx[pos+len(needle)-1]
			injectBefore := map[int]string{startSrc: "&" + string(color)}
			injectAfter := map[int]string{endSrc: "&r"}
			var out []rune
			for i := 0; i < len(rs); i++ {
				if code, ok := injectBefore[i]; ok {
					out = append(out, []rune(code)...)
				}
				out = append(out, rs[i])
				if code, ok := injectAfter[i]; ok {
					out = append(out, []rune(code)...)
				}
			}
			return string(out)
		}
		start = pos + len(needle)
	}
	return s
}

// recolorString replaces the color code that applies to each occurrence of term
// with the new color. It does not insert surrounding color/reset codes.
// If no color code is active for a matched term, the string is left unchanged
// for that occurrence (to avoid coloring unintended spans).
func recolorString(s, term string, color byte, ci bool) string {
	if s == "" || term == "" {
		return s
	}
	rs := []rune(s)
	// Build stripped text and mappings
	var stripped []rune
	var srcIdx []int
	var colorCodeIdxAt []int
	lastColorIdx := -1
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '&' || r == '\u00A7' {
			if i+1 < len(rs) {
				code := rs[i+1]
				if (code >= '0' && code <= '9') || (code >= 'a' && code <= 'f') || (code >= 'A' && code <= 'F') {
					if code >= 'A' && code <= 'F' {
						code = code - 'A' + 'a'
					}
					lastColorIdx = i + 1
				} else if code == 'r' || code == 'R' {
					lastColorIdx = -1
				}
				i++
				continue
			}
		}
		stripped = append(stripped, r)
		srcIdx = append(srcIdx, i)
		colorCodeIdxAt = append(colorCodeIdxAt, lastColorIdx)
	}
	hay := string(stripped)
	needle := term
	if ci {
		hay = strings.ToLower(hay)
		needle = strings.ToLower(term)
	}
	if len(needle) == 0 || len(hay) < len(needle) {
		return s
	}
	injectBefore := make(map[int]string)
	injectAfter := make(map[int]string)
	modified := false
	for start := 0; start <= len(hay)-len(needle); {
		idx := strings.Index(hay[start:], needle)
		if idx < 0 {
			break
		}
		pos := start + idx
		end := pos + len(needle) - 1
		if pos < len(srcIdx) {
			if codeIdx := colorCodeIdxAt[pos]; codeIdx >= 0 {
				rs[codeIdx] = rune(color)
				modified = true
			} else {
				injectBefore[srcIdx[pos]] = "&" + string(color)
				injectAfter[srcIdx[end]] = "&r"
				modified = true
			}
		}
		start = pos + len(needle)
	}
	if !modified {
		return s
	}
	var out []rune
	for i := 0; i < len(rs); i++ {
		if code, ok := injectBefore[i]; ok {
			out = append(out, []rune(code)...)
		}
		out = append(out, rs[i])
		if code, ok := injectAfter[i]; ok {
			out = append(out, []rune(code)...)
		}
	}
	return string(out)
}

// chapterDetail handles GET "/chapter/{chapter}".
func (a *App) chapterDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "chapter")
	ch, _ := a.QB.chapterMap[name]
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
	data["Failures"] = nil
	a.render(w, "errors.gohtml", data)
}

// chapterRaw handles GET "/chapter/{chapter}/raw".
func (a *App) chapterRaw(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "chapter")

	ch, _ := a.QB.chapterMap[name]
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

	ch, _ := a.QB.chapterMap[cname]
	q, _ := a.QB.questMap[qid]
	if ch == nil || q == nil {
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

	if err := r.ParseMultipartForm(2 << 20); err != nil {
		// the normal editor submits a non-multipart form, so lets try
		// to parse that if it isn't actually multipart
		if err == http.ErrNotMultipart {
			err = r.ParseForm()
		}
		if err != nil {
			writeError(w, isAjax, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	cname := chi.URLParam(r, "chapter")
	qid := chi.URLParam(r, "quest")
	title := strings.TrimSpace(r.Form.Get("title"))
	subtitle := strings.TrimSpace(r.Form.Get("subtitle"))
	desc := r.Form.Get("description")

	slog.Debug("saving quest", "chapter", cname, "quest", qid,
		"title", title, "subtitle", subtitle, "desc", desc)

	// it makes sense to re-read the chapter from disk before saving as
	// edits to other quests from elsewhere could be lost if we don't
	path := filepath.Join(a.Root, "quests", "chapters", cname+".snbt")

	chapter, err := NewChapterFromPath(path)
	if err != nil {
		writeError(w, isAjax, "open chapter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	quest, ok := chapter.questMap[qid]
	if !ok {
		writeError(w, isAjax, "quest not found", http.StatusNotFound)
		return
	}

	quest.Title = title
	quest.Subtitle = subtitle
	quest.Description = desc

	if err := chapter.Save(path); err != nil {
		writeError(w, isAjax, "saving chapter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh in-memory data
	a.reload()

	if isAjax {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	// Redirect back to quest detail
	http.Redirect(w, r, "/chapter/"+cname+"/"+qid, http.StatusSeeOther)
}
