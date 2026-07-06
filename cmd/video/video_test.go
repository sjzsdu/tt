package videocmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	applyVideoConfigDefaults(&plan, videoCommandConfig{Width: 1920, Height: 1080, FPS: 30})
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
	plan := Plan{Sections: []PlanSection{{Index: 1, Narration: "hello", StartMillis: 0, EndMillis: 1500}, {Index: 2, Narration: "world", StartMillis: 1500, EndMillis: 3000}}}
	got := renderVideoSRT(plan)
	want := "1\n00:00:00,000 --> 00:00:01,500\nhello\n\n2\n00:00:01,500 --> 00:00:03,000\nworld\n"
	if got != want {
		t.Fatalf("SRT = %q, want %q", got, want)
	}
}

func TestRenderVideoSRTSplitsSectionIntoTimedSentenceCues(t *testing.T) {
	plan := Plan{Sections: []PlanSection{{Index: 1, Narration: "第一句。第二句更长一点。", StartMillis: 0, EndMillis: 3000}}}
	got := renderVideoSRT(plan)
	if !strings.Contains(got, "1\n00:00:00,000 -->") || !strings.Contains(got, "第一句。") {
		t.Fatalf("first cue missing: %q", got)
	}
	if !strings.Contains(got, "2\n") || !strings.Contains(got, "第二句更长一点。") {
		t.Fatalf("second cue missing: %q", got)
	}
	if strings.Contains(got, "第一句。第二句更长一点。") {
		t.Fatalf("section was not split into sentence cues: %q", got)
	}
}

func TestSplitVideoSubtitleTextKeepsDottedWordsTogether(t *testing.T) {
	parts := splitVideoSubtitleText(".slide 文件只描述内容和结构。模板负责最终视觉呈现。")
	if len(parts) != 2 {
		t.Fatalf("parts = %#v, want 2 cues", parts)
	}
	if !strings.Contains(parts[0], ".slide 文件") {
		t.Fatalf("dotted word was split incorrectly: %#v", parts)
	}
}

func TestVideoScalePadFilterPreservesFullSlideCanvas(t *testing.T) {
	got := videoScalePadFilter(1280, 720)
	want := "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2"
	if got != want {
		t.Fatalf("filter = %q, want %q", got, want)
	}
}

func TestVideoScalePadFilterNormalizesInvalidDimensions(t *testing.T) {
	got := videoScalePadFilter(1279, 0)
	want := "scale=1278:1080:force_original_aspect_ratio=decrease,pad=1278:1080:(ow-iw)/2:(oh-ih)/2"
	if got != want {
		t.Fatalf("filter = %q, want %q", got, want)
	}
}

func TestVideoTimelineFilterUsesCrossFades(t *testing.T) {
	plan := &Plan{Meta: ScriptMeta{FPS: 30}, Sections: []PlanSection{
		{Index: 1, DurationMillis: 2000},
		{Index: 2, DurationMillis: 3000},
		{Index: 3, DurationMillis: 4000},
	}}
	transition := videoTimelineTransitionSeconds(plan)
	if transition <= 0 {
		t.Fatalf("transition = %f, want positive", transition)
	}
	filter := videoTimelineFilter(plan, transition)
	if !strings.Contains(filter, "xfade=transition=fade") || !strings.Contains(filter, "acrossfade") {
		t.Fatalf("filter does not use cross fades: %s", filter)
	}
	if !strings.Contains(filter, "[v2]") || !strings.Contains(filter, "[a2]") {
		t.Fatalf("filter missing final labels: %s", filter)
	}
}

func TestVideoTimelineTransitionDisabledForVeryShortSections(t *testing.T) {
	plan := &Plan{Sections: []PlanSection{{DurationMillis: 250}, {DurationMillis: 250}}}
	if got := videoTimelineTransitionSeconds(plan); got != 0 {
		t.Fatalf("transition = %f, want 0", got)
	}
}

func TestBrowserTimelineScriptSchedulesSlidesBySectionStart(t *testing.T) {
	plan := &Plan{Sections: []PlanSection{
		{Slide: 1, StartMillis: 0},
		{Slide: 3, StartMillis: 2500},
	}}
	script := browserTimelineScript(plan)
	if !strings.Contains(script, "deck.slide(0, 0, 0), 0") {
		t.Fatalf("script missing first slide schedule: %s", script)
	}
	if !strings.Contains(script, "deck.slide(2, 0, 0), 2500") {
		t.Fatalf("script missing third slide schedule: %s", script)
	}
}

func TestBrowserTimelineScriptClampsNegativeStart(t *testing.T) {
	plan := &Plan{Sections: []PlanSection{{Slide: 2, StartMillis: -50}}}
	script := browserTimelineScript(plan)
	if !strings.Contains(script, "deck.slide(1, 0, 0), 0") {
		t.Fatalf("script did not clamp negative start: %s", script)
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
	var decoded Plan
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

func TestVideoAudioDirTTSProviderFindsSectionAudio(t *testing.T) {
	tmp := t.TempDir()
	audioPath := filepath.Join(tmp, "001.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := newVideoTTSProvider(videoTTSOptions{Mode: "audio-dir", AudioDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Synthesize(context.Background(), videoTTSRequest{Index: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.AudioPath != audioPath {
		t.Fatalf("audio path = %q, want %q", result.AudioPath, audioPath)
	}
}

func TestVideoCommandTTSProviderCreatesAudio(t *testing.T) {
	tmp := t.TempDir()
	provider, err := newVideoTTSProvider(videoTTSOptions{
		Mode:    "command",
		WorkDir: tmp,
		Command: "printf '%s' '{{.Text}}' > '{{.Output}}'",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Synthesize(context.Background(), videoTTSRequest{Index: 2, Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result.AudioPath != filepath.Join(tmp, "002.wav") {
		t.Fatalf("audio path = %q", result.AudioPath)
	}
	data, err := os.ReadFile(result.AudioPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("audio content = %q", string(data))
	}
}

func TestVideoBailianTTSProviderDownloadsAudio(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TEST_DASHSCOPE_API_KEY", "test-key")
	var sawGeneration bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			sawGeneration = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("authorization = %q", got)
			}
			var payload videoBailianTTSRequestPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Model != "qwen3-tts-flash" || payload.Input["text"] != "你好" || payload.Input["voice"] != "Cherry" {
				t.Fatalf("payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{"status_code":200,"output":{"audio":{"url":"` + serverURL(r) + `/audio.wav"}}}`))
		case "/audio.wav":
			_, _ = w.Write([]byte("audio"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := newVideoTTSProvider(videoTTSOptions{
		Mode:             "bailian",
		WorkDir:          tmp,
		BailianAPIKeyEnv: "TEST_DASHSCOPE_API_KEY",
		BailianBaseURL:   server.URL + "/api/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Synthesize(context.Background(), videoTTSRequest{Index: 1, Text: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	if !sawGeneration {
		t.Fatal("generation endpoint was not called")
	}
	data, err := os.ReadFile(result.AudioPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "audio" {
		t.Fatalf("audio = %q", string(data))
	}
}

func TestVideoBailianTTSProviderRequiresAPIKey(t *testing.T) {
	_, err := newVideoTTSProvider(videoTTSOptions{Mode: "bailian", BailianAPIKeyEnv: "MISSING_TEST_DASHSCOPE_API_KEY"})
	if err == nil || !strings.Contains(err.Error(), "MISSING_TEST_DASHSCOPE_API_KEY") {
		t.Fatalf("err = %v, want missing env", err)
	}
}

func TestVideoProgressWritesReadableNonTerminalSteps(t *testing.T) {
	var buf bytes.Buffer
	progress := &videoProgress{out: &buf, start: time.Now()}
	progress.Step("正在截图第 %d/%d 页 slide", 1, 3)
	progress.Done("视频生成完成")
	got := buf.String()
	if !strings.Contains(got, "video: 正在截图第 1/3 页 slide") || !strings.Contains(got, "video: 视频生成完成") {
		t.Fatalf("progress output = %q", got)
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type fixedVideoTTSProvider struct{}

func (fixedVideoTTSProvider) Synthesize(_ context.Context, req videoTTSRequest) (videoTTSResult, error) {
	return videoTTSResult{AudioPath: filepath.Join("audio", req.Title+".wav"), Duration: time.Duration(req.Index) * time.Second}, nil
}

func TestApplyVideoTTSProviderUpdatesAudioAndTiming(t *testing.T) {
	plan := Plan{Sections: []PlanSection{
		{Index: 1, Title: "a", DurationMillis: 1500},
		{Index: 2, Title: "b", DurationMillis: 1500},
	}}
	if err := applyVideoTTSProvider(context.Background(), fixedVideoTTSProvider{}, &plan, nil); err != nil {
		t.Fatal(err)
	}
	if plan.Sections[0].Audio != filepath.Join("audio", "a.wav") || plan.Sections[1].Audio != filepath.Join("audio", "b.wav") {
		t.Fatalf("audio not applied: %+v", plan.Sections)
	}
	if plan.Sections[0].EndMillis != 1000 || plan.Sections[1].StartMillis != 1000 || plan.TotalDuration != 3000 {
		t.Fatalf("timing not recomputed: %+v total=%d", plan.Sections, plan.TotalDuration)
	}
}

func TestRunVideoDoctorValidatesScriptAndTTS(t *testing.T) {
	tmp := t.TempDir()
	slide := filepath.Join(tmp, "deck.slide")
	if err := os.WriteFile(slide, []byte("# Deck\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(tmp, "talk.md")
	if err := os.WriteFile(script, []byte(`---
slides: ./deck.slide
---

# Opening
slide: 1

Hello.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	report := runVideoDoctor(videoDoctorOptions{Script: script, TTSMode: "none"})
	if len(report.Checks) == 0 {
		t.Fatal("doctor returned no checks")
	}
	var sawScript, sawSlides, sawTTS bool
	for _, check := range report.Checks {
		switch check.Name {
		case "script":
			sawScript = check.OK
		case "slides":
			sawSlides = check.OK
		case "tts":
			sawTTS = check.OK
		}
	}
	if !sawScript || !sawSlides || !sawTTS {
		t.Fatalf("doctor did not validate script/slides/tts: %+v", report.Checks)
	}
}

func TestRunVideoDoctorReportsBadTTSConfig(t *testing.T) {
	report := runVideoDoctor(videoDoctorOptions{TTSMode: "audio-dir"})
	for _, check := range report.Checks {
		if check.Name == "tts" {
			if check.OK || !strings.Contains(check.Detail, "--audio-dir") {
				t.Fatalf("tts check = %+v", check)
			}
			return
		}
	}
	t.Fatal("missing tts check")
}
