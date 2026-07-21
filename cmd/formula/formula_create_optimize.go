package formulacmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/agents"
	"github.com/sjzsdu/tt/internal/formula"
	spec "github.com/sjzsdu/tt/internal/formula/spec"
	pcwrap "github.com/sjzsdu/tt/internal/picoclaw"
)

func runFormulaCreate(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("formula name is required")
	}
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		return fmt.Errorf("formula prompt is required")
	}

	projectRoot, _ := os.Getwd()
	formulaRT, err := newFormulaPicoclawRuntime(projectRoot)
	if err != nil {
		return err
	}
	defer formulaRT.Close()
	formulaWriter, err := agents.FormulaWriter()
	if err != nil {
		return fmt.Errorf("load formula writer agent failed: %w", err)
	}
	embedded := []pcwrap.EmbeddedAgent{formulaWriter}
	session := "cli:formula:create:" + name
	runner, err := formulaRT.Runtime.NewDirectRunner(pcwrap.RunOptions{Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: formulaRT.Workspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	if err != nil {
		return err
	}
	defer runner.Close()

	message := buildFormulaCreatePrompt(name, prompt)
	loading := startLLMLoading("正在用 formula-writer agent 生成 formula", formulaDebug)
	resp, err := runner.ProcessDirect(pcwrap.RunOptions{Message: message, Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: formulaRT.Workspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	loading.Stop()
	if err != nil {
		return err
	}
	toml := extractFormulaTOML(resp)
	if toml == "" {
		return fmt.Errorf("formula-writer returned empty formula")
	}
	toml = stripFormulaOutputKeyLines(toml)

	if formulaCreateStdout {
		fmt.Fprintln(cmd.OutOrStdout(), toml)
		return nil
	}

	outPath := formulaCreateOutputPath(name)
	if !formulaCreateForce {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("formula file already exists: %s (use --force to overwrite)", outPath)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(toml+"\n"), 0o644); err != nil {
		return err
	}

	p := formula.NewParser()
	f, err := p.ParseFile(outPath)
	if err != nil {
		return fmt.Errorf("generated formula written to %s but failed to parse: %w", outPath, err)
	}
	if err := f.Validate(); err != nil {
		return fmt.Errorf("generated formula written to %s but failed validation: %w", outPath, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created formula: %s\n", outPath)
	fmt.Fprintf(out, "Formula %q is valid.\n", f.Formula)
	fmt.Fprintf(out, "Next: tt formula compile %s --dir %s\n", f.Formula, filepath.Dir(outPath))
	return nil
}

func runFormulaOptimize(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("formula name is required")
	}
	suggestion := strings.TrimSpace(strings.Join(args[1:], " "))
	if suggestion == "" {
		return fmt.Errorf("optimization suggestion is required")
	}

	p := formula.NewParser(getSearchPaths()...)
	f, err := p.LoadByName(name)
	if err != nil {
		return fmt.Errorf("formula %q not found: %w", name, err)
	}
	if strings.TrimSpace(f.Source) == "" {
		return fmt.Errorf("formula %q source path is unknown", name)
	}
	if !formula.IsTOMLFilename(f.Source) && !formulaOptimizeBuiltin && strings.TrimSpace(formulaOptimizeOutput) == "" && !formulaOptimizeStdout {
		return fmt.Errorf("formula %q is not a TOML file (%s); use --output <path.toml> or --stdout", name, f.Source)
	}
	var existing []byte
	if strings.HasPrefix(strings.TrimSpace(f.Source), "builtin:") {
		data, ok, err := formula.BuiltinFormulaContent(name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("builtin formula %q not found", name)
		}
		existing = data
	} else {
		existing, err = os.ReadFile(f.Source)
		if err != nil {
			return fmt.Errorf("read formula %s: %w", f.Source, err)
		}
	}

	projectRoot, _ := os.Getwd()
	formulaRT, err := newFormulaPicoclawRuntime(projectRoot)
	if err != nil {
		return err
	}
	defer formulaRT.Close()
	formulaWriter, err := agents.FormulaWriter()
	if err != nil {
		return fmt.Errorf("load formula writer agent failed: %w", err)
	}
	embedded := []pcwrap.EmbeddedAgent{formulaWriter}
	session := "cli:formula:optimize:" + name
	runner, err := formulaRT.Runtime.NewDirectRunner(pcwrap.RunOptions{Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: formulaRT.Workspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	if err != nil {
		return err
	}
	defer runner.Close()

	message := buildFormulaOptimizePrompt(name, string(existing), suggestion)
	loading := startLLMLoading("正在用 formula-writer agent 优化 formula", formulaDebug)
	resp, err := runner.ProcessDirect(pcwrap.RunOptions{Message: message, Session: session, Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: formulaRT.Workspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
	loading.Stop()
	if err != nil {
		return err
	}
	toml := extractFormulaTOML(resp)
	if toml == "" {
		return fmt.Errorf("formula-writer returned empty optimized formula")
	}
	optimized, err := validateFormulaTOMLContent(toml)
	if err != nil {
		repairMessage := buildFormulaOptimizeRepairPrompt(name, suggestion, toml, err)
		loading := startLLMLoading("生成结果校验失败，正在让 formula-writer 修复 TOML", formulaDebug)
		resp, repairErr := runner.ProcessDirect(pcwrap.RunOptions{Message: repairMessage, Session: session + ":repair", Agent: agents.FormulaWriterID, Model: formulaModel, Workspace: formulaRT.Workspace, Debug: formulaDebug, Quiet: !formulaDebug, EmbeddedAgents: embedded})
		loading.Stop()
		if repairErr != nil {
			return fmt.Errorf("optimized formula failed validation: %w; repair attempt failed: %w", err, repairErr)
		}
		toml = extractFormulaTOML(resp)
		if toml == "" {
			return fmt.Errorf("optimized formula failed validation: %w; repair attempt returned empty formula", err)
		}
		optimized, err = validateFormulaTOMLContent(toml)
		if err != nil {
			return fmt.Errorf("optimized formula failed validation after repair: %w", err)
		}
	}
	if optimized.Formula != f.Formula {
		return fmt.Errorf("optimized formula changed name from %q to %q", f.Formula, optimized.Formula)
	}

	if formulaOptimizeStdout {
		fmt.Fprintln(cmd.OutOrStdout(), toml)
		return nil
	}
	outPath := strings.TrimSpace(formulaOptimizeOutput)
	if outPath == "" {
		if formula.IsTOMLFilename(f.Source) {
			outPath = f.Source
		} else {
			outPath = filepath.Join(formulaDefaultDir(formulaMustLoadTTConfig()), name+formula.CanonicalTOMLExt)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(toml+"\n"), 0o644); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Optimized formula: %s\n", outPath)
	fmt.Fprintf(out, "Formula %q is valid.\n", optimized.Formula)
	fmt.Fprintf(out, "Next: tt formula compile %s --dir %s\n", optimized.Formula, filepath.Dir(outPath))
	return nil
}

func buildFormulaCreatePrompt(name, userPrompt string) string {
	return fmt.Sprintf(`Create a tt formula TOML file.

Formula name: %s
User request:
%s

Requirements:
- Output only valid TOML, preferably no Markdown fences.
- Set formula = %q exactly.
- Use version = 1 and type = "workflow".
- Prefer script steps for deterministic context collection or validation.
- Prefer agent steps for reasoning, planning, implementation, review, and reporting. Use assistant for reasoning/planning/implementation/review steps; use reporter for report/final-summary steps.
- Use safe argv-style script commands; avoid shell.
- Do not add output_key. Step id is the output key by default, and normal authoring should not use output_key.
- Use step ids consistently for data consumed downstream.
- Add depends_on and input_context where data flows between steps.
- If a condition or loop depends on agent output, make that step output ONLY compact JSON.
- Use embedded agent assistant for normal problem-solving agent steps. Use reporter only for report/final-summary steps. Keep specialized agents only when a formula explicitly depends on that specialist behavior.
`, name, userPrompt, name)
}

func buildFormulaOptimizePrompt(name, currentTOML, suggestion string) string {
	return fmt.Sprintf(`Optimize an existing tt formula TOML file.

Formula name: %s
User optimization request:
%s

Current formula TOML:
---BEGIN TOML---
%s
---END TOML---

Requirements:
- Output only the full optimized TOML, preferably no Markdown fences.
- Preserve formula = %q exactly.
- Preserve the user's existing intent unless the suggestion explicitly changes it.
- Improve step boundaries, data flow, step-id context, input_context, conditions, loops, script safety, descriptions, and agent choices where useful.
- Prefer script steps for deterministic context collection or validation.
- Prefer agent steps for reasoning, planning, implementation, review, and reporting. Use assistant for reasoning/planning/implementation/review steps; use reporter for report/final-summary steps.
- Use safe argv-style script commands; avoid shell.
- Do not add output_key unless the user explicitly asks for a legacy alias. Step id is the output key by default.
- For agent config, use exactly one TOML style per step: either agent.name = "assistant" OR [steps.agent] name = "assistant", never both in the same [[steps]].
- Prefer preserving the current file's style. If the current formula uses agent.name = "...", keep using dotted agent.name and do not add [steps.agent] tables.
- Do not remove important variables or steps unless the suggestion asks for simplification.
- Ensure all depends_on references point to existing local step ids.
`, name, suggestion, currentTOML, name)
}

func buildFormulaOptimizeRepairPrompt(name, suggestion, invalidTOML string, validationErr error) string {
	return fmt.Sprintf(`The optimized tt formula TOML failed local validation.

Formula name: %s
Original optimization request:
%s

Validation error:
%v

Invalid TOML to repair:
---BEGIN TOML---
%s
---END TOML---

Return only the full repaired TOML.

Hard requirements:
- Preserve formula = %q exactly.
- Fix the validation error without changing the user's intent.
- Do not mix dotted agent keys and agent tables in the same step. If a step has agent.name = "...", do not also add [steps.agent] for that step.
- Do not add output_key unless the original formula already used it and preserving it is necessary.
- Prefer dotted agent.name = "..." style for consistency with the current formula.
- Ensure all TOML tables are valid and every [[steps]] table is closed before starting the next step.
`, name, suggestion, validationErr, invalidTOML, name)
}

func validateFormulaTOMLContent(content string) (*spec.Formula, error) {
	p := formula.NewParser()
	f, err := p.ParseTOML([]byte(content))
	if err != nil {
		return nil, err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return f, nil
}

func formulaCreateOutputPath(name string) string {
	if strings.TrimSpace(formulaCreateOutput) != "" {
		return formulaCreateOutput
	}
	dir := strings.TrimSpace(formulaDir)
	if dir == "" {
		dir = formulaDefaultDir(formulaMustLoadTTConfig())
	}
	return filepath.Join(dir, name+".toml")
}

func extractFormulaTOML(resp string) string {
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return ""
	}
	for _, fence := range []string{"```toml", "```TOML", "```"} {
		idx := strings.Index(resp, fence)
		if idx < 0 {
			continue
		}
		start := idx + len(fence)
		if nl := strings.Index(resp[start:], "\n"); nl >= 0 {
			start += nl + 1
		}
		end := strings.Index(resp[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(resp[start : start+end])
		}
	}
	return strings.TrimSpace(resp)
}

func stripFormulaOutputKeyLines(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "output_key") {
			if before, after, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(before) == "output_key" && strings.TrimSpace(after) != "" {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
