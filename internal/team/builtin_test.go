package team

import "testing"

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
