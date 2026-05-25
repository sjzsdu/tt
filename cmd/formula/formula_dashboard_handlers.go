package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tt/internal/formulaui"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formularun"
	"github.com/sjzsdu/tt/internal/formularunview"
	"github.com/sjzsdu/tt/internal/webui"
	"nhooyr.io/websocket"
)

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
	if target.Status != string(executor.StatusFailed) {
		http.Error(w, "only failed steps can be retried", http.StatusBadRequest)
		return
	}
	if snapshot.Status == "running" || snapshot.Status == "waiting_input" {
		http.Error(w, "run is already active", http.StatusConflict)
		return
	}
	recipe, err := formularun.LoadRecipe(s.store.Dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	initialResults, initialContext := formularunview.BuildResumeStateExcluding(recipe, snapshot, map[string]bool{resolvedStepID: true})
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
		if err := executeFormulaResumeWithAdvice(&cobra.Command{}, recipe, s.store, s, s.store.Meta.Vars, initialResults, initialContext, map[string]string{resolvedStepID: advice}); err != nil {
			s.logf("retry step %s failed: %v", resolvedStepID, err)
		}
	}()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
	}{OK: true})
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
	recipe, err := formularun.LoadRecipe(s.store.Dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	initialResults, initialContext := formularunview.BuildResumeState(recipe, snapshot)
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
		if err := executeFormulaResume(&cobra.Command{}, recipe, s.store, s, s.store.Meta.Vars, initialResults, initialContext); err != nil {
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
