package cmd

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

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	videoOutPath     string
	videoPlanPath    string
	videoSRTPath     string
	videoWordsPerMin int
	videoJSON        bool
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Generate videos from script-driven slide narrations",
	Long:  "Generate videos from a centralized markdown script that maps narration sections to .slide pages.",
}

var videoGenerateCmd = &cobra.Command{
	Use:   "generate <script.md>",
	Short: "Generate a slide video plan and subtitles from a script file",
	Long: `Generate a slide-video production plan from a centralized script file.

Script format:

  ---
  slides: ./deck.slide
  voice: zh-CN-XiaoxiaoNeural
  width: 1920
  height: 1080
  fps: 30
  ---

  # Opening
  slide: 1

  Narration shown as subtitles and used for speech synthesis.

  # Problem
  slide: 2

  More narration.

This MVP validates the script, writes a deterministic JSON plan, and writes SRT
subtitles. Rendering backends for screenshot, TTS, and ffmpeg composition can
consume the plan in a follow-up step.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := buildVideoPlanFromFile(args[0], videoWordsPerMin)
		if err != nil {
			return err
		}
		if videoOutPath != "" {
			plan.Output = videoOutPath
		}
		if videoPlanPath == "" && videoOutPath != "" {
			videoPlanPath = strings.TrimSuffix(videoOutPath, filepath.Ext(videoOutPath)) + ".plan.json"
		}
		if videoSRTPath == "" && videoOutPath != "" {
			videoSRTPath = strings.TrimSuffix(videoOutPath, filepath.Ext(videoOutPath)) + ".srt"
		}
		if videoPlanPath != "" {
			if err := writeVideoPlan(videoPlanPath, plan); err != nil {
				return err
			}
		}
		if videoSRTPath != "" {
			if err := os.WriteFile(videoSRTPath, []byte(renderVideoSRT(plan)), 0o644); err != nil {
				return fmt.Errorf("write SRT failed: %w", err)
			}
		}
		if videoJSON || (videoPlanPath == "" && videoSRTPath == "") {
			return writeVideoPlanTo(cmd.OutOrStdout(), plan)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Video plan ready: %d sections, duration %s\n", len(plan.Sections), formatSRTDuration(plan.TotalDuration))
		if videoPlanPath != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plan: %s\n", videoPlanPath)
		}
		if videoSRTPath != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Subtitles: %s\n", videoSRTPath)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(videoCmd)
	videoCmd.AddCommand(videoGenerateCmd)
	videoGenerateCmd.Flags().StringVarP(&videoOutPath, "out", "o", "", "target mp4 path; also sets default .plan.json and .srt paths")
	videoGenerateCmd.Flags().StringVar(&videoPlanPath, "plan", "", "write production plan JSON to this path")
	videoGenerateCmd.Flags().StringVar(&videoSRTPath, "srt", "", "write subtitles as SRT to this path")
	videoGenerateCmd.Flags().IntVar(&videoWordsPerMin, "wpm", 150, "estimated narration speed used for subtitle timing")
	videoGenerateCmd.Flags().BoolVar(&videoJSON, "json", false, "print the production plan JSON to stdout")
}

type videoScriptMeta struct {
	Title      string  `json:"title,omitempty" yaml:"title"`
	Slides     string  `json:"slides" yaml:"slides"`
	Voice      string  `json:"voice,omitempty" yaml:"voice"`
	Width      int     `json:"width,omitempty" yaml:"width"`
	Height     int     `json:"height,omitempty" yaml:"height"`
	FPS        int     `json:"fps,omitempty" yaml:"fps"`
	Template   string  `json:"template,omitempty" yaml:"template"`
	Transition string  `json:"transition,omitempty" yaml:"transition"`
	Margin     float64 `json:"margin,omitempty" yaml:"margin"`
}

type videoPlan struct {
	Script        string              `json:"script"`
	Output        string              `json:"output,omitempty"`
	Meta          videoScriptMeta     `json:"meta"`
	Sections      []videoPlanSection  `json:"sections"`
	TotalDuration videoDurationMillis `json:"totalDurationMillis"`
}

type videoPlanSection struct {
	Index          int                 `json:"index"`
	Title          string              `json:"title"`
	Slide          int                 `json:"slide"`
	Narration      string              `json:"narration"`
	StartMillis    videoDurationMillis `json:"startMillis"`
	DurationMillis videoDurationMillis `json:"durationMillis"`
	EndMillis      videoDurationMillis `json:"endMillis"`
}

type videoDurationMillis int64

func buildVideoPlanFromFile(scriptPath string, wpm int) (videoPlan, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return videoPlan{}, fmt.Errorf("read video script failed: %w", err)
	}
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return videoPlan{}, fmt.Errorf("resolve script path failed: %w", err)
	}
	return parseVideoScript(abs, string(content), wpm)
}

func parseVideoScript(scriptPath, content string, wpm int) (videoPlan, error) {
	if wpm <= 0 {
		return videoPlan{}, fmt.Errorf("wpm must be positive")
	}
	meta, body, err := parseVideoScriptFrontmatter(content)
	if err != nil {
		return videoPlan{}, err
	}
	if strings.TrimSpace(meta.Slides) == "" {
		return videoPlan{}, fmt.Errorf("video script front matter must set slides")
	}
	if !filepath.IsAbs(meta.Slides) {
		meta.Slides = filepath.Clean(filepath.Join(filepath.Dir(scriptPath), meta.Slides))
	}
	if meta.FPS == 0 {
		meta.FPS = 30
	}
	if meta.Width == 0 {
		meta.Width = 1920
	}
	if meta.Height == 0 {
		meta.Height = 1080
	}
	sections, err := parseVideoScriptSections(body, wpm)
	if err != nil {
		return videoPlan{}, err
	}
	if len(sections) == 0 {
		return videoPlan{}, fmt.Errorf("video script must contain at least one section")
	}
	var cursor int64
	for i := range sections {
		sections[i].Index = i + 1
		sections[i].StartMillis = videoDurationMillis(cursor)
		cursor += int64(sections[i].DurationMillis)
		sections[i].EndMillis = videoDurationMillis(cursor)
	}
	return videoPlan{Script: scriptPath, Meta: meta, Sections: sections, TotalDuration: videoDurationMillis(cursor)}, nil
}

func parseVideoScriptFrontmatter(content string) (videoScriptMeta, string, error) {
	trimmed := strings.TrimLeft(content, "\ufeff\r\n\t ")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return videoScriptMeta{}, content, fmt.Errorf("video script must start with YAML front matter")
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
		return videoScriptMeta{}, "", fmt.Errorf("video script front matter is not closed")
	}
	frontLines := lines[1:end]
	body := trimmed[pos:]
	var meta videoScriptMeta
	if err := yaml.Unmarshal([]byte(strings.Join(frontLines, "")), &meta); err != nil {
		return videoScriptMeta{}, "", fmt.Errorf("parse video script front matter failed: %w", err)
	}
	return meta, body, nil
}

var videoHeadingRe = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

func parseVideoScriptSections(body string, wpm int) ([]videoPlanSection, error) {
	matches := videoHeadingRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("video script body must contain # headings for sections")
	}
	var sections []videoPlanSection
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

func parseVideoScriptSection(title, block string, wpm int) (videoPlanSection, error) {
	lines := strings.Split(block, "\n")
	slide := 0
	var narration []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "slide:") {
			value := strings.TrimSpace(trimmed[len("slide:"):])
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return videoPlanSection{}, fmt.Errorf("slide must be a positive integer")
			}
			slide = parsed
			continue
		}
		narration = append(narration, line)
	}
	if slide == 0 {
		return videoPlanSection{}, fmt.Errorf("missing slide: N mapping")
	}
	text := normalizeNarration(strings.Join(narration, "\n"))
	if text == "" {
		return videoPlanSection{}, fmt.Errorf("missing narration text")
	}
	duration := estimateNarrationDuration(text, wpm)
	return videoPlanSection{Title: title, Slide: slide, Narration: text, DurationMillis: videoDurationMillis(duration)}, nil
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

func writeVideoPlan(path string, plan videoPlan) error {
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

func writeVideoPlanTo(w io.Writer, plan videoPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal video plan failed: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func renderVideoSRT(plan videoPlan) string {
	sections := append([]videoPlanSection(nil), plan.Sections...)
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].Index < sections[j].Index })
	var b strings.Builder
	for i, section := range sections {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n", i+1, formatSRTTimestamp(section.StartMillis), formatSRTTimestamp(section.EndMillis), section.Narration)
		if i+1 < len(sections) {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatSRTTimestamp(ms videoDurationMillis) string {
	total := int64(ms)
	hours := total / 3_600_000
	total %= 3_600_000
	minutes := total / 60_000
	total %= 60_000
	seconds := total / 1000
	millis := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func formatSRTDuration(ms videoDurationMillis) string {
	d := time.Duration(ms) * time.Millisecond
	return d.Round(time.Millisecond).String()
}
