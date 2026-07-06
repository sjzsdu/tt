package ttconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	globalDirName    = ".tt"
	configFileName   = "config.json"
	projectDirName   = ".tt"
	envConfigPath    = "TT_CONFIG"
	envProjectConfig = "TT_PROJECT_CONFIG"
)

type Config struct {
	Picoclaw     PicoclawConfig     `json:"picoclaw,omitempty"`
	Agent        AgentConfig        `json:"agent,omitempty"`
	Debate       DebateConfig       `json:"debate,omitempty"`
	Markdown     MarkdownConfig     `json:"markdown,omitempty"`
	Conversation ConversationConfig `json:"conversation,omitempty"`
	Mirror       MirrorConfig       `json:"mirror,omitempty"`
	Video        VideoConfig        `json:"video,omitempty"`
	Paths        PathsConfig        `json:"paths,omitempty"`
}

type PathsConfig struct {
	FormulaDir    string `json:"formula_dir,omitempty"`
	AgentDir      string `json:"agent_dir,omitempty"`
	FormulaRunDir string `json:"formula_run_dir,omitempty"`
}

type PicoclawConfig struct {
	Home   string `json:"home,omitempty"`
	Config string `json:"config,omitempty"`
}

type AgentConfig struct {
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Debug   *bool  `json:"debug,omitempty"`
}

type DebateConfig struct {
	Agents []string `json:"agents,omitempty"`
	Judge  string   `json:"judge,omitempty"`
	Rounds *int     `json:"rounds,omitempty"`
	Output string   `json:"output,omitempty"`
}

type MarkdownConfig struct {
	Port        *int     `json:"port,omitempty"`
	Content     string   `json:"content,omitempty"`
	ContentOnly *bool    `json:"content_only,omitempty"`
	Patterns    []string `json:"patterns,omitempty"`
}

type ConversationConfig struct {
	Port     *int     `json:"port,omitempty"`
	File     string   `json:"file,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
}

type MirrorConfig struct {
	SourceDir  string `json:"source_dir,omitempty"`
	TargetDir  string `json:"target_dir,omitempty"`
	ConfigFile string `json:"config_file,omitempty"`
}

type VideoConfig struct {
	OutputDir                   string `json:"output_dir,omitempty"`
	InternalDir                 string `json:"internal_dir,omitempty"`
	TTSMode                     string `json:"tts_mode,omitempty"`
	AudioDir                    string `json:"audio_dir,omitempty"`
	TTSCommand                  string `json:"tts_command,omitempty"`
	BailianAPIKeyEnv            string `json:"bailian_api_key_env,omitempty"`
	BailianBaseURL              string `json:"bailian_base_url,omitempty"`
	BailianModel                string `json:"bailian_model,omitempty"`
	BailianVoice                string `json:"bailian_voice,omitempty"`
	BailianLanguageType         string `json:"bailian_language_type,omitempty"`
	BailianInstructions         string `json:"bailian_instructions,omitempty"`
	BailianOptimizeInstructions *bool  `json:"bailian_optimize_instructions,omitempty"`
	Width                       *int   `json:"width,omitempty"`
	Height                      *int   `json:"height,omitempty"`
	FPS                         *int   `json:"fps,omitempty"`
	WPM                         *int   `json:"wpm,omitempty"`
	Render                      *bool  `json:"render,omitempty"`
	RenderMode                  string `json:"render_mode,omitempty"`
}

type Sources struct {
	GlobalPath  string `json:"global_path,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
}

type Loaded struct {
	Sources Sources
	Global  Config
	Project Config
	Merged  Config
}

func Load(cwd string) (Loaded, error) {
	globalPath, err := resolveGlobalConfigPath()
	if err != nil {
		return Loaded{}, err
	}
	projectPath, err := resolveProjectConfigPath(cwd)
	if err != nil {
		return Loaded{}, err
	}

	globalCfg, err := loadFile(globalPath)
	if err != nil {
		return Loaded{}, fmt.Errorf("load global tt config failed: %w", err)
	}
	projectCfg, err := loadFile(projectPath)
	if err != nil {
		return Loaded{}, fmt.Errorf("load project tt config failed: %w", err)
	}

	merged := Merge(Merge(Config{}, globalCfg), projectCfg)
	return Loaded{
		Sources: Sources{
			GlobalPath:  globalPath,
			ProjectPath: projectPath,
		},
		Global:  globalCfg,
		Project: projectCfg,
		Merged:  merged,
	}, nil
}

func Merge(base Config, overlay Config) Config {
	out := base
	if v := strings.TrimSpace(overlay.Picoclaw.Home); v != "" {
		out.Picoclaw.Home = v
	}
	if v := strings.TrimSpace(overlay.Picoclaw.Config); v != "" {
		out.Picoclaw.Config = v
	}
	if v := strings.TrimSpace(overlay.Agent.Session); v != "" {
		out.Agent.Session = v
	}
	if v := strings.TrimSpace(overlay.Agent.Agent); v != "" {
		out.Agent.Agent = v
	}
	if v := strings.TrimSpace(overlay.Agent.Model); v != "" {
		out.Agent.Model = v
	}
	if overlay.Agent.Debug != nil {
		b := *overlay.Agent.Debug
		out.Agent.Debug = &b
	}
	if len(overlay.Debate.Agents) > 0 {
		out.Debate.Agents = append([]string(nil), overlay.Debate.Agents...)
	}
	if v := strings.TrimSpace(overlay.Debate.Judge); v != "" {
		out.Debate.Judge = v
	}
	if overlay.Debate.Rounds != nil {
		v := *overlay.Debate.Rounds
		out.Debate.Rounds = &v
	}
	if v := strings.TrimSpace(overlay.Debate.Output); v != "" {
		out.Debate.Output = v
	}
	if overlay.Markdown.Port != nil {
		v := *overlay.Markdown.Port
		out.Markdown.Port = &v
	}
	if v := strings.TrimSpace(overlay.Markdown.Content); v != "" {
		out.Markdown.Content = v
	}
	if overlay.Markdown.ContentOnly != nil {
		v := *overlay.Markdown.ContentOnly
		out.Markdown.ContentOnly = &v
	}
	if len(overlay.Markdown.Patterns) > 0 {
		out.Markdown.Patterns = append([]string(nil), overlay.Markdown.Patterns...)
	}
	if overlay.Conversation.Port != nil {
		v := *overlay.Conversation.Port
		out.Conversation.Port = &v
	}
	if v := strings.TrimSpace(overlay.Conversation.File); v != "" {
		out.Conversation.File = v
	}
	if len(overlay.Conversation.Patterns) > 0 {
		out.Conversation.Patterns = append([]string(nil), overlay.Conversation.Patterns...)
	}
	if v := strings.TrimSpace(overlay.Mirror.SourceDir); v != "" {
		out.Mirror.SourceDir = v
	}
	if v := strings.TrimSpace(overlay.Mirror.TargetDir); v != "" {
		out.Mirror.TargetDir = v
	}
	if v := strings.TrimSpace(overlay.Mirror.ConfigFile); v != "" {
		out.Mirror.ConfigFile = v
	}
	if v := strings.TrimSpace(overlay.Video.OutputDir); v != "" {
		out.Video.OutputDir = v
	}
	if v := strings.TrimSpace(overlay.Video.InternalDir); v != "" {
		out.Video.InternalDir = v
	}
	if v := strings.TrimSpace(overlay.Video.TTSMode); v != "" {
		out.Video.TTSMode = v
	}
	if v := strings.TrimSpace(overlay.Video.AudioDir); v != "" {
		out.Video.AudioDir = v
	}
	if v := strings.TrimSpace(overlay.Video.TTSCommand); v != "" {
		out.Video.TTSCommand = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianAPIKeyEnv); v != "" {
		out.Video.BailianAPIKeyEnv = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianBaseURL); v != "" {
		out.Video.BailianBaseURL = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianModel); v != "" {
		out.Video.BailianModel = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianVoice); v != "" {
		out.Video.BailianVoice = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianLanguageType); v != "" {
		out.Video.BailianLanguageType = v
	}
	if v := strings.TrimSpace(overlay.Video.BailianInstructions); v != "" {
		out.Video.BailianInstructions = v
	}
	if overlay.Video.BailianOptimizeInstructions != nil {
		v := *overlay.Video.BailianOptimizeInstructions
		out.Video.BailianOptimizeInstructions = &v
	}
	if overlay.Video.Width != nil {
		v := *overlay.Video.Width
		out.Video.Width = &v
	}
	if overlay.Video.Height != nil {
		v := *overlay.Video.Height
		out.Video.Height = &v
	}
	if overlay.Video.FPS != nil {
		v := *overlay.Video.FPS
		out.Video.FPS = &v
	}
	if overlay.Video.WPM != nil {
		v := *overlay.Video.WPM
		out.Video.WPM = &v
	}
	if overlay.Video.Render != nil {
		v := *overlay.Video.Render
		out.Video.Render = &v
	}
	if v := strings.TrimSpace(overlay.Video.RenderMode); v != "" {
		out.Video.RenderMode = v
	}
	if v := strings.TrimSpace(overlay.Paths.FormulaDir); v != "" {
		out.Paths.FormulaDir = v
	}
	if v := strings.TrimSpace(overlay.Paths.AgentDir); v != "" {
		out.Paths.AgentDir = v
	}
	if v := strings.TrimSpace(overlay.Paths.FormulaRunDir); v != "" {
		out.Paths.FormulaRunDir = v
	}
	return out
}

func IntPtr(v int) *int {
	i := v
	return &i
}

func BoolPtr(v bool) *bool {
	b := v
	return &b
}

func resolveGlobalConfigPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv(envConfigPath)); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir failed: %w", err)
	}
	return filepath.Join(home, globalDirName, configFileName), nil
}

func resolveProjectConfigPath(cwd string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envProjectConfig)); v != "" {
		return v, nil
	}
	start := strings.TrimSpace(cwd)
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory failed: %w", err)
		}
	}
	start, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve absolute working directory failed: %w", err)
	}
	root, err := findProjectSearchRoot(start)
	if err != nil {
		return "", err
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, projectDirName, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("stat project config failed: %w", err)
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return filepath.Join(root, projectDirName, configFileName), nil
}

func findProjectSearchRoot(start string) (string, error) {
	current := start
	lastWithGit := ""
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			lastWithGit = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if lastWithGit != "" {
		return lastWithGit, nil
	}
	return start, nil
}

func loadFile(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s failed: %w", path, err)
	}
	return cfg, nil
}
