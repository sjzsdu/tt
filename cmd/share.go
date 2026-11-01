package cmd

import (
	"archive/zip"
	"context"
	"crypto/subtle"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	mdutil "github.com/sjzsdu/tt/internal/mdutil"
	"github.com/spf13/cobra"
)

var (
	sharePort     int
	shareToken    string
	shareNoAuth   bool
	shareOpen     bool
	sharePatterns []string

	shareServer *http.Server
	shareMu     sync.Mutex
)

const (
	shareCookieName = "tt_share_token"
	// defaultShareToken keeps the share link stable across runs so the same URL
	// works on every device without passing --token. Override with --token.
	defaultShareToken = "ttshare"
)

var shareCmd = &cobra.Command{
	Use:   "share [directory]",
	Short: "Share a directory over the LAN with preview and download",
	Long: "Start a read-only web service that shares a directory with devices on the same network. " +
		"Visitors can browse the file tree, preview common file types, and download files or the whole " +
		"directory as a zip. Access is protected by a token unless --no-auth is set. The service never " +
		"modifies the shared directory and never shares hidden files, .git, node_modules, or vendor.",
	Example: `tt share
tt share ./dist
tt share ~/photos --port 9600
tt share . --token mysecret
tt share . --no-auth`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runShare(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(shareCmd)
	shareCmd.Flags().IntVarP(&sharePort, "port", "p", 9600, "service port")
	shareCmd.Flags().StringVar(&shareToken, "token", "", "access token (default: "+defaultShareToken+")")
	shareCmd.Flags().BoolVar(&shareNoAuth, "no-auth", false, "disable the access token")
	shareCmd.Flags().BoolVar(&shareOpen, "open", true, "open the share in the default browser")
	shareCmd.Flags().StringSliceVarP(&sharePatterns, "pattern", "f", []string{}, "filter shared files by glob patterns, supports multiple values")
}

// resolveShareToken picks the effective token: --no-auth disables auth, an
// explicit --token wins, and otherwise the stable default keeps URLs fixed.
func resolveShareToken(flagValue string, noAuth bool) string {
	if noAuth {
		return ""
	}
	if token := strings.TrimSpace(flagValue); token != "" {
		return token
	}
	return defaultShareToken
}

func runShare(cmd *cobra.Command, args []string) error {
	root, err := resolveShareRoot(args)
	if err != nil {
		return err
	}

	token := resolveShareToken(shareToken, shareNoAuth)

	handler, err := newShareHandler(root, sharePatterns, token)
	if err != nil {
		return err
	}

	shareMu.Lock()
	defer shareMu.Unlock()
	if shareServer != nil {
		return fmt.Errorf("share service already running on port %d", sharePort)
	}

	tokenQuery := ""
	if token != "" {
		tokenQuery = "/?t=" + url.QueryEscape(token)
	}

	maxPort := sharePort + 20
	var lastErr error
	for port := sharePort; port <= maxPort; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			lastErr = err
			continue
		}

		sharePort = port
		shareServer = &http.Server{Addr: listener.Addr().String(), Handler: handler}
		serverErr := make(chan error, 1)
		go func(server *http.Server) {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}(shareServer)

		localURL := fmt.Sprintf("http://localhost:%d%s", port, tokenQuery)
		fmt.Printf("Share service started: %s\n", localURL)
		for _, ip := range shareLANIPs() {
			fmt.Printf("LAN: http://%s:%d%s\n", ip, port, tokenQuery)
		}
		fmt.Printf("Share directory: %s\n", root)
		fmt.Println("Press Ctrl-C to stop.")
		if shareOpen {
			go openBrowser(localURL)
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		select {
		case <-quit:
			signal.Stop(quit)
			fmt.Println("\nShutting down share service...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := shareServer.Shutdown(ctx)
			cancel()
			shareServer = nil
			return err
		case err := <-serverErr:
			signal.Stop(quit)
			shareServer = nil
			return fmt.Errorf("share service stopped unexpectedly: %w", err)
		}
	}

	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func resolveShareRoot(args []string) (string, error) {
	target := "."
	if len(args) == 1 {
		target = strings.TrimSpace(args[0])
		if target == "" {
			return "", fmt.Errorf("share target directory is required")
		}
	}
	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve share target failed: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat share target %q failed: %w", target, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("share target must be a directory: %s", absPath)
	}
	return absPath, nil
}

// shareLANIPs returns every non-loopback, non-link-local IPv4 address bound to
// this machine so the operator can hand a working URL to other devices.
func shareLANIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if shareIsLANIP(ipnet.IP) {
			out = append(out, ipnet.IP.String())
		}
	}
	sort.Strings(out)
	return out
}

func shareIsLANIP(ip net.IP) bool {
	if ip == nil || ip.To4() == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// shareSkipName reports whether a file or directory name must never be shared.
// Hidden entries and common dependency/VCS directories are excluded so a
// careless `tt share ~` does not publish secrets.
func shareSkipName(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func shareFileKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".pdf":
		return "pdf"
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico", ".avif":
		return "image"
	case ".mp4", ".webm", ".mov", ".ogv", ".m4v":
		return "video"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".opus":
		return "audio"
	case ".txt", ".log", ".json", ".js", ".mjs", ".cjs", ".css", ".scss", ".less",
		".ts", ".tsx", ".jsx", ".vue", ".svelte", ".go", ".py", ".java", ".c", ".h",
		".cpp", ".hpp", ".cc", ".rs", ".rb", ".php", ".sh", ".bash", ".zsh", ".yml",
		".yaml", ".toml", ".ini", ".conf", ".cfg", ".xml", ".csv", ".tsv", ".sql",
		".r", ".swift", ".kt", ".scala", ".lua", ".pl":
		return "text"
	}
	switch strings.ToLower(name) {
	case "makefile", "dockerfile", "license", "readme", "changelog", "notice", "authors":
		return "text"
	}
	return "file"
}

func shareMatchesPattern(patterns []string, relPath, fileName string) bool {
	if len(patterns) == 0 {
		return true
	}
	filtering := false
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		filtering = true
		if match, _ := filepath.Match(pattern, relPath); match {
			return true
		}
		if match, _ := filepath.Match(pattern, fileName); match {
			return true
		}
		if shareMatchDoubleStar(pattern, relPath) {
			return true
		}
	}
	// Blank-only patterns are not a filter; share everything.
	return !filtering
}

// shareMatchDoubleStar handles the `**` glob forms filepath.Match cannot: a
// `dir/**` subtree prefix, a leading `**/` any-depth match, and a bare `**`.
func shareMatchDoubleStar(pattern, relPath string) bool {
	if !strings.Contains(pattern, "**") {
		return false
	}
	if strings.HasSuffix(pattern, "**") {
		prefix := strings.TrimSuffix(pattern, "**")
		prefix = strings.TrimSuffix(prefix, "/")
		if prefix == "" {
			return true
		}
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}
	if strings.HasPrefix(pattern, "**/") {
		rest := strings.TrimPrefix(pattern, "**/")
		candidate := relPath
		for {
			if match, _ := filepath.Match(rest, candidate); match {
				return true
			}
			if !strings.Contains(rest, "/") {
				if match, _ := filepath.Match(rest, filepath.Base(candidate)); match {
					return true
				}
			}
			slash := strings.Index(candidate, "/")
			if slash < 0 {
				return false
			}
			candidate = candidate[slash+1:]
		}
	}
	// Middle `**` (for example `docs/**/index.md`): collapse to one wildcard.
	match, _ := filepath.Match(strings.ReplaceAll(pattern, "**", "*"), relPath)
	return match
}

var (
	errShareNotFound = fmt.Errorf("share path not found")
	// errShareHidden marks paths that exist but must never be shared: hidden
	// segments, skipped directories, and pattern mismatches.
	errShareHidden = fmt.Errorf("share path not shared")
)

// shareResolveError maps resolution failures onto HTTP statuses; both hidden
// and missing paths answer 404 so probing cannot enumerate the filesystem.
func shareResolveError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "path escapes root") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

type shareEntry struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Kind    string `json:"kind"`
	abs     string
}

type shareService struct {
	root     string
	realRoot string
	patterns []string
	token    string
}

// newShareHandler builds the read-only share handler. The resolved real root
// is used later to reject paths that escape the shared directory through
// symlinks.
func newShareHandler(root string, patterns []string, token string) (http.Handler, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = filepath.Clean(root)
	}
	return &shareService{
		root:     filepath.Clean(root),
		realRoot: realRoot,
		patterns: append([]string{}, patterns...),
		token:    token,
	}, nil
}

func (s *shareService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed: the share service is read-only", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(w, r) {
		serveShareTokenPage(w)
		return
	}

	switch {
	case r.URL.Path == "/":
		s.handleIndex(w, r)
	case r.URL.Path == "/favicon.svg" || r.URL.Path == "/favicon.ico":
		s.handleFavicon(w, r)
	case r.URL.Path == "/api/list":
		s.handleList(w, r)
	case r.URL.Path == "/download-all":
		s.handleZip(w, r)
	case strings.HasPrefix(r.URL.Path, "/render/"):
		s.handleRender(w, r)
	case strings.HasPrefix(r.URL.Path, "/file/"):
		s.handleFile(w, r, false)
	case strings.HasPrefix(r.URL.Path, "/download/"):
		s.handleFile(w, r, true)
	default:
		http.NotFound(w, r)
	}
}

// authorize accepts the token from the ?t= query parameter (setting a session
// cookie so later asset requests authenticate automatically) or from the
// cookie set by an earlier visit.
func (s *shareService) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.token == "" {
		return true
	}
	if got := r.URL.Query().Get("t"); got != "" &&
		subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1 {
		http.SetCookie(w, &http.Cookie{
			Name:     shareCookieName,
			Value:    s.token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		return true
	}
	if cookie, err := r.Cookie(shareCookieName); err == nil &&
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.token)) == 1 {
		return true
	}
	return false
}

func (s *shareService) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		RootName string
	}{RootName: filepath.Base(s.root)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := shareIndexTpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render share page failed: %v", err), http.StatusInternalServerError)
	}
}

func (s *shareService) handleFavicon(w http.ResponseWriter, r *http.Request) {
	// A favicon inside the shared directory takes precedence over the built-in logo.
	candidate, err := s.resolve(filepath.Base(r.URL.Path))
	if err == nil {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			file, openErr := os.Open(candidate)
			if openErr == nil {
				defer file.Close()
				if contentType := mime.TypeByExtension(filepath.Ext(candidate)); contentType != "" {
					w.Header().Set("Content-Type", contentType)
				}
				http.ServeContent(w, r, filepath.Base(candidate), info.ModTime(), file)
				return
			}
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(webFaviconSVG))
}

func (s *shareService) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := s.list()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect share files failed: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"root": filepath.Base(s.root), "files": files, "total": len(files)})
}

func (s *shareService) handleRender(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/render")
	absPath, err := s.resolve(rel)
	if err != nil {
		shareResolveError(w, err)
		return
	}
	if shareFileKind(filepath.Base(absPath)) != "markdown" {
		http.NotFound(w, r)
		return
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body := mdutil.SplitDocument(string(content)).Body
	rendered := renderShareMarkdown([]byte(body))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Preview only: documents rendered here never execute scripts, whether
	// opened directly or injected into the share page.
	w.Header().Set("Content-Security-Policy", "sandbox")
	_, _ = w.Write(rendered)
}

func (s *shareService) handleFile(w http.ResponseWriter, r *http.Request, attachment bool) {
	prefix := "/file"
	if attachment {
		prefix = "/download"
	}
	rel := strings.TrimPrefix(r.URL.Path, prefix)
	absPath, err := s.resolve(rel)
	if err != nil {
		shareResolveError(w, err)
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	kind := shareFileKind(filepath.Base(absPath))
	if attachment {
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(absPath)})
		w.Header().Set("Content-Disposition", disposition)
	} else if kind == "html" || (kind == "image" && strings.EqualFold(filepath.Ext(absPath), ".svg")) {
		// Shared HTML/SVG executes with the viewer's LAN access if navigated to
		// directly; sandbox it so previews render but scripts stay disabled.
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	if contentType := mime.TypeByExtension(filepath.Ext(absPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, filepath.Base(absPath), info.ModTime(), file)
}

func (s *shareService) handleZip(w http.ResponseWriter, r *http.Request) {
	files, err := s.list()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect share files failed: %v", err), http.StatusInternalServerError)
		return
	}
	zipName := filepath.Base(s.root) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": zipName}))

	zw := zip.NewWriter(w)
	for _, entry := range files {
		source, err := os.Open(entry.abs)
		if err != nil {
			continue
		}
		header := &zip.FileHeader{
			Name:     entry.Path,
			Method:   zip.Deflate,
			Modified: time.Unix(entry.ModTime, 0),
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			source.Close()
			break
		}
		_, err = io.Copy(writer, source)
		source.Close()
		if err != nil {
			break
		}
	}
	_ = zw.Close()
}

// resolve maps a request-relative path to an absolute file inside the shared
// directory, rejecting lexical traversal, hidden/skipped segments, pattern
// mismatches, and symlinks that escape the shared root.
func (s *shareService) resolve(rel string) (string, error) {
	trimmed := strings.TrimPrefix(rel, "/")
	absPath, err := safeJoin(s.root, trimmed)
	if err != nil {
		return "", err
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + filepath.FromSlash(trimmed)))
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." {
			continue
		}
		if shareSkipName(segment) {
			return "", errShareHidden
		}
	}
	fileName := filepath.Base(cleaned)
	if !shareMatchesPattern(s.patterns, strings.TrimPrefix(cleaned, "/"), fileName) {
		return "", errShareHidden
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", errShareNotFound
	}
	if resolved != s.realRoot && !strings.HasPrefix(resolved, s.realRoot+string(filepath.Separator)) {
		return "", errShareNotFound
	}
	return resolved, nil
}

// list walks the shared directory once and returns every shareable file.
func (s *shareService) list() ([]shareEntry, error) {
	entries := make([]shareEntry, 0)
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != s.root && shareSkipName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shareSkipName(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !shareMatchesPattern(s.patterns, rel, d.Name()) {
			return nil
		}
		// Skip symlinks that leave the shared directory; serve the resolved
		// target for symlinks that stay inside it.
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil {
			return nil
		}
		if resolved != s.realRoot && !strings.HasPrefix(resolved, s.realRoot+string(filepath.Separator)) {
			return nil
		}
		info, infoErr := os.Stat(path)
		if infoErr != nil || info.IsDir() {
			return nil
		}
		entries = append(entries, shareEntry{
			Path:    rel,
			Name:    d.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Kind:    shareFileKind(d.Name()),
			abs:     resolved,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func renderShareMarkdown(content []byte) []byte {
	parserExt := parser.CommonExtensions
	p := parser.NewWithExtensions(parserExt)
	// SkipHTML drops raw HTML blocks/inline nodes and Safelink restricts link
	// protocols, so a shared markdown file cannot inject scripts into readers.
	rendererOpts := mdhtml.RendererOptions{
		Flags: mdhtml.CommonFlags | mdhtml.SkipHTML | mdhtml.Safelink,
	}
	return markdown.ToHTML(content, p, mdhtml.NewRenderer(rendererOpts))
}

func serveShareTokenPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>403 - token required</title>
<style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;background:#f8fafc;color:#0f172a}
main{max-width:26rem;padding:2rem;text-align:center}h1{font-size:1.25rem}p{color:#475569;line-height:1.6}</style></head>
<body><main><h1>Share token required or invalid</h1>
<p>Ask the person sharing this directory for the full link, including its <code>?t=</code> token.</p>
</main></body></html>`))
}

var shareIndexTpl = template.Must(template.New("shareIndex").Parse(shareIndexHTML))

const shareIndexHTML = `<!doctype html>
<html lang="en" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tt share - {{.RootName}}</title>
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<script>(function(){try{var t=localStorage.getItem('tt-share-theme');if(!t){t=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';}document.documentElement.dataset.theme=t;}catch(e){}})();</script>
<style>
:root{
  --bg:#f8fafc;--panel:#ffffff;--panel-2:#f1f5f9;--text:#0f172a;--muted:#64748b;
  --border:#e2e8f0;--accent:#2563eb;--accent-soft:#eff6ff;--danger:#dc2626;
}
[data-theme="dark"]{
  --bg:#0b1220;--panel:#0f172a;--panel-2:#1e293b;--text:#e2e8f0;--muted:#94a3b8;
  --border:#1e293b;--accent:#60a5fa;--accent-soft:#172554;--danger:#f87171;
}
*{box-sizing:border-box}
[hidden]{display:none !important}
body{margin:0;font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:var(--bg);color:var(--text);height:100vh;display:flex;flex-direction:column}
header{display:flex;align-items:center;gap:12px;padding:10px 16px;background:var(--panel);border-bottom:1px solid var(--border);flex-wrap:wrap}
header img{width:28px;height:28px;border-radius:7px}
.brand{font-weight:700;font-size:15px}
.meta{color:var(--muted);font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.spacer{flex:1}
.btn{display:inline-flex;align-items:center;gap:6px;padding:7px 12px;border-radius:8px;border:1px solid var(--border);background:var(--panel);color:var(--text);font-size:13px;text-decoration:none;cursor:pointer}
.btn:hover{background:var(--panel-2)}
.btn:focus-visible,.file:focus-visible,.dl:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.btn-primary{background:var(--accent);border-color:var(--accent);color:#fff}
.btn-primary:hover{filter:brightness(1.1)}
.icon-btn{width:34px;height:34px;padding:0;justify-content:center;font-size:15px}
main{flex:1;display:grid;grid-template-columns:minmax(240px,320px) 1fr;min-height:0}
.tree{overflow:auto;background:var(--panel);border-right:1px solid var(--border);padding:10px}
.tree ul{list-style:none;margin:0;padding:0}
.tree details{margin:2px 0}
.tree summary{cursor:pointer;padding:6px 8px;border-radius:6px;font-weight:600;font-size:14px;user-select:none}
.tree summary:hover{background:var(--panel-2)}
.row{display:flex;align-items:center;gap:4px}
.file{flex:1;min-width:0;display:flex;justify-content:space-between;gap:10px;padding:6px 8px 6px 22px;border:0;border-radius:6px;background:transparent;color:var(--text);font:inherit;font-size:14px;text-align:left;cursor:pointer}
.file:hover{background:var(--panel-2)}
.file[aria-current="true"]{background:var(--accent-soft);box-shadow:inset 3px 0 0 var(--accent);font-weight:600}
.file .name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.file .size{color:var(--muted);font-size:12px;white-space:nowrap}
.dl{padding:4px 8px;border-radius:6px;color:var(--muted);text-decoration:none;font-size:13px}
.dl:hover{background:var(--panel-2);color:var(--accent)}
details > ul{padding-left:14px}
.preview{display:flex;flex-direction:column;min-width:0;min-height:0}
.preview-head{display:flex;align-items:center;gap:12px;padding:10px 16px;border-bottom:1px solid var(--border);background:var(--panel)}
.preview-head h2{margin:0;font-size:14px;font-weight:600;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.preview-body{flex:1;overflow:auto;padding:16px;min-height:0}
.empty{color:var(--muted);text-align:center;padding:56px 20px}
.preview-body img{max-width:100%;height:auto;border-radius:8px;border:1px solid var(--border)}
.preview-body video{max-width:100%;max-height:100%;border-radius:8px}
.preview-body iframe{width:100%;height:100%;min-height:480px;border:1px solid var(--border);border-radius:8px;background:#fff}
.preview-body pre{margin:0;padding:14px;background:var(--panel);border:1px solid var(--border);border-radius:8px;overflow:auto;font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap;word-break:break-word}
.md{max-width:52rem}
.md h1,.md h2{border-bottom:1px solid var(--border);padding-bottom:.3em}
.md img{max-width:100%}
.md pre{white-space:pre;overflow:auto}
.md code{background:var(--panel-2);padding:2px 5px;border-radius:4px;font-size:.9em}
.md pre code{background:none;padding:0}
.md table{border-collapse:collapse}
.md th,.md td{border:1px solid var(--border);padding:6px 10px}
.md blockquote{margin:0;padding:0 1em;border-left:4px solid var(--border);color:var(--muted)}
.card{display:flex;flex-direction:column;align-items:center;gap:12px;padding:48px 20px;border:1px dashed var(--border);border-radius:12px;color:var(--muted);text-align:center}
@media (max-width:720px){
  main{grid-template-columns:1fr;grid-template-rows:minmax(0,40vh) minmax(0,1fr)}
  .tree{border-right:0;border-bottom:1px solid var(--border)}
}
</style>
</head>
<body>
<header>
  <img src="/favicon.svg" alt="" width="28" height="28">
  <div>
    <div class="brand">tt share</div>
    <div class="meta" id="meta">{{.RootName}} - loading...</div>
  </div>
  <div class="spacer"></div>
  <a class="btn btn-primary" href="/download-all" download>Download all (.zip)</a>
  <button class="btn icon-btn" id="theme-toggle" type="button" aria-label="Switch theme">☽</button>
</header>
<main>
  <nav class="tree" id="tree" aria-label="Shared files"><p class="empty">Loading files...</p></nav>
  <section class="preview">
    <div class="preview-head">
      <h2 id="preview-title">Preview</h2>
      <a class="btn" id="preview-dl" hidden>Download</a>
    </div>
    <div class="preview-body" id="preview-body" aria-live="polite">
      <p class="empty">Select a file to preview.</p>
    </div>
  </section>
</main>
<script>
(function(){
  var byPath = {};
  var currentDir = '';
  var treeEl = document.getElementById('tree');
  var metaEl = document.getElementById('meta');
  var titleEl = document.getElementById('preview-title');
  var bodyEl = document.getElementById('preview-body');
  var dlEl = document.getElementById('preview-dl');
  var themeBtn = document.getElementById('theme-toggle');

  function setTheme(theme, save){
    document.documentElement.dataset.theme = theme;
    themeBtn.setAttribute('aria-label', theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme');
    themeBtn.innerHTML = theme === 'dark' ? '☼' : '☽';
    if (save) { try { localStorage.setItem('tt-share-theme', theme); } catch (e) {} }
  }
  var stored = null;
  try { stored = localStorage.getItem('tt-share-theme'); } catch (e) {}
  setTheme(stored || 'light', false);
  themeBtn.addEventListener('click', function(){
    setTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark', true);
  });

  function encPath(path){
    return path.split('/').map(encodeURIComponent).join('/');
  }

  function fmtSize(size){
    if (size < 1024) { return size + ' B'; }
    if (size < 1024 * 1024) { return (size / 1024).toFixed(1) + ' KB'; }
    return (size / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function fail(message){
    bodyEl.innerHTML = '';
    var card = document.createElement('div');
    card.className = 'card';
    card.textContent = message;
    bodyEl.appendChild(card);
  }

  function buildTree(files){
    treeEl.innerHTML = '';
    if (!files.length) {
      var none = document.createElement('p');
      none.className = 'empty';
      none.textContent = 'No shareable files found.';
      treeEl.appendChild(none);
      return;
    }
    var groups = {};
    var order = [];
    files.forEach(function(f){
      byPath[f.path] = f;
      var idx = f.path.lastIndexOf('/');
      var dir = idx < 0 ? '' : f.path.slice(0, idx);
      if (!groups[dir]) { groups[dir] = []; order.push(dir); }
      groups[dir].push(f);
    });
    order.sort();
    var rootList = document.createElement('ul');
    rootList.setAttribute('role', 'list');
    treeEl.appendChild(rootList);
    order.forEach(function(dir){
      var container = rootList;
      if (dir) {
        var details = document.createElement('details');
        details.open = true;
        var summary = document.createElement('summary');
        summary.textContent = dir;
        details.appendChild(summary);
        var sub = document.createElement('ul');
        sub.setAttribute('role', 'list');
        details.appendChild(sub);
        rootList.appendChild(details);
        container = sub;
      }
      groups[dir].forEach(function(f){
        var li = document.createElement('li');
        var wrap = document.createElement('div');
        wrap.className = 'row';
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'file';
        btn.dataset.path = f.path;
        var name = document.createElement('span');
        name.className = 'name';
        name.textContent = f.name;
        var size = document.createElement('span');
        size.className = 'size';
        size.textContent = fmtSize(f.size);
        btn.appendChild(name);
        btn.appendChild(size);
        btn.addEventListener('click', function(){ select(f); });
        var dl = document.createElement('a');
        dl.className = 'dl';
        dl.href = '/download/' + encPath(f.path);
        dl.textContent = 'Download';
        dl.setAttribute('aria-label', 'Download ' + f.path);
        wrap.appendChild(btn);
        wrap.appendChild(dl);
        li.appendChild(wrap);
        container.appendChild(li);
      });
    });
  }

  function select(f){
    document.querySelectorAll('.file[aria-current]').forEach(function(el){ el.removeAttribute('aria-current'); });
    var active = treeEl.querySelector('.file[data-path="' + CSS.escape(f.path) + '"]');
    if (active) { active.setAttribute('aria-current', 'true'); }
    currentDir = '';
    var idx = f.path.lastIndexOf('/');
    if (idx >= 0) { currentDir = f.path.slice(0, idx); }
    titleEl.textContent = f.path + ' - ' + fmtSize(f.size);
    dlEl.hidden = false;
    dlEl.href = '/download/' + encPath(f.path);
    dlEl.textContent = 'Download';
    bodyEl.innerHTML = '';
    var enc = encPath(f.path);

    if (f.size > 2 * 1024 * 1024 && f.kind === 'text') {
      var card = document.createElement('div');
      card.className = 'card';
      card.textContent = 'File too large to preview (' + fmtSize(f.size) + '). Use Download instead.';
      bodyEl.appendChild(card);
      return;
    }

    if (f.kind === 'image') {
      var img = document.createElement('img');
      img.src = '/file/' + enc;
      img.alt = f.name;
      bodyEl.appendChild(img);
      return;
    }
    if (f.kind === 'video') {
      var video = document.createElement('video');
      video.controls = true;
      video.preload = 'metadata';
      video.src = '/file/' + enc;
      bodyEl.appendChild(video);
      return;
    }
    if (f.kind === 'audio') {
      var audio = document.createElement('audio');
      audio.controls = true;
      audio.preload = 'metadata';
      audio.src = '/file/' + enc;
      bodyEl.appendChild(audio);
      return;
    }
    if (f.kind === 'pdf' || f.kind === 'html') {
      var frame = document.createElement('iframe');
      frame.src = '/file/' + enc;
      frame.title = 'Preview of ' + f.path;
      bodyEl.appendChild(frame);
      return;
    }
    if (f.kind === 'markdown') {
      fetch('/render/' + enc).then(function(resp){
        if (!resp.ok) { throw new Error('HTTP ' + resp.status); }
        return resp.text();
      }).then(function(html){
        var wrap = document.createElement('div');
        wrap.className = 'md';
        wrap.innerHTML = html;
        rewriteLinks(wrap);
        bodyEl.appendChild(wrap);
      }).catch(function(){ fail('Preview failed for this file. Use Download instead.'); });
      return;
    }
    if (f.kind === 'text') {
      fetch('/file/' + enc).then(function(resp){
        if (!resp.ok) { throw new Error('HTTP ' + resp.status); }
        return resp.text();
      }).then(function(text){
        var pre = document.createElement('pre');
        pre.textContent = text;
        bodyEl.appendChild(pre);
      }).catch(function(){ fail('Preview failed for this file. Use Download instead.'); });
      return;
    }
    var card = document.createElement('div');
    card.className = 'card';
    card.textContent = 'No inline preview for this file type. Use Download to get the file.';
    bodyEl.appendChild(card);
  }

  // Resolve relative markdown links against the file's directory: images load
  // through /file/, sibling markdown files navigate inside the preview, and
  // everything external opens in a new tab.
  function resolveRel(url){
    if (/^(https?:|mailto:|data:|#|\/\/)/i.test(url) || url.charAt(0) === '/') { return null; }
    var clean = url.split('#')[0].split('?')[0];
    var suffix = url.slice(clean.length);
    var segs = currentDir ? currentDir.split('/') : [];
    clean.split('/').forEach(function(part){
      if (part === '' || part === '.') { return; }
      if (part === '..') { segs.pop(); return; }
      segs.push(part);
    });
    return { path: segs.join('/'), suffix: suffix };
  }

  function rewriteLinks(scope){
    scope.querySelectorAll('img[src]').forEach(function(img){
      var resolved = resolveRel(img.getAttribute('src'));
      if (resolved && byPath[resolved.path]) {
        img.src = '/file/' + encPath(resolved.path) + resolved.suffix;
      }
    });
    scope.querySelectorAll('a[href]').forEach(function(a){
      var href = a.getAttribute('href');
      if (/^(https?:|mailto:)/i.test(href)) {
        a.target = '_blank';
        a.rel = 'noopener noreferrer';
        return;
      }
      if (href.charAt(0) === '#') { return; }
      var resolved = resolveRel(href);
      if (resolved && byPath[resolved.path]) {
        var target = byPath[resolved.path];
        a.addEventListener('click', function(event){
          event.preventDefault();
          if (target.kind === 'markdown') { select(target); }
          else { window.location.href = '/file/' + encPath(target.path); }
        });
      } else if (href.charAt(0) !== '/') {
        a.target = '_blank';
        a.rel = 'noopener noreferrer';
      }
    });
  }

  fetch('/api/list').then(function(resp){
    if (resp.status === 403) { window.location.reload(); return null; }
    if (!resp.ok) { throw new Error('HTTP ' + resp.status); }
    return resp.json();
  }).then(function(data){
    if (!data) { return; }
    metaEl.textContent = data.root + ' - ' + data.total + ' file(s), read-only';
    buildTree(data.files || []);
  }).catch(function(){
    treeEl.innerHTML = '';
    fail('Could not load the file list. Check the connection and reload.');
    metaEl.textContent = 'failed to load';
  });
})();
</script>
</body>
</html>`
