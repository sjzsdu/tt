package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `Generate completion script for the current shell or the shell you specify.

If no shell is provided, tt tries to detect it from your environment.

To load completions:

Bash:

  $ source <(tt completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ tt completion bash > /etc/bash_completion.d/tt
  # macOS:
  $ tt completion bash > $(brew --prefix)/etc/bash_completion.d/tt

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ tt completion zsh > "${fpath[1]}/_tt"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ tt completion fish | source

  # To load completions for each session, execute once:
  $ tt completion fish > ~/.config/fish/completions/tt.fish

PowerShell:

  PS> tt completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> tt completion powershell > tt.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := ""
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = detectCompletionShell()
		}
		shell = normalizeCompletionShell(shell)
		if shell == "" {
			return fmt.Errorf("unable to detect current shell; run 'tt completion bash|zsh|fish|powershell' explicitly")
		}
		return runCompletionScript(cmd, shell)
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

func runCompletionScript(cmd *cobra.Command, shell string) error {
	switch shell {
	case "bash":
		cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell type %q", shell)
	}
	return nil
}

func detectCompletionShell() string {
	for _, key := range []string{"TT_COMPLETION_SHELL", "SHELL", "COMSPEC"} {
		if shell := normalizeCompletionShell(os.Getenv(key)); shell != "" {
			return shell
		}
	}
	return ""
}


func normalizeCompletionShell(shell string) string {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(shell))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "bash", "zsh", "fish", "powershell", "pwsh":
		if base == "pwsh" {
			return "powershell"
		}
		return base
	default:
		return ""
	}
}


