package query

import (
	"math"
	"strings"

	"glucolens/internal/dataset"
)

// Filter keeps rows whose column value is one of Equals (for columns with
// levels) or lies within [Min, Max] (for numeric columns, both inclusive).
type Filter struct {
	Column string   `json:"column"`
	Equals []string `json:"equals,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

type compiledFilter struct {
	col     *dataset.Column
	allowed []bool // level filter, indexed by level
	lo, hi  float64
	ranged  bool
}

func (f compiledFilter) match(i int) bool {
	if f.allowed != nil {
		l := f.col.Level(i)
		if l < 0 || !f.allowed[l] {
			return false
		}
	}
	if f.ranged {
		v := f.col.Num(i)
		if math.IsNaN(v) || v < f.lo || v > f.hi {
			return false
		}
	}
	return true
}

func (e *Engine) compileFilters(filters []Filter) ([]compiledFilter, error) {
	var out []compiledFilter
	for _, f := range filters {
		if len(f.Equals) == 0 && f.Min == nil && f.Max == nil {
			continue // an empty filter keeps everyone
		}
		c, err := e.column(f.Column, "filter column")
		if err != nil {
			return nil, err
		}
		cf := compiledFilter{col: c}
		if len(f.Equals) > 0 {
			if !c.HasLevels() {
				return nil, invalidf("filter on %s: it is a continuous number, so use min/max instead of equals", c.Name)
			}
			cf.allowed = make([]bool, len(c.Levels))
			for _, v := range f.Equals {
				idx, ok := c.FindLevel(v)
				if !ok {
					return nil, invalidf("filter on %s: %q is not a valid value (valid: %s)", c.Name, v, strings.Join(c.Levels, ", "))
				}
				cf.allowed[idx] = true
			}
		}
		if f.Min != nil || f.Max != nil {
			if !c.IsNumeric() {
				return nil, invalidf("filter on %s: it is categorical, so use equals instead of min/max", c.Name)
			}
			cf.ranged = true
			cf.lo, cf.hi = math.Inf(-1), math.Inf(1)
			if f.Min != nil {
				cf.lo = *f.Min
			}
			if f.Max != nil {
				cf.hi = *f.Max
			}
			if cf.lo > cf.hi {
				return nil, invalidf("filter on %s: min (%g) is greater than max (%g)", c.Name, cf.lo, cf.hi)
			}
		}
		out = append(out, cf)
	}
	return out, nil
}

// Select returns the indexes of rows matching every filter. With no
// filters it returns all rows; callers must not modify the result.
func (e *Engine) Select(filters []Filter) ([]int32, error) {
	compiled, err := e.compileFilters(filters)
	if err != nil {
		return nil, err
	}
	if len(compiled) == 0 {
		return e.allRows, nil
	}
	out := make([]int32, 0, len(e.allRows)/4)
	for _, r := range e.allRows {
		i := int(r)
		keep := true
		for _, f := range compiled {
			if !f.match(i) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, r)
		}
	}
	return out, nil
}

// DescribeFilters renders filters as a short phrase such as
// "Gender: Female · Age 45–55 years".
func (e *Engine) DescribeFilters(filters []Filter) string {
	var parts []string
	for _, f := range filters {
		if len(f.Equals) == 0 && f.Min == nil && f.Max == nil {
			continue
		}
		c, ok := e.ds.Column(f.Column)
		if !ok {
			continue
		}
		if len(f.Equals) > 0 {
			vals := make([]string, 0, len(f.Equals))
			for _, v := range f.Equals {
				if idx, ok := c.FindLevel(v); ok {
					vals = append(vals, c.Levels[idx])
				}
			}
			parts = append(parts, c.Short+": "+strings.Join(vals, " or "))
		}
		if f.Min != nil || f.Max != nil {
			parts = append(parts, c.Short+" "+describeRange(c, f.Min, f.Max))
		}
	}
	if len(parts) == 0 {
		return "Everyone"
	}
	return strings.Join(parts, " · ")
}

func describeRange(c *dataset.Column, lo, hi *float64) string {
	switch {
	case lo != nil && hi != nil:
		return dataset.FormatNumber(*lo, c.Decimals) + "–" + c.FormatValue(*hi)
	case lo != nil:
		return "≥ " + c.FormatValue(*lo)
	case hi != nil:
		return "≤ " + c.FormatValue(*hi)
	}
	return ""
}
