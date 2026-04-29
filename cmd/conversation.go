package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	mdutil "tt/internal/mdutil"
)

type convDump struct {
	Conversation convConversation `json:"conversation"`
}

type convConversation struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Context convContext `json:"context"`
}

type convContext struct {
	ConversationID string        `json:"conversation_id"`
	Messages       []convMessage `json:"messages"`
}

type convMessage struct {
	Text  *convText  `json:"text,omitempty"`
	Tool  *convTool  `json:"tool,omitempty"`
	Usage *convUsage `json:"usage,omitempty"`
}

type convText struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Model     string         `json:"model,omitempty"`
	ToolCalls []convToolCall `json:"tool_calls,omitempty"`
}

type convTool struct {
	Name    string         `json:"name"`
	CallID  string         `json:"call_id,omitempty"`
	Content string         `json:"content,omitempty"`
	Output  *convToolValue `json:"output,omitempty"`
}

type convToolCall struct {
	Name      string          `json:"name"`
	CallID    string          `json:"call_id,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type convToolValue struct {
	IsError bool              `json:"is_error"`
	Values  []json.RawMessage `json:"values,omitempty"`
}

type convUsage struct {
	PromptTokens     convUsageMetric `json:"prompt_tokens"`
	CompletionTokens convUsageMetric `json:"completion_tokens"`
	TotalTokens      convUsageMetric `json:"total_tokens"`
	CachedTokens     convUsageMetric `json:"cached_tokens"`
}

type convUsageMetric struct {
	Actual int `json:"actual"`
}

type convListItem struct {
	Name         string
	Path         string
	Relative     string
	Title        string
	Messages     int
	Conversation string
}

type convEntry struct {
	ID          string
	Kind        string
	Role        string
	Title       string
	Subtitle    string
	Body        template.HTML
	Meta        []string
	Usage       []string
	Accent      string
	CallID      string
	RoundLabel  string
	RoundAnchor string
}

type convViewData struct {
	FilePath       string
	Title          string
	ConversationID string
	TotalMessages  int
	Entries        []convEntry
	Files          []convListItem
	RawPath        string
}

var (
	convPort     = 9680
	convFile     string
	convPatterns []string

	convRoot   string
	convServer *http.Server
	convMu     sync.Mutex

	markupAttrPattern = regexp.MustCompile(`([A-Za-z0-9_:-]+)="([^"]*)"`)
)

var conversationCmd = &cobra.Command{
	Use:   "conversation [files...]",
	Short: "Browse conversation dump JSON in a local web UI",
	Long:  "Start a local web service for browsing conversation dump JSON files in the current working tree.",
	RunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := loadTTConfig()
		if err != nil {
			return err
		}
		merged := loaded.Merged
		if !cmd.Flags().Changed("port") && merged.Conversation.Port != nil {
			convPort = *merged.Conversation.Port
		}
		if !cmd.Flags().Changed("file") && strings.TrimSpace(merged.Conversation.File) != "" {
			convFile = merged.Conversation.File
		}
		flagPatterns, _ := cmd.Flags().GetStringSlice("pattern")
		convPatterns = append([]string{}, flagPatterns...)
		if !cmd.Flags().Changed("pattern") && len(merged.Conversation.Patterns) > 0 {
			convPatterns = append(convPatterns, merged.Conversation.Patterns...)
		}
		if len(args) > 0 {
			convPatterns = append([]string{}, args...)
		}
		convRoot = projectRootFromConfig(loaded)
		return runConversationServer()
	},
}

func init() {
	rootCmd.AddCommand(conversationCmd)
	conversationCmd.Flags().IntVarP(&convPort, "port", "p", 9680, "service port")
	conversationCmd.Flags().StringVarP(&convFile, "file", "f", "", "open a specific conversation dump JSON file")
	conversationCmd.Flags().StringSliceVar(&convPatterns, "pattern", []string{}, "filter conversation dump files by glob patterns")
}

func runConversationServer() error {
	convMu.Lock()
	defer convMu.Unlock()
	if convServer != nil {
		return fmt.Errorf("conversation service already running on port %d", convPort)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConversationIndex)
	mux.HandleFunc("/list", handleConversationList)
	mux.HandleFunc("/view/", handleConversationView)
	mux.HandleFunc("/raw/", handleConversationRaw)

	maxPort := convPort + 20
	var lastErr error
	for port := convPort; port <= maxPort; port++ {
		convServer = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		serverErr := make(chan error, 1)
		go func() {
			err := convServer.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()
		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-serverErr:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				convServer = nil
				continue
			}
			convServer = nil
			return err
		default:
			convPort = port
			fmt.Printf("Conversation service started: http://localhost:%d\n", port)
			go openConversationBrowser(fmt.Sprintf("http://localhost:%d", port))
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt)
			<-quit
			fmt.Println("\nShutting down conversation service...")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := convServer.Shutdown(ctx)
			convServer = nil
			return err
		}
	}
	return fmt.Errorf("all candidate ports unavailable: %v", lastErr)
}

func handleConversationIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if strings.TrimSpace(convFile) != "" {
		path := strings.TrimSpace(convFile)
		if !filepath.IsAbs(path) {
			path = filepath.Join(convRoot, path)
		}
		rel, err := filepath.Rel(convRoot, path)
		if err == nil {
			http.Redirect(w, r, "/view/"+filepath.ToSlash(rel), http.StatusFound)
			return
		}
	}
	handleConversationList(w, r)
}

func handleConversationList(w http.ResponseWriter, r *http.Request) {
	files, err := collectConversationFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect conversation files failed: %v", err), http.StatusInternalServerError)
		return
	}
	data := struct {
		Files []convListItem
		Total int
	}{Files: files, Total: len(files)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("conversation-list").Parse(conversationListHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render conversation list failed: %v", err), http.StatusInternalServerError)
	}
}

func handleConversationView(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/view/")
	if strings.TrimSpace(relPath) == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	filePath, err := safeConversationPath(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dump, err := loadConversationDump(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("load conversation dump failed: %v", err), http.StatusInternalServerError)
		return
	}
	files, err := collectConversationFiles()
	if err != nil {
		http.Error(w, fmt.Sprintf("collect conversation files failed: %v", err), http.StatusInternalServerError)
		return
	}
	rel, _ := filepath.Rel(convRoot, filePath)
	data := convViewData{
		FilePath:       "/" + filepath.ToSlash(rel),
		Title:          conversationTitle(dump),
		ConversationID: dump.Conversation.ID,
		TotalMessages:  len(dump.Conversation.Context.Messages),
		Entries:        flattenConversationEntries(dump),
		Files:          files,
		RawPath:        "/raw/" + filepath.ToSlash(rel),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("conversation-view").Parse(conversationViewHTML)).Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("render conversation view failed: %v", err), http.StatusInternalServerError)
	}
}

func handleConversationRaw(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/raw/")
	filePath, err := safeConversationPath(relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read conversation dump failed: %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(filePath)))
	_, _ = w.Write(data)
}

func collectConversationFiles() ([]convListItem, error) {
	files := make([]convListItem, 0)
	err := filepath.WalkDir(convRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipConversationDir(path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isConversationFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(convRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !matchesConversationPatterns(rel, d.Name()) {
			return nil
		}
		dump, err := loadConversationDump(path)
		if err != nil {
			return nil
		}
		files = append(files, convListItem{
			Name:         d.Name(),
			Path:         path,
			Relative:     "/" + rel,
			Title:        conversationTitle(dump),
			Messages:     len(dump.Conversation.Context.Messages),
			Conversation: dump.Conversation.ID,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	return files, nil
}

func safeConversationPath(relPath string) (string, error) {
	trimmed := strings.TrimPrefix(relPath, "/")
	cleaned := filepath.Clean(trimmed)
	absPath := filepath.Join(convRoot, cleaned)
	rootClean := filepath.Clean(convRoot)
	absClean := filepath.Clean(absPath)
	if absClean != rootClean && !strings.HasPrefix(absClean, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return absClean, nil
}

func loadConversationDump(path string) (convDump, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return convDump{}, err
	}
	var dump convDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return convDump{}, err
	}
	return dump, nil
}

func conversationTitle(dump convDump) string {
	if v := strings.TrimSpace(dump.Conversation.Title); v != "" {
		return v
	}
	if v := strings.TrimSpace(dump.Conversation.ID); v != "" {
		return v
	}
	return "Untitled Conversation"
}

func flattenConversationEntries(dump convDump) []convEntry {
	entries := make([]convEntry, 0, len(dump.Conversation.Context.Messages))
	round := 0
	for i, msg := range dump.Conversation.Context.Messages {
		if msg.Text != nil {
			if strings.EqualFold(strings.TrimSpace(msg.Text.Role), "user") {
				round++
			}
			entries = append(entries, buildTextEntry(i, *msg.Text, msg.Usage, round))
		}
		if msg.Tool != nil {
			entries = append(entries, buildToolEntry(i, *msg.Tool, round))
		}
	}
	return entries
}

func buildTextEntry(i int, text convText, usage *convUsage, round int) convEntry {
	role := strings.ToLower(strings.TrimSpace(text.Role))
	title := strings.TrimSpace(text.Role)
	if title == "" {
		title = "Message"
	}
	subtitle := fmt.Sprintf("Message %d", i+1)
	meta := make([]string, 0, 2)
	if v := strings.TrimSpace(text.Model); v != "" {
		meta = append(meta, "model: "+v)
	}
	if len(text.ToolCalls) > 0 {
		meta = append(meta, fmt.Sprintf("tool calls: %d", len(text.ToolCalls)))
	}
	usageMeta := usageLines(usage)
	body := renderTextBlock(text.Content)
	if len(text.ToolCalls) > 0 {
		body += renderToolCalls(text.ToolCalls)
	}
	return convEntry{
		ID:          fmt.Sprintf("msg-%d", i+1),
		Kind:        "text",
		Role:        role,
		Title:       title,
		Subtitle:    subtitle,
		Body:        body,
		Meta:        meta,
		Usage:       usageMeta,
		Accent:      roleAccent(role),
		RoundLabel:  roundLabel(role, round, text.Content),
		RoundAnchor: roundAnchor(role, round),
	}
}

func buildToolEntry(i int, tool convTool, round int) convEntry {
	meta := make([]string, 0, 2)
	if v := strings.TrimSpace(tool.Name); v != "" {
		meta = append(meta, "tool: "+v)
	}
	if v := strings.TrimSpace(tool.CallID); v != "" {
		meta = append(meta, "call: "+v)
	}
	body := template.HTML("")
	if strings.TrimSpace(tool.Content) != "" {
		body += renderTextBlock(tool.Content)
	}
	if tool.Output != nil {
		body += renderToolOutput(tool.Name, *tool.Output)
	}
	return convEntry{
		ID:          fmt.Sprintf("tool-%d", i+1),
		Kind:        "tool",
		Role:        "tool",
		Title:       "Tool",
		Subtitle:    fmt.Sprintf("Message %d", i+1),
		Body:        body,
		Meta:        meta,
		Accent:      "tool",
		CallID:      tool.CallID,
		RoundLabel:  roundLabel("tool", round, tool.Content),
		RoundAnchor: roundAnchor("tool", round),
	}
}

func roundAnchor(role string, round int) string {
	if round <= 0 || !strings.EqualFold(strings.TrimSpace(role), "user") {
		return ""
	}
	return fmt.Sprintf("round-%d", round)
}

func roundLabel(role string, round int, content string) string {
	if round <= 0 || !strings.EqualFold(strings.TrimSpace(role), "user") {
		return ""
	}
	summary := summarizePrompt(content)
	if summary == "" {
		return fmt.Sprintf("第 %d 轮", round)
	}
	return fmt.Sprintf("第 %d 轮 · %s", round, summary)
}

func summarizePrompt(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	runes := []rune(trimmed)
	if len(runes) <= 72 {
		return trimmed
	}
	return string(runes[:72]) + "…"
}

func renderTextBlock(content string) template.HTML {
	return mdutil.RenderMarkdownBlock(content)
}

func renderToolCalls(calls []convToolCall) template.HTML {
	var b strings.Builder
	b.WriteString("<div class=\"tool-calls\">")
	b.WriteString(renderToolSectionHeader("Tool calls", len(calls), false))
	for _, call := range calls {
		toolClass := html.EscapeString(toolKindClass(call.Name))
		icon := html.EscapeString(toolIcon(call.Name))
		label := html.EscapeString(toolDisplayName(call.Name))
		toolNameAttr := html.EscapeString(strings.ToLower(strings.TrimSpace(call.Name)))
		b.WriteString("<details class=\"tool-box tool-call-card " + toolClass + "\" open data-tool-name=\"" + toolNameAttr + "\"><summary><span class=\"tool-summary-main\"><span class=\"tool-icon\">" + icon + "</span><span class=\"tool-name\">" + label + "</span></span>")
		if strings.TrimSpace(call.CallID) != "" {
			b.WriteString(" <span class=\"call-id\">" + html.EscapeString(call.CallID) + "</span>")
		}
		b.WriteString("</summary>")
		b.WriteString(renderToolCallBody(call))
		b.WriteString("</details>")
	}
	b.WriteString("</div>")
	return template.HTML(b.String())
}

func renderToolOutput(toolName string, output convToolValue) template.HTML {
	var b strings.Builder
	stateClass := "ok"
	stateText := "ok"
	if output.IsError {
		stateClass = "error"
		stateText = "error"
	}
	b.WriteString("<div class=\"tool-output\">")
	b.WriteString(renderToolSectionHeader("Output", len(output.Values), true))
	b.WriteString("<div class=\"tool-output-state\"><span class=\"badge " + stateClass + "\">" + stateText + "</span></div>")
	if len(output.Values) == 0 {
		b.WriteString("<div class=\"empty\">No output values</div>")
	} else {
		toolNameAttr := html.EscapeString(strings.ToLower(strings.TrimSpace(toolName)))
		icon := html.EscapeString(toolIcon(toolName))
		for idx, raw := range output.Values {
			b.WriteString(fmt.Sprintf("<details class=\"tool-box tool-output-card %s\" %s data-tool-name=\"%s\"><summary><span class=\"tool-summary-main\"><span class=\"tool-icon\">%s</span><span class=\"tool-name\">Value %d</span></span></summary>", html.EscapeString(toolKindClass(toolName)), openAttr(idx == 0), toolNameAttr, icon, idx+1))
			b.WriteString(renderStructuredValue(raw, true, toolName))
			b.WriteString("</details>")
		}
	}
	b.WriteString("</div>")
	return template.HTML(b.String())
}

func renderToolSectionHeader(title string, count int, searchable bool) string {
	var b strings.Builder
	b.WriteString("<div class=\"tool-section-head\">")
	b.WriteString("<div class=\"section-label\">" + html.EscapeString(title) + "</div>")
	b.WriteString("<div class=\"tool-section-actions\">")
	if count > 0 {
		b.WriteString("<span class=\"chip\">" + html.EscapeString(fmt.Sprintf("%d items", count)) + "</span>")
	}
	if searchable {
		b.WriteString("<input class=\"tool-search-input\" type=\"search\" placeholder=\"搜索输出...\" data-tool-search>")
	}
	b.WriteString("<button type=\"button\" class=\"tool-action-btn\" data-tool-toggle=\"expand\">全部展开</button>")
	b.WriteString("<button type=\"button\" class=\"tool-action-btn\" data-tool-toggle=\"collapse\">全部收起</button>")
	b.WriteString("</div></div>")
	return b.String()
}

func toolDisplayName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "tool"
	}
	return trimmed
}

func toolIcon(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write":
		return "✍"
	case "read":
		return "📖"
	case "patch", "multi_patch":
		return "🩹"
	case "shell":
		return "⌘"
	case "task":
		return "⚙"
	case "todo_write":
		return "☑"
	case "remove":
		return "🗑"
	case "undo":
		return "↶"
	default:
		return "🧰"
	}
}

func renderToolCallBody(call convToolCall) string {
	trimmed := strings.TrimSpace(string(call.Arguments))
	if trimmed == "" {
		return "<div class=\"empty\">No arguments</div>"
	}

	var obj map[string]any
	if err := json.Unmarshal(call.Arguments, &obj); err == nil {
		return renderToolObject(call.Name, obj, true)
	}
	return renderStructuredValue(call.Arguments, true, call.Name)
}

func renderStructuredValue(raw json.RawMessage, markdownText bool, toolName string) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "<div class=\"empty\">No content</div>"
	}

	var val any
	if err := json.Unmarshal([]byte(trimmed), &val); err != nil {
		return renderRawValue(trimmed, markdownText)
	}

	if obj, ok := val.(map[string]any); ok {
		return renderToolObject(toolName, obj, markdownText)
	}
	if list, ok := val.([]any); ok {
		return renderArrayValue(list, markdownText, toolName)
	}
	return renderRawValue(trimmed, markdownText)
}

func renderToolObject(toolName string, obj map[string]any, markdownText bool) string {
	lowerTool := strings.ToLower(strings.TrimSpace(toolName))
	switch lowerTool {
	case "write":
		return renderWriteToolObject(obj)
	case "shell":
		return renderShellToolObject(obj)
	case "patch":
		return renderPatchToolObject(obj)
	case "multi_patch":
		return renderMultiPatchToolObject(obj)
	}

	if len(obj) == 0 {
		return "<div class=\"empty\">Empty object</div>"
	}
	keys := orderedToolKeys(toolName, obj)
	summary := renderToolSummary(toolName, obj)

	var b strings.Builder
	if summary != "" {
		b.WriteString(summary)
	}
	b.WriteString("<div class=\"tool-field-grid\">")
	for _, key := range keys {
		className := "tool-field"
		if isWideField(key, obj[key]) {
			className += " wide"
		}
		b.WriteString("<section class=\"" + className + "\">")
		b.WriteString("<div class=\"tool-field-key\">" + html.EscapeString(key) + "</div>")
		b.WriteString(renderFieldValue(key, obj[key], markdownText, toolName))
		b.WriteString("</section>")
	}
	b.WriteString("</div>")
	return b.String()
}

func renderArrayValue(list []any, markdownText bool, toolName string) string {
	if len(list) == 0 {
		return "<div class=\"empty\">Empty list</div>"
	}
	var b strings.Builder
	b.WriteString("<div class=\"tool-array\">")
	for i, item := range list {
		className := "tool-field"
		if isWideField("item", item) {
			className += " wide"
		}
		b.WriteString("<section class=\"" + className + "\">")
		b.WriteString(fmt.Sprintf("<div class=\"tool-field-key\">Item %d</div>", i+1))
		b.WriteString(renderUnknownValue(item, markdownText, toolName))
		b.WriteString("</section>")
	}
	b.WriteString("</div>")
	return b.String()
}

func renderFieldValue(key string, value any, markdownText bool, toolName string) string {
	switch v := value.(type) {
	case string:
		return renderStringField(key, v, markdownText)
	default:
		return renderUnknownValue(v, markdownText, toolName)
	}
}

func renderStringField(key, value string, markdownText bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "<div class=\"empty\">Empty string</div>"
	}
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if looksLikeTaggedMarkup(trimmed) {
		return renderMarkupBlock(trimmed, markdownText)
	}
	switch lowerKey {
	case "content", "text", "body", "message":
		if markdownText {
			return string(renderTextBlock(trimmed))
		}
		return renderCodeBlock(trimmed)
	case "file_path", "path", "config", "home", "workspace":
		return "<div class=\"tool-inline-value tool-path-value\">" + html.EscapeString(trimmed) + "</div>"
	case "command":
		return renderCodeBlock(trimmed)
	default:
		if looksLikeJSON(trimmed) {
			return renderCodeBlock(prettyJSON(trimmed))
		}
		if markdownText && strings.Contains(trimmed, "\n") {
			return string(renderTextBlock(trimmed))
		}
		if strings.Contains(trimmed, "\n") || len([]rune(trimmed)) > 120 {
			return renderCodeBlock(trimmed)
		}
		return "<div class=\"tool-inline-value\">" + html.EscapeString(trimmed) + "</div>"
	}
}

func renderUnknownValue(value any, markdownText bool, toolName string) string {
	switch v := value.(type) {
	case nil:
		return "<div class=\"empty\">null</div>"
	case map[string]any:
		return renderToolObject(toolName, v, markdownText)
	case []any:
		return renderArrayValue(v, markdownText, toolName)
	case string:
		return renderStringField("", v, markdownText)
	case bool, float64, int, int64:
		return "<div class=\"tool-inline-value\">" + html.EscapeString(fmt.Sprintf("%v", v)) + "</div>"
	default:
		buf, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "<div class=\"tool-inline-value\">" + html.EscapeString(fmt.Sprintf("%v", v)) + "</div>"
		}
		return renderCodeBlock(string(buf))
	}
}

func renderWriteToolObject(obj map[string]any) string {
	var b strings.Builder
	if summary := renderToolSummary("write", obj); summary != "" {
		b.WriteString(summary)
	}
	b.WriteString("<div class=\"tool-tool-layout tool-write-layout\">")
	b.WriteString(renderMetaFields(obj, []string{"file_path", "overwrite"}, "write"))
	if content, ok := obj["content"].(string); ok && strings.TrimSpace(content) != "" {
		b.WriteString("<section class=\"tool-field wide tool-content-field\">")
		b.WriteString("<div class=\"tool-field-key\">content</div>")
		b.WriteString(string(renderTextBlock(content)))
		b.WriteString("</section>")
	}
	b.WriteString(renderRemainingFields(obj, []string{"file_path", "overwrite", "content"}, true, "write"))
	b.WriteString("</div>")
	return b.String()
}

func renderShellToolObject(obj map[string]any) string {
	var b strings.Builder
	if summary := renderToolSummary("shell", obj); summary != "" {
		b.WriteString(summary)
	}
	b.WriteString("<div class=\"tool-tool-layout tool-shell-layout\">")
	if command, ok := obj["command"].(string); ok && strings.TrimSpace(command) != "" {
		b.WriteString("<section class=\"tool-field wide tool-command-field\">")
		b.WriteString("<div class=\"tool-field-key\">command</div>")
		b.WriteString(renderCodeBlock(command))
		b.WriteString("</section>")
	}
	b.WriteString(renderMetaFields(obj, []string{"cwd", "description", "keep_ansi", "env"}, "shell"))
	b.WriteString(renderRemainingFields(obj, []string{"command", "cwd", "description", "keep_ansi", "env"}, false, "shell"))
	b.WriteString("</div>")
	return b.String()
}

func renderPatchToolObject(obj map[string]any) string {
	var b strings.Builder
	if summary := renderToolSummary("patch", obj); summary != "" {
		b.WriteString(summary)
	}
	b.WriteString("<div class=\"tool-tool-layout tool-patch-layout\">")
	b.WriteString(renderMetaFields(obj, []string{"file_path", "replace_all"}, "patch"))
	for _, key := range []string{"old_string", "new_string"} {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			b.WriteString("<section class=\"tool-field wide tool-content-field\">")
			b.WriteString("<div class=\"tool-field-key\">" + html.EscapeString(key) + "</div>")
			b.WriteString(string(renderTextBlock(value)))
			b.WriteString("</section>")
		}
	}
	b.WriteString(renderRemainingFields(obj, []string{"file_path", "replace_all", "old_string", "new_string"}, true, "patch"))
	b.WriteString("</div>")
	return b.String()
}

func renderMultiPatchToolObject(obj map[string]any) string {
	var b strings.Builder
	if summary := renderToolSummary("multi_patch", obj); summary != "" {
		b.WriteString(summary)
	}
	b.WriteString("<div class=\"tool-tool-layout tool-patch-layout\">")
	b.WriteString(renderMetaFields(obj, []string{"file_path"}, "multi_patch"))
	if edits, ok := obj["edits"]; ok {
		b.WriteString("<section class=\"tool-field wide\">")
		b.WriteString("<div class=\"tool-field-key\">edits</div>")
		b.WriteString(renderUnknownValue(edits, true, "multi_patch"))
		b.WriteString("</section>")
	}
	b.WriteString(renderRemainingFields(obj, []string{"file_path", "edits"}, true, "multi_patch"))
	b.WriteString("</div>")
	return b.String()
}

func renderMetaFields(obj map[string]any, keys []string, toolName string) string {
	if len(keys) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<div class=\"tool-field-grid tool-meta-grid\">")
	for _, key := range keys {
		value, ok := obj[key]
		if !ok {
			continue
		}
		className := "tool-field"
		if isWideField(key, value) {
			className += " wide"
		}
		b.WriteString("<section class=\"" + className + "\">")
		b.WriteString("<div class=\"tool-field-key\">" + html.EscapeString(key) + "</div>")
		b.WriteString(renderFieldValue(key, value, false, toolName))
		b.WriteString("</section>")
	}
	b.WriteString("</div>")
	return b.String()
}

func renderRemainingFields(obj map[string]any, excluded []string, markdownText bool, toolName string) string {
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		excludedSet[key] = struct{}{}
	}
	rest := make(map[string]any)
	for key, value := range obj {
		if _, ok := excludedSet[key]; ok {
			continue
		}
		rest[key] = value
	}
	if len(rest) == 0 {
		return ""
	}
	return `<section class="tool-field wide"><div class="tool-field-key">other fields</div>` + renderToolObjectGeneric(rest, markdownText, toolName) + `</section>`
}

func renderToolObjectGeneric(obj map[string]any, markdownText bool, toolName string) string {
	keys := orderedToolKeys(toolName, obj)
	var b strings.Builder
	b.WriteString("<div class=\"tool-field-grid\">")
	for _, key := range keys {
		className := "tool-field"
		if isWideField(key, obj[key]) {
			className += " wide"
		}
		b.WriteString("<section class=\"" + className + "\">")
		b.WriteString("<div class=\"tool-field-key\">" + html.EscapeString(key) + "</div>")
		b.WriteString(renderFieldValue(key, obj[key], markdownText, toolName))
		b.WriteString("</section>")
	}
	b.WriteString("</div>")
	return b.String()
}

func renderRawValue(value string, markdownText bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "<div class=\"empty\">No content</div>"
	}
	if looksLikeTaggedMarkup(trimmed) {
		return renderMarkupBlock(trimmed, markdownText)
	}
	if markdownText {
		return string(renderTextBlock(trimmed))
	}
	return renderCodeBlock(trimmed)
}

func renderCodeBlock(value string) string {
	return "<pre class=\"tool-json\">" + html.EscapeString(value) + "</pre>"
}

func renderToolSummary(toolName string, obj map[string]any) string {
	chips := make([]string, 0, 6)
	appendChip := func(label string, value any) {
		text := scalarSummary(value)
		if text == "" {
			return
		}
		chips = append(chips, "<span class=\"chip\">"+html.EscapeString(label+": "+text)+"</span>")
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "write", "read", "patch", "multi_patch", "remove", "undo":
		appendChip("path", obj["file_path"])
		appendChip("path", obj["path"])
		appendChip("overwrite", obj["overwrite"])
		appendChip("start", obj["start_line"])
		appendChip("end", obj["end_line"])
	case "shell":
		appendChip("cwd", obj["cwd"])
		appendChip("description", obj["description"])
		appendChip("keep_ansi", obj["keep_ansi"])
	case "todo_write":
		appendChip("todos", arrayLenValue(obj["todos"]))
	case "task":
		appendChip("agent_id", obj["agent_id"])
		appendChip("tasks", arrayLenValue(obj["tasks"]))
		appendChip("session_id", obj["session_id"])
	}
	if len(chips) == 0 {
		return ""
	}
	return "<div class=\"tool-summary\">" + strings.Join(chips, "") + "</div>"
}

func orderedToolKeys(toolName string, obj map[string]any) []string {
	seen := make(map[string]bool, len(obj))
	keys := make([]string, 0, len(obj))
	for _, key := range preferredToolFieldOrder(toolName) {
		if _, ok := obj[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	rest := make([]string, 0, len(obj))
	for key := range obj {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func preferredToolFieldOrder(toolName string) []string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "write":
		return []string{"file_path", "overwrite", "content"}
	case "read":
		return []string{"file_path", "start_line", "end_line", "show_line_numbers"}
	case "patch":
		return []string{"file_path", "old_string", "new_string", "replace_all"}
	case "multi_patch":
		return []string{"file_path", "edits"}
	case "shell":
		return []string{"command", "cwd", "description", "env", "keep_ansi"}
	case "todo_write":
		return []string{"todos"}
	case "task":
		return []string{"agent_id", "session_id", "tasks"}
	default:
		return []string{"file_path", "path", "command", "content", "body", "text", "message"}
	}
}

func isWideField(key string, value any) bool {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch lowerKey {
	case "content", "command", "old_string", "new_string", "body", "text", "message", "edits", "todos", "tasks", "values", "arguments":
		return true
	}
	switch v := value.(type) {
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case string:
		return strings.Contains(v, "\n") || len([]rune(v)) > 100
	default:
		return false
	}
}

func toolKindClass(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return "tool-generic"
	}
	return "tool-" + strings.ReplaceAll(lower, "_", "-")
}

func scalarSummary(value any) string {
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		runes := []rune(trimmed)
		if len(runes) > 48 {
			return string(runes[:48]) + "…"
		}
		return trimmed
	case bool, float64, int, int64:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

func arrayLenValue(value any) any {
	if list, ok := value.([]any); ok {
		return len(list)
	}
	return nil
}

func looksLikeTaggedMarkup(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">") && strings.Contains(trimmed, "</")
}

func renderMarkupBlock(value string, markdownText bool) string {
	tag, attrs, inner := parseMarkupBlock(value)
	var b strings.Builder
	b.WriteString("<details class=\"tool-box markup-block\"><summary><span class=\"tool-name\">" + html.EscapeString(tag) + "</span></summary>")
	if len(attrs) > 0 {
		b.WriteString("<div class=\"tool-summary\">")
		for _, attr := range attrs {
			b.WriteString("<span class=\"chip\">" + html.EscapeString(attr) + "</span>")
		}
		b.WriteString("</div>")
	}
	if strings.TrimSpace(inner) != "" {
		b.WriteString("<div class=\"markup-content\">")
		if markdownText {
			b.WriteString(string(mdutil.RenderMarkdownBlock(inner)))
		} else {
			b.WriteString(renderCodeBlock(inner))
		}
		b.WriteString("</div>")
	}
	b.WriteString("<details class=\"markup-raw\"><summary>查看原始内容</summary>")
	b.WriteString(renderCodeBlock(value))
	b.WriteString("</details></details>")
	return b.String()
}

func parseMarkupBlock(value string) (string, []string, string) {
	trimmed := strings.TrimSpace(value)
	headEnd := strings.Index(trimmed, ">")
	if headEnd <= 1 {
		return "markup", nil, trimmed
	}
	head := trimmed[1:headEnd]
	tag := head
	if idx := strings.IndexAny(head, " \t\r\n"); idx >= 0 {
		tag = head[:idx]
	}
	attrs := make([]string, 0, 4)
	for _, match := range markupAttrPattern.FindAllStringSubmatch(head, -1) {
		attrs = append(attrs, match[1]+"="+match[2])
	}
	inner := trimmed[headEnd+1:]
	if cdataStart := strings.Index(inner, "<![CDATA["); cdataStart >= 0 {
		inner = inner[cdataStart+9:]
		if cdataEnd := strings.Index(inner, "]]>"); cdataEnd >= 0 {
			inner = inner[:cdataEnd]
		}
	} else if closeIdx := strings.LastIndex(inner, "</"); closeIdx >= 0 {
		inner = inner[:closeIdx]
	}
	return strings.TrimSpace(tag), attrs, strings.TrimSpace(inner)
}

func looksLikeJSON(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func usageLines(usage *convUsage) []string {
	if usage == nil {
		return nil
	}
	lines := make([]string, 0, 4)
	if usage.PromptTokens.Actual > 0 {
		lines = append(lines, fmt.Sprintf("prompt: %d", usage.PromptTokens.Actual))
	}
	if usage.CompletionTokens.Actual > 0 {
		lines = append(lines, fmt.Sprintf("completion: %d", usage.CompletionTokens.Actual))
	}
	if usage.TotalTokens.Actual > 0 {
		lines = append(lines, fmt.Sprintf("total: %d", usage.TotalTokens.Actual))
	}
	if usage.CachedTokens.Actual > 0 {
		lines = append(lines, fmt.Sprintf("cached: %d", usage.CachedTokens.Actual))
	}
	return lines
}

func roleAccent(role string) string {
	switch role {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "neutral"
	}
}

func prettyJSON(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	var val any
	if err := json.Unmarshal([]byte(trimmed), &val); err != nil {
		return trimmed
	}
	buf, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(buf)
}

func openAttr(open bool) string {
	if open {
		return "open"
	}
	return ""
}

func skipConversationDir(path, name string) bool {
	if path == convRoot {
		return false
	}
	if name == ".git" || name == "node_modules" || name == "vendor" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isConversationFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".json") && strings.Contains(lower, "dump")
}

func matchesConversationPatterns(relPath, fileName string) bool {
	if strings.TrimSpace(convFile) != "" {
		return true
	}
	if len(convPatterns) == 0 {
		return true
	}
	for _, pattern := range convPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if match, _ := filepath.Match(pattern, relPath); match {
			return true
		}
		if match, _ := filepath.Match(pattern, fileName); match {
			return true
		}
	}
	return false
}

func openConversationBrowser(url string) {
	cmd := exec.Command("open", url)
	if err := cmd.Run(); err != nil {
		fmt.Printf("open browser failed: %v\n", err)
	}
}

const conversationListHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Conversation</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 0; background: #f6f8fb; color: #1f2937; }
    .wrap { max-width: 1100px; margin: 0 auto; padding: 32px 20px; }
    .card { background: white; border-radius: 16px; box-shadow: 0 10px 30px rgba(15,23,42,.08); padding: 24px; }
    .item { padding: 18px 0; border-bottom: 1px solid #e5e7eb; }
    .item:last-child { border-bottom: 0; }
    .title { margin: 0 0 8px; font-size: 19px; }
    .path { color: #667085; font-size: 13px; }
    .meta { color: #475467; font-size: 13px; margin-top: 8px; display:flex; gap:12px; flex-wrap:wrap; }
    a { color: #2563eb; text-decoration: none; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>Conversation Dumps</h1>
      <p>Total: {{.Total}}</p>
      {{range .Files}}
      <div class="item">
        <h2 class="title"><a href="/view{{.Relative}}">{{.Title}}</a></h2>
        <div class="path">{{.Relative}}</div>
        <div class="meta">
          <span>{{.Messages}} messages</span>
          <span>{{.Conversation}}</span>
          <a href="/raw{{.Relative}}">download raw</a>
        </div>
      </div>
      {{else}}
      <p>No conversation dumps found.</p>
      {{end}}
    </div>
  </div>
</body>
</html>`

const conversationViewHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/github-markdown-css@5.2.0/github-markdown.min.css">
  <script src="https://cdn.jsdelivr.net/npm/marked@9.1.2/marked.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/mermaid@11.7.0/dist/mermaid.min.js"></script>
  <style>
    :root {
      --bg: #f6f8fb;
      --panel: #fff;
      --line: #e5e7eb;
      --text: #111827;
      --subtle: #667085;
      --blue: #2563eb;
      --blue-soft: #eff6ff;
      --green: #059669;
      --green-soft: #ecfdf3;
      --amber: #b45309;
      --amber-soft: #fffbeb;
      --violet: #7c3aed;
      --violet-soft: #f5f3ff;
      --red: #dc2626;
      --red-soft: #fef2f2;
      --shadow: 0 10px 30px rgba(15,23,42,.08);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg); color: var(--text); }
    .layout { display: grid; grid-template-columns: 280px minmax(0,1fr) 260px; gap: 20px; padding: 20px; height: 100vh; overflow: hidden; }
    .side, .main, .toc-pane { min-height: 0; }
    .side, .toc-pane { background: var(--panel); border: 1px solid var(--line); border-radius: 18px; box-shadow: var(--shadow); overflow: auto; padding: 18px; }
    .main { overflow: auto; }
    .main-inner { max-width: 980px; margin: 0 auto; padding-bottom: 24px; }
    .hero { background: var(--panel); border: 1px solid var(--line); border-radius: 18px; box-shadow: var(--shadow); padding: 24px; margin-bottom: 18px; }
    .cards { display: grid; gap: 14px; }
    .entry { background: var(--panel); border: 1px solid var(--line); border-radius: 18px; box-shadow: var(--shadow); overflow: hidden; }
    .entry-head { display:flex; justify-content:space-between; gap:16px; align-items:flex-start; padding:16px 18px; border-bottom:1px solid var(--line); }
    .entry-body { padding: 18px; }
    .accent-user .entry-head { background: var(--blue-soft); }
    .accent-assistant .entry-head { background: var(--green-soft); }
    .accent-system .entry-head { background: var(--amber-soft); }
    .accent-tool .entry-head { background: var(--violet-soft); }
    .accent-neutral .entry-head { background: #f8fafc; }
    .eyebrow { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; color: var(--subtle); margin-bottom: 6px; }
    .title { margin: 0; font-size: 20px; }
    .subtitle { color: var(--subtle); font-size: 13px; margin-top: 6px; }
    .meta, .usage { display:flex; gap:8px; flex-wrap:wrap; justify-content:flex-end; }
    .chip { display:inline-flex; align-items:center; padding:4px 10px; border-radius:999px; border:1px solid var(--line); font-size:12px; color: var(--subtle); background:#fff; }
    .md-block { display: grid; gap: 12px; }
    .md-actions { display:flex; justify-content:flex-end; }
    .markdown-body.md-render {
      max-width: none;
      margin: 0;
      padding: 0;
      background: transparent;
      border: 0;
      box-shadow: none;
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, sans-serif;
    }
    .markdown-body.md-render pre, .tool-json, .markdown-body.md-render .mermaid {
      overflow-x: auto;
    }
    .markdown-body.md-render .mermaid {
      display: flex;
      justify-content: center;
      padding: 8px 0;
    }
    .markdown-body.md-render > :first-child { margin-top: 0; }
    .markdown-body.md-render > :last-child { margin-bottom: 0; }
    .tool-json {
      margin:0;
      white-space:pre-wrap;
      word-break:break-word;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      background:#0f172a;
      color:#e5eefc;
      border-radius:14px;
      padding:14px 16px;
      overflow:auto;
    }
    .tool-fields, .tool-array {
      display: grid;
      gap: 12px;
    }
    .tool-field-grid {
      display:grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 12px;
    }
    .tool-meta-grid {
      margin-bottom: 12px;
    }
    .tool-tool-layout {
      display:grid;
      gap: 14px;
    }
    .tool-content-field .md-block,
    .tool-content-field .view-pane {
      margin-top: 0;
    }
    .tool-field {
      border: 1px solid var(--line);
      border-radius: 14px;
      background: #f8fafc;
      padding: 12px;
    }
    .tool-field-key {
      font-size: 12px;
      font-weight: 700;
      letter-spacing: .04em;
      text-transform: uppercase;
      color: var(--subtle);
      margin-bottom: 10px;
    }
    .tool-inline-value {
      padding: 10px 12px;
      border-radius: 10px;
      background: #fff;
      border: 1px solid var(--line);
      color: var(--text);
      font-size: 14px;
      line-height: 1.6;
      word-break: break-word;
    }
    .tool-path-value {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      background: #f8fafc;
    }
    .tool-summary {
      display:flex;
      gap:8px;
      flex-wrap:wrap;
      margin-bottom:12px;
    }
    .tool-section-head {
      display:flex;
      align-items:center;
      justify-content:space-between;
      gap:12px;
      flex-wrap:wrap;
      margin: 14px 0 10px;
    }
    .tool-section-actions {
      display:flex;
      align-items:center;
      gap:8px;
      flex-wrap:wrap;
    }
    .tool-action-btn {
      border:1px solid var(--line);
      background:#fff;
      color:#475467;
      border-radius:999px;
      padding:6px 10px;
      font-size:12px;
      font-weight:600;
      cursor:pointer;
    }
    .tool-action-btn:hover { background:#f8fafc; color:#111827; }
    .tool-search-input {
      min-width: 180px;
      border:1px solid var(--line);
      border-radius:999px;
      padding:7px 12px;
      font-size:12px;
      color:var(--text);
      background:#fff;
    }
    .tool-summary-main {
      display:inline-flex;
      align-items:center;
      gap:8px;
    }
    .tool-icon {
      display:inline-flex;
      align-items:center;
      justify-content:center;
      width:24px;
      height:24px;
      border-radius:999px;
      background:#eef2ff;
      font-size:13px;
      line-height:1;
    }
    .copy-btn {
      border:1px solid var(--line);
      background:#fff;
      color:#475467;
      border-radius:999px;
      width:32px;
      height:32px;
      display:inline-flex;
      align-items:center;
      justify-content:center;
      cursor:pointer;
      font-size:14px;
      line-height:1;
    }
    .copy-btn:hover { background:#f8fafc; color:#111827; }
    .copy-btn.copied { background:var(--green-soft); color:var(--green); border-color:#a7f3d0; }
    .section-label { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; color: var(--subtle); margin: 0; }
    .tool-box { border:1px solid var(--line); border-radius:14px; background:#fff; margin-top:10px; overflow:hidden; }
    .tool-box summary { cursor:pointer; padding:12px 14px; font-weight:600; display:flex; align-items:center; justify-content:space-between; gap:12px; }
    .tool-call-card.tool-write > summary,
    .tool-output-card.tool-write > summary { background:#eff6ff; }
    .tool-call-card.tool-shell > summary,
    .tool-output-card.tool-shell > summary { background:#f8fafc; }
    .tool-call-card.tool-patch > summary,
    .tool-output-card.tool-patch > summary,
    .tool-call-card.tool-multi-patch > summary,
    .tool-output-card.tool-multi-patch > summary { background:#f5f3ff; }
    .tool-call-card.tool-read > summary,
    .tool-output-card.tool-read > summary { background:#ecfdf3; }
    .call-id { font-weight:400; color: var(--subtle); font-size:12px; }
    .badge { display:inline-flex; align-items:center; padding:2px 8px; border-radius:999px; font-size:11px; margin-left:8px; }
    .badge.ok { background: var(--green-soft); color: var(--green); }
    .badge.error { background: var(--red-soft); color: var(--red); }
    .file-link { display:block; padding:10px 12px; border-radius:12px; color: var(--text); text-decoration:none; border:1px solid transparent; }
    .file-link:hover { background:#f8fafc; border-color:var(--line); }
    .file-title { font-weight:600; font-size:14px; }
    .file-meta { margin-top:6px; color:var(--subtle); font-size:12px; }
    .toolbar { display:flex; gap:10px; margin-top:16px; }
    .btn { display:inline-flex; align-items:center; justify-content:center; padding:10px 14px; border-radius:12px; text-decoration:none; font-weight:600; border:1px solid var(--line); color: var(--text); background:#fff; }
    .btn.primary { color:#fff; background:var(--blue); border-color:var(--blue); }
    .empty { color:var(--subtle); font-size:13px; }
    .toc-title { margin: 0; font-size: 15px; font-weight: 700; }
    .toc-subtitle { margin: 8px 0 0; color: var(--subtle); font-size: 12px; line-height: 1.6; }
    .toc-list { list-style: none; padding: 0; margin: 18px 0 0; }
    .toc-item { margin: 6px 0; }
    .toc-item.level-3 { padding-left: 12px; }
    .toc-item.level-4 { padding-left: 24px; }
    .toc-link {
      display: block;
      padding: 8px 10px;
      border-left: 3px solid transparent;
      border-radius: 8px;
      color: #475467;
      text-decoration: none;
      font-size: 13px;
      line-height: 1.45;
      transition: .2s ease;
      word-break: break-word;
    }
    .toc-link:hover { color: var(--blue); background: #f8fafc; border-left-color: #93c5fd; }
    .toc-link.active { color: #1d4ed8; background: var(--blue-soft); border-left-color: var(--blue); font-weight: 600; }
    .toc-empty { margin-top: 18px; color: var(--subtle); font-size: 13px; }
    .entry-body.is-collapsed {
      max-height: 420px;
      overflow: hidden;
      position: relative;
    }
    .entry-body.is-collapsed::after {
      content: "";
      position: absolute;
      left: 0;
      right: 0;
      bottom: 0;
      height: 88px;
      pointer-events: none;
      background: linear-gradient(to bottom, rgba(255,255,255,0), rgba(255,255,255,.96) 75%, rgba(255,255,255,1));
    }
    .entry-tools { display:flex; justify-content:center; margin-top: 14px; }
    .toggle-btn {
      width:42px;
      height:42px;
      padding:0;
      display:inline-flex;
      align-items:center;
      justify-content:center;
      border-radius:999px;
      border:1px solid var(--line);
      background:#fff;
      color:#475467;
      cursor:pointer;
      box-shadow:0 4px 14px rgba(15,23,42,.06);
    }
    .toggle-btn-icon {
      width:18px;
      height:18px;
      display:block;
      transition:transform .2s ease;
    }
    .toggle-btn.is-expanded .toggle-btn-icon { transform: rotate(180deg); }
    .toggle-btn:hover { background:#f8fafc; color:#111827; }
    @media (max-width: 1200px) {
      .layout { grid-template-columns: 250px minmax(0,1fr) 220px; }
    }
    @media (max-width: 960px) {
      body { overflow: auto; }
      .layout { grid-template-columns: 1fr; height: auto; }
      .side, .toc-pane, .main { overflow: visible; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <aside class="side">
      <h2 style="margin-top:0">Conversation Files</h2>
      {{range .Files}}
      <a class="file-link" href="/view{{.Relative}}">
        <div class="file-title">{{.Title}}</div>
        <div class="file-meta">{{.Messages}} messages · {{.Relative}}</div>
      </a>
      {{else}}
      <div class="empty">No conversation dumps found.</div>
      {{end}}
    </aside>
    <main class="main" id="conversation-main">
      <div class="main-inner">
      <section class="hero">
        <div class="eyebrow">Conversation</div>
        <h1 class="title">{{.Title}}</h1>
        <div class="subtitle">{{.FilePath}}</div>
        <div class="toolbar">
          <a class="btn" href="/list">File list</a>
          <a class="btn primary" href="{{.RawPath}}">Download raw</a>
        </div>
        <div class="meta" style="justify-content:flex-start; margin-top:14px;">
          <span class="chip">messages: {{.TotalMessages}}</span>
          <span class="chip">conversation: {{.ConversationID}}</span>
        </div>
      </section>
      <section class="cards">
        {{range .Entries}}
        <article class="entry accent-{{.Accent}}" id="{{.ID}}" {{if .RoundAnchor}}data-round-anchor="{{.RoundAnchor}}" data-round-label="{{.RoundLabel}}"{{end}}>
          <div class="entry-head">
            <div>
              <div class="eyebrow">{{if .Kind}}{{.Kind}}{{else}}entry{{end}}</div>
              <h2 class="title" style="font-size:18px">{{.Title}}</h2>
              <div class="subtitle">{{.Subtitle}}</div>
            </div>
            <div>
              {{if .Meta}}
              <div class="meta">
                {{range .Meta}}<span class="chip">{{.}}</span>{{end}}
              </div>
              {{end}}
              {{if .Usage}}
              <div class="usage" style="margin-top:8px;">
                {{range .Usage}}<span class="chip">{{.}}</span>{{end}}
              </div>
              {{end}}
            </div>
          </div>
          <div class="entry-body">{{.Body}}</div>
        </article>
        {{end}}
      </section>
      </div>
    </main>
    <aside class="toc-pane">
      <h2 class="toc-title">User turns</h2>
      <p class="toc-subtitle">Jump between each user question round to review the conversation flow in the order it happened.</p>
      <ul class="toc-list" id="toc-list"></ul>
      <div class="toc-empty" id="toc-empty">No headings detected yet.</div>
    </aside>
  </div>
  <script>
    marked.setOptions({
      gfm: true,
      breaks: true,
      headerIds: false,
      mangle: false,
      highlight(code, lang) {
        if (lang === 'mermaid') return code;
        return code;
      }
    });

    async function renderConversationMarkdown() {
      const blocks = Array.from(document.querySelectorAll('.md-block'));
      for (const block of blocks) {
        const sourceEl = block.querySelector('.md-source');
        const targetEl = block.querySelector('.md-render');
        if (!sourceEl || !targetEl) continue;
        const sourceText = sourceEl.textContent || '';
        targetEl.innerHTML = marked.parse(sourceText);
      }

      if (typeof mermaid === 'undefined') return;
      mermaid.initialize({
        startOnLoad: false,
        theme: 'default',
        securityLevel: 'loose',
        logLevel: 'error',
        flowchart: { useMaxWidth: true, htmlLabels: true, curve: 'basis' },
        sequence: { useMaxWidth: true },
        journey: { useMaxWidth: true },
        er: { useMaxWidth: true },
        pie: { useMaxWidth: true },
      });

      const blocksWithMermaid = document.querySelectorAll('.md-render code.language-mermaid');
      for (const [index, mermaidBlock] of blocksWithMermaid.entries()) {
        const parent = mermaidBlock.parentElement;
        const graph = document.createElement('div');
        graph.className = 'mermaid';
        graph.setAttribute('data-chart-index', index);
        graph.textContent = mermaidBlock.textContent;
        parent.replaceWith(graph);
      }

      const graphs = document.querySelectorAll('.md-render .mermaid');
      await Promise.all(Array.from(graphs).map(async (graph, index) => {
        try {
          const result = await mermaid.render('conversation-mermaid-' + index, graph.textContent);
          graph.innerHTML = result.svg;
        } catch (err) {
          graph.innerHTML = '<pre style="color:#b42318;white-space:pre-wrap;">' + String(err) + '</pre>';
        }
      }));
    }

    function slugify(text) {
      return String(text || '')
        .toLowerCase()
        .trim()
        .replace(/[^\w\u4e00-\u9fa5\s-]/g, '')
        .replace(/\s+/g, '-')
        .replace(/-+/g, '-');
    }

    function buildConversationTOC() {
      const tocList = document.getElementById('toc-list');
      const tocEmpty = document.getElementById('toc-empty');
      const mainPane = document.getElementById('conversation-main');
      const items = Array.from(document.querySelectorAll('[data-round-anchor]'));

      tocList.innerHTML = '';
      if (!items.length) {
        tocEmpty.textContent = 'No user turns detected yet.';
        tocEmpty.style.display = 'block';
        return;
      }
      tocEmpty.style.display = 'none';

      items.forEach((entry) => {
        const targetId = entry.getAttribute('data-round-anchor');
        if (!targetId) return;
        if (!entry.id) {
          entry.id = targetId;
        }
        const item = document.createElement('li');
        item.className = 'toc-item';
        const link = document.createElement('a');
        link.className = 'toc-link';
        link.href = '#' + entry.id;
        link.dataset.target = entry.id;
        const label = entry.getAttribute('data-round-label') || entry.id;
        link.textContent = label;
        link.title = label;
        link.addEventListener('click', (event) => {
          event.preventDefault();
          entry.scrollIntoView({ behavior: 'smooth', block: 'start' });
          history.replaceState(null, '', '#' + entry.id);
        });
        item.appendChild(link);
        tocList.appendChild(item);
      });

      const links = Array.from(tocList.querySelectorAll('.toc-link'));
      const setActive = (id) => {
        links.forEach((link) => link.classList.toggle('active', link.dataset.target === id));
      };
      const updateActive = () => {
        let active = items[0];
        for (const entry of items) {
          const rect = entry.getBoundingClientRect();
          if (rect.top <= 180) active = entry;
          else break;
        }
        setActive(active.id);
      };
      mainPane.addEventListener('scroll', updateActive, { passive: true });
      window.addEventListener('hashchange', updateActive);
      updateActive();
    }

    function wireCopyButtons() {
      const buttons = Array.from(document.querySelectorAll('[data-copy-source]'));
      buttons.forEach((button) => {
        button.addEventListener('click', async () => {
          const block = button.closest('.md-block');
          const sourceEl = block ? block.querySelector('.md-source') : null;
          const sourceText = sourceEl ? (sourceEl.textContent || '') : '';
          if (!sourceText) return;
          try {
            await navigator.clipboard.writeText(sourceText);
            const previous = button.textContent;
            button.textContent = '✓';
            button.classList.add('copied');
            setTimeout(() => {
              button.textContent = previous;
              button.classList.remove('copied');
            }, 1200);
          } catch (err) {
            console.error('copy failed', err);
          }
        });
      });
    }

    function wireToolSections() {
      const sections = Array.from(document.querySelectorAll('.tool-calls, .tool-output'));
      sections.forEach((section) => {
        const details = Array.from(section.querySelectorAll('.tool-box'));
        const search = section.querySelector('[data-tool-search]');
        const expandBtn = section.querySelector('[data-tool-toggle="expand"]');
        const collapseBtn = section.querySelector('[data-tool-toggle="collapse"]');

        if (search) {
          search.addEventListener('input', () => {
            const keyword = (search.value || '').trim().toLowerCase();
            details.forEach((item) => {
              const text = (item.textContent || '').toLowerCase();
              item.style.display = !keyword || text.includes(keyword) ? '' : 'none';
            });
          });
        }

        if (expandBtn) {
          expandBtn.addEventListener('click', () => {
            details.forEach((item) => {
              if (item.style.display !== 'none') item.open = true;
            });
          });
        }

        if (collapseBtn) {
          collapseBtn.addEventListener('click', () => {
            details.forEach((item) => { item.open = false; });
          });
        }
      });
    }

    function applyConversationFolds() {
      const bodies = Array.from(document.querySelectorAll('.entry-body'));
      bodies.forEach((body) => {
        body.classList.remove('is-collapsed');
        const existingTools = body.parentElement.querySelector('.entry-tools');
        if (existingTools) existingTools.remove();

        if (body.scrollHeight <= 520) return;

        body.classList.add('is-collapsed');
        const tools = document.createElement('div');
        tools.className = 'entry-tools';
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'toggle-btn';
        button.title = '展开全文';
        button.setAttribute('aria-label', '展开全文');
        button.innerHTML = '<svg class="toggle-btn-icon" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 8l5 5 5-5"/></svg>';
        button.addEventListener('click', () => {
          const collapsed = body.classList.toggle('is-collapsed');
          button.classList.toggle('is-expanded', !collapsed);
          button.title = collapsed ? '展开全文' : '收起内容';
          button.setAttribute('aria-label', collapsed ? '展开全文' : '收起内容');
        });
        tools.appendChild(button);
        body.after(tools);
      });
    }

    async function bootConversationView() {
      await renderConversationMarkdown();
      wireCopyButtons();
      wireToolSections();
      applyConversationFolds();
      buildConversationTOC();
    }

    bootConversationView();
  </script>
</body>
</html>`
