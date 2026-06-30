package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleSlideTemplateLoadsProjectTTTemplateAndRewritesAssets(t *testing.T) {
	tmp := t.TempDir()
	templateDir := filepath.Join(tmp, ".tt", "slide", "templates", "brand")
	assetsDir := filepath.Join(templateDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "template.json"), []byte(`{
  "name": "brand",
  "revealTheme": "white",
  "css": "template.css",
  "defaults": { "theme": "light", "transition": "fade", "center": false, "width": 1600, "height": 900, "margin": 0 }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "template.css"), []byte(`.cover { background: url("./assets/bg.png") center / cover no-repeat; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "bg.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot := slideRoot
	slideRoot = filepath.Join(tmp, "slides")
	if err := os.MkdirAll(slideRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/api/template/brand", nil)
	rr := httptest.NewRecorder()
	handleSlideTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideTemplate status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `/template-assets/brand/assets/bg.png`) {
		t.Fatalf("template css did not rewrite asset URL: %s", body)
	}
}

func TestHandleSlideTemplateAssetServesTemplateAsset(t *testing.T) {
	tmp := t.TempDir()
	templateDir := filepath.Join(tmp, ".tt", "slide", "templates", "brand")
	assetsDir := filepath.Join(templateDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "template.json"), []byte(`{"name":"brand","revealTheme":"white","css":"template.css","defaults":{"theme":"light","transition":"fade","center":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "bg.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot := slideRoot
	slideRoot = tmp
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/template-assets/brand/assets/bg.png", nil)
	rr := httptest.NewRecorder()
	handleSlideTemplateAsset(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideTemplateAsset status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "png" {
		t.Fatalf("asset body = %q, want png", rr.Body.String())
	}
}
