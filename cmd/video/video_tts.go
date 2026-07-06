package videocmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type videoTTSOptions struct {
	Mode                        string
	AudioDir                    string
	Command                     string
	WorkDir                     string
	BailianAPIKeyEnv            string
	BailianBaseURL              string
	BailianModel                string
	BailianVoice                string
	BailianLanguageType         string
	BailianInstructions         string
	BailianOptimizeInstructions *bool
}

type videoTTSRequest struct {
	Index  int
	Title  string
	Text   string
	Voice  string
	Output string
}

type videoTTSResult struct {
	AudioPath string
	Duration  time.Duration
}

type videoTTSProvider interface {
	Synthesize(ctx context.Context, req videoTTSRequest) (videoTTSResult, error)
}

type videoNoopTTSProvider struct{}

func (videoNoopTTSProvider) Synthesize(context.Context, videoTTSRequest) (videoTTSResult, error) {
	return videoTTSResult{}, nil
}

type videoAudioDirTTSProvider struct {
	dir string
}

func (p videoAudioDirTTSProvider) Synthesize(_ context.Context, req videoTTSRequest) (videoTTSResult, error) {
	path, err := findVideoAudioFile(p.dir, req.Index)
	if err != nil {
		return videoTTSResult{}, err
	}
	return videoTTSResult{AudioPath: path}, nil
}

type videoCommandTTSProvider struct {
	commandTemplate *template.Template
	workDir         string
}

func (p videoCommandTTSProvider) Synthesize(ctx context.Context, req videoTTSRequest) (videoTTSResult, error) {
	if req.Output == "" {
		req.Output = filepath.Join(p.workDir, fmt.Sprintf("%03d.wav", req.Index))
	}
	if err := os.MkdirAll(filepath.Dir(req.Output), 0o755); err != nil {
		return videoTTSResult{}, fmt.Errorf("create TTS output directory failed: %w", err)
	}
	var rendered strings.Builder
	if err := p.commandTemplate.Execute(&rendered, req); err != nil {
		return videoTTSResult{}, fmt.Errorf("render TTS command failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", rendered.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return videoTTSResult{}, fmt.Errorf("TTS command failed: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(req.Output); err != nil {
		return videoTTSResult{}, fmt.Errorf("TTS command did not create output %s: %w", req.Output, err)
	}
	return videoTTSResult{AudioPath: req.Output}, nil
}

func newVideoTTSProvider(opts videoTTSOptions) (videoTTSProvider, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "none"
	}
	switch mode {
	case "none":
		return videoNoopTTSProvider{}, nil
	case "audio-dir":
		if strings.TrimSpace(opts.AudioDir) == "" {
			return nil, fmt.Errorf("--audio-dir is required when --tts audio-dir")
		}
		abs, err := filepath.Abs(opts.AudioDir)
		if err != nil {
			return nil, fmt.Errorf("resolve audio dir failed: %w", err)
		}
		return videoAudioDirTTSProvider{dir: abs}, nil
	case "command":
		if strings.TrimSpace(opts.Command) == "" {
			return nil, fmt.Errorf("--tts-command is required when --tts command")
		}
		workDir := opts.WorkDir
		if strings.TrimSpace(workDir) == "" {
			workDir = filepath.Join(os.TempDir(), "tt-video-audio")
		}
		abs, err := filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("resolve workdir failed: %w", err)
		}
		tmpl, err := template.New("tts-command").Parse(opts.Command)
		if err != nil {
			return nil, fmt.Errorf("parse TTS command template failed: %w", err)
		}
		return videoCommandTTSProvider{commandTemplate: tmpl, workDir: abs}, nil
	case "bailian":
		return newVideoBailianTTSProvider(opts)
	default:
		return nil, fmt.Errorf("unsupported --tts %q; expected none, audio-dir, command, or bailian", opts.Mode)
	}
}

func applyVideoTTSProvider(ctx context.Context, provider videoTTSProvider, plan *Plan, progress *videoProgress) error {
	if provider == nil {
		return nil
	}
	var cursor int64
	for i := range plan.Sections {
		section := &plan.Sections[i]
		progress.Step("正在合成第 %d/%d 段语音: %s", i+1, len(plan.Sections), section.Title)
		result, err := provider.Synthesize(ctx, videoTTSRequest{
			Index: section.Index,
			Title: section.Title,
			Text:  section.Narration,
			Voice: plan.Meta.Voice,
		})
		if err != nil {
			return fmt.Errorf("synthesize section %d failed: %w", section.Index, err)
		}
		if result.AudioPath != "" {
			section.Audio = result.AudioPath
		}
		if result.Duration > 0 {
			section.DurationMillis = DurationMillis(result.Duration / time.Millisecond)
		}
		section.StartMillis = DurationMillis(cursor)
		cursor += int64(section.DurationMillis)
		section.EndMillis = DurationMillis(cursor)
	}
	plan.TotalDuration = DurationMillis(cursor)
	progress.Step("语音处理完成，总时长 %s", formatSRTDuration(plan.TotalDuration))
	return nil
}

func findVideoAudioFile(dir string, index int) (string, error) {
	exts := []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg"}
	names := []string{fmt.Sprintf("%03d", index), fmt.Sprintf("%02d", index), strconv.Itoa(index)}
	for _, name := range names {
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("no audio file found for section %d in %s; expected names like %03d.wav or %d.mp3", index, dir, index, index)
}
