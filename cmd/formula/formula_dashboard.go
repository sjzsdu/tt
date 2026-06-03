package formulacmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tt/internal/formula/ir"
	"github.com/sjzsdu/tt/internal/formularun"
	"github.com/sjzsdu/tt/internal/formulaui"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	"github.com/sjzsdu/tt/internal/webui"
	"nhooyr.io/websocket"
)

type formulaDashboardServer struct {
	mu              sync.Mutex
	server          *http.Server
	port            int
	started         time.Time
	recipe          string
	state           formulaui.Snapshot
	store           *formularun.Store
	readonly        bool
	clients         map[*websocket.Conn]struct{}
	shutdown        chan struct{}
	directProcessor formulaDashboardDirectProcessor
}

type formulaDashboardDirectProcessor interface {
	ProcessDirectContext(ctx context.Context, opt pcwrap.RunOptions) (string, error)
}

func newFormulaDashboardServerFromSnapshot(snapshot formulaui.Snapshot) *formulaDashboardServer {
	return &formulaDashboardServer{
		started:  time.Now(),
		recipe:   snapshot.RecipeName,
		state:    formulaui.CloneSnapshot(snapshot),
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

func newFormulaDashboardServer(workflow *ir.Workflow) *formulaDashboardServer {
	steps, edges := formulaui.BuildWorkflowGraph(workflow)
	name := ""
	description := ""
	if workflow != nil {
		name = workflow.Name
		description = workflow.Description
	}
	return &formulaDashboardServer{
		started: time.Now(),
		recipe:  name,
		state: formulaui.Snapshot{
			RecipeName:  name,
			Description: description,
			Status:      "running",
			Steps:       steps,
			Edges:       edges,
			Logs:        []formulaui.LogEntry{},
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
	mux.HandleFunc("/api/agent-session", s.handleAgentSession)
	mux.HandleFunc("/api/final-report-chat", s.handleFinalReportChat)
	mux.HandleFunc("/api/final-report-chat/message", s.handleFinalReportChatMessage)
	mux.HandleFunc("/api/final-report-chat/promote", s.handleFinalReportChatPromote)
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

func (s *formulaDashboardServer) snapshot() formulaui.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return formulaui.CloneSnapshot(s.state)
}

func (s *formulaDashboardServer) snapshotMessageLocked() []byte {
	msg := formulaui.Message{Type: "state", State: formulaui.CloneSnapshot(s.state)}
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
	snapshot := formulaui.CloneSnapshot(s.state)
	s.mu.Unlock()
	return s.store.SaveState(snapshot)
}
