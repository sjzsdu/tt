package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
	ttconfig "github.com/sjzsdu/tt/internal/ttconfig"
)

type agentWebServer struct {
	mu        sync.Mutex
	server    *http.Server
	port      int
	started   time.Time
	workspace string
	cfg       ttconfig.Config
	rt        *pcwrap.Runtime
	runner    *pcwrap.DirectRunner
	embedded  []pcwrap.EmbeddedAgent
	defaults  agentWebDefaults
}

type agentWebDefaults struct {
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Session string `json:"session"`
	Debug   bool   `json:"debug"`
}

type agentWebAgent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

type agentWebState struct {
	StartedAt string           `json:"started_at"`
	Workspace string           `json:"workspace"`
	Defaults  agentWebDefaults `json:"defaults"`
	Agents    []agentWebAgent  `json:"agents"`
}

type agentWebChatRequest struct {
	Message string `json:"message"`
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Model   string `json:"model"`
}

type agentWebChatResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

func runAgentWeb(cmd *cobra.Command, cfg ttconfig.Config, sources ttconfig.Sources, flags agentRunFlags) error {
	workspace, resolvedHome, resolvedConfig, restoreStorage, err := useTTAgentStorage(cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	if err != nil {
		return err
	}
	defer restoreStorage()
	cfg.Picoclaw.Home = resolvedHome
	cfg.Picoclaw.Config = resolvedConfig
	if err := ensurePicoclawConfigAvailable(cfg.Picoclaw.Home, cfg.Picoclaw.Config); err != nil {
		return err
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:      cfg.Picoclaw.Home,
		Config:    cfg.Picoclaw.Config,
		TTConfig:  cfg,
		TTSources: sources,
	})
	if err != nil {
		return picoclawUnavailableError(err, cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	}

	embeddedAgents, err := agents.All()
	if err != nil {
		return fmt.Errorf("load agents failed: %w", err)
	}

	debug := flags.Debug
	if cfg.Agent.Debug != nil {
		debug = *cfg.Agent.Debug
	}
	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          cfg.Agent.Agent,
		Model:          cfg.Agent.Model,
		Workspace:      workspace,
		Debug:          debug,
		Quiet:          !debug,
		EmbeddedAgents: embeddedAgents,
	})
	if err != nil {
		return picoclawUnavailableError(err, cfg.Picoclaw.Home, cfg.Picoclaw.Config)
	}
	defer runner.Close()

	server := &agentWebServer{
		started:   time.Now(),
		workspace: workspace,
		cfg:       cfg,
		rt:        rt,
		runner:    runner,
		embedded:  embeddedAgents,
		defaults: agentWebDefaults{
			Agent:   cfg.Agent.Agent,
			Model:   cfg.Agent.Model,
			Session: cfg.Agent.Session,
			Debug:   debug,
		},
	}
	if server.defaults.Session == "" {
		server.defaults.Session = "cli:default"
	}
	if err := server.start(flags.WebPort); err != nil {
		return err
	}
	server.waitForInterrupt()
	return nil
}

func (s *agentWebServer) start(port int) error {
	if port <= 0 {
		port = 9710
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/chat", s.handleChat)

	maxPort := port + 20
	var lastErr error
	for candidate := port; candidate <= maxPort; candidate++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err != nil {
			lastErr = err
			continue
		}
		srv := &http.Server{Addr: fmt.Sprintf(":%d", candidate), Handler: mux}
		s.server = srv
		s.port = candidate
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				fmt.Printf("agent web server failed: %v\n", err)
			}
		}()
		url := fmt.Sprintf("http://localhost:%d", candidate)
		fmt.Printf("Agent web UI started: %s\n", url)
		fmt.Println("Press Ctrl-C to stop the web UI.")
		go openBrowser(url)
		return nil
	}
	return fmt.Errorf("all candidate agent web ports unavailable: %v", lastErr)
}

func (s *agentWebServer) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
		s.server = nil
	}
}

func (s *agentWebServer) waitForInterrupt() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	s.close()
}

func (s *agentWebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = agentWebIndexTemplate.Execute(w, nil)
}

func (s *agentWebServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeAgentWebJSON(w, s.state())
}

func (s *agentWebServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req agentWebChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Agent) == "" {
		req.Agent = s.defaults.Agent
	}
	if strings.TrimSpace(req.Session) == "" {
		req.Session = s.defaults.Session
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = s.defaults.Model
	}

	s.mu.Lock()
	response, err := s.runner.ProcessDirectContext(r.Context(), pcwrap.RunOptions{
		Message:        req.Message,
		Agent:          req.Agent,
		Session:        req.Session,
		Model:          req.Model,
		Workspace:      s.workspace,
		Debug:          s.defaults.Debug,
		Quiet:          !s.defaults.Debug,
		EmbeddedAgents: s.embedded,
	})
	s.mu.Unlock()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeAgentWebJSON(w, agentWebChatResponse{Error: err.Error()})
		return
	}
	writeAgentWebJSON(w, agentWebChatResponse{Response: response})
}

func (s *agentWebServer) state() agentWebState {
	agentsOut := make([]agentWebAgent, 0, len(s.embedded))
	for _, agent := range s.embedded {
		agentsOut = append(agentsOut, agentWebAgent{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Aliases:     agent.Aliases,
			Skills:      agent.Skills,
			Tools:       agent.Tools,
		})
	}
	return agentWebState{
		StartedAt: s.started.Format(time.RFC3339),
		Workspace: s.workspace,
		Defaults:  s.defaults,
		Agents:    agentsOut,
	}
}

func writeAgentWebJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

var agentWebIndexTemplate = template.Must(template.New("agent-web").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<title>tt agent web</title>
<style>
:root{color-scheme:dark;--bg:#0d1117;--panel:#161b22;--line:#30363d;--text:#e6edf3;--muted:#8b949e;--accent:#2f81f7;--danger:#f85149}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.app{display:grid;grid-template-columns:280px 1fr;min-height:100vh}.side{border-right:1px solid var(--line);background:var(--panel);padding:16px;overflow:auto}.main{display:flex;flex-direction:column;min-width:0}
h1{font-size:18px;margin:0 0 12px}.label{display:block;margin:12px 0 6px;color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.05em}
select,input,textarea,button{width:100%;border:1px solid var(--line);border-radius:8px;background:#0d1117;color:var(--text);padding:10px;font:inherit}button{background:var(--accent);border-color:var(--accent);font-weight:600;cursor:pointer}button:disabled{opacity:.55;cursor:not-allowed}
.meta{color:var(--muted);font-size:12px;word-break:break-all;margin-top:12px}.messages{flex:1;overflow:auto;padding:24px;display:flex;flex-direction:column;gap:14px}.msg{max-width:980px;border:1px solid var(--line);border-radius:12px;padding:14px 16px;white-space:pre-wrap}.user{align-self:flex-end;background:#1f2937}.assistant{align-self:flex-start;background:var(--panel)}.error{border-color:var(--danger);color:#ffb4ad}.composer{border-top:1px solid var(--line);padding:16px;background:var(--panel);display:grid;grid-template-columns:1fr 120px;gap:12px}textarea{min-height:64px;resize:vertical}.hint{color:var(--muted);font-size:12px;margin-top:8px}
</style>
</head>
<body>
<div class="app">
  <aside class="side">
    <h1>tt agent web</h1>
    <label class="label" for="agent">Agent</label>
    <select id="agent"></select>
    <label class="label" for="session">Session</label>
    <input id="session" placeholder="cli:default" />
    <label class="label" for="model">Model</label>
    <input id="model" placeholder="default" />
    <div class="meta" id="meta"></div>
    <div class="hint">MVP：当前页面使用同步请求，长任务期间请等待返回。Ctrl+Enter 发送。</div>
  </aside>
  <main class="main">
    <div class="messages" id="messages"></div>
    <form class="composer" id="form">
      <textarea id="message" placeholder="输入消息，例如：帮我理解这个项目的架构"></textarea>
      <button id="send" type="submit">发送</button>
    </form>
  </main>
</div>
<script>
const state={agents:[],busy:false};
const $=id=>document.getElementById(id);
function add(role,text,cls=''){const el=document.createElement('div');el.className='msg '+role+(cls?' '+cls:'');el.textContent=text;$('messages').appendChild(el);$('messages').scrollTop=$('messages').scrollHeight;return el;}
async function loadState(){const res=await fetch('/api/state');const data=await res.json();state.agents=data.agents||[];const sel=$('agent');sel.innerHTML='';for(const a of state.agents){const opt=document.createElement('option');opt.value=a.id;opt.textContent=a.description?(a.id+' - '+a.description):a.id;sel.appendChild(opt);}if(data.defaults?.agent)sel.value=data.defaults.agent;if(data.defaults?.session)$('session').value=data.defaults.session;if(data.defaults?.model)$('model').value=data.defaults.model;$('meta').textContent='Workspace: '+(data.workspace||'(unknown)');add('assistant','Agent web 已启动。请选择 agent 后开始对话。');}
$('form').addEventListener('submit',async e=>{e.preventDefault();if(state.busy)return;const text=$('message').value.trim();if(!text)return;$('message').value='';add('user',text);state.busy=true;$('send').disabled=true;const pending=add('assistant','正在等待 agent 回复...');try{const res=await fetch('/api/chat',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:text,agent:$('agent').value,session:$('session').value,model:$('model').value})});const data=await res.json().catch(()=>({error:'invalid server response'}));pending.textContent=data.error||data.response||'';if(!res.ok||data.error)pending.classList.add('error');}catch(err){pending.textContent=String(err);pending.classList.add('error');}finally{state.busy=false;$('send').disabled=false;$('message').focus();}});
$('message').addEventListener('keydown',e=>{if(e.key==='Enter'&&e.ctrlKey){$('form').requestSubmit();}});
loadState().catch(err=>add('assistant',String(err),'error'));
</script>
</body>
</html>`))
