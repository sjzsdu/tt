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
render_mode: browser
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
	if plan.Meta.RenderMode != "browser" {
		t.Fatalf("render mode = %q, want browser", plan.Meta.RenderMode)
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

func TestVideoRenderWorkerCountCapsConcurrency(t *testing.T) {
	if got := videoRenderWorkerCount(0); got != 1 {
		t.Fatalf("workers for empty = %d, want 1", got)
	}
	if got := videoRenderWorkerCount(3); got != 3 {
		t.Fatalf("workers for three = %d, want 3", got)
	}
	if got := videoRenderWorkerCount(99); got != 4 {
		t.Fatalf("workers for many = %d, want 4", got)
	}
}

func TestVideoSlidesLikelyAnimatedDetectsFragments(t *testing.T) {
	tmp := t.TempDir()
	deck := filepath.Join(tmp, "deck.slide")
	if err := os.WriteFile(deck, []byte("# Demo\n\n- One <!-- .element: class=\"fragment\" -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !videoSlidesLikelyAnimated(deck) {
		t.Fatalf("expected fragment deck to be treated as animated")
	}
	staticDeck := filepath.Join(tmp, "static.slide")
	if err := os.WriteFile(staticDeck, []byte("# Demo\n\nPlain slide.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if videoSlidesLikelyAnimated(staticDeck) {
		t.Fatalf("expected static deck to remain static")
	}
}

func TestShouldRefuseStaticFallbackForQualitySensitiveModes(t *testing.T) {
	if !shouldRefuseStaticFallback("browser", false) {
		t.Fatalf("explicit browser mode must not silently fall back")
	}
	if !shouldRefuseStaticFallback("auto", true) {
		t.Fatalf("animated auto mode must not fall back to static rendering")
	}
	if shouldRefuseStaticFallback("auto", false) {
		t.Fatalf("static auto mode may use static renderer")
	}
	if shouldRefuseStaticFallback("segments", false) {
		t.Fatalf("segments mode is already static")
	}
}

func TestLintVideoPlanFlagsRepetitiveGenericNarration(t *testing.T) {
	plan := Plan{Meta: ScriptMeta{Slides: filepath.Join(t.TempDir(), "missing.slide")}, Sections: []PlanSection{
		{Index: 1, Title: "A", Slide: 1, Narration: "我们可以看到这一页非常重要。核心在于这一页形成一个整体。", DurationMillis: 5000},
		{Index: 2, Title: "B", Slide: 2, Narration: "我们可以看到这一页非常重要。核心在于这一页形成一个整体。", DurationMillis: 5000},
	}}
	report := lintVideoPlan(plan, "auto")
	var hasGeneric, hasRepetition bool
	for _, issue := range report.Issues {
		if issue.Code == "generic-narration" {
			hasGeneric = true
		}
		if issue.Code == "repetitive-narration" {
			hasRepetition = true
		}
	}
	if !hasGeneric || !hasRepetition {
		t.Fatalf("issues = %+v, want generic and repetitive warnings", report.Issues)
	}
}

func TestLintVideoPlanWarnsAnimatedSegments(t *testing.T) {
	tmp := t.TempDir()
	deck := filepath.Join(tmp, "deck.slide")
	if err := os.WriteFile(deck, []byte("# Demo\n\n<span class=\"fragment\">Step</span>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Meta: ScriptMeta{Slides: deck}, Sections: []PlanSection{{Index: 1, Slide: 1, Narration: "这是一段足够长的讲稿，用于说明动画页面不应该被强制使用静态分段模式渲染。", DurationMillis: 8000}}}
	report := lintVideoPlan(plan, "segments")
	if !report.AnimatedSlides {
		t.Fatalf("expected animated slides")
	}
	for _, issue := range report.Issues {
		if issue.Code == "animated-segments" {
			return
		}
	}
	t.Fatalf("issues = %+v, want animated-segments warning", report.Issues)
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
	if !strings.Contains(filter, "xfade=transition=fade") {
		t.Fatalf("filter does not use video cross fades: %s", filter)
	}
	if strings.Contains(filter, "acrossfade") {
		t.Fatalf("filter must not overlap narration audio: %s", filter)
	}
	if !strings.Contains(filter, "concat=n=3:v=0:a=1") {
		t.Fatalf("filter must preserve audio order with concat: %s", filter)
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
	if !strings.Contains(script, "{at:0,slide:0}") {
		t.Fatalf("script missing first slide schedule: %s", script)
	}
	if !strings.Contains(script, "{at:2500,slide:2}") {
		t.Fatalf("script missing third slide schedule: %s", script)
	}
	if !strings.Contains(script, "window.__ttVideoSeek") || !strings.Contains(script, "capture.goTo(current.slide)") {
		t.Fatalf("script missing deterministic seek function: %s", script)
	}
	if !strings.Contains(script, "tt-video-subtitle-overlay") || !strings.Contains(script, "const subtitles = ") {
		t.Fatalf("script missing browser subtitle overlay: %s", script)
	}
}

func TestBrowserSubtitleCuesMirrorTimedSentenceCues(t *testing.T) {
	plan := &Plan{Sections: []PlanSection{{Narration: "第一句。第二句。", StartMillis: 0, EndMillis: 2000}}}
	cues := browserSubtitleCues(plan)
	if len(cues) != 2 {
		t.Fatalf("cues = %+v, want two sentence cues", cues)
	}
	if cues[0].Start != 0 || cues[1].End != 2000 || cues[0].Text != "第一句。" || cues[1].Text != "第二句。" {
		t.Fatalf("unexpected cues: %+v", cues)
	}
}

func TestBrowserTimelineScriptClampsNegativeStart(t *testing.T) {
	plan := &Plan{Sections: []PlanSection{{Slide: 2, StartMillis: -50}}}
	script := browserTimelineScript(plan)
	if !strings.Contains(script, "{at:0,slide:1}") {
		t.Fatalf("script did not clamp negative start: %s", script)
	}
}

func TestBrowserRecordingTimeoutAllowsSlowerThanRealtimeCapture(t *testing.T) {
	plan := &Plan{Meta: ScriptMeta{FPS: 24}, TotalDuration: 90_000}
	got := browserRecordingTimeout(plan)
	if got < 18*time.Minute {
		t.Fatalf("timeout = %s, want enough room for slow frame capture", got)
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

func TestVideoProgressClearsTerminalLineOnEachStep(t *testing.T) {
	var buf bytes.Buffer
	progress := &videoProgress{out: &buf, start: time.Now(), terminal: true}
	progress.Step("这是一个非常长的进度提示")
	progress.Step("短提示")
	got := buf.String()
	if strings.Count(got, "\033[K") != 2 {
		t.Fatalf("terminal progress should clear each step, got %q", got)
	}
	if !strings.Contains(got, "\r\033[K短提示") {
		t.Fatalf("short terminal step did not clear line first: %q", got)
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

func TestParseVideoSectionSelection(t *testing.T) {
	selected, err := parseVideoSectionSelection("2,4-6", 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{2, 4, 5, 6} {
		if !selected[index] {
			t.Fatalf("section %d was not selected: %#v", index, selected)
		}
	}
	if selected[1] || selected[3] {
		t.Fatalf("unexpected section selected: %#v", selected)
	}
	if _, err := parseVideoSectionSelection("3-7", 6); err == nil {
		t.Fatal("out-of-range selection should fail")
	}
}

func TestVideoReusePolicyReusesOnlyHealthyUnselectedArtifacts(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "segment-001.mp4")
	empty := filepath.Join(dir, "segment-002.mp4")
	if err := os.WriteFile(ready, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	policy := videoReusePolicy{Enabled: true, Regenerate: map[int]bool{3: true}}
	if !policy.shouldReuse(1, ready) {
		t.Fatal("healthy cached artifact should be reused")
	}
	if policy.shouldReuse(2, empty) {
		t.Fatal("empty cached artifact must not be reused")
	}
	if policy.shouldReuse(3, ready) {
		t.Fatal("selected section must be regenerated")
	}
}

func TestApplyVideoTTSProviderKeepsResumedAudio(t *testing.T) {
	plan := Plan{Sections: []PlanSection{
		{Index: 1, Title: "cached", Audio: "audio/001.wav", DurationMillis: 1200},
		{Index: 2, Title: "new", DurationMillis: 1500},
	}}
	if err := applyVideoTTSProvider(context.Background(), fixedVideoTTSProvider{}, &plan, nil); err != nil {
		t.Fatal(err)
	}
	if plan.Sections[0].Audio != "audio/001.wav" || plan.Sections[0].DurationMillis != 1200 {
		t.Fatalf("cached audio was replaced: %+v", plan.Sections[0])
	}
	if plan.Sections[1].Audio != filepath.Join("audio", "new.wav") {
		t.Fatalf("uncached audio was not synthesized: %+v", plan.Sections[1])
	}
	if plan.Sections[1].StartMillis != 1200 || plan.TotalDuration != 3200 {
		t.Fatalf("resumed timing was not recomputed: %+v total=%d", plan.Sections, plan.TotalDuration)
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
