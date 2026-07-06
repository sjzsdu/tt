package videocmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func renderVideoPlanBrowserContinuous(ctx context.Context, baseURL string, plan *Plan, workDir string, opts videoRenderOptions) error {
	if len(plan.Sections) == 0 {
		return fmt.Errorf("plan has no sections")
	}
	framesDir := filepath.Join(workDir, "browser-frames")
	if err := os.RemoveAll(framesDir); err != nil {
		return fmt.Errorf("clean browser frames failed: %w", err)
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return fmt.Errorf("create browser frames dir failed: %w", err)
	}
	videoOnly := filepath.Join(workDir, "browser-video.mp4")
	audioTrack := filepath.Join(workDir, "browser-audio.m4a")
	merged := filepath.Join(workDir, "browser-merged.mp4")

	frameCount, err := recordBrowserSlideFrames(ctx, baseURL, plan, framesDir, opts.Progress)
	if err != nil {
		return err
	}
	minFrames := max(2, plan.Meta.FPS*2)
	if frameCount < minFrames {
		return fmt.Errorf("browser recording captured only %d frames", frameCount)
	}
	if err := encodeBrowserFrames(ctx, framesDir, frameCount, plan, videoOnly); err != nil {
		return err
	}
	if err := renderBrowserAudioTrack(ctx, plan, workDir, audioTrack); err != nil {
		return err
	}
	if err := runCommand(ctx, "ffmpeg", "-y", "-i", videoOnly, "-i", audioTrack, "-c:v", "copy", "-c:a", "aac", "-shortest", merged); err != nil {
		return fmt.Errorf("merge browser video and audio failed: %w", err)
	}
	if opts.SRTPath != "" {
		opts.Progress.Step("正在生成并嵌入字幕")
		if err := os.WriteFile(opts.SRTPath, []byte(renderVideoSRT(*plan)), 0o644); err != nil {
			return fmt.Errorf("write SRT failed: %w", err)
		}
		if err := burnVideoSubtitles(ctx, merged, opts.SRTPath, plan.Output); err != nil {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(plan.Output), 0o755); err != nil && filepath.Dir(plan.Output) != "." {
		return fmt.Errorf("create output directory failed: %w", err)
	}
	return os.Rename(merged, plan.Output)
}

func recordBrowserSlideFrames(ctx context.Context, baseURL string, plan *Plan, framesDir string, progress *videoProgress) (int, error) {
	chromePath := findVideoChromePath()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", true), chromedp.Flag("disable-gpu", true), chromedp.Flag("no-sandbox", true), chromedp.WindowSize(plan.Meta.Width, plan.Meta.Height))
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()
	browserCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(nil))
	defer cancel()
	recordCtx, cancel := context.WithTimeout(browserCtx, time.Duration(plan.TotalDuration)*time.Millisecond+10*time.Second)
	defer cancel()

	url := buildSlideURL(baseURL, plan, plan.Sections[0].Slide)
	if err := chromedp.Run(recordCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.WaitReady(".reveal .slides", chromedp.ByQuery),
		chromedp.WaitVisible(".reveal .slides section", chromedp.ByQuery),
		chromedp.Evaluate(browserTimelineScript(plan), nil),
		chromedp.Evaluate(browserRepaintDriverScript(), nil),
	); err != nil {
		return 0, fmt.Errorf("prepare browser continuous recording failed: %w", err)
	}

	frameInterval := time.Second / time.Duration(max(1, plan.Meta.FPS))
	started := time.Now()
	totalDuration := time.Duration(plan.TotalDuration) * time.Millisecond
	targetFrames := max(1, int((totalDuration+frameInterval-1)/frameInterval))
	frameCount := 0
	for frameCount < targetFrames {
		targetTime := started.Add(time.Duration(frameCount) * frameInterval)
		if sleep := time.Until(targetTime); sleep > 0 {
			time.Sleep(sleep)
		}
		var buf []byte
		if err := chromedp.Run(recordCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
			return frameCount, fmt.Errorf("capture browser frame failed: %w", err)
		}
		frameCount++
		name := filepath.Join(framesDir, fmt.Sprintf("frame-%06d.png", frameCount))
		if err := os.WriteFile(name, buf, 0o644); err != nil {
			return frameCount, fmt.Errorf("write browser frame failed: %w", err)
		}
		if frameCount%max(1, plan.Meta.FPS) == 0 {
			progress.Step("浏览器连续录制已捕获 %d 帧", frameCount)
		}
	}
	return frameCount, nil
}

func browserRepaintDriverScript() string {
	return `(() => {
  let el = document.getElementById('tt-video-repaint-driver');
  if (!el) {
    el = document.createElement('div');
    el.id = 'tt-video-repaint-driver';
    Object.assign(el.style, {
      position: 'fixed',
      width: '1px',
      height: '1px',
      left: '-10px',
      top: '-10px',
      opacity: '0',
      pointerEvents: 'none'
    });
    document.body.appendChild(el);
  }
  let tick = 0;
  const pump = () => {
    tick += 1;
    el.style.transform = 'translateX(' + (tick % 2) + 'px)';
    window.__ttVideoRepaintFrame = requestAnimationFrame(pump);
  };
  if (window.__ttVideoRepaintFrame) cancelAnimationFrame(window.__ttVideoRepaintFrame);
  pump();
})();`
}

func browserTimelineScript(plan *Plan) string {
	var b strings.Builder
	b.WriteString(`(() => { const deck = window.Reveal; if (!deck) return;`)
	for _, section := range plan.Sections {
		at := int64(section.StartMillis)
		if at < 0 {
			at = 0
		}
		fmt.Fprintf(&b, "setTimeout(() => deck.slide(%d, 0, 0), %d);", section.Slide-1, at)
	}
	b.WriteString(`})();`)
	return b.String()
}

func encodeBrowserFrames(ctx context.Context, framesDir string, frameCount int, plan *Plan, output string) error {
	return runCommand(ctx, "ffmpeg", "-y", "-framerate", strconv.Itoa(plan.Meta.FPS), "-i", filepath.Join(framesDir, "frame-%06d.png"), "-vf", videoScalePadFilter(plan.Meta.Width, plan.Meta.Height), "-frames:v", strconv.Itoa(frameCount), "-pix_fmt", "yuv420p", "-c:v", "libx264", "-crf", "23", "-preset", "medium", "-movflags", "+faststart", output)
}

func renderBrowserAudioTrack(ctx context.Context, plan *Plan, workDir string, output string) error {
	concatPath := filepath.Join(workDir, "browser-audio-concat.txt")
	var b strings.Builder
	for _, section := range plan.Sections {
		audio := section.Audio
		if strings.TrimSpace(audio) == "" {
			audio = filepath.Join(workDir, fmt.Sprintf("browser-silent-%03d.wav", section.Index))
			if err := runCommand(ctx, "ffmpeg", "-y", "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100", "-t", secondsArg(section.DurationMillis), audio); err != nil {
				return fmt.Errorf("create browser silent audio failed: %w", err)
			}
		}
		abs, err := filepath.Abs(audio)
		if err != nil {
			return fmt.Errorf("resolve audio path failed: %w", err)
		}
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(abs, "'", "'\\''"))
		b.WriteString("'\n")
	}
	if err := os.WriteFile(concatPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write browser audio concat failed: %w", err)
	}
	return runCommand(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-c:a", "aac", output)
}
