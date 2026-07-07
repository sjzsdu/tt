package videocmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ScriptMeta struct {
	Title      string  `json:"title,omitempty" yaml:"title"`
	Slides     string  `json:"slides" yaml:"slides"`
	Voice      string  `json:"voice,omitempty" yaml:"voice"`
	Width      int     `json:"width,omitempty" yaml:"width"`
	Height     int     `json:"height,omitempty" yaml:"height"`
	FPS        int     `json:"fps,omitempty" yaml:"fps"`
	RenderMode string  `json:"renderMode,omitempty" yaml:"render_mode"`
	Template   string  `json:"template,omitempty" yaml:"template"`
	Transition string  `json:"transition,omitempty" yaml:"transition"`
	Margin     float64 `json:"margin,omitempty" yaml:"margin"`
}

type Plan struct {
	Script        string         `json:"script"`
	Output        string         `json:"output,omitempty"`
	Meta          ScriptMeta     `json:"meta"`
	Sections      []PlanSection  `json:"sections"`
	TotalDuration DurationMillis `json:"totalDurationMillis"`
}

type PlanSection struct {
	Index          int            `json:"index"`
	Title          string         `json:"title"`
	Slide          int            `json:"slide"`
	Narration      string         `json:"narration"`
	Audio          string         `json:"audio,omitempty"`
	StartMillis    DurationMillis `json:"startMillis"`
	DurationMillis DurationMillis `json:"durationMillis"`
	EndMillis      DurationMillis `json:"endMillis"`
}

type DurationMillis int64

func buildVideoPlanFromFile(scriptPath string, wpm int) (Plan, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return Plan{}, fmt.Errorf("read video script failed: %w", err)
	}
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve script path failed: %w", err)
	}
	return parseVideoScript(abs, string(content), wpm)
}

func parseVideoScript(scriptPath, content string, wpm int) (Plan, error) {
	if wpm <= 0 {
		return Plan{}, fmt.Errorf("wpm must be positive")
	}
	meta, body, err := parseVideoScriptFrontmatter(content)
	if err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(meta.Slides) == "" {
		return Plan{}, fmt.Errorf("video script front matter must set slides")
	}
	if !filepath.IsAbs(meta.Slides) {
		meta.Slides = filepath.Clean(filepath.Join(filepath.Dir(scriptPath), meta.Slides))
	}
	sections, err := parseVideoScriptSections(body, wpm)
	if err != nil {
		return Plan{}, err
	}
	if len(sections) == 0 {
		return Plan{}, fmt.Errorf("video script must contain at least one section")
	}
	var cursor int64
	for i := range sections {
		sections[i].Index = i + 1
		sections[i].StartMillis = DurationMillis(cursor)
		cursor += int64(sections[i].DurationMillis)
		sections[i].EndMillis = DurationMillis(cursor)
	}
	return Plan{Script: scriptPath, Meta: meta, Sections: sections, TotalDuration: DurationMillis(cursor)}, nil
}

func parseVideoScriptFrontmatter(content string) (ScriptMeta, string, error) {
	trimmed := strings.TrimLeft(content, "\ufeff\r\n\t ")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return ScriptMeta{}, content, fmt.Errorf("video script must start with YAML front matter")
	}
	lines := strings.SplitAfter(trimmed, "\n")
	var end int
	pos := 0
	for i, line := range lines {
		pos += len(line)
		if i == 0 {
			continue
		}
		if strings.TrimSpace(line) == "---" {
			end = i
			break
		}
	}
	if end == 0 {
		return ScriptMeta{}, "", fmt.Errorf("video script front matter is not closed")
	}
	frontLines := lines[1:end]
	body := trimmed[pos:]
	var meta ScriptMeta
	if err := yaml.Unmarshal([]byte(strings.Join(frontLines, "")), &meta); err != nil {
		return ScriptMeta{}, "", fmt.Errorf("parse video script front matter failed: %w", err)
	}
	return meta, body, nil
}

var videoHeadingRe = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

func parseVideoScriptSections(body string, wpm int) ([]PlanSection, error) {
	matches := videoHeadingRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("video script body must contain # headings for sections")
	}
	var sections []PlanSection
	for i, match := range matches {
		title := strings.TrimSpace(body[match[2]:match[3]])
		start := match[1]
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		block := strings.TrimSpace(body[start:end])
		section, err := parseVideoScriptSection(title, block, wpm)
		if err != nil {
			return nil, fmt.Errorf("section %q: %w", title, err)
		}
		sections = append(sections, section)
	}
	return sections, nil
}

func parseVideoScriptSection(title, block string, wpm int) (PlanSection, error) {
	lines := strings.Split(block, "\n")
	slide := 0
	var narration []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "slide:") {
			value := strings.TrimSpace(trimmed[len("slide:"):])
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return PlanSection{}, fmt.Errorf("slide must be a positive integer")
			}
			slide = parsed
			continue
		}
		narration = append(narration, line)
	}
	if slide == 0 {
		return PlanSection{}, fmt.Errorf("missing slide: N mapping")
	}
	text := normalizeNarration(strings.Join(narration, "\n"))
	if text == "" {
		return PlanSection{}, fmt.Errorf("missing narration text")
	}
	duration := estimateNarrationDuration(text, wpm)
	return PlanSection{Title: title, Slide: slide, Narration: text, DurationMillis: DurationMillis(duration)}, nil
}

func normalizeNarration(text string) string {
	lines := strings.Split(text, "\n")
	var kept []string
	blank := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(kept) > 0 && !blank {
				kept = append(kept, "")
				blank = true
			}
			continue
		}
		kept = append(kept, trimmed)
		blank = false
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}

func estimateNarrationDuration(text string, wpm int) int64 {
	words := countNarrationUnits(text)
	minutes := float64(words) / float64(wpm)
	millis := int64(math.Ceil(minutes * float64(time.Minute/time.Millisecond)))
	if millis < 1500 {
		millis = 1500
	}
	return millis
}

func countNarrationUnits(text string) int {
	fields := strings.Fields(text)
	units := 0
	for _, field := range fields {
		runes := []rune(strings.TrimSpace(field))
		asciiWord := false
		for _, r := range runes {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				asciiWord = true
				continue
			}
			if r >= 0x4E00 && r <= 0x9FFF {
				units++
			}
		}
		if asciiWord {
			units++
		}
	}
	if units == 0 && strings.TrimSpace(text) != "" {
		return 1
	}
	return units
}

func writeVideoPlan(path string, plan Plan) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create plan directory failed: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal video plan failed: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write video plan failed: %w", err)
	}
	return nil
}

func writeVideoPlanTo(w io.Writer, plan Plan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal video plan failed: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func renderVideoSRT(plan Plan) string {
	sections := append([]PlanSection(nil), plan.Sections...)
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].Index < sections[j].Index })
	var b strings.Builder
	cueIndex := 1
	for _, section := range sections {
		cues := buildVideoSubtitleCues(section)
		for _, cue := range cues {
			fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n", cueIndex, formatSRTTimestamp(cue.Start), formatSRTTimestamp(cue.End), cue.Text)
			cueIndex++
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

type videoSubtitleCue struct {
	Start DurationMillis
	End   DurationMillis
	Text  string
}

func buildVideoSubtitleCues(section PlanSection) []videoSubtitleCue {
	parts := splitVideoSubtitleText(section.Narration)
	if len(parts) == 0 {
		return nil
	}
	start := int64(section.StartMillis)
	end := int64(section.EndMillis)
	if end <= start {
		end = start + int64(section.DurationMillis)
	}
	if end <= start {
		end = start + 1000
	}
	totalWeight := 0
	weights := make([]int, len(parts))
	for i, part := range parts {
		weights[i] = countNarrationUnits(part)
		if weights[i] <= 0 {
			weights[i] = 1
		}
		totalWeight += weights[i]
	}
	cues := make([]videoSubtitleCue, 0, len(parts))
	cursor := start
	duration := end - start
	for i, part := range parts {
		cueEnd := end
		if i+1 < len(parts) {
			cueEnd = start + duration*int64(cumulativeVideoSubtitleWeight(weights[:i+1]))/int64(totalWeight)
			if cueEnd <= cursor {
				cueEnd = cursor + 1
			}
		}
		cues = append(cues, videoSubtitleCue{Start: DurationMillis(cursor), End: DurationMillis(cueEnd), Text: part})
		cursor = cueEnd
	}
	return cues
}

func cumulativeVideoSubtitleWeight(weights []int) int {
	total := 0
	for _, weight := range weights {
		total += weight
	}
	return total
}

func splitVideoSubtitleText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	var b strings.Builder
	flush := func() {
		part := strings.TrimSpace(b.String())
		b.Reset()
		if part != "" {
			parts = append(parts, part)
		}
	}
	runes := []rune(text)
	for i, r := range runes {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			flush()
			continue
		}
		b.WriteRune(r)
		var prev, next rune
		if i > 0 {
			prev = runes[i-1]
		}
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		if isVideoSubtitleSentenceEnd(prev, r, next) {
			flush()
		}
	}
	flush()
	return parts
}

func isVideoSubtitleSentenceEnd(prev, r, next rune) bool {
	switch r {
	case '。', '！', '？', '；', '：':
		return true
	case '.', '!', '?', ';', ':':
		return !isVideoSubtitleWordRune(prev) && !isVideoSubtitleWordRune(next) || next == 0 || next == ' ' || next == '\t'
	default:
		return false
	}
}

func isVideoSubtitleWordRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
}

func formatSRTTimestamp(ms DurationMillis) string {
	total := int64(ms)
	hours := total / 3_600_000
	total %= 3_600_000
	minutes := total / 60_000
	total %= 60_000
	seconds := total / 1000
	millis := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func formatSRTDuration(ms DurationMillis) string {
	d := time.Duration(ms) * time.Millisecond
	return d.Round(time.Millisecond).String()
}
