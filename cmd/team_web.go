package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	actions    teamDashboardActions
	subs       map[chan struct{}]struct{}
	open       func(string)
}

type teamDashboardState struct {
	Team         teamDashboardTeam                `json:"team"`
	Thread       teamruntime.Thread               `json:"thread"`
	Round        *teamruntime.RoundState          `json:"round,omitempty"`
	Agents       []teamDashboardAgent             `json:"agents"`
	Events       []teamruntime.Event              `json:"events"`
	Board        teamruntime.BlackboardProjection `json:"blackboard"`
	Memory       teamruntime.MemoryDocument       `json:"memory"`
	MemoryReview teamruntime.MemoryReview         `json:"memory_review"`
	Controls     teamDashboardControls            `json:"controls"`
}

type teamDashboardTeam struct {
	Team         string `json:"team"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
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
		subs:       map[chan struct{}]struct{}{},
		open:       openBrowser,
	}
}

func (s *teamDashboardServer) setActions(actions teamDashboardActions) {
	s.mu.Lock()
	s.actions = actions
	s.mu.Unlock()
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
	s.subs = map[chan struct{}]struct{}{}
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
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/follow-up", s.handleFollowUp)
	mux.HandleFunc("/api/resume", s.handleResume)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/memory/retry", s.handleMemoryRetry)
	mux.HandleFunc("/api/memory/rollback", s.handleMemoryRollback)
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

func (s *teamDashboardServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	subscriber := s.subscribe()
	defer s.unsubscribe(subscriber)
	if err := s.writeStateEvent(w); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case _, open := <-subscriber:
			if !open {
				return
			}
			if err := s.writeStateEvent(w); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *teamDashboardServer) handleFollowUp(w http.ResponseWriter, r *http.Request) {
	actions, ok := s.prepareMutation(w, r)
	if !ok {
		return
	}
	var input struct {
		Message string `json:"message"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	decoder := json.NewDecoder(bufio.NewReader(r.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}
	if err := actions.FollowUp(input.Message); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.notifyState()
	writeAccepted(w)
}

func (s *teamDashboardServer) handleResume(w http.ResponseWriter, r *http.Request) {
	actions, ok := s.prepareMutation(w, r)
	if !ok {
		return
	}
	if err := actions.Resume(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.notifyState()
	writeAccepted(w)
}

func (s *teamDashboardServer) handleStop(w http.ResponseWriter, r *http.Request) {
	actions, ok := s.prepareMutation(w, r)
	if !ok {
		return
	}
	if err := actions.Stop(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.notifyState()
	writeAccepted(w)
}

func (s *teamDashboardServer) handleMemoryRetry(w http.ResponseWriter, r *http.Request) {
	actions, ok := s.prepareMutation(w, r)
	if !ok {
		return
	}
	if err := actions.RetryMemory(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.notifyState()
	writeAccepted(w)
}

func (s *teamDashboardServer) handleMemoryRollback(w http.ResponseWriter, r *http.Request) {
	actions, ok := s.prepareMutation(w, r)
	if !ok {
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	decoder := json.NewDecoder(bufio.NewReader(r.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Version < 1 {
		http.Error(w, "invalid memory version", http.StatusBadRequest)
		return
	}
	if err := actions.RollbackMemory(input.Version); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.notifyState()
	writeAccepted(w)
}

func (s *teamDashboardServer) prepareMutation(w http.ResponseWriter, r *http.Request) (teamDashboardActions, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	if !isLoopbackRequest(r) || !isSameOriginMutation(r) {
		http.Error(w, "dashboard mutations require a same-origin loopback request", http.StatusForbidden)
		return nil, false
	}
	s.mu.Lock()
	actions := s.actions
	s.mu.Unlock()
	if actions == nil {
		http.Error(w, "team dashboard controls are unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	return actions, true
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isSameOriginMutation(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := parsed.Hostname()
	requestHost := r.Host
	if host, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestHost = host
	}
	return strings.EqualFold(originHost, requestHost) && net.ParseIP(originHost).IsLoopback()
}

func writeAccepted(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{\"accepted\":true}\n"))
}

func (s *teamDashboardServer) subscribe() chan struct{} {
	subscriber := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[subscriber] = struct{}{}
	s.mu.Unlock()
	return subscriber
}

func (s *teamDashboardServer) unsubscribe(subscriber chan struct{}) {
	s.mu.Lock()
	delete(s.subs, subscriber)
	s.mu.Unlock()
}

func (s *teamDashboardServer) notifyState() {
	s.mu.Lock()
	subscribers := make([]chan struct{}, 0, len(s.subs))
	for subscriber := range s.subs {
		subscribers = append(subscribers, subscriber)
	}
	s.mu.Unlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (s *teamDashboardServer) writeStateEvent(w http.ResponseWriter) error {
	state, err := s.snapshot()
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	lastID := int64(0)
	if len(state.Events) > 0 {
		lastID = state.Events[len(state.Events)-1].ID
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: state\ndata: %s\n\n", lastID, data)
	return err
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
	memoryReview, err := teamruntime.LoadMemoryReview(thread.MemoryPath, thread.Team)
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
	controls := teamDashboardControls{}
	s.mu.Lock()
	actions := s.actions
	s.mu.Unlock()
	if actions != nil {
		controls = actions.Controls()
	}
	return teamDashboardState{
		Team: teamDashboardTeam{
			Team:         s.definition.Team,
			Title:        s.definition.Title,
			Description:  s.definition.Description,
			DefaultModel: s.definition.DefaultModel,
		},
		Thread:       thread,
		Round:        state.Current,
		Agents:       agents,
		Events:       events,
		Board:        teamruntime.ProjectBlackboard(events, thread.CurrentRound),
		Memory:       memory,
		MemoryReview: memoryReview,
		Controls:     controls,
	}, nil
}
