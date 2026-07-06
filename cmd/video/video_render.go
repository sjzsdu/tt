package videocmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type videoRenderOptions struct {
	WorkDir  string
	SRTPath  string
	Progress *videoProgress
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

	opts.Progress.Step("正在启动 slide 捕获服务")
	if err := captureVideoSlides(ctx, baseURL, plan, workDir, opts.Progress); err != nil {
		return err
	}
	segments, err := renderVideoSegments(ctx, plan, workDir, opts.Progress)
	if err != nil {
		return err
	}
	concatPath := filepath.Join(workDir, "concat.txt")
	if err := writeVideoConcatFile(concatPath, segments); err != nil {
		return err
	}
	mergedPath := filepath.Join(workDir, "merged.mp4")
	opts.Progress.Step("正在合并 %d 个视频片段", len(segments))
	if err := runCommand(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-c", "copy", mergedPath); err != nil {
		return fmt.Errorf("concat video segments failed: %w", err)
	}
	if opts.SRTPath != "" {
		opts.Progress.Step("正在生成并嵌入字幕")
		if err := os.WriteFile(opts.SRTPath, []byte(renderVideoSRT(*plan)), 0o644); err != nil {
			return fmt.Errorf("write SRT failed: %w", err)
		}
		if err := burnVideoSubtitles(ctx, mergedPath, opts.SRTPath, plan.Output); err != nil {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(plan.Output), 0o755); err != nil && filepath.Dir(plan.Output) != "." {
		return fmt.Errorf("create output directory failed: %w", err)
	}
	return os.Rename(mergedPath, plan.Output)
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
		if muxErr := runCommand(ctx, "ffmpeg", "-y", "-i", input, "-i", srt, "-c", "copy", "-c:s", "mov_text", output); muxErr == nil {
			return nil
		}
		return fmt.Errorf("burn subtitles failed: %w", err)
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
