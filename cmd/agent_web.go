package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
	"github.com/sjzsdu/tt/internal/webui"
)

type agentWebServer struct {
	mu        sync.Mutex
	server    *http.Server
	port      int
	started   time.Time
	workspace string
	cfg       ttconfig.Config
	rt        *pcwrap.Runtime
	runner    *pcwrap.DirectRunner
	embedded  []pcwrap.EmbeddedAgent
	defaults  agentWebDefaults
}

type agentWebDefaults struct {
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Session string `json:"session"`
	Debug   bool   `json:"debug"`
}

type agentWebAgent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

type agentWebState struct {
	StartedAt string           `json:"started_at"`
	Workspace string           `json:"workspace"`
	Defaults  agentWebDefaults `json:"defaults"`
	Agents    []agentWebAgent  `json:"agents"`
}

type agentWebChatRequest struct {
	Message string `json:"message"`
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Model   string `json:"model"`
}

type agentWebChatResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

type agentWebStreamEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
	Error string `json:"error,omitempty"`
}

type agentWebTranscriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agentWebTranscriptResponse struct {
	Session  string                      `json:"session"`
	Agent    string                      `json:"agent,omitempty"`
	Path     string                      `json:"path,omitempty"`
	Messages []agentWebTranscriptMessage `json:"messages,omitempty"`
	Missing  bool                        `json:"missing,omitempty"`
	Message  string                      `json:"message,omitempty"`
}

type agentWebSessionInfo struct {
	Session   string `json:"session"`
	Agent     string `json:"agent,omitempty"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

type agentWebSessionsResponse struct {
	Sessions []agentWebSessionInfo `json:"sessions"`
	Missing  bool                  `json:"missing,omitempty"`
	Message  string                `json:"message,omitempty"`
}

type agentWebGitFile struct {
	Path   string `json:"path"`
	Status string `json:"status,omitempty"`
}

type agentWebGitCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

type agentWebGitContextResponse struct {
	Branch  string              `json:"branch,omitempty"`
	Files   []agentWebGitFile   `json:"files,omitempty"`
	Commits []agentWebGitCommit `json:"commits,omitempty"`
	Diff    string              `json:"diff,omitempty"`
	Missing bool                `json:"missing,omitempty"`
	Message string              `json:"message,omitempty"`
}

const maxAgentWebTranscriptBytes = 256 * 1024

func runAgentWeb(cmd *cobra.Command, cfg ttconfig.Config, sources ttconfig.Sources, flags agentRunFlags) error {
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	cfg.Picoclaw.Home = resolvedHome
	cfg.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(cfg.Picoclaw.Home, cfg.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      cfg.Picoclaw.Home,
		Config:    cfg.Picoclaw.Config,
		TTConfig:  cfg,
		TTSources: sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	}

	embeddedAgents, err := agents.All()
	if err != nil {
		return fmt.Errorf("load agents failed: %w", err)
	}

	debug := flags.Debug
	if cfg.Agent.Debug != nil {
		debug = *cfg.Agent.Debug
	}
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          cfg.Agent.Agent,
		Model:          cfg.Agent.Model,
		Workspace:      workspace,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: embeddedAgents,
	})
	if err != nil {
		return picoclawUnavailableError(err, cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	}
	defer runner.Close()

	server := &agentWebServer{
		started:   time.Now(),
		workspace: workspace,
		cfg:       cfg,
		rt:        rt,
		runner:    runner,
		embedded:  embeddedAgents,
		defaults: agentWebDefaults{
			Agent:   cfg.Agent.Agent,
			Model:   cfg.Agent.Model,
			Session: cfg.Agent.Session,
			Debug:   debug,
		},
	}
	if server.defaults.Session == "" {
		server.defaults.Session = "cli:default"
	}
	if err := server.start(flags.WebPort); err != nil {
		return err
	}
	server.waitForInterrupt()
	return nil
}

func (s *agentWebServer) start(port int) error {
	if port <= 0 {
		port = 9710
	}
	mux := http.NewServeMux()
	mux.Handle("/favicon.svg", webui.AgentFaviconHandler())
	mux.Handle("/assets/", webui.AgentAssetsHandler())
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/transcript", s.handleTranscript)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/git/context", s.handleGitContext)

	maxPort := port + 20
	var lastErr error
	for candidate := port; candidate <= maxPort; candidate++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err != nil {
			lastErr = err
			continue
		}
		srv := &http.Server{Addr: fmt.Sprintf(":%d", candidate), Handler: mux}
		s.server = srv
		s.port = candidate
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				fmt.Printf("agent web server failed: %v\n", err)
			}
		}()
		url := fmt.Sprintf("http://localhost:%d", candidate)
		fmt.Printf("Agent web UI started: %s\n", url)
		fmt.Println("Press Ctrl-C to stop the web UI.")
		go openBrowser(url)
		return nil
	}
	return fmt.Errorf("all candidate agent web ports unavailable: %v", lastErr)
}

func (s *agentWebServer) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
		s.server = nil
	}
}

func (s *agentWebServer) waitForInterrupt() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	s.close()
}

func (s *agentWebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(webui.AgentIndex())
}

func (s *agentWebServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeAgentWebJSON(w, s.state())
}

func (s *agentWebServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req agentWebChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Agent) == "" {
		req.Agent = s.defaults.Agent
	}
	if strings.TrimSpace(req.Session) == "" {
		req.Session = s.defaults.Session
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = s.defaults.Model
	}

	s.mu.Lock()
	response, err := s.runner.ProcessDirectContext(r.Context(), pcwrap.RunOptions{
		Message:        req.Message,
		Agent:          req.Agent,
		Session:        req.Session,
		Model:          req.Model,
		Workspace:      s.workspace,
		Debug:          s.defaults.Debug,
		Quiet:          !s.defaults.Debug,
		EmbeddedAgents: s.embedded,
	})
	s.mu.Unlock()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeAgentWebJSON(w, agentWebChatResponse{Error: err.Error()})
		return
	}
	writeAgentWebJSON(w, agentWebChatResponse{Response: response})
}

func (s *agentWebServer) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	var req agentWebChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Agent) == "" {
		req.Agent = s.defaults.Agent
	}
	if strings.TrimSpace(req.Session) == "" {
		req.Session = s.defaults.Session
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = s.defaults.Model
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	writeAgentWebSSE(w, agentWebStreamEvent{Type: "start"})
	flusher.Flush()

	var streamed strings.Builder
	streamRunner, err := s.rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          req.Agent,
		Model:          req.Model,
		Workspace:      s.workspace,
		Debug:          s.defaults.Debug,
		Quiet:          !s.defaults.Debug,
		EmbeddedAgents: s.embedded,
		OnDelta: func(delta string) {
			if strings.TrimSpace(delta) == "" {
				return
			}
			streamed.WriteString(delta)
			writeAgentWebSSE(w, agentWebStreamEvent{Type: "delta", Delta: delta})
			flusher.Flush()
		},
	})
	if err != nil {
		writeAgentWebSSE(w, agentWebStreamEvent{Type: "error", Error: err.Error()})
		flusher.Flush()
		return
	}
	defer streamRunner.Close()

	response, err := streamRunner.ProcessDirectContext(r.Context(), pcwrap.RunOptions{
		Message:        req.Message,
		Agent:          req.Agent,
		Session:        req.Session,
		Model:          req.Model,
		Workspace:      s.workspace,
		Debug:          s.defaults.Debug,
		Quiet:          !s.defaults.Debug,
		EmbeddedAgents: s.embedded,
		OnDelta:        nil,
	})
	if err != nil {
		writeAgentWebSSE(w, agentWebStreamEvent{Type: "error", Error: err.Error()})
		flusher.Flush()
		return
	}
	streamedText := streamed.String()
	if streamedText == "" {
		for _, chunk := range splitAgentWebStreamChunks(response, 900) {
			writeAgentWebSSE(w, agentWebStreamEvent{Type: "delta", Delta: chunk})
			flusher.Flush()
		}
	} else if strings.HasPrefix(response, streamedText) && len(response) > len(streamedText) {
		writeAgentWebSSE(w, agentWebStreamEvent{Type: "delta", Delta: strings.TrimPrefix(response, streamedText)})
		flusher.Flush()
	}
	writeAgentWebSSE(w, agentWebStreamEvent{Type: "done"})
	flusher.Flush()
}

func (s *agentWebServer) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	if session == "" {
		session = s.defaults.Session
	}
	if agent == "" {
		agent = s.defaults.Agent
	}
	path, content, err := readAgentWebTranscript(s.workspace, session, agent)
	if err != nil {
		writeAgentWebJSON(w, agentWebTranscriptResponse{Session: session, Agent: agent, Missing: true, Message: err.Error()})
		return
	}
	messages := parseAgentWebTranscript(content)
	writeAgentWebJSON(w, agentWebTranscriptResponse{Session: session, Agent: agent, Path: path, Messages: messages})
}

func (s *agentWebServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions, err := listAgentWebSessions(s.workspace)
	if err != nil {
		writeAgentWebJSON(w, agentWebSessionsResponse{Missing: true, Message: err.Error()})
		return
	}
	writeAgentWebJSON(w, agentWebSessionsResponse{Sessions: sessions})
}

func (s *agentWebServer) handleGitContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	file := strings.TrimSpace(r.URL.Query().Get("file"))
	ctx, err := readAgentWebGitContext(s.workspace, file)
	if err != nil {
		writeAgentWebJSON(w, agentWebGitContextResponse{Missing: true, Message: err.Error()})
		return
	}
	writeAgentWebJSON(w, ctx)
}

func (s *agentWebServer) state() agentWebState {
	agentsOut := make([]agentWebAgent, 0, len(s.embedded))
	for _, agent := range s.embedded {
		agentsOut = append(agentsOut, agentWebAgent{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Aliases:     agent.Aliases,
			Skills:      agent.Skills,
			Tools:       agent.Tools,
		})
	}
	return agentWebState{
		StartedAt: s.started.Format(time.RFC3339),
		Workspace: s.workspace,
		Defaults:  s.defaults,
		Agents:    agentsOut,
	}
}

func writeAgentWebJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func writeAgentWebSSE(w io.Writer, event agentWebStreamEvent) {
	payload, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
}

func splitAgentWebStreamChunks(text string, maxRunes int) []string {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 900
	}
	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func readAgentWebGitContext(workspace, file string) (agentWebGitContextResponse, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return agentWebGitContextResponse{}, fmt.Errorf("workspace is not available")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		return agentWebGitContextResponse{}, fmt.Errorf("workspace is not a git repository")
	}
	branch, _ := runAgentWebGit(workspace, "branch", "--show-current")
	status, _ := runAgentWebGit(workspace, "status", "--short")
	log, _ := runAgentWebGit(workspace, "log", "--oneline", "-5")
	resp := agentWebGitContextResponse{
		Branch:  strings.TrimSpace(branch),
		Files:   parseAgentWebGitStatus(status),
		Commits: parseAgentWebGitLog(log),
	}
	file = strings.TrimSpace(file)
	if file != "" {
		if strings.Contains(file, "\x00") || filepath.IsAbs(file) || strings.HasPrefix(filepath.Clean(file), "..") {
			return resp, fmt.Errorf("invalid file path")
		}
		diff, err := runAgentWebGit(workspace, "diff", "--", file)
		if err == nil && strings.TrimSpace(diff) == "" {
			diff, _ = runAgentWebGit(workspace, "diff", "--cached", "--", file)
		}
		resp.Diff = diff
	}
	return resp, nil
}

func runAgentWebGit(workspace string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func parseAgentWebGitStatus(output string) []agentWebGitFile {
	lines := strings.Split(output, "\n")
	files := make([]agentWebGitFile, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		status := strings.TrimSpace(line[:min(len(line), 2)])
		path := strings.TrimSpace(line[min(len(line), 3):])
		if path == "" {
			path = strings.TrimSpace(line)
		}
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		files = append(files, agentWebGitFile{Path: path, Status: status})
	}
	return files
}

func parseAgentWebGitLog(output string) []agentWebGitCommit {
	lines := strings.Split(output, "\n")
	commits := make([]agentWebGitCommit, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		commit := agentWebGitCommit{Hash: parts[0]}
		if len(parts) > 1 {
			commit.Subject = parts[1]
		}
		commits = append(commits, commit)
	}
	return commits
}

func readAgentWebTranscript(workspace, session, agent string) (string, string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", "", fmt.Errorf("workspace is not available")
	}
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", "", fmt.Errorf("read sessions dir: %w", err)
	}
	slug := agentWebSessionFilenameToken(session)
	agentPrefix := ""
	if strings.TrimSpace(agent) != "" {
		agentPrefix = "agent_" + agentWebSessionFilenameToken(agent) + "_"
	}
	var best string
	var bestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := entry.Name()
		if agentPrefix != "" && !strings.HasPrefix(name, agentPrefix) {
			continue
		}
		if slug != "" && !strings.Contains(name, slug) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best = filepath.Join(sessionsDir, name)
			bestMod = info.ModTime()
		}
	}
	if best == "" {
		return "", "", fmt.Errorf("session transcript not found under %s", sessionsDir)
	}
	content, err := readAgentWebFileTail(best, maxAgentWebTranscriptBytes)
	if err != nil {
		return "", "", err
	}
	return best, content, nil
}

func listAgentWebSessions(workspace string) ([]agentWebSessionInfo, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is not available")
	}
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}
	sessions := make([]agentWebSessionInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		agent, session := parseAgentWebSessionFilename(entry.Name())
		if session == "" {
			continue
		}
		sessions = append(sessions, agentWebSessionInfo{
			Session:   session,
			Agent:     agent,
			Path:      filepath.Join(sessionsDir, entry.Name()),
			UpdatedAt: info.ModTime().Format(time.RFC3339),
			Size:      info.Size(),
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	return sessions, nil
}

func parseAgentWebSessionFilename(name string) (agent, session string) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".jsonl")
	if name == "" {
		return "", ""
	}
	const prefix = "agent_"
	if !strings.HasPrefix(name, prefix) {
		return "", name
	}
	rest := strings.TrimPrefix(name, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return "", rest
	}
	return parts[0], parts[1]
}

func parseAgentWebTranscript(content string) []agentWebTranscriptMessage {
	lines := strings.Split(content, "\n")
	messages := make([]agentWebTranscriptMessage, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "...") {
			continue
		}
		var raw struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		role := strings.TrimSpace(raw.Role)
		if role == "" {
			role = "assistant"
		}
		content := strings.TrimSpace(string(raw.Content))
		var text string
		if len(raw.Content) > 0 {
			if err := json.Unmarshal(raw.Content, &text); err != nil {
				var value any
				if err := json.Unmarshal(raw.Content, &value); err == nil {
					if pretty, err := json.MarshalIndent(value, "", "  "); err == nil {
						text = string(pretty)
					}
				}
				if text == "" {
					text = strings.TrimSpace(string(raw.Content))
				}
			}
		}
		text = strings.TrimSpace(text)
		if text == "" && content != "" && content != "null" {
			text = content
		}
		if text == "" {
			continue
		}
		messages = append(messages, agentWebTranscriptMessage{Role: role, Content: text})
	}
	return messages
}

func agentWebSessionFilenameToken(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		keep := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-'
		if keep {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func readAgentWebFileTail(path string, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	offset := int64(0)
	if info.Size() > maxBytes {
		offset = info.Size() - maxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	if offset > 0 {
		return "... transcript truncated to last 256 KiB ...\n" + string(data), nil
	}
	return string(data), nil
}
