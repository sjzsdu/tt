package cmd

import (
	"context"
	"encoding/json"
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
	clients  map[*websocket.Conn]struct{}
	shutdown chan struct{}
}

type formulaDashboardSnapshot struct {
	RecipeName   string                     `json:"recipe_name"`
	Description  string                     `json:"description,omitempty"`
	Phase        string                     `json:"phase,omitempty"`
	Status       string                     `json:"status"`
	StartedAt    string                     `json:"started_at,omitempty"`
	FinishedAt   string                     `json:"finished_at,omitempty"`
	FinalOutput  string                     `json:"final_output,omitempty"`
	Error        string                     `json:"error,omitempty"`
	Steps        []formulaDashboardStep     `json:"steps"`
	Edges        []formulaDashboardEdge     `json:"edges,omitempty"`
	Logs         []formulaDashboardLogEntry `json:"logs,omitempty"`
	WorkspaceDir string                     `json:"workspace_dir,omitempty"`
}

type formulaDashboardStep struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	Description string                `json:"description,omitempty"`
	Notes       string                `json:"notes,omitempty"`
	Type        string                `json:"type,omitempty"`
	Agent       string                `json:"agent"`
	Model       string                `json:"model,omitempty"`
	Session     string                `json:"session,omitempty"`
	Status      string                `json:"status"`
	Output      string                `json:"output,omitempty"`
	Error       string                `json:"error,omitempty"`
	StartedAt   string                `json:"started_at,omitempty"`
	FinishedAt  string                `json:"finished_at,omitempty"`
	DurationMS  int64                 `json:"duration_ms,omitempty"`
	Priority    *int                  `json:"priority,omitempty"`
	Labels      []string              `json:"labels,omitempty"`
	Assignee    string                `json:"assignee,omitempty"`
	OutputKey   string                `json:"output_key,omitempty"`
	InputCtx    []string              `json:"input_ctx,omitempty"`
	Execution   string                `json:"execution,omitempty"`
	Condition   string                `json:"condition,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
	Gate        *formulaDashboardGate `json:"gate,omitempty"`
	DependsOn   []string              `json:"depends_on,omitempty"`
	Depth       int                   `json:"depth,omitempty"`
	Index       int                   `json:"index"`
}

type formulaDashboardGate struct {
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

type formulaDashboardEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type,omitempty"`
}

type formulaDashboardLogEntry struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type formulaDashboardMessage struct {
	Type  string                   `json:"type"`
	State formulaDashboardSnapshot `json:"state"`
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
			StartedAt:   time.Now().Format(time.RFC3339),
			Steps:       steps,
			Edges:       edges,
			Logs:        []formulaDashboardLogEntry{},
		},
		clients:  map[*websocket.Conn]struct{}{},
		shutdown: make(chan struct{}),
	}
}

func buildFormulaDashboardGraph(recipe *formula.Recipe) ([]formulaDashboardStep, []formulaDashboardEdge) {
	depths := computeDashboardDepths(recipe)
	dependsOnMap := map[string][]string{}
	stepIDs := map[string]struct{}{}

	steps := make([]formulaDashboardStep, 0, len(recipe.Steps))
	for index, step := range recipe.Steps {
		if step.IsRoot {
			continue
		}
		stepIDs[step.ID] = struct{}{}
		agentName := ""
		modelName := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			modelName = step.Agent.Model
		}
		var gate *formulaDashboardGate
		if step.Gate != nil {
			gate = &formulaDashboardGate{Type: step.Gate.Type, ID: step.Gate.ID, Timeout: step.Gate.Timeout}
		}
		steps = append(steps, formulaDashboardStep{
			ID:          step.ID,
			Title:       step.Title,
			Description: step.Description,
			Notes:       step.Notes,
			Type:        step.Type,
			Agent:       agentName,
			Model:       modelName,
			Status:      "pending",
			Priority:    step.Priority,
			Labels:      append([]string(nil), step.Labels...),
			Assignee:    step.Assignee,
			OutputKey:   step.OutputKey,
			InputCtx:    append([]string(nil), step.InputCtx...),
			Execution:   step.Execution,
			Condition:   step.Condition,
			Metadata:    cloneStringMap(step.Metadata),
			Gate:        gate,
			Depth:       depths[step.ID],
			Index:       index,
		})
	}

	edges := make([]formulaDashboardEdge, 0, len(recipe.Deps))
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		if _, ok := stepIDs[dep.StepID]; !ok {
			continue
		}
		if _, ok := stepIDs[dep.DependsOnID]; !ok {
			continue
		}
		dependsOnMap[dep.StepID] = append(dependsOnMap[dep.StepID], dep.DependsOnID)
		edges = append(edges, formulaDashboardEdge{From: dep.DependsOnID, To: dep.StepID, Type: dep.Type})
	}

	for i := range steps {
		steps[i].DependsOn = append([]string(nil), dependsOnMap[steps[i].ID]...)
	}

	return steps, edges
}

func computeDashboardDepths(recipe *formula.Recipe) map[string]int {
	stepIDs := map[string]struct{}{}
	for _, step := range recipe.Steps {
		if !step.IsRoot {
			stepIDs[step.ID] = struct{}{}
		}
	}

	parents := map[string][]string{}
	for _, dep := range recipe.Deps {
		if dep.Type == "parent-child" {
			continue
		}
		if _, ok := stepIDs[dep.StepID]; !ok {
			continue
		}
		if _, ok := stepIDs[dep.DependsOnID]; !ok {
			continue
		}
		parents[dep.StepID] = append(parents[dep.StepID], dep.DependsOnID)
	}

	depths := map[string]int{}
	var visit func(string, map[string]bool) int
	visit = func(id string, visiting map[string]bool) int {
		if depth, ok := depths[id]; ok {
			return depth
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true
		maxDepth := 0
		for _, parentID := range parents[id] {
			if next := visit(parentID, visiting) + 1; next > maxDepth {
				maxDepth = next
			}
		}
		delete(visiting, id)
		depths[id] = maxDepth
		return maxDepth
	}

	for id := range stepIDs {
		visit(id, map[string]bool{})
	}
	return depths
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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
		Type  string                   `json:"type"`
		State formulaDashboardSnapshot `json:"state"`
	}{Type: "state", State: s.snapshot()})
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
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
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
		break
	}
	if s.state.Status == "pending" {
		s.state.Status = "running"
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepCompleted(stepID, output string) {
	s.mu.Lock()
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		s.state.Steps[i].Status = "completed"
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		break
	}
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) markStepFailed(stepID, errMsg, output string) {
	s.mu.Lock()
	for i := range s.state.Steps {
		if s.state.Steps[i].ID != stepID {
			continue
		}
		s.state.Steps[i].Status = "failed"
		s.state.Steps[i].Error = errMsg
		s.state.Steps[i].Output = output
		s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
		if s.state.Steps[i].StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, s.state.Steps[i].StartedAt); err == nil {
				s.state.Steps[i].DurationMS = time.Since(started).Milliseconds()
			}
		}
		break
	}
	s.state.Status = "failed"
	s.state.Error = errMsg
	s.mu.Unlock()
	s.broadcast()
}

func (s *formulaDashboardServer) finalize(result *executor.RunResult, runErr error) {
	s.mu.Lock()
	if result != nil {
		s.state.RecipeName = result.RecipeName
		s.state.FinalOutput = result.FinalOutput
		s.state.Status = "completed"
		if runErr != nil {
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
	s.state.FinishedAt = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
	s.broadcast()
}

func cloneFormulaDashboardSnapshot(s formulaDashboardSnapshot) formulaDashboardSnapshot {
	cp := s
	cp.Steps = make([]formulaDashboardStep, len(s.Steps))
	for i, step := range s.Steps {
		cp.Steps[i] = step
		cp.Steps[i].Labels = append([]string(nil), step.Labels...)
		cp.Steps[i].InputCtx = append([]string(nil), step.InputCtx...)
		cp.Steps[i].DependsOn = append([]string(nil), step.DependsOn...)
		cp.Steps[i].Metadata = cloneStringMap(step.Metadata)
		if step.Gate != nil {
			gate := *step.Gate
			cp.Steps[i].Gate = &gate
		}
	}
	cp.Edges = append([]formulaDashboardEdge(nil), s.Edges...)
	cp.Logs = append([]formulaDashboardLogEntry(nil), s.Logs...)
	return cp
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

func formulaDashboardWorkspace(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, ".tt")
}
