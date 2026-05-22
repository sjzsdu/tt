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

	"github.com/spf13/cobra"

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

type formulaDashboardSnapshot struct {
	RecipeName   string                     `json:"recipe_name"`
	Description  string                     `json:"description,omitempty"`
	Phase        string                     `json:"phase,omitempty"`
	Status       string                     `json:"status"`
	FinalOutput  string                     `json:"final_output,omitempty"`
	Error        string                     `json:"error,omitempty"`
	Steps        []formulaDashboardStep     `json:"steps"`
	Edges        []formulaDashboardEdge     `json:"edges,omitempty"`
	Logs         []formulaDashboardLogEntry `json:"logs,omitempty"`
	WorkspaceDir string                     `json:"workspace_dir,omitempty"`
	RunID        string                     `json:"run_id,omitempty"`
}

type formulaDashboardStep struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Description       string                      `json:"description,omitempty"`
	Notes             string                      `json:"notes,omitempty"`
	Type              string                      `json:"type,omitempty"`
	Agent             string                      `json:"agent"`
	Model             string                      `json:"model,omitempty"`
	Session           string                      `json:"session,omitempty"`
	Status            string                      `json:"status"`
	Output            string                      `json:"output,omitempty"`
	Error             string                      `json:"error,omitempty"`
	StartedAt         string                      `json:"-"`
	FinishedAt        string                      `json:"-"`
	DurationMS        int64                       `json:"duration_ms,omitempty"`
	Priority          *int                        `json:"priority,omitempty"`
	Labels            []string                    `json:"labels,omitempty"`
	Assignee          string                      `json:"assignee,omitempty"`
	OutputKey         string                      `json:"output_key,omitempty"`
	InputCtx          []string                    `json:"input_ctx,omitempty"`
	Execution         string                      `json:"execution,omitempty"`
	Condition         string                      `json:"condition,omitempty"`
	Metadata          map[string]string           `json:"metadata,omitempty"`
	Gate              *formulaDashboardGate       `json:"gate,omitempty"`
	Loop              *formulaDashboardLoop       `json:"loop,omitempty"`
	DependsOn         []string                    `json:"depends_on,omitempty"`
	Activities        []formulaStepActivity       `json:"activities,omitempty"`
	HumanInputRequest *executor.HumanInputRequest `json:"human_input_request,omitempty"`
	Depth             int                         `json:"depth,omitempty"`
	Index             int                         `json:"index"`
}

type formulaStepActivity struct {
	At         string `json:"at"`
	StepID     string `json:"step_id"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

type formulaDashboardLoop struct {
	Count          int                        `json:"count,omitempty"`
	Until          string                     `json:"until,omitempty"`
	Max            int                        `json:"max,omitempty"`
	Range          string                     `json:"range,omitempty"`
	ForEach        string                     `json:"for_each,omitempty"`
	Var            string                     `json:"var,omitempty"`
	Parallel       bool                       `json:"parallel,omitempty"`
	MaxConcurrency int                        `json:"max_concurrency,omitempty"`
	Summary        string                     `json:"summary,omitempty"`
	Body           []formulaDashboardLoopBody `json:"body,omitempty"`
}

type formulaDashboardLoopBody struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	OutputKey   string   `json:"output_key,omitempty"`
	InputCtx    []string `json:"input_ctx,omitempty"`
	Condition   string   `json:"condition,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
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
		if step.IsRoot || isFormulaDashboardHiddenBoundary(step) {
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
		loop := buildFormulaDashboardLoop(step.Loop)
		var humanInputRequest *executor.HumanInputRequest
		if step.Execution == executor.HumanInputExecution && step.Form != nil {
			humanInputRequest = &executor.HumanInputRequest{Reason: step.Description, Form: step.Form}
		}
		steps = append(steps, formulaDashboardStep{
			ID:                step.ID,
			Title:             step.Title,
			Description:       step.Description,
			Notes:             step.Notes,
			Type:              step.Type,
			Agent:             agentName,
			Model:             modelName,
			Status:            "pending",
			Priority:          step.Priority,
			Labels:            append([]string(nil), step.Labels...),
			Assignee:          step.Assignee,
			OutputKey:         step.OutputKey,
			InputCtx:          append([]string(nil), step.InputCtx...),
			Execution:         step.Execution,
			Condition:         step.Condition,
			Metadata:          cloneStringMap(step.Metadata),
			Gate:              gate,
			Loop:              loop,
			HumanInputRequest: humanInputRequest,
			Depth:             depths[step.ID],
			Index:             index,
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
		if !step.IsRoot && !isFormulaDashboardHiddenBoundary(step) {
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

func isFormulaDashboardHiddenBoundary(step formula.RecipeStep) bool {
	return step.Execution == "noop" && step.Metadata != nil && step.Metadata["formula_boundary"] != ""
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

func buildFormulaDashboardLoop(loop *formula.LoopSpec) *formulaDashboardLoop {
	if loop == nil {
		return nil
	}
	dashboardLoop := &formulaDashboardLoop{
		Count:          loop.Count,
		Until:          loop.Until,
		Max:            loop.Max,
		Range:          loop.Range,
		ForEach:        loop.ForEach,
		Var:            loop.Var,
		Parallel:       loop.Parallel,
		MaxConcurrency: loop.MaxConcurrency,
		Summary:        dashboardLoopSummary(loop),
		Body:           make([]formulaDashboardLoopBody, 0, len(loop.Body)),
	}
	for _, body := range loop.Body {
		if body == nil {
			continue
		}
		agentName := ""
		modelName := ""
		if body.Agent != nil {
			agentName = body.Agent.Name
			modelName = body.Agent.Model
		}
		dashboardLoop.Body = append(dashboardLoop.Body, formulaDashboardLoopBody{
			ID:          body.ID,
			Title:       body.Title,
			Description: body.Description,
			Agent:       agentName,
			Model:       modelName,
			OutputKey:   body.OutputKey,
			InputCtx:    append([]string(nil), body.InputCtx...),
			Condition:   body.Condition,
			DependsOn:   append(append([]string(nil), body.DependsOn...), body.Needs...),
		})
	}
	return dashboardLoop
}

func dashboardLoopSummary(loop *formula.LoopSpec) string {
	if loop == nil {
		return ""
	}
	switch {
	case loop.ForEach != "":
		mode := "sequential"
		if loop.Parallel {
			mode = "parallel"
		}
		if loop.MaxConcurrency > 0 {
			return fmt.Sprintf("foreach %s as %s · %s · max concurrency %d", loop.ForEach, loop.Var, mode, loop.MaxConcurrency)
		}
		return fmt.Sprintf("foreach %s as %s · %s", loop.ForEach, loop.Var, mode)
	case loop.Until != "":
		max := loop.Max
		if max <= 0 {
			max = 1
		}
		return fmt.Sprintf("until %s · max %d", loop.Until, max)
	case loop.Count > 0:
		return fmt.Sprintf("count %d", loop.Count)
	case loop.Range != "":
		if loop.Var != "" {
			return fmt.Sprintf("for %s in %s", loop.Var, loop.Range)
		}
		return fmt.Sprintf("range %s", loop.Range)
	default:
		return fmt.Sprintf("%d body step(s)", len(loop.Body))
	}
}

func cloneDashboardLoop(src *formulaDashboardLoop) *formulaDashboardLoop {
	if src == nil {
		return nil
	}
	cp := *src
	cp.Body = make([]formulaDashboardLoopBody, len(src.Body))
	copy(cp.Body, src.Body)
	for i := range cp.Body {
		cp.Body[i].InputCtx = append([]string(nil), src.Body[i].InputCtx...)
	}
	return &cp
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
	resolvedStepID, err := resolveFormulaRunStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var target *formulaDashboardStep
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
	initialResults, initialContext := buildResumeStateExcluding(recipe, snapshot, map[string]bool{resolvedStepID: true})
	resetSnapshotStepForRetry(&snapshot, resolvedStepID)
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
	s.state = cloneFormulaDashboardSnapshot(snapshot)
	s.readonly = false
	s.mu.Unlock()
	s.broadcast()
	advice := strings.TrimSpace(req.Advice)
	go func() {
		if err := executeFormulaRecipeWithAdvice(&cobra.Command{}, recipe, s.store, s, s.store.Meta.Vars, initialResults, initialContext, map[string]string{resolvedStepID: advice}); err != nil {
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
	resolvedStepID, err := resolveFormulaRunStepID(snapshot, req.StepID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var request executor.HumanInputRequest
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
	if err := markSnapshotStepCompletedWithOutput(&snapshot, resolvedStepID, output); err != nil {
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
	initialResults, initialContext := buildResumeState(recipe, snapshot)
	s.store.Meta.Status = formularun.StatusRunning
	s.store.Meta.Error = ""
	s.store.Meta.FinishedAt = ""
	s.store.Meta.PID = os.Getpid()
	s.store.Meta.TTVersion = version
	_ = s.store.SaveMetadata()
	_ = s.store.AppendEvent(formularun.Event{Type: "run_resumed", Status: formularun.StatusRunning})
	resetSnapshotForResume(&snapshot)
	s.mu.Lock()
	s.state = cloneFormulaDashboardSnapshot(snapshot)
	s.readonly = false
	s.mu.Unlock()
	s.broadcast()
	go func() {
		if err := executeFormulaRecipe(&cobra.Command{}, recipe, s.store, s, s.store.Meta.Vars, initialResults, initialContext); err != nil {
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

func loopParentStepID(stepID string) string {
	idx := strings.Index(stepID, ".iter")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

func appendStepActivity(step *formulaDashboardStep, activity formulaStepActivity) {
	if step == nil || activity.StepID == "" {
		return
	}
	for i := len(step.Activities) - 1; i >= 0; i-- {
		if step.Activities[i].StepID != activity.StepID {
			continue
		}
		if activity.Title == "" {
			activity.Title = step.Activities[i].Title
		}
		step.Activities[i] = activity
		return
	}
	step.Activities = append(step.Activities, activity)
	if len(step.Activities) > 80 {
		step.Activities = append([]formulaStepActivity(nil), step.Activities[len(step.Activities)-80:]...)
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

func cloneFormulaDashboardSnapshot(s formulaDashboardSnapshot) formulaDashboardSnapshot {
	cp := s
	cp.Steps = make([]formulaDashboardStep, len(s.Steps))
	for i, step := range s.Steps {
		cp.Steps[i] = step
		cp.Steps[i].Labels = append([]string(nil), step.Labels...)
		cp.Steps[i].InputCtx = append([]string(nil), step.InputCtx...)
		cp.Steps[i].DependsOn = append([]string(nil), step.DependsOn...)
		cp.Steps[i].Activities = append([]formulaStepActivity(nil), step.Activities...)
		cp.Steps[i].Loop = cloneDashboardLoop(step.Loop)
		cp.Steps[i].Metadata = cloneStringMap(step.Metadata)
		cp.Steps[i].HumanInputRequest = cloneHumanInputRequest(step.HumanInputRequest)
		if step.Gate != nil {
			gate := *step.Gate
			cp.Steps[i].Gate = &gate
		}
	}
	cp.Edges = append([]formulaDashboardEdge(nil), s.Edges...)
	cp.Logs = append([]formulaDashboardLogEntry(nil), s.Logs...)
	return cp
}

func cloneHumanInputRequest(src *executor.HumanInputRequest) *executor.HumanInputRequest {
	if src == nil {
		return nil
	}
	cp := *src
	if src.Form != nil {
		form := *src.Form
		form.Fields = make([]*formula.FormField, len(src.Form.Fields))
		for i, field := range src.Form.Fields {
			if field == nil {
				continue
			}
			fieldCopy := *field
			fieldCopy.Options = append([]string(nil), field.Options...)
			form.Fields[i] = &fieldCopy
		}
		cp.Form = &form
	}
	return &cp
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
