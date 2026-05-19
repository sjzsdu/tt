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
	collectManifestEntrypoints(res.Path, p, opts)
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
			pf.Exports = collectPackageExports(m["exports"])
			pf.EntryHints = collectNPMEntryHints(m)
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
func collectPackageExports(v any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case string:
			if t != "" && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		case map[string]any:
			for k, item := range t {
				if isPublicExportKey(k) && !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
				walk(item)
			}
		}
	}
	walk(v)
	sort.Strings(out)
	return out
}
func isPublicExportKey(k string) bool {
	return k == "." || strings.HasPrefix(k, "./")
}

func collectNPMEntryHints(m map[string]any) []string {
	var out []string
	for _, key := range []string{"types", "typings", "module", "main", "browser"} {
		if v := strAny(m[key]); v != "" {
			out = append(out, v)
		}
	}
	for _, v := range collectPackageExports(m["exports"]) {
		if looksLikeSourcePath(v) {
			out = append(out, v)
		}
	}
	return cleanStrings(out)
}

func looksLikeSourcePath(v string) bool {
	v = strings.TrimSpace(v)
	return strings.HasSuffix(v, ".ts") || strings.HasSuffix(v, ".tsx") || strings.HasSuffix(v, ".js") || strings.HasSuffix(v, ".jsx") || strings.HasSuffix(v, ".mjs") || strings.HasSuffix(v, ".cjs") || strings.HasSuffix(v, ".d.ts") || strings.HasSuffix(v, ".py") || strings.HasSuffix(v, ".rs") || strings.HasSuffix(v, ".go")
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
	patterns := []string{`(?m)^export\s+(?:declare\s+)?(?:function|class|interface|type|const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)`, `(?m)^export\s*\{([^}]+)\}`, `(?m)^pub\s+(?:fn|struct|enum|trait|mod|type|const|static)\s+([A-Za-z_][A-Za-z0-9_]*)`, `(?m)^func\s+([A-Z][A-Za-z0-9_]*)\s*\(`, `(?m)^type\s+([A-Z][A-Za-z0-9_]*)\b`, `(?m)^(?:var|const)\s+(?:\([^)]*\b([A-Z][A-Za-z0-9_]*)\b|([A-Z][A-Za-z0-9_]*))`, `(?m)^class\s+([A-Z][A-Za-z0-9_]*)`, `(?m)^def\s+([a-zA-Z_][A-Za-z0-9_]*)\s*\(`, `(?m)__all__\s*=\s*\[([^\]]+)\]`}
	seen := map[string]bool{}
	for _, pat := range patterns {
		for _, m := range regexp.MustCompile(pat).FindAllStringSubmatch(s, 50) {
			for _, name := range exportedNamesFromMatch(firstNonEmptyCapture(m[1:])) {
				if seen[name] {
					continue
				}
				seen[name] = true
				out = append(out, APISymbol{Name: name, Kind: "symbol", Source: rel, Evidence: fmt.Sprintf("%s exports %s", rel, name)})
			}
		}
	}
	return out
}

func firstNonEmptyCapture(values []string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func exportedNamesFromMatch(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, "\"'"))
		if part == "" || strings.HasPrefix(part, "from ") {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if len(fields) >= 3 && fields[1] == "as" {
			name = fields[2]
		}
		name = strings.Trim(name, "\"'{}")
		if !strings.HasPrefix(name, "_") && regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
			out = append(out, name)
		}
	}
	return out
}

func collectManifestEntrypoints(root string, p *RepoProfile, opts Options) {
	seen := map[string]bool{}
	for _, ep := range p.EntryPoints {
		seen[filepath.ToSlash(ep.Path)] = true
	}
	for _, pf := range p.PackageFiles {
		candidates := append([]string{}, pf.EntryHints...)
		if pf.Ecosystem == "python" {
			pkg := strings.ReplaceAll(pf.Name, "-", "_")
			candidates = append(candidates, filepath.Join(pkg, "__init__.py"), filepath.Join("src", pkg, "__init__.py"))
		}
		if pf.Ecosystem == "rust" {
			candidates = append(candidates, "src/lib.rs")
		}
		for _, candidate := range candidates {
			candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "./")
			if candidate == "" || !looksLikeSourcePath(candidate) {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(candidate))
			info, err := os.Stat(path)
			if err != nil || info.IsDir() || info.Size() > opts.MaxFileSize {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if seen[rel] {
				continue
			}
			seen[rel] = true
			p.EntryPoints = append(p.EntryPoints, EntryPoint{Path: rel, Language: languageFor(rel), Kind: "manifest-entrypoint", Evidence: pf.Path})
			p.PublicAPIs = append(p.PublicAPIs, extractPublicSymbols(path, rel, strings.ToLower(rel))...)
		}
	}
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
	p.PublicAPIs = dedupeAPIs(p.PublicAPIs)
	p.InstallHints = cleanInstallHints(p.InstallHints)
	sort.Slice(p.Readmes, func(i, j int) bool { return p.Readmes[i].Path < p.Readmes[j].Path })
}
func dedupeAPIs(values []APISymbol) []APISymbol {
	seen := map[string]bool{}
	var out []APISymbol
	for _, api := range values {
		key := strings.ToLower(api.Name + "\x00" + api.Source)
		if api.Name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, api)
	}
	return out
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "@"))
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		return "repo"
	}
	return s
}
