package cmd

import formulacmd "github.com/sjzsdu/tt/cmd/formula"

var formulaCommandDeps formulacmd.Dependencies

func init() {
	formulaCommandDeps = formulacmd.Dependencies{
		Version: version,
		StartLLMLoading: func(message string, quiet bool) formulacmd.LoadingStatus {
			return startLLMLoading(message, quiet)
		},
		OpenBrowser: openBrowser,
		RunMarkdownPreview: func(options formulacmd.MarkdownPreviewOptions) error {
			mdRoot = options.Root
			mdContent = options.Content
			mdContentOnly = options.ContentOnly
			mdPort = options.Port
			mdInitialPath = options.InitialPath
			return runMarkdownServer()
		},
		UseTTAgentStorage:             useTTAgentStorage,
		EnsurePicoclawConfigAvailable: ensurePicoclawConfigAvailable,
		PicoclawUnavailableError:      picoclawUnavailableError,
	}
	rootCmd.AddCommand(formulacmd.New(formulaCommandDeps))
}
