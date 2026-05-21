// Package smartcoll implements the rule-based Smart Collection DSL
// for the ebooks plugin. Ported from the audiobooks plugin's
// internal/smartcoll with the same envelope shape and operator set,
// adapted to the ebook field domain (title, author, series, genre,
// year, rating, language, publisher, isbn, format, tags). The query
// definition stored in smart_collection.query_def is wire-compatible
// with the audiobooks plugin's DSL except for the field/sort catalogs.
package smartcoll

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const defaultSortField = "added_at"

type QueryDefinition struct {
	LibraryIDs []int64      `json:"library_ids,omitempty"`
	Match      string       `json:"match"`
	Groups     []QueryGroup `json:"groups"`
	Sort       QuerySort    `json:"sort"`
	Limit      *int         `json:"limit,omitempty"`
}

type QueryGroup struct {
	Match string      `json:"match"`
	Rules []QueryRule `json:"rules"`
}

type QueryRule struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type QuerySort struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type queryFieldDef struct {
	validOps     map[string]bool
	isArray      bool
	personalized bool
}

type querySortDef struct {
	defaultOrder string
	personalized bool
}

var queryFieldAliases = map[string]string{
	"authors": "author",
	"genres":  "genre",
	"formats": "format",
}

var querySortAliases = map[string]string{
	"sort_title":     "title",
	"recently_added": "added_at",
}

// queryFieldDefs is the ebook-domain rule field catalog. Diffs vs the
// audiobook catalog: no narrator/duration_seconds; adds format
// (multi-value array — books usually have epub + maybe pdf + maybe
// mobi), language, tags, isbn. Personalized fields target reading
// progress rather than listening — finished, in_progress, last_read,
// annotation_count.
var queryFieldDefs = map[string]queryFieldDef{
	"title":             {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}},
	"author":            {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}, isArray: true},
	"series":            {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}},
	"genre":             {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}, isArray: true},
	"tag":               {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}, isArray: true},
	"format":            {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}, isArray: true},
	"year":              {validOps: map[string]bool{"is": true, "is_not": true, "gt": true, "gte": true, "lt": true, "lte": true, "between": true}},
	"rating":            {validOps: map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true, "between": true}},
	"language":          {validOps: map[string]bool{"is": true, "is_not": true}},
	"publisher":         {validOps: map[string]bool{"is": true, "is_not": true, "contains": true}},
	"isbn":              {validOps: map[string]bool{"is": true, "is_not": true}},
	"added_at":          {validOps: map[string]bool{"gt": true, "lt": true, "between": true, "in_last": true}},
	"finished":          {validOps: map[string]bool{"is": true}, personalized: true},
	"in_progress":       {validOps: map[string]bool{"is": true}, personalized: true},
	"last_read":         {validOps: map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true, "between": true, "in_last": true}, personalized: true},
	"abandoned":         {validOps: map[string]bool{"is": true}, personalized: true},
	"annotation_count":  {validOps: map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true, "between": true}, personalized: true},
}

var querySortDefs = map[string]querySortDef{
	"title":     {defaultOrder: "asc"},
	"added_at":  {defaultOrder: "desc"},
	"year":      {defaultOrder: "desc"},
	"rating":    {defaultOrder: "desc"},
	"random":    {defaultOrder: "asc"},
	"progress":  {defaultOrder: "desc", personalized: true},
	"last_read": {defaultOrder: "desc", personalized: true},
}

func (q QueryDefinition) Normalize() QueryDefinition {
	out := q
	out.Match = normalizeMatch(out.Match)
	out.LibraryIDs = normalizeLibraryIDs(out.LibraryIDs)
	out.Sort = NormalizeSort(out.Sort)
	if out.Groups == nil {
		out.Groups = []QueryGroup{}
	}
	for i := range out.Groups {
		out.Groups[i].Match = normalizeMatch(out.Groups[i].Match)
		if out.Groups[i].Rules == nil {
			out.Groups[i].Rules = []QueryRule{}
		}
		for j := range out.Groups[i].Rules {
			field := strings.ToLower(strings.TrimSpace(out.Groups[i].Rules[j].Field))
			if canon, ok := queryFieldAliases[field]; ok {
				field = canon
			}
			out.Groups[i].Rules[j].Field = field
			out.Groups[i].Rules[j].Op = strings.ToLower(strings.TrimSpace(out.Groups[i].Rules[j].Op))
		}
	}
	return out
}

func (q QueryDefinition) Validate(allowPersonalized bool) error {
	n := q.Normalize()
	for _, id := range n.LibraryIDs {
		if id <= 0 {
			return fmt.Errorf("library_ids must contain positive ids")
		}
	}
	if n.Match != "all" && n.Match != "any" {
		return fmt.Errorf("match must be 'all' or 'any'")
	}
	for i, g := range n.Groups {
		if g.Match != "all" && g.Match != "any" {
			return fmt.Errorf("groups[%d].match must be 'all' or 'any'", i)
		}
		for j, r := range g.Rules {
			def, ok := queryFieldDefs[r.Field]
			if !ok {
				return fmt.Errorf("groups[%d].rules[%d].field %q is not supported", i, j, r.Field)
			}
			if def.personalized && !allowPersonalized {
				return fmt.Errorf("groups[%d].rules[%d].field %q requires user scope", i, j, r.Field)
			}
			if !def.validOps[r.Op] {
				return fmt.Errorf("groups[%d].rules[%d].op %q is not valid for field %q", i, j, r.Op, r.Field)
			}
		}
	}
	if n.Sort.Field != "" {
		def, ok := querySortDefs[n.Sort.Field]
		if !ok {
			return fmt.Errorf("sort.field %q is not supported", n.Sort.Field)
		}
		if def.personalized && !allowPersonalized {
			return fmt.Errorf("sort.field %q requires user scope", n.Sort.Field)
		}
	}
	if n.Sort.Order != "" && n.Sort.Order != "asc" && n.Sort.Order != "desc" {
		return fmt.Errorf("sort.order must be 'asc' or 'desc'")
	}
	if n.Limit != nil && *n.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	return nil
}

func NormalizeSort(s QuerySort) QuerySort {
	out := QuerySort{
		Field: strings.ToLower(strings.TrimSpace(s.Field)),
		Order: strings.ToLower(strings.TrimSpace(s.Order)),
	}
	if canon, ok := querySortAliases[out.Field]; ok {
		out.Field = canon
	}
	if out.Field == "" {
		out.Field = defaultSortField
	}
	if out.Order == "" {
		if def, ok := querySortDefs[out.Field]; ok {
			out.Order = def.defaultOrder
		}
	}
	return out
}

func (q QueryDefinition) MarshalJSON() ([]byte, error) {
	type alias QueryDefinition
	return json.Marshal(alias(q.Normalize()))
}

func normalizeMatch(m string) string {
	n := strings.ToLower(strings.TrimSpace(m))
	if n == "" {
		return "all"
	}
	return n
}

func normalizeLibraryIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			out = append(out, id)
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
