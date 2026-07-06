package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVideoScriptBuildsPlan(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "talk.md")
	content := `---
title: Demo
slides: ./deck.slide
voice: zh-CN-XiaoxiaoNeural
---

# Opening
slide: 1

大家好，今天介绍 tt slide video。

# Problem
slide: 2

Teams need repeatable product videos.
`
	plan, err := parseVideoScript(scriptPath, content, 120)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Meta.Title != "Demo" || plan.Meta.FPS != 30 || plan.Meta.Width != 1920 || plan.Meta.Height != 1080 {
		t.Fatalf("unexpected meta: %+v", plan.Meta)
	}
	if plan.Meta.Slides != filepath.Join(filepath.Dir(scriptPath), "deck.slide") {
		t.Fatalf("slides path = %q", plan.Meta.Slides)
	}
	if len(plan.Sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(plan.Sections))
	}
	if plan.Sections[0].Title != "Opening" || plan.Sections[0].Slide != 1 {
		t.Fatalf("first section = %+v", plan.Sections[0])
	}
	if plan.Sections[1].StartMillis != plan.Sections[0].EndMillis {
		t.Fatalf("sections are not contiguous: %+v %+v", plan.Sections[0], plan.Sections[1])
	}
	if plan.TotalDuration != plan.Sections[1].EndMillis {
		t.Fatalf("total duration = %d, want %d", plan.TotalDuration, plan.Sections[1].EndMillis)
	}
}

func TestParseVideoScriptRequiresSlideMapping(t *testing.T) {
	_, err := parseVideoScript("/tmp/talk.md", `---
slides: ./deck.slide
---

# Opening

No slide mapping.
`, 150)
	if err == nil || !strings.Contains(err.Error(), "missing slide") {
		t.Fatalf("err = %v, want missing slide", err)
	}
}

func TestRenderVideoSRT(t *testing.T) {
	plan := videoPlan{Sections: []videoPlanSection{{Index: 1, Narration: "hello", StartMillis: 0, EndMillis: 1500}, {Index: 2, Narration: "world", StartMillis: 1500, EndMillis: 3000}}}
	got := renderVideoSRT(plan)
	want := "1\n00:00:00,000 --> 00:00:01,500\nhello\n\n2\n00:00:01,500 --> 00:00:03,000\nworld\n"
	if got != want {
		t.Fatalf("SRT = %q, want %q", got, want)
	}
}

func TestVideoGenerateCommandWritesPlanAndSRT(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "talk.md")
	if err := os.WriteFile(script, []byte(`---
slides: ./deck.slide
---

# Opening
slide: 1

Hello video.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(tmp, "talk.plan.json")
	srtPath := filepath.Join(tmp, "talk.srt")
	plan, err := buildVideoPlanFromFile(script, 150)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeVideoPlan(planPath, plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srtPath, []byte(renderVideoSRT(plan)), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded videoPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Sections) != 1 || decoded.Sections[0].Narration != "Hello video." {
		t.Fatalf("decoded plan = %+v", decoded)
	}
	if got, err := os.ReadFile(srtPath); err != nil || !strings.Contains(string(got), "Hello video.") {
		t.Fatalf("srt = %q, err = %v", string(got), err)
	}
}
