package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	teamruntime "github.com/sjzsdu/tt/internal/team"
	"github.com/sjzsdu/tt/internal/webui"
)

type teamDashboardServer struct {
	mu         sync.Mutex
	server     *http.Server
	listener   net.Listener
	port       int
	store      *teamruntime.Store
	definition *teamruntime.Definition
	open       func(string)
}

type teamDashboardState struct {
	Team   teamDashboardTeam                `json:"team"`
	Thread teamruntime.Thread               `json:"thread"`
	Round  *teamruntime.RoundState          `json:"round,omitempty"`
	Agents []teamDashboardAgent             `json:"agents"`
	Events []teamruntime.Event              `json:"events"`
	Board  teamruntime.BlackboardProjection `json:"blackboard"`
	Memory teamruntime.MemoryDocument       `json:"memory"`
}

type teamDashboardTeam struct {
	Team        string `json:"team"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type teamDashboardAgent struct {
	ID               string `json:"id"`
	Role             string `json:"role,omitempty"`
	Agent            string `json:"agent,omitempty"`
	Model            string `json:"model,omitempty"`
	Facilitator      bool   `json:"facilitator,omitempty"`
	Finalizer        bool   `json:"finalizer,omitempty"`
	MemoryMaintainer bool   `json:"memory_maintainer,omitempty"`
}

func newTeamDashboardServer(store *teamruntime.Store, definition *teamruntime.Definition) *teamDashboardServer {
	return &teamDashboardServer{
		store:      store,
		definition: definition,
		open:       openBrowser,
	}
}

func (s *teamDashboardServer) start(port int, launchBrowser bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return fmt.Errorf("team dashboard already running on port %d", s.port)
	}
	if s.store == nil || s.definition == nil {
		return fmt.Errorf("team dashboard requires a store and definition")
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid team dashboard port %d", port)
	}

	var (
		listener net.Listener
		err      error
	)
	if port == 0 {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	} else {
		for candidate := port; candidate <= port+20 && candidate <= 65535; candidate++ {
			listener, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(candidate))
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return fmt.Errorf("start team dashboard: %w", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.listener = listener
	s.server = server
	s.port = actualPort
	url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
	go func() {
		_ = server.Serve(listener)
	}()
	if launchBrowser && s.open != nil {
		go s.open(url)
	}
	return nil
}

func (s *teamDashboardServer) close() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func (s *teamDashboardServer) url() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *teamDashboardServer) wait(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	s.close()
}

func (s *teamDashboardServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/favicon.svg", webui.TeamFaviconHandler())
	mux.Handle("/assets/", webui.TeamAssetsHandler())
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	return mux
}

func (s *teamDashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(webui.TeamIndex())
}

func (s *teamDashboardServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, err := s.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *teamDashboardServer) snapshot() (teamDashboardState, error) {
	thread, state, events, err := s.store.Snapshot()
	if err != nil {
		return teamDashboardState{}, err
	}
	for i := range events {
		events[i].Session = ""
	}
	memory, err := teamruntime.LoadMemory(thread.MemoryPath, thread.Team)
	if err != nil {
		return teamDashboardState{}, err
	}
	agents := make([]teamDashboardAgent, 0, len(s.definition.Agents))
	for _, member := range s.definition.Agents {
		agents = append(agents, teamDashboardAgent{
			ID:               member.ID,
			Role:             member.Role,
			Agent:            member.Agent,
			Model:            member.Model,
			Facilitator:      strings.EqualFold(member.ID, s.definition.Coordination.Facilitator),
			Finalizer:        strings.EqualFold(member.ID, s.definition.Coordination.Finalizer),
			MemoryMaintainer: strings.EqualFold(member.ID, s.definition.Memory.Maintainer),
		})
	}
	return teamDashboardState{
		Team: teamDashboardTeam{
			Team:        s.definition.Team,
			Title:       s.definition.Title,
			Description: s.definition.Description,
		},
		Thread: thread,
		Round:  state.Current,
		Agents: agents,
		Events: events,
		Board:  teamruntime.ProjectBlackboard(events, thread.CurrentRound),
		Memory: memory,
	}, nil
}
