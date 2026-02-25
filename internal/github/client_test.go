package github

import (
	"testing"
)

func TestParseClosedPRBranches(t *testing.T) {
	input := []byte(`[
		{"headRefName": "wt-1", "state": "MERGED"},
		{"headRefName": "wt-2", "state": "CLOSED"},
		{"headRefName": "wt-3", "state": "OPEN"},
		{"headRefName": "feature-x", "state": "MERGED"},
		{"headRefName": "wt-10", "state": "OPEN"}
	]`)

	got, err := ParseClosedPRBranches(input)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"wt-1", "wt-2", "feature-x"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %v", len(got), len(want), got)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestParseClosedPRBranchesEmpty(t *testing.T) {
	got, err := ParseClosedPRBranches([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestParseClosedPRBranchesInvalid(t *testing.T) {
	_, err := ParseClosedPRBranches([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
