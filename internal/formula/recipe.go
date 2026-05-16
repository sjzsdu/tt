package formula

import (
	"fmt"
	"sort"
)

type RunOptions struct {
	Message string
	Session string
	Agent   string
	Model   string
}

// Recipe is the output of formula compilation — a flattened, ordered list of
// steps with namespaced IDs and all dependency edges. Variable placeholders
// ({{var}}) are preserved; substitution happens at instantiation time.
type Recipe struct {
	Name        string
	Description string
	Steps       []RecipeStep
	Deps        []RecipeDep
	Vars        map[string]*VarDef
	Phase       string
	Pour        bool
	RootOnly    bool
}

// RecipeStep represents a single step in a compiled recipe.
type RecipeStep struct {
	ID          string
	Title       string
	Description string
	Notes       string
	Type        string
	Priority    *int
	Labels      []string
	Assignee    string
	IsRoot      bool
	Metadata    map[string]string
	Gate        *RecipeGate
	Agent       *AgentConfig
	OutputKey   string
	InputCtx    []string
	Execution   string
	Condition   string
}

// RecipeGate describes an async coordination gate on a step.
type RecipeGate struct {
	Type    string
	ID      string
	Timeout string
}

// RecipeDep represents a dependency edge between two recipe steps.
type RecipeDep struct {
	StepID      string
	DependsOnID string
	Type        string
	Metadata    string
}

// RootStep returns the root step (always Steps[0]) or nil if empty.
func (r *Recipe) RootStep() *RecipeStep {
	if len(r.Steps) == 0 {
		return nil
	}
	return &r.Steps[0]
}

// StepByID returns the step with the given ID, or nil if not found.
func (r *Recipe) StepByID(id string) *RecipeStep {
	for i := range r.Steps {
		if r.Steps[i].ID == id {
			return &r.Steps[i]
		}
	}
	return nil
}

// VariableNames returns the sorted list of variable names defined in the formula.
func (r *Recipe) VariableNames() []string {
	names := make([]string, 0, len(r.Vars))
	for name := range r.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// toRecipe converts a resolved Formula into a Recipe.
func toRecipe(f *Formula) (*Recipe, error) {
	r := &Recipe{
		Name:        f.Formula,
		Description: f.Description,
		Vars:        f.Vars,
		Phase:       f.Phase,
		Pour:        f.Pour,
	}

	rootTitle := f.Formula
	if _, hasTitle := f.Vars["title"]; hasTitle {
		rootTitle = "{{title}}"
	}
	rootDesc := f.Description
	if _, hasDesc := f.Vars["desc"]; hasDesc {
		rootDesc = "{{desc}}"
	}

	rootOnly := (!f.Pour && f.Phase == "vapor") || len(f.Steps) == 0

	rootType := "task"
	if rootOnly {
		rootType = "task"
	}

	rootStep := RecipeStep{
		ID:          f.Formula,
		Title:       rootTitle,
		Description: rootDesc,
		Type:        rootType,
		IsRoot:      true,
	}
	defPriority := 2
	rootStep.Priority = &defPriority
	r.Steps = append(r.Steps, rootStep)

	r.RootOnly = rootOnly

	idMapping := make(map[string]string)
	flattenSteps(f.Steps, f.Formula, idMapping, &r.Steps, &r.Deps)
	collectRecipeDeps(f.Steps, idMapping, &r.Deps)

	return r, nil
}

func flattenSteps(steps []*Step, parentID string, idMapping map[string]string, out *[]RecipeStep, deps *[]RecipeDep) {
	for _, step := range steps {
		issueID := parentID + "." + step.ID
		idMapping[step.ID] = issueID

		stepType := step.Type
		if stepType == "" {
			stepType = "task"
		}
		if len(step.Children) > 0 {
			stepType = "epic"
		}

		rs := RecipeStep{
			ID:          issueID,
			Title:       step.Title,
			Description: step.Description,
			Notes:       step.Notes,
			Type:        stepType,
			Priority:    step.Priority,
			Labels:      step.Labels,
			Assignee:    step.Assignee,
			Metadata:    step.Metadata,
			Agent:       step.Agent,
			OutputKey:   step.OutputKey,
			InputCtx:    step.InputCtx,
			Execution:   step.Execution,
			Condition:   step.Condition,
		}

		if step.WaitsFor != "" {
			rs.Labels = append(rs.Labels, "gate:"+step.WaitsFor)
		}

		*out = append(*out, rs)

		*deps = append(*deps, RecipeDep{
			StepID:      issueID,
			DependsOnID: parentID,
			Type:        "parent-child",
		})

		if step.Gate != nil {
			gateID := parentID + ".gate-" + step.ID
			gateTitle := fmt.Sprintf("Gate: %s", step.Gate.Type)
			if step.Gate.ID != "" {
				gateTitle = fmt.Sprintf("Gate: %s %s", step.Gate.Type, step.Gate.ID)
			}

			gateStep := RecipeStep{
				ID:          gateID,
				Title:       gateTitle,
				Description: fmt.Sprintf("Async gate for step %s", step.ID),
				Type:        "gate",
				Gate: &RecipeGate{
					Type:    step.Gate.Type,
					ID:      step.Gate.ID,
					Timeout: step.Gate.Timeout,
				},
			}
			defP := 2
			gateStep.Priority = &defP
			*out = append(*out, gateStep)

			idMapping["gate-"+step.ID] = gateID

			*deps = append(*deps, RecipeDep{
				StepID:      gateID,
				DependsOnID: parentID,
				Type:        "parent-child",
			})
			*deps = append(*deps, RecipeDep{
				StepID:      issueID,
				DependsOnID: gateID,
				Type:        "blocks",
			})
		}

		if len(step.Children) > 0 {
			flattenSteps(step.Children, issueID, idMapping, out, deps)
		}
	}
}

func collectRecipeDeps(steps []*Step, idMapping map[string]string, deps *[]RecipeDep) {
	for _, step := range steps {
		issueID := idMapping[step.ID]

		for _, depID := range step.DependsOn {
			if depIssueID, ok := idMapping[depID]; ok {
				*deps = append(*deps, RecipeDep{
					StepID:      issueID,
					DependsOnID: depIssueID,
					Type:        "blocks",
				})
			}
		}

		for _, needID := range step.Needs {
			if needIssueID, ok := idMapping[needID]; ok {
				*deps = append(*deps, RecipeDep{
					StepID:      issueID,
					DependsOnID: needIssueID,
					Type:        "blocks",
				})
			}
		}

		if step.WaitsFor != "" {
			waitsForSpec := ParseWaitsFor(step.WaitsFor)
			if waitsForSpec != nil {
				spawnerStepID := waitsForSpec.SpawnerID
				if spawnerStepID == "" && len(step.Needs) > 0 {
					spawnerStepID = step.Needs[0]
				}
				if spawnerStepID != "" {
					if spawnerIssueID, ok := idMapping[spawnerStepID]; ok {
						metadata := fmt.Sprintf(`{"gate":%q}`, waitsForSpec.Gate)
						*deps = append(*deps, RecipeDep{
							StepID:      issueID,
							DependsOnID: spawnerIssueID,
							Type:        "waits-for",
							Metadata:    metadata,
						})
					}
				}
			}
		}

		if len(step.Children) > 0 {
			collectRecipeDeps(step.Children, idMapping, deps)
		}
	}
}
