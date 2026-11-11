package assistant

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"glucolens/internal/dataset"
	"glucolens/internal/query"
)

// Filters reach the model as plain strings ("age >= 50", "gender = Female",
// "smoking_status in Never, Former"). Open models produce nested filter
// objects unreliably, and Groq rejects a tool call whose arguments do not
// match the schema, so a flat list of strings is the robust shape.

const filterHelp = "write each filter as text such as 'gender = Female', 'age >= 50' or 'smoking_status in Never, Former'"

func parseFilters(ds *dataset.Dataset, raw []string) ([]query.Filter, error) {
	out := make([]query.Filter, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r) == "" {
			continue
		}
		f, err := parseFilter(ds, r)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func parseFilter(ds *dataset.Dataset, raw string) (query.Filter, error) {
	s := strings.TrimSpace(raw)
	col, op, val, ok := splitCondition(s)
	if !ok {
		return query.Filter{}, fmt.Errorf("could not read filter %q: %s", raw, filterHelp)
	}
	c, found := ds.Column(col)
	if !found {
		c, found = ds.Column(strings.ToLower(strings.ReplaceAll(col, " ", "_")))
	}
	if !found {
		return query.Filter{}, fmt.Errorf("filter %q: unknown column %q; use a column name from the instructions", raw, col)
	}
	f := query.Filter{Column: c.Name}
	val = trimValue(val)

	switch op {
	case ">=", ">", "<=", "<":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil || !c.IsNumeric() {
			return query.Filter{}, fmt.Errorf("filter %q: %s needs a number after %s", raw, c.Name, op)
		}
		switch op {
		case ">=":
			f.Min = &v
		case ">":
			x := math.Nextafter(v, math.Inf(1))
			f.Min = &x
		case "<=":
			f.Max = &v
		case "<":
			x := math.Nextafter(v, math.Inf(-1))
			f.Max = &x
		}
		return f, nil
	}

	// =, :, in
	if c.Kind == dataset.KindContinuous {
		if lo, hi, isRange := numericRange(val); isRange {
			f.Min, f.Max = &lo, &hi
			return f, nil
		}
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return query.Filter{}, fmt.Errorf("filter %q: %s is a number; use >= and <= to choose a range", raw, c.Name)
		}
		f.Min, f.Max = &v, &v
		return f, nil
	}
	for _, part := range strings.Split(val, ",") {
		if p := trimValue(part); p != "" {
			f.Equals = append(f.Equals, p)
		}
	}
	if len(f.Equals) == 0 {
		return query.Filter{}, fmt.Errorf("filter %q has no value: %s", raw, filterHelp)
	}
	return f, nil
}

// splitCondition finds the column, operator and value in "column op value".
func splitCondition(s string) (col, op, val string, ok bool) {
	lower := strings.ToLower(s)
	if i := strings.Index(lower, " in "); i > 0 {
		return strings.TrimSpace(s[:i]), "in", strings.TrimSpace(s[i+4:]), true
	}
	i := strings.IndexAny(s, "<>=:!")
	if i <= 0 {
		return "", "", "", false
	}
	op = s[i : i+1]
	rest := s[i+1:]
	if strings.HasPrefix(rest, "=") {
		op += "="
		rest = rest[1:]
	}
	switch op {
	case "==":
		op = "="
	case "!", "!=":
		return "", "", "", false // exclusions are not supported; list the values to keep
	}
	return strings.TrimSpace(s[:i]), op, strings.TrimSpace(rest), true
}

func trimValue(v string) string {
	return strings.Trim(strings.TrimSpace(v), "[](){}'\"` ")
}

// numericRange reads "40-50" or "40 to 50".
func numericRange(v string) (float64, float64, bool) {
	for _, sep := range []string{" to ", "–", "-"} {
		if i := strings.Index(v, sep); i > 0 {
			lo, err1 := strconv.ParseFloat(strings.TrimSpace(v[:i]), 64)
			hi, err2 := strconv.ParseFloat(strings.TrimSpace(v[i+len(sep):]), 64)
			if err1 == nil && err2 == nil && lo <= hi {
				return lo, hi, true
			}
		}
	}
	return 0, 0, false
}
