package query

import (
	"math"
	"strings"

	"glucolens/internal/dataset"
)

// grouping assigns each row to one of a small number of labelled groups.
type grouping struct {
	col    *dataset.Column
	labels []string
	assign func(i int) int // group index, or -1 when the value is missing
	// source says how the groups were formed: levels, groups, bands or quintiles.
	source string
}

// groupingFor builds groups for a column: its levels for yes/no, discrete
// and categorical columns, otherwise the overlay's groups, its general
// clinical bands, or quintiles of the data.
func (e *Engine) groupingFor(c *dataset.Column) (*grouping, error) {
	if !c.Analysable() {
		return nil, invalidf("column %q cannot be used to form groups", c.Name)
	}
	if len(c.Groups) > 0 && c.IsNumeric() {
		return rangeGrouping(c, c.Groups, "groups"), nil
	}
	if c.HasLevels() {
		if len(c.Levels) > maxGroups {
			return nil, invalidf("%s has %d values; that is too many groups (limit %d)", c.Label, len(c.Levels), maxGroups)
		}
		return &grouping{col: c, labels: c.Levels, assign: c.Level, source: "levels"}, nil
	}
	if bands := c.GeneralBands(); len(bands) >= 2 {
		ranges := make([]dataset.Range, len(bands))
		for i, b := range bands {
			ranges[i] = dataset.Range{Label: b.Label, Min: b.Min, Max: b.Max}
		}
		return rangeGrouping(c, ranges, "bands"), nil
	}
	return rangeGrouping(c, quintileRanges(c), "quintiles"), nil
}

func rangeGrouping(c *dataset.Column, ranges []dataset.Range, source string) *grouping {
	labels := make([]string, len(ranges))
	for i, r := range ranges {
		labels[i] = r.Label
	}
	return &grouping{
		col:    c,
		labels: labels,
		source: source,
		assign: func(i int) int {
			v := c.Num(i)
			if math.IsNaN(v) {
				return -1
			}
			for k, r := range ranges {
				if r.Contains(v) {
					return k
				}
			}
			return -1
		},
	}
}

// quintileRanges splits a numeric column at its 20th, 40th, 60th and 80th
// percentiles, merging cut points that coincide.
func quintileRanges(c *dataset.Column) []dataset.Range {
	sorted := c.Sorted()
	if len(sorted) == 0 {
		return nil
	}
	var cuts []float64
	for _, p := range []float64{0.2, 0.4, 0.6, 0.8} {
		q := roundTo(dataset.Quantile(sorted, p), c.Decimals)
		if q <= sorted[0] || q > sorted[len(sorted)-1] {
			continue
		}
		if len(cuts) == 0 || q > cuts[len(cuts)-1] {
			cuts = append(cuts, q)
		}
	}
	if len(cuts) == 0 {
		return []dataset.Range{{Label: "All"}}
	}
	ranges := make([]dataset.Range, 0, len(cuts)+1)
	for i := 0; i <= len(cuts); i++ {
		var r dataset.Range
		switch {
		case i == 0:
			hi := cuts[0]
			r = dataset.Range{Label: "Under " + dataset.FormatNumber(hi, c.Decimals), Max: &hi}
		case i == len(cuts):
			lo := cuts[i-1]
			r = dataset.Range{Label: dataset.FormatNumber(lo, c.Decimals) + "+", Min: &lo}
		default:
			lo, hi := cuts[i-1], cuts[i]
			r = dataset.Range{Label: dataset.FormatNumber(lo, c.Decimals) + "–" + dataset.FormatNumber(hi, c.Decimals), Min: &lo, Max: &hi}
		}
		ranges = append(ranges, r)
	}
	return ranges
}

func roundTo(f float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(f*p) / p
}

// phrase describes group idx for sentences: "with family history of
// diabetes" for yes/no columns, "65+ years" for numeric groups, otherwise
// the group label.
func (g *grouping) phrase(idx int) string {
	label := g.labels[idx]
	c := g.col
	if c.Kind == dataset.KindBinary {
		if idx == 1 {
			return "with " + lowerFirst(c.Label)
		}
		return "without " + lowerFirst(c.Label)
	}
	if (g.source == "groups" || g.source == "quintiles") && c.Unit != "" &&
		c.Unit != "%" && !strings.HasPrefix(c.Unit, "/") && label != "" {
		last := label[len(label)-1]
		if last >= '0' && last <= '9' || last == '+' {
			return label + " " + c.Unit
		}
	}
	return label
}

// referenceIndex picks the comparison group: the requested label, the
// column's configured reference, or the first group with people in it.
func (g *grouping) referenceIndex(requested string, counts []int) (int, error) {
	find := func(label string) int {
		for i, l := range g.labels {
			if strings.EqualFold(strings.TrimSpace(label), l) {
				return i
			}
		}
		return -1
	}
	if requested != "" {
		idx := find(requested)
		if idx < 0 {
			return -1, invalidf("reference %q is not a group of %s (groups: %s)", requested, g.col.Name, strings.Join(g.labels, ", "))
		}
		return idx, nil
	}
	if g.col.Reference != "" {
		if idx := find(g.col.Reference); idx >= 0 && counts[idx] > 0 {
			return idx, nil
		}
	}
	for i, n := range counts {
		if n > 0 {
			return i, nil
		}
	}
	return 0, nil
}
