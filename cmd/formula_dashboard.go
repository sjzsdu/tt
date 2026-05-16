package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
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
	RecipeName  string                     `json:"recipe_name"`
	Status      string                     `json:"status"`
	StartedAt   string                     `json:"started_at,omitempty"`
	FinishedAt  string                     `json:"finished_at,omitempty"`
	FinalOutput string                     `json:"final_output,omitempty"`
	Error       string                     `json:"error,omitempty"`
	Steps       []formulaDashboardStep     `json:"steps"`
	Logs        []formulaDashboardLogEntry `json:"logs,omitempty"`
}

type formulaDashboardStep struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Agent      string `json:"agent"`
	Model      string `json:"model,omitempty"`
	Session    string `json:"session,omitempty"`
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

type formulaDashboardLogEntry struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type formulaDashboardMessage struct {
	Type  string                   `json:"type"`
	State formulaDashboardSnapshot `json:"state"`
}

const formulaDashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>tt formula dashboard</title>
  <style>
    :root { color-scheme: dark; --bg: #0b1020; --panel: #121a31; --panel2: #0f172a; --border: #24304b; --text: #e5e7eb; --muted: #94a3b8; --green: #22c55e; --yellow: #f59e0b; --red: #ef4444; --blue: #60a5fa; }
    body { margin: 0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif; background: linear-gradient(180deg, #090d1a, #111827); color: var(--text); }
    header { padding: 18px 22px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; gap: 16px; }
    h1 { margin: 0; font-size: 18px; }
    .sub { color: var(--muted); font-size: 13px; margin-top: 4px; }
    main { display: grid; grid-template-columns: 1.15fr 0.85fr; gap: 16px; padding: 16px; }
    .card { background: rgba(18,26,49,.92); border: 1px solid var(--border); border-radius: 14px; overflow: hidden; }
    .card h2 { margin: 0; font-size: 14px; padding: 14px 16px; border-bottom: 1px solid var(--border); background: rgba(15,23,42,.7); }
    .card .body { padding: 16px; }
    .steps { display: grid; gap: 12px; }
    .step { border: 1px solid var(--border); border-radius: 12px; background: rgba(15,23,42,.55); padding: 12px 14px; }
    .step.running { border-color: rgba(96,165,250,.7); box-shadow: 0 0 0 1px rgba(96,165,250,.15) inset; }
    .step.completed { border-color: rgba(34,197,94,.65); }
    .step.failed { border-color: rgba(239,68,68,.7); }
    .step.skipped { opacity: .7; }
    .row { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
    .title { font-weight: 600; }
    .meta { color: var(--muted); font-size: 12px; margin-top: 6px; }
    .badge { font-size: 11px; padding: 3px 8px; border-radius: 999px; border: 1px solid var(--border); background: rgba(255,255,255,.03); text-transform: uppercase; letter-spacing: .05em; }
    .badge.running { color: var(--blue); border-color: rgba(96,165,250,.5); }
    .badge.completed { color: var(--green); border-color: rgba(34,197,94,.5); }
    .badge.failed { color: var(--red); border-color: rgba(239,68,68,.5); }
    .badge.skipped { color: var(--yellow); border-color: rgba(245,158,11,.5); }
    pre { margin: 10px 0 0; white-space: pre-wrap; word-break: break-word; background: #080d1a; border: 1px solid var(--border); border-radius: 10px; padding: 12px; color: #dbeafe; font-size: 12px; line-height: 1.5; }
    .logs { max-height: 340px; overflow: auto; display: grid; gap: 8px; }
    .log { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; color: #cbd5e1; border-bottom: 1px dashed rgba(148,163,184,.2); padding-bottom: 8px; }
    .footer { padding: 0 16px 16px; color: var(--muted); font-size: 12px; }
    .summary { display: grid; gap: 8px; }
    .summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; }
    .stat { background: rgba(15,23,42,.7); border: 1px solid var(--border); border-radius: 10px; padding: 10px 12px; }
    .stat .k { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .04em; }
    .stat .v { margin-top: 4px; font-weight: 600; }
    @media (max-width: 1100px) { main { grid-template-columns: 1fr; } .summary-grid { grid-template-columns: repeat(2, 1fr); } }
  </style>
</head>
<body>
  <header>
    <div>
      <h1 id="recipe-name">tt formula dashboard</h1>
      <div class="sub" id="status-line">waiting for run...</div>
    </div>
    <div class="sub">Web dashboard • live formula run</div>
  </header>
  <main>
    <section class="card">
      <h2>Pipeline</h2>
      <div class="body">
        <div class="summary">
          <div class="summary-grid" id="summary-grid"></div>
          <div class="steps" id="steps"></div>
        </div>
      </div>
    </section>
    <section class="card">
      <h2>Logs & Final Output</h2>
      <div class="body">
        <div class="logs" id="logs"></div>
        <h3 style="margin:16px 0 8px;font-size:13px;color:#cbd5e1;">Final Output</h3>
        <pre id="final-output">(waiting)</pre>
      </div>
      <div class="footer" id="footer"></div>
    </section>
  </main>
  <script>
    const stateUrl = '/api/state';
    let current = null;

    function esc(s) {
      return String(s ?? '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }

    function badge(status) {
      return '<span class="badge ' + esc(status) + '">' + esc(status) + '</span>';
    }

    function render() {
      if (!current) return;
      document.getElementById('recipe-name').textContent = current.recipe_name || 'tt formula dashboard';
      document.getElementById('status-line').textContent = (current.status || 'pending') + (current.error ? ' • ' + current.error : '');
      document.getElementById('final-output').textContent = current.final_output || '(waiting)';
      document.getElementById('footer').textContent = (current.started_at || '') + (current.finished_at ? ' → ' + current.finished_at : '');

      const summary = [
        ['Status', current.status || 'pending'],
        ['Steps', String((current.steps || []).length)],
        ['Logs', String((current.logs || []).length)],
        ['Error', current.error || 'none'],
      ];
      document.getElementById('summary-grid').innerHTML = summary.map(([k, v]) => '<div class="stat"><div class="k">' + esc(k) + '</div><div class="v">' + esc(v) + '</div></div>').join('');

      document.getElementById('steps').innerHTML = (current.steps || []).map(step => {
        const extra = [step.agent ? 'agent: ' + step.agent : '', step.model ? 'model: ' + step.model : '', step.session ? 'session: ' + step.session : '', step.started_at ? 'started: ' + step.started_at : '', step.finished_at ? 'finished: ' + step.finished_at : '', step.duration_ms ? 'duration: ' + step.duration_ms + 'ms' : ''].filter(Boolean).join(' • ');
        const output = step.output ? '<pre>' + esc(step.output) + '</pre>' : '';
        const err = step.error ? '<pre style="color:#fecaca;">' + esc(step.error) + '</pre>' : '';
        return '<div class="step ' + esc(step.status || 'pending') + '"><div class="row"><div class="title">' + esc(step.title || step.id) + '</div>' + badge(step.status || 'pending') + '</div><div class="meta">' + esc(step.id) + (extra ? ' • ' + esc(extra) : '') + '</div>' + output + err + '</div>';
      }).join('');

      document.getElementById('logs').innerHTML = (current.logs || []).map(log => '<div class="log">[' + esc(log.at) + '] ' + esc(log.text) + '</div>').join('');
    }

    function connect() {
      const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
      const ws = new WebSocket(proto + location.host + '/ws');
      ws.onmessage = ev => {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === 'state') {
            current = msg.state;
            render();
          }
        } catch (e) {
          console.error(e);
        }
      };
      ws.onclose = () => setTimeout(connect, 1500);
    }

    fetch(stateUrl).then(r => r.json()).then(msg => {
      current = msg.state;
      render();
      connect();
    }).catch(err => {
      document.getElementById('status-line').textContent = 'failed to load state: ' + err;
    });
  </script>
</body>
</html>`

func newFormulaDashboardServer(recipe *formula.Recipe) *formulaDashboardServer {
	steps := make([]formulaDashboardStep, 0, len(recipe.Steps))
	for _, step := range recipe.Steps {
		if step.IsRoot {
			continue
		}
		agentName := ""
		modelName := ""
		if step.Agent != nil {
			agentName = step.Agent.Name
			if step.Agent.Model != "" {
				modelName = step.Agent.Model
			}
		}
		steps = append(steps, formulaDashboardStep{ID: step.ID, Title: step.Title, Agent: agentName, Model: modelName, Status: "pending"})
	}
	return &formulaDashboardServer{
		started:  time.Now(),
		recipe:   recipe.Name,
		state:    formulaDashboardSnapshot{RecipeName: recipe.Name, Status: "running", StartedAt: time.Now().Format(time.RFC3339), Steps: steps, Logs: []formulaDashboardLogEntry{}},
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
	if err := template.Must(template.New("formula-dashboard").Parse(formulaDashboardHTML)).Execute(w, nil); err != nil {
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
				s.state.Steps[i].FinishedAt = time.Now().Format(time.RFC3339)
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
	cp.Steps = append([]formulaDashboardStep(nil), s.Steps...)
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
