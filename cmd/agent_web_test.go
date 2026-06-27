package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestParseAgentWebTranscript(t *testing.T) {
	content := strings.Join([]string{
		`{"role":"user","content":"hello"}`,
		`{"role":"assistant","content":"# Hi\n\n**ok**"}`,
		`not json`,
		`{"role":"tool","content":{"name":"exec"}}`,
	}, "\n")
	messages := parseAgentWebTranscript(content)
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3: %+v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Fatalf("first message = %+v", messages[0])
	}
	if messages[1].Role != "assistant" || !strings.Contains(messages[1].Content, "**ok**") {
		t.Fatalf("second message = %+v", messages[1])
	}
}

func TestAgentWebTranscriptHandler(t *testing.T) {
	workspace := t.TempDir()
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(sessionsDir, "agent_coder_cli_test.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"role":"user","content":"hello"}`+"\n"+`{"role":"assistant","content":"world"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &agentWebServer{workspace: workspace, defaults: agentWebDefaults{Agent: "coder", Session: "cli:test"}}
	req := httptest.NewRequest(http.MethodGet, "/api/transcript?agent=coder&session=cli:test", nil)
	w := httptest.NewRecorder()
	server.handleTranscript(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp agentWebTranscriptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Missing || resp.Path != transcript {
		t.Fatalf("response = %+v", resp)
	}
	if len(resp.Messages) != 2 || resp.Messages[1].Content != "world" {
		t.Fatalf("messages = %+v", resp.Messages)
	}
}

func TestParseAgentWebSessionFilename(t *testing.T) {
	agent, session := parseAgentWebSessionFilename("agent_coder_cli_test.jsonl")
	if agent != "coder" || session != "cli_test" {
		t.Fatalf("agent/session = %q/%q", agent, session)
	}
	agent, session = parseAgentWebSessionFilename("plain_session.jsonl")
	if agent != "" || session != "plain_session" {
		t.Fatalf("plain agent/session = %q/%q", agent, session)
	}
}

func TestAgentWebSessionsHandler(t *testing.T) {
	workspace := t.TempDir()
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"agent_coder_cli_one.jsonl", "agent_planner_cli_two.jsonl", "ignore.txt"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(sessionsDir, name), []byte(`{"role":"user","content":"hi"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := &agentWebServer{workspace: workspace}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	server.handleSessions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp agentWebSessionsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Missing || len(resp.Sessions) != 2 {
		t.Fatalf("response = %+v", resp)
	}
	foundCoder := false
	for _, session := range resp.Sessions {
		if session.Agent == "coder" && session.Session == "cli_one" {
			foundCoder = true
		}
	}
	if !foundCoder {
		t.Fatalf("coder session not found: %+v", resp.Sessions)
	}
}
