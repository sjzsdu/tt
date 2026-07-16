package videocmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type videoReusePolicy struct {
	Enabled    bool
	Regenerate map[int]bool
}

func (p videoReusePolicy) shouldReuse(section int, path string) bool {
	if !p.Enabled || p.Regenerate[section] {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func parseVideoSectionSelection(value string, total int) (map[int]bool, error) {
	selected := map[int]bool{}
	value = strings.TrimSpace(value)
	if value == "" {
		return selected, nil
	}
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("invalid --sections %q: empty item", value)
		}
		parts := strings.Split(token, "-")
		if len(parts) > 2 {
			return nil, fmt.Errorf("invalid --sections item %q", token)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid --sections item %q", token)
		}
		end := start
		if len(parts) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid --sections item %q", token)
			}
		}
		if start < 1 || end < start || end > total {
			return nil, fmt.Errorf("section range %q is outside 1-%d", token, total)
		}
		for section := start; section <= end; section++ {
			selected[section] = true
		}
	}
	return selected, nil
}

func reuseVideoAudioCache(ctx context.Context, plan *Plan, audioDir string, policy videoReusePolicy) int {
	if plan == nil || !policy.Enabled {
		return 0
	}
	reused := 0
	for i := range plan.Sections {
		section := &plan.Sections[i]
		if policy.Regenerate[section.Index] {
			continue
		}
		path, err := findVideoAudioFile(audioDir, section.Index)
		if err != nil || !policy.shouldReuse(section.Index, path) {
			continue
		}
		section.Audio = path
		if duration := probeAudioDuration(ctx, path); duration > 0 {
			section.DurationMillis = DurationMillis(duration / time.Millisecond)
		}
		reused++
	}
	return reused
}

func allVideoSectionsHaveAudio(plan Plan) bool {
	if len(plan.Sections) == 0 {
		return false
	}
	for _, section := range plan.Sections {
		if strings.TrimSpace(section.Audio) == "" {
			return false
		}
	}
	return true
}

type videoPreview struct {
	Script        string          `json:"script"`
	Title         string          `json:"title,omitempty"`
	Slides        string          `json:"slides"`
	RenderMode    string          `json:"renderMode"`
	TotalDuration DurationMillis  `json:"totalDurationMillis"`
	Sections      []PlanSection   `json:"sections"`
	Quality       videoLintReport `json:"quality"`
}

func writeVideoPreview(w io.Writer, plan Plan, report videoLintReport, asJSON bool) error {
	preview := videoPreview{
		Script: plan.Script, Title: plan.Meta.Title, Slides: plan.Meta.Slides,
		RenderMode: report.RenderMode, TotalDuration: plan.TotalDuration,
		Sections: plan.Sections, Quality: report,
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(preview)
	}
	_, _ = fmt.Fprintf(w, "Video preview: %s\n", firstNonEmpty(plan.Meta.Title, videoArtifactBaseName(plan.Script)))
	_, _ = fmt.Fprintf(w, "Slides: %s\nSections: %d\nDuration: %s\nRender mode: %s\n\n", plan.Meta.Slides, len(plan.Sections), formatSRTDuration(plan.TotalDuration), report.RenderMode)
	_, _ = fmt.Fprintln(w, "Section\tSlide\tDuration\tTitle")
	sections := append([]PlanSection(nil), plan.Sections...)
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].Index < sections[j].Index })
	for _, section := range sections {
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", section.Index, section.Slide, formatSRTDuration(section.DurationMillis), section.Title)
	}
	_, _ = fmt.Fprintln(w, "\nQuality findings:")
	return writeVideoLintReport(w, report)
}
