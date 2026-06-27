package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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

type agentWebTranscriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agentWebTranscriptResponse struct {
	Session  string                      `json:"session"`
	Agent    string                      `json:"agent,omitempty"`
	Path     string                      `json:"path,omitempty"`
	Messages []agentWebTranscriptMessage `json:"messages,omitempty"`
	Missing  bool                        `json:"missing,omitempty"`
	Message  string                      `json:"message,omitempty"`
}

type agentWebSessionInfo struct {
	Session   string `json:"session"`
	Agent     string `json:"agent,omitempty"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

type agentWebSessionsResponse struct {
	Sessions []agentWebSessionInfo `json:"sessions"`
	Missing  bool                  `json:"missing,omitempty"`
	Message  string                `json:"message,omitempty"`
}

const maxAgentWebTranscriptBytes = 256 * 1024

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
	mux.HandleFunc("/api/transcript", s.handleTranscript)
	mux.HandleFunc("/api/sessions", s.handleSessions)

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

func (s *agentWebServer) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	if session == "" {
		session = s.defaults.Session
	}
	if agent == "" {
		agent = s.defaults.Agent
	}
	path, content, err := readAgentWebTranscript(s.workspace, session, agent)
	if err != nil {
		writeAgentWebJSON(w, agentWebTranscriptResponse{Session: session, Agent: agent, Missing: true, Message: err.Error()})
		return
	}
	messages := parseAgentWebTranscript(content)
	writeAgentWebJSON(w, agentWebTranscriptResponse{Session: session, Agent: agent, Path: path, Messages: messages})
}

func (s *agentWebServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions, err := listAgentWebSessions(s.workspace)
	if err != nil {
		writeAgentWebJSON(w, agentWebSessionsResponse{Missing: true, Message: err.Error()})
		return
	}
	writeAgentWebJSON(w, agentWebSessionsResponse{Sessions: sessions})
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

func readAgentWebTranscript(workspace, session, agent string) (string, string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", "", fmt.Errorf("workspace is not available")
	}
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", "", fmt.Errorf("read sessions dir: %w", err)
	}
	slug := agentWebSessionFilenameToken(session)
	agentPrefix := ""
	if strings.TrimSpace(agent) != "" {
		agentPrefix = "agent_" + agentWebSessionFilenameToken(agent) + "_"
	}
	var best string
	var bestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := entry.Name()
		if agentPrefix != "" && !strings.HasPrefix(name, agentPrefix) {
			continue
		}
		if slug != "" && !strings.Contains(name, slug) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best = filepath.Join(sessionsDir, name)
			bestMod = info.ModTime()
		}
	}
	if best == "" {
		return "", "", fmt.Errorf("session transcript not found under %s", sessionsDir)
	}
	content, err := readAgentWebFileTail(best, maxAgentWebTranscriptBytes)
	if err != nil {
		return "", "", err
	}
	return best, content, nil
}

func listAgentWebSessions(workspace string) ([]agentWebSessionInfo, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is not available")
	}
	sessionsDir := filepath.Join(workspace, ".tt", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}
	sessions := make([]agentWebSessionInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		agent, session := parseAgentWebSessionFilename(entry.Name())
		if session == "" {
			continue
		}
		sessions = append(sessions, agentWebSessionInfo{
			Session:   session,
			Agent:     agent,
			Path:      filepath.Join(sessionsDir, entry.Name()),
			UpdatedAt: info.ModTime().Format(time.RFC3339),
			Size:      info.Size(),
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	return sessions, nil
}

func parseAgentWebSessionFilename(name string) (agent, session string) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".jsonl")
	if name == "" {
		return "", ""
	}
	const prefix = "agent_"
	if !strings.HasPrefix(name, prefix) {
		return "", name
	}
	rest := strings.TrimPrefix(name, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return "", rest
	}
	return parts[0], parts[1]
}

func parseAgentWebTranscript(content string) []agentWebTranscriptMessage {
	lines := strings.Split(content, "\n")
	messages := make([]agentWebTranscriptMessage, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "...") {
			continue
		}
		var raw struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		role := strings.TrimSpace(raw.Role)
		if role == "" {
			role = "assistant"
		}
		content := strings.TrimSpace(string(raw.Content))
		var text string
		if len(raw.Content) > 0 {
			if err := json.Unmarshal(raw.Content, &text); err != nil {
				text = strings.TrimSpace(string(raw.Content))
			}
		}
		text = strings.TrimSpace(text)
		if text == "" && content != "" && content != "null" {
			text = content
		}
		if text == "" {
			continue
		}
		messages = append(messages, agentWebTranscriptMessage{Role: role, Content: text})
	}
	return messages
}

func agentWebSessionFilenameToken(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		keep := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-'
		if keep {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func readAgentWebFileTail(path string, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	offset := int64(0)
	if info.Size() > maxBytes {
		offset = info.Size() - maxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	if offset > 0 {
		return "... transcript truncated to last 256 KiB ...\n" + string(data), nil
	}
	return string(data), nil
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
	.meta{color:var(--muted);font-size:12px;word-break:break-all;margin-top:12px}.messages{flex:1;overflow:auto;padding:24px;display:flex;flex-direction:column;gap:14px}.msg{max-width:980px;border:1px solid var(--line);border-radius:12px;padding:14px 16px}.msg-content{white-space:normal}.msg-content p{margin:.4em 0}.msg-content pre{overflow:auto;background:#0b1020;border:1px solid var(--line);border-radius:8px;padding:12px}.msg-content code{background:#0b1020;border:1px solid var(--line);border-radius:5px;padding:1px 4px}.msg-content pre code{border:0;padding:0}.msg-content a{color:#79c0ff}.msg-content h1,.msg-content h2,.msg-content h3{margin:.6em 0 .3em}.msg-content ul{margin:.4em 0 .4em 1.4em;padding:0}.user{align-self:flex-end;background:#1f2937}.assistant{align-self:flex-start;background:var(--panel)}.system{align-self:center;color:var(--muted);font-size:12px}.error{border-color:var(--danger);color:#ffb4ad}.composer{border-top:1px solid var(--line);padding:16px;background:var(--panel);display:grid;grid-template-columns:1fr 120px;gap:12px}textarea{min-height:64px;resize:vertical}.hint{color:var(--muted);font-size:12px;margin-top:8px}.secondary{margin-top:10px;background:#21262d;border-color:var(--line)}
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
	    <label class="label" for="session-list">History</label>
	    <select id="session-list"></select>
	    <button class="secondary" id="new-session" type="button">新建 session</button>
	    <label class="label" for="model">Model</label>
	    <input id="model" placeholder="default" />
	    <button class="secondary" id="load-history" type="button">加载历史</button>
	    <div class="meta" id="meta"></div>
	    <div class="hint">当前使用同步请求，长任务期间请等待返回。回复支持轻量 Markdown 渲染。Ctrl+Enter 发送。</div>
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
	const state={agents:[],sessions:[],busy:false,loadedHistory:false};
	const $=id=>document.getElementById(id);
	function escapeHtml(s){return String(s||'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}
	function inlineMd(s){return escapeHtml(s).replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g,'<a href="$2" target="_blank" rel="noreferrer">$1</a>').replace(/\*\*([^*]+)\*\*/g,'<strong>$1</strong>').replace(/\x60([^\x60]+)\x60/g,'<code>$1</code>');}
	function renderMarkdown(text){const fence=String.fromCharCode(96,96,96);const lines=String(text||'').split('\n');let html='',inCode=false,buf=[];function flushPara(){if(buf.length){html+='<p>'+inlineMd(buf.join(' '))+'</p>';buf=[];}}for(const line of lines){if(line.startsWith(fence)){flushPara();inCode=!inCode;if(inCode){html+='<pre><code>';}else{html+='</code></pre>';}continue;}if(inCode){html+=escapeHtml(line)+'\n';continue;}if(/^#{1,3}\s+/.test(line)){flushPara();const n=line.match(/^#+/)[0].length;html+='<h'+n+'>'+inlineMd(line.replace(/^#{1,3}\s+/,''))+'</h'+n+'>';continue;}if(/^[-*]\s+/.test(line)){flushPara();html+='<ul><li>'+inlineMd(line.replace(/^[-*]\s+/,''))+'</li></ul>';continue;}if(line.trim()===''){flushPara();continue;}buf.push(line.trim());}flushPara();if(inCode)html+='</code></pre>';return html;}
	function setMessage(el,text,cls){el.className='msg '+(cls||'');const body=el.querySelector('.msg-content')||el;body.innerHTML=renderMarkdown(text);$('messages').scrollTop=$('messages').scrollHeight;}
	function add(role,text,cls=''){const el=document.createElement('div');el.className='msg '+role+(cls?' '+cls:'');const body=document.createElement('div');body.className='msg-content';body.innerHTML=renderMarkdown(text);el.appendChild(body);$('messages').appendChild(el);$('messages').scrollTop=$('messages').scrollHeight;return el;}
	function clearMessages(){ $('messages').innerHTML=''; }
	function sessionLabel(s){const agent=s.agent?('['+s.agent+'] '):'';const size=s.size?(' · '+Math.round(s.size/1024)+' KiB'):'';const at=s.updated_at?(' · '+new Date(s.updated_at).toLocaleString()):'';return agent+s.session+size+at;}
	function refreshSessionSelect(){const sel=$('session-list');sel.innerHTML='';const empty=document.createElement('option');empty.value='';empty.textContent='选择历史 session...';sel.appendChild(empty);const currentAgent=$('agent').value;for(const s of state.sessions){if(currentAgent&&s.agent&&s.agent!==currentAgent)continue;const opt=document.createElement('option');opt.value=s.session;opt.dataset.agent=s.agent||'';opt.textContent=sessionLabel(s);sel.appendChild(opt);}}
	async function loadSessions(){const res=await fetch('/api/sessions');const data=await res.json();state.sessions=data.sessions||[];refreshSessionSelect();}
	async function loadTranscript(){const params=new URLSearchParams({agent:$('agent').value||'',session:$('session').value||''});const res=await fetch('/api/transcript?'+params.toString());const data=await res.json();clearMessages();if(data.missing){add('system','没有找到这个 session 的历史记录，可以直接开始新对话。');return;}for(const m of data.messages||[]){const role=m.role==='user'?'user':'assistant';add(role,m.content);}add('system','已加载历史：'+(data.path||''));state.loadedHistory=true;}
	async function loadState(){const res=await fetch('/api/state');const data=await res.json();state.agents=data.agents||[];const sel=$('agent');sel.innerHTML='';for(const a of state.agents){const opt=document.createElement('option');opt.value=a.id;opt.textContent=a.description?(a.id+' - '+a.description):a.id;sel.appendChild(opt);}if(data.defaults?.agent)sel.value=data.defaults.agent;if(data.defaults?.session)$('session').value=data.defaults.session;if(data.defaults?.model)$('model').value=data.defaults.model;$('meta').textContent='Workspace: '+(data.workspace||'(unknown)');await loadSessions();await loadTranscript();}
	$('form').addEventListener('submit',async e=>{e.preventDefault();if(state.busy)return;const text=$('message').value.trim();if(!text)return;$('message').value='';add('user',text);state.busy=true;$('send').disabled=true;const pending=add('assistant','正在等待 agent 回复...');try{const res=await fetch('/api/chat',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:text,agent:$('agent').value,session:$('session').value,model:$('model').value})});const data=await res.json().catch(()=>({error:'invalid server response'}));setMessage(pending,data.error||data.response||'','assistant'+((!res.ok||data.error)?' error':''));}catch(err){setMessage(pending,String(err),'assistant error');}finally{state.busy=false;$('send').disabled=false;$('message').focus();}});
	$('load-history').addEventListener('click',()=>loadTranscript().catch(err=>{clearMessages();add('assistant',String(err),'error');}));
	$('session-list').addEventListener('change',()=>{const opt=$('session-list').selectedOptions[0];if(!opt||!opt.value)return;$('session').value=opt.value;if(opt.dataset.agent)$('agent').value=opt.dataset.agent;loadTranscript().catch(err=>{clearMessages();add('assistant',String(err),'error');});});
	$('new-session').addEventListener('click',()=>{const d=new Date();const stamp=d.toISOString().replace(/[-:.TZ]/g,'').slice(0,14);$('session').value='web:'+($('agent').value||'agent')+':'+stamp;$('session-list').value='';clearMessages();add('system','已创建新 session：'+$('session').value);$('message').focus();});
	$('agent').addEventListener('change',()=>{refreshSessionSelect();loadTranscript().catch(()=>{});});
	$('session').addEventListener('change',()=>loadTranscript().catch(()=>{}));
$('message').addEventListener('keydown',e=>{if(e.key==='Enter'&&e.ctrlKey){$('form').requestSubmit();}});
loadState().catch(err=>add('assistant',String(err),'error'));
</script>
</body>
</html>`))
