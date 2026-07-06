package videocmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	videoOutPath      string
	videoPlanPath     string
	videoSRTPath      string
	videoWordsPerMin  int
	videoJSON         bool
	videoTTSMode      string
	videoAudioDir     string
	videoTTSCommand   string
	videoDoctorScript string
	videoRender       bool
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
		progress := startVideoProgress("正在准备视频生成", videoJSON)
		cfg, err := resolveVideoCommandConfig(cmd)
		if err != nil {
			progress.Clear()
			return err
		}
		progress.Step("正在解析讲稿 %s", args[0])
		plan, err := buildVideoPlanFromFile(args[0], cfg.WPM)
		if err != nil {
			progress.Clear()
			return err
		}
		applyVideoConfigDefaults(&plan, cfg)
		progress.Step("已解析 %d 段讲稿，准备输出目录", len(plan.Sections))
		if videoOutPath != "" {
			plan.Output = videoOutPath
		}
		artifactDir := defaultVideoArtifactDir(plan.Script, cfg.OutputDir)
		if plan.Output == "" {
			plan.Output = filepath.Join(artifactDir, "output.mp4")
		}
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			progress.Clear()
			return fmt.Errorf("create video artifact directory failed: %w", err)
		}
		internalDir := defaultVideoInternalDir(plan.Script, cfg.InternalDir)
		progress.Step("正在初始化 TTS provider: %s", cfg.TTSMode)
		provider, err := newVideoTTSProvider(videoTTSOptions{
			Mode:                        cfg.TTSMode,
			AudioDir:                    cfg.AudioDir,
			Command:                     cfg.TTSCommand,
			WorkDir:                     filepath.Join(internalDir, "audio"),
			BailianAPIKeyEnv:            cfg.BailianAPIKeyEnv,
			BailianBaseURL:              cfg.BailianBaseURL,
			BailianModel:                cfg.BailianModel,
			BailianVoice:                cfg.BailianVoice,
			BailianLanguageType:         cfg.BailianLanguageType,
			BailianInstructions:         cfg.BailianInstructions,
			BailianOptimizeInstructions: cfg.BailianOptimizeInstructions,
		})
		if err != nil {
			progress.Clear()
			return err
		}
		if err := applyVideoTTSProvider(cmd.Context(), provider, &plan, progress); err != nil {
			progress.Clear()
			return err
		}
		if videoPlanPath == "" {
			videoPlanPath = filepath.Join(artifactDir, "plan.json")
		}
		if videoSRTPath == "" {
			videoSRTPath = filepath.Join(artifactDir, "subtitles.srt")
		}
		if cfg.Render || strings.EqualFold(filepath.Ext(plan.Output), ".mp4") {
			if err := renderVideoPlan(cmd.Context(), &plan, videoRenderOptions{WorkDir: filepath.Join(internalDir, "work"), SRTPath: videoSRTPath, Progress: progress}); err != nil {
				progress.Clear()
				return err
			}
		}
		progress.Step("正在写入 plan 和字幕文件")
		if videoPlanPath != "" {
			if err := writeVideoPlan(videoPlanPath, plan); err != nil {
				progress.Clear()
				return err
			}
		}
		if videoSRTPath != "" {
			if err := os.WriteFile(videoSRTPath, []byte(renderVideoSRT(plan)), 0o644); err != nil {
				progress.Clear()
				return fmt.Errorf("write SRT failed: %w", err)
			}
		}
		if videoJSON || (videoPlanPath == "" && videoSRTPath == "") {
			progress.Clear()
			return writeVideoPlanTo(cmd.OutOrStdout(), plan)
		}
		progress.Done("视频生成完成")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Video plan ready: %d sections, duration %s\n", len(plan.Sections), formatSRTDuration(plan.TotalDuration))
		if plan.Output != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Output: %s\n", plan.Output)
		}
		if videoPlanPath != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plan: %s\n", videoPlanPath)
		}
		if videoSRTPath != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Subtitles: %s\n", videoSRTPath)
		}
		return nil
	},
}

var videoDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check whether slide video generation dependencies are available",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveVideoCommandConfig(cmd)
		if err != nil {
			return err
		}
		report := runVideoDoctor(videoDoctorOptions{
			Script:                      videoDoctorScript,
			TTSMode:                     cfg.TTSMode,
			AudioDir:                    cfg.AudioDir,
			TTSCommand:                  cfg.TTSCommand,
			BailianAPIKeyEnv:            cfg.BailianAPIKeyEnv,
			BailianBaseURL:              cfg.BailianBaseURL,
			BailianModel:                cfg.BailianModel,
			BailianVoice:                cfg.BailianVoice,
			BailianLanguageType:         cfg.BailianLanguageType,
			BailianInstructions:         cfg.BailianInstructions,
			BailianOptimizeInstructions: cfg.BailianOptimizeInstructions,
		})
		for _, check := range report.Checks {
			status := "ok"
			if !check.OK {
				status = "missing"
			}
			if check.Detail == "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", status, check.Name)
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", status, check.Name, check.Detail)
		}
		if !report.OK {
			return fmt.Errorf("video doctor found missing requirements")
		}
		return nil
	},
}

func New(deps Dependencies) *cobra.Command {
	configureDependencies(deps)
	videoCmd.AddCommand(videoGenerateCmd)
	videoCmd.AddCommand(videoDoctorCmd)
	videoGenerateCmd.Flags().StringVarP(&videoOutPath, "out", "o", "", "target mp4 path; also sets default .plan.json and .srt paths")
	videoGenerateCmd.Flags().StringVar(&videoPlanPath, "plan", "", "write production plan JSON to this path")
	videoGenerateCmd.Flags().StringVar(&videoSRTPath, "srt", "", "write subtitles as SRT to this path")
	videoGenerateCmd.Flags().IntVar(&videoWordsPerMin, "wpm", 150, "estimated narration speed used for subtitle timing")
	videoGenerateCmd.Flags().BoolVar(&videoJSON, "json", false, "print the production plan JSON to stdout")
	videoGenerateCmd.Flags().StringVar(&videoTTSMode, "tts", "none", "TTS provider: none, audio-dir, command, bailian")
	videoGenerateCmd.Flags().StringVar(&videoAudioDir, "audio-dir", "", "directory containing existing narration audio for --tts audio-dir")
	videoGenerateCmd.Flags().StringVar(&videoTTSCommand, "tts-command", "", "shell command template for --tts command; supports {{.Text}}, {{.Output}}, {{.Voice}}, {{.Index}}, {{.Title}}")
	videoGenerateCmd.Flags().BoolVar(&videoRender, "render", false, "render an mp4 using Chrome screenshots and ffmpeg")
	videoDoctorCmd.Flags().StringVar(&videoDoctorScript, "script", "", "optional video script to validate")
	videoDoctorCmd.Flags().StringVar(&videoTTSMode, "tts", "none", "TTS provider to validate: none, audio-dir, command, bailian")
	videoDoctorCmd.Flags().StringVar(&videoAudioDir, "audio-dir", "", "directory containing existing narration audio for --tts audio-dir")
	videoDoctorCmd.Flags().StringVar(&videoTTSCommand, "tts-command", "", "shell command template for --tts command")
	return videoCmd
}
