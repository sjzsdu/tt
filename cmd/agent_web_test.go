package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func TestAgentWebStateHandler(t *testing.T) {
	server := &agentWebServer{
		started:   time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		workspace: "/tmp/project",
		defaults:  agentWebDefaults{Agent: "coder", Session: "cli:test", Model: "test-model"},
		embedded: []pcwrap.EmbeddedAgent{
			{ID: "coder", Name: "Coder", Description: "writes code", Aliases: []string{"coder"}, Skills: []string{"code-context"}, Tools: []string{"exec"}},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	w := httptest.NewRecorder()
	server.handleState(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var state agentWebState
	if err := json.NewDecoder(w.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Workspace != "/tmp/project" {
		t.Fatalf("workspace = %q", state.Workspace)
	}
	if state.Defaults.Agent != "coder" || state.Defaults.Session != "cli:test" {
		t.Fatalf("defaults = %+v", state.Defaults)
	}
	if len(state.Agents) != 1 || state.Agents[0].ID != "coder" {
		t.Fatalf("agents = %+v", state.Agents)
	}
}

func TestAgentWebIndexHandler(t *testing.T) {
	server := &agentWebServer{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	server.handleIndex(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	if body := w.Body.String(); body == "" || !strings.Contains(body, "tt agent web") {
		t.Fatalf("index body does not contain title")
	}
}
