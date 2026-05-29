package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formularun"
	"github.com/sjzsdu/tt/internal/formularunview"
	"github.com/sjzsdu/tt/internal/formulaui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/sjzsdu/tt/internal/webui"
	"nhooyr.io/websocket"
)

const finalReportChatAgent = "coder"

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
		Type  string             `json:"type"`
		State formulaui.Snapshot `json:"state"`
	}{Type: "state", State: s.snapshot()})
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
	resolvedStepID, err := formularunview.ResolveStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var target *formulaui.Step
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
	if target.Status != formulaui.StatusFailed {
		http.Error(w, "only failed steps can be retried", http.StatusBadRequest)
		return
	}
	if snapshot.Status == "running" || snapshot.Status == "waiting_input" {
		http.Error(w, "run is already active", http.StatusConflict)
		return
	}
	initialResults, initialContext := formularunview.BuildResumeStateExcluding(snapshot, map[string]bool{resolvedStepID: true})
	formularunview.ResetStepForRetry(&snapshot, resolvedStepID)
	snapshot.Status = "running"
	snapshot.Error = ""
	s.store.Meta.Status = formularun.StatusRunning
	s.store.Meta.Error = ""
	s.store.Meta.FinishedAt = ""
	s.store.Meta.PID = os.Getpid()
	s.store.Meta.TTVersion = version
	_ = s.store.SaveMetadata()
	_ = s.store.AppendEvent(formularun.Event{Type: "step_retry_requested", StepID: resolvedStepID, Status: formularun.StatusRunning})
	s.mu.Lock()
	s.state = formulaui.CloneSnapshot(snapshot)
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
		OK   bool                       `json:"ok"`
		Chat *formulaui.FinalReportChat `json:"chat,omitempty"`
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
	userMsg := formulaui.FinalReportChatMessage{Role: "user", Content: prompt, At: time.Now().Format(time.RFC3339)}
	s.mu.Lock()
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, userMsg)
	s.state.FinalReportChat.Status = "running"
	s.state.FinalReportChat.Error = ""
	s.mu.Unlock()
	s.broadcast()

	processor, err := s.finalReportDirectProcessor()
	if err != nil {
		s.failFinalReportChat(err)
		s.broadcast()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response, err := processor.ProcessDirectContext(r.Context(), pcwrap.RunOptions{
		Agent:     finalReportChatAgent,
		Session:   chat.SessionID,
		Message:   s.buildFinalReportChatPrompt(prompt),
		Workspace: s.snapshot().WorkspaceDir,
		Quiet:     true,
	})
	if err != nil {
		s.failFinalReportChat(err)
		s.broadcast()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	assistantMsg := formulaui.FinalReportChatMessage{Role: "assistant", Content: strings.TrimSpace(response), At: time.Now().Format(time.RFC3339)}
	s.mu.Lock()
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, assistantMsg)
	s.state.FinalReportChat.Status = "idle"
	s.state.FinalReportChat.Error = ""
	s.mu.Unlock()
	s.broadcast()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
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
	s.state.FinalReportChat.Messages = append(s.state.FinalReportChat.Messages, formulaui.FinalReportChatMessage{Role: "system", Content: "Promoted latest assistant response to final report.", At: time.Now().Format(time.RFC3339)})
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

func (s *formulaDashboardServer) ensureFinalReportChat() (*formulaui.FinalReportChat, error) {
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
		s.state.FinalReportChat = &formulaui.FinalReportChat{
			SessionID: runID + ":final-report-chat",
			Agent:     finalReportChatAgent,
			Status:    "idle",
			Messages:  []formulaui.FinalReportChatMessage{},
		}
		needsBroadcast = true
	}
	chat := *s.state.FinalReportChat
	chat.Messages = append([]formulaui.FinalReportChatMessage(nil), s.state.FinalReportChat.Messages...)
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
	runner, err := runtime.Runtime.NewDirectRunner(pcwrap.RunOptions{Workspace: s.snapshot().WorkspaceDir, Quiet: true, EmbeddedAgents: embeddedAgents})
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
	if workspace := strings.TrimSpace(snapshot.WorkspaceDir); workspace != "" {
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
	resolvedStepID, err := formularunview.ResolveWaitingInputStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var request formulaui.HumanInputRequest
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
	if err := formularunview.MarkStepCompletedWithOutput(&snapshot, resolvedStepID, output); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	snapshot.Status = "running"
	snapshot.Error = ""
	if err := s.store.SaveState(snapshot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.store.AppendEvent(formularun.Event{Type: "human_input_submitted", StepID: resolvedStepID, Status: "completed"})
	initialResults, initialContext := formularunview.BuildResumeState(snapshot)
	s.store.Meta.Status = formularun.StatusRunning
	s.store.Meta.Error = ""
	s.store.Meta.FinishedAt = ""
	s.store.Meta.PID = os.Getpid()
	s.store.Meta.TTVersion = version
	_ = s.store.SaveMetadata()
	_ = s.store.AppendEvent(formularun.Event{Type: "run_resumed", Status: formularun.StatusRunning})
	formularunview.ResetForResume(&snapshot)
	s.mu.Lock()
	s.state = formulaui.CloneSnapshot(snapshot)
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
