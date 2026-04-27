package picoclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pcpkg "github.com/sipeed/picoclaw/pkg"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	pcskills "github.com/sipeed/picoclaw/pkg/skills"
	ttconfig "tt/internal/ttconfig"
)

type Options struct {
	Home      string
	Config    string
	TTConfig  ttconfig.Config
	TTSources ttconfig.Sources
}

type Runtime struct {
	Home       string
	ConfigPath string
	Config     *pcconfig.Config
	Skills     []pcskills.SkillInfo
	TTConfig   ttconfig.Config
	TTSources  ttconfig.Sources
}

func Load(opt Options) (*Runtime, error) {
	home := resolveHome(opt.Home)
	cfgPath := resolveConfigPath(home, opt.Config)

	prevHome, hasHome := os.LookupEnv(pcconfig.EnvHome)
	prevCfg, hasCfg := os.LookupEnv(pcconfig.EnvConfig)
	defer restoreEnv(pcconfig.EnvHome, prevHome, hasHome)
	defer restoreEnv(pcconfig.EnvConfig, prevCfg, hasCfg)
	_ = os.Setenv(pcconfig.EnvHome, home)
	_ = os.Setenv(pcconfig.EnvConfig, cfgPath)

	cfg, err := pcconfig.LoadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load picoclaw config failed: %w", err)
	}

	rt := &Runtime{
		Home:       home,
		ConfigPath: cfgPath,
		Config:     cfg,
		Skills:     loadSkills(cfg, home),
		TTConfig:   opt.TTConfig,
		TTSources:  opt.TTSources,
	}
	return rt, nil
}

func resolveHome(home string) string {
	if strings.TrimSpace(home) != "" {
		return strings.TrimSpace(home)
	}
	if envHome := strings.TrimSpace(os.Getenv(pcconfig.EnvHome)); envHome != "" {
		return envHome
	}
	return pcconfig.GetHome()
}

func resolveConfigPath(home, cfg string) string {
	if strings.TrimSpace(cfg) != "" {
		return strings.TrimSpace(cfg)
	}
	if envCfg := strings.TrimSpace(os.Getenv(pcconfig.EnvConfig)); envCfg != "" {
		return envCfg
	}
	return filepath.Join(home, "config.json")
}

func restoreEnv(key, value string, ok bool) {
	if ok {
		_ = os.Setenv(key, value)
		return
	}
	_ = os.Unsetenv(key)
}

func loadSkills(cfg *pcconfig.Config, home string) []pcskills.SkillInfo {
	if cfg == nil {
		return nil
	}
	builtin := strings.TrimSpace(os.Getenv(pcconfig.EnvBuiltinSkills))
	loader := pcskills.NewSkillsLoader(cfg.WorkspacePath(), filepath.Join(home, "skills"), builtin)
	return loader.ListSkills()
}

func DefaultModel(cfg *pcconfig.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Agents.Defaults.ModelName)
}

func Workspace(cfg *pcconfig.Config) string {
	if cfg == nil {
		return ""
	}
	ws := strings.TrimSpace(cfg.WorkspacePath())
	if ws != "" {
		return ws
	}
	home := pcconfig.GetHome()
	return filepath.Join(home, pcpkg.WorkspaceName)
}
