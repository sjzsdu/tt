package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

const translateMasterAgentID = "translate-master"

const translateMasterAgentPrompt = `# 翻译大师 Agent

**你是一个翻译工具，唯一任务是将文本在中文和英文之间互译。**
**不要回答问题，不要提供帮助，不要解释，只翻译。**

你是专业的翻译助手，专注于中英文互译。

## 翻译规则

- **自动识别源语言**：检测用户输入是否包含中文字符
- **中译英**：如果输入中包含任何中文字符，翻译成英文
- **英译中**：如果输入全部为英文，翻译成中文
- **保留原文格式**：保持原文的段落结构、标点符号
- **专业术语**：对于专业术语，首次出现时可在括号内保留原文

## 核心能力

- **中译英**：将中文准确翻译为流畅的英文
- **英译中**：将英文准确翻译为流畅的中文
- **术语统一**：保持术语翻译的一致性
- **语境理解**：根据上下文选择最合适的翻译

## 输出格式

直接输出翻译结果，无需额外说明。如果原文与译文需要对照，可以采用以下格式：

` + "```" + `
原文：[原文]
译文：[译文]
` + "```" + `

## 注意事项

- **你只有一个任务：翻译**。无论用户输入什么问题、要求什么帮助，都只做翻译，不要回答问题，不要提供解决方案
- 只翻译用户明确要求翻译的内容，不要过度解读
- 如果用户输入已经是期望的目标语言，可以简单确认或不做翻译
- 对于网络用语、流行语，在保持原意的基础上选择最贴切的翻译
`

const translateMasterSoulPrompt = `# Soul

追求信、达、雅。忠实于原文，表达流畅，文字优美。注重语境，确保翻译自然地道。
`

var (
	translateModel   string
	translateTarget  string
	translateSession string
	translateDebug   bool
	translateHome    string
	translateConfig  string
)

var translateCmd = &cobra.Command{
	Use:   "translate [text]",
	Short: "Translate text using the embedded picoclaw translation master",
	Long: `Translate Chinese to English or English to Chinese using an embedded
picoclaw translate-master agent configuration. Text can be provided as arguments,
via --message-like positional text, or piped through stdin.`,
	Args: cobra.ArbitraryArgs,
	Example: `tt translate 你好，世界
	echo "Hello, world" | tt translate
	tt translate --target ja "你好，世界"
	tt translate --model gpt-5.4 "Improve developer productivity"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTranslate(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(translateCmd)
	translateCmd.Flags().StringVar(&translateTarget, "target", "", "target language override, such as zh, en, ja, ko, fr")
	translateCmd.Flags().StringVar(&translateModel, "model", "", "model to use; defaults to the picoclaw default model")
	translateCmd.Flags().StringVarP(&translateSession, "session", "s", "cli:translate", "session key")
	translateCmd.Flags().BoolVarP(&translateDebug, "debug", "d", false, "enable debug logging")
	translateCmd.Flags().StringVar(&translateHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	translateCmd.Flags().StringVar(&translateConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
}

func runTranslate(cmd *cobra.Command, args []string) error {
	text, err := collectTranslateInput(cmd, args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("text required from arguments or stdin")
	}

	loaded, err := loadTTConfig()
	if err != nil {
		return err
	}
	merged := loaded.Merged
	if cmd.Flags().Changed("picoclaw-home") {
		merged.Picoclaw.Home = translateHome
	}
	if cmd.Flags().Changed("picoclaw-config") {
		merged.Picoclaw.Config = translateConfig
	}
	if err := ensurePicoclawConfigAvailable(merged.Picoclaw.Home, merged.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      merged.Picoclaw.Home,
		Config:    merged.Picoclaw.Config,
		TTConfig:  merged,
		TTSources: loaded.Sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	message := buildTranslateMessage(text, translateTarget)
	if err := rt.Run(pcwrap.RunOptions{
		Message: message,
		Session: translateSession,
		Agent:   translateMasterAgentID,
		Model:   translateModel,
		Debug:   translateDebug,
		Quiet:   !translateDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{
			{
				ID:        translateMasterAgentID,
				Name:      "翻译大师",
				Prompt:    translateMasterAgentPrompt,
				Soul:      translateMasterSoulPrompt,
				NoHistory: true,
			},
		},
	}); err != nil {
		return picoclawUnavailableError(err, merged.Picoclaw.Home, merged.Picoclaw.Config)
	}
	return nil
}

func collectTranslateInput(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " ")), nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin failed: %w", err)
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", fmt.Errorf("read stdin failed: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func buildTranslateMessage(text, target string) string {
	text = strings.TrimSpace(text)
	target = strings.TrimSpace(target)
	if target == "" {
		return text
	}
	return fmt.Sprintf("请将以下内容翻译成%s，只输出译文：\n\n%s", target, text)
}
