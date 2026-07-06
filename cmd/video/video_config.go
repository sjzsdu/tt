package videocmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type videoCommandConfig struct {
	OutputDir                   string
	InternalDir                 string
	TTSMode                     string
	AudioDir                    string
	TTSCommand                  string
	BailianAPIKeyEnv            string
	BailianBaseURL              string
	BailianModel                string
	BailianVoice                string
	BailianLanguageType         string
	BailianInstructions         string
	BailianOptimizeInstructions *bool
	Width                       int
	Height                      int
	FPS                         int
	WPM                         int
	Render                      bool
}

func resolveVideoCommandConfig(cmd *cobra.Command) (videoCommandConfig, error) {
	loaded, err := loadVideoTTConfig()
	if err != nil {
		return videoCommandConfig{}, err
	}
	cfg := videoCommandConfig{
		OutputDir:                   strings.TrimSpace(loaded.Merged.Video.OutputDir),
		InternalDir:                 strings.TrimSpace(loaded.Merged.Video.InternalDir),
		TTSMode:                     firstNonEmpty(loaded.Merged.Video.TTSMode, "none"),
		AudioDir:                    strings.TrimSpace(loaded.Merged.Video.AudioDir),
		TTSCommand:                  strings.TrimSpace(loaded.Merged.Video.TTSCommand),
		BailianAPIKeyEnv:            firstNonEmpty(loaded.Merged.Video.BailianAPIKeyEnv, "DASHSCOPE_API_KEY"),
		BailianBaseURL:              firstNonEmpty(loaded.Merged.Video.BailianBaseURL, strings.TrimSpace(os.Getenv("DASHSCOPE_BASE_HTTP_API_URL")), strings.TrimSpace(os.Getenv("BAILIAN_BASE_URL"))),
		BailianModel:                firstNonEmpty(loaded.Merged.Video.BailianModel, "qwen3-tts-flash"),
		BailianVoice:                firstNonEmpty(loaded.Merged.Video.BailianVoice, "Cherry"),
		BailianLanguageType:         firstNonEmpty(loaded.Merged.Video.BailianLanguageType, "Auto"),
		BailianInstructions:         strings.TrimSpace(loaded.Merged.Video.BailianInstructions),
		BailianOptimizeInstructions: loaded.Merged.Video.BailianOptimizeInstructions,
		Width:                       1920,
		Height:                      1080,
		FPS:                         30,
		WPM:                         150,
	}
	if loaded.Merged.Video.Width != nil {
		cfg.Width = *loaded.Merged.Video.Width
	}
	if loaded.Merged.Video.Height != nil {
		cfg.Height = *loaded.Merged.Video.Height
	}
	if loaded.Merged.Video.FPS != nil {
		cfg.FPS = *loaded.Merged.Video.FPS
	}
	if loaded.Merged.Video.WPM != nil {
		cfg.WPM = *loaded.Merged.Video.WPM
	}
	if loaded.Merged.Video.Render != nil {
		cfg.Render = *loaded.Merged.Video.Render
	}
	if cmd.Flags().Changed("wpm") {
		cfg.WPM = videoWordsPerMin
	}
	if cmd.Flags().Changed("tts") {
		cfg.TTSMode = videoTTSMode
	}
	if cmd.Flags().Changed("audio-dir") {
		cfg.AudioDir = videoAudioDir
	}
	if cmd.Flags().Changed("tts-command") {
		cfg.TTSCommand = videoTTSCommand
	}
	if cmd.Flags().Changed("render") {
		cfg.Render = videoRender
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = filepath.Join(".tt", "video")
	}
	if cfg.InternalDir == "" {
		cfg.InternalDir = filepath.Join(".tt", "video")
	}
	return cfg, nil
}

func applyVideoConfigDefaults(plan *Plan, cfg videoCommandConfig) {
	if plan.Meta.Width == 0 {
		plan.Meta.Width = cfg.Width
	}
	if plan.Meta.Height == 0 {
		plan.Meta.Height = cfg.Height
	}
	if plan.Meta.FPS == 0 {
		plan.Meta.FPS = cfg.FPS
	}
}
