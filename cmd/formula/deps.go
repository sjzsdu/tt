package formulacmd

import "fmt"

type LoadingStatus interface {
	Stop()
}

type MarkdownPreviewOptions struct {
	Root        string
	Content     string
	ContentOnly bool
	Port        int
	InitialPath string
}

type Dependencies struct {
	Version                       string
	StartLLMLoading               func(message string, quiet bool) LoadingStatus
	OpenBrowser                   func(url string)
	RunMarkdownPreview            func(options MarkdownPreviewOptions) error
	UseTTAgentStorage             func(home, configPath string) (workspace, resolvedHome, resolvedConfig string, restore func(), err error)
	EnsurePicoclawConfigAvailable func(home, configPath string) error
	PicoclawUnavailableError      func(err error, home, configPath string) error
}

var formulaDeps Dependencies
var version string

type noopLoadingStatus struct{}

func (noopLoadingStatus) Stop() {}

func configureDependencies(deps Dependencies) {
	formulaDeps = deps
	version = deps.Version
}

func startLLMLoading(message string, quiet bool) LoadingStatus {
	if formulaDeps.StartLLMLoading != nil {
		return formulaDeps.StartLLMLoading(message, quiet)
	}
	return noopLoadingStatus{}
}

func openBrowser(url string) {
	if formulaDeps.OpenBrowser != nil {
		formulaDeps.OpenBrowser(url)
	}
}

func runMarkdownPreview(options MarkdownPreviewOptions) error {
	if formulaDeps.RunMarkdownPreview != nil {
		return formulaDeps.RunMarkdownPreview(options)
	}
	return fmt.Errorf("markdown preview dependency is not configured")
}

func useTTAgentStorage(home, configPath string) (workspace, resolvedHome, resolvedConfig string, restore func(), err error) {
	if formulaDeps.UseTTAgentStorage != nil {
		return formulaDeps.UseTTAgentStorage(home, configPath)
	}
	return "", home, configPath, func() {}, nil
}

func ensurePicoclawConfigAvailable(home, configPath string) error {
	if formulaDeps.EnsurePicoclawConfigAvailable != nil {
		return formulaDeps.EnsurePicoclawConfigAvailable(home, configPath)
	}
	return nil
}

func picoclawUnavailableError(err error, home, configPath string) error {
	if formulaDeps.PicoclawUnavailableError != nil {
		return formulaDeps.PicoclawUnavailableError(err, home, configPath)
	}
	return fmt.Errorf("picoclaw unavailable: %w", err)
}
