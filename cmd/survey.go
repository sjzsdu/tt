package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

var (
	surveyModel   string
	surveySession string
	surveyDebug   bool
	surveyHome    string
	surveyConfig  string
	surveyDeep    bool
	surveyMax     int
)

var surveyCmd = &cobra.Command{
	Use:   "survey",
	Short: "Survey the machine: discover installed tools and analyze capabilities with AI",
	Long: `Survey scans PATH for executables and uses an embedded picoclaw agent
to analyze what this machine can do.

It filters out common system utilities to focus on interesting tools,
then asks the agent to summarize the machine's capabilities.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSurvey(cmd)
	},
}

func init() {
	rootCmd.AddCommand(surveyCmd)
	surveyCmd.Flags().StringVar(&surveyModel, "model", "", "model to use; defaults to picoclaw config default")
	surveyCmd.Flags().StringVarP(&surveySession, "session", "s", "cli:survey", "session key")
	surveyCmd.Flags().BoolVarP(&surveyDebug, "debug", "d", false, "enable debug logging")
	surveyCmd.Flags().StringVar(&surveyHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	surveyCmd.Flags().StringVar(&surveyConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	surveyCmd.Flags().BoolVar(&surveyDeep, "deep", false, "include more system utilities in scan")
	surveyCmd.Flags().IntVar(&surveyMax, "max", 300, "max tools to include in analysis (0=all)")
}

func runSurvey(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "Scanning PATH...")
	tools := scanPATH(surveyDeep)
	total := len(tools)
	if surveyMax > 0 && total > surveyMax {
		tools = tools[:surveyMax]
	}
	fmt.Fprintf(out, "Found %d tools. Analyzing...\n\n", total)

	prompt := buildSurveyPrompt(tools)

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	if surveyHome != "" {
		merged.Picoclaw.Home = surveyHome
	}
	if surveyConfig != "" {
		merged.Picoclaw.Config = surveyConfig
	}
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(merged.Picoclaw.Home, merged.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	merged.Picoclaw.Home = resolvedHome
	merged.Picoclaw.Config = resolvedConfig

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}

	surveyAgent := pcwrap.EmbeddedAgent{
		ID:   "survey-analyst",
		Name: "Survey Analyst",
		Prompt: `按类别列出工具，每个工具一行，格式：工具名  一句话说明。

示例：
开发构建: go  Go语言编译器, cargo  Rust包管理器, make  构建工具, cmake  跨平台构建
云平台: docker  容器引擎, gcloud  Google Cloud CLI, kubectl  Kubernetes管理
网络工具: curl  HTTP客户端, nmap  网络扫描, ssh  远程连接
开发辅助: git  版本控制, jq  JSON处理, tmux  终端复用

直接开始，不要标题、不要总结、不要分段。`,
	}

	loading := startLLMLoading("AI agent analyzing", surveyDebug)
	defer loading.Stop()

	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        surveySession,
		Agent:          "survey-analyst",
		Model:          surveyModel,
		Workspace:      workspace,
		Debug:          surveyDebug,
		Quiet:          !surveyDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{surveyAgent},
		BeforeOutput:   loading.Stop,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	defer dr.Close()

	resp, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message: prompt,
		Session: surveySession,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}

	fmt.Fprintln(out, resp)
	return nil
}

type toolEntry struct {
	Name string
	Path string
}

// Common system utilities to exclude (hard filter)
var excludeBins = map[string]bool{
	// coreutils
	"[": true, "arch": true, "b2sum": true, "base32": true, "base64": true,
	"basename": true, "cat": true, "chcon": true, "chgrp": true, "chmod": true,
	"chown": true, "chroot": true, "cksum": true, "comm": true, "cp": true,
	"csplit": true, "cut": true, "date": true, "dd": true, "df": true,
	"dir": true, "dircolors": true, "dirname": true, "du": true, "echo": true,
	"env": true, "expand": true, "expr": true, "factor": true, "false": true,
	"fmt": true, "fold": true, "groups": true, "head": true, "hostid": true,
	"id": true, "install": true, "join": true, "link": true, "ln": true,
	"logname": true, "ls": true, "md5sum": true, "mkdir": true, "mkfifo": true,
	"mknod": true, "mktemp": true, "mv": true, "nice": true, "nl": true,
	"nohup": true, "nproc": true, "numfmt": true, "od": true, "paste": true,
	"pathchk": true, "pink": true, "pr": true, "printenv": true, "printf": true,
	"ptx": true, "pwd": true, "readlink": true, "realpath": true, "rm": true,
	"rmdir": true, "runcon": true, "seq": true, "sha1sum": true, "sha224sum": true,
	"sha256sum": true, "sha384sum": true, "sha512sum": true, "shred": true,
	"shuf": true, "sleep": true, "sort": true, "split": true, "stat": true,
	"stdbuf": true, "stty": true, "sum": true, "sync": true, "tac": true,
	"tail": true, "tee": true, "test": true, "timeout": true, "touch": true,
	"tr": true, "true": true, "truncate": true, "tsort": true, "tty": true,
	"uname": true, "unexpand": true, "uniq": true, "unlink": true, "uptime": true,
	"users": true, "vdir": true, "wc": true, "who": true, "whoami": true,
	"yes": true,
	// common system
	"security": true, "defaults": true, "osascript": true, "open": true,
	"pbcopy": true, "pbpaste": true, "say": true,
}

// Directories to skip entirely (exact base name match)
var excludeDirs = map[string]bool{
	"shims":        true,
	".bin":         true,
	"node_modules": true,
	".nvm":         true,
	".rbenv":       true,
	".pyenv":       true,
	".goenv":       true,
	".nodenv":      true,
	".cargo":       true,
	"Library":      true, // matches Library/Python etc.
}

// System directories to skip (unless --deep)
var systemDirs = []string{
	"/usr/bin", "/usr/sbin", "/sbin", "/bin",
	"/Library/Frameworks/Python.framework",
	"/opt/homebrew/sbin",
}

func scanPATH(deep bool) []toolEntry {
	pathEnv := os.Getenv("PATH")
	dirs := strings.Split(pathEnv, string(os.PathListSeparator))

	seen := make(map[string]bool)
	var tools []toolEntry

	for _, dir := range dirs {
		if shouldSkipDir(dir) {
			continue
		}
		if !deep && isSystemDir(dir) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}
			seen[name] = true

			if excludeBins[name] {
				continue
			}

			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue
			}

			tools = append(tools, toolEntry{Name: name, Path: fullPath})
		}
	}

	return tools
}

func shouldSkipDir(dir string) bool {
	// Check each path component against exclude list
	for {
		base := filepath.Base(dir)
		if base == "." || base == "/" {
			break
		}
		if excludeDirs[base] {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

func isSystemDir(dir string) bool {
	for _, sysDir := range systemDirs {
		if strings.HasPrefix(dir, sysDir) {
			return true
		}
	}
	return false
}

func buildSurveyPrompt(tools []toolEntry) string {
	var sb strings.Builder

	sb.WriteString("以下是这台机器上发现的CLI工具列表：\n\n")
	sb.WriteString(fmt.Sprintf("系统: %s/%s, %d CPUs\n\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()))

	// group tools by first letter
	currentLetter := ""
	for _, t := range tools {
		first := strings.ToUpper(t.Name[:1])
		if first != currentLetter {
			if currentLetter != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("[%s]\n", first))
			currentLetter = first
		}
		sb.WriteString(fmt.Sprintf("  %s\n", t.Name))
	}

	sb.WriteString("\n请分析这些工具，总结这台机器的能力。")
	return sb.String()
}
