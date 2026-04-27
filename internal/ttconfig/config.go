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
	Picoclaw PicoclawConfig `json:"picoclaw,omitempty"`
	Agent    AgentConfig    `json:"agent,omitempty"`
	Markdown MarkdownConfig `json:"markdown,omitempty"`
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

type MarkdownConfig struct {
	Port        *int     `json:"port,omitempty"`
	Content     string   `json:"content,omitempty"`
	ContentOnly *bool    `json:"content_only,omitempty"`
	Patterns    []string `json:"patterns,omitempty"`
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
