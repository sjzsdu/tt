package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tt/internal/beads"
)

// ---------------------------------------------------------------------------
// Fake bd binary helper (shared with internal/beads tests)
// ---------------------------------------------------------------------------

func writeFakeBeadsBd(t *testing.T, dir, stdout string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake bd scripts require unix shell")
	}
	path := filepath.Join(dir, "bd")
	body := "#!/bin/sh\n"
	if stdout != "" {
		body += "printf '%s' '" + strings.ReplaceAll(stdout, "'", "'\\''") + "'\n"
	}
	if exitCode != 0 {
		body += "exit 1\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}
	return path
}

func newTestBeadsDashboard(t *testing.T, fakeJSON string) *beadsDashboardServer {
	return newTestBeadsDashboardWithOpts(t, fakeJSON, true)
}

func newTestBeadsDashboardWithOpts(t *testing.T, fakeJSON string, readonly bool) *beadsDashboardServer {
	t.Helper()
	dir := t.TempDir()
	bin := writeFakeBeadsBd(t, dir, fakeJSON, 0)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &beads.Runner{Bin: bin, Workspace: workspace, Timeout: 5 * time.Second}
	adapter := beads.NewAdapter(runner)
	return newBeadsDashboardServer(adapter, readonly)
}

// ---------------------------------------------------------------------------
// Health endpoint
// ---------------------------------------------------------------------------

func TestBeadsHealthEndpoint(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("health status = %v", body["status"])
	}
	if body["read_only"] != true {
		t.Fatalf("health read_only = %v", body["read_only"])
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("cache-control = %q", cache)
	}
}

func TestBeadsHealthRejectsPost(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST health status = %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Workspace endpoint
// ---------------------------------------------------------------------------

func TestBeadsWorkspaceEndpoint(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("workspace status = %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["workspace"] == nil || body["workspace"] == "" {
		t.Fatalf("workspace = %v", body["workspace"])
	}
	if body["bin"] == nil || body["bin"] == "" {
		t.Fatalf("bin = %v", body["bin"])
	}
}

// ---------------------------------------------------------------------------
// List issues endpoint
// ---------------------------------------------------------------------------

func TestBeadsListIssuesEndpoint(t *testing.T) {
	issues := `[{"id":"tt-abc","title":"Test issue","status":"open","priority":1}]`
	dashboard := newTestBeadsDashboard(t, issues)
	request := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
	var result []beads.Issue
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ID != "tt-abc" {
		t.Fatalf("list result = %+v", result)
	}
}

func TestBeadsListIssuesWithFilters(t *testing.T) {
	issues := `[{"id":"tt-abc","title":"Open issue","status":"open"}]`
	dashboard := newTestBeadsDashboard(t, issues)
	request := httptest.NewRequest(http.MethodGet, "/api/issues?status=open&labels=backend&limit=10", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list with filters status = %d", response.Code)
	}
}

func TestBeadsListIssuesRejectsPost(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodDelete, "/api/issues", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE list status = %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Issue detail endpoint
// ---------------------------------------------------------------------------

func TestBeadsIssueDetailEndpoint(t *testing.T) {
	issue := `[{"id":"tt-abc","title":"Detail test","status":"open","description":"A description"}]`
	dashboard := newTestBeadsDashboard(t, issue)
	request := httptest.NewRequest(http.MethodGet, "/api/issues/tt-abc", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", response.Code, response.Body.String())
	}
	var result beads.Issue
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != "tt-abc" {
		t.Fatalf("detail ID = %q", result.ID)
	}
}

func TestBeadsIssueDetailMissingID(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/api/issues/", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	// Should return 404 or 500 depending on how the empty ID is handled.
	if response.Code == http.StatusOK {
		t.Fatalf("expected error for empty issue ID, got %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Search endpoint
// ---------------------------------------------------------------------------

func TestBeadsSearchEndpoint(t *testing.T) {
	issues := `[{"id":"tt-abc","title":"Dashboard","status":"open"}]`
	dashboard := newTestBeadsDashboard(t, issues)
	request := httptest.NewRequest(http.MethodGet, "/api/search?q=dashboard", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d", response.Code)
	}
	var result []beads.Issue
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("search result = %d items", len(result))
	}
}

func TestBeadsSearchEmptyQuery(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty search status = %d", response.Code)
	}
	var result []beads.Issue
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("empty search result = %d items", len(result))
	}
}

// ---------------------------------------------------------------------------
// Graph endpoint
// ---------------------------------------------------------------------------

func TestBeadsGraphEndpoint(t *testing.T) {
	graph := `{"issues":[{"id":"tt-abc","title":"Root","status":"open"}],"dependencies":[]}`
	dashboard := newTestBeadsDashboard(t, graph)
	request := httptest.NewRequest(http.MethodGet, "/api/graph/tt-abc", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("graph status = %d, body = %s", response.Code, response.Body.String())
	}
	var result beads.GraphResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("graph issues = %d", len(result.Issues))
	}
}

func TestBeadsGraphMissingID(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "{}")
	request := httptest.NewRequest(http.MethodGet, "/api/graph/", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code == http.StatusOK {
		t.Fatalf("expected error for empty graph ID, got %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Index handler
// ---------------------------------------------------------------------------

func TestBeadsIndexHandler(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("index status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Beads Dashboard") && !strings.Contains(body, "beads") {
		t.Fatalf("index body = %q", body)
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("cache-control = %q", cache)
	}
}

func TestBeadsIndexNotFound(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("nonexistent path status = %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

func TestBeadsDashboardStartsOnLoopback(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	dashboard.open = nil // Don't open browser.
	if err := dashboard.start(0, false); err != nil {
		t.Fatal(err)
	}
	defer dashboard.close()
	if !strings.HasPrefix(dashboard.url(), "http://127.0.0.1:") {
		t.Fatalf("url = %q", dashboard.url())
	}
	// Verify we can reach the health endpoint.
	response, err := http.Get(dashboard.url() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
	// Verify double-start is rejected.
	if err := dashboard.start(0, false); err == nil {
		t.Fatal("expected already-running error")
	}
}

func TestBeadsDashboardCleanShutdown(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	dashboard.open = nil
	if err := dashboard.start(0, false); err != nil {
		t.Fatal(err)
	}
	url := dashboard.url()
	// Verify server is running.
	response, err := http.Get(url + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	// Trigger shutdown via context cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dashboard.wait(ctx)
	// After shutdown, requests should fail.
	_, err = http.Get(url + "/api/health")
	if err == nil {
		t.Fatal("expected connection error after shutdown")
	}
}

func TestBeadsDashboardPortSelection(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	dashboard.open = nil
	// Use a specific port. We try 19720 which is unlikely to be in use.
	if err := dashboard.start(19720, false); err != nil {
		// If port is taken, that's OK for CI; skip.
		t.Skipf("port 19720 unavailable: %v", err)
	}
	defer dashboard.close()
	if dashboard.port < 19720 || dashboard.port > 19740 {
		t.Fatalf("port = %d, want in [19720, 19740]", dashboard.port)
	}
}

func TestBeadsDashboardInvalidPort(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	if err := dashboard.start(-1, false); err == nil {
		t.Fatal("expected error for negative port")
	}
	if err := dashboard.start(70000, false); err == nil {
		t.Fatal("expected error for port > 65535")
	}
}

// ---------------------------------------------------------------------------
// Favicon
// ---------------------------------------------------------------------------

func TestBeadsFaviconHandler(t *testing.T) {
	dashboard := newTestBeadsDashboard(t, "[]")
	request := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	response := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("favicon status = %d", response.Code)
	}
}

// ---------------------------------------------------------------------------
// Mutation helpers
// ---------------------------------------------------------------------------

func localBeadsMutationRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var bodyReader *bytes.Buffer
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	} else {
		bodyReader = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.RemoteAddr = "127.0.0.1:43120"
	req.Host = "127.0.0.1:9720"
	req.Header.Set("Origin", "http://127.0.0.1:9720")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// ---------------------------------------------------------------------------
// Create issue tests
// ---------------------------------------------------------------------------

func TestBeadsCreateIssueEndpoint(t *testing.T) {
	created := `[{"id":"tt-new","title":"Created","status":"open"}]`
	dashboard := newTestBeadsDashboardWithOpts(t, created, false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{"title":"Created"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var issue beads.Issue
	if err := json.Unmarshal(resp.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.ID != "tt-new" {
		t.Fatalf("created issue ID = %q", issue.ID)
	}
}

func TestBeadsCreateIssueEmptyTitle(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{"title":""}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("empty title status = %d", resp.Code)
	}
}

func TestBeadsCreateIssueReadOnly(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", true) // read-only
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{"title":"Test"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("read-only create status = %d", resp.Code)
	}
}

func TestBeadsCreateIssueRejectsRemote(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{"title":"Test"}`)
	req.RemoteAddr = "203.0.113.8:9000" // non-loopback
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("remote create status = %d", resp.Code)
	}
}

func TestBeadsCreateIssueRejectsCrossOrigin(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{"title":"Test"}`)
	req.Header.Set("Origin", "https://evil.com")
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("cross-origin create status = %d", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Update issue tests
// ---------------------------------------------------------------------------

func TestBeadsUpdateIssueEndpoint(t *testing.T) {
	updated := `[{"id":"tt-abc","title":"Updated","status":"in_progress"}]`
	dashboard := newTestBeadsDashboardWithOpts(t, updated, false)
	req := localBeadsMutationRequest(t, http.MethodPatch, "/api/issues/tt-abc", `{"title":"Updated","status":"in_progress"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var issue beads.Issue
	if err := json.Unmarshal(resp.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.Title != "Updated" {
		t.Fatalf("updated title = %q", issue.Title)
	}
}

func TestBeadsUpdateIssueInvalidStatus(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPatch, "/api/issues/tt-abc", `{"status":"invalid"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d", resp.Code)
	}
}

func TestBeadsUpdateIssueReadOnly(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", true)
	req := localBeadsMutationRequest(t, http.MethodPatch, "/api/issues/tt-abc", `{"title":"X"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("read-only update status = %d", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Add dependency tests
// ---------------------------------------------------------------------------

func TestBeadsAddDependencyEndpoint(t *testing.T) {
	result := `[{"id":"tt-abc","title":"Root","status":"open"}]`
	dashboard := newTestBeadsDashboardWithOpts(t, result, false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues/tt-abc/dependencies",
		`{"depends_on_id":"tt-def","type":"blocks"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("add dep status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestBeadsAddDependencyMissingTarget(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues/tt-abc/dependencies",
		`{"depends_on_id":"","type":"blocks"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("missing target status = %d", resp.Code)
	}
}

func TestBeadsAddDependencyInvalidType(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues/tt-abc/dependencies",
		`{"depends_on_id":"tt-def","type":"invalid"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid dep type status = %d", resp.Code)
	}
}

func TestBeadsAddDependencyReadOnly(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", true)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues/tt-abc/dependencies",
		`{"depends_on_id":"tt-def","type":"blocks"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("read-only dep status = %d", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Invalid JSON / unknown fields
// ---------------------------------------------------------------------------

func TestBeadsCreateIssueRejectsUnknownFields(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues",
		`{"title":"Test","unknown_field":"bad"}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields status = %d", resp.Code)
	}
}

func TestBeadsCreateIssueRejectsInvalidJSON(t *testing.T) {
	dashboard := newTestBeadsDashboardWithOpts(t, "[]", false)
	req := localBeadsMutationRequest(t, http.MethodPost, "/api/issues", `{not json}`)
	resp := httptest.NewRecorder()
	dashboard.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status = %d", resp.Code)
	}
}
