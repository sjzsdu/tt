package videocmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type videoLintSeverity string

const (
	videoLintInfo    videoLintSeverity = "info"
	videoLintWarning videoLintSeverity = "warning"
	videoLintError   videoLintSeverity = "error"
)

type videoLintIssue struct {
	Severity videoLintSeverity `json:"severity"`
	Section  int               `json:"section,omitempty"`
	Slide    int               `json:"slide,omitempty"`
	Title    string            `json:"title,omitempty"`
	Code     string            `json:"code"`
	Message  string            `json:"message"`
}

type videoLintReport struct {
	Script         string           `json:"script"`
	Slides         string           `json:"slides"`
	Sections       int              `json:"sections"`
	Duration       DurationMillis   `json:"durationMillis"`
	RenderMode     string           `json:"renderMode"`
	AnimatedSlides bool             `json:"animatedSlides"`
	SectionTitles  map[int]string   `json:"-"`
	Issues         []videoLintIssue `json:"issues"`
}

func lintVideoPlan(plan Plan, renderMode string) videoLintReport {
	mode := strings.ToLower(strings.TrimSpace(renderMode))
	if mode == "" {
		mode = "auto"
	}
	report := videoLintReport{
		Script:         plan.Script,
		Slides:         plan.Meta.Slides,
		Sections:       len(plan.Sections),
		Duration:       plan.TotalDuration,
		RenderMode:     mode,
		AnimatedSlides: videoSlidesLikelyAnimated(plan.Meta.Slides),
		SectionTitles:  map[int]string{},
	}
	for _, section := range plan.Sections {
		report.SectionTitles[section.Index] = section.Title
	}
	if len(plan.Sections) == 0 {
		report.add(videoLintError, 0, 0, "empty-script", "讲稿没有任何 section，无法生成有发布价值的视频。")
		return report
	}
	if report.AnimatedSlides && mode == "segments" {
		report.add(videoLintWarning, 0, 0, "animated-segments", "slide 可能包含动画或 fragment，但 render_mode=segments 会丢失连续动画。建议使用 auto 或 browser。")
	}
	if report.AnimatedSlides && mode == "auto" {
		report.add(videoLintInfo, 0, 0, "auto-browser", "检测到动画线索，auto 会优先使用浏览器连续录制以保障画面质量。")
	}
	lintVideoPacing(&report, plan)
	lintVideoNarrationSpecificity(&report, plan)
	lintVideoNarrationRepetition(&report, plan)
	sort.SliceStable(report.Issues, func(i, j int) bool {
		if report.Issues[i].Severity != report.Issues[j].Severity {
			return severityRank(report.Issues[i].Severity) > severityRank(report.Issues[j].Severity)
		}
		return report.Issues[i].Section < report.Issues[j].Section
	})
	return report
}

func (r *videoLintReport) add(severity videoLintSeverity, section, slide int, code, message string) {
	issue := videoLintIssue{Severity: severity, Section: section, Slide: slide, Code: code, Message: message}
	if r.SectionTitles != nil {
		issue.Title = r.SectionTitles[section]
	}
	r.Issues = append(r.Issues, issue)
}

func (r videoLintReport) hasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == videoLintError {
			return true
		}
	}
	return false
}

func (r videoLintReport) warningCount() int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == videoLintWarning {
			count++
		}
	}
	return count
}

func lintVideoPacing(report *videoLintReport, plan Plan) {
	var total int64
	seenSlides := map[int]int{}
	for _, section := range plan.Sections {
		seconds := float64(section.DurationMillis) / 1000
		total += int64(section.DurationMillis)
		seenSlides[section.Slide]++
		if seconds < 4 {
			report.add(videoLintWarning, section.Index, section.Slide, "too-short", "这一页讲解少于 4 秒，观众可能来不及理解画面。建议合并或补充具体解释。")
		}
		if seconds > 90 {
			report.add(videoLintWarning, section.Index, section.Slide, "too-long", "这一页讲解超过 90 秒，节奏容易拖沓。建议拆页或增加画面变化。")
		}
	}
	avg := float64(total) / float64(len(plan.Sections)) / 1000
	if avg > 60 {
		report.add(videoLintWarning, 0, 0, "slow-average", "平均每页超过 60 秒，长视频容易显得平。建议增加章节切换或拆分视频。")
	}
	for slide, count := range seenSlides {
		if count >= 3 {
			report.add(videoLintInfo, 0, slide, "reused-slide", fmt.Sprintf("slide %d 被连续或重复讲解 %d 次。若是长讲解，建议拆成多个视觉状态。", slide, count))
		}
	}
}

func lintVideoNarrationSpecificity(report *videoLintReport, plan Plan) {
	for _, section := range plan.Sections {
		text := section.Narration
		units := countNarrationUnits(text)
		if units < 35 {
			report.add(videoLintWarning, section.Index, section.Slide, "thin-narration", "讲稿偏短，容易像提示词而不是正式口播。建议补充例子、判断标准或结论。")
		}
		generic := genericPhraseScore(text)
		if generic >= 3 {
			report.add(videoLintWarning, section.Index, section.Slide, "generic-narration", "讲稿中泛化表达偏多，容易出现模板感。建议替换成这一页独有的信息。")
		}
		if punctuationDensity(text) < 0.012 && units > 80 {
			report.add(videoLintInfo, section.Index, section.Slide, "few-pauses", "长段讲稿停顿偏少，TTS 语气可能生硬。建议增加逗号、句号或分段。")
		}
	}
}

func lintVideoNarrationRepetition(report *videoLintReport, plan Plan) {
	for i := 0; i < len(plan.Sections); i++ {
		for j := i + 1; j < len(plan.Sections); j++ {
			similarity := narrationJaccard(plan.Sections[i].Narration, plan.Sections[j].Narration)
			if similarity >= 0.72 {
				report.add(videoLintWarning, plan.Sections[j].Index, plan.Sections[j].Slide, "repetitive-narration", fmt.Sprintf("与第 %d 段讲稿相似度 %.0f%%，容易显得重复。建议补充该页独有内容。", plan.Sections[i].Index, similarity*100))
			}
		}
	}
}

func writeVideoLintReport(w io.Writer, report videoLintReport) error {
	status := "PASS"
	for _, issue := range report.Issues {
		if issue.Severity == videoLintWarning {
			status = "WARN"
		}
		if issue.Severity == videoLintError {
			status = "FAIL"
			break
		}
	}
	_, _ = fmt.Fprintf(w, "Video lint: %s\n", status)
	_, _ = fmt.Fprintf(w, "Script: %s\n", report.Script)
	_, _ = fmt.Fprintf(w, "Sections: %d, duration: %s, render_mode: %s\n", report.Sections, formatSRTDuration(report.Duration), report.RenderMode)
	if report.AnimatedSlides {
		_, _ = fmt.Fprintln(w, "Slides: animation cues detected")
	}
	if len(report.Issues) == 0 {
		_, _ = fmt.Fprintln(w, "No quality issues found.")
		return nil
	}
	for _, issue := range report.Issues {
		loc := "global"
		if issue.Section > 0 {
			loc = fmt.Sprintf("section %d", issue.Section)
			if issue.Slide > 0 {
				loc += fmt.Sprintf(" / slide %d", issue.Slide)
			}
		} else if issue.Slide > 0 {
			loc = fmt.Sprintf("slide %d", issue.Slide)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", issue.Severity, loc, issue.Code, issue.Message)
	}
	return nil
}

func severityRank(severity videoLintSeverity) int {
	switch severity {
	case videoLintError:
		return 3
	case videoLintWarning:
		return 2
	default:
		return 1
	}
}

var videoGenericPhrases = []string{
	"我们可以看到", "可以看出", "非常重要", "核心在于", "这一页", "接下来", "整体来看", "简单来说", "从这个角度", "形成一个",
}

func genericPhraseScore(text string) int {
	score := 0
	for _, phrase := range videoGenericPhrases {
		score += strings.Count(text, phrase)
	}
	return score
}

func punctuationDensity(text string) float64 {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	count := 0
	for _, r := range runes {
		switch r {
		case '，', '。', '；', '：', '、', '！', '？', ',', '.', ';', ':', '!', '?':
			count++
		}
	}
	return float64(count) / float64(len(runes))
}

func narrationJaccard(a, b string) float64 {
	left := narrationTokenSet(a)
	right := narrationTokenSet(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	for token := range left {
		if right[token] {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

var asciiTokenRe = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._-]*`)

func narrationTokenSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, token := range asciiTokenRe.FindAllString(strings.ToLower(text), -1) {
		if len(token) >= 2 {
			set[token] = true
		}
	}
	var chars []rune
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			chars = append(chars, r)
		}
	}
	for i := 0; i+1 < len(chars); i++ {
		set[string(chars[i:i+2])] = true
	}
	return set
}

func videoSlidesLikelyAnimated(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(data))
	markers := []string{"fragment", "data-auto-animate", "auto-animate", "animate__", "data-fragment-index", "<!-- .element:"}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
