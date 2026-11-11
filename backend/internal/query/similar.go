package query

import (
	"math"
	"slices"
	"sort"

	"glucolens/internal/dataset"
)

// Match modes for SimilarRequest.
const (
	MatchAll       = "all"
	MatchLifestyle = "lifestyle"
)

// SimilarRequest finds the people most like the given values.
type SimilarRequest struct {
	Values   map[string]any `json:"values"`
	K        int            `json:"k,omitempty"`
	MatchOn  string         `json:"matchOn,omitempty"`
	Outcomes []Outcome      `json:"outcomes,omitempty"`
}

// SimilarOutcome compares an outcome's rate among similar people with the
// whole dataset.
type SimilarOutcome struct {
	Outcome OutcomeInfo `json:"outcome"`
	Similar Rate        `json:"similar"`
	Overall Rate        `json:"overall"`
	Ratio   float64     `json:"ratio"`
}

// SimilarValue describes one column among the similar people.
type SimilarValue struct {
	Column      string       `json:"column"`
	Label       string       `json:"label"`
	Kind        dataset.Kind `json:"kind"`
	Matched     bool         `json:"matched"`
	Your        any          `json:"your,omitempty"`
	YourDisplay string       `json:"yourDisplay,omitempty"`
	Median      *float64     `json:"median,omitempty"`
	P25         *float64     `json:"p25,omitempty"`
	P75         *float64     `json:"p75,omitempty"`
	Top         string       `json:"top,omitempty"`
	TopShare    *float64     `json:"topShare,omitempty"`
}

// SimilarResponse summarises the K nearest people.
type SimilarResponse struct {
	K        int               `json:"k"`
	MatchOn  string            `json:"matchOn"`
	Features []string          `json:"features"`
	Outcomes []SimilarOutcome  `json:"outcomes"`
	Values   []SimilarValue    `json:"values"`
	Invalid  map[string]string `json:"invalid,omitempty"`
}

var lifestyleRoles = map[string]bool{
	dataset.RoleDemographic: true, dataset.RoleLifestyle: true,
	dataset.RoleHistory: true, dataset.RoleBody: true,
}

// Similar finds the K people closest to the given values (standardised
// distance on numbers, mismatch on categories) and reports their outcomes.
func (e *Engine) Similar(req SimilarRequest) (*SimilarResponse, error) {
	vals, invalid := e.ds.NormalizeAll(req.Values)
	matchOn := req.MatchOn
	if matchOn == "" {
		matchOn = MatchAll
	}
	if matchOn != MatchAll && matchOn != MatchLifestyle {
		return nil, invalidf("matchOn must be %q or %q", MatchAll, MatchLifestyle)
	}

	type feature struct {
		col   *dataset.Column
		val   dataset.Value
		scale float64
	}
	var feats []feature
	used := map[string]bool{}
	for _, c := range e.ds.Columns {
		v, ok := vals[c.Name]
		if !ok || c.Role == dataset.RoleOutcome || !c.Analysable() {
			continue
		}
		if matchOn == MatchLifestyle && !lifestyleRoles[c.Role] {
			continue
		}
		f := feature{col: c, val: v}
		if c.IsNumeric() && c.Kind != dataset.KindBinary {
			f.scale = 1
			if c.Stats != nil && c.Stats.Std > 0 {
				f.scale = c.Stats.Std
			}
		}
		feats = append(feats, f)
		used[c.Name] = true
	}
	if len(feats) < 2 {
		return nil, invalidf("enter at least two of your values to find similar people")
	}

	k := req.K
	if k <= 0 {
		k = 500
	}
	k = min(max(k, 20), 5000, e.ds.Rows)

	dist := make([]float64, e.ds.Rows)
	for i := range dist {
		d := 0.0
		for _, f := range feats {
			if f.scale > 0 {
				x := f.col.Num(i)
				if math.IsNaN(x) {
					d++
					continue
				}
				z := (x - f.val.Num) / f.scale
				d += z * z
				continue
			}
			if f.col.Level(i) != f.val.Level {
				d++
			}
		}
		dist[i] = d
	}
	idx := make([]int32, e.ds.Rows)
	for i := range idx {
		idx[i] = int32(i)
	}
	slices.SortFunc(idx, func(a, b int32) int {
		switch {
		case dist[a] < dist[b]:
			return -1
		case dist[a] > dist[b]:
			return 1
		}
		return int(a - b)
	})
	nearest := idx[:k]

	resp := &SimilarResponse{K: k, MatchOn: matchOn, Invalid: invalid}
	for _, f := range feats {
		resp.Features = append(resp.Features, f.col.Name)
	}

	outcomes := req.Outcomes
	if len(outcomes) == 0 {
		for _, p := range e.ds.Outcomes {
			if !used[p.Outcome.Column] {
				outcomes = append(outcomes, Outcome{Preset: p.ID})
			}
		}
	}
	for _, o := range outcomes {
		co, err := e.compileOutcome(o)
		if err != nil {
			return nil, err
		}
		sim := countEvents(co, nearest)
		all := countEvents(co, e.allRows)
		so := SimilarOutcome{Outcome: co.info, Similar: sim, Overall: all}
		if all.Value > 0 {
			so.Ratio = round(sim.Value/all.Value, 3)
		}
		resp.Outcomes = append(resp.Outcomes, so)
	}

	for _, c := range e.ds.Columns {
		if !c.Analysable() {
			continue
		}
		sv := SimilarValue{Column: c.Name, Label: c.Label, Kind: c.Kind, Matched: used[c.Name]}
		if v, ok := vals[c.Name]; ok {
			sv.Your, sv.YourDisplay = v.JSON(), v.Display
		}
		if c.IsNumeric() && c.Kind != dataset.KindBinary {
			xs := make([]float64, 0, len(nearest))
			for _, r := range nearest {
				if x := c.Num(int(r)); !math.IsNaN(x) {
					xs = append(xs, x)
				}
			}
			sort.Float64s(xs)
			if len(xs) > 0 {
				sv.Median = ptr(dataset.Quantile(xs, 0.5))
				sv.P25 = ptr(dataset.Quantile(xs, 0.25))
				sv.P75 = ptr(dataset.Quantile(xs, 0.75))
			}
		}
		if c.HasLevels() {
			counts := make([]int, len(c.Levels))
			n := 0
			for _, r := range nearest {
				if l := c.Level(int(r)); l >= 0 {
					counts[l]++
					n++
				}
			}
			best := 0
			for l, cnt := range counts {
				if cnt > counts[best] {
					best = l
				}
			}
			if n > 0 {
				sv.Top = c.Levels[best]
				sv.TopShare = ptr(round(float64(counts[best])/float64(n), 4))
			}
		}
		resp.Values = append(resp.Values, sv)
	}
	return resp, nil
}

func countEvents(co *compiledOutcome, rows []int32) Rate {
	events, n := 0, 0
	for _, r := range rows {
		ev, known := co.event(int(r))
		if !known {
			continue
		}
		n++
		if ev {
			events++
		}
	}
	return newRate(events, n)
}
