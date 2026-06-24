package formulacmd

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

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula/run"
	"github.com/sjzsdu/tt/internal/formula/runview"
	"github.com/sjzsdu/tt/internal/formula/ui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/sjzsdu/tt/internal/webui"
	"nhooyr.io/websocket"
)

const finalReportChatAgent = "coder"
const maxAgentSessionBytes = 256 * 1024

func (s *formulaDashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(webui.FormulaIndex()); err != nil {
		http.Error(w, fmt.Sprintf("render formula dashboard failed: %v", err), http.StatusInternalServerError)
	}
}

func (s *formulaDashboardServer) handleState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Type  string      `json:"type"`
		State ui.Snapshot `json:"state"`
	}{Type: "state", State: s.snapshot()})
}

func (s *formulaDashboardServer) handleStopRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.store == nil {
		http.Error(w, "dashboard is not attached to a run store", http.StatusBadRequest)
		return
	}
	stopFile := filepath.Join(s.store.Dir, "stop-requested")
	payload := fmt.Sprintf("requested_at=%s\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(stopFile, []byte(payload), 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.store.AppendEvent(run.Event{Type: "stop_requested", Status: run.StatusRunning})
	s.mu.Lock()
	s.state.StopRequested = true
	s.state.Logs = append(s.state.Logs, ui.LogEntry{At: time.Now().Format("15:04:05"), Text: "Graceful stop requested. The current iteration will finish before exit."})
	s.mu.Unlock()
	s.broadcast()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK       bool   `json:"ok"`
		StopFile string `json:"stop_file"`
	}{OK: true, StopFile: stopFile})
}

type agentSessionResponse struct {
	Session string `json:"session"`
	Agent   string `json:"agent,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Missing bool   `json:"missing,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *formulaDashboardServer) handleAgentSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	if session == "" {
		http.Error(w, "session is required", http.StatusBadRequest)
		return
	}
	workspace := ""
	if s.store != nil {
		workspace = strings.TrimSpace(s.store.Meta.WorkspaceDir)
	}
	if workspace == "" {
		workspace = strings.TrimSpace(s.snapshot().WorkspaceDir)
	}
	if workspace == "" {
		http.Error(w, "workspace is not available", http.StatusBadRequest)
		return
	}
	path, content, err := readAgentSessionTranscript(workspace, session, agent)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(agentSessionResponse{Session: session, Agent: agent, Missing: true, Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(agentSessionResponse{Session: session, Agent: agent, Path: path, Content: content})
}

func readAgentSessionTranscript(workspace, session, agent string) (string, string, error) {
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", "", fmt.Errorf("read sessions dir: %w", err)
	}
	slug := sessionFilenameToken(session)
	agentPrefix := ""
	if strings.TrimSpace(agent) != "" {
		agentPrefix = "agent_" + sessionFilenameToken(agent) + "_"
	}
	var best string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := entry.Name()
		if agentPrefix != "" && !strings.HasPrefix(name, agentPrefix) {
			continue
		}
		if !strings.Contains(name, slug) {
			continue
		}
		best = filepath.Join(sessionsDir, name)
		break
	}
	if best == "" {
		return "", "", fmt.Errorf("session transcript not found under %s", sessionsDir)
	}
	content, err := readFileTail(best, maxAgentSessionBytes)
	if err != nil {
		return "", "", err
	}
	return best, content, nil
}

func sessionFilenameToken(value string) string {
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

func readFileTail(path string, maxBytes int64) (string, error) {
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

type formulaHumanInputSubmitRequest struct {
	StepID   string         `json:"step_id"`
	Response map[string]any `json:"response"`
}

type formulaRetryStepRequest struct {
	StepID string `json:"step_id"`
	Advice string `json:"advice,omitempty"`
}

type finalReportChatMessageRequest struct {
	Message string `json:"message"`
}

type formulaConfirmRepairRequest struct {
	StepID  string `json:"step_id"`
	Attempt int    `json:"attempt"`
}

func (s *formulaDashboardServer) handleRetryStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.readonly || s.store == nil {
		http.Error(w, "dashboard is read-only or not attached to a run store", http.StatusBadRequest)
		return
	}
	var req formulaRetryStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.StepID) == "" {
		http.Error(w, "step_id is required", http.StatusBadRequest)
		return
	}
	snapshot := s.snapshot()
	resolvedStepID, err := runview.ResolveStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var target *ui.Step
	for i := range snapshot.Steps {
		if snapshot.Steps[i].ID == resolvedStepID {
			target = &snapshot.Steps[i]
			break
		}
	}
	if target == nil {
		http.Error(w, fmt.Sprintf("step %q not found", resolvedStepID), http.StatusBadRequest)
		return
	}
	if target.Status != ui.StatusFailed {
		http.Error(w, "only failed steps can be retried", http.StatusBadRequest)
		return
	}
	if snapshot.Status == "running" || snapshot.Status == "waiting_input" {
		http.Error(w, "run is already active", http.StatusConflict)
		return
	}
	initialResults, initialContext := runview.BuildResumeStateExcluding(snapshot, map[string]bool{resolvedStepID: true})
	runview.ResetStepForRetry(&snapshot, resolvedStepID)
	snapshot.Status = "running"
	snapshot.Error = ""
	s.store.Meta.Status = run.StatusRunning
	s.store.Meta.Error = ""
	s.store.Meta.FinishedAt = ""
	s.store.Meta.PID = os.Getpid()
	s.store.Meta.TTVersion = version
	_ = s.store.SaveMetadata()
	_ = s.store.AppendEvent(run.Event{Type: "step_retry_requested", StepID: resolvedStepID, Status: run.StatusRunning})
	s.mu.Lock()
	s.state = ui.CloneSnapshot(snapshot)
	s.readonly = false
	s.mu.Unlock()
	s.broadcast()
	advice := strings.TrimSpace(req.Advice)
	go func() {
		if err := executeFormulaResumeWithAdvice(&cobra.Command{}, s.store.Meta.Formula, s.store, s, s.store.Meta.Vars, initialResults, initialContext, map[string]string{resolvedStepID: advice}); err != nil {
			s.logf("retry step %s failed: %v", resolvedStepID, err)
		}
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *formulaDashboardServer) handleConfirmRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.store == nil {
		http.Error(w, "dashboard is not attached to a run store", http.StatusBadRequest)
		return
	}
	var req formulaConfirmRepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.StepID) == "" || req.Attempt <= 0 {
		http.Error(w, "step_id and positive attempt are required", http.StatusBadRequest)
		return
	}
	if !s.confirmRepair(req.StepID, req.Attempt) {
		http.Error(w, "repair record not found", http.StatusNotFound)
		return
	}
	if err := s.persistSnapshot(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.SaveRepairs(s.snapshot().Repairs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.broadcast()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *formulaDashboardServer) handleFinalReportChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := s.ensureFinalReportChat(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK   bool                `json:"ok"`
		Chat *ui.FinalReportChat `json:"chat,omitempty"`
	}{OK: true, Chat: s.snapshot().FinalReportChat})
}

func (s *formulaDashboardServer) handleFinalReportChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req finalReportChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	chat, err := s.ensureFinalReportChat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userMsg := ui.FinalReportChatMessage{Role: "user", Content: prompt, At: time.Now().Format(time.RFC3339)}
	s.mu.Lock()
	if s.state.FinalReportChat != nil && strings.EqualFold(strings.TrimSpace(s.state.FinalReportChat.Status), "running") {
		s.mu.Unlock()
		http.Error(w, "final report chat is already running", http.StatusConflict)
		return
	}
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, userMsg)
	s.state.FinalReportChat.Status = "running"
	s.state.FinalReportChat.Error = ""
	s.mu.Unlock()
	s.broadcast()

	sessionID := chat.SessionID
	chatPrompt := s.buildFinalReportChatPrompt(prompt)
	workspace := s.finalReportChatWorkspace()
	go s.processFinalReportChatMessage(sessionID, chatPrompt, workspace)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *formulaDashboardServer) processFinalReportChatMessage(sessionID, prompt, workspace string) {
	processor, err := s.finalReportDirectProcessor()
	if err != nil {
		s.failFinalReportChat(err)
		s.broadcast()
		return
	}
	response, err := processor.ProcessDirectContext(context.Background(), pcwrap.RunOptions{
		Agent:     finalReportChatAgent,
		Session:   sessionID,
		Message:   prompt,
		Workspace: workspace,
		Quiet:     true,
	})
	if err != nil {
		s.failFinalReportChat(err)
		s.broadcast()
		return
	}
	assistantMsg := ui.FinalReportChatMessage{Role: "assistant", Content: strings.TrimSpace(response), At: time.Now().Format(time.RFC3339)}
	s.mu.Lock()
	if s.state.FinalReportChat == nil {
		s.mu.Unlock()
		return
	}
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, assistantMsg)
	s.state.FinalReportChat.Status = "idle"
	s.state.FinalReportChat.Error = ""
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) handleFinalReportChatPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	if s.state.FinalReportChat == nil || len(s.state.FinalReportChat.Messages) == 0 {
		s.mu.Unlock()
		http.Error(w, "no final report chat response is available", http.StatusBadRequest)
		return
	}
	var promoted string
	for i := len(s.state.FinalReportChat.Messages) - 1; i >= 0; i-- {
		msg := s.state.FinalReportChat.Messages[i]
		if strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") && strings.TrimSpace(msg.Content) != "" {
			promoted = strings.TrimSpace(msg.Content)
			break
		}
	}
	if promoted == "" {
		s.mu.Unlock()
		http.Error(w, "no assistant response is available to promote", http.StatusBadRequest)
		return
	}
	s.state.FinalOutput = promoted
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, ui.FinalReportChatMessage{Role: "system", Content: "Promoted latest assistant response to final report.", At: time.Now().Format(time.RFC3339)})
	s.state.FinalReportChat.Status = "idle"
	s.state.FinalReportChat.Error = ""
	s.appendLogLocked("Final report updated from chat")
	s.mu.Unlock()
	s.broadcast()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *formulaDashboardServer) failFinalReportChat(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.FinalReportChat == nil {
		return
	}
	s.state.FinalReportChat.Status = "failed"
	s.state.FinalReportChat.Error = err.Error()
}

func (s *formulaDashboardServer) ensureFinalReportChat() (*ui.FinalReportChat, error) {
	needsBroadcast := false
	s.mu.Lock()
	if strings.TrimSpace(s.state.FinalOutput) == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("final report is not available")
	}
	if s.state.FinalReportChat == nil {
		runID := strings.TrimSpace(s.state.RunID)
		if runID == "" {
			runID = strings.TrimSpace(s.recipe)
		}
		if runID == "" {
			runID = "formula"
		}
		s.state.FinalReportChat = &ui.FinalReportChat{
			SessionID: runID + ":final-report-chat",
			Agent:     finalReportChatAgent,
			Status:    "idle",
			Messages:  []ui.FinalReportChatMessage{},
		}
		needsBroadcast = true
	}
	chat := *s.state.FinalReportChat
	chat.Messages = append([]ui.FinalReportChatMessage(nil), s.state.FinalReportChat.Messages...)
	s.mu.Unlock()
	if needsBroadcast {
		s.broadcast()
	}
	return &chat, nil
}

func (s *formulaDashboardServer) finalReportDirectProcessor() (formulaDashboardDirectProcessor, error) {
	if s.directProcessor != nil {
		return s.directProcessor, nil
	}
	runtime, err := newFormulaPicoclawRuntime("")
	if err != nil {
		return nil, err
	}
	if err := runtime.Validate(); err != nil {
		return nil, err
	}
	embeddedAgents, err := agents.List()
	if err != nil {
		return nil, fmt.Errorf("list embedded agents failed: %w", err)
	}
	runner, err := runtime.Runtime.NewDirectRunner(pcwrap.RunOptions{Workspace: s.finalReportChatWorkspace(), Quiet: true, EmbeddedAgents: embeddedAgents})
	if err != nil {
		return nil, runtime.UnavailableError(err)
	}
	s.directProcessor = runner
	return s.directProcessor, nil
}

func (s *formulaDashboardServer) buildFinalReportChatPrompt(message string) string {
	snapshot := s.snapshot()
	var b strings.Builder
	b.WriteString("You are continuing work after a formula run completed. The user's goal is to close the remaining gap between the produced result and their expectation.\n")
	b.WriteString("Do not treat this as a passive Q&A unless the user only asks a question. If the user asks for changes, inspect and modify the repository, run focused validation, and report what changed.\n")
	if workspace := s.finalReportChatWorkspace(); workspace != "" {
		b.WriteString("All repository operations MUST happen in this workspace: ")
		b.WriteString(workspace)
		b.WriteString("\n")
	}
	b.WriteString("Keep the conversation continuous: use prior messages, avoid asking the user to repeat context, and drive toward a shippable final state.\n\n")
	b.WriteString("Final report:\n")
	b.WriteString(snapshot.FinalOutput)
	if chat := snapshot.FinalReportChat; chat != nil && len(chat.Messages) > 0 {
		b.WriteString("\n\nConversation so far:\n")
		for _, item := range chat.Messages {
			role := strings.TrimSpace(item.Role)
			if role == "" {
				role = "user"
			}
			label := strings.ToUpper(role[:1]) + role[1:]
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(item.Content)
			b.WriteString("\n\n")
		}
	}
	b.WriteString("User request:\n")
	b.WriteString(message)
	return b.String()
}

func (s *formulaDashboardServer) finalReportChatWorkspace() string {
	if s == nil {
		return ""
	}
	return formulaCodeWorkspace(s.snapshot().WorkspaceDir)
}

func (s *formulaDashboardServer) handleHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.readonly || s.store == nil {
		http.Error(w, "dashboard is read-only or not attached to a run store", http.StatusBadRequest)
		return
	}
	var req formulaHumanInputSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.StepID) == "" {
		http.Error(w, "step_id is required", http.StatusBadRequest)
		return
	}
	if len(req.Response) == 0 {
		http.Error(w, "response is required", http.StatusBadRequest)
		return
	}
	snapshot := s.snapshot()
	resolvedStepID, err := runview.ResolveWaitingInputStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var request ui.HumanInputRequest
	if err := s.store.LoadStepHumanInputRequest(resolvedStepID, &request); err != nil {
		http.Error(w, fmt.Sprintf("load human input request failed: %v", err), http.StatusInternalServerError)
		return
	}
	if err := validateHumanInputResponse(&request, req.Response); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	outputBytes, err := json.MarshalIndent(req.Response, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	output := string(outputBytes)
	if err := s.store.SaveStepHumanInputResponse(resolvedStepID, req.Response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.SaveStepOutput(resolvedStepID, output); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := runview.MarkStepCompletedWithOutput(&snapshot, resolvedStepID, output); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	snapshot.Status = "running"
	snapshot.Error = ""
	if err := s.store.SaveState(snapshot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.store.AppendEvent(run.Event{Type: "human_input_submitted", StepID: resolvedStepID, Status: "completed"})
	initialResults, initialContext := runview.BuildResumeState(snapshot)
	s.store.Meta.Status = run.StatusRunning
	s.store.Meta.Error = ""
	s.store.Meta.FinishedAt = ""
	s.store.Meta.PID = os.Getpid()
	s.store.Meta.TTVersion = version
	_ = s.store.SaveMetadata()
	_ = s.store.AppendEvent(run.Event{Type: "run_resumed", Status: run.StatusRunning})
	runview.ResetForResume(&snapshot)
	s.mu.Lock()
	s.state = ui.CloneSnapshot(snapshot)
	s.readonly = false
	s.mu.Unlock()
	s.broadcast()
	go func() {
		if err := executeFormulaResume(&cobra.Command{}, s.store.Meta.Formula, s.store, s, s.store.Meta.Vars, initialResults, initialContext); err != nil {
			s.logf("resume after human input failed: %v", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (s *formulaDashboardServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	s.mu.Lock()
	s.clients[conn] = struct{}{}
	payload := s.snapshotMessageLocked()
	s.mu.Unlock()
	ctx := context.Background()
	_ = conn.Write(ctx, websocket.MessageText, payload)
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}
