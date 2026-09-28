package retrieval

import "testing"

func TestRRFPrefersOverlap(t *testing.T) {
	kw := []rankedItem{{ID: 1, Rank: 1}, {ID: 2, Rank: 2}}
	vec := []rankedItem{{ID: 1, Rank: 2}, {ID: 3, Rank: 1}}
	got := rrfMerge([][]rankedItem{kw, vec})
	if len(got) == 0 || got[0].ID != 1 {
		t.Fatalf("expected id 1 first, got %+v", got)
	}
}

func TestRRFMissingListAddsNothing(t *testing.T) {
	only := rrfMerge([][]rankedItem{{{ID: 7, Rank: 1}}})
	if len(only) != 1 || only[0].ID != 7 {
		t.Fatalf("%+v", only)
	}
}
