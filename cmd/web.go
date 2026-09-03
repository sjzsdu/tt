package cmd

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	webPort int = 9597
	webRoot string

	webServer *http.Server
	webMu     sync.Mutex
)

var webCmd = &cobra.Command{
	Use:     "web [directory]",
	Short:   "Browse HTML files in a local web service",
	Long:    "Start a local web service for browsing HTML files and their static assets in a target directory.",
	Args:    cobra.MaximumNArgs(1),
	Example: "tt web\n tt web ./dist\n tt web ./public --port 9597",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveWebRoot(args)
		if err != nil {
			return err
		}
		webRoot = root
		return runWebServer()
	},
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().IntVarP(&webPort, "port", "p", 9597, "service port")
}

func resolveWebRoot(args []string) (string, error) {
	root := "."
	if len(args) == 1 {
		root = strings.TrimSpace(args[0])
		if root == "" {
			return "", fmt.Errorf("web target directory is required")
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve web target directory failed: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("stat web target directory %q failed: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("web target must be a directory: %s", absRoot)
	}
	return absRoot, nil
}

func runWebServer() error {
	webMu.Lock()
	defer webMu.Unlock()

	if webServer != nil {
		return fmt.Errorf("web service already running on port %d", webPort)
	}

	handler := newWebHandler(webRoot)
	maxPort := webPort + 20
	var lastErr error
	for port := webPort; port <= maxPort; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			lastErr = err
			continue
		}

		webPort = port
		webServer = &http.Server{Addr: listener.Addr().String(), Handler: handler}
		serverErr := make(chan error, 1)
		go func(server *http.Server) {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}(webServer)

		fmt.Printf("Web service started: http://localhost:%d\n", port)
		go openBrowser(fmt.Sprintf("http://localhost:%d", port))
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		select {
		case <-quit:
			signal.Stop(quit)
			fmt.Println("\nShutting down web service...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := webServer.Shutdown(ctx)
			cancel()
			webServer = nil
			return err
		case err := <-serverErr:
			signal.Stop(quit)
			webServer = nil
			return fmt.Errorf("web service stopped unexpectedly: %w", err)
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func newWebHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			handleWebIndex(w, root)
			return
		}

		absPath, err := safeJoin(root, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		info, err := os.Stat(absPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if info.IsDir() {
			indexPath := filepath.Join(absPath, "index.html")
			if indexInfo, indexErr := os.Stat(indexPath); indexErr != nil || indexInfo.IsDir() {
				http.NotFound(w, r)
				return
			}
			absPath = indexPath
			info, err = os.Stat(absPath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}

		file, err := os.Open(absPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		if contentType := mime.TypeByExtension(filepath.Ext(absPath)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, filepath.Base(absPath), info.ModTime(), file)
	})
}

type webFile struct {
	Relative string
	Size     int64
}

func collectWebFiles(root string) ([]webFile, error) {
	files := make([]webFile, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skipWebDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isHTMLFile(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, webFile{Relative: filepath.ToSlash(relative), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	return files, nil
}

func handleWebIndex(w http.ResponseWriter, root string) {
	files, err := collectWebFiles(root)
	if err != nil {
		http.Error(w, fmt.Sprintf("collect HTML files failed: %v", err), http.StatusInternalServerError)
		return
	}

	var body strings.Builder
	body.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>tt web</title><style>body{font:16px system-ui,sans-serif;line-height:1.5;max-width:880px;margin:40px auto;padding:0 20px;color:#1f2937}h1{margin-bottom:4px}p{color:#6b7280}ul{padding:0;list-style:none;border-top:1px solid #e5e7eb}li{padding:10px 0;border-bottom:1px solid #e5e7eb;display:flex;justify-content:space-between;gap:20px}a{color:#2563eb;text-decoration:none}a:hover{text-decoration:underline}.size{color:#9ca3af;white-space:nowrap}</style></head><body>`)
	body.WriteString("<h1>HTML files</h1><p>" + html.EscapeString(root) + "</p>")
	if len(files) == 0 {
		body.WriteString("<p>No HTML files found.</p>")
	} else {
		body.WriteString("<ul>")
		for _, file := range files {
			body.WriteString(`<li><a href="/`)
			body.WriteString(escapeViewPath(file.Relative))
			body.WriteString(`">`)
			body.WriteString(html.EscapeString(file.Relative))
			body.WriteString(`</a><span class="size">`)
			body.WriteString(formatWebFileSize(file.Size))
			body.WriteString(`</span></li>`)
		}
		body.WriteString("</ul>")
	}
	body.WriteString("</body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body.String()))
}

func skipWebDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".")
}

func isHTMLFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func formatWebFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}
