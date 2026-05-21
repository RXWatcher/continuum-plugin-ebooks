package smartcoll

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/backend"
)

// Candidate is the input shape to Evaluate — one backend ebook plus
// the optional per-user reading state needed by personalized rules.
// CreatedAt is the EbookSummary's added-at timestamp; the
// EbookSummary itself doesn't carry that today, so the handler
// populates it from a future store-join.
type Candidate struct {
	Item            backend.EbookSummary
	CreatedAt       time.Time
	IsFinished      bool
	ProgressPct     float32
	CurrentPosition int
	LastReadAt      time.Time
	AnnotationCount int
}

type EvaluateOptions struct {
	AllowPersonalized bool
	UserSeed          string
	Now               time.Time
	AbandonedAfter    time.Duration
}

func Evaluate(ctx context.Context, qd QueryDefinition, candidates []Candidate, opts EvaluateOptions) []Candidate {
	_ = ctx
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.AbandonedAfter == 0 {
		opts.AbandonedAfter = 60 * 24 * time.Hour
	}
	qd = qd.Normalize()
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if matchDefinition(qd, c, opts) {
			out = append(out, c)
		}
	}
	sortCandidates(out, qd.Sort, opts)
	if qd.Limit != nil && *qd.Limit > 0 && *qd.Limit < len(out) {
		out = out[:*qd.Limit]
	}
	return out
}

func matchDefinition(qd QueryDefinition, c Candidate, opts EvaluateOptions) bool {
	if len(qd.Groups) == 0 {
		return true
	}
	if qd.Match == "any" {
		for _, g := range qd.Groups {
			if matchGroup(g, c, opts) {
				return true
			}
		}
		return false
	}
	for _, g := range qd.Groups {
		if !matchGroup(g, c, opts) {
			return false
		}
	}
	return true
}

func matchGroup(g QueryGroup, c Candidate, opts EvaluateOptions) bool {
	if len(g.Rules) == 0 {
		return true
	}
	if g.Match == "any" {
		for _, r := range g.Rules {
			if matchRule(r, c, opts) {
				return true
			}
		}
		return false
	}
	for _, r := range g.Rules {
		if !matchRule(r, c, opts) {
			return false
		}
	}
	return true
}

func matchRule(r QueryRule, c Candidate, opts EvaluateOptions) bool {
	def, ok := queryFieldDefs[r.Field]
	if !ok {
		return false
	}
	if def.personalized && !opts.AllowPersonalized {
		return false
	}
	switch r.Field {
	case "title":
		return cmpString(c.Item.Title, r)
	case "author":
		return cmpStringArray(c.Item.Authors, nil, r)
	case "series":
		return cmpString(c.Item.Series, r)
	case "genre":
		// Genres aren't on the summary; covered by detail. No data
		// → cmpStringArray handles is_not (matches) vs is/contains
		// (no-match) consistently.
		return cmpStringArray(nil, nil, r)
	case "tag":
		return cmpStringArray(nil, nil, r)
	case "format":
		return cmpStringArray(c.Item.Formats, nil, r)
	case "year":
		return cmpInt(c.Item.Year, r)
	case "rating":
		return cmpFloat(c.Item.Rating, r)
	case "language":
		return cmpString(c.Item.Language, r)
	case "publisher":
		return cmpString("", r) // not on summary
	case "isbn":
		return cmpString("", r) // not on summary
	case "added_at":
		return cmpTime(c.CreatedAt, r, opts.Now)
	case "finished":
		return cmpBool(c.IsFinished, r)
	case "in_progress":
		return cmpBool(!c.IsFinished && c.ProgressPct > 0, r)
	case "last_read":
		return cmpTime(c.LastReadAt, r, opts.Now)
	case "abandoned":
		abandoned := !c.IsFinished && c.ProgressPct > 0 &&
			!c.LastReadAt.IsZero() &&
			opts.Now.Sub(c.LastReadAt) >= opts.AbandonedAfter
		return cmpBool(abandoned, r)
	case "annotation_count":
		return cmpInt(c.AnnotationCount, r)
	}
	return false
}

func sortCandidates(items []Candidate, s QuerySort, opts EvaluateOptions) {
	descending := s.Order == "desc"
	switch s.Field {
	case "title":
		sortBy(items, descending, func(c Candidate) string { return strings.ToLower(c.Item.Title) })
	case "added_at":
		sortByTime(items, descending, func(c Candidate) time.Time { return c.CreatedAt })
	case "year":
		sortByInt(items, descending, func(c Candidate) int { return c.Item.Year })
	case "rating":
		sortByFloat(items, descending, func(c Candidate) float64 { return c.Item.Rating })
	case "progress":
		sortByFloat(items, descending, func(c Candidate) float64 { return float64(c.ProgressPct) })
	case "last_read":
		sortByTime(items, descending, func(c Candidate) time.Time { return c.LastReadAt })
	case "random":
		shuffleSeeded(items, opts.UserSeed)
	default:
		sortByTime(items, true, func(c Candidate) time.Time { return c.CreatedAt })
	}
}

// -------- Comparison helpers (mirror audiobook evaluator) --------

func cmpString(v string, r QueryRule) bool {
	rv, _ := r.Value.(string)
	switch r.Op {
	case "is":
		return strings.EqualFold(v, rv)
	case "is_not":
		return !strings.EqualFold(v, rv)
	case "contains":
		return strings.Contains(strings.ToLower(v), strings.ToLower(rv))
	}
	return false
}

func cmpStringArray(plain []string, fromRefs []string, r QueryRule) bool {
	rv, _ := r.Value.(string)
	rvLower := strings.ToLower(rv)
	all := make([]string, 0, len(plain)+len(fromRefs))
	all = append(all, plain...)
	all = append(all, fromRefs...)
	switch r.Op {
	case "is":
		for _, s := range all {
			if strings.EqualFold(s, rv) {
				return true
			}
		}
		return false
	case "is_not":
		for _, s := range all {
			if strings.EqualFold(s, rv) {
				return false
			}
		}
		return true
	case "contains":
		for _, s := range all {
			if strings.Contains(strings.ToLower(s), rvLower) {
				return true
			}
		}
		return false
	}
	return false
}

func cmpInt(v int, r QueryRule) bool {
	switch r.Op {
	case "is":
		return v == intValue(r.Value)
	case "is_not":
		return v != intValue(r.Value)
	case "gt":
		return v > intValue(r.Value)
	case "gte":
		return v >= intValue(r.Value)
	case "lt":
		return v < intValue(r.Value)
	case "lte":
		return v <= intValue(r.Value)
	case "between":
		lo, hi := intRangeValue(r.Value)
		return v >= lo && v <= hi
	}
	return false
}

func cmpFloat(v float64, r QueryRule) bool {
	switch r.Op {
	case "gt":
		return v > floatValue(r.Value)
	case "gte":
		return v >= floatValue(r.Value)
	case "lt":
		return v < floatValue(r.Value)
	case "lte":
		return v <= floatValue(r.Value)
	case "between":
		lo, hi := floatRangeValue(r.Value)
		return v >= lo && v <= hi
	}
	return false
}

func cmpBool(v bool, r QueryRule) bool {
	rv, _ := r.Value.(bool)
	if r.Op == "is" {
		return v == rv
	}
	return false
}

func cmpTime(v time.Time, r QueryRule, now time.Time) bool {
	if r.Op == "in_last" {
		d := parseDuration(r.Value)
		if d == 0 {
			return false
		}
		cutoff := now.Add(-d)
		return !v.IsZero() && v.After(cutoff)
	}
	rv := parseTime(r.Value)
	switch r.Op {
	case "gt":
		return !v.IsZero() && v.After(rv)
	case "gte":
		return !v.IsZero() && !v.Before(rv)
	case "lt":
		return !v.IsZero() && v.Before(rv)
	case "lte":
		return !v.IsZero() && !v.After(rv)
	case "between":
		lo, hi := parseTimeRange(r.Value)
		return !v.IsZero() && !v.Before(lo) && !v.After(hi)
	}
	return false
}

func intValue(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func intRangeValue(v any) (int, int) {
	if r, ok := v.([]any); ok && len(r) == 2 {
		return intValue(r[0]), intValue(r[1])
	}
	return 0, 0
}

func floatValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func floatRangeValue(v any) (float64, float64) {
	if r, ok := v.([]any); ok && len(r) == 2 {
		return floatValue(r[0]), floatValue(r[1])
	}
	return 0, 0
}

func parseTime(v any) time.Time {
	switch x := v.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t
		}
	case int64:
		return time.UnixMilli(x)
	case float64:
		return time.UnixMilli(int64(x))
	case int:
		return time.UnixMilli(int64(x))
	}
	return time.Time{}
}

func parseTimeRange(v any) (time.Time, time.Time) {
	if r, ok := v.([]any); ok && len(r) == 2 {
		return parseTime(r[0]), parseTime(r[1])
	}
	return time.Time{}, time.Time{}
}

func parseDuration(v any) time.Duration {
	switch x := v.(type) {
	case map[string]any:
		n := intValue(x["value"])
		unit, _ := x["unit"].(string)
		return durationFromUnit(n, strings.ToLower(unit))
	case int, int64, float64, json.Number:
		return durationFromUnit(intValue(v), "days")
	case string:
		return parseDurationString(x)
	}
	return 0
}

func durationFromUnit(n int, unit string) time.Duration {
	if n <= 0 {
		return 0
	}
	switch unit {
	case "hour", "hours", "h":
		return time.Duration(n) * time.Hour
	case "day", "days", "d", "":
		return time.Duration(n) * 24 * time.Hour
	case "week", "weeks", "w":
		return time.Duration(n) * 7 * 24 * time.Hour
	case "month", "months":
		return time.Duration(n) * 30 * 24 * time.Hour
	case "year", "years", "y":
		return time.Duration(n) * 365 * 24 * time.Hour
	}
	return 0
}

func parseDurationString(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	last := s[len(s)-1:]
	num := s[:len(s)-1]
	n := 0
	for _, r := range num {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return durationFromUnit(n, last)
}

// -------- Sort helpers --------

func sortBy(items []Candidate, descending bool, key func(Candidate) string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])
		if descending {
			return a > b
		}
		return a < b
	})
}

func sortByInt(items []Candidate, descending bool, key func(Candidate) int) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])
		if descending {
			return a > b
		}
		return a < b
	})
}

func sortByFloat(items []Candidate, descending bool, key func(Candidate) float64) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])
		if descending {
			return a > b
		}
		return a < b
	})
}

func sortByTime(items []Candidate, descending bool, key func(Candidate) time.Time) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])
		if descending {
			return a.After(b)
		}
		return a.Before(b)
	})
}

func shuffleSeeded(items []Candidate, seed string) {
	if seed == "" {
		seed = "random"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	r := rand.New(rand.NewSource(int64(h.Sum64()))) //nolint:gosec // ordering hint
	r.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}
