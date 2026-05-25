package formulaui

import (
	"strings"

	"github.com/sjzsdu/tt/internal/executor"
	"github.com/sjzsdu/tt/internal/formula"
)

type Snapshot struct {
	RecipeName   string     `json:"recipe_name"`
	Description  string     `json:"description,omitempty"`
	Phase        string     `json:"phase,omitempty"`
	Status       string     `json:"status"`
	FinalOutput  string     `json:"final_output,omitempty"`
	Error        string     `json:"error,omitempty"`
	Steps        []Step     `json:"steps"`
	Edges        []Edge     `json:"edges,omitempty"`
	Logs         []LogEntry `json:"logs,omitempty"`
	WorkspaceDir string     `json:"workspace_dir,omitempty"`
	RunID        string     `json:"run_id,omitempty"`
}

type Step struct {
	ID                string                      `json:"id"`
	Title             string                      `json:"title"`
	Description       string                      `json:"description,omitempty"`
	Notes             string                      `json:"notes,omitempty"`
	Type              string                      `json:"type,omitempty"`
	Agent             string                      `json:"agent"`
	Model             string                      `json:"model,omitempty"`
	Session           string                      `json:"session,omitempty"`
	Status            string                      `json:"status"`
	Output            string                      `json:"output,omitempty"`
	Error             string                      `json:"error,omitempty"`
	StartedAt         string                      `json:"-"`
	FinishedAt        string                      `json:"-"`
	DurationMS        int64                       `json:"duration_ms,omitempty"`
	Priority          *int                        `json:"priority,omitempty"`
	Labels            []string                    `json:"labels,omitempty"`
	Assignee          string                      `json:"assignee,omitempty"`
	OutputKey         string                      `json:"output_key,omitempty"`
	InputCtx          []string                    `json:"input_ctx,omitempty"`
	Execution         string                      `json:"execution,omitempty"`
	Condition         string                      `json:"condition,omitempty"`
	Metadata          map[string]string           `json:"metadata,omitempty"`
	Gate              *Gate                       `json:"gate,omitempty"`
	Loop              *Loop                       `json:"loop,omitempty"`
	DependsOn         []string                    `json:"depends_on,omitempty"`
	Activities        []StepActivity              `json:"activities,omitempty"`
	HumanInputRequest *executor.HumanInputRequest `json:"human_input_request,omitempty"`
	Depth             int                         `json:"depth,omitempty"`
	Index             int                         `json:"index"`
}

type StepActivity struct {
	At         string `json:"at"`
	StepID     string `json:"step_id"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

type Loop struct {
	Count          int        `json:"count,omitempty"`
	Until          string     `json:"until,omitempty"`
	Max            int        `json:"max,omitempty"`
	Range          string     `json:"range,omitempty"`
	ForEach        string     `json:"for_each,omitempty"`
	Var            string     `json:"var,omitempty"`
	Parallel       bool       `json:"parallel,omitempty"`
	MaxConcurrency int        `json:"max_concurrency,omitempty"`
	Summary        string     `json:"summary,omitempty"`
	Body           []LoopBody `json:"body,omitempty"`
}

type LoopBody struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	OutputKey   string   `json:"output_key,omitempty"`
	InputCtx    []string `json:"input_ctx,omitempty"`
	Condition   string   `json:"condition,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type Gate struct {
	Type    string `json:"type,omitempty"`
	ID      string `json:"id,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type,omitempty"`
}

type LogEntry struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type Message struct {
	Type  string   `json:"type"`
	State Snapshot `json:"state"`
}

func LoopParentStepID(stepID string) string {
	idx := strings.Index(stepID, ".iter")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

func AppendStepActivity(step *Step, activity StepActivity) {
	if step == nil || activity.StepID == "" {
		return
	}
	for i := len(step.Activities) - 1; i >= 0; i-- {
		if step.Activities[i].StepID != activity.StepID {
			continue
		}
		if activity.Title == "" {
			activity.Title = step.Activities[i].Title
		}
		step.Activities[i] = activity
		return
	}
	step.Activities = append(step.Activities, activity)
	if len(step.Activities) > 80 {
		step.Activities = append([]StepActivity(nil), step.Activities[len(step.Activities)-80:]...)
	}
}

func CloneSnapshot(s Snapshot) Snapshot {
	cp := s
	cp.Steps = make([]Step, len(s.Steps))
	for i, step := range s.Steps {
		cp.Steps[i] = step
		cp.Steps[i].Labels = append([]string(nil), step.Labels...)
		cp.Steps[i].InputCtx = append([]string(nil), step.InputCtx...)
		cp.Steps[i].DependsOn = append([]string(nil), step.DependsOn...)
		cp.Steps[i].Activities = append([]StepActivity(nil), step.Activities...)
		cp.Steps[i].Loop = CloneLoop(step.Loop)
		cp.Steps[i].Metadata = CloneStringMap(step.Metadata)
		cp.Steps[i].HumanInputRequest = CloneHumanInputRequest(step.HumanInputRequest)
		if step.Gate != nil {
			gate := *step.Gate
			cp.Steps[i].Gate = &gate
		}
	}
	cp.Edges = append([]Edge(nil), s.Edges...)
	cp.Logs = append([]LogEntry(nil), s.Logs...)
	return cp
}

func CloneHumanInputRequest(src *executor.HumanInputRequest) *executor.HumanInputRequest {
	if src == nil {
		return nil
	}
	cp := *src
	if src.Form != nil {
		form := *src.Form
		form.Fields = make([]*formula.FormField, len(src.Form.Fields))
		for i, field := range src.Form.Fields {
			if field == nil {
				continue
			}
			fieldCopy := *field
			fieldCopy.Options = append([]string(nil), field.Options...)
			form.Fields[i] = &fieldCopy
		}
		cp.Form = &form
	}
	return &cp
}
