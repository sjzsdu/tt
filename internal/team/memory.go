package team

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const emptyMemoryContent = `# Team Memory

No durable team memory has been recorded yet.`

type MemoryDocument struct {
	Team         string `json:"team"`
	Version      int    `json:"version"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	SourceThread string `json:"source_thread,omitempty"`
	SourceRound  int    `json:"source_round,omitempty"`
	Content      string `json:"content"`
	Path         string `json:"path,omitempty"`
}

func DefaultMemoryPath(workspace, teamName string) string {
	return filepath.Join(workspace, ".tt", "team-memory", slug(teamName), "memory.md")
}

func ResolveMemoryPath(workspace string, definition *Definition) string {
	if definition == nil || strings.TrimSpace(definition.Memory.Path) == "" {
		name := ""
		if definition != nil {
			name = definition.Team
		}
		return DefaultMemoryPath(workspace, name)
	}
	path := strings.TrimSpace(definition.Memory.Path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(workspace, path)
}

func LoadMemory(path, teamName string) (MemoryDocument, error) {
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return MemoryDocument{
			Team:    strings.TrimSpace(teamName),
			Version: 0,
			Content: emptyMemoryContent,
			Path:    path,
		}, nil
	}
	if err != nil {
		return MemoryDocument{}, fmt.Errorf("read team memory %q: %w", path, err)
	}
	document, err := parseMemoryMarkdown(string(data))
	if err != nil {
		return MemoryDocument{}, fmt.Errorf("parse team memory %q: %w", path, err)
	}
	if document.Team == "" {
		document.Team = strings.TrimSpace(teamName)
	}
	document.Path = path
	return document, nil
}

func SaveMemory(path string, document MemoryDocument) error {
	path = filepath.Clean(path)
	if strings.TrimSpace(document.Team) == "" {
		return fmt.Errorf("team memory team is required")
	}
	if document.Version < 1 {
		return fmt.Errorf("team memory version must be greater than zero")
	}
	if strings.TrimSpace(document.UpdatedAt) == "" {
		document.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	document.Content = normalizeMemoryContent(document.Content)
	if document.Content == "" {
		document.Content = emptyMemoryContent
	}
	data := renderMemoryMarkdown(document)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create team memory directory: %w", err)
	}
	historyPath := filepath.Join(filepath.Dir(path), "versions", fmt.Sprintf("%06d.md", document.Version))
	if err := atomicWriteFile(historyPath, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write team memory history: %w", err)
	}
	if err := atomicWriteFile(path, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write team memory: %w", err)
	}
	return nil
}

func UpgradeMemory(path, teamName, threadID string, round int, content string, maxChars int) (MemoryDocument, error) {
	current, err := LoadMemory(path, teamName)
	if err != nil {
		return MemoryDocument{}, err
	}
	content = normalizeMemoryContent(content)
	if content == "" {
		content = current.Content
	}
	if maxChars > 0 && utf8.RuneCountInString(content) > maxChars {
		return MemoryDocument{}, fmt.Errorf("team memory update exceeds max_chars (%d)", maxChars)
	}
	next := MemoryDocument{
		Team:         strings.TrimSpace(teamName),
		Version:      current.Version + 1,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		SourceThread: strings.TrimSpace(threadID),
		SourceRound:  round,
		Content:      content,
		Path:         filepath.Clean(path),
	}
	if err := SaveMemory(path, next); err != nil {
		return MemoryDocument{}, err
	}
	return next, nil
}

func parseMemoryMarkdown(raw string) (MemoryDocument, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return MemoryDocument{Content: normalizeMemoryContent(raw)}, nil
	}
	rest := strings.TrimPrefix(raw, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return MemoryDocument{}, fmt.Errorf("unterminated front matter")
	}
	header := rest[:end]
	content := rest[end+5:]
	document := MemoryDocument{Content: normalizeMemoryContent(content)}
	scanner := bufio.NewScanner(strings.NewReader(header))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "team":
			document.Team = value
		case "version":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return MemoryDocument{}, fmt.Errorf("invalid version %q", value)
			}
			document.Version = parsed
		case "updated_at":
			document.UpdatedAt = value
		case "source_thread":
			document.SourceThread = value
		case "source_round":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return MemoryDocument{}, fmt.Errorf("invalid source_round %q", value)
			}
			document.SourceRound = parsed
		}
	}
	if err := scanner.Err(); err != nil {
		return MemoryDocument{}, err
	}
	return document, nil
}

func renderMemoryMarkdown(document MemoryDocument) string {
	return fmt.Sprintf(`---
team: %q
version: %d
updated_at: %q
source_thread: %q
source_round: %d
---

%s
`, document.Team, document.Version, document.UpdatedAt, document.SourceThread, document.SourceRound, strings.TrimSpace(document.Content))
}

func normalizeMemoryContent(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	for _, fence := range []string{"```markdown", "```md", "```"} {
		if strings.HasPrefix(content, fence) && strings.HasSuffix(content, "```") {
			content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, fence), "```"))
			break
		}
	}
	return content
}
