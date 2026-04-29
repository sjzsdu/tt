package cmd

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	mdutil "tt/internal/mdutil"
)

var (
	skillPort      = 9695
	skillFile      string
	skillRootFlag  string
	skillEditStart bool

	skillServer *http.Server
	skillMu     sync.Mutex
	skillRoot   string
)

var skillCmd = &cobra.Command{
	Use:   "skill [path]",
	Short: "Browse and edit skill markdown files",
	Long:  "Start a local web UI for skill files. It extracts frontmatter, renders the remaining markdown body, and supports editing plus saving the full document.",
	Args:  cobra.MaximumNArgs(1),
	Example: `tt skill
	tt skill create-cmd
	tt skill --file .forge/skills/create-cmd/SKILL.md --edit`,
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}

		root := projectRootFromConfig(loaded)
		if strings.TrimSpace(skillRootFlag) != "" {
			root = skillRootFlag
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolve skill root failed: %w", err)
		}
		if fi, err := os.Stat(absRoot); err != nil || !fi.IsDir() {
			return fmt.Errorf("skill root not found: %s", absRoot)
		}
		skillRoot = absRoot

		if len(args) > 0 && strings.TrimSpace(skillFile) == "" {
			candidate := args[0]
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(skillRoot, candidate)
			}
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				absDir, err := filepath.Abs(candidate)
				if err != nil {
					return fmt.Errorf("resolve skill directory failed: %w", err)
				}
				skillRoot = absDir
			} else {
				skillFile = args[0]
			}
		}

		return runSkillServer()
	},
}

func init() {
	rootCmd.AddCommand(skillCmd)
	skillCmd.Flags().IntVarP(&skillPort, "port", "p", 9695, "service port")
	skillCmd.Flags().StringVarP(&skillFile, "file", "f", "", "open a specific skill markdown file")
	skillCmd.Flags().StringVar(&skillRootFlag, "root", "", "override the skill root directory")
	skillCmd.Flags().BoolVar(&skillEditStart, "edit", false, "open the current document in edit mode by default")
}

func runSkillServer() error {
	skillMu.Lock()
	defer skillMu.Unlock()

	if skillServer != nil {
		return fmt.Errorf("skill service already running on port %d", skillPort)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleSkillIndex)
	mux.HandleFunc("/list", handleSkillList)
	mux.HandleFunc("/view/", handleSkillView)
	mux.HandleFunc("/edit/", handleSkillEdit)
	mux.HandleFunc("/raw/", handleSkillRaw)
	mux.HandleFunc("/save/", handleSkillSave)

	maxPort := skillPort + 20
	var lastErr error
	for port := skillPort; port <= maxPort; port++ {
		skillServer = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		serverErr := make(chan error, 1)
		go func() {
			err := skillServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()

		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-serverErr:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				skillServer = nil
				continue
			}
			skillServer = nil
			return err
		default:
			skillPort = port
			fmt.Printf("Skill service started: http://localhost:%d\n", port)
			go openSkillBrowser(fmt.Sprintf("http://localhost:%d", port))
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt)
			<-quit
			fmt.Println("\nShutting down skill service...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := skillServer.Shutdown(ctx)
			skillServer = nil
			return err
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func handleSkillIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if strings.TrimSpace(skillFile) != "" {
		path := strings.TrimSpace(skillFile)
		if !filepath.IsAbs(path) {
			path = filepath.Join(skillRoot, path)
		}
		rel, err := filepath.Rel(skillRoot, path)
		if err == nil {
			http.Redirect(w, r, "/view/"+filepath.ToSlash(rel), http.StatusFound)
			return
		}
	}
	handleSkillList(w, r)
}

func handleSkillList(w http.ResponseWriter, r *http.Request) {
	files, err := collectSkillFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect skill files failed: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Files []skillEntry
		Total int
	}{Files: files, Total: len(files)}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("skill-list").Parse(skillListHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render skill list failed: %v", err), http.StatusInternalServerError)
	}
}

func handleSkillView(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/view/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	content, filePath, err := resolveSkillContent(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	files, err := collectSkillFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect skill files failed: %v", err), http.StatusInternalServerError)
		return
	}

	doc := parseSkillDocument(content)
	rel, _ := filepath.Rel(skillRoot, filePath)
	data := skillViewData{
		FilePath:       "/" + filepath.ToSlash(rel),
		Title:          skillTitle(doc, rel),
		Subtitle:       filePath,
		Frontmatter:    doc.Fields,
		FrontmatterRaw: doc.Frontmatter,
		Body:           doc.Body,
		RawContent:     content,
		Files:          files,
		RawPath:        "/raw/" + filepath.ToSlash(rel),
		EditPath:       "/edit/" + filepath.ToSlash(rel),
		SavePath:       "/save/" + filepath.ToSlash(rel),
		EditMode:       skillEditStart,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("skill-view").Parse(skillViewHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render skill view failed: %v", err), http.StatusInternalServerError)
	}
}

func handleSkillEdit(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/edit/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	content, filePath, err := resolveSkillContent(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	files, err := collectSkillFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect skill files failed: %v", err), http.StatusInternalServerError)
		return
	}
	doc := parseSkillDocument(content)
	rel, _ := filepath.Rel(skillRoot, filePath)
	data := skillViewData{
		FilePath:       "/" + filepath.ToSlash(rel),
		Title:          skillTitle(doc, rel),
		Subtitle:       filePath,
		Frontmatter:    doc.Fields,
		FrontmatterRaw: doc.Frontmatter,
		Body:           doc.Body,
		RawContent:     content,
		Files:          files,
		RawPath:        "/raw/" + filepath.ToSlash(rel),
		EditPath:       "/edit/" + filepath.ToSlash(rel),
		SavePath:       "/save/" + filepath.ToSlash(rel),
		EditMode:       true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("skill-view").Parse(skillViewHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render skill edit failed: %v", err), http.StatusInternalServerError)
	}
}

func handleSkillRaw(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/raw/")
	filePath, err := resolveSkillPath(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read skill file failed: %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(filePath)))
	_, _ = w.Write(content)
}

func handleSkillSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	relPath := strings.TrimPrefix(r.URL.Path, "/save/")
	filePath, err := resolveSkillPath(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("parse form failed: %v", err), http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("save skill file failed: %v", err), http.StatusInternalServerError)
		return
	}
	if r.FormValue("return") == "edit" {
		http.Redirect(w, r, "/edit/"+filepath.ToSlash(relPath), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/view/"+filepath.ToSlash(relPath), http.StatusFound)
}

type skillEntry struct {
	Path        string
	Name        string
	Relative    string
	Title       string
	Description string
	Size        int64
}
type skillDocument struct {
	Frontmatter string
	Body        string
	Fields      []skillField
}

type skillField struct {
	Key   string
	Value string
}

type skillViewData struct {
	FilePath       string
	Title          string
	Subtitle       string
	RawPath        string
	EditPath       string
	SavePath       string
	Frontmatter    []skillField
	FrontmatterRaw string
	Body           string
	RawContent     string
	Files          []skillEntry
	EditMode       bool
}

func collectSkillFiles() ([]skillEntry, error) {
	files := make([]skillEntry, 0)
	err := filepath.WalkDir(skillRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipSkillDir(path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSkillFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(skillRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		content, _ := os.ReadFile(path)
		doc := parseSkillDocument(string(content))
		info, _ := d.Info()
		size := int64(len(content))
		if info != nil {
			size = info.Size()
		}
		files = append(files, skillEntry{
			Path:        path,
			Name:        d.Name(),
			Relative:    "/" + rel,
			Title:       skillTitle(doc, rel),
			Description: skillDescription(doc),
			Size:        size,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	return files, nil
}

func resolveSkillContent(relPath string) (string, string, error) {
	filePath, err := resolveSkillPath(relPath)
	if err != nil {
		return "", "", err
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("skill file not found: %w", err)
	}
	return string(content), filePath, nil
}

func resolveSkillPath(relPath string) (string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", fmt.Errorf("skill path is required")
	}
	if !filepath.IsAbs(trimmed) {
		trimmed = filepath.Join(skillRoot, trimmed)
	}
	cleaned := filepath.Clean(trimmed)
	if info, err := os.Stat(cleaned); err == nil {
		if info.IsDir() {
			candidate := filepath.Join(cleaned, "SKILL.md")
			if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
				return candidate, nil
			}
		}
		if strings.EqualFold(filepath.Base(cleaned), "SKILL.md") {
			return cleaned, nil
		}
		return "", fmt.Errorf("skill file must be SKILL.md: %s", cleaned)
	}
	if !strings.EqualFold(filepath.Base(cleaned), "SKILL.md") {
		candidate := filepath.Join(cleaned, "SKILL.md")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
		return "", fmt.Errorf("skill file must be SKILL.md: %s", cleaned)
	}
	return "", fmt.Errorf("skill file not found: %s", cleaned)
}

func parseSkillDocument(content string) skillDocument {
	loadedDoc := mdutil.SplitDocument(content)
	doc := skillDocument{Frontmatter: loadedDoc.Frontmatter, Body: loadedDoc.Body}
	if strings.TrimSpace(doc.Frontmatter) == "" {
		return doc
	}
	raw, err := mdutil.ParseYAMLFrontmatter(doc.Frontmatter)
	if err != nil {
		doc.Fields = []skillField{{Key: "frontmatter_error", Value: err.Error()}}
		return doc
	}
	doc.Fields = flattenYAMLFields(raw)
	return doc
}

func flattenYAMLFields(value any) []skillField {
	fields := make([]skillField, 0)
	flattenYAMLInto("", value, func(key string, val any) {
		fields = append(fields, skillField{Key: key, Value: formatYAMLValue(val)})
	})
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	return fields
}

func flattenYAMLInto(prefix string, value any, emit func(string, any)) {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenYAMLInto(next, v[key], emit)
		}
	case map[any]any:
		keys := make([]string, 0, len(v))
		lookup := make(map[string]any, len(v))
		for key, val := range v {
			s := fmt.Sprint(key)
			keys = append(keys, s)
			lookup[s] = val
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenYAMLInto(next, lookup[key], emit)
		}
	case []any:
		if len(v) == 0 {
			if prefix == "" {
				prefix = "value"
			}
			emit(prefix, "[]")
			return
		}
		for i, item := range v {
			next := fmt.Sprintf("%s[%d]", prefix, i)
			if prefix == "" {
				next = fmt.Sprintf("[%d]", i)
			}
			flattenYAMLInto(next, item, emit)
		}
	default:
		if strings.TrimSpace(prefix) == "" {
			prefix = "value"
		}
		emit(prefix, v)
	}
}

func formatYAMLValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool, int, int64, float64, uint, uint64:
		return fmt.Sprint(v)
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func skillTitle(doc skillDocument, rel string) string {
	if title := skillFrontmatterString(doc.Frontmatter, doc.Fields, "name", "title"); strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	base := filepath.Base(filepath.Dir(rel))
	if base != "." && base != string(filepath.Separator) && strings.TrimSpace(base) != "" {
		return base
	}
	if strings.TrimSpace(rel) != "" {
		return filepath.Base(rel)
	}
	return "Untitled Skill"
}

func skillDescription(doc skillDocument) string {
	if desc := skillFrontmatterString(doc.Frontmatter, doc.Fields, "description", "summary"); strings.TrimSpace(desc) != "" {
		return strings.TrimSpace(desc)
	}
	return ""
}

func skillFrontmatterString(front string, fields []skillField, keys ...string) string {
	if strings.TrimSpace(front) == "" {
		return ""
	}
	raw, err := mdutil.ParseYAMLFrontmatter(front)
	if err != nil || raw == nil {
		return ""
	}
	data, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if v, ok := data[key]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	for _, field := range fields {
		for _, key := range keys {
			if strings.EqualFold(field.Key, key) && strings.TrimSpace(field.Value) != "" {
				return strings.TrimSpace(field.Value)
			}
		}
	}
	return ""
}

func isSkillFile(name string) bool {
	return strings.EqualFold(name, "SKILL.md")
}

func skipSkillDir(path, name string) bool {
	if path == skillRoot {
		return false
	}
	if name == ".git" || name == "node_modules" || name == "vendor" {
		return true
	}
	return false
}

func openSkillBrowser(url string) {
	cmd := exec.Command("open", url)
	if err := cmd.Run(); err != nil {
		fmt.Printf("open browser failed: %v\n", err)
	}
}

const skillListHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Skill</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #f6f8fb; color: #111827; }
    .wrap { max-width: 1100px; margin: 0 auto; padding: 32px 20px; }
    .card { background: #fff; border-radius: 16px; box-shadow: 0 10px 30px rgba(15,23,42,.08); padding: 24px; }
    .item { padding: 18px 0; border-bottom: 1px solid #e5e7eb; }
    .item:last-child { border-bottom: 0; }
    .title { margin: 0 0 8px; font-size: 19px; }
    .meta { color: #475467; font-size: 13px; margin-top: 8px; display:flex; gap:12px; flex-wrap:wrap; }
    .path { color: #667085; font-size: 13px; }
    a { color: #2563eb; text-decoration: none; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>Skill Files</h1>
      <p>Total: {{.Total}}</p>
      {{range .Files}}
      <div class="item">
        <h2 class="title"><a href="/view{{.Relative}}">{{.Title}}</a></h2>
        <div class="path">{{.Relative}}</div>
        {{if .Description}}<div class="path" style="margin-top:8px;">{{.Description}}</div>{{end}}
        <div class="meta">
          <span>{{.Size}} bytes</span>
          <a href="/raw{{.Relative}}">raw</a>
          <a href="/edit{{.Relative}}">edit</a>
        </div>
      </div>
      {{else}}
      <p>No skill files found.</p>
      {{end}}
    </div>
  </div>
</body>
</html>`

const skillViewHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/github-markdown-css@5.2.0/github-markdown.min.css">
  <script src="https://cdn.jsdelivr.net/npm/marked@9.1.2/marked.min.js"></script>
  <style>
    :root {
      --bg: #f6f8fb;
      --panel: #fff;
      --line: #e5e7eb;
      --text: #111827;
      --subtle: #667085;
      --blue: #2563eb;
      --blue-soft: #eff6ff;
      --green: #059669;
      --green-soft: #ecfdf3;
      --shadow: 0 10px 30px rgba(15,23,42,.08);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg); color: var(--text); }
    .layout { display:grid; grid-template-columns: 280px minmax(0,1fr); gap: 20px; padding: 20px; min-height: 100vh; }
    .side, .main { min-height: 0; }
    .side { background: var(--panel); border: 1px solid var(--line); border-radius: 18px; box-shadow: var(--shadow); overflow: auto; padding: 18px; }
    .main { overflow: auto; }
    .main-inner { max-width: 1040px; margin: 0 auto; }
    .hero, .panel { background: var(--panel); border: 1px solid var(--line); border-radius: 18px; box-shadow: var(--shadow); padding: 24px; margin-bottom: 18px; }
    .title { margin: 0; font-size: 22px; }
    .subtitle { color: var(--subtle); font-size: 13px; margin-top: 8px; word-break: break-all; }
    .toolbar { display:flex; gap:10px; flex-wrap:wrap; margin-top:16px; }
    .btn { display:inline-flex; align-items:center; justify-content:center; padding:10px 14px; border-radius:12px; text-decoration:none; font-weight:600; border:1px solid var(--line); color: var(--text); background:#fff; cursor:pointer; }
    .btn.primary { color:#fff; background:var(--blue); border-color:var(--blue); }
    .btn.secondary { color: var(--blue); background: var(--blue-soft); border-color: #bfdbfe; }
    .chip { display:inline-flex; align-items:center; padding:4px 10px; border-radius:999px; border:1px solid var(--line); font-size:12px; color: var(--subtle); background:#fff; }
    .meta-row { display:flex; gap:8px; flex-wrap:wrap; margin-top: 14px; }
    .fields { display:grid; gap: 12px; }
    .field { border:1px solid var(--line); border-radius:14px; background:#f8fafc; padding:12px; }
    .field-key { font-size:12px; font-weight:700; letter-spacing:.04em; text-transform:uppercase; color: var(--subtle); margin-bottom: 8px; }
    .field-value { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; white-space: pre-wrap; word-break: break-word; }
    .markdown-body.md-render {
      max-width: none;
      margin: 0;
      padding: 0;
      background: transparent;
      border: 0;
      box-shadow: none;
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, sans-serif;
    }
    .markdown-body.md-render > :first-child { margin-top: 0; }
    .markdown-body.md-render > :last-child { margin-bottom: 0; }
    .editor { display:none; gap: 12px; }
    .editor textarea {
      width: 100%;
      min-height: 60vh;
      resize: vertical;
      border: 1px solid var(--line);
      border-radius: 14px;
      padding: 16px;
      font: 14px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;
      color: var(--text);
      background: #fff;
    }
    .editor-hint { color: var(--subtle); font-size: 13px; line-height: 1.6; }
    .file-link { display:block; padding:10px 12px; border-radius:12px; color: var(--text); text-decoration:none; border:1px solid transparent; }
    .file-link:hover { background:#f8fafc; border-color:var(--line); }
    .file-title { font-weight:600; font-size:14px; }
    .file-meta { margin-top:6px; color:var(--subtle); font-size:12px; }
    .empty { color: var(--subtle); font-size: 13px; }
    .mode-edit .view-pane { display:none; }
    .mode-edit .editor { display:grid; }
    .mode-edit .btn[data-mode="edit"] { background: var(--blue); color: #fff; border-color: var(--blue); }
    .mode-view .editor { display:none; }
    .mode-view .view-pane { display:block; }
    .mode-view .btn[data-mode="view"] { background: var(--green); color: #fff; border-color: var(--green); }
    @media (max-width: 960px) {
      .layout { grid-template-columns: 1fr; }
      .side { order: 2; }
    }
  </style>
</head>
<body class="mode-{{if .EditMode}}edit{{else}}view{{end}}">
  <div class="layout">
    <aside class="side">
      <h2 style="margin-top:0">Skill Files</h2>
      {{range .Files}}
      <a class="file-link" href="/view{{.Relative}}">
        <div class="file-title">{{.Title}}</div>
        <div class="file-meta">{{.Relative}}</div>
      </a>
      {{else}}
      <div class="empty">No skill files found.</div>
      {{end}}
    </aside>
    <main class="main">
      <div class="main-inner">
        <section class="hero">
          <div class="title">{{.Title}}</div>
          <div class="subtitle">{{.Subtitle}}</div>
          <div class="toolbar">
            <a class="btn" href="/">File list</a>
            <a class="btn secondary" href="{{.RawPath}}">Raw</a>
            <a class="btn" href="{{.EditPath}}" data-mode="edit">Edit</a>
            <button class="btn primary" type="button" id="save-btn" data-mode="view">Save</button>
          </div>
          <div class="meta-row">
            <span class="chip">frontmatter: {{len .Frontmatter}}</span>
            <span class="chip">body markdown</span>
          </div>
        </section>

        <section class="panel view-pane">
          <h3 style="margin-top:0">Frontmatter</h3>
          {{if .Frontmatter}}
          <div class="fields">
            {{range .Frontmatter}}
            <div class="field">
              <div class="field-key">{{.Key}}</div>
              <div class="field-value">{{.Value}}</div>
            </div>
            {{end}}
          </div>
          {{else}}
          <div class="empty">No frontmatter detected.</div>
          {{end}}
        </section>

        <section class="panel view-pane">
          <h3 style="margin-top:0">Markdown Preview</h3>
          <article class="markdown-body md-render" id="preview"></article>
        </section>

        <section class="panel editor">
          <div class="editor-hint">Edit the full skill document below, including frontmatter. Saving will overwrite the current file.</div>
          <textarea id="editor">{{.RawContent}}</textarea>
        </section>
      </div>
    </main>
  </div>
  <script>
    marked.setOptions({ gfm: true, breaks: true, headerIds: false, mangle: false });

    const bodyEl = document.body;
    const previewEl = document.getElementById('preview');
    const editorEl = document.getElementById('editor');
    const saveBtn = document.getElementById('save-btn');
    const editLink = document.querySelector('[data-mode="edit"]');
    const viewLink = document.querySelector('[data-mode="view"]');
    const savePath = {{printf "%q" .SavePath}};
    const editPath = {{printf "%q" .EditPath}};
    const initialBody = {{printf "%q" .Body}};

    function splitDocument(text) {
      const normalized = String(text || '').replace(/^\uFEFF/, '');
      if (!normalized.startsWith('---\n') && !normalized.startsWith('---\r\n')) {
        return { body: normalized };
      }
      const lines = normalized.split(/\r?\n/);
      if (lines[0].trim() !== '---') return { body: normalized };
      for (let i = 1; i < lines.length; i++) {
        if (lines[i].trim() === '---') {
          return { body: lines.slice(i + 1).join('\n') };
        }
      }
      return { body: normalized };
    }

    function renderPreview() {
      if (!previewEl) return;
      const text = editorEl ? editorEl.value : initialBody;
      const parts = splitDocument(text);
      previewEl.innerHTML = marked.parse(parts.body || '');
    }

    async function saveDocument() {
      if (!editorEl) return;
      const form = new FormData();
      form.append('content', editorEl.value);
      form.append('return', bodyEl.classList.contains('mode-edit') ? 'edit' : 'view');
      const res = await fetch(savePath, { method: 'POST', body: form, redirect: 'follow' });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'save failed');
      }
      window.location.href = res.url || (viewLink ? viewLink.getAttribute('href') : editPath);
    }

    function setMode(mode) {
      bodyEl.classList.toggle('mode-edit', mode === 'edit');
      bodyEl.classList.toggle('mode-view', mode !== 'edit');
      if (mode === 'edit' && editorEl) {
        editorEl.focus();
      }
      renderPreview();
    }

    if (editorEl) {
      editorEl.addEventListener('input', renderPreview);
      editorEl.addEventListener('keydown', (event) => {
        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
          event.preventDefault();
          saveDocument().catch((err) => alert(String(err)));
        }
      });
    }

    if (saveBtn) {
      saveBtn.addEventListener('click', () => {
        saveDocument().catch((err) => alert(String(err)));
      });
    }

    if (editLink) {
      editLink.addEventListener('click', (event) => {
        event.preventDefault();
        history.replaceState(null, '', editPath);
        setMode('edit');
      });
    }

    if (viewLink) {
      viewLink.addEventListener('click', (event) => {
        event.preventDefault();
        history.replaceState(null, '', viewLink.getAttribute('href'));
        setMode('view');
      });
    }

    renderPreview();
    if (bodyEl.classList.contains('mode-edit')) {
      setMode('edit');
    } else {
      setMode('view');
    }
  </script>
</body>
</html>`
