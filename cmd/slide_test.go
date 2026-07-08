package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestHandleSlideRawFileDisablesCache(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "deck.slide"), []byte("# deck\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot := slideRoot
	slideRoot = tmp
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/raw/deck.slide", nil)
	rr := httptest.NewRecorder()
	handleSlideRawFile(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideRawFile status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestHandleSlideRawContentDisablesCache(t *testing.T) {
	oldContent := slideContent
	slideContent = "# inline\n"
	defer func() { slideContent = oldContent }()

	req := httptest.NewRequest(http.MethodGet, "/raw-content", nil)
	rr := httptest.NewRecorder()
	handleSlideRawContent(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideRawContent status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestNormalizeSlideFormulaRunID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "formula run path", raw: "shan-yi-zhe/20260704-140657-c764fd", want: "shan-yi-zhe/20260704-140657-c764fd"},
		{name: "custom protocol URL", raw: "tt-formula-run://shan-yi-zhe/20260704-140657-c764fd", want: "shan-yi-zhe/20260704-140657-c764fd"},
		{name: "custom protocol compact", raw: "tt-formula-run:shan-yi-zhe/20260704-140657-c764fd", want: "shan-yi-zhe/20260704-140657-c764fd"},
		{name: "latest", raw: "latest", want: "latest"},
		{name: "leaf id", raw: "20260704-140657-c764fd", want: "20260704-140657-c764fd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSlideFormulaRunID(tt.raw)
			if err != nil {
				t.Fatalf("normalizeSlideFormulaRunID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeSlideFormulaRunID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSlideFormulaRunIDRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{"", "../run", "/tmp/run", "--web-port", "formula/run/extra", "formula//run", `formula\run`} {
		if got, err := normalizeSlideFormulaRunID(raw); err == nil {
			t.Fatalf("normalizeSlideFormulaRunID(%q) = %q, want error", raw, got)
		}
	}
}

func TestSlideChildServiceManagerDedupesAndShutdownSignalsChildren(t *testing.T) {
	manager := newSlideChildServiceManager()
	starts := 0
	signalFile := filepath.Join(t.TempDir(), "interrupted")
	readyFile := filepath.Join(t.TempDir(), "ready")
	reopened := make(chan string, 1)
	oldOpenBrowser := slideOpenBrowser
	slideOpenBrowser = func(url string) {
		reopened <- url
	}
	t.Cleanup(func() { slideOpenBrowser = oldOpenBrowser })

	manager.startProcess = func(workspace string, args []string) (*exec.Cmd, string, error) {
		starts++
		cmd := exec.Command(os.Args[0], "-test.run=TestSlideChildServiceHelperProcess")
		cmd.Env = append(os.Environ(),
			"TT_SLIDE_HELPER_PROCESS=1",
			"TT_SLIDE_HELPER_SIGNAL_FILE="+signalFile,
			"TT_SLIDE_HELPER_READY_FILE="+readyFile,
		)
		if err := cmd.Start(); err != nil {
			return nil, "", err
		}
		return cmd, "tt-test-helper", nil
	}

	pid1, command1, err := manager.start("same", "test", "target", "", nil, 19999, "http://localhost:19999")
	if err != nil {
		t.Fatalf("first start error = %v", err)
	}
	pid2, command2, err := manager.start("same", "test", "target", "", nil, 19999, "http://localhost:19999")
	if err != nil {
		t.Fatalf("second start error = %v", err)
	}
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
	if pid2 != pid1 || command2 != command1 {
		t.Fatalf("second start returned pid=%d command=%q, want pid=%d command=%q", pid2, command2, pid1, command1)
	}
	select {
	case got := <-reopened:
		if got != "http://localhost:19999" {
			t.Fatalf("reopened URL = %q, want http://localhost:19999", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate start did not reopen service URL")
	}

	waitForFile(t, readyFile, 2*time.Second)
	manager.shutdown(2 * time.Second)
	if _, err := os.Stat(signalFile); err != nil {
		t.Fatalf("helper did not observe interrupt: %v", err)
	}
	if services := manager.snapshot(); len(services) != 0 {
		t.Fatalf("services after shutdown = %d, want 0", len(services))
	}
}

func TestSlideChildServiceHelperProcess(t *testing.T) {
	if os.Getenv("TT_SLIDE_HELPER_PROCESS") != "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	_ = os.WriteFile(os.Getenv("TT_SLIDE_HELPER_READY_FILE"), []byte("ready"), 0o644)
	select {
	case <-signals:
		_ = os.WriteFile(os.Getenv("TT_SLIDE_HELPER_SIGNAL_FILE"), []byte("interrupted"), 0o644)
		os.Exit(0)
	case <-time.After(10 * time.Second):
		os.Exit(2)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNormalizeSlideFormulaName(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain name", raw: "shan-yi-zhe", want: "shan-yi-zhe"},
		{name: "custom protocol URL", raw: "tt-formula-show://shan-yi-zhe", want: "shan-yi-zhe"},
		{name: "custom protocol compact", raw: "tt-formula-show:shan-yi-zhe", want: "shan-yi-zhe"},
		{name: "formula show URL", raw: "tt-formula://show/shan-yi-zhe", want: "shan-yi-zhe"},
		{name: "formula show compact", raw: "tt-formula:show/shan-yi-zhe", want: "shan-yi-zhe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSlideFormulaName(tt.raw)
			if err != nil {
				t.Fatalf("normalizeSlideFormulaName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeSlideFormulaName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSlideFormulaNameRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{"", "../formula", "/tmp/formula", "--markdown", "formula/name", `formula\name`, "bad name", "bad..name"} {
		if got, err := normalizeSlideFormulaName(raw); err == nil {
			t.Fatalf("normalizeSlideFormulaName(%q) = %q, want error", raw, got)
		}
	}
}

func TestNormalizeSlideMarkdownPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative markdown", raw: "docs/intro.md", want: "docs/intro.md"},
		{name: "custom protocol URL", raw: "tt-md://docs/intro.md", want: "docs/intro.md"},
		{name: "custom protocol compact", raw: "tt-md:docs/intro.md", want: "docs/intro.md"},
		{name: "markdown alias protocol", raw: "tt-markdown://docs/intro.markdown", want: "docs/intro.markdown"},
		{name: "url encoded path", raw: "tt-md://docs/intro%20note.md", want: "docs/intro note.md"},
		{name: "dot segment", raw: "./README.md", want: "README.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSlideMarkdownPath(tt.raw)
			if err != nil {
				t.Fatalf("normalizeSlideMarkdownPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeSlideMarkdownPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSlideMarkdownPathRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{"", "../README.md", "/tmp/README.md", "--port.md", "docs/../secret.md", `docs\intro.md`, "docs/intro.txt"} {
		if got, err := normalizeSlideMarkdownPath(raw); err == nil {
			t.Fatalf("normalizeSlideMarkdownPath(%q) = %q, want error", raw, got)
		}
	}
}

func TestHandleSlideOpenFormulaRunRejectsInvalidID(t *testing.T) {
	body, _ := json.Marshal(slideOpenFormulaRunRequest{RunID: "../run"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/formula-run/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenFormulaRun(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("handleSlideOpenFormulaRun status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid formula run id") {
		t.Fatalf("handleSlideOpenFormulaRun body = %s", rr.Body.String())
	}
}

func TestHandleSlideOpenFormulaShowRejectsInvalidName(t *testing.T) {
	body, _ := json.Marshal(slideOpenFormulaShowRequest{Name: "../shan-yi-zhe"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/formula-show/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenFormulaShow(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("handleSlideOpenFormulaShow status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid formula name") {
		t.Fatalf("handleSlideOpenFormulaShow body = %s", rr.Body.String())
	}
}

func TestHandleSlideOpenFormulaShowStartsPreview(t *testing.T) {
	workspace := t.TempDir()
	oldInvocationDir := slideInvocationDir
	slideInvocationDir = workspace
	t.Cleanup(func() { slideInvocationDir = oldInvocationDir })

	oldStart := startSlideFormulaShowPreview
	startSlideFormulaShowPreview = func(gotWorkspace, gotName string, gotPort int) (int, string, error) {
		if gotWorkspace != workspace {
			t.Fatalf("workspace = %q, want %q", gotWorkspace, workspace)
		}
		if gotName != "shan-yi-zhe" {
			t.Fatalf("name = %q, want shan-yi-zhe", gotName)
		}
		if gotPort != 9598 {
			t.Fatalf("port = %d, want 9598", gotPort)
		}
		return 2345, "tt formula show shan-yi-zhe --markdown --port 9598", nil
	}
	t.Cleanup(func() { startSlideFormulaShowPreview = oldStart })

	body, _ := json.Marshal(slideOpenFormulaShowRequest{Name: "tt-formula-show://shan-yi-zhe"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/formula-show/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenFormulaShow(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideOpenFormulaShow status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp slideOpenFormulaShowResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Name != "shan-yi-zhe" || resp.PID != 2345 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestHandleSlideOpenMarkdownRejectsInvalidPath(t *testing.T) {
	body, _ := json.Marshal(slideOpenMarkdownRequest{Path: "../README.md"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/markdown/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenMarkdown(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("handleSlideOpenMarkdown status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid markdown path") {
		t.Fatalf("handleSlideOpenMarkdown body = %s", rr.Body.String())
	}
}

func TestHandleSlideOpenMarkdownRejectsMissingFile(t *testing.T) {
	oldInvocationDir := slideInvocationDir
	slideInvocationDir = t.TempDir()
	t.Cleanup(func() { slideInvocationDir = oldInvocationDir })

	body, _ := json.Marshal(slideOpenMarkdownRequest{Path: "missing.md"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/markdown/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenMarkdown(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("handleSlideOpenMarkdown status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "markdown file not found") {
		t.Fatalf("handleSlideOpenMarkdown body = %s", rr.Body.String())
	}
}

func TestHandleSlideOpenMarkdownStartsViewer(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "docs", "intro.md"), []byte("# Intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldInvocationDir := slideInvocationDir
	slideInvocationDir = workspace
	t.Cleanup(func() { slideInvocationDir = oldInvocationDir })

	oldStart := startSlideMarkdownViewer
	startSlideMarkdownViewer = func(gotWorkspace, gotPath string, gotPort int) (int, string, error) {
		if gotWorkspace != workspace {
			t.Fatalf("workspace = %q, want %q", gotWorkspace, workspace)
		}
		if gotPath != "docs/intro.md" {
			t.Fatalf("path = %q, want docs/intro.md", gotPath)
		}
		if gotPort != 9595 {
			t.Fatalf("port = %d, want 9595", gotPort)
		}
		return 1234, "tt md docs/intro.md --port 9595", nil
	}
	t.Cleanup(func() { startSlideMarkdownViewer = oldStart })

	body, _ := json.Marshal(slideOpenMarkdownRequest{Path: "tt-md://docs/intro.md"})
	req := httptest.NewRequest(http.MethodPost, "/api/actions/markdown/open", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSlideOpenMarkdown(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideOpenMarkdown status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp slideOpenMarkdownResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Path != "docs/intro.md" || resp.PID != 1234 {
		t.Fatalf("response = %+v", resp)
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

func TestHandleSlideWidgetsLoadsProjectWidgets(t *testing.T) {
	tmp := t.TempDir()
	widgetDir := filepath.Join(tmp, ".tt", "slide", "widgets")
	if err := os.MkdirAll(widgetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetDir, "gua.html"), []byte(`<div>{{ name }}</div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(widgetDir, "gua.css"), []byte(`.gua{color:red}`), 0o644); err != nil {
		t.Fatal(err)
	}

	oldRoot := slideRoot
	slideRoot = filepath.Join(tmp, "slides")
	if err := os.MkdirAll(slideRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/api/widgets", nil)
	rr := httptest.NewRecorder()
	handleSlideWidgets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideWidgets status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"gua"`, `\u003cdiv\u003e{{ name }}\u003c/div\u003e`, `.gua{color:red}`, `"source":"project"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("widgets response missing %q:\n%s", want, body)
		}
	}
}

func TestHandleSlideTemplateLoadsBuiltInMagicloud(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := slideRoot
	slideRoot = filepath.Join(tmp, "slides")
	if err := os.MkdirAll(slideRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/api/template/magicloud", nil)
	rr := httptest.NewRecorder()
	handleSlideTemplate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideTemplate built-in magicloud status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"name":"magicloud"`, `/template-assets/magicloud/assets/logo-dark.png`, `--cover-background`} {
		if !strings.Contains(body, want) {
			t.Fatalf("built-in magicloud response missing %q:\n%s", want, body)
		}
	}
}

func TestHandleSlideTemplateAssetServesBuiltInMagicloudAsset(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := slideRoot
	slideRoot = filepath.Join(tmp, "slides")
	if err := os.MkdirAll(slideRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/template-assets/magicloud/assets/logo-dark.png", nil)
	rr := httptest.NewRecorder()
	handleSlideTemplateAsset(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideTemplateAsset built-in magicloud status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() == 0 {
		t.Fatal("built-in magicloud asset body is empty")
	}
}

func TestHandleSlideWidgetsLoadsBuiltInWidgets(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := slideRoot
	slideRoot = filepath.Join(tmp, "slides")
	if err := os.MkdirAll(slideRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { slideRoot = oldRoot }()

	req := httptest.NewRequest(http.MethodGet, "/api/widgets", nil)
	rr := httptest.NewRecorder()
	handleSlideWidgets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handleSlideWidgets built-in status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`"64gua"`, `"64gua-full"`, `"bagua"`, `"bianyao"`, `"gua"`, `"source":"built-in"`, `gua64-card`, `gua64full-stage`, `noteSummary`, `gua64-yao-label`, `bianyao-stage`, `changesHtml`} {
		if !strings.Contains(body, want) {
			t.Fatalf("built-in widgets response missing %q:\n%s", want, body)
		}
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
	for _, want := range []string{"Available slide templates:", "brand", "project", templateDir, "dark", "built-in", "white", "magicloud"} {
		if !strings.Contains(got, want) {
			t.Fatalf("template list missing %q:\n%s", want, got)
		}
	}
}
