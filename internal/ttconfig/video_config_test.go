package ttconfig

import "testing"

func TestMergeVideoConfig(t *testing.T) {
	base := Config{Video: VideoConfig{
		OutputDir:    "videos",
		InternalDir:  ".tt/video",
		TTSMode:      "none",
		BailianModel: "qwen-tts",
		BailianVoice: "Cherry",
		Width:        IntPtr(1280),
		Height:       IntPtr(720),
		FPS:          IntPtr(24),
		WPM:          IntPtr(140),
		Render:       BoolPtr(false),
	}}
	overlay := Config{Video: VideoConfig{
		TTSMode:             "command",
		TTSCommand:          "say {{.Text}} -o {{.Output}}",
		BailianAPIKeyEnv:    "DASHSCOPE_API_KEY",
		BailianBaseURL:      "https://workspace.cn-beijing.maas.aliyuncs.com/api/v1",
		BailianLanguageType: "Chinese",
		FPS:                 IntPtr(30),
		Render:              BoolPtr(true),
	}}
	merged := Merge(base, overlay)
	if merged.Video.OutputDir != "videos" || merged.Video.InternalDir != ".tt/video" {
		t.Fatalf("video dirs were not preserved: %+v", merged.Video)
	}
	if merged.Video.TTSMode != "command" || merged.Video.TTSCommand == "" {
		t.Fatalf("video tts overlay not applied: %+v", merged.Video)
	}
	if merged.Video.BailianModel != "qwen-tts" || merged.Video.BailianBaseURL == "" || merged.Video.BailianLanguageType != "Chinese" {
		t.Fatalf("video bailian merge wrong: %+v", merged.Video)
	}
	if merged.Video.Width == nil || *merged.Video.Width != 1280 || merged.Video.FPS == nil || *merged.Video.FPS != 30 {
		t.Fatalf("video numeric merge wrong: %+v", merged.Video)
	}
	if merged.Video.Render == nil || !*merged.Video.Render {
		t.Fatalf("video render merge wrong: %+v", merged.Video)
	}
}
