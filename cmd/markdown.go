package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	mdutil "github.com/sjzsdu/tt/internal/mdutil"
	"github.com/sjzsdu/tt/internal/webui"
	"github.com/spf13/cobra"
	"nhooyr.io/websocket"
)

var (
	mdPort        = 9595
	mdContent     string
	mdContentOnly bool
	mdPatterns    []string

	mdServer *http.Server
	mdMu     sync.Mutex
	mdRoot   string

	mdClients   = make(map[*websocket.Conn]bool)
	mdClientsMu sync.Mutex
	mdWatcher   *fsnotify.Watcher
	mdWatchMu   sync.Mutex
)

var markdownCmd = &cobra.Command{
	Use:   "markdown [files...]",
	Short: "Browse markdown files in a local web UI",
	Long:  "Start a local web service for browsing markdown files in the current working tree.",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}
		merged := loaded.Merged
		if !cmd.Flags().Changed("port") && merged.Markdown.Port != nil {
			mdPort = *merged.Markdown.Port
		}
		if !cmd.Flags().Changed("content") && strings.TrimSpace(merged.Markdown.Content) != "" {
			mdContent = merged.Markdown.Content
		}
		if !cmd.Flags().Changed("content-only") && merged.Markdown.ContentOnly != nil {
			mdContentOnly = *merged.Markdown.ContentOnly
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve markdown cwd failed: %w", err)
		}
		mdRoot = cwd

		flagPatterns, _ := cmd.Flags().GetStringSlice("pattern")
		mdPatterns = append([]string{}, flagPatterns...)
		if !cmd.Flags().Changed("pattern") && len(merged.Markdown.Patterns) > 0 {
			mdPatterns = append(mdPatterns, merged.Markdown.Patterns...)
		}
		if len(args) == 1 {
			candidate := args[0]
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(cwd, candidate)
			}
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				absRoot, err := filepath.Abs(candidate)
				if err != nil {
					return fmt.Errorf("resolve markdown root failed: %w", err)
				}
				mdRoot = absRoot
				mdPatterns = nil
				return runMarkdownServer()
			}
		}
		if len(args) > 0 {
			if len(mdPatterns) > 0 {
				mergedPatterns := append([]string{mdPatterns[0]}, args...)
				mdPatterns = mergedPatterns
			} else {
				mdPatterns = append([]string{}, args...)
			}
		}

		return runMarkdownServer()
	},
}

func init() {
	rootCmd.AddCommand(markdownCmd)
	markdownCmd.Flags().IntVarP(&mdPort, "port", "p", 9595, "service port")
	markdownCmd.Flags().StringVarP(&mdContent, "content", "c", "", "render provided markdown content directly")
	markdownCmd.Flags().BoolVar(&mdContentOnly, "content-only", false, "only show provided markdown content")
	markdownCmd.Flags().StringSliceVarP(&mdPatterns, "pattern", "f", []string{}, "filter markdown files by glob patterns, supports multiple values")
}

func runMarkdownServer() error {
	mdMu.Lock()
	defer mdMu.Unlock()

	if mdServer != nil {
		return fmt.Errorf("markdown service already running on port %d", mdPort)
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.MarkdownAssetsHandler())
	mux.HandleFunc("/", handleApp)
	mux.HandleFunc("/list", handleApp)
	mux.HandleFunc("/api/list", handleListAPI)
	mux.HandleFunc("/api/document/", handleDocumentAPI)
	mux.HandleFunc("/api/content", handleContentAPI)
	mux.HandleFunc("/view/", handleView)
	mux.HandleFunc("/edit/", handleEdit)
	mux.HandleFunc("/save/", handleSave)
	mux.HandleFunc("/delete/", handleDelete)
	mux.HandleFunc("/raw/", handleRaw)
	mux.HandleFunc("/raw-content", handleRawContent)
	mux.HandleFunc("/images/", handleImages)
	mux.HandleFunc("/ws", handleWS)

	maxPort := mdPort + 20
	var lastErr error
	for port := mdPort; port <= maxPort; port++ {
		mdServer = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		serverErr := make(chan error, 1)
		go func() {
			err := mdServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()

		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-serverErr:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				mdServer = nil
				continue
			}
			mdServer = nil
			return err
		default:
			mdPort = port
			fmt.Printf("Markdown service started: http://localhost:%d\n", port)
			if err := initWatcher(); err != nil {
				fmt.Printf("watcher init warning: %v\n", err)
			}
			go openBrowser(fmt.Sprintf("http://localhost:%d", port))
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt)
			<-quit
			fmt.Println("\nShutting down markdown service...")
			cleanupWatcher()
			closeClients()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := mdServer.Shutdown(ctx)
			mdServer = nil
			return err
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func handleApp(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/list" {
		http.NotFound(w, r)
		return
	}
	serveMarkdownApp(w, r)
}

func serveMarkdownApp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(webui.MarkdownIndex()); err != nil {
		http.Error(w, fmt.Sprintf("render markdown app failed: %v", err), http.StatusInternalServerError)
	}
}

func handleListAPI(w http.ResponseWriter, r *http.Request) {
	files, err := collectFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect markdown files failed: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"files": files, "total": len(files), "contentMode": mdContent != "", "contentOnly": mdContentOnly, "workspaceName": markdownWorkspaceName()})
}

func markdownWorkspaceName() string {
	if mdContent != "" && mdContentOnly {
		return "Markdown Content"
	}
	root := strings.TrimSpace(mdRoot)
	if root == "" {
		return "Markdown Files"
	}
	name := filepath.Base(filepath.Clean(root))
	if name == "." || name == string(filepath.Separator) {
		return root
	}
	return name
}

func handleView(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/view")
	if relPath == "" || relPath == "/" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	serveMarkdownApp(w, r)
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/edit")
	if relPath == "" || relPath == "/" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	serveMarkdownApp(w, r)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	relPath := strings.TrimPrefix(r.URL.Path, "/delete")
	if relPath == "" || relPath == "/" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}
	if mdContent != "" {
		http.Error(w, "deleting direct markdown content is not supported", http.StatusBadRequest)
		return
	}
	absPath, err := safeJoin(mdRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.Remove(absPath); err != nil {
		http.Error(w, fmt.Sprintf("delete markdown failed: %v", err), http.StatusInternalServerError)
		return
	}

	files, _ := collectFiles()
	if len(files) > 0 {
		http.Redirect(w, r, "/view"+files[0].Relative+"?t="+fmt.Sprint(time.Now().Unix()), http.StatusFound)
	} else {
		http.Redirect(w, r, "/list?t="+fmt.Sprint(time.Now().Unix()), http.StatusFound)
	}
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	relPath := strings.TrimPrefix(r.URL.Path, "/save")
	if relPath == "" || relPath == "/" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}
	absPath, err := safeJoin(mdRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if mdContent != "" {
		http.Error(w, "editing direct markdown content is not supported", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("parse form failed: %v", err), http.StatusBadRequest)
		return
	}

	body := r.FormValue("content")
	var fmLines []string
	for key, vals := range r.Form {
		if strings.HasPrefix(key, "fm_") {
			fieldKey := strings.TrimPrefix(key, "fm_")
			if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
				fmLines = append(fmLines, fmt.Sprintf("%s: %s", fieldKey, vals[0]))
			}
		}
	}

	var finalContent string
	if len(fmLines) > 0 {
		finalContent = "---\n" + strings.Join(fmLines, "\n") + "\n---\n" + body
	} else {
		finalContent = body
	}

	if err := os.WriteFile(absPath, []byte(finalContent), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("save markdown failed: %v", err), http.StatusInternalServerError)
		return
	}
	broadcastReload(filepath.Base(absPath) + " saved")
	http.Redirect(w, r, "/view"+relPath, http.StatusSeeOther)
}

func handleDocumentAPI(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/document")
	if relPath == "" || relPath == "/" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}
	content, viewPath, err := resolveViewContent(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	files, err := collectFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect markdown files failed: %v", err), http.StatusInternalServerError)
		return
	}
	currentDir := filepath.Dir(viewPath)
	if currentDir == "." {
		currentDir = "/"
	}
	doc := mdutil.SplitDocument(content)
	bodyProcessed := sanitizeMermaid(doc.Body)
	bodyProcessed = rewriteLocalImages(bodyProcessed, currentDir)

	_, _, fmFields, hasFM := parseFrontmatter(content)
	writeJSON(w, map[string]any{
		"filePath":          viewPath,
		"rawPath":           "/raw" + viewPath,
		"contentHTML":       bodyProcessed,
		"contentText":       doc.Body,
		"fullContent":       content,
		"files":             files,
		"editing":           strings.HasPrefix(r.URL.Path, "/edit/"),
		"hasFrontmatter":    hasFM,
		"frontmatterFields": fmFields,
		"frontmatterRaw":    doc.Frontmatter,
	})
}

func handleRaw(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/raw")
	if relPath == "" || relPath == "/" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}

	content, viewPath, err := resolveViewContent(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	fileName := filepath.Base(viewPath)
	if !strings.HasSuffix(strings.ToLower(fileName), ".md") && !strings.HasSuffix(strings.ToLower(fileName), ".markdown") {
		fileName += ".md"
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	_, _ = w.Write([]byte(content))
}

func handleImages(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/images")
	relPath = filepath.Clean("/" + strings.TrimPrefix(relPath, "/"))
	absPath, err := safeJoin(mdRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read image failed: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", mimeType(absPath))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	_, _ = w.Write(content)
}

func handleContent(w http.ResponseWriter, r *http.Request) {
	serveMarkdownApp(w, r)
}

func handleContentAPI(w http.ResponseWriter, r *http.Request) {
	files := []mdFile{}
	if !mdContentOnly {
		collected, err := collectFiles()
		if err == nil {
			files = collected
		}
	}

	processed := sanitizeMermaid(mdContent)
	processed = rewriteLocalImages(processed, "/")
	name := contentFileName()
	writeJSON(w, map[string]any{
		"filePath":          "/" + name,
		"rawPath":           "/raw-content",
		"contentHTML":       processed,
		"contentText":       mdContent,
		"fullContent":       mdContent,
		"files":             files,
		"editing":           false,
		"hasFrontmatter":    false,
		"frontmatterFields": []frontmatterField{},
		"frontmatterRaw":    "",
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, fmt.Sprintf("write json failed: %v", err), http.StatusInternalServerError)
	}
}

func handleRawContent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", contentFileName()))
	_, _ = w.Write([]byte(mdContent))
}

type mdFile struct {
	Path           string
	Name           string
	Size           int64
	Relative       string
	Title          string
	Description    string
	HasFrontmatter bool
	FrontmatterNum int
}

func collectFiles() ([]mdFile, error) {
	files := make([]mdFile, 0)
	err := filepath.WalkDir(mdRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name(), path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isMarkdownFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(mdRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !matchesAny(rel, d.Name()) {
			return nil
		}
		content, _ := os.ReadFile(path)
		fmTitle, fmDesc, _, hasFM := parseFrontmatter(string(content))
		title, desc := extractMeta(string(content))
		if fmTitle != "" {
			title = fmTitle
		}
		if fmDesc != "" {
			desc = fmDesc
		}
		info, _ := d.Info()
		size := int64(len(content))
		if info != nil {
			size = info.Size()
		}
		files = append(files, mdFile{
			Path:           path,
			Name:           d.Name(),
			Size:           size,
			Relative:       "/" + rel,
			Title:          title,
			Description:    desc,
			HasFrontmatter: hasFM,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if mdContent != "" {
		fmTitle, fmDesc, _, hasFM := parseFrontmatter(mdContent)
		title, desc := extractMeta(mdContent)
		if fmTitle != "" {
			title = fmTitle
		}
		if fmDesc != "" {
			desc = fmDesc
		}
		files = append(files, mdFile{
			Path:           "/" + contentFileName(),
			Name:           contentFileName(),
			Size:           int64(len(mdContent)),
			Relative:       "/" + contentFileName(),
			Title:          title,
			Description:    desc,
			HasFrontmatter: hasFM,
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	return files, nil
}

func resolveViewContent(relPath string) (string, string, error) {
	if mdContent != "" {
		virtualPath := "/" + contentFileName()
		if relPath == virtualPath {
			return mdContent, virtualPath, nil
		}
	}
	absPath, err := safeJoin(mdRoot, relPath)
	if err != nil {
		return "", "", err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", "", fmt.Errorf("markdown file not found: %w", err)
	}
	rel, err := filepath.Rel(mdRoot, absPath)
	if err != nil {
		return "", "", err
	}
	return string(content), "/" + filepath.ToSlash(rel), nil
}

func safeJoin(root, relPath string) (string, error) {
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

func skipDir(name, path string) bool {
	if path == mdRoot {
		return false
	}
	if name == ".git" || name == "node_modules" || name == "vendor" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isMarkdownFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func matchesAny(relPath, fileName string) bool {
	if len(mdPatterns) == 0 {
		return true
	}
	for _, pattern := range mdPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if match, _ := filepath.Match(pattern, relPath); match {
			return true
		}
		if match, _ := filepath.Match(pattern, fileName); match {
			return true
		}
		if fi, err := os.Stat(filepath.Join(mdRoot, pattern)); err == nil && fi.IsDir() {
			prefix := strings.TrimPrefix(filepath.ToSlash(pattern), "./")
			if strings.HasPrefix(relPath, prefix+"/") || relPath == prefix {
				return true
			}
		}
		if strings.Contains(pattern, "**") {
			simple := strings.ReplaceAll(pattern, "**", "*")
			if match, _ := filepath.Match(simple, relPath); match {
				return true
			}
		}
	}
	return false
}

type frontmatterField struct {
	Key   string
	Value string
}

func parseFrontmatter(content string) (title, desc string, fields []frontmatterField, hasFM bool) {
	doc := mdutil.SplitDocument(content)
	if !doc.HasFrontmatter {
		return "", "", nil, false
	}
	hasFM = true
	if strings.TrimSpace(doc.Frontmatter) == "" {
		return "", "", fields, hasFM
	}
	raw, err := mdutil.ParseYAMLFrontmatter(doc.Frontmatter)
	if err != nil {
		return "", "", []frontmatterField{{Key: "error", Value: err.Error()}}, true
	}
	fields = flattenFrontmatter(raw)
	// 提取title和description
	for _, f := range fields {
		if strings.EqualFold(f.Key, "name") || strings.EqualFold(f.Key, "title") {
			if title == "" {
				title = f.Value
			}
		}
		if strings.EqualFold(f.Key, "description") || strings.EqualFold(f.Key, "summary") {
			if desc == "" {
				desc = f.Value
			}
		}
	}
	return title, desc, fields, hasFM
}

func flattenFrontmatter(value any) []frontmatterField {
	fields := make([]frontmatterField, 0)
	flattenFMInto("", value, func(key string, val any) {
		fields = append(fields, frontmatterField{Key: key, Value: formatFMValue(val)})
	})
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	return fields
}

func flattenFMInto(prefix string, value any, emit func(string, any)) {
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
			flattenFMInto(next, v[key], emit)
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
			flattenFMInto(next, lookup[key], emit)
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
			flattenFMInto(next, item, emit)
		}
	default:
		if strings.TrimSpace(prefix) == "" {
			prefix = "value"
		}
		emit(prefix, v)
	}
}

func formatFMValue(value any) string {
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

func extractMeta(content string) (string, string) {
	lines := strings.Split(content, "\n")
	foundTitle := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !foundTitle && strings.HasPrefix(trimmed, "#") {
			foundTitle = true
			continue
		}
		if foundTitle {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "#") {
				continue
			}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "+") {
				continue
			}
			desc := trimmed
			if len(desc) > 120 {
				desc = desc[:120] + "..."
			}
			if desc != "" {
				return firstHeading(content), desc
			}
		}
	}
	title := firstHeading(content)
	if title == "" {
		title = "Untitled Document"
	}
	return title, "No description"
}

func firstHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func contentFileName() string {
	title := firstHeading(mdContent)
	if title == "" {
		return "document.md"
	}
	name := strings.ToLower(title)
	name = strings.ReplaceAll(name, " ", "-")
	name = regexp.MustCompile(`[^a-z0-9\-]`).ReplaceAllString(name, "")
	if name == "" {
		return "document.md"
	}
	return name + ".md"
}

func sanitizeMermaid(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	inMermaid := false
	isClassDiagram := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```mermaid") {
			inMermaid = true
			isClassDiagram = false
			result.WriteString(line + "\n")
			continue
		}
		if inMermaid && strings.HasPrefix(trimmed, "```") {
			inMermaid = false
			isClassDiagram = false
			result.WriteString(line + "\n")
			continue
		}
		if inMermaid {
			if strings.HasPrefix(trimmed, "classDiagram") {
				isClassDiagram = true
			}
			line = strings.ReplaceAll(line, "interface{}", "any")
			line = strings.ReplaceAll(line, "map[string]interface{}", "map[string]any")
			line = strings.ReplaceAll(line, "[]interface{}", "[]any")
			line = strings.ReplaceAll(line, "...interface{}", "...any")
			line = strings.ReplaceAll(line, "chan interface{}", "chan any")
			if isClassDiagram {
				line = trimMermaidPtr(line)
			}
		}
		result.WriteString(line + "\n")
	}
	return result.String()
}

func trimMermaidPtr(line string) string {
	patterns := []string{"-->", "<--", "..", "*--", "o--", "--o", "--*", "<|--", "--|>", "<|..", "..|>"}
	for _, pattern := range patterns {
		if !strings.Contains(line, pattern) {
			continue
		}
		parts := strings.Split(line, pattern)
		if len(parts) != 2 {
			continue
		}
		right := strings.TrimSpace(parts[1])
		if strings.HasPrefix(right, "*") {
			tokens := strings.Fields(right)
			if len(tokens) > 0 {
				tokens[0] = strings.TrimPrefix(tokens[0], "*")
				right = strings.Join(tokens, " ")
			}
		}
		return parts[0] + pattern + " " + right
	}
	return line
}

func rewriteLocalImages(content, currentDir string) string {
	var result strings.Builder
	for i := 0; i < len(content); i++ {
		if i+1 < len(content) && content[i] == '!' && content[i+1] == '[' {
			start := i
			i += 2
			altEnd := strings.Index(content[i:], "]")
			if altEnd == -1 {
				result.WriteString(content[start:i])
				continue
			}
			altText := content[i : i+altEnd]
			i += altEnd + 1
			if i >= len(content) || content[i] != '(' {
				result.WriteString(content[start:i])
				continue
			}
			i++
			pathEnd := strings.Index(content[i:], ")")
			if pathEnd == -1 {
				result.WriteString(content[start:i])
				continue
			}
			imagePath := content[i : i+pathEnd]
			i += pathEnd
			if strings.HasPrefix(strings.ToLower(imagePath), "http://") || strings.HasPrefix(strings.ToLower(imagePath), "https://") {
				result.WriteString(fmt.Sprintf("![%s](%s)", altText, imagePath))
				continue
			}
			imagePath = strings.Split(imagePath, "?")[0]
			imagePath = strings.Split(imagePath, "#")[0]
			resolved := imagePath
			if !strings.HasPrefix(resolved, "/") {
				baseDir := strings.TrimPrefix(currentDir, "/")
				resolved = filepath.Join(baseDir, imagePath)
			}
			resolved = filepath.ToSlash(filepath.Clean(resolved))
			if !strings.HasPrefix(resolved, "/") {
				resolved = "/" + resolved
			}
			result.WriteString(fmt.Sprintf("![%s](/images%s)", altText, resolved))
			continue
		}
		result.WriteByte(content[i])
	}
	return result.String()
}

func mimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".tif", ".tiff":
		return "image/tiff"
	default:
		return "application/octet-stream"
	}
}

func openBrowser(url string) {
	cmd := exec.Command("open", url)
	if err := cmd.Run(); err != nil {
		fmt.Printf("open browser failed: %v\n", err)
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, OriginPatterns: []string{"*"}})
	if err != nil {
		log.Printf("websocket accept failed: %v", err)
		return
	}
	mdClientsMu.Lock()
	mdClients[conn] = true
	mdClientsMu.Unlock()
	ctx := context.Background()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connected"}`))
	defer func() {
		mdClientsMu.Lock()
		delete(mdClients, conn)
		mdClientsMu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func broadcastReload(message string) {
	mdClientsMu.Lock()
	defer mdClientsMu.Unlock()
	payload := fmt.Sprintf(`{"type":"reload","message":%q}`, message)
	ctx := context.Background()
	for conn := range mdClients {
		_ = conn.Write(ctx, websocket.MessageText, []byte(payload))
	}
}

func closeClients() {
	mdClientsMu.Lock()
	defer mdClientsMu.Unlock()
	for conn := range mdClients {
		_ = conn.Close(websocket.StatusNormalClosure, "server closed")
	}
	mdClients = make(map[*websocket.Conn]bool)
}

func initWatcher() error {
	mdWatchMu.Lock()
	defer mdWatchMu.Unlock()
	if mdWatcher != nil {
		_ = mdWatcher.Close()
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	mdWatcher = watcher
	dirs := make(map[string]bool)
	_ = filepath.WalkDir(mdRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name(), path) {
				return filepath.SkipDir
			}
			dirs[path] = true
		}
		return nil
	})
	for dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			log.Printf("watch dir failed %s: %v", dir, err)
		}
	}
	go watchFiles()
	return nil
}

func watchFiles() {
	debounce := map[string]*time.Timer{}
	var mu sync.Mutex
	for {
		mdWatchMu.Lock()
		watcher := mdWatcher
		mdWatchMu.Unlock()
		if watcher == nil {
			return
		}
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			name := filepath.Base(event.Name)
			if !isMarkdownFile(name) {
				continue
			}
			rel, err := filepath.Rel(mdRoot, event.Name)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if !matchesAny(rel, name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			mu.Lock()
			if timer, exists := debounce[event.Name]; exists {
				timer.Stop()
			}
			debounce[event.Name] = time.AfterFunc(300*time.Millisecond, func() {
				broadcastReload(filepath.Base(event.Name) + " changed")
				mu.Lock()
				delete(debounce, event.Name)
				mu.Unlock()
			})
			mu.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func cleanupWatcher() {
	mdWatchMu.Lock()
	defer mdWatchMu.Unlock()
	if mdWatcher != nil {
		_ = mdWatcher.Close()
		mdWatcher = nil
	}
}

func getwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
