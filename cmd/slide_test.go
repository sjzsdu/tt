package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSlideTemplateFixture(t *testing.T, root, name string) string {
	t.Helper()
	templateDir := filepath.Join(root, ".tt", "slide", "templates", name)
	assetsDir := filepath.Join(templateDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "template.json"), []byte(`{"name":"`+name+`","revealTheme":"white","css":"template.css","defaults":{"theme":"light","transition":"fade","center":false},"vars":{"slide-padding":"96px 112px 72px","cover-bg":"url(\"./assets/bg.png\") center / cover no-repeat"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "template.css"), []byte(`.cover { background: url("./assets/bg.png") center / cover no-repeat; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "bg.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return templateDir
}

func TestHandleSlideTemplateLoadsProjectTTTemplateAndRewritesAssets(t *testing.T) {
	tmp := t.TempDir()
	writeSlideTemplateFixture(t, tmp, "brand")

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
	if !strings.Contains(body, `--slide-padding: 96px 112px 72px`) || !strings.Contains(body, `--cover-bg: url(\"/template-assets/brand/assets/bg.png\") center / cover no-repeat`) {
		t.Fatalf("template vars were not injected/re-written: %s", body)
	}
}

func TestHandleSlideTemplateAssetServesTemplateAsset(t *testing.T) {
	tmp := t.TempDir()
	writeSlideTemplateFixture(t, tmp, "brand")

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

func TestPrintSlideTemplatesListsBuiltInAndProjectTemplates(t *testing.T) {
	tmp := t.TempDir()
	templateDir := writeSlideTemplateFixture(t, tmp, "brand")
	oldRoot := slideRoot
	slideRoot = tmp
	defer func() { slideRoot = oldRoot }()

	var out bytes.Buffer
	if err := printSlideTemplates(&out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Available slide templates:", "brand", "project", templateDir, "dark", "built-in", "white"} {
		if !strings.Contains(got, want) {
			t.Fatalf("template list missing %q:\n%s", want, got)
		}
	}
}
