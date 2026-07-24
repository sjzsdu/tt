package team

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const emptyMemoryContent = `# Team Memory

No durable team memory has been recorded yet.`

type MemoryDocument struct {
	Team         string  `json:"team"`
	Version      int     `json:"version"`
	UpdatedAt    string  `json:"updated_at,omitempty"`
	SourceThread string  `json:"source_thread,omitempty"`
	SourceRound  int     `json:"source_round,omitempty"`
	SourceEvents []int64 `json:"source_events,omitempty"`
	RestoredFrom int     `json:"restored_from,omitempty"`
	Content      string  `json:"content"`
	Path         string  `json:"path,omitempty"`
}

type MemoryProposal struct {
	ID              string  `json:"id"`
	Team            string  `json:"team"`
	BaseVersion     int     `json:"base_version"`
	ProposedVersion int     `json:"proposed_version"`
	CreatedAt       string  `json:"created_at"`
	SourceThread    string  `json:"source_thread"`
	SourceRound     int     `json:"source_round"`
	SourceEvents    []int64 `json:"source_events,omitempty"`
	Maintainer      string  `json:"maintainer,omitempty"`
	Content         string  `json:"content"`
	Diff            string  `json:"diff"`
	Status          string  `json:"status"`
	Error           string  `json:"error,omitempty"`
}

type MemoryReview struct {
	Current   MemoryDocument   `json:"current"`
	Versions  []MemoryDocument `json:"versions"`
	Proposals []MemoryProposal `json:"proposals"`
}

var unsafeMemoryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|password|passwd|access[_-]?token|secret)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{16,}`),
	regexp.MustCompile(`(?i)\[(unverified|unverified_claim|hidden_reasoning|chain_of_thought)\]`),
	regexp.MustCompile(`\[TEAM_PHASE:[A-Z_]+\]`),
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
	proposal, err := ProposeMemory(path, teamName, threadID, round, nil, "", content, maxChars)
	if err != nil {
		return MemoryDocument{}, err
	}
	return PromoteMemory(path, proposal)
}

func ProposeMemory(path, teamName, threadID string, round int, sourceEvents []int64, maintainer, content string, maxChars int) (MemoryProposal, error) {
	current, err := LoadMemory(path, teamName)
	if err != nil {
		return MemoryProposal{}, err
	}
	content = normalizeMemoryContent(content)
	if content == "" {
		content = current.Content
	}
	proposal := MemoryProposal{
		ID:              memoryProposalID(threadID, round),
		Team:            strings.TrimSpace(teamName),
		BaseVersion:     current.Version,
		ProposedVersion: current.Version + 1,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		SourceThread:    strings.TrimSpace(threadID),
		SourceRound:     round,
		SourceEvents:    append([]int64(nil), sourceEvents...),
		Maintainer:      strings.TrimSpace(maintainer),
		Content:         content,
		Diff:            memoryDiff(current.Content, content, current.Version, current.Version+1),
		Status:          "pending",
	}
	if err := validateMemoryContent(content, maxChars); err != nil {
		proposal.Status = "rejected"
		proposal.Error = err.Error()
		_ = saveMemoryProposal(path, proposal)
		return proposal, err
	}
	if err := saveMemoryProposal(path, proposal); err != nil {
		return MemoryProposal{}, err
	}
	return proposal, nil
}

func PromoteMemory(path string, proposal MemoryProposal) (MemoryDocument, error) {
	current, err := LoadMemory(path, proposal.Team)
	if err != nil {
		return MemoryDocument{}, err
	}
	if current.Version != proposal.BaseVersion {
		return MemoryDocument{}, fmt.Errorf("team memory changed since proposal %s (current v%d, base v%d)", proposal.ID, current.Version, proposal.BaseVersion)
	}
	next := MemoryDocument{
		Team:         proposal.Team,
		Version:      proposal.ProposedVersion,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		SourceThread: proposal.SourceThread,
		SourceRound:  proposal.SourceRound,
		SourceEvents: append([]int64(nil), proposal.SourceEvents...),
		Content:      proposal.Content,
		Path:         filepath.Clean(path),
	}
	if err := SaveMemory(path, next); err != nil {
		return MemoryDocument{}, err
	}
	proposal.Status = "promoted"
	proposal.Error = ""
	if err := saveMemoryProposal(path, proposal); err != nil {
		return MemoryDocument{}, err
	}
	return next, nil
}

func RollbackMemory(path, teamName, threadID string, round, version int) (MemoryDocument, error) {
	target, err := LoadMemoryVersion(path, teamName, version)
	if err != nil {
		return MemoryDocument{}, err
	}
	current, err := LoadMemory(path, teamName)
	if err != nil {
		return MemoryDocument{}, err
	}
	next := target
	next.Version = current.Version + 1
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	next.SourceThread = strings.TrimSpace(threadID)
	next.SourceRound = round
	next.SourceEvents = nil
	next.RestoredFrom = version
	next.Path = filepath.Clean(path)
	if err := SaveMemory(path, next); err != nil {
		return MemoryDocument{}, err
	}
	return next, nil
}

func LoadMemoryVersion(path, teamName string, version int) (MemoryDocument, error) {
	if version < 1 {
		return MemoryDocument{}, fmt.Errorf("team memory version must be greater than zero")
	}
	historyPath := filepath.Join(filepath.Dir(filepath.Clean(path)), "versions", fmt.Sprintf("%06d.md", version))
	document, err := LoadMemory(historyPath, teamName)
	if err != nil {
		return MemoryDocument{}, fmt.Errorf("load team memory version %d: %w", version, err)
	}
	if document.Version != version {
		return MemoryDocument{}, fmt.Errorf("team memory history %d contains version %d", version, document.Version)
	}
	document.Path = historyPath
	return document, nil
}

func LoadMemoryReview(path, teamName string) (MemoryReview, error) {
	current, err := LoadMemory(path, teamName)
	if err != nil {
		return MemoryReview{}, err
	}
	review := MemoryReview{Current: current}
	versionDir := filepath.Join(filepath.Dir(filepath.Clean(path)), "versions")
	entries, err := os.ReadDir(versionDir)
	if err != nil && !os.IsNotExist(err) {
		return MemoryReview{}, fmt.Errorf("list team memory versions: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		version, parseErr := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".md"))
		if parseErr != nil {
			continue
		}
		document, loadErr := LoadMemoryVersion(path, teamName, version)
		if loadErr != nil {
			return MemoryReview{}, loadErr
		}
		review.Versions = append(review.Versions, document)
	}
	sort.Slice(review.Versions, func(i, j int) bool { return review.Versions[i].Version > review.Versions[j].Version })
	proposalDir := filepath.Join(filepath.Dir(filepath.Clean(path)), "proposals")
	proposals, err := os.ReadDir(proposalDir)
	if err != nil && !os.IsNotExist(err) {
		return MemoryReview{}, fmt.Errorf("list team memory proposals: %w", err)
	}
	for _, entry := range proposals {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var proposal MemoryProposal
		if err := readJSON(filepath.Join(proposalDir, entry.Name()), &proposal); err != nil {
			return MemoryReview{}, fmt.Errorf("load team memory proposal %q: %w", entry.Name(), err)
		}
		review.Proposals = append(review.Proposals, proposal)
	}
	sort.Slice(review.Proposals, func(i, j int) bool { return review.Proposals[i].CreatedAt > review.Proposals[j].CreatedAt })
	return review, nil
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
		case "source_events":
			if value != "" {
				for _, item := range strings.Split(value, ",") {
					parsed, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
					if err != nil {
						return MemoryDocument{}, fmt.Errorf("invalid source_events %q", value)
					}
					document.SourceEvents = append(document.SourceEvents, parsed)
				}
			}
		case "restored_from":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return MemoryDocument{}, fmt.Errorf("invalid restored_from %q", value)
			}
			document.RestoredFrom = parsed
		}
	}
	if err := scanner.Err(); err != nil {
		return MemoryDocument{}, err
	}
	return document, nil
}

func renderMemoryMarkdown(document MemoryDocument) string {
	sourceEvents := make([]string, len(document.SourceEvents))
	for i, eventID := range document.SourceEvents {
		sourceEvents[i] = strconv.FormatInt(eventID, 10)
	}
	return fmt.Sprintf(`---
team: %q
version: %d
updated_at: %q
source_thread: %q
source_round: %d
source_events: %q
restored_from: %d
---

%s
`, document.Team, document.Version, document.UpdatedAt, document.SourceThread, document.SourceRound, strings.Join(sourceEvents, ","), document.RestoredFrom, strings.TrimSpace(document.Content))
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

func validateMemoryContent(content string, maxChars int) error {
	if maxChars > 0 && utf8.RuneCountInString(content) > maxChars {
		return fmt.Errorf("team memory update exceeds max_chars (%d)", maxChars)
	}
	for _, pattern := range unsafeMemoryPatterns {
		if pattern.MatchString(content) {
			return fmt.Errorf("team memory update contains unsafe or unverified content matching %q", pattern.String())
		}
	}
	return nil
}

func memoryProposalID(threadID string, round int) string {
	return fmt.Sprintf("%s-round-%04d", slug(strings.ReplaceAll(threadID, "/", "-")), round)
}

func saveMemoryProposal(path string, proposal MemoryProposal) error {
	proposalPath := filepath.Join(filepath.Dir(filepath.Clean(path)), "proposals", proposal.ID+".json")
	if err := writeJSONAtomic(proposalPath, proposal); err != nil {
		return fmt.Errorf("write team memory proposal: %w", err)
	}
	return nil
}

func memoryDiff(before, after string, beforeVersion, afterVersion int) string {
	if before == after {
		return fmt.Sprintf("--- memory v%d\n+++ memory v%d\n", beforeVersion, afterVersion)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "--- memory v%d\n+++ memory v%d\n", beforeVersion, afterVersion)
	for _, line := range strings.Split(strings.TrimSpace(before), "\n") {
		fmt.Fprintf(&builder, "-%s\n", line)
	}
	for _, line := range strings.Split(strings.TrimSpace(after), "\n") {
		fmt.Fprintf(&builder, "+%s\n", line)
	}
	return builder.String()
}
