package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/spf13/cobra"
)

var (
	translateModel    string
	translateTarget   string
	translateLangFrom string
	translateLangTo   string
	translateSession  string
	translateDebug    bool
	translateHome     string
	translateConfig   string
	translateFile     []string
	translateDir      string
	translatePattern  string
	translateURL      string
	translateOutput   string
	translateOutputDir string
	translateFormat   string
	translateInPlace  bool
	translateGlossary string
	translatePreserve bool
	translateMaxSize  int64
	translateRender   bool
)

var translateCmd = &cobra.Command{
	Use:   "translate [text]",
	Short: "Translate text using the embedded picoclaw translation master",
	Long: `Translate Chinese to English or English to Chinese using an embedded
picoclaw translate-master agent configuration. Text can be provided as arguments,
via --message-like positional text, piped through stdin, from files, directories,
or URLs.

Input sources:
  - Direct text arguments
  - stdin (piped input)
  - Single/multiple files (--file)
  - Directory with glob pattern (--dir --pattern)
  - Web page content (--url)

Output formats:
  - text: plain text (default)
  - markdown: formatted markdown with metadata
  - json: structured output with source/target comparison`,
	Args: cobra.ArbitraryArgs,
	Example: `tt translate 你好，世界
	echo "Hello, world" | tt translate
	tt translate --target ja "你好，世界"
	tt translate --model gpt-5.4 "Improve developer productivity"
	tt translate --file docs/readme.md --output readme-zh.md
	tt translate --file "*.md" --output-dir ./zh/
	tt translate --dir ./docs --pattern "**/*.md" --output-dir ./docs-zh
	tt translate --url https://example.com/article --format markdown
	tt translate --file api.md --glossary glossary.json --preserve`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTranslate(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(translateCmd)
	translateCmd.Flags().StringVar(&translateTarget, "target", "", "target language override, such as zh, en, ja, ko, fr")
	translateCmd.Flags().StringVar(&translateLangFrom, "lang-from", "", "source language (auto-detect if empty)")
	translateCmd.Flags().StringVar(&translateLangTo, "lang-to", "", "target language (alias for --target)")
	translateCmd.Flags().StringVar(&translateModel, "model", "", "model to use; defaults to the picoclaw default model")
	translateCmd.Flags().StringVarP(&translateSession, "session", "s", "cli:translate", "session key")
	translateCmd.Flags().BoolVarP(&translateDebug, "debug", "d", false, "enable debug logging")
	translateCmd.Flags().StringVar(&translateHome, "picoclaw-home", "", "override PICOCLAW_HOME for this run")
	translateCmd.Flags().StringVar(&translateConfig, "picoclaw-config", "", "override PICOCLAW_CONFIG for this run")
	translateCmd.Flags().StringArrayVar(&translateFile, "file", nil, "file(s) to translate (repeatable)")
	translateCmd.Flags().StringVar(&translateDir, "dir", "", "directory to translate")
	translateCmd.Flags().StringVar(&translatePattern, "pattern", "**/*.md", "glob pattern for directory translation")
	translateCmd.Flags().StringVar(&translateURL, "url", "", "URL to fetch and translate")
	translateCmd.Flags().StringVarP(&translateOutput, "output", "o", "", "output file path")
	translateCmd.Flags().StringVar(&translateOutputDir, "output-dir", "", "output directory for batch translation")
	translateCmd.Flags().StringVar(&translateFormat, "format", "text", "output format: text, markdown, json")
	translateCmd.Flags().BoolVar(&translateInPlace, "in-place", false, "overwrite original files (requires confirmation)")
	translateCmd.Flags().StringVar(&translateGlossary, "glossary", "", "glossary file (JSON format: {\"term\": \"translation\"})")
	translateCmd.Flags().BoolVar(&translatePreserve, "preserve", false, "preserve markdown formatting in translation")
	translateCmd.Flags().Int64Var(&translateMaxSize, "max-size", 10*1024*1024, "max file size in bytes (default 10MB)")
	translateCmd.Flags().BoolVar(&translateRender, "render", false, "render JavaScript for SPA sites using headless Chrome")
}

func runTranslate(cmd *cobra.Command, args []string) error {
	target := translateTarget
	if translateLangTo != "" && target == "" {
		target = translateLangTo
	}

	inputs, err := collectTranslateInputs(cmd, args)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no text provided from arguments, stdin, files, directory, or URL")
	}

	glossary, err := loadGlossary(translateGlossary)
	if err != nil {
		return fmt.Errorf("load glossary failed: %w", err)
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

	results := make([]translateResult, 0, len(inputs))
	for _, input := range inputs {
		result, err := translateSingle(rt, input, target, glossary)
		if err != nil {
			return fmt.Errorf("translate %q failed: %w", input.Source, err)
		}
		results = append(results, *result)
	}

	return outputTranslateResults(cmd, results)
}

type translateInput struct {
	Text     string
	Source   string
	FilePath string
}

type translateResult struct {
	Input      translateInput
	Original   string
	Translated string
}

func collectTranslateInputs(cmd *cobra.Command, args []string) ([]translateInput, error) {
	var inputs []translateInput

	if len(args) > 0 {
		text := strings.TrimSpace(strings.Join(args, " "))
		if text != "" {
			inputs = append(inputs, translateInput{Text: text, Source: "args"})
		}
	}

	for _, filePath := range translateFile {
		text, err := readFileContent(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file %q failed: %w", filePath, err)
		}
		absPath, _ := filepath.Abs(filePath)
		inputs = append(inputs, translateInput{Text: text, Source: filePath, FilePath: absPath})
	}

	if translateDir != "" {
		files, err := globFiles(translateDir, translatePattern)
		if err != nil {
			return nil, fmt.Errorf("glob directory %q failed: %w", translateDir, err)
		}
		for _, filePath := range files {
			text, err := readFileContent(filePath)
			if err != nil {
				return nil, fmt.Errorf("read file %q failed: %w", filePath, err)
			}
			inputs = append(inputs, translateInput{Text: text, Source: filePath, FilePath: filePath})
		}
	}

	if translateURL != "" {
		text, err := fetchURLContent(translateURL, translateRender)
		if err != nil {
			return nil, fmt.Errorf("fetch URL %q failed: %w", translateURL, err)
		}
		inputs = append(inputs, translateInput{Text: text, Source: translateURL})
	}

	if len(inputs) == 0 {
		text, err := readStdin(cmd)
		if err != nil {
			return nil, err
		}
		if text != "" {
			inputs = append(inputs, translateInput{Text: text, Source: "stdin"})
		}
	}

	return inputs, nil
}

func readFileContent(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > translateMaxSize {
		return "", fmt.Errorf("file too large (%d bytes > %d limit)", info.Size(), translateMaxSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func globFiles(dir, pattern string) ([]string, error) {
	if !strings.HasPrefix(pattern, "**") {
		pattern = "**/" + pattern
	}

	fullPattern := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if info.IsDir() || strings.HasPrefix(filepath.Base(match), ".") {
			continue
		}
		files = append(files, match)
	}
	return files, nil
}

func fetchURLContent(url string, render bool) (string, error) {
	if render {
		return fetchURLWithRender(url)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, translateMaxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
	}

	text := string(data)
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		text = stripHTMLTags(text)
	}

	return strings.TrimSpace(text), nil
}

func fetchURLWithRender(url string) (string, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(nil))
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var text string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second),
		chromedp.Text("body", &text, chromedp.BySearch),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp render failed: %w", err)
	}

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n\n\n", "\n\n")

	return text, nil
}

func stripHTMLTags(html string) string {
	result := html
	result = strings.ReplaceAll(result, "<script>", "")
	result = strings.ReplaceAll(result, "</script>", "")
	result = strings.ReplaceAll(result, "<style>", "")
	result = strings.ReplaceAll(result, "</style>", "")

	var output strings.Builder
	inTag := false
	for _, r := range result {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			output.WriteRune(r)
		}
	}

	text := output.String()
	text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	return strings.TrimSpace(text)
}

func readStdin(cmd *cobra.Command) (string, error) {
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

func loadGlossary(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	glossary := make(map[string]string)
	if err := json.Unmarshal(data, &glossary); err != nil {
		return nil, fmt.Errorf("parse glossary JSON failed: %w", err)
	}

	return glossary, nil
}

func translateSingle(rt *pcwrap.Runtime, input translateInput, target string, glossary map[string]string) (*translateResult, error) {
	text := input.Text

	if glossary != nil {
		text = applyGlossary(text, glossary)
	}

	message := buildTranslateMessage(text, target, translateLangFrom, translatePreserve)

	loading := startLLMLoading(fmt.Sprintf("正在翻译 %s", input.Source), translateDebug)
	defer loading.Stop()

	translateMaster, err := agents.TranslateMaster()
	if err != nil {
		loading.Stop()
		return nil, fmt.Errorf("load translate master agent failed: %w", err)
	}

	dr, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Session:        translateSession,
		Agent:          agents.TranslateMasterID,
		Model:          translateModel,
		Debug:          translateDebug,
		Quiet:          !translateDebug,
		EmbeddedAgents: []pcwrap.EmbeddedAgent{translateMaster},
		BeforeOutput:   loading.Stop,
	})
	if err != nil {
		return nil, err
	}
	defer dr.Close()

	translated, err := dr.ProcessDirect(pcwrap.RunOptions{
		Message: message,
		Session: translateSession,
	})
	if err != nil {
		return nil, err
	}

	return &translateResult{
		Input:      input,
		Original:   input.Text,
		Translated: translated,
	}, nil
}

func applyGlossary(text string, glossary map[string]string) string {
	for term, translation := range glossary {
		re := strings.NewReplacer(term, translation, strings.ToLower(term), translation, strings.Title(term), translation)
		text = re.Replace(text)
	}
	return text
}

func buildTranslateMessage(text, target, langFrom string, preserve bool) string {
	text = strings.TrimSpace(text)
	target = strings.TrimSpace(target)
	langFrom = strings.TrimSpace(langFrom)

	var sb strings.Builder

	if target != "" {
		sb.WriteString(fmt.Sprintf("请将以下内容翻译成%s，只输出译文：\n\n", target))
	} else if langFrom != "" {
		sb.WriteString(fmt.Sprintf("源语言为%s，请翻译成最合适的语言，只输出译文：\n\n", langFrom))
	} else {
		sb.WriteString("请翻译以下内容，只输出译文：\n\n")
	}

	if preserve {
		sb.WriteString("[请保留原文的 Markdown 格式标记，包括标题、链接、代码块等]\n\n")
	}

	sb.WriteString(text)
	return sb.String()
}

func outputTranslateResults(cmd *cobra.Command, results []translateResult) error {
	out := cmd.OutOrStdout()

	format := translateFormat
	if format == "" {
		format = "text"
	}

	if translateOutput != "" && len(results) == 1 {
		return outputSingleFile(results[0], format)
	}

	if translateOutputDir != "" {
		return outputBatchDirectory(results, format)
	}

	if translateInPlace {
		return outputInPlace(results)
	}

	switch format {
	case "json":
		return outputJSON(results, out)
	case "markdown":
		return outputMarkdown(results, out)
	default:
		return outputText(results, out)
	}
}

func outputSingleFile(result translateResult, format string) error {
	content, err := formatResult(result, format)
	if err != nil {
		return err
	}
	return os.WriteFile(translateOutput, []byte(content), 0644)
}

func outputBatchDirectory(results []translateResult, format string) error {
	if err := os.MkdirAll(translateOutputDir, 0755); err != nil {
		return err
	}

	for _, result := range results {
		var outPath string
		if result.Input.FilePath != "" {
			relPath, err := filepath.Rel(translateDir, result.Input.FilePath)
			if err != nil {
				relPath = filepath.Base(result.Input.FilePath)
			}
			outPath = filepath.Join(translateOutputDir, relPath)
		} else {
			outPath = filepath.Join(translateOutputDir, fmt.Sprintf("translation-%d.%s", time.Now().UnixNano(), formatExt(format)))
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}

		content, err := formatResult(result, format)
		if err != nil {
			return err
		}

		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func outputInPlace(results []translateResult) error {
	if !translateInPlace {
		return fmt.Errorf("--in-place requires explicit confirmation")
	}

	for _, result := range results {
		if result.Input.FilePath == "" {
			continue
		}

		content, err := formatResult(result, "text")
		if err != nil {
			return err
		}

		if err := os.WriteFile(result.Input.FilePath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func outputText(results []translateResult, w io.Writer) error {
	for i, result := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, result.Translated)
	}
	return nil
}

func outputMarkdown(results []translateResult, w io.Writer) error {
	fmt.Fprintln(w, "# Translation Results")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Translated at: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	for i, result := range results {
		fmt.Fprintf(w, "## %d. %s\n\n", i+1, result.Input.Source)
		fmt.Fprintln(w, "### Original")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w, result.Original)
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "### Translation")
		fmt.Fprintln(w)
		fmt.Fprintln(w, result.Translated)
		fmt.Fprintln(w)
	}

	return nil
}

func outputJSON(results []translateResult, w io.Writer) error {
	type jsonResult struct {
		Source     string `json:"source"`
		Original   string `json:"original"`
		Translated string `json:"translated"`
	}

	jsonResults := make([]jsonResult, len(results))
	for i, r := range results {
		jsonResults[i] = jsonResult{
			Source:     r.Input.Source,
			Original:   r.Original,
			Translated: r.Translated,
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

func formatResult(result translateResult, format string) (string, error) {
	switch format {
	case "json":
		data, err := json.Marshal(struct {
			Source     string `json:"source"`
			Original   string `json:"original"`
			Translated string `json:"translated"`
		}{
			Source:     result.Input.Source,
			Original:   result.Original,
			Translated: result.Translated,
		})
		return string(data), err
	case "markdown":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", result.Input.Source))
		sb.WriteString("## Original\n\n")
		sb.WriteString("```\n")
		sb.WriteString(result.Original)
		sb.WriteString("\n```\n\n")
		sb.WriteString("## Translation\n\n")
		sb.WriteString(result.Translated)
		sb.WriteString("\n")
		return sb.String(), nil
	default:
		return result.Translated, nil
	}
}

func formatExt(format string) string {
	switch format {
	case "json":
		return "json"
	case "markdown":
		return "md"
	default:
		return "txt"
	}
}
