package generate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sjzsdu/tt/internal/util"
)

type Files struct {
	Agent string
	Soul  string
}

type PromptOptions struct {
	Role    string
	Context string
}

type EmbeddedOptions struct {
	ID        string
	Name      string
	Skills    []string
	Tools     []string
	NoHistory bool
}

func BuildGenerationPrompt(opt PromptOptions) (string, error) {
	role := strings.TrimSpace(opt.Role)
	if role == "" {
		return "", fmt.Errorf("role required")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "请为下面这个 agent 角色生成 OpenClaw / Picoclaw 使用的 Agent.md 和 soul.md。\n\n角色：%s\n", role)
	if ctx := strings.TrimSpace(opt.Context); ctx != "" {
		fmt.Fprintf(&b, "\n额外上下文：\n%s\n", ctx)
	}
	b.WriteString(`
请先在心里分析这个职业真实的工作方式，再输出最终文件。要求：
- 默认按 standard 长度生成：Agent.md 约 900-1400 个中文字符，soul.md 约 400-700 个中文字符，合计尽量不超过 2200 个中文字符。
- 提示词必须高密度；每条规则都应该能改变模型行为。删掉重复解释、背景铺垫、泛泛价值观和过细枚举。
- 内容必须针对这个角色定制，不能套用通用模板。
- 写出该角色独有的判断框架、工作流程、交付物、质量标准、风险边界和自检方式。
- Agent.md 偏操作说明，让模型知道遇到任务时怎么推进。
- soul.md 偏内在气质，让模型知道应该如何取舍、坚持什么、避免什么。
- Agent.md 必须能回答：这个角色服务谁、负责什么、不负责什么、如何澄清需求、如何推进任务、交付物是什么、如何验收。
- soul.md 必须体现职业信念、取舍偏好、反模式、压力下的判断，而不是重复操作步骤。
- 禁止使用"专业、高效、负责"这类没有行为约束的空话。
- 禁止输出所有职业都能复用的通用段落。
- 默认中文。
- 只输出 <Agent.md>...</Agent.md> 和 <soul.md>...</soul.md> 两个标签。
`)
	return b.String(), nil
}

func ParseResponse(response string) (Files, error) {
	agent := extractTagged(response, "Agent.md")
	soul := extractTagged(response, "soul.md")
	if agent == "" || soul == "" {
		return Files{}, fmt.Errorf("model response must contain <Agent.md>...</Agent.md> and <soul.md>...</soul.md>")
	}
	return Files{Agent: ensureTrailingNewline(agent), Soul: ensureTrailingNewline(soul)}, nil
}

func RenderEmbeddedMarkdown(files Files, opt EmbeddedOptions) (string, error) {
	id := strings.TrimSpace(opt.ID)
	if id == "" {
		return "", fmt.Errorf("embedded agent id required")
	}
	name := strings.TrimSpace(opt.Name)
	if name == "" {
		name = id
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", yamlScalar(id))
	fmt.Fprintf(&b, "name: %s\n", yamlScalar(name))
	if len(compactStrings(opt.Skills)) > 0 {
		b.WriteString("skills:\n")
		for _, skill := range compactStrings(opt.Skills) {
			fmt.Fprintf(&b, "  - %s\n", yamlScalar(skill))
		}
	}
	if len(compactStrings(opt.Tools)) > 0 {
		b.WriteString("tools:\n")
		for _, tool := range compactStrings(opt.Tools) {
			fmt.Fprintf(&b, "  - %s\n", yamlScalar(tool))
		}
	}
	fmt.Fprintf(&b, "no_history: %t\n", opt.NoHistory)
	if soul := strings.TrimSpace(files.Soul); soul != "" {
		b.WriteString("soul: |\n")
		for _, line := range strings.Split(soul, "\n") {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimSpace(files.Agent))
	b.WriteString("\n")
	return b.String(), nil
}

func DefaultEmbeddedID(role string) string {
	slug := strings.ToLower(strings.TrimSpace(role))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "agent"
	}
	return slug
}

func extractTagged(input, tag string) string {
	re := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `>\s*(.*?)\s*</` + regexp.QuoteMeta(tag) + `>`)
	m := re.FindStringSubmatch(input)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(stripMarkdownFence(m[1]))
}

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return s
}

func ensureTrailingNewline(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func compactStrings(values []string) []string {
	return util.CompactStrings(values)
}

func yamlScalar(value string) string {
	return util.YamlScalar(value)
}
