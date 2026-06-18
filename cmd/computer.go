package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

var (
	computerModel   string
	computerSession string
	computerDebug   bool
	computerHome    string
	computerConfig  string
	computerDeep    bool
	computerMax     int
)

var computerCmd = &cobra.Command{
	Use:   "computer",
	Short: "Show machine profile: system info, installed tools, env vars, and AI recommendations",
	Long: `Computer scans your system to show:
  - System info (OS, CPU, memory, disk)
  - Important environment variables
  - Installed CLI tools (categorized by AI)
  - AI-powered recommendations for modern tools you might be missing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputer(cmd)
	},
}

type toolEntry struct {
	Name string
	Path string
}

func init() {
	rootCmd.AddCommand(computerCmd)
	computerCmd.Flags().StringVar(&computerModel, "model", "", "model to use; defaults to picoclaw config default")
	computerCmd.Flags().StringVarP(&computerSession, "session", "s", "cli:computer", "session key")
	computerCmd.Flags().BoolVarP(&computerDebug, "debug", "d", false, "enable debug logging")
	computerCmd.Flags().StringVar(&computerHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	computerCmd.Flags().StringVar(&computerConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	computerCmd.Flags().BoolVar(&computerDeep, "deep", false, "include system utilities in scan")
	computerCmd.Flags().IntVar(&computerMax, "max", 0, "max tools to send to AI (0=all)")
}

func runComputer(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	// 1. System info
	printSystemInfo(out)

	// 2. Environment variables
	printEnvVars(out)

	// 3. Scan tools
	fmt.Fprintln(out, "Scanning PATH...")
	tools := scanComputerPATH(computerDeep)
	total := len(tools)
	toSend := tools
	if computerMax > 0 && total > computerMax {
		toSend = tools[:computerMax]
	}
	fmt.Fprintf(out, "Found %d tools\n\n", total)

	// 4. Build prompt with all raw data, let agent categorize and analyze
	prompt := buildComputerPrompt(tools, toSend)

	// 5. AI analysis and recommendations
	if err := printAIAnalysis(cmd, prompt); err != nil {
		fmt.Fprintf(out, "\nAI analysis unavailable: %v\n", err)
	}

	return nil
}

func printSystemInfo(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprintln(out, "=== 系统信息 ===")
	fmt.Fprintf(out, "OS:       %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "CPUs:     %d\n", runtime.NumCPU())

	if mem := getMemoryInfo(); mem != "" {
		fmt.Fprintf(out, "Memory:   %s\n", mem)
	}
	if disk := getDiskUsage(); disk != "" {
		fmt.Fprintf(out, "Disk:     %s\n", disk)
	}
	fmt.Fprintln(out)
}

func getMemoryInfo() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").CombinedOutput()
		if err != nil {
			return ""
		}
		var bytes int64
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &bytes)
		return fmt.Sprintf("%.0f GB", float64(bytes)/(1024*1024*1024))
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			fmt.Sscanf(strings.TrimPrefix(line, "MemTotal:"), "%d kB", &kb)
			return fmt.Sprintf("%.0f GB", float64(kb)/(1024*1024))
		}
	}
	return ""
}

func getDiskUsage() string {
	out, err := exec.Command("df", "-h", ".").CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) >= 5 {
		return fmt.Sprintf("%s total, %s used, %s avail (%s)", fields[1], fields[2], fields[3], fields[4])
	}
	return ""
}

func printEnvVars(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprintln(out, "=== 环境变量 ===")

	// Just print all env vars, let agent decide what's important
	envs := os.Environ()
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]

		// Truncate PATH-like values
		if key == "PATH" {
			parts := strings.Split(val, string(os.PathListSeparator))
			if len(parts) > 5 {
				val = fmt.Sprintf("(%d dirs) %s ... %s", len(parts), strings.Join(parts[:2], ":"), parts[len(parts)-1])
			}
		}
		if len(val) > 80 {
			val = val[:77] + "..."
		}
		fmt.Fprintf(out, "%-30s %s\n", key+"=", val)
	}
	fmt.Fprintln(out)
}

func buildComputerPrompt(allTools []toolEntry, toSend []toolEntry) string {
	var sb strings.Builder

	sb.WriteString("=== 机器原始数据 ===\n\n")
	sb.WriteString(fmt.Sprintf("系统: %s/%s, %d CPUs\n\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()))

	// List all tool names
	sb.WriteString("已装工具 (")
	sb.WriteString(fmt.Sprintf("%d", len(allTools)))
	sb.WriteString("):\n")
	names := make([]string, len(toSend))
	for i, t := range toSend {
		names[i] = t.Name
	}
	// Group into lines of ~80 chars
	line := ""
	for _, name := range names {
		if len(line)+len(name)+1 > 76 {
			sb.WriteString(line + "\n")
			line = name
		} else {
			if line != "" {
				line += " "
			}
			line += name
		}
	}
	if line != "" {
		sb.WriteString(line + "\n")
	}

	sb.WriteString(`
请完成以下任务:

1. 按功能领域分组列出工具（开发语言、包管理、构建、版本控制、容器、云、网络、数据库、AI/ML、开发辅助、文本处理、安全等）
   格式：类别名: tool1 tool2 tool3

2. 给出5-10条实用建议，推荐用户可能缺的现代热门工具
   格式：你有X，但没装Y（简短说明）

先输出分组，再输出建议。`)

	return sb.String()
}

// PATH scanning - only filter truly useless system noise
var computerExcludeBins = map[string]bool{
	// coreutils that add zero signal
	"[": true, "arch": true, "base32": true, "base64": true,
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
	"pathchk": true, "pr": true, "printenv": true, "printf": true,
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
	// macOS system noise
	"security": true, "defaults": true, "osascript": true, "open": true,
	"pbcopy": true, "pbpaste": true, "say": true,
}

var computerExcludeDirs = map[string]bool{
	"shims": true, ".bin": true, "node_modules": true,
	".nvm": true, ".rbenv": true, ".pyenv": true,
	".goenv": true, ".nodenv": true, ".cargo": true,
	"Library": true,
}

var computerSystemDirs = []string{
	"/usr/bin", "/usr/sbin", "/sbin", "/bin",
	"/Library/Frameworks/Python.framework",
	"/opt/homebrew/sbin",
}

func scanComputerPATH(deep bool) []toolEntry {
	pathEnv := os.Getenv("PATH")
	dirs := strings.Split(pathEnv, string(os.PathListSeparator))

	seen := make(map[string]bool)
	var tools []toolEntry

	for _, dir := range dirs {
		if computerShouldSkipDir(dir) {
			continue
		}
		if !deep && computerIsSystemDir(dir) {
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
			if seen[name] || computerExcludeBins[name] {
				continue
			}
			seen[name] = true

			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil || info.Mode()&0111 == 0 {
				continue
			}

			tools = append(tools, toolEntry{Name: name, Path: fullPath})
		}
	}

	return tools
}

func computerShouldSkipDir(dir string) bool {
	for {
		base := filepath.Base(dir)
		if base == "." || base == "/" {
			break
		}
		if computerExcludeDirs[base] {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

func computerIsSystemDir(dir string) bool {
	for _, sysDir := range computerSystemDirs {
		if strings.HasPrefix(dir, sysDir) {
			return true
		}
	}
	return false
}

func printAIAnalysis(cmd *cobra.Command, prompt string) error {
	out := cmd.OutOrStdout()

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	if computerHome != "" {
		merged.Picoclaw.Home = computerHome
	}
	if computerConfig != "" {
		merged.Picoclaw.Config = computerConfig
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
		return err
	}

	advisor := pcwrap.EmbeddedAgent{
		ID:   "computer-advisor",
		Name: "Computer Advisor",
		Prompt: `你是一个开发者工具专家。根据提供的机器数据，完成两个任务：
1. 按功能领域分组列出已装工具
2. 推荐实用的现代工具

输出格式：
先按类别列出工具，每个类别一行，格式：类别名: tool1 tool2 tool3
然后输出建议，格式：你有X，但没装Y（说明）

保持简洁，直接开始，不要标题。`,
	}

	loading := startLLMLoading("AI analyzing", computerDebug)
	defer loading.Stop()

	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        computerSession,
		Agent:          "computer-advisor",
		Model:          computerModel,
		Workspace:      workspace,
		Debug:          computerDebug,
		Quiet:          !computerDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{advisor},
		BeforeOutput:   loading.Stop,
	})
	if err != nil {
		return err
	}
	defer dr.Close()

	resp, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message: prompt,
		Session: computerSession,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "=== AI 分析 ===")
	fmt.Fprintln(out, resp)
	return nil
}
