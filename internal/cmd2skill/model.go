package cmd2skill

import "time"

type Options struct {
	TargetDir   string
	DryRun      bool
	Examples    bool
	Depth       int
	Markdown    bool
	Timeout     time.Duration
	MaxCommands int
}

type CLIModel struct {
	Name     string
	Root     *CommandNode
	Failures []DiscoveryFailure
}

type CommandNode struct {
	Name        string
	Path        []string
	Description string
	Usage       string
	Flags       []Flag
	Examples    []Example
	Children    []*CommandNode
	RawHelp     string
	Section     string
}

type Flag struct {
	Name        string
	Shorthand   string
	Type        string
	Description string
	Global      bool
}

type Example struct {
	Command string
	Desc    string
}

type DiscoveryFailure struct {
	Path  []string
	Error string
}
