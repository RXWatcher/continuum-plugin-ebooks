package smartcoll

import (
	"context"
	"testing"
	"time"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/backend"
)

// TestNormalize_AppliesAliasesAndDefaults pins the canonicalisation:
// authors → author (plural alias), sort_title → title, empty sort
// field → added_at, empty match → all.
func TestNormalize_AppliesAliasesAndDefaults(t *testing.T) {
	q := QueryDefinition{
		Match: "",
		Groups: []QueryGroup{{
			Match: "",
			Rules: []QueryRule{{Field: "Authors", Op: "IS", Value: "Sanderson"}},
		}},
		Sort: QuerySort{Field: "sort_title"},
	}
	n := q.Normalize()
	if n.Match != "all" {
		t.Errorf("top match default = %q", n.Match)
	}
	if n.Groups[0].Match != "all" {
		t.Errorf("group match default = %q", n.Groups[0].Match)
	}
	if n.Groups[0].Rules[0].Field != "author" {
		t.Errorf("authors alias not applied: %q", n.Groups[0].Rules[0].Field)
	}
	if n.Sort.Field != "title" {
		t.Errorf("sort alias not applied: %q", n.Sort.Field)
	}
}

// TestValidate_RejectsBadRules pins the validation surface: unknown
// field, unsupported op for field, personalized field without scope,
// negative limit.
func TestValidate_RejectsBadRules(t *testing.T) {
	cases := []struct {
		name string
		q    QueryDefinition
		err  string
	}{
		{
			name: "unknown field",
			q:    QueryDefinition{Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{{Field: "not_a_field", Op: "is", Value: "x"}}}}},
			err:  "not supported",
		},
		{
			name: "bad op for field",
			q:    QueryDefinition{Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{{Field: "year", Op: "contains", Value: 2024}}}}},
			err:  "not valid for field",
		},
		{
			name: "personalized field requires scope",
			q:    QueryDefinition{Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{{Field: "finished", Op: "is", Value: true}}}}},
			err:  "requires user scope",
		},
		{
			name: "negative limit",
			q:    QueryDefinition{Limit: intPtr(-5)},
			err:  "limit must be positive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.q.Validate(false)
			if err == nil || !contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want substring %q", err, tc.err)
			}
		})
	}
}

// TestEvaluate_FiltersByAuthor checks the most common rule path —
// author equality against the ebook summary's Authors[] slice.
func TestEvaluate_FiltersByAuthor(t *testing.T) {
	cands := []Candidate{
		{Item: backend.EbookSummary{ID: "a", Title: "Way of Kings", Authors: []string{"Brandon Sanderson"}}},
		{Item: backend.EbookSummary{ID: "b", Title: "Other Book", Authors: []string{"Robert Jordan"}}},
	}
	qd := QueryDefinition{
		Match: "all",
		Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{
			{Field: "author", Op: "is", Value: "Brandon Sanderson"},
		}}},
	}
	out := Evaluate(context.Background(), qd, cands, EvaluateOptions{})
	if len(out) != 1 || out[0].Item.ID != "a" {
		t.Fatalf("got %d hits, want exactly 'a': %+v", len(out), out)
	}
}

// TestEvaluate_FormatContains exercises the multi-format ebook
// scenario — a book with epub + pdf matches "format contains pdf".
func TestEvaluate_FormatContains(t *testing.T) {
	cands := []Candidate{
		{Item: backend.EbookSummary{ID: "a", Formats: []string{"epub", "pdf"}}},
		{Item: backend.EbookSummary{ID: "b", Formats: []string{"epub"}}},
	}
	qd := QueryDefinition{
		Match: "all",
		Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{
			{Field: "format", Op: "is", Value: "pdf"},
		}}},
	}
	out := Evaluate(context.Background(), qd, cands, EvaluateOptions{})
	if len(out) != 1 || out[0].Item.ID != "a" {
		t.Errorf("format=pdf: %+v", out)
	}
}

// TestEvaluate_SortByYearDesc + Limit confirms ordering + cap.
func TestEvaluate_SortByYearDesc(t *testing.T) {
	limit := 2
	cands := []Candidate{
		{Item: backend.EbookSummary{ID: "old", Year: 2010}},
		{Item: backend.EbookSummary{ID: "mid", Year: 2020}},
		{Item: backend.EbookSummary{ID: "new", Year: 2025}},
	}
	qd := QueryDefinition{
		Match: "all",
		Sort:  QuerySort{Field: "year", Order: "desc"},
		Limit: &limit,
	}
	out := Evaluate(context.Background(), qd, cands, EvaluateOptions{})
	if len(out) != 2 || out[0].Item.ID != "new" || out[1].Item.ID != "mid" {
		t.Errorf("sort by year desc + limit 2: %+v", out)
	}
}

// TestEvaluate_PersonalizedDropsWhenDisallowed mirrors the audiobook
// behaviour — finished:is:true rule is dropped when AllowPersonalized
// is false; the matchRule returns false on dropped rules, so the
// all-group fails and no candidate survives.
func TestEvaluate_PersonalizedDropsWhenDisallowed(t *testing.T) {
	cands := []Candidate{
		{Item: backend.EbookSummary{ID: "a"}, IsFinished: true},
		{Item: backend.EbookSummary{ID: "b"}},
	}
	qd := QueryDefinition{
		Match: "all",
		Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{
			{Field: "finished", Op: "is", Value: true},
		}}},
	}
	out := Evaluate(context.Background(), qd, cands, EvaluateOptions{AllowPersonalized: false})
	if len(out) != 0 {
		t.Errorf("personalized rule with AllowPersonalized=false should match nothing: %+v", out)
	}
	out = Evaluate(context.Background(), qd, cands, EvaluateOptions{AllowPersonalized: true})
	if len(out) != 1 || out[0].Item.ID != "a" {
		t.Errorf("with AllowPersonalized=true: %+v", out)
	}
}

// TestEvaluate_AddedInLast30Days exercises the in_last operator on
// the synthesized CreatedAt time (which the handler populates from
// store row metadata in the integration path).
func TestEvaluate_AddedInLast30Days(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	cands := []Candidate{
		{Item: backend.EbookSummary{ID: "old"}, CreatedAt: now.AddDate(0, 0, -60)},
		{Item: backend.EbookSummary{ID: "recent"}, CreatedAt: now.AddDate(0, 0, -10)},
	}
	qd := QueryDefinition{
		Match: "all",
		Groups: []QueryGroup{{Match: "all", Rules: []QueryRule{
			{Field: "added_at", Op: "in_last", Value: map[string]any{"value": 30, "unit": "days"}},
		}}},
	}
	out := Evaluate(context.Background(), qd, cands, EvaluateOptions{Now: now})
	if len(out) != 1 || out[0].Item.ID != "recent" {
		t.Errorf("in_last 30d: %+v", out)
	}
}

func intPtr(n int) *int { return &n }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
