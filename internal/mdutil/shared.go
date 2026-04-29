package mdutil

import (
	"html"
	"html/template"
	"strings"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Raw            string
	Frontmatter    string
	Body           string
	HasFrontmatter bool
}

func SplitDocument(content string) Document {
	doc := Document{Raw: content, Body: content}
	trimmed := strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(trimmed, "---") {
		return doc
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return doc
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			doc.Frontmatter = strings.Join(lines[1:i], "\n")
			doc.Body = strings.Join(lines[i+1:], "\n")
			doc.HasFrontmatter = true
			return doc
		}
	}
	return doc
}

func ParseYAMLFrontmatter(frontmatter string) (any, error) {
	frontmatter = strings.TrimSpace(frontmatter)
	if frontmatter == "" {
		return nil, nil
	}
	var data any
	if err := yaml.Unmarshal([]byte(frontmatter), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func RenderMarkdownBlock(content string) template.HTML {
	escaped := html.EscapeString(strings.TrimSpace(content))
	if escaped == "" {
		return template.HTML("<div class=\"empty\">No content</div>")
	}
	return template.HTML(`
<div class="md-block">
  <div class="md-actions">
    <button type="button" class="copy-btn" data-copy-source>⧉</button>
  </div>
  <div class="markdown-body md-render"></div>
  <script type="text/plain" class="md-source">` + escaped + `</script>
</div>`)
}
