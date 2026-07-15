package spec

// Formula is the root structure for formula definition files.
type Formula struct {
	Formula     string                `json:"formula" toml:"formula"`
	Description string                `json:"description,omitempty" toml:"description,omitempty"`
	Title       string                `json:"title,omitempty" toml:"title,omitempty"`
	Aliases     []string              `json:"aliases,omitempty" toml:"aliases,omitempty"`
	Category    string                `json:"category,omitempty" toml:"category,omitempty"`
	Tags        []string              `json:"tags,omitempty" toml:"tags,omitempty"`
	Version     int                   `json:"version" toml:"version"`
	Contract    string                `json:"contract,omitempty" toml:"contract,omitempty"`
	Type        Type                  `json:"type" toml:"type"`
	Extends     []string              `json:"extends,omitempty" toml:"extends,omitempty"`
	Vars        map[string]*VarDef    `json:"vars,omitempty" toml:"vars,omitempty"`
	Outputs     map[string]*OutputDef `json:"outputs,omitempty" toml:"outputs,omitempty"`
	Steps       []*Step               `json:"steps,omitempty" toml:"steps,omitempty"`
	Template    []*Step               `json:"template,omitempty" toml:"template,omitempty"`
	Compose     *ComposeRules         `json:"compose,omitempty" toml:"compose,omitempty"`
	Advice      []*AdviceRule         `json:"advice,omitempty" toml:"advice,omitempty"`
	Pointcuts   []*Pointcut           `json:"pointcuts,omitempty" toml:"pointcuts,omitempty"`
	Phase       string                `json:"phase,omitempty" toml:"phase,omitempty"`
	Pour        bool                  `json:"pour,omitempty" toml:"pour,omitempty"`
	Worktree    bool                  `json:"worktree,omitempty" toml:"worktree,omitempty"`
	Workspace   *WorkspaceSpec        `json:"workspace,omitempty" toml:"workspace,omitempty"`
	Preflight   *PreflightSpec        `json:"preflight,omitempty" toml:"preflight,omitempty"`
	Source      string                `json:"-" toml:"-"`
}

// OutputDef declares one public Formula output. From is a runtime context
// path, usually a step ID or an explicit step output_key. Vars are the Formula
// input contract; Outputs form the corresponding public result contract.
type OutputDef struct {
	From        string `json:"from" toml:"from"`
	Type        string `json:"type,omitempty" toml:"type,omitempty"`
	Required    bool   `json:"required,omitempty" toml:"required,omitempty"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
}

// PreflightSpec contains configurable checks to run before starting a formula.
type PreflightSpec struct {
	Checks []*PreflightCheck `json:"checks,omitempty" toml:"checks,omitempty"`
}

// PreflightCheck defines one pre-run environment check.
type PreflightCheck struct {
	Type          string   `json:"type" toml:"type"`
	Name          string   `json:"name,omitempty" toml:"name,omitempty"`
	Command       string   `json:"command,omitempty" toml:"command,omitempty"`
	Args          []string `json:"args,omitempty" toml:"args,omitempty"`
	Env           string   `json:"env,omitempty" toml:"env,omitempty"`
	Path          string   `json:"path,omitempty" toml:"path,omitempty"`
	Message       string   `json:"message,omitempty" toml:"message,omitempty"`
	Condition     string   `json:"condition,omitempty" toml:"condition,omitempty"`
	RequireRepo   bool     `json:"require_repo,omitempty" toml:"require_repo,omitempty"`
	RequireRemote bool     `json:"require_remote,omitempty" toml:"require_remote,omitempty"`
}

// VarDef defines a template variable with optional validation.
type VarDef struct {
	Description string   `json:"description,omitempty" toml:"description,omitempty"`
	Default     *string  `json:"default,omitempty" toml:"default,omitempty"`
	Required    bool     `json:"required,omitempty" toml:"required,omitempty"`
	Enum        []string `json:"enum,omitempty" toml:"enum,omitempty"`
	Pattern     string   `json:"pattern,omitempty" toml:"pattern,omitempty"`
	Type        string   `json:"type,omitempty" toml:"type,omitempty"`
}

// Step defines a workflow node in a formula document.
type Step struct {
	ID              string               `json:"id" toml:"id"`
	Title           string               `json:"title" toml:"title"`
	Description     string               `json:"description,omitempty" toml:"description,omitempty"`
	DescriptionFile string               `json:"description_file,omitempty" toml:"description_file,omitempty"`
	Notes           string               `json:"notes,omitempty" toml:"notes,omitempty"`
	Type            string               `json:"type,omitempty" toml:"type,omitempty"`
	Priority        *int                 `json:"priority,omitempty" toml:"priority,omitempty"`
	Labels          []string             `json:"labels,omitempty" toml:"tags,omitempty"`
	Metadata        map[string]string    `json:"metadata,omitempty" toml:"metadata,omitempty"`
	DependsOn       []string             `json:"depends_on,omitempty" toml:"depends_on,omitempty"`
	Needs           []string             `json:"needs,omitempty" toml:"needs,omitempty"`
	WaitsFor        string               `json:"waits_for,omitempty" toml:"waits_for,omitempty"`
	Assignee        string               `json:"assignee,omitempty" toml:"assignee,omitempty"`
	Expand          string               `json:"expand,omitempty" toml:"expand,omitempty"`
	ExpandVars      map[string]string    `json:"expand_vars,omitempty" toml:"expand_vars,omitempty"`
	Embed           string               `json:"embed,omitempty" toml:"embed,omitempty"`
	EmbedVars       map[string]string    `json:"embed_vars,omitempty" toml:"embed_vars,omitempty"`
	Formula         string               `json:"formula,omitempty" toml:"formula,omitempty"`
	With            map[string]string    `json:"with,omitempty" toml:"with,omitempty"`
	Condition       string               `json:"condition,omitempty" toml:"condition,omitempty"`
	Idempotent      bool                 `json:"idempotent,omitempty" toml:"idempotent,omitempty"`
	Children        []*Step              `json:"children,omitempty" toml:"children,omitempty"`
	Gate            *Gate                `json:"gate,omitempty" toml:"gate,omitempty"`
	Loop            *LoopSpec            `json:"loop,omitempty" toml:"loop,omitempty"`
	OnComplete      *OnCompleteSpec      `json:"on_complete,omitempty" toml:"on_complete,omitempty"`
	Retry           *RetrySpec           `json:"retry,omitempty" toml:"retry,omitempty"`
	Timeout         string               `json:"timeout,omitempty" toml:"timeout,omitempty"`
	Agent           *AgentConfig         `json:"agent,omitempty" toml:"agent,omitempty"`
	Script          *ScriptSpec          `json:"script,omitempty" toml:"script,omitempty"`
	ExternalAgent   *ExternalAgentConfig `json:"external_agent,omitempty" toml:"external_agent,omitempty"`
	Aggregate       *AggregateSpec       `json:"aggregate,omitempty" toml:"aggregate,omitempty"`
	Tool            *ToolSpec            `json:"tool,omitempty" toml:"tool,omitempty"`
	WriteFiles      *WriteFilesSpec      `json:"write_files,omitempty" toml:"write_files,omitempty"`
	Form            *FormSpec            `json:"form,omitempty" toml:"form,omitempty"`
	DynamicForm     bool                 `json:"dynamic_form,omitempty" toml:"dynamic_form,omitempty"`
	Validate        *ValidateSpec        `json:"validate,omitempty" toml:"validate,omitempty"`
	OutputKey       string               `json:"output_key,omitempty" toml:"output_key,omitempty"`
	InputCtx        []string             `json:"input_context,omitempty" toml:"input_context,omitempty"`
	Execution       string               `json:"execution,omitempty" toml:"execution,omitempty"`
	SourceFormula   string               `json:"-" toml:"-"`
	SourceLocation  string               `json:"-" toml:"-"`
}

// Gate defines an async wait condition for formula steps.
type Gate struct {
	Type    string `json:"type" toml:"type"`
	ID      string `json:"id,omitempty" toml:"id,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

// AgentConfig specifies which agent executes a step and how.
type AgentConfig struct {
	Name    string `json:"name" toml:"name"`
	Model   string `json:"model,omitempty" toml:"model,omitempty"`
	Cwd     string `json:"cwd,omitempty" toml:"cwd,omitempty"`
	Session string `json:"session,omitempty" toml:"session,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
	Retries int    `json:"retries,omitempty" toml:"retries,omitempty"`
}

// ExternalAgentConfig configures an execution="external_agent" step.
type ExternalAgentConfig struct {
	Driver    string   `json:"driver,omitempty" toml:"driver,omitempty"`
	Provider  string   `json:"provider,omitempty" toml:"provider,omitempty"`
	Model     string   `json:"model,omitempty" toml:"model,omitempty"`
	Mode      string   `json:"mode,omitempty" toml:"mode,omitempty"`
	Resume    string   `json:"resume,omitempty" toml:"resume,omitempty"`
	Cwd       string   `json:"cwd,omitempty" toml:"cwd,omitempty"`
	Timeout   string   `json:"timeout,omitempty" toml:"timeout,omitempty"`
	ExtraArgs []string `json:"extra_args,omitempty" toml:"extra_args,omitempty"`
}

// ScriptSpec describes a deterministic local command step.
type ScriptSpec struct {
	Command         []string          `json:"command,omitempty" toml:"command,omitempty"`
	Script          string            `json:"script,omitempty" toml:"script,omitempty"`
	Shell           string            `json:"shell,omitempty" toml:"shell,omitempty"`
	Cwd             string            `json:"cwd,omitempty" toml:"cwd,omitempty"`
	Env             map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	Format          string            `json:"format,omitempty" toml:"format,omitempty"`
	Timeout         string            `json:"timeout,omitempty" toml:"timeout,omitempty"`
	ContinueOnError bool              `json:"continue_on_error,omitempty" toml:"continue_on_error,omitempty"`
}

// AggregateSpec describes a deterministic projection/collection over JSON context.
type AggregateSpec struct {
	Source  string   `json:"source" toml:"source"`
	As      string   `json:"as,omitempty" toml:"as,omitempty"`
	Require []string `json:"require,omitempty" toml:"require,omitempty"`
	Include []string `json:"include,omitempty" toml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" toml:"exclude,omitempty"`
	Flatten bool     `json:"flatten,omitempty" toml:"flatten,omitempty"`
}

// ToolSpec selects and configures a deterministic built-in local tool.
type ToolSpec struct {
	Name        string           `json:"name" toml:"name"`
	WriteFiles  *WriteFilesSpec  `json:"write_files,omitempty" toml:"write_files,omitempty"`
	Sleep       *SleepSpec       `json:"sleep,omitempty" toml:"sleep,omitempty"`
	GitFetch    *GitFetchSpec    `json:"git_fetch,omitempty" toml:"git_fetch,omitempty"`
	GitPush     *GitPushSpec     `json:"git_push,omitempty" toml:"git_push,omitempty"`
	GitBranch   *GitBranchSpec   `json:"git_branch,omitempty" toml:"git_branch,omitempty"`
	GitCheckout *GitCheckoutSpec `json:"git_checkout,omitempty" toml:"git_checkout,omitempty"`
	GitWorktree *GitWorktreeSpec `json:"git_worktree,omitempty" toml:"git_worktree,omitempty"`
}

// SleepSpec describes a deterministic wait duration.
type SleepSpec struct {
	Duration string `json:"duration,omitempty" toml:"duration,omitempty"`
	Seconds  int    `json:"seconds,omitempty" toml:"seconds,omitempty"`
}

type GitFetchSpec struct {
	Remote string `json:"remote,omitempty" toml:"remote,omitempty"`
	Prune  bool   `json:"prune,omitempty" toml:"prune,omitempty"`
	Tags   bool   `json:"tags,omitempty" toml:"tags,omitempty"`
	All    bool   `json:"all,omitempty" toml:"all,omitempty"`
}

type GitPushSpec struct {
	Remote         string `json:"remote,omitempty" toml:"remote,omitempty"`
	Branch         string `json:"branch,omitempty" toml:"branch,omitempty"`
	Refspec        string `json:"refspec,omitempty" toml:"refspec,omitempty"`
	SetUpstream    bool   `json:"set_upstream,omitempty" toml:"set_upstream,omitempty"`
	ForceWithLease bool   `json:"force_with_lease,omitempty" toml:"force_with_lease,omitempty"`
	Tags           bool   `json:"tags,omitempty" toml:"tags,omitempty"`
}

type GitBranchSpec struct {
	Name        string `json:"name,omitempty" toml:"name,omitempty"`
	StartPoint  string `json:"start_point,omitempty" toml:"start_point,omitempty"`
	All         bool   `json:"all,omitempty" toml:"all,omitempty"`
	Delete      string `json:"delete,omitempty" toml:"delete,omitempty"`
	ForceDelete string `json:"force_delete,omitempty" toml:"force_delete,omitempty"`
}

type GitCheckoutSpec struct {
	Branch     string `json:"branch" toml:"branch"`
	Create     bool   `json:"create,omitempty" toml:"create,omitempty"`
	StartPoint string `json:"start_point,omitempty" toml:"start_point,omitempty"`
}

type GitWorktreeSpec struct {
	Path        string   `json:"path,omitempty" toml:"path,omitempty"`
	Branch      string   `json:"branch,omitempty" toml:"branch,omitempty"`
	StartPoint  string   `json:"start_point,omitempty" toml:"start_point,omitempty"`
	SparseMode  string   `json:"sparse_mode,omitempty" toml:"sparse_mode,omitempty"`
	Create      bool     `json:"create,omitempty" toml:"create,omitempty"`
	Detach      bool     `json:"detach,omitempty" toml:"detach,omitempty"`
	Force       bool     `json:"force,omitempty" toml:"force,omitempty"`
	List        bool     `json:"list,omitempty" toml:"list,omitempty"`
	Porcelain   bool     `json:"porcelain,omitempty" toml:"porcelain,omitempty"`
	Remove      string   `json:"remove,omitempty" toml:"remove,omitempty"`
	Prune       bool     `json:"prune,omitempty" toml:"prune,omitempty"`
	SparsePaths []string `json:"sparse_paths,omitempty" toml:"sparse_paths,omitempty"`
}

// WriteFilesSpec describes deterministic file creation from JSON context objects.
type WriteFilesSpec struct {
	Source      string `json:"source" toml:"source"`
	Root        string `json:"root,omitempty" toml:"root,omitempty"`
	DirName     string `json:"dir_name" toml:"dir_name"`
	FilenameKey string `json:"filename_key,omitempty" toml:"filename_key,omitempty"`
	TitleKey    string `json:"title_key,omitempty" toml:"title_key,omitempty"`
	SummaryKey  string `json:"summary_key,omitempty" toml:"summary_key,omitempty"`
	ContentKey  string `json:"content_key,omitempty" toml:"content_key,omitempty"`
}

// FormSpec describes a human input form for execution="human_input" steps.
type FormSpec struct {
	Title       string       `json:"title,omitempty" toml:"title,omitempty"`
	Description string       `json:"description,omitempty" toml:"description,omitempty"`
	SubmitLabel string       `json:"submit_label,omitempty" toml:"submit_label,omitempty"`
	Fields      []*FormField `json:"fields,omitempty" toml:"fields,omitempty"`
}

// FormField describes one input control in a human input form.
type FormField struct {
	Name        string   `json:"name" toml:"name"`
	Label       string   `json:"label" toml:"label"`
	Type        string   `json:"type" toml:"type"`
	Required    bool     `json:"required,omitempty" toml:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty" toml:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty" toml:"default,omitempty"`
	Options     []string `json:"options,omitempty" toml:"options,omitempty"`
	Help        string   `json:"help,omitempty" toml:"help,omitempty"`
}

// ValidateSpec describes runtime validation for step output.
type ValidateSpec struct {
	Format       string   `json:"format,omitempty" toml:"format,omitempty"`
	Required     []string `json:"required,omitempty" toml:"required,omitempty"`
	ItemRequired []string `json:"item_required,omitempty" toml:"item_required,omitempty"`
	MinItems     int      `json:"min_items,omitempty" toml:"min_items,omitempty"`
}

// RetrySpec defines first-class transient retry semantics.
type RetrySpec struct {
	MaxAttempts int    `json:"max_attempts,omitempty" toml:"max_attempts,omitempty"`
	OnExhausted string `json:"on_exhausted,omitempty" toml:"on_exhausted,omitempty"`
}

// LoopSpec defines iteration over a body of steps.
type LoopSpec struct {
	Count          int     `json:"count,omitempty" toml:"count,omitempty"`
	Until          string  `json:"until,omitempty" toml:"until,omitempty"`
	Max            int     `json:"max,omitempty" toml:"max,omitempty"`
	MaxExpr        string  `json:"max_expr,omitempty" toml:"max_expr,omitempty"`
	Range          string  `json:"range,omitempty" toml:"range,omitempty"`
	ForEach        string  `json:"for_each,omitempty" toml:"for_each,omitempty"`
	Var            string  `json:"var,omitempty" toml:"var,omitempty"`
	Parallel       bool    `json:"parallel,omitempty" toml:"parallel,omitempty"`
	MaxConcurrency int     `json:"max_concurrency,omitempty" toml:"max_concurrency,omitempty"`
	Body           []*Step `json:"body" toml:"body"`
}

// OnCompleteSpec defines actions triggered when a step completes.
type OnCompleteSpec struct {
	ForEach    string            `json:"for_each,omitempty" toml:"for_each,omitempty"`
	Bond       string            `json:"bond,omitempty" toml:"bond,omitempty"`
	Vars       map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
	Parallel   bool              `json:"parallel,omitempty" toml:"parallel,omitempty"`
	Sequential bool              `json:"sequential,omitempty" toml:"sequential,omitempty"`
}

// BranchRule defines parallel execution paths that rejoin.
type BranchRule struct {
	From  string   `json:"from" toml:"from"`
	Steps []string `json:"steps" toml:"steps"`
	Join  string   `json:"join" toml:"join"`
}

// GateRule defines a condition that must be satisfied before a step proceeds.
type GateRule struct {
	Before    string `json:"before" toml:"before"`
	Condition string `json:"condition" toml:"condition"`
}

// ComposeRules define how formulas can be bonded together.
type ComposeRules struct {
	BondPoints []*BondPoint  `json:"bond_points,omitempty" toml:"bond_points,omitempty"`
	Hooks      []*Hook       `json:"hooks,omitempty" toml:"hooks,omitempty"`
	Expand     []*ExpandRule `json:"expand,omitempty" toml:"expand,omitempty"`
	Map        []*MapRule    `json:"map,omitempty" toml:"map,omitempty"`
	Branch     []*BranchRule `json:"branch,omitempty" toml:"branch,omitempty"`
	Gate       []*GateRule   `json:"gate,omitempty" toml:"gate,omitempty"`
	Aspects    []string      `json:"aspects,omitempty" toml:"aspects,omitempty"`
}

// ExpandRule applies an expansion template to a single target step.
type ExpandRule struct {
	Target string            `json:"target" toml:"target"`
	With   string            `json:"with" toml:"with"`
	Vars   map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// MapRule applies an expansion template to all matching steps.
type MapRule struct {
	Select string            `json:"select" toml:"select"`
	With   string            `json:"with" toml:"with"`
	Vars   map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// BondPoint is a named attachment site for composition.
type BondPoint struct {
	ID          string `json:"id" toml:"id"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
	AfterStep   string `json:"after_step,omitempty" toml:"after_step,omitempty"`
	BeforeStep  string `json:"before_step,omitempty" toml:"before_step,omitempty"`
	Parallel    bool   `json:"parallel,omitempty" toml:"parallel,omitempty"`
}

// Hook defines automatic formula attachment based on conditions.
type Hook struct {
	Trigger string            `json:"trigger" toml:"trigger"`
	Attach  string            `json:"attach" toml:"attach"`
	At      string            `json:"at,omitempty" toml:"at,omitempty"`
	Vars    map[string]string `json:"vars,omitempty" toml:"vars,omitempty"`
}

// Pointcut defines a target pattern for advice application.
type Pointcut struct {
	Glob  string `json:"glob,omitempty" toml:"glob,omitempty"`
	Type  string `json:"type,omitempty" toml:"type,omitempty"`
	Label string `json:"label,omitempty" toml:"label,omitempty"`
}

// AdviceRule defines a step transformation rule.
type AdviceRule struct {
	Target string        `json:"target" toml:"target"`
	Before *AdviceStep   `json:"before,omitempty" toml:"before,omitempty"`
	After  *AdviceStep   `json:"after,omitempty" toml:"after,omitempty"`
	Around *AroundAdvice `json:"around,omitempty" toml:"around,omitempty"`
}

// AdviceStep defines a step to insert via advice.
type AdviceStep struct {
	ID          string            `json:"id" toml:"id"`
	Title       string            `json:"title,omitempty" toml:"title,omitempty"`
	Description string            `json:"description,omitempty" toml:"description,omitempty"`
	Type        string            `json:"type,omitempty" toml:"type,omitempty"`
	Args        map[string]string `json:"args,omitempty" toml:"args,omitempty"`
	Output      map[string]string `json:"output,omitempty" toml:"output,omitempty"`
}

// AroundAdvice wraps a target with before and after steps.
type AroundAdvice struct {
	Before []*AdviceStep `json:"before,omitempty" toml:"before,omitempty"`
	After  []*AdviceStep `json:"after,omitempty" toml:"after,omitempty"`
}

// RalphSpec defines an inline run/check retry loop.
// It is retained for backwards compatibility: TOML/JSON files may use
// `check` or `ralph` keys on a step, which are decoded into this type
// and then mapped to `Step.Retry` during unmarshalling.
type RalphSpec struct {
	MaxAttempts int             `json:"max_attempts,omitempty" toml:"max_attempts,omitempty"`
	Check       *RalphCheckSpec `json:"check,omitempty" toml:"check,omitempty"`
}

// RalphCheckSpec defines the validation step.
type RalphCheckSpec struct {
	Mode    string `json:"mode,omitempty" toml:"mode,omitempty"`
	Path    string `json:"path,omitempty" toml:"path,omitempty"`
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

// WaitsForSpec holds the parsed waits_for field.
type WaitsForSpec struct {
	Gate      string
	SpawnerID string
}

// WorkspaceSpec declares a formula-level execution workspace policy.
type WorkspaceSpec struct {
	Kind           string `json:"kind,omitempty" toml:"kind,omitempty"`
	Path           string `json:"path,omitempty" toml:"path,omitempty"`
	Cleanup        *bool  `json:"cleanup,omitempty" toml:"cleanup,omitempty"`
	Branch         string `json:"branch,omitempty" toml:"branch,omitempty"`
	Base           string `json:"base,omitempty" toml:"base,omitempty"`
	BranchSlugFrom string `json:"branch_slug_from,omitempty" toml:"branch_slug_from,omitempty"`
	BranchPrefix   string `json:"branch_prefix,omitempty" toml:"branch_prefix,omitempty"`
}

// StringPtr returns a pointer to s.
func StringPtr(s string) *string { return &s }
