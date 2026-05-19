package repo2skill

import "time"

type Options struct {
	TargetDir   string
	DryRun      bool
	Markdown    bool
	Intent      string
	Language    string
	MaxFiles    int
	MaxFileSize int64
	Timeout     time.Duration
	KeepTemp    bool
	Analyzer    Analyzer
}

type RepoProfile struct {
	Name          string
	Source        string
	LocalPath     string
	Intent        string
	Languages     []string
	PackageFiles  []PackageFile
	Readmes       []DocFile
	Docs          []DocFile
	Examples      []CodeFile
	Tests         []CodeFile
	EntryPoints   []EntryPoint
	PublicAPIs    []APISymbol
	InstallHints  []string
	UsageSnippets []Snippet
	Warnings      []string
}

type PackageFile struct {
	Path         string
	Ecosystem    string
	Name         string
	Version      string
	Description  string
	Exports      []string
	Dependencies []string
}

type DocFile struct {
	Path    string
	Title   string
	Summary string
	Content string
}

type CodeFile struct {
	Path    string
	Kind    string
	Summary string
	Content string
}

type EntryPoint struct {
	Path     string
	Language string
	Kind     string
	Evidence string
}

type APISymbol struct {
	Name     string
	Kind     string
	Source   string
	Evidence string
}

type Snippet struct {
	Source string
	Code   string
}

type SkillModel struct {
	Profile       *RepoProfile
	Purpose       string
	WhenToUse     []string
	WhenNotToUse  []string
	Install       []string
	PublicAPI     []APISymbol
	Recipes       []Recipe
	BestPractices []string
	Gotchas       []string
}

type Recipe struct {
	Title       string
	Description string
	Example     string
	Evidence    []string
}
