package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/tt/internal/agents"
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

func TestHandleSlideSaveContentWritesSlideFile(t *testing.T) {
	tmp := t.TempDir()
	slidePath := filepath.Join(tmp, "deck.slide")
	if err := os.WriteFile(slidePath, []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot, oldContent := slideRoot, slideContent
	slideRoot, slideContent = tmp, ""
	defer func() { slideRoot, slideContent = oldRoot, oldContent }()

	body, _ := json.Marshal(slideSaveContentRequest{File: "/deck.slide", Content: "# new\n---\n# slide 2\n"})
	req := httptest.NewRequest(http.MethodPut, "/api/slide/content", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideSaveContent(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideSaveContent status = %d, body = %s", rr.Code, rr.Body.String())
	}
	got, err := os.ReadFile(slidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# new\n---\n# slide 2\n" {
		t.Fatalf("slide content = %q", string(got))
	}
}

func TestHandleSlideSaveContentRejectsNonSlideFile(t *testing.T) {
	tmp := t.TempDir()
	textPath := filepath.Join(tmp, "notes.txt")
	if err := os.WriteFile(textPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot, oldContent := slideRoot, slideContent
	slideRoot, slideContent = tmp, ""
	defer func() { slideRoot, slideContent = oldRoot, oldContent }()

	body, _ := json.Marshal(slideSaveContentRequest{File: "/notes.txt", Content: "new"})
	req := httptest.NewRequest(http.MethodPut, "/api/slide/content", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideSaveContent(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("handleSlideSaveContent status = %d, want 400", rr.Code)
	}
	got, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("non-slide file was modified: %q", string(got))
	}
}

func TestCleanSlideWriterOutputRemovesCodeFence(t *testing.T) {
	got := cleanSlideWriterOutput("```slide\n.media-right\n\n# Title\n```")
	want := ".media-right\n\n# Title"
	if got != want {
		t.Fatalf("cleanSlideWriterOutput = %q, want %q", got, want)
	}
}

func TestBuildSlideWriterPromptIncludesContext(t *testing.T) {
	prompt := buildSlideWriterPrompt(slideRewriteRequest{
		File:          "/deck.slide",
		SlideIndex:    2,
		PreviousSlide: "# prev",
		SlideSource:   "# current",
		NextSlide:     "# next",
		Instruction:   "make it visual",
	})
	for _, want := range []string{"当前页: 3", "# prev", "# current", "# next", "make it visual", "只输出修改后的当前 slide block"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSlideWriterAgentIsEmbedded(t *testing.T) {
	agent, err := agents.Get(agents.SlideWriterID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID != agents.SlideWriterID {
		t.Fatalf("agent ID = %q", agent.ID)
	}
	if !agent.NoHistory {
		t.Fatal("slide-writer should use no_history")
	}
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
