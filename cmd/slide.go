package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	slidePort    = 9596
	slideContent string
	slideFiles   []string

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
	Short:   "Present markdown as a slide deck in the browser",
	Long:    "Start a local web service that renders markdown files as a reveal.js presentation. Supports .slide and .md extensions. Use --- to separate slides.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve slide cwd failed: %w", err)
		}
		slideRoot = cwd

		if slideContent != "" {
			return runSlideServer()
		}

		if len(args) == 0 {
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
	slideCmd.Flags().StringVarP(&slideContent, "content", "c", "", "render provided markdown content directly as slides")
}

func isSlideFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".slide") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
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
			if slideContent != "" {
				browserURL += "?content=1"
			} else if len(slideFiles) == 1 {
				browserURL += "?file=" + url.QueryEscape(slideRelPath(slideFiles[0]))
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

	content, err := os.ReadFile(absPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("file not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write(content)
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
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, OriginPatterns: []string{"*"}})
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
