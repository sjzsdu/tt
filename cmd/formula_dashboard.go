package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularun"
	"github.com/sjzsdu/tt/internal/webui"
	"nhooyr.io/websocket"
)

type formulaDashboardServer struct {
	mu       sync.Mutex
	server   *http.Server
	port     int
	started  time.Time
	recipe   string
	state    formulaDashboardSnapshot
	store    *formularun.Store
	readonly bool
	clients  map[*websocket.Conn]struct{}
	shutdown chan struct{}
}

func newFormulaDashboardServerFromSnapshot(snapshot formulaDashboardSnapshot) *formulaDashboardServer {
	return &formulaDashboardServer{
		started:  time.Now(),
		recipe:   snapshot.RecipeName,
		state:    cloneFormulaDashboardSnapshot(snapshot),
		clients:  map[*websocket.Conn]struct{}{},
		shutdown: make(chan struct{}),
		readonly: true,
	}
}

func (s *formulaDashboardServer) attachStore(store *formularun.Store) {
	if s == nil {
		return
	}
	s.store = store
	if store != nil {
		s.state.RunID = store.Meta.RunID
	}
	_ = s.persistSnapshot()
}

func newFormulaDashboardServer(recipe *formula.Recipe) *formulaDashboardServer {
	steps, edges := buildFormulaDashboardGraph(recipe)
	return &formulaDashboardServer{
		started: time.Now(),
		recipe:  recipe.Name,
		state: formulaDashboardSnapshot{
			RecipeName:  recipe.Name,
			Description: recipe.Description,
			Phase:       recipe.Phase,
			Status:      "running",
			Steps:       steps,
			Edges:       edges,
			Logs:        []formulaDashboardLogEntry{},
		},
		clients:  map[*websocket.Conn]struct{}{},
		shutdown: make(chan struct{}),
	}
}

func (s *formulaDashboardServer) start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return fmt.Errorf("formula dashboard already running on port %d", s.port)
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.FormulaAssetsHandler())
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/human-input", s.handleHumanInput)
	mux.HandleFunc("/api/retry-step", s.handleRetryStep)
	mux.HandleFunc("/ws", s.handleWS)

	maxPort := port + 20
	var lastErr error
	for candidate := port; candidate <= maxPort; candidate++ {
		srv := &http.Server{Addr: fmt.Sprintf(":%d", candidate), Handler: mux}
		errCh := make(chan error, 1)
		go func() {
			err := srv.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
		time.Sleep(120 * time.Millisecond)
		select {
		case err := <-errCh:
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				lastErr = err
				continue
			}
			return err
		default:
			s.server = srv
			s.port = candidate
			fmt.Printf("Formula dashboard started: http://localhost:%d\n", candidate)
			go openBrowser(fmt.Sprintf("http://localhost:%d", candidate))
			return nil
		}
	}
	return fmt.Errorf("all candidate dashboard ports unavailable: %v", lastErr)
}

func (s *formulaDashboardServer) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.clients {
		_ = conn.Close(websocket.StatusNormalClosure, "server closed")
	}
	s.clients = map[*websocket.Conn]struct{}{}
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
		s.server = nil
	}
}

func (s *formulaDashboardServer) waitForInterrupt() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	close(s.shutdown)
	s.close()
}

func (s *formulaDashboardServer) snapshot() formulaDashboardSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneFormulaDashboardSnapshot(s.state)
}

func (s *formulaDashboardServer) snapshotMessageLocked() []byte {
	msg := formulaDashboardMessage{Type: "state", State: cloneFormulaDashboardSnapshot(s.state)}
	b, _ := json.Marshal(msg)
	return b
}

func (s *formulaDashboardServer) broadcast() {
	_ = s.persistSnapshot()
	s.mu.Lock()
	payload := s.snapshotMessageLocked()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for conn := range s.clients {
		clients = append(clients, conn)
	}
	s.mu.Unlock()
	ctx := context.Background()
	for _, conn := range clients {
		if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
	}
}

func (s *formulaDashboardServer) persistSnapshot() error {
	if s == nil || s.store == nil || s.readonly {
		return nil
	}
	s.mu.Lock()
	snapshot := cloneFormulaDashboardSnapshot(s.state)
	s.mu.Unlock()
	return s.store.SaveState(snapshot)
}

func (s *formulaDashboardServer) logf(format string, args ...any) {
	s.mu.Lock()
	s.state.Logs = append(s.state.Logs, formulaDashboardLogEntry{At: time.Now().Format("15:04:05"), Text: fmt.Sprintf(format, args...)})
	if len(s.state.Logs) > 200 {
		s.state.Logs = append([]formulaDashboardLogEntry(nil), s.state.Logs[len(s.state.Logs)-200:]...)
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepRunning(stepID, title, agent, model, session string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		if title != "" {
			s.state.Steps[i].Title = title
		}
		s.state.Steps[i].Agent = agent
		s.state.Steps[i].Model = model
		s.state.Steps[i].Session = session
		s.state.Steps[i].Status = "running"
		s.state.Steps[i].StartedAt = time.Now().Format(time.RFC3339)
		s.state.Steps[i].FinishedAt = ""
		s.state.Steps[i].DurationMS = 0
		s.state.Steps[i].Error = ""
		s.state.Steps[i].Output = ""
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: "running", Detail: fmt.Sprintf("Agent %s started this step", agent)})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "running", fmt.Sprintf("Agent %s started loop body", agent), "", "", 0)
	}
	if s.state.Status == "pending" {
		s.state.Status = "running"
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepCompleted(stepID, output string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "completed"
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "completed", Detail: fmt.Sprintf("Completed with %d chars of output", len(output)), Output: output, DurationMS: s.state.Steps[i].DurationMS})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "completed", fmt.Sprintf("Completed with %d chars of output", len(output)), output, "", 0)
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepFailed(stepID, errMsg, output string) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		s.state.Steps[i].Status = "failed"
		s.state.Steps[i].Error = errMsg
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "failed", Detail: errMsg, Output: output, Error: errMsg, DurationMS: s.state.Steps[i].DurationMS})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, "", "failed", errMsg, output, errMsg, 0)
	}
	s.state.Status = "failed"
	s.state.Error = errMsg
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepWaitingInput(stepID, title string, request *executor.HumanInputRequest) {
	s.mu.Lock()
	found := false
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		found = true
		if title != "" {
			s.state.Steps[i].Title = title
		}
		s.state.Steps[i].Status = "waiting_input"
		if request != nil {
			s.state.Steps[i].HumanInputRequest = request
		}
		s.state.Steps[i].FinishedAt = ""
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: s.state.Steps[i].Title, Status: "waiting_input", Detail: "Waiting for human input"})
		break
	}
	if !found {
		s.markLoopActivityLocked(stepID, title, "waiting_input", "Waiting for human input", "", "", 0)
	}
	s.state.Status = "waiting_input"
	s.state.Error = ""
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markLoopActivityLocked(stepID, title, status, detail, output, errMsg string, durationMS int64) {
	parentID := loopParentStepID(stepID)
	if parentID == "" {
		return
	}
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != parentID {
			continue
		}
		if status == "running" && s.state.Steps[i].Status == "pending" {
			s.state.Steps[i].Status = "running"
			s.state.Steps[i].StartedAt = time.Now().Format(time.RFC3339)
		}
		appendStepActivity(&s.state.Steps[i], formulaStepActivity{At: time.Now().Format("15:04:05"), StepID: stepID, Title: title, Status: status, Detail: detail, Output: output, Error: errMsg, DurationMS: durationMS})
		return
	}
}

func (s *formulaDashboardServer) finalize(result *executor.RunResult, runErr error) {
	s.mu.Lock()
	if result != nil {
		s.state.RecipeName = result.RecipeName
		s.state.FinalOutput = result.FinalOutput
		s.state.Status = "completed"
		if result.WaitingInput > 0 {
			s.state.Status = "waiting_input"
		}
		var waitingErr executor.WaitingInputError
		if runErr != nil && !errors.As(runErr, &waitingErr) {
			s.state.Status = "failed"
			s.state.Error = runErr.Error()
		}
		for _, step := range result.Steps {
			for i := range s.state.Steps {
				if s.state.Steps[i].ID != step.StepID {
					continue
				}
				s.state.Steps[i].Title = step.Title
				s.state.Steps[i].Status = string(step.Status)
				s.state.Steps[i].Output = step.Output
				s.state.Steps[i].Error = step.Error
				if step.Status == executor.StatusSkipped {
					s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
				}
				break
			}
		}
	}
	s.mu.Unlock()
	s.broadcast()
}

func renderFormulaPrompt(cwd, prompt string) string {
	if strings.TrimSpace(cwd) == "" {
		return prompt
	}
	return fmt.Sprintf("Project root: %s\n\n%s", cwd, prompt)
}

func waitForFormulaDashboardExit(d *formulaDashboardServer) {
	if d == nil {
		return
	}
	d.waitForInterrupt()
}

func formulaAgentWorkspace(cwd string) string {
	return formulaDashboardWorkspace(cwd)
}

func formulaDashboardWorkspace(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, ".tt")
}
