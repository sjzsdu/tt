package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
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
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/list", handleList)
	mux.HandleFunc("/view/", handleView)
	mux.HandleFunc("/edit/", handleEdit)
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

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if mdContent != "" {
		handleContent(w, r)
		return
	}
	handleList(w, r)
}

func handleList(w http.ResponseWriter, r *http.Request) {
	files, err := collectFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect markdown files failed: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Files       []mdFile
		FilesJSON   template.JS
		Total       int
	}{Files: files, Total: len(files)}
	if filesJSON, err := json.Marshal(files); err == nil {
		data.FilesJSON = template.JS(filesJSON)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("list").Parse(listHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render list failed: %v", err), http.StatusInternalServerError)
	}
}

func handleView(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/view")
	if relPath == "" || relPath == "/" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := renderMarkdownPage(w, relPath, false); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	}
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/edit")
	if relPath == "" || relPath == "/" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := renderMarkdownPage(w, relPath, true); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	}
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
	broadcastReload(filepath.Base(absPath) + " deleted")
	http.Redirect(w, r, "/list", http.StatusSeeOther)
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
	content := r.FormValue("content")
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("save markdown failed: %v", err), http.StatusInternalServerError)
		return
	}
	broadcastReload(filepath.Base(absPath) + " saved")
	http.Redirect(w, r, "/view"+relPath, http.StatusSeeOther)
}


func renderMarkdownPage(w http.ResponseWriter, relPath string, editing bool) error {
	content, viewPath, err := resolveViewContent(relPath)
	if err != nil {
		return err
	}
	files, err := collectFiles()
	if err != nil {
		return fmt.Errorf("collect markdown files failed: %w", err)
	}
	filesJSON, _ := json.Marshal(files)
	currentDir := filepath.Dir(viewPath)
	if currentDir == "." {
		currentDir = "/"
	}
	processed := sanitizeMermaid(content)
	processed = rewriteLocalImages(processed, currentDir)
	data := struct {
		FilePath    string
		RawPath     string
		ContentHTML template.HTML
		ContentText string
		Files       []mdFile
		FilesJSON   template.JS
		Editing     bool
	}{
		FilePath:    viewPath,
		RawPath:     "/raw" + viewPath,
		ContentHTML: template.HTML(processed),
		ContentText: content,
		Files:       files,
		FilesJSON:   template.JS(filesJSON),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("view").Parse(viewHTML)).Execute(w, data); err != nil {
		return fmt.Errorf("render view failed: %w", err)
	}
	return nil
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
	data := struct {
		FilePath string
		RawPath  string
		Content  template.HTML
		Files    []mdFile
	}{
		FilePath: "/" + name,
		RawPath:  "/raw-content",
		Content:  template.HTML(processed),
		Files:    files,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("view").Parse(viewHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render content failed: %v", err), http.StatusInternalServerError)
	}
}

func handleRawContent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", contentFileName()))
	_, _ = w.Write([]byte(mdContent))
}

type mdFile struct {
	Path        string
	Name        string
	Size        int64
	Relative    string
	Title       string
	Description string
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
		title, desc := extractMeta(string(content))
		info, _ := d.Info()
		size := int64(len(content))
		if info != nil {
			size = info.Size()
		}
		files = append(files, mdFile{
			Path:        path,
			Name:        d.Name(),
			Size:        size,
			Relative:    "/" + rel,
			Title:       title,
			Description: desc,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if mdContent != "" {
		title, desc := extractMeta(mdContent)
		files = append(files, mdFile{
			Path:        "/" + contentFileName(),
			Name:        contentFileName(),
			Size:        int64(len(mdContent)),
			Relative:    "/" + contentFileName(),
			Title:       title,
			Description: desc,
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

const listHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Markdown</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #f6f8fa; color: #24292f; }
    .wrap { max-width: 1100px; margin: 0 auto; padding: 32px 20px; }
    .card { background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,.06); padding: 24px; }
    .file { padding: 16px 0; border-bottom: 1px solid #d8dee4; }
    .file:last-child { border-bottom: 0; }
    .path { font-size: 14px; color: #57606a; }
    .title { margin: 0 0 6px; font-size: 18px; }
    a { color: #0969da; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .desc { color: #57606a; margin-top: 8px; }
    .meta { font-size: 12px; color: #8c959f; margin-top: 6px; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>Markdown Files</h1>
      <p>Total: {{.Total}}</p>
      {{range .Files}}
      <div class="file">
        <h2 class="title"><a href="/view{{.Relative}}">{{if .Title}}{{.Title}}{{else}}{{.Name}}{{end}}</a></h2>
        <div class="path">{{.Relative}}</div>
        <div class="desc">{{.Description}}</div>
        <div class="meta">{{.Size}} bytes · <a href="/raw{{.Relative}}">download raw</a></div>
      </div>
      {{else}}
      <p>No markdown files found.</p>
      {{end}}
    </div>
  </div>
</body>
</html>`

const viewHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.FilePath}}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/github-markdown-css@5.2.0/github-markdown.min.css">
  <script src="https://cdn.jsdelivr.net/npm/marked@9.1.2/marked.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/mermaid@11.14.0/dist/mermaid.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/canvg@4.0.1/lib/umd.js"></script>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f8fb;
      --panel: #ffffff;
      --text: #1f2937;
      --subtle: #667085;
      --line: #e5e7eb;
      --line-strong: #d0d7de;
      --brand: #2563eb;
      --brand-soft: #eff6ff;
      --brand-strong: #1d4ed8;
      --green: #059669;
      --green-soft: #ecfdf3;
      --shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
      --radius: 16px;
    }
    * { box-sizing: border-box; }
    html, body { margin: 0; height: 100%; background: var(--bg); color: var(--text); font-family: -apple-system, BlinkMacSystemFont, sans-serif; }
    body { overflow: hidden; }
    a { color: inherit; }
    .layout { display: grid; grid-template-columns: 280px minmax(0, 1fr) 260px; height: 100vh; overflow: hidden; }
    .pane { min-height: 0; }
    .files-pane, .toc-pane { background: var(--panel); height: 100vh; overflow-y: auto; position: sticky; top: 0; }
    .files-pane { border-right: 1px solid var(--line); }
    .toc-pane { border-left: 1px solid var(--line); }
    .content-pane { background: var(--bg); overflow-y: auto; height: 100vh; }
    .files-pane::-webkit-scrollbar, .toc-pane::-webkit-scrollbar, .content-pane::-webkit-scrollbar { width: 10px; }
    .files-pane::-webkit-scrollbar-thumb, .toc-pane::-webkit-scrollbar-thumb, .content-pane::-webkit-scrollbar-thumb { background: #cbd5e1; border-radius: 999px; border: 2px solid transparent; background-clip: padding-box; }
    .files-pane::-webkit-scrollbar-track, .toc-pane::-webkit-scrollbar-track, .content-pane::-webkit-scrollbar-track { background: transparent; }
    .section { padding: 20px 18px; }
    .section-title, .toc-title { margin: 0; font-size: 15px; font-weight: 700; }
    .section-subtitle, .toc-empty { margin: 8px 0 0; color: var(--subtle); font-size: 12px; line-height: 1.6; }
    .file-toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      margin: 14px 0 12px;
    }
    .file-view-toggle {
      display: inline-flex;
      padding: 2px;
      border-radius: 12px;
      background: #f8fafc;
      border: 1px solid #e5e7eb;
      box-shadow: inset 0 1px 0 rgba(255,255,255,.9);
      overflow: hidden;
    }
    .file-view-toggle .btn {
      padding: 6px 11px;
      font-size: 12px;
      box-shadow: none;
      border-radius: 10px;
      border-color: transparent;
      background: transparent;
      color: #475467;
      min-width: 52px;
    }
    .file-view-toggle .btn:hover {
      background: rgba(255,255,255,.72);
      color: #111827;
    }
    .file-view-toggle .btn.active {
      background: #fff;
      color: var(--brand-strong);
      border-color: #dbeafe;
      box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
    }
    .file-list, .toc-list { list-style: none; padding: 0; margin: 16px 0 0; }
    .file-item { margin: 6px 0; }
    .tree-list {
      list-style: none;
      padding: 0;
      margin: 14px 0 0;
      border-top: 1px solid #edf2f7;
    }
    .tree-node { margin: 0; }
    .tree-row {
      display: flex;
      align-items: center;
      gap: 8px;
      min-height: 36px;
      padding: 0 10px 0 12px;
      border-bottom: 1px solid #f3f6fb;
      color: #334155;
    }
    .tree-row.folder-row {
      background: #fff;
      font-weight: 600;
    }
    .tree-row.folder-row:hover { background: #f8fafc; }
    .tree-spacer { width: 14px; flex: none; }
    .tree-toggle {
      width: 18px;
      height: 18px;
      border: none;
      background: transparent;
      color: #94a3b8;
      cursor: pointer;
      padding: 0;
      font-size: 12px;
      line-height: 1;
      border-radius: 999px;
    }
    .tree-toggle:hover { background: #eef2ff; color: #334155; }
    .tree-folder-name { font-weight: 500; }
    .tree-folder-name::before {
      content: '';
      display: none;
    }
    .tree-file-name { padding-left: 2px; }
    .tree-file-name::before {
      content: '';
      display: none;
    }
    .tree-file-name:hover {
      background: #f8fafc;
      color: var(--green);
    }
    .tree-file-name.active {
      background: var(--green-soft);
      color: var(--green);
      font-weight: 600;
    }
    .tree-children {
      list-style: none;
      margin: 0;
      padding: 0 0 0 18px;
      border-left: 1px solid #edf2f7;
      background: #fff;
      overflow: hidden;
      max-height: 2000px;
      opacity: 1;
      transition: max-height .22s ease, opacity .18s ease;
    }
    .tree-children.collapsed {
      max-height: 0;
      opacity: 0;
    }
    .tree-children .tree-row { min-height: 34px; }
    .file-item, .toc-item { margin: 6px 0; }
    .file-link {
      display: block;
      padding: 10px 12px;
      border-radius: 12px;
      color: #334155;
      text-decoration: none;
      font-size: 13px;
      line-height: 1.45;
      border: 1px solid transparent;
      transition: .2s ease;
      word-break: break-word;
    }
    .file-link:hover {
      background: #f8fafc;
      border-color: #e2e8f0;
      color: var(--green);
      transform: translateX(2px);
    }
    .file-link.active {
      background: var(--green-soft);
      border-color: #a7f3d0;
      color: var(--green);
      font-weight: 600;
      box-shadow: inset 0 0 0 1px rgba(5, 150, 105, 0.08);
    }
    .toolbar {
      position: sticky;
      top: 0;
      z-index: 10;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 16px 20px;
      background: rgba(246, 248, 251, 0.92);
      backdrop-filter: blur(10px);
      border-bottom: 1px solid var(--line);
    }
    .toolbar-title { min-width: 0; }
    .toolbar-title strong { display: block; font-size: 16px; line-height: 1.4; word-break: break-word; }
    .toolbar-title span { display: block; margin-top: 4px; color: var(--subtle); font-size: 12px; }
    .toolbar-actions {
      display: inline-flex;
      align-items: center;
      justify-content: flex-end;
      gap: 4px;
      flex-wrap: wrap;
      padding: 3px;
      border: 1px solid #e5e7eb;
      border-radius: 13px;
      background: rgba(255,255,255,.78);
      box-shadow: inset 0 1px 0 rgba(255,255,255,.9), 0 1px 2px rgba(15,23,42,.03);
    }
    .toolbar-actions > * { flex: none; }
    .toolbar-actions form { display: inline-flex; margin: 0; align-items: center; }
    .toolbar-actions form .btn { margin: 0; }
    .btn {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 7px 10px;
      border-radius: 10px;
      text-decoration: none;
      font-size: 13px;
      font-weight: 600;
      border: 1px solid transparent;
      transition: background .18s ease, border-color .18s ease, color .18s ease, transform .18s ease, box-shadow .18s ease;
      white-space: nowrap;
      box-shadow: none;
    }
    .btn:hover { transform: translateY(-1px); }
    .btn svg { width: 14px; height: 14px; flex: none; }
    .btn-secondary { color: #0f172a; background: #fff; border-color: transparent; }
    .btn-secondary:hover { background: #f8fafc; border-color: #e2e8f0; color: #0f172a; }
    .btn-outline { color: #475467; background: transparent; border-color: transparent; }
    .btn-outline:hover { background: #fff; border-color: #e5e7eb; color: #111827; }
    .btn-danger { color: #b42318; background: transparent; border-color: transparent; }
    .btn-danger:hover { background: #fff5f5; border-color: #fecaca; color: #7f1d1d; }
    .doc-wrap { padding: 24px; }
    .editor-form {
      max-width: 960px;
      margin: 0 auto;
      padding: 24px;
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
    }
    .editor-actions {
      display: flex;
      gap: 10px;
      justify-content: flex-end;
      flex-wrap: wrap;
      margin-bottom: 14px;
    }
    .editor-textarea {
      width: 100%;
      min-height: 72vh;
      resize: vertical;
      border: 1px solid var(--line-strong);
      border-radius: 14px;
      padding: 20px;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 14px;
      line-height: 1.7;
      color: var(--text);
      background: #fbfdff;
      outline: none;
    }
    .editor-textarea:focus {
      border-color: #93c5fd;
      box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.12);
    }
    .editor-hint {
      margin-top: 10px;
      color: var(--subtle);
      font-size: 12px;
      line-height: 1.6;
    }
    .markdown-body {
      max-width: 960px;
      margin: 0 auto;
      padding: 36px 40px;
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
    }
    .markdown-body h1, .markdown-body h2, .markdown-body h3, .markdown-body h4 { scroll-margin-top: 24px; }
    .markdown-body img { max-width: 100%; height: auto; }
    .markdown-body pre { overflow-x: auto; }
    .markdown-body table { display: block; overflow-x: auto; }
    .mermaid-figure {
      margin: 20px 0;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: linear-gradient(180deg, #f8fafc 0%, #eef4ff 100%);
      box-shadow: 0 18px 40px rgba(15, 23, 42, .08);
      overflow: hidden;
    }
    .mermaid-toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      padding: 8px 12px;
      border-bottom: 1px solid rgba(148, 163, 184, .22);
      background: rgba(255, 255, 255, .82);
      backdrop-filter: blur(12px);
    }
    .mermaid-toolbar-text {
      font-size: 12px;
      color: #475467;
      letter-spacing: .01em;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 42%;
    }
    .mermaid-toolbar-actions {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    .mermaid-tool-btn {
      appearance: none;
      border: 1px solid rgba(37, 99, 235, .18);
      background: rgba(255, 255, 255, .96);
      color: #1d4ed8;
      border-radius: 999px;
      min-width: 30px;
      height: 30px;
      padding: 0 10px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 12px;
      font-weight: 700;
      cursor: pointer;
      transition: transform .18s ease, box-shadow .18s ease, background .18s ease;
      box-shadow: 0 6px 14px rgba(37, 99, 235, .10);
    }
    .mermaid-tool-btn:hover {
      transform: translateY(-1px);
      background: #eff6ff;
      box-shadow: 0 10px 18px rgba(37, 99, 235, .14);
    }
    .mermaid-tool-btn:active { transform: translateY(0); }
    .mermaid-viewport {
      position: relative;
      min-height: 180px;
      overflow: hidden;
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: grab;
      background:
        radial-gradient(circle at top, rgba(255,255,255,.92), rgba(239,244,255,.72) 55%, rgba(226,232,240,.68)),
        linear-gradient(90deg, rgba(148,163,184,.12) 1px, transparent 1px),
        linear-gradient(rgba(148,163,184,.12) 1px, transparent 1px);
      background-size: auto, 24px 24px, 24px 24px;
      background-position: center, center, center;
    }
    .mermaid {
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 14px;
      min-height: 0;
      width: 100%;
      overflow: hidden;
    }
    .mermaid svg {
      display: block;
      max-width: 100%;
      height: auto;
      transform-origin: center center;
      user-select: none;
      touch-action: none;
    }
    .toc-item.level-3 { padding-left: 12px; }
    .toc-item.level-4 { padding-left: 24px; }
    .toc-link {
      display: block;
      padding: 8px 10px;
      border-left: 3px solid transparent;
      border-radius: 8px;
      color: #475467;
      text-decoration: none;
      font-size: 13px;
      line-height: 1.45;
      transition: .2s ease;
      word-break: break-word;
    }
    .toc-link:hover { color: var(--brand); background: #f8fafc; border-left-color: #93c5fd; }
    .toc-link.active { color: var(--brand-strong); background: var(--brand-soft); border-left-color: var(--brand); font-weight: 600; }
    @media (max-width: 1200px) {
      .layout { grid-template-columns: 250px minmax(0, 1fr) 220px; }
      .markdown-body { padding: 30px 28px; }
    }
    @media (max-width: 960px) {
      body { overflow: auto; }
      .layout { grid-template-columns: 1fr; height: auto; }
      .pane { overflow: visible; }
      .files-pane, .toc-pane { border: 0; border-bottom: 1px solid var(--line); height: auto; overflow: visible; }
      .content-pane { overflow: visible; }
      .toolbar { flex-direction: column; align-items: flex-start; }
      .toolbar-actions { justify-content: flex-start; }
      .doc-wrap { padding: 16px; }
      .markdown-body { padding: 24px 20px; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <aside class="pane files-pane">
      <div class="section">
        <h3 class="section-title">Markdown Files</h3>
        <p class="section-subtitle">File list and document content scroll independently, so the file tree stays visible while reading.</p>
        <div class="file-toolbar">
          <div></div>
          <div class="file-view-toggle">
            <button type="button" class="btn btn-outline active" id="flat-view-btn">Flat</button>
            <button type="button" class="btn btn-outline" id="tree-view-btn">Tree</button>
          </div>
        </div>
        <ul class="file-list" id="file-list"></ul>
        <script id="files-data" type="application/json">{{.FilesJSON}}</script>
      </div>
    </aside>
    <main class="pane content-pane" id="content-pane">
      <div class="toolbar">
        <div class="toolbar-title">
          <strong>{{.FilePath}}</strong>
          <span>{{if .Editing}}Editing markdown source{{else}}Markdown preview with Mermaid support{{end}}</span>
        </div>
        <div class="toolbar-actions">
          <a class="btn btn-secondary" href="/list">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>
            <span>File list</span>
          </a>
          {{if .Editing}}
          <a class="btn btn-outline" href="/view{{.FilePath}}">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M7 8l-4 4 4 4M3 12h18"/></svg>
            <span>Preview</span>
          </a>
          {{else}}
          <a class="btn btn-outline" href="/edit{{.FilePath}}">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M16.862 3.487a2.1 2.1 0 1 1 2.97 2.97L7.5 18.789 3 20l1.211-4.5L16.862 3.487Z"/></svg>
            <span>Edit</span>
          </a>
          <form method="post" action="/delete{{.FilePath}}" onsubmit="return confirm('Delete this markdown file? This cannot be undone.')">
            <button class="btn btn-danger" type="submit">Delete</button>
          </form>
          {{end}}
        </div>
      </div>
      <div class="doc-wrap">
        {{if .Editing}}
        <form class="editor-form" method="post" action="/save{{.FilePath}}">
          <div class="editor-actions">
            <a class="btn btn-outline" href="/view{{.FilePath}}">Cancel</a>
            <button class="btn btn-secondary" type="submit">Save</button>
            <button class="btn btn-danger" type="submit" formaction="/delete{{.FilePath}}" formmethod="post" onclick="return confirm('Delete this markdown file? This cannot be undone.')">Delete</button>
          </div>
          <textarea class="editor-textarea" name="content" spellcheck="false">{{.ContentText}}</textarea>
          <div class="editor-hint">保存后会直接写回原文件，并刷新页面列表。</div>
        </form>
        {{else}}
        <article id="content" class="markdown-body"></article>
        <textarea hidden id="markdown-content">{{.ContentText}}</textarea>
        {{end}}
      </div>
    </main>
    <aside class="pane toc-pane">
      <div class="section">
        <h3 class="toc-title">On this page</h3>
        <p class="section-subtitle">Jump between headings and keep the current section highlighted while scrolling.</p>
        <ul class="toc-list" id="toc-list"></ul>
        <div class="toc-empty" id="toc-empty">No headings detected in this document yet.</div>
      </div>
    </aside>
  </div>
  <script>
    const filesData = JSON.parse(document.getElementById('files-data')?.textContent || '[]');
    const fileList = document.getElementById('file-list');
    const flatViewBtn = document.getElementById('flat-view-btn');
    const treeViewBtn = document.getElementById('tree-view-btn');
    const treeStateKey = 'md-tree-expanded';
    const expandedPaths = new Set(JSON.parse(localStorage.getItem(treeStateKey) || '[]'));
    const currentPagePath = '{{.FilePath}}';
    let fileViewMode = localStorage.getItem('md-file-view-mode') || 'flat';
    function fileDisplayName(file) {
      return file.Title || file.Relative || file.Name || 'Untitled';
    }

    function renderFlatFiles() {
      fileList.innerHTML = '';
      if (!filesData.length) {
        fileList.innerHTML = '<li class="toc-empty">No markdown files</li>';
        return;
      }
      filesData.forEach((file) => {
        const li = document.createElement('li');
        li.className = 'file-item';
        const a = document.createElement('a');
        a.className = 'file-link' + (file.Relative === '{{.FilePath}}' ? ' active' : '');
        a.href = '/view' + file.Relative;
        a.textContent = fileDisplayName(file);
        li.appendChild(a);
        fileList.appendChild(li);
      });
    }


    function buildTree(files) {
      const root = { name: '', children: new Map(), files: [] };
      for (const file of files) {
        const parts = String(file.Relative || '').replace(/^\//, '').split('/').filter(Boolean);
        let node = root;
        parts.forEach((part, index) => {
          if (index === parts.length - 1) {
            node.files.push(file);
            return;
          }
          if (!node.children.has(part)) {
            node.children.set(part, { name: part, children: new Map(), files: [] });
          }
          node = node.children.get(part);
        });
      }
      return root;
    }

    function renderTreeNode(node, depth = 0, parentPath = '') {
      const ul = document.createElement('ul');
      ul.className = depth === 0 ? 'tree-list' : 'tree-children';
      [...node.children.values()].sort((a, b) => a.name.localeCompare(b.name)).forEach((child) => {
        const childPath = parentPath ? parentPath + '/' + child.name : child.name;
        const isExpanded = expandedPaths.has(childPath) || child.files.length > 0;
        const li = document.createElement('li');
        li.className = 'tree-node';
        const row = document.createElement('div');
        row.className = 'tree-row folder-row';
        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'tree-toggle';
        toggle.textContent = isExpanded ? '▾' : '▸';
        toggle.setAttribute('aria-label', (isExpanded ? 'Collapse ' : 'Expand ') + child.name);
        const label = document.createElement('span');
        label.className = 'tree-folder-name';
        label.textContent = child.name;
        row.append(toggle, label);
        li.appendChild(row);
        const childList = renderTreeNode(child, depth + 1, childPath);
        if (!isExpanded) childList.classList.add('collapsed');
        li.appendChild(childList);
        const persist = () => {
          localStorage.setItem(treeStateKey, JSON.stringify(Array.from(expandedPaths)));
        };
        toggle.addEventListener('click', () => {
          const collapsed = childList.classList.toggle('collapsed');
          if (collapsed) expandedPaths.delete(childPath);
          else expandedPaths.add(childPath);
          toggle.textContent = collapsed ? '▸' : '▾';
          toggle.setAttribute('aria-label', (collapsed ? 'Expand ' : 'Collapse ') + child.name);
          persist();
        });
        ul.appendChild(li);
      });
      node.files.sort((a, b) => fileDisplayName(a).localeCompare(fileDisplayName(b))).forEach((file) => {
        const li = document.createElement('li');
        li.className = 'tree-node';
        const row = document.createElement('div');
        row.className = 'tree-row';
        const spacer = document.createElement('span');
        spacer.className = 'tree-spacer';
        const a = document.createElement('a');
        a.className = 'file-link tree-file-name' + (file.Relative === currentPagePath ? ' active' : '');
        a.href = '/view' + file.Relative;
        a.textContent = fileDisplayName(file);
        row.append(spacer, a);
        li.appendChild(row);
        ul.appendChild(li);
      });
      return ul;
    }

    function renderTreeFiles() {
      fileList.innerHTML = '';
      if (!filesData.length) {
        fileList.innerHTML = '<li class="toc-empty">No markdown files</li>';
        return;
      }
      const tree = buildTree(filesData);
      fileList.appendChild(renderTreeNode(tree));
    }

    function setFileViewMode(mode) {
      fileViewMode = mode;
      localStorage.setItem('md-file-view-mode', mode);
      flatViewBtn.classList.toggle('active', mode === 'flat');
      treeViewBtn.classList.toggle('active', mode === 'tree');
      if (mode === 'tree') renderTreeFiles();
      else renderFlatFiles();
    }

    flatViewBtn.addEventListener('click', () => setFileViewMode('flat'));
    treeViewBtn.addEventListener('click', () => setFileViewMode('tree'));
    setFileViewMode(fileViewMode);
    const sourceEl = document.getElementById('markdown-content');
    const source = sourceEl ? (sourceEl.value || '') : '';
    const tocList = document.getElementById('toc-list');
    const tocEmpty = document.getElementById('toc-empty');
    const contentPane = document.getElementById('content-pane');
    const isEditing = {{if .Editing}}true{{else}}false{{end}};
    const viewPath = '/view{{.FilePath}}';

    marked.setOptions({
      gfm: true,
      breaks: true,
      headerIds: false,
      mangle: false,
      highlight(code, lang) {
        if (lang === 'mermaid') return code;
        return code;
      }
    });

    function slugify(text) {
      return String(text || '')
        .toLowerCase()
        .trim()
        .replace(/[^\w\u4e00-\u9fa5\s-]/g, '')
        .replace(/\s+/g, '-')
        .replace(/-+/g, '-');
    }

    function decorateHeadings() {
      const used = new Map();
      const headings = content.querySelectorAll('h1, h2, h3, h4');
      headings.forEach((heading) => {
        const level = Number(heading.tagName.slice(1));
        const base = slugify(heading.textContent) || 'section';
        const count = used.get(base) || 0;
        used.set(base, count + 1);
        heading.id = count === 0 ? base : (base + '-' + count);
        heading.dataset.level = String(level);
      });
      return headings;
    }

    function buildTOC(headings) {
      tocList.innerHTML = '';
      if (!headings.length) {
        tocEmpty.style.display = 'block';
        return;
      }
      tocEmpty.style.display = 'none';
      headings.forEach((heading) => {
        const level = Math.min(Math.max(Number(heading.dataset.level || 1), 1), 4);
        const item = document.createElement('li');
        item.className = 'toc-item level-' + level;
        const link = document.createElement('a');
        link.className = 'toc-link';
        link.href = '#' + heading.id;
        link.dataset.target = heading.id;
        link.textContent = heading.textContent.trim();
        link.addEventListener('click', (event) => {
          event.preventDefault();
          heading.scrollIntoView({ behavior: 'smooth', block: 'start' });
          history.replaceState(null, '', '#' + heading.id);
        });
        item.appendChild(link);
        tocList.appendChild(item);
      });
    }

    function activateTOC() {
      const links = Array.from(document.querySelectorAll('.toc-link'));
      const headings = links.map((link) => document.getElementById(link.dataset.target)).filter(Boolean);
      if (!links.length || !headings.length) return;

      const setActive = (id) => {
        links.forEach((link) => link.classList.toggle('active', link.dataset.target === id));
      };

      const updateActive = () => {
        let active = headings[0];
        for (const heading of headings) {
          const rect = heading.getBoundingClientRect();
          if (rect.top <= 140) active = heading;
          else break;
        }
        setActive(active.id);
      };

      contentPane.addEventListener('scroll', updateActive, { passive: true });
      window.addEventListener('hashchange', updateActive);
      updateActive();
    }

    function renderMarkdown() {
      if (!content) return;
      content.innerHTML = marked.parse(source);
      const headings = Array.from(decorateHeadings());
      buildTOC(headings);
      activateTOC();
    }

    async function ensureMermaidLoaded() {
      if (window.mermaid) return window.mermaid;
      const urls = [
        'https://cdn.jsdelivr.net/npm/mermaid@11.14.0/dist/mermaid.min.js',
        'https://unpkg.com/mermaid@11.14.0/dist/mermaid.min.js'
      ];
      for (const url of urls) {
        try {
          await new Promise((resolve, reject) => {
            const script = document.createElement('script');
            script.src = url;
            script.async = true;
            script.onload = resolve;
            script.onerror = reject;
            document.head.appendChild(script);
          });
          if (window.mermaid) return window.mermaid;
        } catch (_) {}
      }
      return null;
    }

    async function renderMermaid() {
      const mermaidApi = await ensureMermaidLoaded();
      if (!mermaidApi) return;
      mermaidApi.initialize({
        startOnLoad: false,
        theme: 'default',
        securityLevel: 'loose',
        logLevel: 'error',
        flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' },
        sequence: { useMaxWidth: true },
        journey: { useMaxWidth: true },
        er: { useMaxWidth: true },
        pie: { useMaxWidth: true },
      });
      const makeMermaidFigure = (sourceText, index) => {
        const shell = document.createElement('section');
        shell.className = 'mermaid-figure';

        const toolbar = document.createElement('div');
        toolbar.className = 'mermaid-toolbar';

        const label = document.createElement('div');
        label.className = 'mermaid-toolbar-text';
        label.textContent = 'Drag to move';

        const actions = document.createElement('div');
        actions.className = 'mermaid-toolbar-actions';

        const zoomOut = document.createElement('button');
        zoomOut.type = 'button';
        zoomOut.className = 'mermaid-tool-btn';
        zoomOut.setAttribute('aria-label', 'Zoom out Mermaid diagram');
        zoomOut.textContent = '−';

        const reset = document.createElement('button');
        reset.type = 'button';
        reset.className = 'mermaid-tool-btn';
        reset.setAttribute('aria-label', 'Reset Mermaid diagram position and zoom');
        reset.textContent = 'R';

        const zoomIn = document.createElement('button');
        zoomIn.type = 'button';
        zoomIn.className = 'mermaid-tool-btn';
        zoomIn.setAttribute('aria-label', 'Zoom in Mermaid diagram');
        zoomIn.textContent = '+';

        const exportSvg = document.createElement('button');
        exportSvg.type = 'button';
        exportSvg.className = 'mermaid-tool-btn';
        exportSvg.setAttribute('aria-label', 'Export Mermaid diagram as SVG');
        exportSvg.textContent = 'SVG';

        const downloadPng = document.createElement('button');
        downloadPng.type = 'button';
        downloadPng.className = 'mermaid-tool-btn';
        downloadPng.setAttribute('aria-label', 'Download Mermaid diagram as PNG');
        downloadPng.textContent = 'PNG';

        const copyPng = document.createElement('button');
        copyPng.type = 'button';
        copyPng.className = 'mermaid-tool-btn';
        copyPng.setAttribute('aria-label', 'Copy Mermaid diagram as PNG to clipboard');
        copyPng.textContent = 'Copy';

        actions.append(zoomOut, reset, zoomIn, exportSvg, downloadPng, copyPng);
        toolbar.append(label, actions);

        const viewport = document.createElement('div');
        viewport.className = 'mermaid-viewport';

        const graph = document.createElement('div');
        graph.className = 'mermaid';
        graph.setAttribute('data-chart-index', index);
        graph.textContent = sourceText;

        viewport.appendChild(graph);
        shell.append(toolbar, viewport);

        shell.__mermaidRefs = { shell, viewport, graph, zoomOut, zoomIn, reset, exportSvg, downloadPng, copyPng };
        return shell;
      };

      const downloadBlob = (blob, filename) => {
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      };

      const svgToBlob = (svgEl) => {
        const clone = svgEl.cloneNode(true);
        clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
        clone.setAttribute('xmlns:xlink', 'http://www.w3.org/1999/xlink');
        const css = '\n          svg { font-family: -apple-system, BlinkMacSystemFont, sans-serif; }\n        ';
        const style = document.createElementNS('http://www.w3.org/2000/svg', 'style');
        style.textContent = css;
        clone.insertBefore(style, clone.firstChild);
        const source = new XMLSerializer().serializeToString(clone);
        return new Blob([source], { type: 'image/svg+xml;charset=utf-8' });
      };

      const ensureCanvgLoaded = async () => {
        const existing = window.Canvg || (window.canvg && window.canvg.Canvg);
        if (existing) return existing;
        const urls = [
          'https://esm.sh/canvg@4.0.2',
          'https://cdn.skypack.dev/canvg@4.0.2'
        ];
        for (const url of urls) {
          try {
            const mod = await import(url);
            const CanvgClass = mod.Canvg || (mod.default && mod.default.Canvg);
            if (CanvgClass) {
              window.Canvg = CanvgClass;
              return CanvgClass;
            }
          } catch (_) {}
        }
        throw new Error('canvg is not loaded');
      };

      const prepareSvgForExport = (svgEl) => {
        const clone = svgEl.cloneNode(true);
        clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
        clone.setAttribute('xmlns:xlink', 'http://www.w3.org/1999/xlink');
        clone.removeAttribute('style');
        const exportStyle = document.createElementNS('http://www.w3.org/2000/svg', 'style');
        exportStyle.textContent = [
          'text, tspan { font-family: -apple-system, BlinkMacSystemFont, Segoe UI, Arial, sans-serif !important; fill: #111827; }',
          '.label, .nodeLabel, .edgeLabel { color: #111827; fill: #111827; }',
          'foreignObject * { font-family: -apple-system, BlinkMacSystemFont, Segoe UI, Arial, sans-serif !important; color: #111827; }'
        ].join('\n');
        clone.insertBefore(exportStyle, clone.firstChild);
        const rect = svgEl.getBoundingClientRect();
        const viewBox = clone.getAttribute('viewBox');
        let width = Math.max(1, Math.ceil(rect.width));
        let height = Math.max(1, Math.ceil(rect.height));
        if ((!width || !height) && viewBox) {
          const parts = viewBox.split(/\s+/).map(Number);
          if (parts.length === 4) {
            width = Math.max(1, Math.ceil(parts[2]));
            height = Math.max(1, Math.ceil(parts[3]));
          }
        }
        clone.setAttribute('width', String(width));
        clone.setAttribute('height', String(height));
        const serialized = new XMLSerializer().serializeToString(clone);
        return { serialized, width, height };
      };

      const svgToPngViaImage = async (serialized, width, height) => {
        const canvas = document.createElement('canvas');
        canvas.width = width * 2;
        canvas.height = height * 2;
        const ctx = canvas.getContext('2d');
        ctx.scale(2, 2);
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, width, height);
        const img = new Image();
        img.decoding = 'async';
        await new Promise((resolve, reject) => {
          img.onload = resolve;
          img.onerror = reject;
          img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(serialized);
        });
        ctx.drawImage(img, 0, 0, width, height);
        return new Promise((resolve, reject) => {
          canvas.toBlob((blob) => {
            if (!blob) reject(new Error('PNG export failed'));
            else resolve(blob);
          }, 'image/png');
        });
      };

      const svgToPngViaCanvg = async (serialized, width, height) => {
        const canvas = document.createElement('canvas');
        canvas.width = width * 2;
        canvas.height = height * 2;
        const ctx = canvas.getContext('2d');
        ctx.scale(2, 2);
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, width, height);
        const CanvgClass = await ensureCanvgLoaded();
        const v = await CanvgClass.from(ctx, serialized, { DOMParser });
        await v.render();
        return new Promise((resolve, reject) => {
          canvas.toBlob((blob) => {
            if (!blob) reject(new Error('PNG export failed'));
            else resolve(blob);
          }, 'image/png');
        });
      };

      const svgToPngBlob = async (svgEl) => {
        const { serialized, width, height } = prepareSvgForExport(svgEl);
        try {
          return await svgToPngViaImage(serialized, width, height);
        } catch (err) {
          console.warn('native SVG PNG export failed, falling back to canvg', err);
          return await svgToPngViaCanvg(serialized, width, height);
        }
      };

      const copyBlobToClipboard = async (blob) => {
        if (!navigator.clipboard || typeof ClipboardItem === 'undefined') {
          throw new Error('Clipboard PNG copy is not supported in this browser');
        }
        await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
      };

      const bindPanZoom = ({ viewport, graph, zoomOut, zoomIn, reset, exportSvg, downloadPng, copyPng }) => {
        if (!graph) return;
        const svg = graph.querySelector('svg');
        if (!svg || !zoomOut || !zoomIn || !reset || !exportSvg || !downloadPng || !copyPng) return;
        const state = { scale: 1, x: 0, y: 0, dragging: false, pointerId: null, startX: 0, startY: 0 };
        const minScale = 0.7;
        const maxScale = 2.5;
        const step = 0.2;
        const apply = () => {
          svg.style.transform = 'translate(' + state.x + 'px, ' + state.y + 'px) scale(' + state.scale + ')';
        };
        const setScale = (next) => {
          state.scale = Math.min(maxScale, Math.max(minScale, next));
          apply();
        };

        const fitViewportHeight = () => {
          const rect = svg.getBoundingClientRect();
          const naturalHeight = Math.max(180, Math.ceil(rect.height + 36));
          viewport.style.height = 'auto';
          viewport.style.minHeight = naturalHeight + 'px';
        };
        fitViewportHeight();

        zoomOut.addEventListener('click', () => setScale(state.scale - step));
        zoomIn.addEventListener('click', () => setScale(state.scale + step));
        reset.addEventListener('click', () => {
          state.scale = 1;
          state.x = 0;
          state.y = 0;
          apply();
        });
        exportSvg.addEventListener('click', () => {
          const svg = graph.querySelector('svg');
          if (!svg) return;
          downloadBlob(svgToBlob(svg), 'mermaid-diagram.svg');
        });

        downloadPng.addEventListener('click', async () => {
          const svg = graph.querySelector('svg');
          if (!svg) return;
          try {
            const blob = await svgToPngBlob(svg);
            if (blob) downloadBlob(blob, 'mermaid-diagram.png');
          } catch (err) {
            console.error('png export failed', err);
            alert('PNG export failed: ' + String(err.message || err));
          }
        });

        copyPng.addEventListener('click', async () => {
          const svg = graph.querySelector('svg');
          if (!svg) return;
          try {
            const blob = await svgToPngBlob(svg);
            if (blob) await copyBlobToClipboard(blob);
          } catch (err) {
            console.error('png copy failed', err);
            alert('PNG copy failed: ' + String(err.message || err));
          }
        });

        viewport.addEventListener('wheel', (event) => {
          event.preventDefault();
          const factor = event.deltaY > 0 ? 0.95 : 1.05;
          setScale(state.scale * factor);
        }, { passive: false });

        viewport.addEventListener('dblclick', () => setScale(state.scale * 1.2));

        svg.style.cursor = 'grab';
        svg.style.transition = 'transform 0.1s ease-out';

        const startDrag = (event) => {
          state.dragging = true;
          state.pointerId = event.pointerId;
          state.startX = event.clientX - state.x;
          state.startY = event.clientY - state.y;
          viewport.style.cursor = 'grabbing';
          svg.style.cursor = 'grabbing';
          viewport.setPointerCapture(event.pointerId);
          event.preventDefault();
        };

        viewport.addEventListener('pointerdown', startDrag);

        document.addEventListener('pointermove', (event) => {
          if (!state.dragging || state.pointerId !== event.pointerId) return;
          state.x = event.clientX - state.startX;
          state.y = event.clientY - state.startY;
          apply();
        });

        const stopDrag = (event) => {
          if (state.pointerId !== null && event.pointerId !== undefined && state.pointerId !== event.pointerId) return;
          state.dragging = false;
          state.pointerId = null;
          svg.style.cursor = 'grab';
          viewport.style.cursor = 'grab';
        };

        document.addEventListener('pointerup', stopDrag);
        document.addEventListener('pointercancel', stopDrag);

        apply();
      };

      const blocks = content.querySelectorAll('code.language-mermaid');
      for (const [index, block] of blocks.entries()) {
        const parent = block.parentElement;
        const figure = makeMermaidFigure(block.textContent, index);
        parent.replaceWith(figure);
      }
      const figures = Array.from(content.querySelectorAll('.mermaid-figure'));
      await Promise.all(figures.map(async (figure, index) => {
        const mermaidContainer = figure.querySelector('.mermaid');
        try {
          const result = await mermaidApi.render('mermaid-svg-' + index, mermaidContainer.textContent);
          mermaidContainer.innerHTML = result.svg;
          const refs = figure.__mermaidRefs;
          bindPanZoom(refs);
        } catch (err) {
          mermaidContainer.innerHTML = '<pre style="color:#b42318;white-space:pre-wrap;">' + String(err) + '</pre>';
        }
      }));
    }

    async function render() {
      if (!content) return;
      renderMarkdown();
      await renderMermaid();
    }

    render();

    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(wsProtocol + '//' + location.host + '/ws');
    ws.onmessage = function(event) {
      try {
        const payload = JSON.parse(event.data);
        if (payload.type === 'reload') {
          if (isEditing) window.location.href = viewPath;
          else location.reload();
        }
      } catch (_) {}
    };
  </script>
</body>
</html>`
