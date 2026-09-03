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
	got, err := resolveWebRoot([]string{root})
	if err != nil {
		t.Fatalf("resolveWebRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("resolveWebRoot() = %q, want %q", got, root)
	}

	file := filepath.Join(root, "page.html")
	if err := os.WriteFile(file, []byte("<h1>page</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWebRoot([]string{file}); err == nil || !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("resolveWebRoot(file) error = %v, want directory error", err)
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
