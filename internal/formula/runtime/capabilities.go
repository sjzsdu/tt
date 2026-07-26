package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tt/internal/formula/steps"
)

type ScriptCapability struct {
	AllowShell     bool
	DenyUnsafe     bool
	DefaultTimeout time.Duration
}

type scriptCapabilityOutput struct {
	Command    []string `json:"command"`
	Cwd        string   `json:"cwd,omitempty"`
	ExitCode   int      `json:"exit_code"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

func (c ScriptCapability) RunScript(ctx context.Context, req steps.ScriptRequest) (steps.Value, error) {
	argv := compactCommand(req.Command)
	if len(argv) == 0 {
		return steps.Value{}, fmt.Errorf("script command is required")
	}
	if c.DenyUnsafe {
		if err := ValidateScriptCommand(argv); err != nil {
			return steps.Value{}, err
		}
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		if !filepath.IsAbs(cwd) {
			if wd, err := os.Getwd(); err == nil {
				cwd = filepath.Join(wd, cwd)
			}
		}
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	started := time.Now()
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("script timed out after %s", timeout)
	}
	out := scriptCapabilityOutput{Command: argv, Cwd: cmd.Dir, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), DurationMS: time.Since(started).Milliseconds()}
	data, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		return steps.Value{}, marshalErr
	}
	return steps.Value{Type: "json", Raw: data}, err
}

func compactCommand(command []string) []string {
	out := make([]string, 0, len(command))
	for _, part := range command {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func ValidateScriptCommand(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("script command is required")
	}
	base := filepath.Base(argv[0])
	dangerous := map[string]bool{"rm": true, "rmdir": true, "mkfs": true, "dd": true, "shutdown": true, "reboot": true, "halt": true, "poweroff": true, "sudo": true, "su": true, "chmod": true, "chown": true}
	if dangerous[base] {
		return fmt.Errorf("script command %q is denied by formula safety policy", base)
	}
	joined := strings.ToLower(strings.Join(argv, " "))
	patterns := []string{"rm -rf", ":(){", "> /dev/", "mkfs.", "curl | sh", "wget | sh"}
	for _, p := range patterns {
		if strings.Contains(joined, p) {
			return fmt.Errorf("script command contains denied pattern %q", p)
		}
	}
	return nil
}

type DryRunAgentCapability struct{}

func (DryRunAgentCapability) RunAgent(_ context.Context, req steps.AgentRequest) (steps.Value, error) {
	data, err := json.Marshal(map[string]string{"dry_run": "true", "agent": req.Agent, "model": req.Model, "prompt": req.Prompt})
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}

type DryRunScriptCapability struct{}

func (DryRunScriptCapability) RunScript(_ context.Context, req steps.ScriptRequest) (steps.Value, error) {
	data, err := json.Marshal(map[string]any{"dry_run": true, "command": req.Command, "cwd": req.Cwd, "env": req.Env})
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}

// ExternalAgentCapability spawns an external agent CLI (default: jcode) and
// surfaces its structured response. The capability supports four drivers out
// of the box: jcode, codex, opencode, and forge. bl is listed as opt-in but
// routes through jcode's --provider abstraction where supported.
type ExternalAgentCapability struct {
	// Driver is the default agent CLI to invoke when a step omits `driver`.
	// Defaults to "jcode" when empty.
	Driver string
	// DefaultProvider / DefaultModel are inherited by every step unless
	// overridden at the step level.
	DefaultProvider string
	DefaultModel    string
	// DefaultTimeout caps each invocation. Zero falls back to 5 minutes.
	DefaultTimeout time.Duration
	// Resolver maps a driver name to its binary on PATH. Empty entries fall
	// back to looking up the driver name directly via exec.LookPath.
	Resolver func(driver string) (string, error)
}

type externalAgentOutput struct {
	Driver    string `json:"driver"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Mode      string `json:"mode,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Duration  int64  `json:"duration_ms"`
}

func (c ExternalAgentCapability) RunExternalAgent(ctx context.Context, req steps.ExternalAgentRequest) (steps.Value, error) {
	driver := strings.TrimSpace(req.Driver)
	if driver == "" {
		driver = strings.TrimSpace(c.Driver)
	}
	if driver == "" {
		driver = steps.DefaultExternalAgentDriver
	}
	if !steps.SupportedExternalAgentDrivers[driver] {
		return steps.Value{}, fmt.Errorf("external_agent driver %q is not supported", driver)
	}
	bin, err := c.lookupDriver(driver)
	if err != nil {
		return steps.Value{}, err
	}
	provider := firstNonEmpty(req.Provider, c.DefaultProvider)
	model := firstNonEmpty(req.Model, c.DefaultModel)
	extraArgs := append([]string(nil), req.ExtraArgs...)
	codexLastMessagePath := ""
	if driver == "codex" && !hasExternalAgentArg(extraArgs, "--output-last-message", "-o") {
		if f, ferr := os.CreateTemp("", "tt-codex-last-message-*.txt"); ferr == nil {
			codexLastMessagePath = f.Name()
			_ = f.Close()
			defer os.Remove(codexLastMessagePath)
			extraArgs = append(extraArgs, "--output-last-message", codexLastMessagePath)
		}
	}
	argv := buildExternalAgentArgv(driver, provider, model, req.Mode, req.Resume, extraArgs)
	argv = appendAgentPrompt(argv, driver, req.Prompt)
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, argv[1:]...)
	if !externalAgentPromptInArgv(driver, req.Prompt) {
		cmd.Stdin = strings.NewReader(req.Prompt)
	}
	if cwd := strings.TrimSpace(req.Workspace); cwd != "" {
		if !filepath.IsAbs(cwd) {
			if wd, wderr := os.Getwd(); wderr == nil {
				cwd = filepath.Join(wd, cwd)
			}
		}
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()
	started := time.Now()
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		runErr = fmt.Errorf("external_agent %s timed out after %s", driver, timeout)
	}
	text := extractExternalAgentText(driver, stdout.String())
	if codexLastMessagePath != "" {
		if data, rerr := os.ReadFile(codexLastMessagePath); rerr == nil && strings.TrimSpace(string(data)) != "" {
			text = strings.TrimSpace(string(data))
		}
	}
	out := externalAgentOutput{
		Driver:    driver,
		Provider:  provider,
		Model:     model,
		Mode:      req.Mode,
		SessionID: extractExternalAgentSessionID(driver, stdout.String()),
		Text:      text,
		Stderr:    cleanExternalAgentStderr(driver, stderr.String()),
		ExitCode:  exitCode,
		Duration:  time.Since(started).Milliseconds(),
	}
	data, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		return steps.Value{}, marshalErr
	}
	return steps.Value{Type: "json", Raw: data}, runErr
}

func cleanExternalAgentStderr(driver, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if driver == "forge" {
		return extractForgeText(raw)
	}
	return raw
}

func hasExternalAgentArg(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}

func (c ExternalAgentCapability) lookupDriver(driver string) (string, error) {
	if c.Resolver != nil {
		return c.Resolver(driver)
	}
	lookup := driver
	if driver == "bl" {
		lookup = "jcode"
	}
	path, err := exec.LookPath(lookup)
	if err != nil {
		return "", fmt.Errorf("external_agent driver %q not found on PATH: %w", lookup, err)
	}
	return path, nil
}

func buildExternalAgentArgv(driver, provider, model, mode, resume string, extra []string) []string {
	argv := []string{driver}
	switch driver {
	case "jcode":
		argv = append(argv, "run", "--json")
		if provider != "" {
			argv = append(argv, "--provider", provider)
		}
		if model != "" {
			argv = append(argv, "--model", model)
		}
		if resume != "" {
			argv = append(argv, "--resume", resume)
		}
		argv = append(argv, extra...)
	case "bl":
		argv[0] = "jcode"
		argv = append(argv, "run", "--json", "--provider", "bl")
		if model != "" {
			argv = append(argv, "--model", model)
		}
		if resume != "" {
			argv = append(argv, "--resume", resume)
		}
		argv = append(argv, extra...)
	case "codex":
		argv = append(argv, "exec")
		if model != "" {
			argv = append(argv, "--model", model)
		}
		argv = append(argv, extra...)
		if resume != "" {
			// Options such as --sandbox and --full-auto belong to `codex exec`,
			// not `codex exec resume`. Keep them before the subcommand so a
			// persisted Team member session can actually be resumed.
			argv = append(argv, "resume", resume)
		}
	case "opencode":
		argv = append(argv, "run")
		if model != "" {
			argv = append(argv, "--model", model)
		}
		if resume != "" {
			argv = append(argv, "--session", resume)
		}
		if !hasExternalAgentArg(extra, "--format") {
			argv = append(argv, "--format", "json")
		}
		argv = append(argv, extra...)
	case "forge":
		// forge handles single-shot execution via top-level --prompt. It does not
		// expose a stable --model flag in the CLI help; use the provider/model that
		// forge configured during installation or setup.
		if resume != "" {
			argv = append(argv, "--conversation-id", resume)
		}
		argv = append(argv, extra...)
	}
	return argv
}

func appendAgentPrompt(argv []string, driver, prompt string) []string {
	if strings.TrimSpace(prompt) == "" {
		return argv
	}
	switch driver {
	case "jcode", "codex", "opencode", "bl":
		return append(argv, prompt)
	}
	return argv
}

func externalAgentPromptInArgv(driver, prompt string) bool {
	if strings.TrimSpace(prompt) == "" {
		return false
	}
	switch driver {
	case "jcode", "codex", "opencode", "bl":
		return true
	}
	return false
}

func extractExternalAgentText(driver, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch driver {
	case "jcode", "bl":
		var parsed struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed.Text != "" {
			return parsed.Text
		}
	case "forge":
		return extractForgeText(raw)
	case "opencode":
		if text := extractOpenCodeText(raw); text != "" {
			return text
		}
	}
	return raw
}

func extractOpenCodeText(raw string) string {
	var texts []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		texts = appendJSONText(texts, event)
	}
	return strings.TrimSpace(strings.Join(texts, ""))
}

func appendJSONText(texts []string, value any) []string {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"text", "delta", "content"} {
			if s, ok := v[key].(string); ok && s != "" {
				return append(texts, s)
			}
		}
		for _, key := range []string{"message", "part", "data", "snapshot"} {
			if nested, ok := v[key]; ok {
				texts = appendJSONText(texts, nested)
			}
		}
		if parts, ok := v["parts"]; ok {
			texts = appendJSONText(texts, parts)
		}
	case []any:
		for _, item := range v {
			texts = appendJSONText(texts, item)
		}
	}
	return texts
}

func extractForgeText(raw string) string {
	clean := stripANSI(raw)
	clean = strings.ReplaceAll(clean, "\r", "\n")
	var lines []string
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Ctrl+C to interrupt") || strings.Contains(line, "Synthesizing") || strings.Contains(line, "Migrating credentials") || strings.Contains(line, "Initialize ") || strings.Contains(line, "Finished ") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) {
				c := s[i]
				if c >= 0x40 && c <= 0x7e {
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func extractExternalAgentSessionID(driver, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if driver == "codex" {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "session id:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "session id:"))
			}
		}
		return ""
	}
	if driver == "opencode" {
		return extractOpenCodeSessionID(raw)
	}
	if driver != "jcode" && driver != "bl" {
		return ""
	}
	var parsed struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed.SessionID
	}
	return ""
}

func extractOpenCodeSessionID(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		for _, key := range []string{"session_id", "sessionID"} {
			if s, ok := event[key].(string); ok && s != "" {
				return s
			}
		}
		if typ, _ := event["type"].(string); strings.Contains(typ, "session") {
			if s, ok := event["id"].(string); ok && s != "" {
				return s
			}
		}
		if session, ok := event["session"].(map[string]any); ok {
			if s, ok := session["id"].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// DryRunExternalAgentCapability echoes the request as JSON so formulas can
// be validated without spawning any agent. Used by --dry-run.
type DryRunExternalAgentCapability struct{}

func (DryRunExternalAgentCapability) RunExternalAgent(_ context.Context, req steps.ExternalAgentRequest) (steps.Value, error) {
	data, err := json.Marshal(map[string]any{
		"dry_run":    true,
		"driver":     req.Driver,
		"provider":   req.Provider,
		"model":      req.Model,
		"mode":       req.Mode,
		"resume":     req.Resume,
		"prompt":     req.Prompt,
		"exit_code":  0,
		"text":       "",
		"session_id": "",
		"stderr":     "",
		"duration":   int64(0),
	})
	if err != nil {
		return steps.Value{}, err
	}
	return steps.Value{Type: "json", Raw: data}, nil
}
