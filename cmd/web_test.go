package cmd

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWebRoot(t *testing.T) {
	root := t.TempDir()
	got, openPath, err := resolveWebRoot([]string{root})
	if err != nil {
		t.Fatalf("resolveWebRoot() error = %v", err)
	}
	if got != root || openPath != "/" {
		t.Fatalf("resolveWebRoot() = (%q, %q), want (%q, %q)", got, openPath, root, "/")
	}

	file := filepath.Join(root, "page.html")
	if err := os.WriteFile(file, []byte("<h1>page</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, openPath, err = resolveWebRoot([]string{file})
	if err != nil {
		t.Fatalf("resolveWebRoot(html file) error = %v", err)
	}
	if got != root || openPath != "/page.html" {
		t.Fatalf("resolveWebRoot(html file) = (%q, %q), want (%q, %q)", got, openPath, root, "/page.html")
	}

	nested := filepath.Join(root, "docs", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nested, "page.htm")
	if err := os.WriteFile(nestedFile, []byte("<h1>page</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, openPath, err = resolveWebRoot([]string{nestedFile})
	if err != nil {
		t.Fatalf("resolveWebRoot(nested html file) error = %v", err)
	}
	if wantRoot, wantPath := nested, "/page.htm"; got != wantRoot || openPath != wantPath {
		t.Fatalf("resolveWebRoot(nested html file) = (%q, %q), want (%q, %q)", got, openPath, wantRoot, wantPath)
	}

	notHTML := filepath.Join(root, "README.md")
	if err := os.WriteFile(notHTML, []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveWebRoot([]string{notHTML}); err == nil || !strings.Contains(err.Error(), "must be a directory or an HTML file") {
		t.Fatalf("resolveWebRoot(non-html file) error = %v, want html file error", err)
	}

	missing := filepath.Join(root, "missing.html")
	if _, _, err := resolveWebRoot([]string{missing}); err == nil || !strings.Contains(err.Error(), "stat web target") {
		t.Fatalf("resolveWebRoot(missing file) error = %v, want stat error", err)
	}
}

func TestCollectWebFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"index.html":           "<h1>index</h1>",
		"docs/Guide.HTML":      "<h1>guide</h1>",
		"docs/nested/page.htm": "<h1>page</h1>",
		"README.md":            "# readme",
	} {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "pkg", "ignored.html"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := collectWebFiles(root)
	if err != nil {
		t.Fatalf("collectWebFiles() error = %v", err)
	}
	var got []string
	for _, file := range files {
		got = append(got, file.Relative)
	}
	want := []string{"docs/Guide.HTML", "docs/nested/page.htm", "index.html"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("collectWebFiles() = %v, want %v", got, want)
	}
}

func TestWebHandler(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>home</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := newWebHandler(root)
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest("GET", "http://example.test/", nil))
	if index.Code != 200 || !strings.Contains(index.Body.String(), "index.html") {
		t.Fatalf("GET / status=%d body=%q, want HTML index listing", index.Code, index.Body.String())
	}

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest("GET", "http://example.test/index.html", nil))
	if page.Code != 200 || page.Body.String() != "<h1>home</h1>" {
		t.Fatalf("GET /index.html status=%d body=%q, want rendered file", page.Code, page.Body.String())
	}
	if got := page.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("GET /index.html content type=%q, want text/html", got)
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest("GET", "http://example.test/assets/app.js", nil))
	if asset.Code != 200 || asset.Body.String() != "console.log('ok')" {
		t.Fatalf("GET asset status=%d body=%q, want static asset", asset.Code, asset.Body.String())
	}

	directory := httptest.NewRecorder()
	handler.ServeHTTP(directory, httptest.NewRequest("GET", "http://example.test/assets/", nil))
	if directory.Code != 404 {
		t.Fatalf("GET directory status=%d, want 404 without index.html", directory.Code)
	}
}

func TestWebFavicon(t *testing.T) {
	root := t.TempDir()
	handler := newWebHandler(root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://example.test/favicon.svg", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /favicon.svg status=%d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/svg+xml") {
		t.Fatalf("favicon content type=%q, want image/svg+xml", got)
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("favicon body=%q, want built-in svg markup", rec.Body.String())
	}

	ico := httptest.NewRecorder()
	handler.ServeHTTP(ico, httptest.NewRequest("GET", "http://example.test/favicon.ico", nil))
	if ico.Code != 200 || !strings.Contains(ico.Body.String(), "<svg") {
		t.Fatalf("GET /favicon.ico status=%d body=%q, want built-in svg fallback", ico.Code, ico.Body.String())
	}

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest("GET", "http://example.test/", nil))
	if !strings.Contains(index.Body.String(), `rel="icon"`) || !strings.Contains(index.Body.String(), "/favicon.svg") {
		t.Fatalf("index page missing favicon link, body=%q", index.Body.String())
	}
	if !strings.Contains(index.Body.String(), `<img src="/favicon.svg"`) {
		t.Fatalf("index page missing logo image, body=%q", index.Body.String())
	}

	custom := filepath.Join(root, "favicon.svg")
	if err := os.WriteFile(custom, []byte("<svg>custom</svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	customRec := httptest.NewRecorder()
	handler.ServeHTTP(customRec, httptest.NewRequest("GET", "http://example.test/favicon.svg", nil))
	if customRec.Code != 200 || customRec.Body.String() != "<svg>custom</svg>" {
		t.Fatalf("GET custom favicon status=%d body=%q, want user favicon served", customRec.Code, customRec.Body.String())
	}
}
