package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	formularuntime "github.com/sjzsdu/tt/internal/formula/runtime"
	"github.com/sjzsdu/tt/internal/formula/steps"
)

const verificationPrefix = "[TEAM_VERIFICATION]"

type VerificationRequest struct {
	Commands [][]string `json:"commands"`
}

type VerificationResult struct {
	Command    []string `json:"command"`
	ExitCode   int      `json:"exit_code"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	DurationMS int64    `json:"duration_ms,omitempty"`
}

func verificationInstructions() string {
	return `你是本 Team 配置指定的独立验证者。检查实际工作树后，必须在正文末尾输出一行:
[TEAM_VERIFICATION] {"commands":[["命令","参数"],["另一命令","参数"]]}

只列出足以证明本轮实现的非交互验证命令。不要使用 shell 拼接、重定向或破坏性命令。
运行时会在工作区独立重跑；任一真实退出码非零都会阻止交付并退回实现者。`
}

func parseVerificationRequest(content string) (string, *VerificationRequest) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	clean := make([]string, 0, len(lines))
	var request *VerificationRequest
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, verificationPrefix) {
			clean = append(clean, line)
			continue
		}
		var candidate VerificationRequest
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, verificationPrefix))
		if payload == "" || json.Unmarshal([]byte(payload), &candidate) != nil {
			clean = append(clean, line)
			continue
		}
		request = &candidate
	}
	return strings.TrimSpace(strings.Join(clean, "\n")), request
}

func (e *Engine) runVerification(ctx context.Context, member Agent, request *VerificationRequest) ([]VerificationResult, error) {
	if !e.Definition.Verification.Enabled || !strings.EqualFold(member.ID, e.Definition.Verification.Verifier) {
		return nil, nil
	}
	if request == nil || len(request.Commands) == 0 {
		return nil, fmt.Errorf("verifier %s did not provide verification commands", member.ID)
	}
	if len(request.Commands) > e.Definition.Verification.MaxCommands {
		return nil, fmt.Errorf("verifier requested %d commands; maximum is %d", len(request.Commands), e.Definition.Verification.MaxCommands)
	}
	timeout, _ := time.ParseDuration(e.Definition.Verification.Timeout)
	capability := formularuntime.ScriptCapability{DenyUnsafe: true, DefaultTimeout: timeout}
	results := make([]VerificationResult, 0, len(request.Commands))
	for _, command := range request.Commands {
		value, runErr := capability.RunScript(ctx, steps.ScriptRequest{
			Command: command, Cwd: e.Store.Thread.Workspace, Timeout: timeout,
		})
		var result VerificationResult
		if err := json.Unmarshal(value.Raw, &result); err != nil {
			return results, fmt.Errorf("decode verification result: %w", err)
		}
		result.Stdout = compactVerificationOutput(result.Stdout)
		result.Stderr = compactVerificationOutput(result.Stderr)
		results = append(results, result)
		if runErr != nil || result.ExitCode != 0 {
			return results, fmt.Errorf("verification command %q failed with exit code %d", strings.Join(command, " "), result.ExitCode)
		}
	}
	return results, nil
}

func compactVerificationOutput(value string) string {
	const maxRunes = 4000
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[len(runes)-maxRunes:])
}

func verificationSatisfied(events []Event, round int) bool {
	var latestImplementation, latestVerification int64
	latestPassed := false
	for _, event := range events {
		if event.Round != round {
			continue
		}
		if event.Type == "agent_message" && strings.EqualFold(event.From, "implementer") && event.ID > latestImplementation {
			latestImplementation = event.ID
		}
		if (event.Type == "verification_passed" || event.Type == "verification_failed") && event.ID > latestVerification {
			latestVerification = event.ID
			latestPassed = event.Type == "verification_passed"
		}
	}
	return latestPassed && latestVerification > latestImplementation
}
