package videocmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type videoRenderOptions struct {
	WorkDir  string
	SRTPath  string
	Progress *videoProgress
	Mode     string
}

func defaultVideoArtifactDir(scriptPath, root string) string {
	if strings.TrimSpace(root) == "" {
		root = "videos"
	}
	return filepath.Join(root, safeVideoArtifactName(videoArtifactBaseName(scriptPath)))
}

func defaultVideoInternalDir(scriptPath, root string) string {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(".tt", "video")
	}
	return filepath.Join(root, safeVideoArtifactName(videoArtifactBaseName(scriptPath)))
}

func videoArtifactBaseName(scriptPath string) string {
	base := strings.TrimSuffix(filepath.Base(scriptPath), filepath.Ext(scriptPath))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "video"
	}
	return base
}

func safeVideoArtifactName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x4e00 && r <= 0x9fff {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "video"
	}
	return result
}

func renderVideoPlan(ctx context.Context, plan *Plan, opts videoRenderOptions) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is required to render video: %w", err)
	}
	workDir := opts.WorkDir
	if strings.TrimSpace(workDir) == "" {
		var err error
		workDir, err = os.MkdirTemp("", "tt-video-render-*")
		if err != nil {
			return fmt.Errorf("create video workdir failed: %w", err)
		}
	} else if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create video workdir failed: %w", err)
	}
	stop, baseURL, err := startSlideServer(plan)
	if err != nil {
		return err
	}
	defer stop()

	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "auto"
	}
	originalMode := mode
	if mode == "auto" && videoSlidesLikelyAnimated(plan.Meta.Slides) {
		opts.Progress.Step("auto 检测到 slide 动画，切换为浏览器连续录制以保障质量")
		mode = "browser"
	}
	if mode == "browser" {
		opts.Progress.Step("正在尝试浏览器连续播放录制")
		if err := renderVideoPlanBrowserContinuous(ctx, baseURL, plan, workDir, opts); err == nil {
			return nil
		} else {
			if shouldRefuseStaticFallback(originalMode, videoSlidesLikelyAnimated(plan.Meta.Slides)) {
				return fmt.Errorf("browser continuous rendering failed; refusing static fallback because quality would degrade: %w", err)
			}
			opts.Progress.Step("浏览器连续录制失败，回退稳定合成: %v", err)
		}
	}

	if mode == "auto" {
		opts.Progress.Step("正在使用 auto 模式并发渲染静态视频片段")
	} else {
		opts.Progress.Step("正在启动 slide 捕获服务")
	}
	if err := captureVideoSlidesConcurrent(ctx, baseURL, plan, workDir, opts.Progress); err != nil {
		return err
	}
	segments, err := renderVideoSegmentsConcurrent(ctx, plan, workDir, opts.Progress)
	if err != nil {
		return err
	}
	mergedPath := filepath.Join(workDir, "merged.mp4")
	transition := videoTimelineTransitionSeconds(plan)
	if transition > 0 {
		opts.Progress.Step("正在用时间轴淡入淡出合成 %d 个视频片段", len(segments))
	} else {
		opts.Progress.Step("正在稳定合并 %d 个视频片段", len(segments))
	}
	if err := mergeVideoSegmentsTimeline(ctx, plan, segments, mergedPath); err != nil {
		return fmt.Errorf("merge video segments failed: %w", err)
	}
	if opts.SRTPath != "" {
		opts.Progress.Step("正在写入字幕文件")
		if err := os.WriteFile(opts.SRTPath, []byte(renderVideoSRT(*plan)), 0o644); err != nil {
			return fmt.Errorf("write SRT failed: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(plan.Output), 0o755); err != nil && filepath.Dir(plan.Output) != "." {
		return fmt.Errorf("create output directory failed: %w", err)
	}
	return os.Rename(mergedPath, plan.Output)
}

func shouldRefuseStaticFallback(originalMode string, animatedSlides bool) bool {
	return strings.EqualFold(strings.TrimSpace(originalMode), "browser") || animatedSlides
}

func captureVideoSlidesConcurrent(ctx context.Context, baseURL string, plan *Plan, workDir string, progress *videoProgress) error {
	workers := videoRenderWorkerCount(len(plan.Sections))
	jobs := make(chan PlanSection)
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chromePath := findVideoChromePath()
			opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", true), chromedp.Flag("disable-gpu", true), chromedp.Flag("no-sandbox", true), chromedp.WindowSize(plan.Meta.Width, plan.Meta.Height))
			if chromePath != "" {
				opts = append(opts, chromedp.ExecPath(chromePath))
			}
			allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
			defer cancelAlloc()
			browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(nil))
			defer cancelBrowser()
			for section := range jobs {
				progress.Step("正在并发截图第 %d/%d 页 slide", section.Index, len(plan.Sections))
				if err := captureVideoSlide(ctx, browserCtx, baseURL, plan, workDir, section); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, section := range plan.Sections {
			select {
			case <-ctx.Done():
				return
			case jobs <- section:
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func captureVideoSlide(ctx context.Context, browserCtx context.Context, baseURL string, plan *Plan, workDir string, section PlanSection) error {
	shotPath := filepath.Join(workDir, fmt.Sprintf("slide-%03d.png", section.Index))
	url := buildSlideURL(baseURL, plan, section.Slide)
	var buf []byte
	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()
	ctxTimeout, cancel := context.WithTimeout(tabCtx, 45*time.Second)
	defer cancel()
	if err := chromedp.Run(ctxTimeout,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.WaitReady(".reveal .slides", chromedp.ByQuery),
		chromedp.WaitVisible(".reveal .slides section", chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf(`window.ttSlideCapture && window.ttSlideCapture.goTo(%d)`, section.Slide-1), nil),
		chromedp.WaitReady(".reveal .slides section.present", chromedp.ByQuery),
		chromedp.Sleep(700*time.Millisecond),
		chromedp.CaptureScreenshot(&buf),
	); err != nil {
		return fmt.Errorf("capture slide %d failed: %w", section.Slide, err)
	}
	if err := os.WriteFile(shotPath, buf, 0o644); err != nil {
		return fmt.Errorf("write slide screenshot failed: %w", err)
	}
	return nil
}

func videoRenderWorkerCount(n int) int {
	if n <= 1 {
		return 1
	}
	if n < 4 {
		return n
	}
	return 4
}

func captureVideoSlides(ctx context.Context, baseURL string, plan *Plan, workDir string, progress *videoProgress) error {
	chromePath := findVideoChromePath()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", true), chromedp.Flag("disable-gpu", true), chromedp.Flag("no-sandbox", true), chromedp.WindowSize(plan.Meta.Width, plan.Meta.Height))
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()
	browserCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(nil))
	defer cancel()
	for _, section := range plan.Sections {
		progress.Step("正在截图第 %d/%d 页 slide", section.Index, len(plan.Sections))
		shotPath := filepath.Join(workDir, fmt.Sprintf("slide-%03d.png", section.Index))
		url := buildSlideURL(baseURL, plan, section.Slide)
		var buf []byte
		tabCtx, tabCancel := chromedp.NewContext(browserCtx)
		ctxTimeout, cancel := context.WithTimeout(tabCtx, 45*time.Second)
		err := chromedp.Run(ctxTimeout,
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.WaitReady(".reveal .slides", chromedp.ByQuery),
			chromedp.WaitVisible(".reveal .slides section", chromedp.ByQuery),
			chromedp.Evaluate(fmt.Sprintf(`window.ttSlideCapture && window.ttSlideCapture.goTo(%d)`, section.Slide-1), nil),
			chromedp.WaitReady(".reveal .slides section.present", chromedp.ByQuery),
			chromedp.Sleep(700*time.Millisecond),
			chromedp.CaptureScreenshot(&buf),
		)
		cancel()
		tabCancel()
		if err != nil {
			return fmt.Errorf("capture slide %d failed: %w", section.Slide, err)
		}
		if err := os.WriteFile(shotPath, buf, 0o644); err != nil {
			return fmt.Errorf("write slide screenshot failed: %w", err)
		}
	}
	return nil
}

func renderVideoSegments(ctx context.Context, plan *Plan, workDir string, progress *videoProgress) ([]string, error) {
	var segments []string
	for _, section := range plan.Sections {
		progress.Step("正在渲染第 %d/%d 个视频片段", section.Index, len(plan.Sections))
		imagePath := filepath.Join(workDir, fmt.Sprintf("slide-%03d.png", section.Index))
		audioPath := section.Audio
		if audioPath == "" {
			audioPath = filepath.Join(workDir, fmt.Sprintf("silent-%03d.wav", section.Index))
			if err := runCommand(ctx, "ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100", "-t", secondsArg(section.DurationMillis), audioPath); err != nil {
				return nil, fmt.Errorf("create silent audio failed: %w", err)
			}
		}
		segmentPath := filepath.Join(workDir, fmt.Sprintf("segment-%03d.mp4", section.Index))
		if err := runCommand(ctx, "ffmpeg", "-y", "-loop", "1", "-i", imagePath, "-i", audioPath, "-t", secondsArg(section.DurationMillis), "-r", strconv.Itoa(plan.Meta.FPS), "-vf", videoScalePadFilter(plan.Meta.Width, plan.Meta.Height), "-pix_fmt", "yuv420p", "-c:v", "libx264", "-c:a", "aac", "-shortest", segmentPath); err != nil {
			return nil, fmt.Errorf("render segment %d failed: %w", section.Index, err)
		}
		segments = append(segments, segmentPath)
	}
	return segments, nil
}

func renderVideoSegmentsConcurrent(ctx context.Context, plan *Plan, workDir string, progress *videoProgress) ([]string, error) {
	segments := make([]string, len(plan.Sections))
	workers := videoRenderWorkerCount(len(plan.Sections))
	jobs := make(chan PlanSection)
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for section := range jobs {
				progress.Step("正在并发渲染第 %d/%d 个视频片段", section.Index, len(plan.Sections))
				segment, err := renderVideoSegment(ctx, plan, workDir, section)
				if err != nil {
					errCh <- err
					return
				}
				segments[section.Index-1] = segment
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, section := range plan.Sections {
			select {
			case <-ctx.Done():
				return
			case jobs <- section:
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return segments, nil
}

func renderVideoSegment(ctx context.Context, plan *Plan, workDir string, section PlanSection) (string, error) {
	imagePath := filepath.Join(workDir, fmt.Sprintf("slide-%03d.png", section.Index))
	audioPath := section.Audio
	if audioPath == "" {
		audioPath = filepath.Join(workDir, fmt.Sprintf("silent-%03d.wav", section.Index))
		if err := runCommand(ctx, "ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100", "-t", secondsArg(section.DurationMillis), audioPath); err != nil {
			return "", fmt.Errorf("create silent audio failed: %w", err)
		}
	}
	segmentPath := filepath.Join(workDir, fmt.Sprintf("segment-%03d.mp4", section.Index))
	if err := runCommand(ctx, "ffmpeg", "-y", "-loop", "1", "-i", imagePath, "-i", audioPath, "-t", secondsArg(section.DurationMillis), "-r", strconv.Itoa(plan.Meta.FPS), "-vf", videoScalePadFilter(plan.Meta.Width, plan.Meta.Height), "-pix_fmt", "yuv420p", "-c:v", "libx264", "-c:a", "aac", "-shortest", segmentPath); err != nil {
		return "", fmt.Errorf("render segment %d failed: %w", section.Index, err)
	}
	return segmentPath, nil
}

func mergeVideoSegmentsTimeline(ctx context.Context, plan *Plan, segments []string, output string) error {
	if len(segments) == 0 {
		return fmt.Errorf("no video segments to merge")
	}
	if len(segments) == 1 {
		return runCommand(ctx, "ffmpeg", "-y", "-i", segments[0], "-c", "copy", output)
	}
	transition := videoTimelineTransitionSeconds(plan)
	if transition <= 0 {
		concatPath := filepath.Join(filepath.Dir(output), "concat.txt")
		if err := writeVideoConcatFile(concatPath, segments); err != nil {
			return err
		}
		return runCommand(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-c", "copy", output)
	}
	args := []string{"-y"}
	for _, segment := range segments {
		args = append(args, "-i", segment)
	}
	filter := videoTimelineFilter(plan, transition)
	last := len(segments) - 1
	args = append(args,
		"-filter_complex", filter,
		"-map", fmt.Sprintf("[v%d]", last),
		"-map", fmt.Sprintf("[a%d]", last),
		"-r", strconv.Itoa(plan.Meta.FPS),
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-movflags", "+faststart",
		output,
	)
	return runCommand(ctx, "ffmpeg", args...)
}

func videoTimelineTransitionSeconds(plan *Plan) float64 {
	if len(plan.Sections) < 2 {
		return 0
	}
	transition := 0.65
	for _, section := range plan.Sections {
		seconds := float64(section.DurationMillis) / 1000
		if seconds <= 0 {
			continue
		}
		maxForSection := seconds/3 - 0.05
		if maxForSection < transition {
			transition = maxForSection
		}
	}
	if transition < 0.15 {
		return 0
	}
	return transition
}

func videoTimelineFilter(plan *Plan, transition float64) string {
	var b strings.Builder
	cumulative := float64(plan.Sections[0].DurationMillis) / 1000
	fmt.Fprintf(&b, "[0:v]settb=AVTB[v0];")
	fmt.Fprintf(&b, "[0:a]asetpts=PTS-STARTPTS[a0];")
	for i := 1; i < len(plan.Sections); i++ {
		offset := cumulative - transition
		if offset < 0 {
			offset = 0
		}
		fmt.Fprintf(&b, "[%d:v]settb=AVTB[vbase%d];", i, i)
		fmt.Fprintf(&b, "[v%d][vbase%d]xfade=transition=fade:duration=%.3f:offset=%.3f[v%d];", i-1, i, transition, offset, i)
		cumulative += float64(plan.Sections[i].DurationMillis) / 1000
	}
	for i := 1; i < len(plan.Sections); i++ {
		fmt.Fprintf(&b, "[%d:a]asetpts=PTS-STARTPTS[a%d];", i, i)
	}
	for i := 0; i < len(plan.Sections); i++ {
		fmt.Fprintf(&b, "[a%d]", i)
	}
	fmt.Fprintf(&b, "concat=n=%d:v=0:a=1[a%d];", len(plan.Sections), len(plan.Sections)-1)
	return b.String()
}

func videoScalePadFilter(width, height int) string {
	width = evenVideoDimension(width, 1920)
	height = evenVideoDimension(height, 1080)
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", width, height, width, height)
}

func evenVideoDimension(value, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value%2 != 0 {
		value--
	}
	if value <= 0 {
		return 2
	}
	return value
}

func writeVideoConcatFile(path string, segments []string) error {
	var b strings.Builder
	for _, segment := range segments {
		abs, err := filepath.Abs(segment)
		if err != nil {
			return fmt.Errorf("resolve segment path failed: %w", err)
		}
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(abs, "'", "'\\''"))
		b.WriteString("'\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func burnVideoSubtitles(ctx context.Context, input, srt, output string) error {
	filter := "subtitles=filename='" + strings.ReplaceAll(srt, "'", "\\'") + "'"
	if err := runCommand(ctx, "ffmpeg", "-y", "-i", input, "-vf", filter, "-c:a", "copy", output); err != nil {
		return fmt.Errorf("burn subtitles failed: ffmpeg subtitles filter is unavailable or failed: %w", err)
	}
	return nil
}

func secondsArg(ms DurationMillis) string {
	return fmt.Sprintf("%.3f", float64(ms)/1000.0)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
