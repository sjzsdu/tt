package videocmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type videoDoctorOptions struct {
	Script                      string
	TTSMode                     string
	AudioDir                    string
	TTSCommand                  string
	WorkDir                     string
	BailianAPIKeyEnv            string
	BailianBaseURL              string
	BailianModel                string
	BailianVoice                string
	BailianLanguageType         string
	BailianInstructions         string
	BailianOptimizeInstructions *bool
}

type videoDoctorReport struct {
	OK     bool               `json:"ok"`
	Checks []videoDoctorCheck `json:"checks"`
}

type videoDoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func runVideoDoctor(opts videoDoctorOptions) videoDoctorReport {
	report := videoDoctorReport{OK: true}
	add := func(name string, ok bool, detail string) {
		report.Checks = append(report.Checks, videoDoctorCheck{Name: name, OK: ok, Detail: detail})
		if !ok {
			report.OK = false
		}
	}

	if path, err := exec.LookPath("ffmpeg"); err == nil {
		add("ffmpeg", true, path)
	} else {
		add("ffmpeg", false, "install ffmpeg and ensure it is on PATH")
	}

	if path := findVideoChromePath(); path != "" {
		add("chrome", true, path)
	} else {
		add("chrome", false, "install Google Chrome/Chromium for slide capture")
	}

	if strings.TrimSpace(opts.Script) != "" {
		plan, err := buildVideoPlanFromFile(opts.Script, 150)
		if err != nil {
			add("script", false, err.Error())
		} else {
			add("script", true, fmt.Sprintf("%d sections, slides %s", len(plan.Sections), plan.Meta.Slides))
			if _, err := os.Stat(plan.Meta.Slides); err != nil {
				add("slides", false, err.Error())
			} else {
				add("slides", true, plan.Meta.Slides)
			}
		}
	}

	if _, err := newVideoTTSProvider(videoTTSOptions{
		Mode:                        opts.TTSMode,
		AudioDir:                    opts.AudioDir,
		Command:                     opts.TTSCommand,
		WorkDir:                     opts.WorkDir,
		BailianAPIKeyEnv:            opts.BailianAPIKeyEnv,
		BailianBaseURL:              opts.BailianBaseURL,
		BailianModel:                opts.BailianModel,
		BailianVoice:                opts.BailianVoice,
		BailianLanguageType:         opts.BailianLanguageType,
		BailianInstructions:         opts.BailianInstructions,
		BailianOptimizeInstructions: opts.BailianOptimizeInstructions,
	}); err != nil {
		add("tts", false, err.Error())
	} else {
		mode := strings.TrimSpace(opts.TTSMode)
		if mode == "" {
			mode = "none"
		}
		add("tts", true, mode)
	}

	return report
}

func findVideoChromePath() string {
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
