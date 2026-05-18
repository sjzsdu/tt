package nvwa

import (
	"strings"
	"testing"
)

func TestBuildGenerationPromptIncludesRoleAndContext(t *testing.T) {
	prompt, err := BuildGenerationPrompt(PromptOptions{Role: "前端开发工程师", Context: "偏 B 端后台"})
	if err != nil {
		t.Fatalf("BuildGenerationPrompt returned error: %v", err)
	}
	for _, want := range []string{"前端开发工程师", "偏 B 端后台", "standard 长度", "900-1400", "400-700", "不能套用通用模板", "<Agent.md>", "<soul.md>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestBuildGenerationPromptRequiresRole(t *testing.T) {
	if _, err := BuildGenerationPrompt(PromptOptions{Role: "  \t\n "}); err == nil {
		t.Fatalf("BuildGenerationPrompt should reject empty role")
	}
}

func TestParseResponseExtractsFiles(t *testing.T) {
	files, err := ParseResponse(`<Agent.md>
# 前端 Agent

负责交付 UI。
</Agent.md>
<soul.md>
# 前端 Soul

关心用户体验。
</soul.md>`)
	if err != nil {
		t.Fatalf("ParseResponse returned error: %v", err)
	}
	if !strings.Contains(files.Agent, "负责交付 UI") {
		t.Fatalf("Agent content not extracted: %q", files.Agent)
	}
	if !strings.Contains(files.Soul, "关心用户体验") {
		t.Fatalf("Soul content not extracted: %q", files.Soul)
	}
}

func TestParseResponseRequiresBothFiles(t *testing.T) {
	if _, err := ParseResponse(`<Agent.md># Only Agent</Agent.md>`); err == nil {
		t.Fatalf("ParseResponse should require both files")
	}
}

func TestRenderEmbeddedMarkdown(t *testing.T) {
	doc, err := RenderEmbeddedMarkdown(Files{
		Agent: "# 前端工程师\n\n负责 UI 交付。\n",
		Soul:  "# 灵魂\n\n重视体验。\n",
	}, EmbeddedOptions{
		ID:                  "frontend-engineer",
		Name:                "前端工程师",
		Skills:              []string{"browser", "ui-review"},
		NoHistory:           true,
		EnableResearchTools: true,
	})
	if err != nil {
		t.Fatalf("RenderEmbeddedMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"---\n",
		"id: frontend-engineer",
		"name: \"前端工程师\"",
		"skills:\n  - browser\n  - ui-review",
		"no_history: true",
		"enable_research_tools: true",
		"soul: |\n  # 灵魂",
		"# 前端工程师\n\n负责 UI 交付。",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("embedded doc missing %q:\n%s", want, doc)
		}
	}
}

func TestDefaultEmbeddedID(t *testing.T) {
	if got := DefaultEmbeddedID("Go 后端工程师"); got != "go" {
		t.Fatalf("DefaultEmbeddedID mismatch: %q", got)
	}
	if got := DefaultEmbeddedID("前端开发工程师"); got != "nvwa-agent" {
		t.Fatalf("DefaultEmbeddedID should fallback for non-ascii role: %q", got)
	}
}
