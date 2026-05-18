package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

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
	loading := startLLMLoading("正在翻译", translateDebug)
	defer loading.Stop()
	if err := rt.Run(pcwrap.RunOptions{
		Message:        message,
		Session:        translateSession,
		Agent:          agents.TranslateMasterID,
		Model:          translateModel,
		Debug:          translateDebug,
		Quiet:          !translateDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{agents.TranslateMaster()},
		BeforeOutput:   loading.Stop,
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
