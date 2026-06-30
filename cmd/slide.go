package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
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
	"github.com/sjzsdu/tt/internal/webui"
	"github.com/spf13/cobra"
	"nhooyr.io/websocket"
)

var (
	slidePort          = 9596
	slideContent       string
	slideTemplate      string
	slideTransition    string
	slideControls      bool
	slideProgress      bool
	slideSlideNumber   string
	slideOverview      bool
	slideCenter        string
	slideAutoSlide     int
	slideWidth         int
	slideHeight        int
	slideMargin        float64
	slideListTemplates bool
	slideFiles         []string

	slideServer *http.Server
	slideMu     sync.Mutex
	slideRoot   string

	slideClients   = make(map[*websocket.Conn]bool)
	slideClientsMu sync.Mutex
	slideWatcher   *fsnotify.Watcher
	slideWatchMu   sync.Mutex
)

var slideCmd = &cobra.Command{
	Use:     "slide [files...]",
	Aliases: []string{"sl"},
	Short:   "Present .slide files as a slide deck in the browser",
	Long:    "Start a local web service that renders .slide files as a reveal.js presentation. Use --- to separate slides.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve slide cwd failed: %w", err)
		}
		slideRoot = cwd
		if slideListTemplates {
			return printSlideTemplates(cmd.OutOrStdout())
		}

		if slideContent != "" {
			return runSlideServer()
		}

		if len(args) == 0 {
			slideFiles = collectSlideFiles(cwd)
			return runSlideServer()
		}

		if len(args) == 1 {
			candidate := args[0]
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(cwd, candidate)
			}
			info, err := os.Stat(candidate)
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			if info.IsDir() {
				slideRoot = candidate
				files := collectSlideFiles(candidate)
				if len(files) == 0 {
					return fmt.Errorf("no slide files found in %s", candidate)
				}
				slideFiles = files
				return runSlideServer()
			}
			if !isSlideFile(candidate) {
				return fmt.Errorf("unsupported slide file %s: only .slide files are supported", candidate)
			}
			slideRoot = filepath.Dir(candidate)
			slideFiles = []string{candidate}
			return runSlideServer()
		}

		// Multiple files
		var resolved []string
		for _, arg := range args {
			p := arg
			if !filepath.IsAbs(p) {
				p = filepath.Join(cwd, p)
			}
			if !isSlideFile(p) {
				return fmt.Errorf("unsupported slide file %s: only .slide files are supported", p)
			}
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				resolved = append(resolved, p)
			}
		}
		if len(resolved) == 0 {
			return fmt.Errorf("no valid slide files found")
		}
		slideRoot = filepath.Dir(resolved[0])
		slideFiles = resolved
		return runSlideServer()
	},
}

func init() {
	rootCmd.AddCommand(slideCmd)
	slideCmd.Flags().IntVarP(&slidePort, "port", "p", 9596, "service port")
	slideCmd.Flags().StringVarP(&slideContent, "content", "c", "", "render provided slide content directly")
	slideCmd.Flags().StringVarP(&slideTemplate, "template", "t", "", "override slide template at runtime, e.g. magicloud, dark, light, serif, white")
	slideCmd.Flags().StringVar(&slideTransition, "transition", "", "override slide transition, e.g. none, fade, slide, convex, concave, zoom")
	slideCmd.Flags().BoolVar(&slideControls, "controls", true, "show reveal navigation controls")
	slideCmd.Flags().BoolVar(&slideProgress, "progress", true, "show reveal progress bar")
	slideCmd.Flags().StringVar(&slideSlideNumber, "slide-number", "true", "slide number mode: true, false, h.v, h/v, c, c/t")
	slideCmd.Flags().BoolVar(&slideOverview, "overview", false, "enable reveal built-in overview mode")
	slideCmd.Flags().StringVar(&slideCenter, "center", "auto", "vertical centering: auto, true, false")
	slideCmd.Flags().IntVar(&slideAutoSlide, "auto-slide", 0, "auto-advance interval in milliseconds, 0 disables")
	slideCmd.Flags().IntVar(&slideWidth, "width", 0, "slide canvas width, 0 uses template default")
	slideCmd.Flags().IntVar(&slideHeight, "height", 0, "slide canvas height, 0 uses template default")
	slideCmd.Flags().Float64Var(&slideMargin, "margin", -1, "slide viewport margin, e.g. 0.04; negative uses template default")
	slideCmd.Flags().BoolVar(&slideListTemplates, "list-templates", false, "list available slide templates and exit")
}

func isSlideFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".slide")
}

func collectSlideFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".") {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if isSlideFile(d.Name()) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

type slideTemplateListItem struct {
	Name   string
	Source string
	Path   string
}

func printSlideTemplates(w io.Writer) error {
	items := listAvailableSlideTemplates()
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No slide templates found.")
		return err
	}
	_, _ = fmt.Fprintln(w, "Available slide templates:")
	for _, item := range items {
		if item.Path == "" {
			_, _ = fmt.Fprintf(w, "  %-16s %s\n", item.Name, item.Source)
			continue
		}
		_, _ = fmt.Fprintf(w, "  %-16s %-8s %s\n", item.Name, item.Source, item.Path)
	}
	return nil
}

func listAvailableSlideTemplates() []slideTemplateListItem {
	items := []slideTemplateListItem{
		{Name: "dark", Source: "built-in"},
		{Name: "light", Source: "built-in"},
		{Name: "serif", Source: "built-in"},
		{Name: "white", Source: "built-in"},
	}
	seen := map[string]int{}
	for i, item := range items {
		seen[item.Name] = i
	}

	for _, root := range slideTemplateSearchRootsWithSource() {
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !isSafeSlideTemplateName(entry.Name()) {
				continue
			}
			templateDir := filepath.Join(root.Path, entry.Name())
			if info, err := os.Stat(filepath.Join(templateDir, "template.json")); err != nil || info.IsDir() {
				continue
			}
			item := slideTemplateListItem{Name: entry.Name(), Source: root.Source, Path: templateDir}
			if idx, ok := seen[item.Name]; ok {
				items[idx] = item
				continue
			}
			seen[item.Name] = len(items)
			items = append(items, item)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Source == items[j].Source {
			return items[i].Name < items[j].Name
		}
		return slideTemplateSourceRank(items[i].Source) < slideTemplateSourceRank(items[j].Source)
	})
	return items
}

func slideTemplateSourceRank(source string) int {
	switch source {
	case "project":
		return 0
	case "global":
		return 1
	default:
		return 2
	}
}

func slideRelPath(abs string) string {
	rel, err := filepath.Rel(slideRoot, abs)
	if err != nil {
		return filepath.Base(abs)
	}
	return "/" + filepath.ToSlash(rel)
}

func runSlideServer() error {
	slideMu.Lock()
	defer slideMu.Unlock()

	if slideServer != nil {
		return fmt.Errorf("slide service already running on port %d", slidePort)
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.SlideAssetsHandler())
	mux.HandleFunc("/", handleSlideApp)
	mux.HandleFunc("/raw-content", handleSlideRawContent)
	mux.HandleFunc("/raw/", handleSlideRawFile)
	mux.HandleFunc("/api/list", handleSlideList)
	mux.HandleFunc("/api/template/", handleSlideTemplate)
	mux.HandleFunc("/template-assets/", handleSlideTemplateAsset)
	mux.HandleFunc("/api/d2", handleD2Render)
	mux.HandleFunc("/images/", handleSlideImages)
	mux.HandleFunc("/ws", handleSlideWS)

	maxPort := slidePort + 20
	var lastErr error
	for port := slidePort; port <= maxPort; port++ {
		slideServer = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		serverErr := make(chan error, 1)
		go func() {
			err := slideServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()

		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-serverErr:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				slideServer = nil
				continue
			}
			slideServer = nil
			return err
		default:
			slidePort = port
			fmt.Printf("Slide service started: http://localhost:%d\n", port)
			if err := initSlideWatcher(); err != nil {
				fmt.Printf("slide watcher init warning: %v\n", err)
			}
			browserURL := fmt.Sprintf("http://localhost:%d", port)
			params := url.Values{}
			if slideContent != "" {
				params.Set("content", "1")
			} else if len(slideFiles) == 1 {
				params.Set("file", slideRelPath(slideFiles[0]))
			}
			if strings.TrimSpace(slideTemplate) != "" {
				params.Set("template", strings.TrimSpace(slideTemplate))
			}
			appendSlideConfigParams(params)
			if encoded := params.Encode(); encoded != "" {
				browserURL += "?" + encoded
			}
			go openBrowser(browserURL)
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt)
			<-quit
			fmt.Println("\nShutting down slide service...")
			cleanupSlideWatcher()
			closeSlideClients()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := slideServer.Shutdown(ctx)
			slideServer = nil
			return err
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func appendSlideConfigParams(params url.Values) {
	if strings.TrimSpace(slideTransition) != "" {
		params.Set("transition", strings.TrimSpace(slideTransition))
	}
	params.Set("controls", fmt.Sprintf("%t", slideControls))
	params.Set("progress", fmt.Sprintf("%t", slideProgress))
	if strings.TrimSpace(slideSlideNumber) != "" {
		params.Set("slideNumber", strings.TrimSpace(slideSlideNumber))
	}
	params.Set("overview", fmt.Sprintf("%t", slideOverview))
	if strings.TrimSpace(slideCenter) != "" && strings.TrimSpace(slideCenter) != "auto" {
		params.Set("center", strings.TrimSpace(slideCenter))
	}
	if slideAutoSlide > 0 {
		params.Set("autoSlide", fmt.Sprintf("%d", slideAutoSlide))
	}
	if slideWidth > 0 {
		params.Set("width", fmt.Sprintf("%d", slideWidth))
	}
	if slideHeight > 0 {
		params.Set("height", fmt.Sprintf("%d", slideHeight))
	}
	if slideMargin >= 0 {
		params.Set("margin", fmt.Sprintf("%g", slideMargin))
	}
}

func handleSlideApp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(webui.SlideIndex()); err != nil {
		http.Error(w, fmt.Sprintf("render slide app failed: %v", err), http.StatusInternalServerError)
	}
}

func handleSlideRawContent(w http.ResponseWriter, r *http.Request) {
	if slideContent == "" {
		http.Error(w, "no content mode", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(slideContent))
}

func handleSlideRawFile(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/raw")
	if relPath == "" || relPath == "/" {
		http.Error(w, "file path is required", http.StatusBadRequest)
		return
	}

	absPath, err := safeJoin(slideRoot, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("file not found: %v", err), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "directories cannot be served through /raw", http.StatusBadRequest)
		return
	}

	if isSlideFile(absPath) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	}
	http.ServeFile(w, r, absPath)
}

func handleSlideList(w http.ResponseWriter, r *http.Request) {
	var files []map[string]string
	for _, f := range slideFiles {
		rel := slideRelPath(f)
		name := filepath.Base(f)
		files = append(files, map[string]string{
			"path": rel,
			"name": name,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"files": files,
		"total": len(files),
	})
}

type slideTemplateDefaults struct {
	Theme      string   `json:"theme"`
	Transition string   `json:"transition"`
	Center     bool     `json:"center"`
	Margin     *float64 `json:"margin,omitempty"`
	Width      *int     `json:"width,omitempty"`
	Height     *int     `json:"height,omitempty"`
}

type slideTemplateManifest struct {
	Name        string                `json:"name"`
	RevealTheme string                `json:"revealTheme"`
	CSS         string                `json:"css"`
	Defaults    slideTemplateDefaults `json:"defaults"`
	Vars        map[string]string     `json:"vars,omitempty"`
}

type slideTemplateResponse struct {
	Name        string                `json:"name"`
	RevealTheme string                `json:"revealTheme"`
	CSS         string                `json:"css"`
	Defaults    slideTemplateDefaults `json:"defaults"`
}

type slideTemplateSearchRoot struct {
	Source string
	Path   string
}

var slideTemplateAssetURLPattern = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)['"]?\s*\)`)

func handleSlideTemplate(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/template/"), "/")
	if !isSafeSlideTemplateName(name) {
		http.Error(w, "invalid template name", http.StatusBadRequest)
		return
	}

	templateDir, err := findSlideTemplateDir(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	manifestBytes, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		http.Error(w, fmt.Sprintf("read template manifest failed: %v", err), http.StatusNotFound)
		return
	}

	var manifest slideTemplateManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		http.Error(w, fmt.Sprintf("parse template manifest failed: %v", err), http.StatusBadRequest)
		return
	}
	if manifest.Name == "" {
		manifest.Name = name
	}
	if manifest.RevealTheme == "" {
		manifest.RevealTheme = "white"
	}
	if manifest.CSS == "" {
		manifest.CSS = "template.css"
	}
	if manifest.Defaults.Theme == "" {
		manifest.Defaults.Theme = "light"
	}
	if manifest.Defaults.Transition == "" {
		manifest.Defaults.Transition = "fade"
	}
	if filepath.IsAbs(manifest.CSS) || strings.Contains(filepath.Clean("/"+manifest.CSS), "..") {
		http.Error(w, "invalid template css path", http.StatusBadRequest)
		return
	}

	cssPath := filepath.Join(templateDir, filepath.FromSlash(manifest.CSS))
	cssBytes, err := os.ReadFile(cssPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read template css failed: %v", err), http.StatusNotFound)
		return
	}

	css := renderSlideTemplateVarsCSS(name, manifest.Vars) + rewriteSlideTemplateAssetURLs(name, filepath.ToSlash(filepath.Dir(manifest.CSS)), string(cssBytes))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(slideTemplateResponse{
		Name:        manifest.Name,
		RevealTheme: manifest.RevealTheme,
		CSS:         css,
		Defaults:    manifest.Defaults,
	})
}

func renderSlideTemplateVarsCSS(templateName string, vars map[string]string) string {
	if len(vars) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vars))
	for key := range vars {
		if isSafeSlideTemplateCSSVarName(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(":root {\n")
	for _, key := range keys {
		value := strings.TrimSpace(vars[key])
		if value == "" || strings.ContainsAny(value, "{};") {
			continue
		}
		value = rewriteSlideTemplateAssetURLs(templateName, ".", value)
		b.WriteString("  --")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString(";\n")
	}
	b.WriteString("}\n\n")
	return b.String()
}

func isSafeSlideTemplateCSSVarName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func handleSlideTemplateAsset(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/template-assets/")
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) != 2 || !isSafeSlideTemplateName(parts[0]) {
		http.Error(w, "invalid template asset path", http.StatusBadRequest)
		return
	}
	templateName := parts[0]
	assetPath := filepath.Clean("/" + parts[1])
	if assetPath == "/" || strings.Contains(assetPath, "..") {
		http.Error(w, "invalid template asset path", http.StatusBadRequest)
		return
	}

	templateDir, err := findSlideTemplateDir(templateName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	absPath, err := safeJoin(templateDir, assetPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read template asset failed: %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mimeType(absPath))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	_, _ = w.Write(content)
}

func isSafeSlideTemplateName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func findSlideTemplateDir(name string) (string, error) {
	for _, base := range slideTemplateSearchRoots() {
		candidate := filepath.Join(base, name)
		info, err := os.Stat(filepath.Join(candidate, "template.json"))
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("template %s not found", name)
}

func slideTemplateSearchRoots() []string {
	var roots []string
	for _, root := range slideTemplateSearchRootsWithSource() {
		roots = append(roots, root.Path)
	}
	return roots
}

func slideTemplateSearchRootsWithSource() []slideTemplateSearchRoot {
	var roots []slideTemplateSearchRoot
	if projectRoot := findNearestTTDir(slideRoot); projectRoot != "" {
		roots = append(roots, slideTemplateSearchRoot{Source: "project", Path: filepath.Join(projectRoot, "slide", "templates")})
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, slideTemplateSearchRoot{Source: "global", Path: filepath.Join(home, ".tt", "slide", "templates")})
	}
	return roots
}

func findNearestTTDir(start string) string {
	if start == "" {
		return ""
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".tt")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func rewriteSlideTemplateAssetURLs(templateName, cssDir, css string) string {
	return slideTemplateAssetURLPattern.ReplaceAllStringFunc(css, func(match string) string {
		parts := slideTemplateAssetURLPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		raw := strings.TrimSpace(parts[2])
		lower := strings.ToLower(raw)
		if strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "#") {
			return match
		}
		joined := filepath.ToSlash(filepath.Clean("/" + pathJoinSlash(cssDir, raw)))
		joined = strings.TrimPrefix(joined, "/")
		if joined == "" || strings.Contains(joined, "..") {
			return match
		}
		return fmt.Sprintf("url(\"/template-assets/%s/%s\")", templateName, joined)
	})
}

func pathJoinSlash(base, rel string) string {
	if base == "." || base == "/" {
		base = ""
	}
	if base == "" {
		return rel
	}
	return strings.TrimSuffix(base, "/") + "/" + rel
}

func handleSlideImages(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/images")
	relPath = filepath.Clean("/" + strings.TrimPrefix(relPath, "/"))
	absPath, err := safeJoin(slideRoot, relPath)
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

func handleSlideWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1", "localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		log.Printf("slide websocket accept failed: %v", err)
		return
	}
	slideClientsMu.Lock()
	slideClients[conn] = true
	slideClientsMu.Unlock()
	ctx := context.Background()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connected"}`))
	defer func() {
		slideClientsMu.Lock()
		delete(slideClients, conn)
		slideClientsMu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func broadcastSlideReload(message string) {
	slideClientsMu.Lock()
	defer slideClientsMu.Unlock()
	payload := fmt.Sprintf(`{"type":"reload","message":%q}`, message)
	ctx := context.Background()
	for conn := range slideClients {
		_ = conn.Write(ctx, websocket.MessageText, []byte(payload))
	}
}

func closeSlideClients() {
	slideClientsMu.Lock()
	defer slideClientsMu.Unlock()
	for conn := range slideClients {
		_ = conn.Close(websocket.StatusNormalClosure, "server closed")
	}
	slideClients = make(map[*websocket.Conn]bool)
}

func initSlideWatcher() error {
	slideWatchMu.Lock()
	defer slideWatchMu.Unlock()
	if slideWatcher != nil {
		_ = slideWatcher.Close()
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	slideWatcher = watcher

	dirs := make(map[string]bool)
	_ = filepath.WalkDir(slideRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".") {
				if path != slideRoot {
					return filepath.SkipDir
				}
			}
			dirs[path] = true
		}
		return nil
	})
	for dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			log.Printf("slide watch dir failed %s: %v", dir, err)
		}
	}
	go watchSlideFiles()
	return nil
}

func watchSlideFiles() {
	debounce := map[string]*time.Timer{}
	var mu sync.Mutex
	for {
		slideWatchMu.Lock()
		watcher := slideWatcher
		slideWatchMu.Unlock()
		if watcher == nil {
			return
		}
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			name := filepath.Base(event.Name)
			if !isSlideFile(name) {
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
				broadcastSlideReload(name + " changed")
				mu.Lock()
				delete(debounce, event.Name)
				mu.Unlock()
			})
			mu.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("slide watcher error: %v", err)
		}
	}
}

func cleanupSlideWatcher() {
	slideWatchMu.Lock()
	defer slideWatchMu.Unlock()
	if slideWatcher != nil {
		_ = slideWatcher.Close()
		slideWatcher = nil
	}
}

type d2Request struct {
	Code  string `json:"code"`
	Theme string `json:"theme"`
}

type d2Response struct {
	SVG string `json:"svg"`
}

func handleD2Render(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req d2Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	svg, err := renderD2(req.Code, req.Theme)
	if err != nil {
		http.Error(w, fmt.Sprintf("d2 render failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(d2Response{SVG: svg})
}

func renderD2(code, theme string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "slide-d2-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputFile := filepath.Join(tmpDir, "input.d2")
	outputFile := filepath.Join(tmpDir, "output.svg")

	if err := os.WriteFile(inputFile, []byte(code), 0o644); err != nil {
		return "", fmt.Errorf("write d2 input failed: %w", err)
	}

	args := []string{"--theme", theme}
	if theme == "" {
		args = []string{"--theme", "dark"}
	}
	args = append(args, inputFile, outputFile)

	cmd := exec.Command("d2", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("d2 execution failed: %w\n%s", err, string(output))
	}

	svgBytes, err := os.ReadFile(outputFile)
	if err != nil {
		return "", fmt.Errorf("read d2 output failed: %w", err)
	}

	return string(svgBytes), nil
}
