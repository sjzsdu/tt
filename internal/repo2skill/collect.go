package repo2skill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func Collect(input string, opts Options) (*RepoProfile, func(), error) {
	if opts.Intent == "" {
		opts.Intent = "use-library"
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 200
	}
	if opts.MaxFileSize <= 0 {
		opts.MaxFileSize = 256 * 1024
	}
	res, cleanup, err := resolveRepo(input, opts)
	if err != nil {
		return nil, nil, err
	}
	p := &RepoProfile{Name: sanitizeName(res.Name), Source: res.Source, LocalPath: res.Path, Intent: opts.Intent}
	count := 0
	err = filepath.WalkDir(res.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(res.Path, path)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if count >= opts.MaxFiles {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > opts.MaxFileSize {
			return nil
		}
		lower := strings.ToLower(filepath.ToSlash(rel))
		switch {
		case isPackageFile(lower):
			if pf, ok := parsePackageFile(path, rel); ok {
				p.PackageFiles = append(p.PackageFiles, pf)
				count++
			}
		case isReadme(lower):
			if df, ok := readDoc(path, rel, opts.MaxFileSize); ok {
				p.Readmes = append(p.Readmes, df)
				p.UsageSnippets = append(p.UsageSnippets, extractSnippets(df)...)
				count++
			}
		case isDoc(lower):
			if df, ok := readDoc(path, rel, opts.MaxFileSize); ok {
				p.Docs = append(p.Docs, df)
				p.UsageSnippets = append(p.UsageSnippets, extractSnippets(df)...)
				count++
			}
		case isExample(lower):
			if cf, ok := readCode(path, rel, "example"); ok {
				p.Examples = append(p.Examples, cf)
				count++
			}
		case isTest(lower):
			if cf, ok := readCode(path, rel, "test"); ok {
				p.Tests = append(p.Tests, cf)
				count++
			}
		case isEntrypoint(lower):
			p.EntryPoints = append(p.EntryPoints, EntryPoint{Path: rel, Language: languageFor(lower), Kind: "entrypoint", Evidence: rel})
			if syms := extractPublicSymbols(path, rel, lower); len(syms) > 0 {
				p.PublicAPIs = append(p.PublicAPIs, syms...)
			}
			count++
		}
		return nil
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	postProcessProfile(p)
	return p, cleanup, nil
}

func skipDir(n string) bool {
	switch n {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__":
		return true
	}
	return false
}
func isPackageFile(p string) bool {
	b := filepath.Base(p)
	return b == "package.json" || b == "go.mod" || b == "pyproject.toml" || b == "cargo.toml" || b == "pom.xml" || b == "build.gradle" || strings.HasSuffix(b, ".csproj")
}
func isReadme(p string) bool { return strings.HasPrefix(filepath.Base(p), "readme") }
func isDoc(p string) bool {
	return strings.HasPrefix(p, "docs/") || strings.HasPrefix(p, "doc/") || strings.Contains(p, "/docs/") || strings.HasPrefix(filepath.Base(p), "changelog") || strings.HasPrefix(filepath.Base(p), "migration")
}
func isExample(p string) bool {
	return strings.Contains(p, "example") || strings.Contains(p, "demo") || strings.HasPrefix(p, "samples/")
}
func isTest(p string) bool {
	return strings.Contains(filepath.Base(p), "test") || strings.Contains(filepath.Base(p), "spec")
}
func isEntrypoint(p string) bool {
	b := filepath.Base(p)
	return b == "index.ts" || b == "index.js" || b == "mod.rs" || b == "lib.rs" || b == "__init__.py" || strings.HasSuffix(p, ".go")
}
func languageFor(p string) string {
	switch filepath.Ext(p) {
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".go":
		return "Go"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	}
	return ""
}

func parsePackageFile(path, rel string) (PackageFile, bool) {
	b := filepath.Base(strings.ToLower(rel))
	pf := PackageFile{Path: rel}
	data, _ := os.ReadFile(path)
	s := string(data)
	switch b {
	case "package.json":
		pf.Ecosystem = "npm"
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			pf.Name = strAny(m["name"])
			pf.Version = strAny(m["version"])
			pf.Description = strAny(m["description"])
			pf.Exports = collectJSONKeys(m["exports"])
			pf.Dependencies = collectJSONKeys(m["dependencies"])
		}
	case "go.mod":
		pf.Ecosystem = "go"
		pf.Name = firstRegex(s, `(?m)^module\s+(.+)$`)
	case "pyproject.toml":
		pf.Ecosystem = "python"
		pf.Name = firstRegex(s, `(?m)^name\s*=\s*["']([^"']+)`)
		pf.Description = firstRegex(s, `(?m)^description\s*=\s*["']([^"']+)`)
	case "cargo.toml":
		pf.Ecosystem = "rust"
		pf.Name = firstRegex(s, `(?m)^name\s*=\s*["']([^"']+)`)
		pf.Description = firstRegex(s, `(?m)^description\s*=\s*["']([^"']+)`)
	default:
		pf.Ecosystem = "jvm/dotnet"
	}
	return pf, true
}
func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func collectJSONKeys(v any) []string {
	var out []string
	if m, ok := v.(map[string]any); ok {
		for k := range m {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
func firstRegex(s, pat string) string {
	m := regexp.MustCompile(pat).FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readDoc(path, rel string, max int64) (DocFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DocFile{}, false
	}
	c := truncate(string(data), 12000)
	return DocFile{Path: rel, Title: firstHeading(c), Summary: firstParagraph(c), Content: c}, true
}
func readCode(path, rel, kind string) (CodeFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CodeFile{}, false
	}
	return CodeFile{Path: rel, Kind: kind, Summary: "", Content: truncate(string(data), 8000)}, true
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n..."
	}
	return s
}
func firstHeading(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}
func firstParagraph(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		return truncate(line, 300)
	}
	return ""
}

func extractSnippets(d DocFile) []Snippet {
	re := regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\\n(.*?)```")
	ms := re.FindAllStringSubmatch(d.Content, 8)
	out := []Snippet{}
	for _, m := range ms {
		code := strings.TrimSpace(m[1])
		if code != "" {
			out = append(out, Snippet{Source: d.Path, Code: truncate(code, 1500)})
		}
	}
	return out
}
func extractPublicSymbols(path, rel, lower string) []APISymbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := string(data)
	var out []APISymbol
	patterns := []string{`(?m)^export\s+(?:declare\s+)?(?:function|class|interface|type|const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)`, `(?m)^pub\s+(?:fn|struct|enum|trait|mod)\s+([A-Za-z_][A-Za-z0-9_]*)`, `(?m)^func\s+([A-Z][A-Za-z0-9_]*)\s*\(`, `(?m)^class\s+([A-Z][A-Za-z0-9_]*)`, `(?m)^def\s+([a-zA-Z_][A-Za-z0-9_]*)\s*\(`}
	for _, pat := range patterns {
		for _, m := range regexp.MustCompile(pat).FindAllStringSubmatch(s, 50) {
			out = append(out, APISymbol{Name: m[1], Kind: "symbol", Source: rel, Evidence: fmt.Sprintf("%s exports %s", rel, m[1])})
		}
	}
	return out
}

func postProcessProfile(p *RepoProfile) {
	seenLang := map[string]bool{}
	for _, ep := range p.EntryPoints {
		if ep.Language != "" && !seenLang[ep.Language] {
			p.Languages = append(p.Languages, ep.Language)
			seenLang[ep.Language] = true
		}
	}
	for _, pf := range p.PackageFiles {
		if pf.Name != "" {
			p.Name = sanitizeName(pf.Name)
			break
		}
	}
	for _, pf := range p.PackageFiles {
		if pf.Name == "" {
			continue
		}
		switch pf.Ecosystem {
		case "npm":
			p.InstallHints = append(p.InstallHints, "npm install "+pf.Name)
		case "go":
			p.InstallHints = append(p.InstallHints, "go get "+pf.Name)
		case "python":
			p.InstallHints = append(p.InstallHints, "pip install "+pf.Name)
		case "rust":
			p.InstallHints = append(p.InstallHints, "cargo add "+pf.Name)
		}
	}
	sort.Slice(p.Readmes, func(i, j int) bool { return p.Readmes[i].Path < p.Readmes[j].Path })
}
func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "@"))
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		return "repo"
	}
	return s
}
