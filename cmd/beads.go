package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/beads"
	"github.com/sjzsdu/tt/internal/webui"
)

// ---------------------------------------------------------------------------
// Cobra command
// ---------------------------------------------------------------------------

var (
	beadsPort    int
	beadsBrowser bool
	beadsWorkspace string
	beadsReadOnly  bool
)

var beadsCmd = &cobra.Command{
	Use:     "beads",
	Aliases: []string{"bead"},
	Short:   "Launch the Beads issue dashboard",
	Long: `Start a loopback-only web dashboard for browsing, querying, and
visualizing issues managed by bd (beads). The dashboard serves on
127.0.0.1 and is not exposed on public interfaces by default.`,
	RunE: runBeadsDashboard,
}

func init() {
	beadsCmd.Flags().IntVar(&beadsPort, "port", 9720, "dashboard port (0 for random)")
	beadsCmd.Flags().BoolVar(&beadsBrowser, "open", true, "open the dashboard in the default browser")
	beadsCmd.Flags().StringVar(&beadsWorkspace, "workspace", "", "beads workspace directory (default: auto-detect)")
	beadsCmd.Flags().BoolVar(&beadsReadOnly, "read-only", true, "disable mutation endpoints")
	rootCmd.AddCommand(beadsCmd)
}

func runBeadsDashboard(cmd *cobra.Command, args []string) error {
	adapter, err := beads.NewAdapterFromWorkspace(beadsWorkspace)
	if err != nil {
		return fmt.Errorf("beads dashboard: %w", err)
	}
	srv := newBeadsDashboardServer(adapter, beadsReadOnly)
	if err := srv.start(beadsPort, beadsBrowser); err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Beads dashboard running at %s\n", srv.url())
	fmt.Fprintln(cmd.ErrOrStderr(), "Press Ctrl-C to stop.")
	runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	srv.wait(runCtx)
	return nil
}

// ---------------------------------------------------------------------------
// Dashboard server
// ---------------------------------------------------------------------------

type beadsDashboardServer struct {
	mu       sync.Mutex
	writeMu  sync.Mutex // serializes mutation requests
	server   *http.Server
	listener net.Listener
	port     int
	adapter  *beads.Adapter
	readonly bool
	open     func(string)
}

func newBeadsDashboardServer(adapter *beads.Adapter, readonly bool) *beadsDashboardServer {
	return &beadsDashboardServer{
		adapter:  adapter,
		readonly: readonly,
		open:     openBrowser,
	}
}

func (s *beadsDashboardServer) start(port int, launchBrowser bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return fmt.Errorf("beads dashboard already running on port %d", s.port)
	}
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid beads dashboard port %d", port)
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
		return fmt.Errorf("start beads dashboard: %w", err)
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

func (s *beadsDashboardServer) close() {
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

func (s *beadsDashboardServer) url() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *beadsDashboardServer) wait(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	s.close()
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

func (s *beadsDashboardServer) routes() http.Handler {
	mux := http.NewServeMux()
	// Static assets.
	mux.Handle("/favicon.svg", webui.BeadsFaviconHandler())
	mux.Handle("/assets/", webui.BeadsAssetsHandler())
	// SPA index.
	mux.HandleFunc("/", s.handleIndex)
	// Read API endpoints.
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/workspace", s.handleWorkspace)
	mux.HandleFunc("/api/issues", s.handleIssues)          // GET list, POST create
	mux.HandleFunc("/api/issues/", s.handleIssueRoutes)     // GET show, PATCH update, POST deps
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/graph/", s.handleGraph)
	return mux
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *beadsDashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(webui.BeadsIndex())
}

func (s *beadsDashboardServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	beadsWriteJSON(w, map[string]any{
		"status":    "ok",
		"read_only": s.readonly,
		"port":      s.port,
		"uptime":    time.Now().Format(time.RFC3339),
	})
}

func (s *beadsDashboardServer) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runner := s.adapter.Runner()
	beadsWriteJSON(w, map[string]any{
		"workspace": runner.Workspace,
		"bin":       runner.Bin,
	})
}

// handleIssues dispatches /api/issues: GET for list, POST for create.
func (s *beadsDashboardServer) handleIssues(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListIssues(w, r)
	case http.MethodPost:
		s.handleCreateIssue(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *beadsDashboardServer) handleListIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	var opts []beads.ListOption
	if status := q.Get("status"); status != "" {
		opts = append(opts, beads.WithStatus(beads.IssueStatus(status)))
	}
	if labels := q.Get("labels"); labels != "" {
		for _, l := range strings.Split(labels, ",") {
			l = strings.TrimSpace(l)
			if l != "" {
				opts = append(opts, beads.WithLabels(l))
			}
		}
	}
	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil && n > 0 {
			opts = append(opts, beads.WithLimit(n))
		}
	}
	issues, err := s.adapter.List(r.Context(), opts...)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, issues)
}

// handleIssueRoutes dispatches /api/issues/{id} and /api/issues/{id}/dependencies.
func (s *beadsDashboardServer) handleIssueRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/issues/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	// Check for /api/issues/{id}/dependencies
	if strings.HasSuffix(path, "/dependencies") {
		id := strings.TrimSuffix(path, "/dependencies")
		s.handleIssueDependencies(w, r, id)
		return
	}
	// Otherwise it's /api/issues/{id}
	switch r.Method {
	case http.MethodGet:
		s.handleIssueDetail(w, r, path)
	case http.MethodPatch:
		s.handleUpdateIssue(w, r, path)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *beadsDashboardServer) handleIssueDetail(w http.ResponseWriter, r *http.Request, id string) {
	issue, err := s.adapter.Show(r.Context(), id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, issue)
}

func (s *beadsDashboardServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		beadsWriteJSON(w, []beads.Issue{})
		return
	}
	issues, err := s.adapter.Search(r.Context(), query)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, issues)
}

func (s *beadsDashboardServer) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Extract issue ID from path: /api/graph/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/graph/")
	if id == "" {
		writeAPIError(w, fmt.Errorf("issue ID is required"))
		return
	}
	result, err := s.adapter.Graph(r.Context(), id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, result)
}

// ---------------------------------------------------------------------------
// Mutation handlers
// ---------------------------------------------------------------------------

// prepareBeadsMutation checks that the request is a safe mutation:
// - must be a loopback request
// - must pass same-origin check
// - server must not be in read-only mode
// Returns true if the caller should proceed.
func (s *beadsDashboardServer) prepareBeadsMutation(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		w.Header().Set("Allow", "POST, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "mutations require a loopback request", http.StatusForbidden)
		return false
	}
	if !isBeadsSameOrigin(r) {
		http.Error(w, "mutations require a same-origin request", http.StatusForbidden)
		return false
	}
	if s.readonly {
		http.Error(w, "dashboard is in read-only mode", http.StatusForbidden)
		return false
	}
	return true
}

// beadsDecodeJSONBody decodes a bounded JSON body, rejecting unknown fields.
func beadsDecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024 // 64 KB default
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(bufio.NewReader(r.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		beadsWriteAPIError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func (s *beadsDashboardServer) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	if !s.prepareBeadsMutation(w, r) {
		return
	}
	var input struct {
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		Design             string   `json:"design"`
		AcceptanceCriteria string   `json:"acceptance_criteria"`
		Priority           int      `json:"priority"`
		IssueType          string   `json:"issue_type"`
		Assignee           string   `json:"assignee"`
		Labels             []string `json:"labels"`
		EstimatedMinutes   int      `json:"estimated_minutes"`
		Dependencies       []string `json:"dependencies"`
	}
	if !beadsDecodeJSONBody(w, r, &input, 64*1024) {
		return
	}
	if strings.TrimSpace(input.Title) == "" {
		beadsWriteAPIError(w, http.StatusBadRequest, "title is required")
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	issue, err := s.adapter.CreateIssue(r.Context(), beads.CreateIssueInput{
		Title:              input.Title,
		Description:        input.Description,
		Design:             input.Design,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		IssueType:          input.IssueType,
		Assignee:           input.Assignee,
		Labels:             input.Labels,
		EstimatedMinutes:   input.EstimatedMinutes,
		Dependencies:       input.Dependencies,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(issue)
}

func (s *beadsDashboardServer) handleUpdateIssue(w http.ResponseWriter, r *http.Request, id string) {
	if !s.prepareBeadsMutation(w, r) {
		return
	}
	if !beads.IsValidIssueIDPublic(id) {
		beadsWriteAPIError(w, http.StatusBadRequest, "invalid issue ID")
		return
	}
	var input struct {
		Title              *string  `json:"title"`
		Description        *string  `json:"description"`
		Design             *string  `json:"design"`
		AcceptanceCriteria *string  `json:"acceptance_criteria"`
		Priority           *int     `json:"priority"`
		Status             *string  `json:"status"`
		Assignee           *string  `json:"assignee"`
		Labels             []string `json:"labels"`
		EstimatedMinutes   *int     `json:"estimated_minutes"`
	}
	if !beadsDecodeJSONBody(w, r, &input, 64*1024) {
		return
	}
	// Validate status enum if provided.
	var statusPtr *beads.IssueStatus
	if input.Status != nil {
		st := beads.IssueStatus(*input.Status)
		if !beads.IsValidStatus(st) {
			beadsWriteAPIError(w, http.StatusBadRequest, "invalid status: "+*input.Status)
			return
		}
		statusPtr = &st
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	issue, err := s.adapter.UpdateIssue(r.Context(), id, beads.UpdateIssueInput{
		Title:              input.Title,
		Description:        input.Description,
		Design:             input.Design,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		Status:             statusPtr,
		Assignee:           input.Assignee,
		Labels:             input.Labels,
		EstimatedMinutes:   input.EstimatedMinutes,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, issue)
}

func (s *beadsDashboardServer) handleIssueDependencies(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.prepareBeadsMutation(w, r) {
		return
	}
	if !beads.IsValidIssueIDPublic(id) {
		beadsWriteAPIError(w, http.StatusBadRequest, "invalid issue ID")
		return
	}
	var input struct {
		DependsOnID string `json:"depends_on_id"`
		Type        string `json:"type"`
	}
	if !beadsDecodeJSONBody(w, r, &input, 4*1024) {
		return
	}
	if strings.TrimSpace(input.DependsOnID) == "" {
		beadsWriteAPIError(w, http.StatusBadRequest, "depends_on_id is required")
		return
	}
	if !beads.IsValidIssueIDPublic(input.DependsOnID) {
		beadsWriteAPIError(w, http.StatusBadRequest, "invalid depends_on_id")
		return
	}
	depType := beads.DependencyType(input.Type)
	if depType == "" {
		depType = beads.DepBlocks
	}
	if !beads.IsValidDependencyType(depType) {
		beadsWriteAPIError(w, http.StatusBadRequest, "invalid dependency type: "+input.Type)
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.adapter.AddDependency(r.Context(), id, input.DependsOnID, depType); err != nil {
		writeAPIError(w, err)
		return
	}
	// Return refreshed issue.
	issue, err := s.adapter.Show(r.Context(), id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	beadsWriteJSON(w, issue)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func beadsWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(v)
}

func beadsWriteAPIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func writeAPIError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		status = http.StatusNotFound
	case strings.Contains(msg, "invalid"):
		status = http.StatusBadRequest
	case strings.Contains(msg, "timed out"):
		status = http.StatusGatewayTimeout
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
	})
}

// isBeadsSameOrigin checks that the Origin header (if present) matches the
// loopback host the dashboard is serving on.
func isBeadsSameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true // no Origin header is acceptable (same-origin by default)
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
	return strings.EqualFold(originHost, requestHost) && net.ParseIP(originHost) != nil && net.ParseIP(originHost).IsLoopback()
}
