package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveShareRoot(t *testing.T) {
	root := t.TempDir()
	got, err := resolveShareRoot([]string{root})
	if err != nil {
		t.Fatalf("resolveShareRoot() error = %v", err)
	}
	absRoot, _ := filepath.Abs(root)
	if got != absRoot {
		t.Fatalf("resolveShareRoot() = %q, want %q", got, absRoot)
	}

	file := filepath.Join(root, "page.html")
	if err := os.WriteFile(file, []byte("<h1>page</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveShareRoot([]string{file}); err == nil || !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("resolveShareRoot(file) error = %v, want directory error", err)
	}

	if _, err := resolveShareRoot([]string{filepath.Join(root, "missing")}); err == nil || !strings.Contains(err.Error(), "stat share target") {
		t.Fatalf("resolveShareRoot(missing) error = %v, want stat error", err)
	}
}

func TestShareSkipName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"readme.md", false},
		{"photos", false},
		{".git", true},
		{".env", true},
		{".hidden", true},
		{"node_modules", true},
		{"vendor", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shareSkipName(tt.name); got != tt.want {
				t.Fatalf("shareSkipName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestShareFileKind(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"README.md", "markdown"},
		{"notes.MARKDOWN", "markdown"},
		{"index.html", "html"},
		{"page.htm", "html"},
		{"doc.pdf", "pdf"},
		{"logo.png", "image"},
		{"logo.svg", "image"},
		{"clip.mp4", "video"},
		{"song.mp3", "audio"},
		{"main.go", "text"},
		{"data.json", "text"},
		{"Makefile", "text"},
		{"archive.zip", "file"},
		{"binary", "file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shareFileKind(tt.name); got != tt.want {
				t.Fatalf("shareFileKind(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestShareMatchesPattern(t *testing.T) {
	tests := []struct {
		patterns []string
		relPath  string
		want     bool
	}{
		{nil, "docs/readme.md", true},
		{[]string{"*.md"}, "readme.md", true},
		{[]string{"*.md"}, "photo.png", false},
		{[]string{"docs/*"}, "docs/readme.md", true},
		{[]string{"docs/**"}, "docs/api/index.md", true},
		{[]string{"  "}, "photo.png", true},
	}
	for _, tt := range tests {
		name := strings.Join(tt.patterns, ",") + "|" + tt.relPath
		t.Run(name, func(t *testing.T) {
			got := shareMatchesPattern(tt.patterns, tt.relPath, filepath.Base(tt.relPath))
			if got != tt.want {
				t.Fatalf("shareMatchesPattern(%v, %q) = %v, want %v", tt.patterns, tt.relPath, got, tt.want)
			}
		})
	}
}

func TestShareIsLANIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.5", true},
		{"10.0.0.7", true},
		{"172.16.3.4", true},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"0.0.0.0", false},
		{"::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := shareIsLANIP(net.ParseIP(tt.ip)); got != tt.want {
				t.Fatalf("shareIsLANIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// shareFixture builds a directory tree covering every listing rule: normal
// files, nested dirs, hidden entries, dependency dirs, and a symlink leaving
// the shared root.
func shareFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"README.md":                 "# Share\n\nhello **world**\n",
		"docs/guide.md":             "# Guide",
		"docs/notes.txt":            "plain text",
		"img/logo.png":              "png-bytes",
		".env":                      "SECRET=1",
		".hidden/secret.md":         "hidden",
		"node_modules/pkg/index.js": "alert(1)",
		"vendor/lib.go":             "package lib",
	}
	for path, content := range files {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(filepath.Dir(root), "outside-"+filepath.Base(root)+".txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Logf("symlink unavailable, skipping escape coverage: %v", err)
	}
	return root
}

func shareTestHandler(t *testing.T, token string, patterns []string) http.Handler {
	t.Helper()
	handler, err := newShareHandler(shareFixture(t), patterns, token)
	if err != nil {
		t.Fatalf("newShareHandler() error = %v", err)
	}
	return handler
}

func TestShareListsOnlyShareableFiles(t *testing.T) {
	handler := shareTestHandler(t, "", nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/api/list", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/list status = %d, want 200", rec.Code)
	}

	var payload struct {
		Root  string       `json:"root"`
		Total int          `json:"total"`
		Files []shareEntry `json:"files"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list: %v", err)
	}

	var got []string
	for _, file := range payload.Files {
		got = append(got, file.Path)
	}
	want := []string{"README.md", "docs/guide.md", "docs/notes.txt", "img/logo.png"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list = %v, want %v", got, want)
	}
	if payload.Total != len(want) {
		t.Fatalf("total = %d, want %d", payload.Total, len(want))
	}
	for _, file := range payload.Files {
		if file.Kind == "" {
			t.Fatalf("entry %q missing kind", file.Path)
		}
	}
}

func TestShareListHonorsPattern(t *testing.T) {
	handler := shareTestHandler(t, "", []string{"*.md"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/api/list", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/list status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "README.md") || !strings.Contains(body, "docs/guide.md") {
		t.Fatalf("pattern list missing markdown files: %s", body)
	}
	if strings.Contains(body, "notes.txt") || strings.Contains(body, "logo.png") {
		t.Fatalf("pattern list leaked non-matching files: %s", body)
	}
}

func TestShareIndexPage(t *testing.T) {
	handler := shareTestHandler(t, "", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Download all (.zip)") {
		t.Fatalf("index page missing zip download action")
	}
	if !strings.Contains(body, "theme-toggle") {
		t.Fatalf("index page missing theme toggle")
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("index content type = %q, want text/html", contentType)
	}
}

func TestShareTokenAuth(t *testing.T) {
	handler := shareTestHandler(t, "s3cret", nil)

	tests := []struct {
		name       string
		target     string
		cookie     *http.Cookie
		wantStatus int
		wantCookie bool
	}{
		{name: "missing token rejected", target: "http://share.test/", wantStatus: http.StatusForbidden},
		{name: "wrong token rejected", target: "http://share.test/?t=wrong", wantStatus: http.StatusForbidden},
		{name: "query token accepted", target: "http://share.test/?t=s3cret", wantStatus: http.StatusOK, wantCookie: true},
		{
			name:       "cookie accepted",
			target:     "http://share.test/api/list",
			cookie:     &http.Cookie{Name: shareCookieName, Value: "s3cret"},
			wantStatus: http.StatusOK,
		},
		{name: "api without token rejected", target: "http://share.test/api/list", wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.target, nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			setCookie := rec.Header().Get("Set-Cookie")
			if tt.wantCookie && !strings.Contains(setCookie, shareCookieName+"=s3cret") {
				t.Fatalf("Set-Cookie = %q, want %s token cookie", setCookie, shareCookieName)
			}
			if !tt.wantCookie && setCookie != "" && strings.Contains(setCookie, shareCookieName) {
				t.Fatalf("unexpected auth cookie on failure: %q", setCookie)
			}
		})
	}
}

func TestShareTokenSetOnEveryAuthorizedQuery(t *testing.T) {
	handler := shareTestHandler(t, "s3cret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/api/list?t=s3cret", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if setCookie := rec.Header().Get("Set-Cookie"); !strings.Contains(setCookie, shareCookieName+"=s3cret") {
		t.Fatalf("Set-Cookie = %q, want token cookie", setCookie)
	}
}

func TestShareIsReadOnly(t *testing.T) {
	handler := shareTestHandler(t, "", nil)

	tests := []struct {
		method string
		target string
	}{
		{http.MethodPost, "http://share.test/"},
		{http.MethodPost, "http://share.test/api/list"},
		{http.MethodPut, "http://share.test/file/README.md"},
		{http.MethodDelete, "http://share.test/file/README.md"},
		{http.MethodPatch, "http://share.test/download/README.md"},
	}
	for _, tt := range tests {
		name := tt.method + " " + tt.target
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader("x=1"))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", rec.Code)
			}
			if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Fatalf("Allow = %q, want GET, HEAD", allow)
			}
		})
	}
}

func TestShareFileInlineAndDownload(t *testing.T) {
	handler := shareTestHandler(t, "", nil)

	inline := httptest.NewRecorder()
	handler.ServeHTTP(inline, httptest.NewRequest("GET", "http://share.test/file/README.md", nil))
	if inline.Code != http.StatusOK {
		t.Fatalf("GET /file/README.md status = %d, want 200", inline.Code)
	}
	if got := inline.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("inline Content-Disposition = %q, want empty", got)
	}
	if !strings.Contains(inline.Body.String(), "hello **world**") {
		t.Fatalf("inline body = %q, want raw markdown", inline.Body.String())
	}

	download := httptest.NewRecorder()
	handler.ServeHTTP(download, httptest.NewRequest("GET", "http://share.test/download/README.md", nil))
	if download.Code != http.StatusOK {
		t.Fatalf("GET /download/README.md status = %d, want 200", download.Code)
	}
	if disposition := download.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "README.md") {
		t.Fatalf("download Content-Disposition = %q, want attachment README.md", disposition)
	}
}

func TestShareRejectsUnsafePaths(t *testing.T) {
	handler := shareTestHandler(t, "", nil)

	tests := []struct {
		name   string
		target string
	}{
		{"traversal", "http://share.test/file/../../etc/passwd"},
		{"encoded traversal", "http://share.test/file/%2e%2e/%2e%2e/etc/passwd"},
		{"hidden file", "http://share.test/file/.env"},
		{"hidden dir", "http://share.test/file/.hidden/secret.md"},
		{"dependency dir", "http://share.test/file/node_modules/pkg/index.js"},
		{"vendor dir", "http://share.test/file/vendor/lib.go"},
		{"symlink escape", "http://share.test/file/escape.txt"},
		{"symlink escape download", "http://share.test/download/escape.txt"},
		{"missing file", "http://share.test/file/nope.txt"},
		{"directory", "http://share.test/file/docs"},
		{"unknown route", "http://share.test/save/README.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.target, nil)
			// httptest keeps raw path segments; Go's server would clean
			// unencoded traversal before the handler, so normalize like the
			// real mux does for the encoded case.
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
				t.Fatalf("GET %s status = %d, want 404/400", tt.target, rec.Code)
			}
		})
	}
}

func TestShareRenderMarkdownStripsHTML(t *testing.T) {
	root := shareFixture(t)
	if err := os.WriteFile(filepath.Join(root, "evil.md"), []byte("# hi\n\n<script>alert(1)</script>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newShareHandler(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/render/README.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /render/README.md status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<strong>world</strong>") {
		t.Fatalf("rendered markdown missing emphasis: %s", body)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("CSP = %q, want sandbox", csp)
	}

	// Raw HTML in markdown must be dropped, not passed through.
	evilRec := httptest.NewRecorder()
	handler.ServeHTTP(evilRec, httptest.NewRequest("GET", "http://share.test/render/evil.md", nil))
	if evilRec.Code != http.StatusOK {
		t.Fatalf("GET /render/evil.md status = %d, want 200", evilRec.Code)
	}
	if strings.Contains(evilRec.Body.String(), "<script") {
		t.Fatalf("rendered markdown leaked raw HTML: %s", evilRec.Body.String())
	}
}

func TestShareRenderRejectsNonMarkdown(t *testing.T) {
	handler := shareTestHandler(t, "", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/render/notes.txt", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestShareServesSVGPreviewHeader(t *testing.T) {
	root := shareFixture(t)
	if err := os.WriteFile(filepath.Join(root, "logo.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newShareHandler(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/file/logo.svg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("svg CSP = %q, want sandbox", csp)
	}
}

func TestShareDownloadAllZip(t *testing.T) {
	handler := shareTestHandler(t, "", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/download-all", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /download-all status = %d, want 200", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Fatalf("content type = %q, want application/zip", contentType)
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", disposition)
	}

	archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var names []string
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	got := strings.Join(names, "\n")
	for _, want := range []string{"README.md", "docs/guide.md", "docs/notes.txt", "img/logo.png"} {
		if !strings.Contains(got, want) {
			t.Fatalf("zip missing %q, got: %s", want, got)
		}
	}
	for _, banned := range []string{".env", "node_modules", "vendor", "escape.txt"} {
		if strings.Contains(got, banned) {
			t.Fatalf("zip leaked %q, got: %s", banned, got)
		}
	}

	// The archive must actually decompress to the original content.
	for _, file := range archive.File {
		if file.Name != "README.md" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry: %v", err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatalf("read zip entry: %v", err)
		}
		if !strings.Contains(string(content), "hello **world**") {
			t.Fatalf("zip README content = %q", content)
		}
	}
}

func TestShareFavicon(t *testing.T) {
	handler := shareTestHandler(t, "", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://share.test/favicon.svg", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "image/svg") {
		t.Fatalf("content type = %q, want image/svg", contentType)
	}
}

func TestResolveShareToken(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		noAuth    bool
		want      string
	}{
		{name: "default is stable", want: defaultShareToken},
		{name: "explicit token wins", flagValue: "mysecret", want: "mysecret"},
		{name: "explicit token trimmed", flagValue: "  mysecret  ", want: "mysecret"},
		{name: "no-auth disables token", flagValue: "mysecret", noAuth: true, want: ""},
		{name: "no-auth without token", noAuth: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveShareToken(tt.flagValue, tt.noAuth); got != tt.want {
				t.Fatalf("resolveShareToken(%q, %v) = %q, want %q", tt.flagValue, tt.noAuth, got, tt.want)
			}
		})
	}
	if defaultShareToken == "" {
		t.Fatal("defaultShareToken must not be empty")
	}
}
