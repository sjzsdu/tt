package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	jsonPort int = 9696
	jsonFile string

	jsonServer *http.Server
	jsonMu     sync.Mutex
	jsonRoot   string
)

var jsonCmd = &cobra.Command{
	Use:   "json [path]",
	Short: "Browse and edit JSON files in a local web UI",
	Long:  "Start a local web service for browsing JSON files, showing them in a formatted preview, and editing plus saving the underlying file.",
	Args:  cobra.MaximumNArgs(1),
	Example: `tt json
	tt json data/config.json
	tt json ~/projects/sample-data`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve json cwd failed: %w", err)
		}

		jsonRoot = cwd
		if strings.TrimSpace(jsonFile) != "" {
			resolved, root, err := resolveJSONSelection(jsonFile, cwd)
			if err != nil {
				return err
			}
			jsonFile = resolved
			jsonRoot = root
		}

		if len(args) == 1 && strings.TrimSpace(jsonFile) == "" {
			resolved, root, err := resolveJSONSelection(args[0], cwd)
			if err != nil {
				return err
			}
			jsonFile = resolved
			jsonRoot = root
		}

		return runJSONServer()
	},
}

func init() {
	rootCmd.AddCommand(jsonCmd)
	jsonCmd.Flags().IntVarP(&jsonPort, "port", "p", 9696, "service port")
	jsonCmd.Flags().StringVarP(&jsonFile, "file", "f", "", "open a specific JSON file")
}

func runJSONServer() error {
	jsonMu.Lock()
	defer jsonMu.Unlock()

	if jsonServer != nil {
		return fmt.Errorf("json service already running on port %d", jsonPort)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleJSONIndex)
	mux.HandleFunc("/list", handleJSONList)
	mux.HandleFunc("/view/", handleJSONView)
	mux.HandleFunc("/edit/", handleJSONEdit)
	mux.HandleFunc("/save/", handleJSONSave)
	mux.HandleFunc("/patch/", handleJSONPatch)
	mux.HandleFunc("/raw/", handleJSONRaw)

	maxPort := jsonPort + 20
	var lastErr error
	for port := jsonPort; port <= maxPort; port++ {
		jsonServer = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		serverErr := make(chan error, 1)
		go func() {
			err := jsonServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()

		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-serverErr:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				jsonServer = nil
				continue
			}
			jsonServer = nil
			return err
		default:
			jsonPort = port
			fmt.Printf("JSON service started: http://localhost:%d\n", port)
			go openBrowser(fmt.Sprintf("http://localhost:%d", port))
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt)
			<-quit
			fmt.Println("\nShutting down json service...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := jsonServer.Shutdown(ctx)
			jsonServer = nil
			return err
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func handleJSONIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if strings.TrimSpace(jsonFile) != "" {
		rel, err := jsonRelativePath(jsonFile, jsonRoot)
		if err == nil {
			http.Redirect(w, r, "/view/"+rel, http.StatusFound)
			return
		}
	}
	handleJSONList(w, r)
}

func handleJSONList(w http.ResponseWriter, r *http.Request) {
	files, err := collectJSONFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect json files failed: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Files []jsonEntry
		Total int
	}{Files: files, Total: len(files)}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("json-list").Parse(jsonListHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render json list failed: %v", err), http.StatusInternalServerError)
	}
}

func handleJSONView(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/view/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := renderJSONPage(w, relPath, false); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	}
}

func handleJSONEdit(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/edit/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := renderJSONPage(w, relPath, true); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	}
}

func handleJSONSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/save/")
	if strings.TrimSpace(relPath) == "" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}

	absPath, err := jsonSafeJoin(jsonRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("parse form failed: %v", err), http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	pretty, err := formatJSON(content)
	if err != nil {
		http.Error(w, fmt.Sprintf("save json failed: %v", err), http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(absPath, []byte(pretty+"\n"), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("save json failed: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/view/"+relPath, http.StatusSeeOther)
}

func handleJSONPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/patch/")
	if strings.TrimSpace(relPath) == "" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}

	absPath, err := jsonSafeJoin(jsonRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("parse form failed: %v", err), http.StatusBadRequest)
		return
	}

	pathStr := r.FormValue("path")
	newValue := r.FormValue("value")

	var path []string
	if err := stdjson.Unmarshal([]byte(pathStr), &path); err != nil {
		http.Error(w, fmt.Sprintf("invalid path: %v", err), http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read file failed: %v", err), http.StatusInternalServerError)
		return
	}

	var data any
	if err := stdjson.Unmarshal(content, &data); err != nil {
		http.Error(w, fmt.Sprintf("parse json failed: %v", err), http.StatusBadRequest)
		return
	}

	if err := setJSONPath(&data, path, newValue); err != nil {
		http.Error(w, fmt.Sprintf("update value failed: %v", err), http.StatusBadRequest)
		return
	}

	dataBytes, _ := stdjson.Marshal(data)
	pretty, err := formatJSON(string(dataBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf("format json failed: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(absPath, []byte(pretty+"\n"), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("save failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok": true}`))
}

func setJSONPath(root *any, path []string, newValue string) error {
	if len(path) > 0 && path[0] == "root" {
		path = path[1:]
	}

	var parsedVal any
	if err := stdjson.Unmarshal([]byte(newValue), &parsedVal); err != nil {
		return fmt.Errorf("invalid value format: %v", err)
	}

	if len(path) == 0 {
		*root = parsedVal
		return nil
	}

	current := *root
	for i, key := range path {
		if i == len(path)-1 {
			switch container := current.(type) {
			case map[string]any:
				container[key] = parsedVal
				return nil
			case []any:
				idx, err := strconv.Atoi(key)
				if err != nil || idx < 0 || idx >= len(container) {
					return fmt.Errorf("invalid array index: %s", key)
				}
				container[idx] = parsedVal
				return nil
			default:
				return fmt.Errorf("path not found at %s", key)
			}
		}

		switch container := current.(type) {
		case map[string]any:
			next, ok := container[key]
			if !ok {
				return fmt.Errorf("key not found: %s", key)
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 || idx >= len(container) {
				return fmt.Errorf("invalid array index: %s", key)
			}
			current = container[idx]
		default:
			return fmt.Errorf("path not found at %s", key)
		}
	}
	return nil
}

func handleJSONRaw(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/raw/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	absPath, err := jsonSafeJoin(jsonRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read json failed: %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(absPath)))
	_, _ = w.Write(content)
}

func renderJSONPage(w http.ResponseWriter, relPath string, editing bool) error {
	content, filePath, err := resolveJSONContent(relPath)
	if err != nil {
		return err
	}

	files, err := collectJSONFiles()
	if err != nil {
		return fmt.Errorf("collect json files failed: %w", err)
	}

	rel, err := filepath.Rel(jsonRoot, filePath)
	if err != nil {
		return fmt.Errorf("resolve json relative path failed: %w", err)
	}

	page := jsonPageData{
		FilePath:    "/" + filepath.ToSlash(rel),
		Title:       filepath.Base(filePath),
		Subtitle:    filePath,
		RawPath:     "/raw/" + filepath.ToSlash(rel),
		EditPath:    "/edit/" + filepath.ToSlash(rel),
		SavePath:    "/save/" + filepath.ToSlash(rel),
		Files:       files,
		Editing:     editing,
		ContentText: content,
	}

	pretty, prettyErr := formatJSON(content)
	if prettyErr == nil {
		page.PrettyText = pretty
	} else {
		page.PrettyText = content
	}

	treeHTML, outlineRoot, err := buildJSONView(content)
	if err != nil {
		page.TreeHTML = template.HTML("<pre class=\"json-pre\">" + template.HTMLEscapeString(content) + "</pre>")
		if page.PrettyText == "" {
			page.PrettyText = content
		}
		page.ParseError = err.Error()
	} else {
		page.TreeHTML = template.HTML(treeHTML)
		page.OutlineHTML = template.HTML(renderJSONOutline(outlineRoot.Children))
		page.ParseError = ""
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("json-view").Parse(jsonViewHTML)).Execute(w, page); err != nil {
		return fmt.Errorf("render json page failed: %w", err)
	}
	return nil
}

func collectJSONFiles() ([]jsonEntry, error) {
	files := make([]jsonEntry, 0)
	err := filepath.WalkDir(jsonRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || strings.HasPrefix(name, ".") && path != jsonRoot {
				if path != jsonRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isJSONFile(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(jsonRoot, path)
		if err != nil {
			return nil
		}
		files = append(files, jsonEntry{
			Path:     path,
			Name:     d.Name(),
			Relative: "/" + filepath.ToSlash(rel),
			Size:     info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	return files, nil
}

func resolveJSONSelection(input, cwd string) (string, string, error) {
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return "", cwd, nil
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve json path failed: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", "", fmt.Errorf("json path not found: %s", absPath)
	}
	if info.IsDir() {
		return "", absPath, nil
	}
	if !isJSONFile(info.Name()) {
		return "", "", fmt.Errorf("json file expected: %s", absPath)
	}
	return absPath, filepath.Dir(absPath), nil
}

func resolveJSONContent(relPath string) (string, string, error) {
	if strings.TrimSpace(jsonFile) != "" {
		content, err := os.ReadFile(jsonFile)
		if err != nil {
			return "", "", fmt.Errorf("read json failed: %w", err)
		}
		return string(content), jsonFile, nil
	}

	absPath, err := jsonSafeJoin(jsonRoot, relPath)
	if err != nil {
		return "", "", err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", "", fmt.Errorf("read json failed: %w", err)
	}
	return string(content), absPath, nil
}

func formatJSON(raw string) (string, error) {
	var b bytes.Buffer
	if err := stdjson.Indent(&b, []byte(raw), "", "  "); err != nil {
		return "", err
	}
	return b.String(), nil
}

type jsonOutlineNode struct {
	Label    string
	Anchor   string
	Kind     string
	Children []jsonOutlineNode
}

func buildJSONView(raw string) (string, jsonOutlineNode, error) {
	var value any
	if err := stdjson.Unmarshal([]byte(raw), &value); err != nil {
		return "", jsonOutlineNode{}, err
	}
	return renderJSONNode(value, "root", []string{"root"}, 0)
}

func renderJSONNode(value any, label string, path []string, depth int) (string, jsonOutlineNode, error) {
	anchor := anchorForJSONPath(path)
	escapedLabel := template.HTMLEscapeString(label)

	switch v := value.(type) {
	case map[string]any:
		keys := sortedJSONKeys(v)
		var childHTML strings.Builder
		children := make([]jsonOutlineNode, 0, len(keys))
		for _, key := range keys {
			html, outline, err := renderJSONNode(v[key], key, append(path, key), depth+1)
			if err != nil {
				return "", jsonOutlineNode{}, err
			}
			childHTML.WriteString(html)
			children = append(children, outline)
		}
		openAttr := ""
		if depth == 0 {
			openAttr = " open"
		}
		headline := "object"
		typeClass := "object"
		if label == "root" {
			html := `<details class="json-node json-object json-root" id="` + anchor + `"` + openAttr + `><summary><span class="json-label">` + escapedLabel + `</span><span class="json-type ` + typeClass + `">` + headline + `</span><span class="json-meta">` + fmt.Sprintf("%d key(s)", len(keys)) + `</span></summary><div class="json-children">` + childHTML.String() + `</div></details>`
			return html, jsonOutlineNode{Label: "root", Anchor: anchor, Kind: "object", Children: children}, nil
		}
		html := `<details class="json-node json-object" id="` + anchor + `"` + openAttr + `><summary><span class="json-label">` + escapedLabel + `</span><span class="json-type ` + typeClass + `">` + headline + `</span><span class="json-meta">` + fmt.Sprintf("%d key(s)", len(keys)) + `</span></summary><div class="json-children">` + childHTML.String() + `</div></details>`
		return html, jsonOutlineNode{Label: label, Anchor: anchor, Kind: "object", Children: children}, nil
	case []any:
		var childHTML strings.Builder
		children := make([]jsonOutlineNode, 0, len(v))
		for i, item := range v {
			idxLabel := fmt.Sprintf("[%d]", i)
			html, outline, err := renderJSONNode(item, idxLabel, append(path, fmt.Sprintf("%d", i)), depth+1)
			if err != nil {
				return "", jsonOutlineNode{}, err
			}
			childHTML.WriteString(html)
			children = append(children, outline)
		}
		openAttr := ""
		if depth == 0 {
			openAttr = " open"
		}
		html := `<details class="json-node json-array" id="` + anchor + `"` + openAttr + `><summary><span class="json-label">` + escapedLabel + `</span><span class="json-type array">array</span><span class="json-meta">` + fmt.Sprintf("%d item(s)", len(v)) + `</span></summary><div class="json-children">` + childHTML.String() + `</div></details>`
		return html, jsonOutlineNode{Label: label, Anchor: anchor, Kind: "array", Children: children}, nil
	default:
		literal, err := stdjson.Marshal(v)
		if err != nil {
			literal = []byte(fmt.Sprintf("%v", v))
		}
		strLit := string(literal)
		valueClass := "string"
		if strLit == "null" {
			valueClass = "null"
		} else if strLit == "true" || strLit == "false" {
			valueClass = "boolean"
		} else if strLit[0] >= '0' && strLit[0] <= '9' || strLit[0] == '-' {
			valueClass = "number"
		}
		pathBytes, _ := stdjson.Marshal(path)
		encodedPath := base64.RawURLEncoding.EncodeToString(pathBytes)
		encodedValue := base64.RawURLEncoding.EncodeToString([]byte(strLit))
		html := `<div class="json-node json-leaf" id="` + anchor + `"><span class="json-leaf-label">` + escapedLabel + `</span><span class="json-sep">:</span><span class="json-value ` + valueClass + `">` + template.HTMLEscapeString(strLit) + `</span><button class="json-edit-btn" data-path="` + encodedPath + `" data-value="` + encodedValue + `" data-class="` + valueClass + `" title="Edit value">✎</button></div>`
		return html, jsonOutlineNode{Label: label, Anchor: anchor, Kind: "value"}, nil
	}
}

func renderJSONOutline(nodes []jsonOutlineNode) string {
	if len(nodes) == 0 {
		return `<div class="outline-empty">No navigable object keys</div>`
	}
	var b strings.Builder
	b.WriteString(`<nav class="outline-tree">`)
	for _, node := range nodes {
		b.WriteString(renderJSONOutlineNode(node, 0))
	}
	b.WriteString(`</nav>`)
	return b.String()
}

func renderJSONOutlineNode(node jsonOutlineNode, depth int) string {
	escapedAnchor := template.HTMLEscapeString(node.Anchor)
	escapedLabel := template.HTMLEscapeString(node.Label)
	kind := node.Kind
	if kind == "" {
		kind = "value"
	}

	if kind == "value" {
		return ""
	}

	var b strings.Builder
	openAttr := ""
	if depth == 0 {
		openAttr = " open"
	}
	b.WriteString(`<details class="outline-node outline-group outline-` + kind + `"` + openAttr + ` data-anchor="` + escapedAnchor + `">`)
	b.WriteString(`<summary><a class="outline-link" href="#` + escapedAnchor + `">` + escapedLabel + `</a></summary>`)
	b.WriteString(`<div class="outline-children">`)
	for _, child := range node.Children {
		b.WriteString(renderJSONOutlineNode(child, depth+1))
	}
	b.WriteString(`</div></details>`)
	return b.String()
}

func anchorForJSONPath(path []string) string {
	joined := strings.Join(path, "/")
	if joined == "" {
		joined = "root"
	}
	return "json-" + base64.RawURLEncoding.EncodeToString([]byte(joined))
}

func sortedJSONKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func jsonSafeJoin(root, relPath string) (string, error) {
	trimmed := strings.TrimPrefix(relPath, "/")
	cleaned := filepath.Clean(trimmed)
	absPath := filepath.Join(root, cleaned)
	rootClean := filepath.Clean(root)
	absClean := filepath.Clean(absPath)
	if absClean != rootClean && !strings.HasPrefix(absClean, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return absClean, nil
}

func jsonRelativePath(absPath, root string) (string, error) {
	absClean := filepath.Clean(absPath)
	rootClean := filepath.Clean(root)
	if absClean == rootClean || absClean == rootClean+string(os.PathSeparator) {
		return "/", nil
	}
	if !strings.HasPrefix(absClean, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	rel, err := filepath.Rel(rootClean, absClean)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

func isJSONFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".json")
}

const jsonListHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>JSON files</title>
  <style>
    :root {
      --bg: #0b1220;
      --panel: rgba(7, 12, 24, 0.58);
      --panel-2: rgba(15, 23, 42, 0.72);
      --line: rgba(148, 163, 184, 0.2);
      --text: #e2e8f0;
      --muted: #94a3b8;
      --brand: #60a5fa;
      --brand-2: #93c5fd;
      --shadow: 0 20px 60px rgba(2, 6, 23, 0.35);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background: radial-gradient(circle at top left, #16213f 0, #0b1220 48%, #050816 100%);
      padding: 32px;
    }
    .shell {
      width: min(1200px, 100%);
      margin: 0 auto;
      background: var(--panel);
      border: 1px solid rgba(148, 163, 184, 0.16);
      border-radius: 28px;
      box-shadow: var(--shadow);
      overflow: hidden;
      backdrop-filter: blur(16px);
    }
    .header {
      padding: 24px 28px;
      border-bottom: 1px solid var(--line);
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
    }
    .title { font-size: 22px; font-weight: 700; }
    .meta { color: var(--muted); font-size: 13px; margin-top: 6px; }
    .actions a {
      color: var(--brand-2);
      text-decoration: none;
      font-weight: 600;
      font-size: 13px;
    }
    .actions a:hover { text-decoration: underline; }
    .content { padding: 22px 28px 30px; }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 10px;
    }
    .item a {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      padding: 16px 18px;
      border-radius: 16px;
      text-decoration: none;
      color: inherit;
      background: rgba(15, 23, 42, 0.68);
      border: 1px solid var(--line);
      transition: transform .18s ease, border-color .18s ease, background .18s ease;
    }
    .item a:hover {
      transform: translateY(-1px);
      border-color: rgba(96, 165, 250, 0.55);
      background: rgba(15, 23, 42, 0.92);
    }
    .path { font-weight: 600; word-break: break-all; }
    .size { color: var(--muted); font-size: 13px; white-space: nowrap; }
    .empty {
      padding: 42px 16px;
      text-align: center;
      color: var(--muted);
      border: 1px dashed rgba(148, 163, 184, 0.24);
      border-radius: 20px;
      background: rgba(15, 23, 42, 0.36);
    }
    @media (max-width: 720px) {
      body { padding: 16px; }
      .header, .content { padding-left: 18px; padding-right: 18px; }
      .item a { flex-direction: column; align-items: flex-start; }
    }
  </style>
</head>
<body>
  <div class="shell">
    <div class="header">
      <div>
        <div class="title">JSON files</div>
        <div class="meta">{{.Total}} file(s) found under the selected root</div>
      </div>
      <div class="actions"><a href="/">Refresh</a></div>
    </div>
    <div class="content">
      {{if .Files}}
      <ul class="list">
        {{range .Files}}
        <li class="item"><a href="/view{{.Relative}}"><span class="path">{{.Relative}}</span><span class="size">{{.Size}} bytes</span></a></li>
        {{end}}
      </ul>
      {{else}}
      <div class="empty">No JSON files found.</div>
      {{end}}
    </div>
  </div>
</body>
</html>`

const jsonViewHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <style>
    :root {
      --bg: #0b1220;
      --panel: rgba(7, 12, 24, 0.58);
      --panel-2: rgba(15, 23, 42, 0.72);
      --line: rgba(148, 163, 184, 0.18);
      --text: #e2e8f0;
      --muted: #94a3b8;
      --brand: #60a5fa;
      --brand-strong: #2563eb;
      --success: #10b981;
      --shadow: 0 20px 60px rgba(2, 6, 23, 0.35);
    }
    * { box-sizing: border-box; }
    html, body { height: 100%; }
    body {
      margin: 0;
      overflow: hidden;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background: radial-gradient(circle at top left, #16213f 0, #0b1220 48%, #050816 100%);
    }
    a { color: inherit; }
    .layout {
      display: grid;
      grid-template-columns: 300px minmax(0, 1fr) 320px;
      height: 100vh;
      overflow: hidden;
    }
    .sidebar,
    .outline-pane {
      height: 100vh;
      overflow: auto;
      background: rgba(7, 12, 24, 0.74);
      border-right: 1px solid var(--line);
      padding: 20px;
      scrollbar-gutter: stable;
    }
    .outline-pane {
      border-right: 0;
      border-left: 0;
      padding: 12px 10px;
    }
    .outline-header {
      padding-bottom: 6px;
      margin-bottom: 8px;
    }
    .outline-title {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: .05em;
      color: #64748b;
    }
    .outline-title svg {
      opacity: 0.6;
    }
    .main {
      min-width: 0;
      height: 100vh;
      padding: 20px;
      overflow: hidden;
    }
    .card {
      height: 100%;
      min-height: 0;
      display: flex;
      flex-direction: column;
      background: var(--panel);
      border: 1px solid rgba(148, 163, 184, 0.16);
      border-radius: 28px;
      box-shadow: var(--shadow);
      backdrop-filter: blur(16px);
      overflow: hidden;
    }
    .card-header {
      padding: 20px 24px;
      border-bottom: 1px solid var(--line);
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      flex-wrap: wrap;
    }
    .title { font-size: 22px; font-weight: 700; }
    .subtitle { color: var(--muted); font-size: 13px; margin-top: 6px; word-break: break-all; }
    .toolbar-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      align-items: center;
    }
    .btn {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 10px 14px;
      border-radius: 999px;
      border: 1px solid var(--line);
      text-decoration: none;
      color: var(--text);
      background: rgba(15, 23, 42, 0.72);
      font-size: 14px;
      cursor: pointer;
      white-space: nowrap;
    }
    .btn:hover {
      border-color: rgba(96, 165, 250, 0.55);
      background: rgba(15, 23, 42, 0.92);
    }
    .btn-primary {
      background: linear-gradient(135deg, var(--brand-strong), var(--brand));
      border-color: rgba(96, 165, 250, 0.3);
    }
    .btn-success {
      background: linear-gradient(135deg, #059669, var(--success));
      border-color: rgba(16, 185, 129, 0.3);
    }
    .file-list,
    .outline-tree {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 10px;
    }
    .file-item a {
      display: block;
      padding: 12px 14px;
      border-radius: 16px;
      text-decoration: none;
      color: inherit;
      background: rgba(15, 23, 42, 0.68);
      border: 1px solid var(--line);
      transition: transform .18s ease, border-color .18s ease, background .18s ease;
    }
    .file-item a:hover {
      transform: translateY(-1px);
      border-color: rgba(96, 165, 250, 0.55);
      background: rgba(15, 23, 42, 0.92);
    }
    .file-item a.active {
      border-color: rgba(96, 165, 250, 0.7);
      box-shadow: 0 0 0 1px rgba(96, 165, 250, 0.18) inset;
    }
    .file-name { font-size: 14px; font-weight: 600; word-break: break-all; }
    .file-meta { margin-top: 6px; color: var(--muted); font-size: 12px; }
    .sidebar-title,
    .outline-title {
      font-size: 14px;
      font-weight: 700;
      letter-spacing: .02em;
      text-transform: uppercase;
      color: #cbd5e1;
      margin-bottom: 14px;
    }
    .sidebar-meta,
    .outline-meta {
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 16px;
      line-height: 1.5;
    }
    .panel {
      padding: 24px;
      min-height: 0;
      overflow: auto;
      flex: 1;
    }
    .notice {
      margin-bottom: 14px;
      padding: 14px 16px;
      border-radius: 16px;
      background: rgba(2, 6, 23, 0.72);
      border: 1px solid rgba(96, 165, 250, 0.25);
      color: #cbd5e1;
      font-size: 13px;
    }
    .notice.error {
      border-color: rgba(251, 113, 133, 0.3);
      color: #fecdd3;
    }
    .json-tree {
      display: grid;
      gap: 6px;
      min-width: 0;
    }
    .json-node {
      border-radius: 12px;
      background: rgba(2, 6, 23, 0.5);
      overflow: hidden;
      scroll-margin-top: 16px;
    }
    .json-node > summary {
      list-style: none;
      cursor: pointer;
      display: flex;
      gap: 10px;
      align-items: center;
      padding: 14px 16px;
      user-select: none;
    }
    .json-node > summary::-webkit-details-marker { display: none; }
    .json-node > summary::before {
      content: "▸";
      color: var(--brand);
      width: 16px;
      flex: 0 0 auto;
      transition: transform .18s ease;
    }
    .json-node[open] > summary::before { transform: rotate(90deg); }
    .json-label {
      color: #a5b4fc;
      font-weight: 600;
      word-break: break-all;
    }
    .json-type {
      font-size: 11px;
      padding: 2px 8px;
      border-radius: 6px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.03em;
    }
    .json-type.object { background: rgba(168, 85, 247, 0.15); color: #d8b4fe; border: 1px solid rgba(168, 85, 247, 0.25); }
    .json-type.array { background: rgba(34, 211, 238, 0.15); color: #67e8f9; border: 1px solid rgba(34, 211, 238, 0.25); }
    .json-meta {
      margin-left: auto;
      color: #64748b;
      font-size: 12px;
      white-space: nowrap;
    }
    .json-children {
      padding: 6px 12px 10px 28px;
      display: grid;
      gap: 4px;
      background: rgba(0, 0, 0, 0.12);
    }
    .json-leaf {
      display: flex;
      gap: 8px;
      align-items: baseline;
      padding: 6px 10px;
      border-radius: 8px;
      scroll-margin-top: 16px;
      overflow: auto;
      margin-left: 16px;
    }
    .json-leaf:hover {
      background: rgba(15, 23, 42, 0.3);
    }
    .json-leaf-label { font-weight: 600; color: #94a3b8; }
    .json-sep { color: #475569; }
    .json-value {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .json-value.string { color: #34d399; }
    .json-value.number { color: #fbbf24; }
    .json-value.boolean { color: #f472b6; }
    .json-value.null { color: #64748b; font-style: italic; }
    .json-edit-btn {
      opacity: 0;
      background: transparent;
      border: none;
      color: #64748b;
      cursor: pointer;
      padding: 2px 6px;
      font-size: 12px;
      margin-left: 4px;
      border-radius: 4px;
      transition: opacity .15s, color .15s;
    }
    .json-leaf:hover .json-edit-btn {
      opacity: 1;
    }
    .json-edit-btn:hover {
      color: #60a5fa;
      background: rgba(96, 165, 250, 0.1);
    }
    .json-edit-input {
      background: rgba(15, 23, 42, 0.8);
      border: 1px solid rgba(96, 165, 250, 0.4);
      color: #e2e8f0;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 13px;
      padding: 4px 8px;
      border-radius: 6px;
      width: 200px;
      outline: none;
    }
    .json-edit-input:focus {
      border-color: #60a5fa;
      box-shadow: 0 0 0 3px rgba(96, 165, 250, 0.15);
    }
    .json-edit-actions {
      display: inline-flex;
      gap: 4px;
      margin-left: 6px;
    }
    .json-edit-actions button {
      background: transparent;
      border: none;
      cursor: pointer;
      padding: 2px 4px;
      font-size: 12px;
      border-radius: 4px;
    }
    .json-edit-save { color: #34d399; }
    .json-edit-save:hover { background: rgba(52, 211, 153, 0.15); }
    .json-edit-cancel { color: #f87171; }
    .json-edit-cancel:hover { background: rgba(248, 113, 113, 0.15); }
    .json-toast {
      position: fixed;
      right: 20px;
      bottom: 20px;
      max-width: 360px;
      padding: 10px 12px;
      border-radius: 10px;
      background: rgba(15, 23, 42, 0.96);
      color: #e2e8f0;
      border: 1px solid rgba(148, 163, 184, 0.14);
      box-shadow: 0 18px 40px rgba(2, 6, 23, 0.35);
      font-size: 12px;
      line-height: 1.5;
      opacity: 0;
      transform: translateY(8px);
      pointer-events: none;
      transition: opacity .18s ease, transform .18s ease;
      z-index: 40;
    }
    .json-toast.show {
      opacity: 1;
      transform: translateY(0);
    }
    .json-toast.error {
      border-color: rgba(248, 113, 113, 0.28);
      color: #fecaca;
    }
    .json-toast.success {
      border-color: rgba(52, 211, 153, 0.28);
      color: #bbf7d0;
    }
    .json-node > summary {
      padding: 10px 12px;
      border-radius: 10px;
    }
    .json-node > summary::before {
      color: #60a5fa;
    }
    .json-node[open] > summary::before {
      transform: rotate(90deg);
    }
    .json-pre,
    .json-editor {
      width: 100%;
      border-radius: 20px;
      border: 1px solid var(--line);
      background: rgba(2, 6, 23, 0.78);
      color: #dbeafe;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 14px;
      line-height: 1.7;
      tab-size: 2;
    }
    .json-pre {
      margin: 0;
      padding: 20px 22px;
      overflow: auto;
      white-space: pre;
      scroll-margin-top: 24px;
    }
    .json-editor {
      min-height: 72vh;
      resize: vertical;
      padding: 18px 20px;
      outline: none;
    }
    .json-editor:focus {
      border-color: rgba(96, 165, 250, 0.55);
      box-shadow: 0 0 0 4px rgba(96, 165, 250, 0.12);
    }
    .editor-form {
      display: grid;
      gap: 14px;
    }
    .editor-actions {
      display: flex;
      justify-content: flex-end;
      gap: 10px;
      flex-wrap: wrap;
    }
    .hint {
      color: var(--muted);
      font-size: 12px;
      line-height: 1.6;
    }
    .empty {
      padding: 36px 18px;
      text-align: center;
      color: var(--muted);
      border: 1px dashed rgba(148, 163, 184, 0.22);
      border-radius: 20px;
      background: rgba(15, 23, 42, 0.36);
    }
    .outline-node {
      margin: 0;
    }
    .outline-group {
      background: transparent;
    }
    .outline-group > summary {
      list-style: none;
      cursor: pointer;
      display: flex;
      align-items: center;
      gap: 5px;
      padding: 2px 4px;
      border-radius: 0;
      user-select: none;
    }
    .outline-group > summary:hover {
      background: rgba(96, 165, 250, 0.06);
    }
    .outline-group > summary::-webkit-details-marker { display: none; }
    .outline-group > summary::before {
      content: "▶";
      font-size: 8px;
      color: #64748b;
      transition: transform .15s ease;
      flex: 0 0 auto;
    }
    .outline-group[open] > summary::before {
      transform: rotate(90deg);
    }
    .outline-link {
      flex: 1;
      min-width: 0;
      text-decoration: none;
      color: #94a3b8;
      font-size: 12px;
      font-weight: 500;
      word-break: break-all;
    }
    .outline-link:hover {
      color: #e2e8f0;
    }
    .outline-kind {
      display: none;
    }
    .outline-children {
      display: grid;
      gap: 1px;
      padding: 1px 0 2px 10px;
      margin-left: 2px;
    }
    .outline-leaf {
      display: flex;
      align-items: center;
      gap: 5px;
      padding: 2px 4px;
      border-radius: 0;
    }
    .outline-leaf:hover {
      background: rgba(96, 165, 250, 0.06);
    }
    .outline-leaf .outline-link {
      white-space: normal;
    }
    .outline-leaf .outline-kind {
      background: rgba(148, 163, 184, 0.12);
      border-color: rgba(148, 163, 184, 0.14);
      color: #cbd5e1;
    }
    .outline-empty {
      color: #475569;
      font-size: 12px;
      padding: 8px 0;
    }
    @media (max-width: 1280px) {
      .layout { grid-template-columns: 260px minmax(0, 1fr) 280px; }
    }
    @media (max-width: 1100px) {
      body { overflow: auto; }
      .layout { grid-template-columns: 1fr; height: auto; overflow: visible; }
      .sidebar, .outline-pane, .main { height: auto; overflow: visible; }
      .main { min-height: 0; }
      .card { height: auto; }
      .panel { overflow: visible; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <aside class="sidebar">
      <div class="sidebar-title">JSON files</div>
      <div class="sidebar-meta">{{len .Files}} file(s) in root</div>
      {{if .Files}}
      <ul class="file-list">
        {{range .Files}}
        <li class="file-item"><a class="{{if eq $.FilePath .Relative}}active{{end}}" href="/view{{.Relative}}"><div class="file-name">{{.Relative}}</div><div class="file-meta">{{.Size}} bytes</div></a></li>
        {{end}}
      </ul>
      {{else}}
      <div class="empty">No JSON files found.</div>
      {{end}}
    </aside>

    <main class="main">
      <div class="card" data-file-path="{{.FilePath}}">
        <div class="card-header">
          <div>
            <div class="title">{{.Title}}</div>
            <div class="subtitle">{{.Subtitle}}</div>
          </div>
          <div class="toolbar-actions">
            <a class="btn" href="{{.RawPath}}">Raw</a>
            {{if .Editing}}
            <a class="btn" href="/view{{.FilePath}}">Preview</a>
            <button class="btn btn-success" type="submit" form="json-editor-form">Save</button>
            {{else}}
            <a class="btn btn-primary" href="/edit{{.FilePath}}">Edit</a>
            {{end}}
          </div>
        </div>
        <div class="panel">
          {{if .ParseError}}
          <div class="notice error">{{.ParseError}}</div>
          {{else}}
          <div class="notice">支持折叠展开，右侧目录可以直接跳转到对应节点。</div>
          {{end}}

          {{if .Editing}}
          <form id="json-editor-form" class="editor-form" method="post" action="{{.SavePath}}">
            <textarea class="json-editor" name="content" spellcheck="false">{{.ContentText}}</textarea>
            <div class="editor-actions">
              <a class="btn" href="/view{{.FilePath}}">Cancel</a>
              <button class="btn btn-success" type="submit">Save</button>
            </div>
            <div class="hint">保存时会先校验 JSON，再按格式化后的内容写回文件。</div>
          </form>
          {{else}}
          {{if .TreeHTML}}
          <div class="json-tree">{{.TreeHTML}}</div>
          {{else}}
          <pre class="json-pre">{{.PrettyText}}</pre>
          {{end}}
          {{end}}
        </div>
      </div>
    </main>

    <aside class="outline-pane">
      <div class="outline-header">
        <div class="outline-title">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
          Navigator
        </div>
      </div>
      {{if .OutlineHTML}}
      {{.OutlineHTML}}
      {{else}}
      <div class="outline-empty">No keys found</div>
      {{end}}
    </aside>
  </div>

  <script>
    function openAncestors(node) {
      let current = node;
      while (current) {
        if (current.tagName === 'DETAILS') {
          current.open = true;
        }
        current = current.parentElement;
      }
    }

    document.querySelectorAll('.outline-link').forEach((link) => {
      link.addEventListener('click', (event) => {
        const href = link.getAttribute('href');
        if (!href || !href.startsWith('#')) return;
        const target = document.querySelector(href);
        if (!target) return;
        event.preventDefault();
        openAncestors(target);
        target.scrollIntoView({ behavior: 'smooth', block: 'start', inline: 'nearest' });
      });
    });

    function decodeBase64URL(value) {
      const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
      const padding = '='.repeat((4 - normalized.length % 4) % 4);
      const binary = atob(normalized + padding);
      const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
      return new TextDecoder().decode(bytes);
    }

    function encodeBase64URL(value) {
      const bytes = new TextEncoder().encode(value);
      let binary = '';
      bytes.forEach((byte) => {
        binary += String.fromCharCode(byte);
      });
      return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    }

    function encodeJSONValue(rawValue, valueClass) {
      if (valueClass === 'string') {
        return JSON.stringify(rawValue);
      }
      if (valueClass === 'number') {
        return rawValue;
      }
      if (valueClass === 'boolean') {
        const normalized = rawValue.trim().toLowerCase();
        if (normalized !== 'true' && normalized !== 'false') {
          throw new Error('Boolean only accepts true or false');
        }
        return normalized;
      }
      if (valueClass === 'null') {
        const normalized = rawValue.trim().toLowerCase();
        if (normalized !== 'null') {
          throw new Error('Null only accepts null');
        }
        return 'null';
      }
      return rawValue;
    }

    let toastTimer;
    function showToast(message, kind) {
      let node = document.querySelector('.json-toast');
      if (!node) {
        node = document.createElement('div');
        node.className = 'json-toast';
        document.body.appendChild(node);
      }
      node.textContent = message;
      node.className = 'json-toast show ' + (kind || '');
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => {
        node.className = 'json-toast ' + (kind || '');
      }, 2200);
    }

    function startLeafEdit(leaf, btn) {
      const labelText = leaf.querySelector('.json-leaf-label').textContent;
      const rawValue = decodeBase64URL(btn.dataset.value || '');
      const valueClass = btn.dataset.class || '';
      let editableValue = rawValue;
      if (valueClass === 'string') {
        try {
          editableValue = JSON.parse(rawValue);
        } catch (e) {}
      }

      leaf.dataset.path = btn.dataset.path || '';
      leaf.dataset.valueClass = valueClass;
      leaf.dataset.originalValue = btn.dataset.value || '';

      leaf.innerHTML = '<span class="json-leaf-label"></span><span class="json-sep">:</span><input class="json-edit-input" type="text"><div class="json-edit-actions"><button class="json-edit-save" type="button">✓</button><button class="json-edit-cancel" type="button">✕</button></div>';
      leaf.querySelector('.json-leaf-label').textContent = labelText;
      leaf.querySelector('.json-edit-input').value = editableValue;
      leaf.querySelector('.json-edit-input').focus();
      leaf.querySelector('.json-edit-input').select();
    }

    function restoreLeaf(leaf) {
      const labelText = leaf.querySelector('.json-leaf-label').textContent;
      const rawValue = decodeBase64URL(leaf.dataset.originalValue || '');
      const valueClass = leaf.dataset.valueClass || '';
      const path = leaf.dataset.path || '';

      leaf.innerHTML = '<span class="json-leaf-label"></span><span class="json-sep">:</span><span class="json-value"></span><button class="json-edit-btn" type="button" title="Edit value">✎</button>';
      leaf.querySelector('.json-leaf-label').textContent = labelText;
      const valueNode = leaf.querySelector('.json-value');
      valueNode.textContent = rawValue;
      if (valueClass) {
        valueNode.classList.add(valueClass);
      }
      const nextBtn = leaf.querySelector('.json-edit-btn');
      nextBtn.dataset.path = path;
      nextBtn.dataset.value = leaf.dataset.originalValue || '';
      nextBtn.dataset.class = valueClass;
    }

    document.addEventListener('click', function(e) {
      if (e.target.classList.contains('json-edit-btn')) {
        startLeafEdit(e.target.closest('.json-leaf'), e.target);
        return;
      }

      if (e.target.classList.contains('json-edit-save')) {
        const leaf = e.target.closest('.json-leaf');
        const input = leaf.querySelector('.json-edit-input');
        const rawValue = input.value;
        const path = JSON.parse(decodeBase64URL(leaf.dataset.path || ''));
        const valueClass = leaf.dataset.valueClass || '';
        const card = document.querySelector('.card');
        const filePath = card.getAttribute('data-file-path');

        let newValue;
        try {
          newValue = encodeJSONValue(rawValue, valueClass);
        } catch (err) {
          showToast(err.message || String(err), 'error');
          input.focus();
          return;
        }

        fetch('/patch' + filePath, {
          method: 'POST',
          headers: {'Content-Type': 'application/x-www-form-urlencoded'},
          body: 'path=' + encodeURIComponent(JSON.stringify(path)) + '&value=' + encodeURIComponent(newValue)
        }).then(async function(response) {
          if (response.ok) {
            showToast('Saved', 'success');
            leaf.dataset.originalValue = encodeBase64URL(newValue);
            restoreLeaf(leaf);
          } else {
            const text = await response.text();
            showToast(text || 'Failed to save', 'error');
          }
        }).catch(function(e) {
          showToast('Error: ' + e, 'error');
        });
      }

      if (e.target.classList.contains('json-edit-cancel')) {
        restoreLeaf(e.target.closest('.json-leaf'));
      }
    });

    document.addEventListener('keydown', function(e) {
      if (!e.target.classList.contains('json-edit-input')) return;
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        e.target.closest('.json-leaf').querySelector('.json-edit-save').click();
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.target.closest('.json-leaf').querySelector('.json-edit-cancel').click();
      }
    });
  </script>
</body>
</html>`

type jsonEntry struct {
	Path     string
	Name     string
	Relative string
	Size     int64
}

type jsonPageData struct {
	FilePath    string
	Title       string
	Subtitle    string
	RawPath     string
	EditPath    string
	SavePath    string
	Files       []jsonEntry
	Editing     bool
	ContentText string
	PrettyText  string
	ParseError  string
	TreeHTML    template.HTML
	OutlineHTML template.HTML
}
