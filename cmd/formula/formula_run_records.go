package formulacmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/tt/internal/formula"
	"github.com/sjzsdu/tt/internal/formularun"
)

func runFormulaRuns(cmd *cobra.Command, args []string) error {
	_ = args
	records, err := formularun.List("")
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(records) == 0 {
		fmt.Fprintln(out, "No saved formula runs found.")
		return nil
	}
	records = filterFormulaRunRecords(records)
	if len(records) == 0 {
		fmt.Fprintln(out, "No matching formula runs found.")
		return nil
	}
	limit := formulaRunsLimit
	if limit <= 0 || limit > len(records) {
		limit = len(records)
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFORMULA\tSTATUS\tSTARTED\tFINISHED")
	for _, record := range records[:limit] {
		meta := record.Metadata
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", record.ID, meta.Formula, meta.Status, shortTime(meta.StartedAt), shortTime(meta.FinishedAt))
	}
	return w.Flush()
}

func filterFormulaRunRecords(records []formularun.Record) []formularun.Record {
	formulaFilter := strings.TrimSpace(formulaRunsFormula)
	statusFilter := strings.TrimSpace(formulaRunsStatus)
	if formulaFilter == "" && statusFilter == "" {
		return records
	}
	out := make([]formularun.Record, 0, len(records))
	for _, record := range records {
		if formulaFilter != "" && !strings.EqualFold(record.Metadata.Formula, formulaFilter) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(record.Metadata.Status, statusFilter) {
			continue
		}
		out = append(out, record)
	}
	return out
}

func runFormulaRunOpen(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	workflow, err := formula.CompileWorkflowByName(cmd.Context(), record.Metadata.Formula, getSearchPaths(), record.Metadata.Vars)
	if err != nil {
		return err
	}
	snapshot, err := loadFormulaRunSnapshot(record.Dir, workflow)
	if err != nil {
		return fmt.Errorf("load formula run state failed: %w", err)
	}
	dashboard := newFormulaDashboardServerFromSnapshot(snapshot)
	if err := dashboard.start(formulaWebPort); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Opened formula run: %s\n", record.ID)
	fmt.Fprintf(out, "Web dashboard: http://localhost:%d\n", dashboard.port)
	fmt.Fprintln(out, "Press Ctrl-C to stop the dashboard.")
	waitForFormulaDashboardExit(dashboard)
	return nil
}

func runFormulaRunShow(cmd *cobra.Command, args []string) error {
	id := "latest"
	if len(args) > 0 {
		id = args[0]
	}
	record, err := formularun.Resolve("", id)
	if err != nil {
		return err
	}
	workflow, _ := formula.CompileWorkflowByName(cmd.Context(), record.Metadata.Formula, getSearchPaths(), record.Metadata.Vars)
	snapshot, _ := loadFormulaRunSnapshot(record.Dir, workflow)
	out := cmd.OutOrStdout()
	meta := record.Metadata
	fmt.Fprintf(out, "Run: %s\n", record.ID)
	fmt.Fprintf(out, "Formula: %s\n", meta.Formula)
	fmt.Fprintf(out, "Status: %s\n", meta.Status)
	if meta.Error != "" {
		fmt.Fprintf(out, "Error: %s\n", meta.Error)
	}
	fmt.Fprintf(out, "Started: %s\n", shortTime(meta.StartedAt))
	fmt.Fprintf(out, "Finished: %s\n", shortTime(meta.FinishedAt))
	fmt.Fprintf(out, "Directory: %s\n", record.Dir)
	if meta.PID != 0 {
		fmt.Fprintf(out, "PID: %d\n", meta.PID)
	}
	if meta.TTVersion != "" {
		fmt.Fprintf(out, "tt Version: %s\n", meta.TTVersion)
	}
	if meta.GitBranch != "" || meta.GitCommit != "" {
		dirty := "clean"
		if meta.GitDirty {
			dirty = "dirty"
		}
		fmt.Fprintf(out, "Git: %s %s (%s)\n", meta.GitBranch, meta.GitCommit, dirty)
	}
	fmt.Fprintf(out, "Sessions: %s\n", filepath.Join(meta.WorkspaceDir, ".tt", "sessions"))
	if strings.TrimSpace(formulaRunShowStep) != "" {
		return renderFormulaRunStep(out, record, snapshot, formulaRunShowStep)
	}
	if len(snapshot.Steps) > 0 {
		fmt.Fprintln(out, "\nSteps:")
		for _, step := range snapshot.Steps {
			fmt.Fprintf(out, "  [%s] %s (%s)\n", step.Status, step.ID, step.Title)
			if step.Error != "" {
				fmt.Fprintf(out, "    Error: %s\n", step.Error)
			}
		}
	}
	if snapshot.FinalOutput != "" {
		fmt.Fprintf(out, "\n--- Final Output ---\n\n%s\n", snapshot.FinalOutput)
	}
	return nil
}

func runFormulaRunRm(cmd *cobra.Command, args []string) error {
	if !formulaRunRmYes {
		return fmt.Errorf("refusing to delete formula run %q without --yes", args[0])
	}
	record, err := formularun.Delete("", args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted formula run: %s\n", record.ID)
	return nil
}
