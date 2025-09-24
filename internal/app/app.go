package app

import (
	"embed"
	"fmt"
	"html/template"
	"bytes"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
        if !ok { continue }
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
    if err := r.ParseForm(); err != nil {
        http.Error(w, "invalid form", http.StatusBadRequest)
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
        http.Error(w, "open chapter: "+err.Error(), http.StatusInternalServerError)
        return
    }
    v, err := snbt.Decode(f)
    f.Close()
    if err != nil {
        http.Error(w, "decode: "+err.Error(), http.StatusInternalServerError)
        return
    }
    m, ok := v.(map[string]any)
    if !ok {
        http.Error(w, "chapter not a compound", http.StatusInternalServerError)
        return
    }
    arr, ok := m["quests"].([]any)
    if !ok {
        http.Error(w, "chapter missing quests", http.StatusInternalServerError)
        return
    }
    // Find quest by id
    found := false
    for i := range arr {
        if qm, ok := arr[i].(map[string]any); ok {
            if id, _ := qm["id"].(string); id == qid {
                if title != "" { qm["title"] = title } else { delete(qm, "title") }
                if subtitle != "" { qm["subtitle"] = subtitle } else { delete(qm, "subtitle") }
                // description as list of strings split by lines
                dlines := strings.Split(desc, "\n")
                // trim trailing empty lines
                j := len(dlines)
                for j > 0 && strings.TrimSpace(dlines[j-1]) == "" { j-- }
                dlines = dlines[:j]
                if len(dlines) > 0 {
                    dl := make([]any, 0, len(dlines))
                    for _, s := range dlines { dl = append(dl, s) }
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
        http.Error(w, "quest not found", http.StatusNotFound)
        return
    }
    m["quests"] = arr
    // Encode back to file
    var buf bytes.Buffer
    if err := snbt.Encode(&buf, m); err != nil {
        http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
        return
    }
    if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
        http.Error(w, "write: "+err.Error(), http.StatusInternalServerError)
        return
    }
    // Refresh in-memory data
    if err := a.scan(); err != nil {
        log.Printf("rescan error: %v", err)
    }
    // Redirect back to quest detail
    http.Redirect(w, r, "/chapter/"+cname+"/"+qid, http.StatusSeeOther)
}
