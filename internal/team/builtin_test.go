package team

import (
	"strings"
	"testing"
)

func TestBuiltinTeamsIncludesProductReview(t *testing.T) {
	entries, err := BuiltinTeams()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name == "product-review" {
			if entry.Agents != 3 || entry.Title == "" {
				t.Fatalf("entry = %+v", entry)
			}
			return
		}
	}
	t.Fatalf("product-review missing from builtin teams: %+v", entries)
}

func TestBuiltinTeamsIncludesSoftwareDevelopment(t *testing.T) {
	entries, err := BuiltinTeams()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name != "software-development" {
			continue
		}
		if entry.Agents != 6 || entry.Title != "Software Development Team" {
			t.Fatalf("entry = %+v", entry)
		}
		definition, err := Load("software-development")
		if err != nil {
			t.Fatal(err)
		}
		if definition.Coordination.Finalizer != "delivery-lead" ||
			definition.Coordination.InitialHandoff != "delivery-lead" ||
			definition.Coordination.DeliveryOwner != "implementer" ||
			definition.Coordination.MaxHandoffTargets != 1 ||
			definition.Coordination.ReviewWaves != 0 ||
			definition.Memory.Maintainer != "delivery-lead" ||
			definition.Limits.MaxAgentTurns != 18 ||
			definition.Limits.MaxReviewTurnsPerAgent != 3 ||
			definition.Limits.MaxResponseChars != 3000 ||
			!definition.Verification.Enabled ||
			definition.Verification.Verifier != "test-engineer" {
			t.Fatalf("definition = %+v", definition)
		}
		for _, id := range []string{
			"requirements-researcher",
			"architect",
			"implementer",
			"code-reviewer",
			"test-engineer",
		} {
			if _, ok := definition.AgentByID(id); !ok {
				t.Fatalf("software-development missing agent %q", id)
			}
		}
		implementer, _ := definition.AgentByID("implementer")
		reviewer, _ := definition.AgentByID("code-reviewer")
		tester, _ := definition.AgentByID("test-engineer")
		lead, _ := definition.AgentByID("delivery-lead")
		if !strings.Contains(implementer.Prompt, "sole member authorized to modify") ||
			!strings.Contains(implementer.Prompt, "ask only @code-reviewer") ||
			!strings.Contains(implementer.Prompt, "plan-only answer") ||
			!strings.Contains(reviewer.Prompt, "@test-engineer") ||
			!strings.Contains(tester.Prompt, "@delivery-lead") ||
			!strings.Contains(lead.Prompt, "Never fabricate edits") ||
			!strings.Contains(lead.Prompt, "strongest coherent") ||
			!strings.Contains(lead.Prompt, "Do not emit") ||
			!strings.Contains(implementer.Prompt, "does not mean the fewest") {
			t.Fatalf("software-development delivery protocol is incomplete")
		}
		for _, member := range definition.Agents {
			if member.External == nil || member.External.Driver != "codex" {
				t.Fatalf("agent %q is not configured for external codex: %+v", member.ID, member.External)
			}
			if member.ID == "implementer" {
				if !strings.Contains(strings.Join(member.External.ExtraArgs, " "), "--full-auto") {
					t.Fatalf("implementer does not have codex write mode: %+v", member.External)
				}
				continue
			}
			if !strings.Contains(member.Prompt, "Never modify the working tree") {
				t.Fatalf("agent %q does not enforce the single-writer protocol", member.ID)
			}
			if !strings.Contains(strings.Join(member.External.ExtraArgs, " "), "read-only") {
				t.Fatalf("agent %q does not enforce codex read-only sandbox: %+v", member.ID, member.External)
			}
		}
		return
	}
	t.Fatalf("software-development missing from builtin teams: %+v", entries)
}

func TestBuiltinTeamContentHandlesUnknownAndEmptyNames(t *testing.T) {
	for _, name := range []string{"", "missing-team"} {
		data, ok, err := BuiltinTeamContent(name)
		if err != nil {
			t.Fatal(err)
		}
		if ok || data != nil {
			t.Fatalf("BuiltinTeamContent(%q) = %q, %v", name, data, ok)
		}
	}
}
