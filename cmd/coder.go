package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	coderruntime "github.com/sjzsdu/tt/internal/coder"
)

var (
	coderRoot       string
	coderProject    string
	coderName       string
	coderID         string
	coderDecision   string
	coderReviewer   string
	coderComment    string
	coderSetValues  []string
	coderOutputJSON bool
)

var coderCmd = &cobra.Command{
	Use:   "coder",
	Short: "Run human-led product development workflows",
	Long: `Run human-led product development workflows backed by persistent coder
projects, context packets, review gates, dynamic forms, decisions, tasks, and artifacts.`,
}

var coderStartCmd = &cobra.Command{
	Use:   "start <idea>",
	Short: "Create a coder project and initial product intent review gate",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCoderStart,
}

var coderStatusCmd = &cobra.Command{
	Use:   "status [project]",
	Short: "Show current coder project status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCoderStatus,
}

var coderShowCmd = &cobra.Command{
	Use:   "show [project]",
	Short: "Show traceable coder project details",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCoderShow,
}

var coderApproveCmd = &cobra.Command{
	Use:   "approve <gate>",
	Short: "Submit human review feedback for a coder review gate",
	Args:  cobra.ExactArgs(1),
	RunE:  runCoderApprove,
}

func init() {
	rootCmd.AddCommand(coderCmd)
	coderCmd.PersistentFlags().StringVar(&coderRoot, "root", "", "coder project root (default: <project>/.tt/coder/projects)")
	coderCmd.PersistentFlags().StringVarP(&coderProject, "project", "p", "", "coder project id; defaults to the latest project where supported")
	coderCmd.PersistentFlags().BoolVar(&coderOutputJSON, "json", false, "output machine-readable JSON")

	coderStartCmd.Flags().StringVar(&coderName, "name", "", "project display name")
	coderStartCmd.Flags().StringVar(&coderID, "id", "", "project id; defaults to a slug from --name or idea")

	coderApproveCmd.Flags().StringVar(&coderDecision, "decision", coderruntime.ReviewDecisionApprove, "review decision: approve, approve_with_changes, request_revision, or reject")
	coderApproveCmd.Flags().StringVar(&coderReviewer, "reviewer", "human", "reviewer id")
	coderApproveCmd.Flags().StringVar(&coderComment, "comment", "", "freeform review comment")
	coderApproveCmd.Flags().StringArrayVar(&coderSetValues, "set", nil, "form answer as key=value; repeatable")

	coderCmd.AddCommand(coderStartCmd, coderStatusCmd, coderShowCmd, coderApproveCmd)
}

func runCoderStart(cmd *cobra.Command, args []string) error {
	idea := strings.TrimSpace(strings.Join(args, " "))
	name := strings.TrimSpace(coderName)
	if name == "" {
		name = firstSentence(idea)
	}
	project := coderruntime.NewProject(coderID, name, idea)
	project.OwnerIntent = idea
	project.Status = coderruntime.ProjectStatusPlanning

	root, err := resolveCoderRoot()
	if err != nil {
		return err
	}
	store, err := coderruntime.CreateStore(root, project)
	if err != nil {
		return err
	}

	gateID := "product-intent"
	packet, err := store.SaveContextPacket(coderruntime.ContextPacket{
		Product: coderruntime.ProductContext{
			Vision:         idea,
			CurrentStage:   "product_intent_review",
			HumanDirection: "请先确认产品方向、目标用户、MVP 范围和非目标。",
		},
		Phase: coderruntime.PhaseContext{
			Name:      "product_intent",
			Objective: "确认产品意图和 MVP 方向",
			SuccessCriteria: []string{
				"确认产品要解决的问题",
				"确认目标用户",
				"确认 MVP 功能和非目标",
			},
		},
		ReviewGates: coderruntime.ReviewGateLinks{Current: gateID},
	})
	if err != nil {
		return err
	}

	gate := coderruntime.ReviewGate{
		ID:             gateID,
		ProjectID:      store.Project.ID,
		Type:           coderruntime.GateTypeProductIntent,
		Status:         coderruntime.GateStatusWaitingHuman,
		Title:          "确认产品意图",
		Summary:        "确认产品目标、目标用户、MVP 范围和非目标。",
		FormSpecID:     gateID + "-form",
		CreatedByAgent: "coder",
	}
	if err := store.SaveReviewGate(gate); err != nil {
		return err
	}
	form := initialProductIntentForm(gateID)
	if err := store.SaveDynamicFormSpec(form); err != nil {
		return err
	}
	project = store.Project
	project.CurrentGate = gateID
	project.CurrentContext = packet.Version
	project.UpdatedAt = packet.CreatedAt
	if err := store.SaveProject(project); err != nil {
		return err
	}

	if coderOutputJSON {
		return writeCoderJSON(cmd, map[string]any{"project": project, "context": packet, "gate": gate, "form": form, "root": root})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created coder project %s\n", project.ID)
	fmt.Fprintf(out, "Root: %s\n", root)
	fmt.Fprintf(out, "Current phase: %s\n", packet.Phase.Name)
	fmt.Fprintf(out, "Waiting review gate: %s (%s)\n", gate.ID, gate.Title)
	fmt.Fprintf(out, "\nTry:\n")
	fmt.Fprintf(out, "  tt coder status --project %s\n", project.ID)
	fmt.Fprintf(out, "  tt coder show --project %s\n", project.ID)
	fmt.Fprintf(out, "  tt coder approve %s --project %s --decision approve_with_changes --set target_users=\"small teams\" --comment \"先做 MVP\"\n", gate.ID, project.ID)
	return nil
}

func runCoderStatus(cmd *cobra.Command, args []string) error {
	store, err := openCoderStoreFromArgs(args)
	if err != nil {
		return err
	}
	packet, _ := store.LoadContextPacket(0)
	gate := coderruntime.ReviewGate{}
	if store.Project.CurrentGate != "" {
		gate, _ = store.LoadReviewGate(store.Project.CurrentGate)
	}
	if coderOutputJSON {
		return writeCoderJSON(cmd, map[string]any{"project": store.Project, "context": packet, "current_gate": gate})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Project: %s (%s)\n", store.Project.ID, store.Project.Name)
	fmt.Fprintf(out, "Status: %s\n", store.Project.Status)
	fmt.Fprintf(out, "Vision: %s\n", store.Project.Vision)
	if packet.Version > 0 {
		fmt.Fprintf(out, "Context: v%d phase=%s objective=%s\n", packet.Version, packet.Phase.Name, packet.Phase.Objective)
	}
	if gate.ID != "" {
		fmt.Fprintf(out, "Waiting gate: %s [%s] %s\n", gate.ID, gate.Status, gate.Title)
	} else {
		fmt.Fprintln(out, "Waiting gate: none")
	}
	return nil
}

func runCoderShow(cmd *cobra.Command, args []string) error {
	store, err := openCoderStoreFromArgs(args)
	if err != nil {
		return err
	}
	packet, _ := store.LoadContextPacket(0)
	var gate coderruntime.ReviewGate
	var form coderruntime.DynamicFormSpec
	var response coderruntime.HumanReviewResponse
	if store.Project.CurrentGate != "" {
		gate, _ = store.LoadReviewGate(store.Project.CurrentGate)
		form, _ = store.LoadDynamicFormSpec(store.Project.CurrentGate)
		response, _ = store.LoadHumanReviewResponse(store.Project.CurrentGate)
	}
	decisions, _ := store.Decisions()
	tasks, _ := store.Tasks()
	artifacts, _ := store.Artifacts()
	payload := map[string]any{"project": store.Project, "context": packet, "current_gate": gate, "form": form, "response": response, "decisions": decisions, "tasks": tasks, "artifacts": artifacts, "dir": store.Dir}
	if coderOutputJSON {
		return writeCoderJSON(cmd, payload)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Project: %s (%s)\n", store.Project.ID, store.Project.Name)
	fmt.Fprintf(out, "Directory: %s\n", store.Dir)
	fmt.Fprintf(out, "Status: %s\n", store.Project.Status)
	fmt.Fprintf(out, "Vision: %s\n", store.Project.Vision)
	if packet.Version > 0 {
		fmt.Fprintf(out, "\nContext v%d\n", packet.Version)
		fmt.Fprintf(out, "  Stage: %s\n", packet.Product.CurrentStage)
		fmt.Fprintf(out, "  Human direction: %s\n", packet.Product.HumanDirection)
		fmt.Fprintf(out, "  Phase: %s - %s\n", packet.Phase.Name, packet.Phase.Objective)
	}
	if gate.ID != "" {
		fmt.Fprintf(out, "\nCurrent Review Gate\n")
		fmt.Fprintf(out, "  %s [%s] %s\n", gate.ID, gate.Status, gate.Title)
		if len(form.Fields) > 0 {
			fmt.Fprintf(out, "  Form fields:\n")
			for _, field := range form.Fields {
				fmt.Fprintf(out, "    - %s (%s): %s\n", field.ID, field.Type, field.Label)
			}
		}
	}
	fmt.Fprintf(out, "\nTrace\n")
	fmt.Fprintf(out, "  Decisions: %d\n", len(decisions))
	fmt.Fprintf(out, "  Tasks: %d\n", len(tasks))
	fmt.Fprintf(out, "  Artifacts: %d\n", len(artifacts))
	return nil
}

func runCoderApprove(cmd *cobra.Command, args []string) error {
	store, err := openCoderStoreFromArgs(nil)
	if err != nil {
		return err
	}
	gateID := strings.TrimSpace(args[0])
	gate, err := store.LoadReviewGate(gateID)
	if err != nil {
		return err
	}
	answers, err := parseCoderSetValues(coderSetValues)
	if err != nil {
		return err
	}
	responseID := gate.ID + "-response"
	response := coderruntime.HumanReviewResponse{
		ID:              responseID,
		GateID:          gate.ID,
		ProjectID:       store.Project.ID,
		Decision:        coderDecision,
		Answers:         answers,
		FreeformComment: coderComment,
		Reviewer:        coderReviewer,
	}
	if err := store.SaveHumanReviewResponse(response); err != nil {
		return err
	}
	response, _ = store.LoadHumanReviewResponse(gate.ID)

	resolved := coderDecision == coderruntime.ReviewDecisionApprove || coderDecision == coderruntime.ReviewDecisionApproveWithChanges
	if resolved {
		if coderDecision == coderruntime.ReviewDecisionApproveWithChanges {
			gate.Status = coderruntime.GateStatusApprovedWithChanges
		} else {
			gate.Status = coderruntime.GateStatusApproved
		}
		gate.ApprovedBy = response.Reviewer
		gate.ResponseID = response.ID
		gate.ResolvedAt = response.CreatedAt
	} else if coderDecision == coderruntime.ReviewDecisionReject {
		gate.Status = coderruntime.GateStatusRejected
		gate.ResponseID = response.ID
		gate.ResolvedAt = response.CreatedAt
	} else {
		gate.Status = coderruntime.GateStatusPending
		gate.ResponseID = response.ID
	}
	if err := store.SaveReviewGate(gate); err != nil {
		return err
	}
	if err := store.AppendDecision(coderruntime.Decision{ID: gate.ID + "-" + coderDecision, ProjectID: store.Project.ID, Source: "human_review", Content: fmt.Sprintf("%s: %s", gate.Title, coderDecision), Reason: coderComment}); err != nil {
		return err
	}
	project := store.Project
	if resolved {
		project.CurrentGate = ""
		project.Status = coderruntime.ProjectStatusPlanning
	}
	if err := store.SaveProject(project); err != nil {
		return err
	}
	latest, _ := store.LoadContextPacket(0)
	latest.ID = ""
	latest.Version = 0
	latest.ReviewGates.Current = project.CurrentGate
	latest.ReviewGates.Completed = appendIfMissing(latest.ReviewGates.Completed, gate.ID)
	latest.Product.HumanDirection = strings.TrimSpace(strings.Join([]string{latest.Product.HumanDirection, coderComment}, "\n"))
	_, _ = store.SaveContextPacket(latest)

	if coderOutputJSON {
		return writeCoderJSON(cmd, map[string]any{"project": project, "gate": gate, "response": response})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Submitted review for gate %s: %s\n", gate.ID, coderDecision)
	fmt.Fprintf(out, "Project: %s\n", project.ID)
	if project.CurrentGate == "" {
		fmt.Fprintln(out, "Current gate: none")
	} else {
		fmt.Fprintf(out, "Current gate: %s\n", project.CurrentGate)
	}
	return nil
}

func resolveCoderRoot() (string, error) {
	if strings.TrimSpace(coderRoot) != "" {
		return filepath.Abs(coderRoot)
	}
	loaded, err := loadTTConfig()
	if err != nil {
		return "", err
	}
	return coderruntime.DefaultRoot(projectRootFromConfig(loaded)), nil
}

func openCoderStoreFromArgs(args []string) (*coderruntime.Store, error) {
	root, err := resolveCoderRoot()
	if err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(coderProject)
	if projectID == "" && len(args) > 0 {
		projectID = strings.TrimSpace(args[0])
	}
	if projectID == "" {
		records, err := coderruntime.ListProjects(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("no coder projects found; run `tt coder start <idea>` first")
			}
			return nil, err
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("no coder projects found; run `tt coder start <idea>` first")
		}
		projectID = records[0].ID
	}
	return coderruntime.OpenStore(root, projectID)
}

func initialProductIntentForm(gateID string) coderruntime.DynamicFormSpec {
	return coderruntime.DynamicFormSpec{
		ID:          gateID + "-form",
		GateID:      gateID,
		Title:       "确认产品意图",
		Description: "请确认产品目标、目标用户、MVP 范围和明确不做的事情。",
		Fields: []coderruntime.FormField{
			{ID: "product_name", Label: "产品名", Type: "text", Required: true},
			{ID: "target_users", Label: "目标用户", Type: "textarea", Required: true},
			{ID: "core_problem", Label: "核心问题", Type: "textarea", Required: true},
			{ID: "mvp_features", Label: "MVP 功能", Type: "textarea", Required: true},
			{ID: "non_goals", Label: "非目标", Type: "textarea"},
			{ID: "priority", Label: "优先级", Type: "select", Options: []string{"最快可用", "体验优先", "技术稳健", "商业验证"}},
		},
		SubmitActions: []string{coderruntime.ReviewDecisionApprove, coderruntime.ReviewDecisionApproveWithChanges, coderruntime.ReviewDecisionRequestRevision, coderruntime.ReviewDecisionReject},
	}
}

func parseCoderSetValues(values []string) (map[string]any, error) {
	answers := map[string]any{}
	for _, value := range values {
		key, raw, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --set %q, expected key=value", value)
		}
		answers[strings.TrimSpace(key)] = strings.TrimSpace(raw)
	}
	return answers, nil
}

func firstSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "coder project"
	}
	for _, sep := range []string{"。", ".", "\n"} {
		if idx := strings.Index(value, sep); idx > 0 {
			value = value[:idx]
			break
		}
	}
	fields := strings.Fields(value)
	if len(fields) > 8 {
		fields = fields[:8]
	}
	return strings.Join(fields, " ")
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func writeCoderJSON(cmd *cobra.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
